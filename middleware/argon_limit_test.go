// SPDX-License-Identifier: MIT

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"lanpaper/config"
)

// withArgonConfig installs a hashed admin credential for one test and restores
// the global state afterwards, including the failure budget and caches.
func withArgonConfig(t *testing.T, password string) {
	t.Helper()
	hash, err := GeneratePasswordHash(password)
	if err != nil {
		t.Fatal(err)
	}
	old := config.Current
	t.Cleanup(func() { config.Current = old })
	config.Current.AdminUser = "admin"
	config.Current.AdminPasswordHash = hash
	config.Current.AdminPass = ""
	config.Current.DisableAuth = false
	resetRateCounts()
	t.Cleanup(resetRateCounts)
	verifiedCredentials.reset()
	t.Cleanup(verifiedCredentials.reset)
}

// holdArgonSlots occupies every slot, as if a flood were in progress.
func holdArgonSlots(t *testing.T) {
	t.Helper()
	for i := 0; i < argonConcurrency; i++ {
		argonSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for i := 0; i < argonConcurrency; i++ {
			<-argonSlots
		}
	})
}

func shortQueueWait(t *testing.T) {
	t.Helper()
	old := argonQueueWait
	argonQueueWait = 50 * time.Millisecond
	t.Cleanup(func() { argonQueueWait = old })
}

func TestArgonSlotsRefuseWhenSaturated(t *testing.T) {
	withArgonConfig(t, "correct horse")
	shortQueueWait(t)
	holdArgonSlots(t)

	if acquireArgonSlot() {
		t.Fatal("a slot was granted while every slot was held")
	}
	result, _ := verifyAdminPassword("admin", "wrong", "10.0.0.1")
	if result != authBusy {
		t.Fatalf("saturated verification = %v, want authBusy", result)
	}
}

// A refusal for capacity is not a failed login: it must not spend the
// brute-force budget, or a flood would lock out legitimate administrators.
func TestBusyRefusalDoesNotCountAsFailure(t *testing.T) {
	withArgonConfig(t, "correct horse")
	shortQueueWait(t)
	holdArgonSlots(t)

	for i := 0; i < authMaxFailures+5; i++ {
		if result, _ := verifyAdminPassword("admin", "wrong", "10.0.0.2"); result != authBusy {
			t.Fatalf("attempt %d = %v, want authBusy", i, result)
		}
	}
	if locked, _ := budgetExhausted("authfail", "10.0.0.2", authMaxFailures, authFailWindow); locked {
		t.Fatal("capacity refusals were counted as authentication failures")
	}
}

func TestVerifiedCredentialSkipsArgonAfterFirstSuccess(t *testing.T) {
	withArgonConfig(t, "correct horse")
	if result, _ := verifyAdminPassword("admin", "correct horse", "10.0.0.3"); result != authOK {
		t.Fatalf("first verification = %v, want authOK", result)
	}
	// With every slot held, only a cache hit can still succeed.
	shortQueueWait(t)
	holdArgonSlots(t)
	if result, _ := verifyAdminPassword("admin", "correct horse", "10.0.0.3"); result != authOK {
		t.Fatalf("cached verification = %v, want authOK without an Argon2 slot", result)
	}
}

func TestFailuresAndWrongUsersAreNeverCached(t *testing.T) {
	withArgonConfig(t, "correct horse")
	if result, _ := verifyAdminPassword("admin", "wrong", "10.0.0.4"); result != authInvalid {
		t.Fatalf("wrong password = %v, want authInvalid", result)
	}
	if result, _ := verifyAdminPassword("someone", "correct horse", "10.0.0.4"); result != authInvalid {
		t.Fatalf("wrong user = %v, want authInvalid", result)
	}
	if len(verifiedCredentials.entries) != 0 {
		t.Fatalf("%d entries cached after failed attempts", len(verifiedCredentials.entries))
	}
}

func TestVerifiedCacheDigestSeparatesFieldsAndFingerprint(t *testing.T) {
	c := newVerifiedCache()
	if c.key == nil {
		t.Skip("no random key available")
	}
	a, _ := c.digest("fp", "ab", "c")
	b, _ := c.digest("fp", "a", "bc")
	if a == b {
		t.Fatal("different field splits produced the same digest")
	}
	other, _ := c.digest("fp2", "ab", "c")
	if a == other {
		t.Fatal("a changed credential fingerprint did not change the digest")
	}
}

func TestVerifiedCacheEntriesExpire(t *testing.T) {
	c := newVerifiedCache()
	d, _ := c.digest("fp", "u", "p")
	now := time.Now()
	c.remember(d, now)
	if !c.hit(d, now.Add(verifiedTTL-time.Second)) {
		t.Fatal("entry missing before its TTL")
	}
	if c.hit(d, now.Add(verifiedTTL+time.Second)) {
		t.Fatal("entry still served after its TTL")
	}
}

func TestVerifiedCacheIsBounded(t *testing.T) {
	c := newVerifiedCache()
	now := time.Now()
	for i := 0; i < verifiedMaxSize*2; i++ {
		d, _ := c.digest("fp", "u", string(rune('a'+i%26))+time.Duration(i).String())
		c.remember(d, now)
	}
	if n := len(c.entries); n > verifiedMaxSize {
		t.Fatalf("cache grew to %d entries, limit %d", n, verifiedMaxSize)
	}
}

// The HTTP layer must answer a saturated Basic-auth request with 503 and a
// Retry-After, not with 401, so that clients know to retry.
func TestBasicAuthSaturationReturns503(t *testing.T) {
	withArgonConfig(t, "correct horse")
	shortQueueWait(t)
	holdArgonSlots(t)

	h := MaybeBasicAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler reached without a verified credential")
	})
	req := httptest.NewRequest("GET", "/api/wallpapers", nil)
	req.SetBasicAuth("admin", "wrong")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != 503 || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("status %d, Retry-After %q; want 503 with Retry-After", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestDummyHashIsComputedOnceOnDemand(t *testing.T) {
	first := dummyHash()
	if first == "" || dummyHash() != first {
		t.Fatal("dummy hash is empty or unstable")
	}
}

// Repeated evaluations must share one pending release, so an attacker cannot
// make the process return memory more often than argonMemoryReleaseDelay.
func TestMemoryReleaseIsCoalescedAndRearms(t *testing.T) {
	saved := argonMemoryReleaseDelay
	argonMemoryReleaseDelay = 10 * time.Millisecond
	t.Cleanup(func() { argonMemoryReleaseDelay = saved })
	// An earlier test may have left a release pending with the long delay.
	releaseMu.Lock()
	releasePending = false
	releaseMu.Unlock()

	scheduleMemoryRelease()
	scheduleMemoryRelease()
	scheduleMemoryRelease()
	releaseMu.Lock()
	pending := releasePending
	releaseMu.Unlock()
	if !pending {
		t.Fatal("release was not scheduled")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		releaseMu.Lock()
		pending = releasePending
		releaseMu.Unlock()
		if !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scheduled release never ran")
		}
		time.Sleep(time.Millisecond)
	}
	scheduleMemoryRelease()
	releaseMu.Lock()
	rearmed := releasePending
	releaseMu.Unlock()
	if !rearmed {
		t.Fatal("a later evaluation must be able to schedule another release")
	}
}
