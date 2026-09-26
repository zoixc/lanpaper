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
	counts   = map[string]*counter{}
)

// StartCleaner removes stale per-IP counters periodically.
// Call once from main; runs until the process exits.
func StartCleaner() {
	cleanerInterval := time.Duration(config.RateLimitCleanerInterval) * time.Second
	ticker := time.NewTicker(cleanerInterval)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		muCounts.Lock()
		for key, c := range counts {
			if now.Sub(c.windowFrom) > time.Minute {
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

// PublicRateLimit enforces the configured public (per-minute) rate limit on
// the wrapped handler. It is used for the public link endpoints mounted at
// "/" so that the internet-facing URLs are covered by RATE_PUBLIC_PER_MIN.
func PublicRateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isOverLimit(clientIP(r), config.Current.Rate.PublicPerMin, config.Current.Rate.Burst) {
			log.Printf("Rate limit exceeded for IP: %s", clientIP(r))
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

// clientIP returns the real client IP.
// X-Real-IP and X-Forwarded-For are honoured only when the request originates
// from the configured TrustedProxy, preventing IP spoofing.
// Both headers are validated as proper IP addresses before use.
func clientIP(r *http.Request) string {
	if config.IsTrustedProxy(r.RemoteAddr) {
		if xr := r.Header.Get("X-Real-IP"); xr != "" {
			candidate := strings.TrimSpace(xr)
			if net.ParseIP(candidate) != nil {
				return candidate
			}
		}
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			// XFF is comma-separated. Take the rightmost (last) entry: it is
			// the address the trusted proxy actually saw, whereas leftmost
			// entries are client-supplied and trivially spoofed.
			raw := xf
			if idx := strings.LastIndexByte(xf, ','); idx >= 0 {
				raw = xf[idx+1:]
			}
			candidate := strings.TrimSpace(raw)
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
