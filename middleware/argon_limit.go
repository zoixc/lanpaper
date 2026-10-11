// SPDX-License-Identifier: MIT

package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"net/http"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"lanpaper/config"
)

// argonConcurrency bounds how many Argon2id evaluations run at once. Each one
// allocates argonMemory (64 MiB). Without a bound, N concurrent wrong
// credentials cost N x 64 MiB and an unauthenticated client can exhaust the
// memory of the process (measured: 10 requests -> 668 MB RSS, 100 -> OOM kill).
// Two slots keep the peak near 128 MiB. The hash parameters do not change.
const argonConcurrency = 2

// argonQueueWait is how long a request waits for a free slot before it is
// refused with 503. Legitimate sign-ins under moderate load wait a fraction of
// a second; a flood is refused instead of queued without limit.
var argonQueueWait = 5 * time.Second

// argonRetryAfter is the Retry-After hint sent with a refusal.
const argonRetryAfter = 2 * time.Second

var argonSlots = make(chan struct{}, argonConcurrency)

// acquireArgonSlot waits up to argonQueueWait for a slot.
func acquireArgonSlot() bool {
	select {
	case argonSlots <- struct{}{}:
		return true
	default:
	}
	timer := time.NewTimer(argonQueueWait)
	defer timer.Stop()
	select {
	case argonSlots <- struct{}{}:
		return true
	case <-timer.C:
		return false
	}
}

func releaseArgonSlot() { <-argonSlots }

// argonMemoryReleaseDelay bounds how often pages go back to the OS. Every
// Argon2 evaluation leaves a 64 MiB buffer behind. A blocking collection right
// after it lets the next evaluation reuse that memory, which keeps bursts from
// growing the peak (341 MB measured for 100 attempts without it).
//
// The collection alone does not return the pages to the OS. A single successful
// sign-in therefore kept the process at about 79 MB instead of 12 MB, and it
// stayed there for minutes. Returning them after each evaluation costs about
// 350-420 ms per attempt under load, so the release runs at most once per this
// delay: a lone sign-in gives the memory back, and an attacker cannot make the
// process release it more often than that.
var argonMemoryReleaseDelay = 30 * time.Second

var (
	releaseMu      sync.Mutex
	releasePending bool
)

// scheduleMemoryRelease arranges one debug.FreeOSMemory after the delay. While a
// release is pending, further calls add nothing.
func scheduleMemoryRelease() {
	releaseMu.Lock()
	defer releaseMu.Unlock()
	if releasePending {
		return
	}
	releasePending = true
	time.AfterFunc(argonMemoryReleaseDelay, func() {
		releaseMu.Lock()
		releasePending = false
		releaseMu.Unlock()
		debug.FreeOSMemory()
	})
}

// compareAdminCredentials evaluates both checks and then schedules the release
// of the buffers they used. See argonMemoryReleaseDelay.
func compareAdminCredentials(user, pass string) (userOK, passOK bool) {
	defer scheduleMemoryRelease()
	defer runtime.GC()
	userOK = secureCompare(user, config.Current.AdminUser)
	passOK = configuredPasswordOK(pass)
	return userOK, passOK
}

// writeAuthBusy answers a request that could not get an Argon2 slot. It is a
// capacity refusal, not a credential failure, so it does not count against the
// brute-force budget.
func writeAuthBusy(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "2")
	http.Error(w, "Authentication capacity reached; retry shortly", http.StatusServiceUnavailable)
}

// Successful Basic-auth verifications are remembered for a short time, so an
// API script that sends the same credentials on every request does not pay a
// 64 MiB Argon2 evaluation each time (measured: about 64 ms per request against
// 0.4 ms with a session). Only a digest is stored. It is keyed with a random
// per-process secret and covers the credential fingerprint, so a changed
// password or hash invalidates every entry. Failures are never cached.
const (
	verifiedTTL     = 5 * time.Minute
	verifiedMaxSize = 64
)

type verifiedCache struct {
	mu      sync.Mutex
	key     []byte
	entries map[[sha256.Size]byte]time.Time
}

var verifiedCredentials = newVerifiedCache()

func newVerifiedCache() *verifiedCache {
	c := &verifiedCache{entries: make(map[[sha256.Size]byte]time.Time)}
	c.key = make([]byte, 32)
	if _, err := rand.Read(c.key); err != nil {
		// Without a secret the cache is simply disabled: correctness is kept,
		// only the per-request cost of Basic auth stays.
		c.key = nil
	}
	return c
}

// digest binds one (fingerprint, user, password) triple. Fields are
// length-prefixed so that different splits of the same bytes do not collide.
func (c *verifiedCache) digest(fingerprint, user, pass string) ([sha256.Size]byte, bool) {
	var out [sha256.Size]byte
	if c.key == nil {
		return out, false
	}
	m := hmac.New(sha256.New, c.key)
	for _, field := range []string{fingerprint, user, pass} {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(field)))
		m.Write(n[:])
		m.Write([]byte(field))
	}
	copy(out[:], m.Sum(nil))
	return out, true
}

func (c *verifiedCache) hit(d [sha256.Size]byte, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	expires, ok := c.entries[d]
	if !ok {
		return false
	}
	if !now.Before(expires) {
		delete(c.entries, d)
		return false
	}
	return true
}

func (c *verifiedCache) remember(d [sha256.Size]byte, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= verifiedMaxSize {
		for k, exp := range c.entries {
			if !now.Before(exp) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= verifiedMaxSize {
			// Full of live entries: skip caching rather than evict. The
			// request still succeeds, it just pays the Argon2 cost.
			return
		}
	}
	c.entries[d] = now.Add(verifiedTTL)
}

// reset drops every entry. Tests use it to start from a known state.
func (c *verifiedCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.entries)
}
