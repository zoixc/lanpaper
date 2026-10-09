// SPDX-License-Identifier: MIT

package middleware

import (
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"lanpaper/config"
)

// window is a fixed-window event counter for one client in one namespace.
type window struct {
	count int
	start time.Time
	span  time.Duration
}

// rateShards splits the counters across independent locks, so requests from
// different clients rarely contend on the same mutex. A key always maps to the
// same shard, so one client's counter is still updated under a single lock.
const rateShards = 64

type rateShard struct {
	mu     sync.Mutex
	counts map[string]*window
}

var rateTable [rateShards]rateShard

// shardFor returns the shard that holds key (FNV-1a, allocation free).
func shardFor(key string) *rateShard {
	h := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return &rateTable[h%rateShards]
}

// StartCleaner removes expired counters periodically.
// Call once from main; runs until the process exits.
func StartCleaner() {
	ticker := time.NewTicker(time.Duration(config.RateLimitCleanerInterval) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		cleanExpiredCounts(time.Now())
	}
}

// resetRateCounts forgets every counter. Tests use it to start from a clean state.
func resetRateCounts() {
	for i := range rateTable {
		rateTable[i].mu.Lock()
		rateTable[i].counts = nil
		rateTable[i].mu.Unlock()
	}
}

// cleanExpiredCounts drops the windows that have run out. It runs on a goroutine
// nobody restarts, so a panic is contained here: without the cleaner the counter
// map would grow with every client IP that ever made a request.
func cleanExpiredCounts(now time.Time) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("Critical: rate-limit cleaner recovered from a panic: %v\n%s", p, debug.Stack())
		}
	}()
	for i := range rateTable {
		sh := &rateTable[i]
		sh.mu.Lock()
		for key, c := range sh.counts {
			if now.Sub(c.start) >= c.span {
				delete(sh.counts, key)
			}
		}
		sh.mu.Unlock()
	}
}

// currentWindow returns the live window for key in sh, starting a new one if
// the previous window has expired. The caller must hold sh.mu.
func currentWindow(sh *rateShard, key string, span time.Duration, now time.Time) *window {
	c, ok := sh.counts[key]
	if !ok || now.Sub(c.start) >= span {
		if sh.counts == nil {
			sh.counts = make(map[string]*window)
		}
		c = &window{start: now, span: span}
		sh.counts[key] = c
	}
	return c
}

func retryAfter(c *window, now time.Time) time.Duration {
	return max(c.start.Add(c.span).Sub(now), time.Second)
}

// allowEvent counts one event and reports whether it fits into the budget of
// limit events per span. When the budget is exhausted, the returned duration
// tells the client when the window resets.
func allowEvent(ns, key string, limit int, span time.Duration) (bool, time.Duration) {
	now := time.Now()
	id := ns + ":" + key
	sh := shardFor(id)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	c := currentWindow(sh, id, span, now)
	if c.count >= limit {
		return false, retryAfter(c, now)
	}
	c.count++
	return true, 0
}

// budgetExhausted reports, without counting an event, whether key has used up
// its budget in the current window.
func budgetExhausted(ns, key string, limit int, span time.Duration) (bool, time.Duration) {
	now := time.Now()
	id := ns + ":" + key
	sh := shardFor(id)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	c, ok := sh.counts[id]
	if !ok || now.Sub(c.start) >= span || c.count < limit {
		return false, 0
	}
	return true, retryAfter(c, now)
}

// recordEvent counts one event and returns the new total for the window.
func recordEvent(ns, key string, span time.Duration) int {
	now := time.Now()
	id := ns + ":" + key
	sh := shardFor(id)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	c := currentWindow(sh, id, span, now)
	c.count++
	return c.count
}

func writeTooManyRequests(w http.ResponseWriter, retry time.Duration, msg string) {
	secs := int((retry + time.Second - 1) / time.Second)
	w.Header().Set("Retry-After", strconv.Itoa(max(secs, 1)))
	http.Error(w, msg, http.StatusTooManyRequests)
}

// PublicRateLimit enforces the configured public (per-minute) rate limit on
// the wrapped handler. It is used for the public link endpoints mounted at
// "/" so that the internet-facing URLs are covered by RATE_PUBLIC_PER_MIN.
func PublicRateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if perMin := config.Current.Rate.PublicPerMin; perMin > 0 {
			key := rateKey(r)
			if ok, retry := allowEvent("public", key, perMin+config.Current.Rate.Burst, time.Minute); !ok {
				log.Printf("Rate limit exceeded for %s", key)
				writeTooManyRequests(w, retry, "Too Many Requests")
				return
			}
		}
		next(w, r)
	}
}

// RateLimitFunc returns the current (perMin, burst) pair on every call.
type RateLimitFunc func() (perMin, burst int)

// RateLimit returns middleware that enforces a per-client rate limit in the
// "upload" namespace using limits provided by fn. A perMin of 0 disables it.
func RateLimit(fn RateLimitFunc) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if perMin, burst := fn(); perMin > 0 {
				key := rateKey(r)
				if ok, retry := allowEvent("upload", key, perMin+burst, time.Minute); !ok {
					log.Printf("Upload rate limit exceeded for %s", key)
					writeTooManyRequests(w, retry, "Rate limit exceeded")
					return
				}
			}
			next(w, r)
		}
	}
}

// clientIP returns the real client IP.
//
// Only a request from the configured TrustedProxy may supply the client
// address, and then only through the rightmost X-Forwarded-For entry: the
// address the trusted proxy itself saw. X-Real-IP is deliberately never read.
// A proxy that appends to X-Forwarded-For but does not overwrite a client-sent
// X-Real-IP lets that header through unchanged, so trusting it let a client
// claim any address, including a private one for the "local" access level,
// and escape the per-client rate limit and login lockout.
func clientIP(r *http.Request) string {
	if config.IsTrustedProxy(r.RemoteAddr) {
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			candidate := rightmostHeader(xf)
			if net.ParseIP(candidate) != nil {
				return candidate
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateKey identifies a client for rate limiting and brute-force protection.
// IPv6 clients are grouped by their /64 prefix: a single host usually
// controls a whole /64 and could otherwise rotate addresses to evade limits.
func rateKey(r *http.Request) string {
	ipStr := clientIP(r)
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ipStr
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}
