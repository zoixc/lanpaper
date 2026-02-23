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

func StartCleaner() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		muCounts.Lock()
		now := time.Now()
		for key, c := range counts {
			if now.Sub(c.WindowFrom) > 5*time.Minute {
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

// isOverLimit is kept for backward compatibility (uses "default" namespace)
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

// RateLimit returns middleware with its own isolated namespace per registration.
func RateLimit(perMin, burst int) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
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
