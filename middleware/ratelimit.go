// SPDX-License-Identifier: MIT

package middleware

import (
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
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

var (
	muCounts sync.Mutex
	counts   = map[string]*window{}
)

// StartCleaner removes expired counters periodically.
// Call once from main; runs until the process exits.
func StartCleaner() {
	ticker := time.NewTicker(time.Duration(config.RateLimitCleanerInterval) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		cleanExpiredCounts(time.Now())
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
	muCounts.Lock()
	defer muCounts.Unlock()
	for key, c := range counts {
		if now.Sub(c.start) >= c.span {
			delete(counts, key)
		}
	}
}

// currentWindow returns the live window for key, starting a new one if the
// previous window has expired. The caller must hold muCounts.
func currentWindow(key string, span time.Duration, now time.Time) *window {
	c, ok := counts[key]
	if !ok || now.Sub(c.start) >= span {
		c = &window{start: now, span: span}
		counts[key] = c
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
	muCounts.Lock()
	defer muCounts.Unlock()
	c := currentWindow(ns+":"+key, span, now)
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
	muCounts.Lock()
	defer muCounts.Unlock()
	c, ok := counts[ns+":"+key]
	if !ok || now.Sub(c.start) >= span || c.count < limit {
		return false, 0
	}
	return true, retryAfter(c, now)
}

// recordEvent counts one event and returns the new total for the window.
func recordEvent(ns, key string, span time.Duration) int {
	now := time.Now()
	muCounts.Lock()
	defer muCounts.Unlock()
	c := currentWindow(ns+":"+key, span, now)
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
