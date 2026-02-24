package middleware

import (
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"lanpaper/config"
)

type counter struct {
	count      int
	windowFrom time.Time
}

var (
	muCounts sync.Mutex
	// key format: "<namespace>:<ip>" — isolates limits per endpoint group.
	counts = map[string]*counter{}
)

// cleanerInterval is the sweep period for idle rate-limit entries.
// 2× the 1-minute window ensures entries expire shortly after their window rolls over.
const cleanerInterval = 2 * time.Minute

// StartCleaner removes stale per-IP counters periodically.
// Call once from main; runs until the process exits.
func StartCleaner() {
	ticker := time.NewTicker(cleanerInterval)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		muCounts.Lock()
		for key, c := range counts {
			if now.Sub(c.windowFrom) > cleanerInterval {
				delete(counts, key)
			}
		}
		muCounts.Unlock()
	}
}

// isOverLimitNS reports whether ip has exceeded perMin+burst requests in the
// current one-minute window for the given namespace.
func isOverLimitNS(ns, ip string, perMin, burst int) bool {
	if perMin <= 0 {
		return false
	}
	key := ns + ":" + ip
	now := time.Now()
	muCounts.Lock()
	defer muCounts.Unlock()
	c, ok := counts[key]
	if !ok || now.Sub(c.windowFrom) > time.Minute {
		counts[key] = &counter{count: 1, windowFrom: now}
		return false
	}
	if c.count >= perMin+burst {
		return true
	}
	c.count++
	return false
}

func isOverLimit(ip string, perMin, burst int) bool {
	return isOverLimitNS("public", ip, perMin, burst)
}

// clientIP returns the real client IP.
// X-Real-IP and X-Forwarded-For are honoured only when the TCP connection
// originates from the configured TrustedProxy, preventing IP spoofing.
func clientIP(r *http.Request) string {
	if config.IsTrustedProxy(r.RemoteAddr) {
		if xr := r.Header.Get("X-Real-IP"); xr != "" {
			return strings.TrimSpace(xr)
		}
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			// XFF is comma-separated; take the leftmost (client) entry.
			if idx := strings.IndexByte(xf, ','); idx >= 0 {
				return strings.TrimSpace(xf[:idx])
			}
			return strings.TrimSpace(xf)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RateLimitFunc returns the current (perMin, burst) pair on every call so that
// live config changes take effect without a server restart.
type RateLimitFunc func() (perMin, burst int)

// RateLimit returns middleware that enforces a per-IP rate limit in the
// "upload" namespace using limits provided by fn.
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
