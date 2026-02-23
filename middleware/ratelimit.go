package middleware

import (
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type counter struct {
	Count      int
	WindowFrom time.Time
}

var (
	muCounts sync.Mutex
	// key format: "<namespace>:<ip>" to isolate rate limits per endpoint group
	counts = map[string]*counter{}
)

// cleanerWindow is how long an idle entry is kept before being evicted.
// Set to 2× the rate-limit window (1 min) so entries expire soon after
// the window rolls over, keeping memory usage low.
const cleanerWindow = 2 * time.Minute

func StartCleaner() {
	ticker := time.NewTicker(cleanerWindow)
	for range ticker.C {
		muCounts.Lock()
		now := time.Now()
		for key, c := range counts {
			if now.Sub(c.WindowFrom) > cleanerWindow {
				delete(counts, key)
			}
		}
		muCounts.Unlock()
	}
}

func isOverLimitNS(ns, ip string, perMin, burst int) bool {
	if perMin <= 0 {
		return false
	}
	key := ns + ":" + ip
	now := time.Now()
	muCounts.Lock()
	defer muCounts.Unlock()
	c, ok := counts[key]
	if !ok || now.Sub(c.WindowFrom) > time.Minute {
		counts[key] = &counter{Count: 1, WindowFrom: now}
		return false
	}
	if c.Count >= perMin+burst {
		return true
	}
	c.Count++
	return false
}

// isOverLimit uses the "public" namespace (used by WithSecurity for public endpoints).
func isOverLimit(ip string, perMin, burst int) bool {
	return isOverLimitNS("public", ip, perMin, burst)
}

func clientIP(r *http.Request) string {
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		return strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

// RateLimitFunc is a function that returns the current (perMin, burst) values.
// Using a function instead of plain ints ensures the rate limit always reflects
// the live config, even if it changes after server start.
type RateLimitFunc func() (perMin, burst int)

// RateLimit returns middleware that enforces a per-IP rate limit in the
// "upload" namespace. The limits are sampled on every request via fn.
func RateLimit(fn RateLimitFunc) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			perMin, burst := fn()
			ip := clientIP(r)
			if isOverLimitNS("upload", ip, perMin, burst) {
				log.Printf("Rate limit exceeded for IP: %s", ip)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next(w, r)
		}
	}
}
