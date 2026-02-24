package middleware

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"lanpaper/config"
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

// clientIP returns the real client IP.
//
// X-Real-IP and X-Forwarded-For are honoured ONLY when the TCP connection
// originates from the configured TrustedProxy address/CIDR. Without a trusted
// proxy configured the raw RemoteAddr is always used, preventing IP spoofing
// in direct / LAN deployments.
func clientIP(r *http.Request) string {
	if config.IsTrustedProxy(r.RemoteAddr) {
		if xr := r.Header.Get("X-Real-IP"); xr != "" {
			return xr
		}
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			// XFF may be a comma-separated list; take the leftmost (client) entry.
			parts := splitAndTrim(xf)
			if len(parts) > 0 {
				return parts[0]
			}
		}
	}
	// Default: use the real TCP remote address.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// splitAndTrim splits a comma-separated header value and trims spaces.
func splitAndTrim(s string) []string {
	parts := make([]string, 0)
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			v := trimSpace(s[start:i])
			if v != "" {
				parts = append(parts, v)
			}
			start = i + 1
		}
	}
	return parts
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
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
