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
	counts   = map[string]*counter{}
)

func StartCleaner() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		muCounts.Lock()
		now := time.Now()
		for ip, c := range counts {
			if now.Sub(c.WindowFrom) > 5*time.Minute {
				delete(counts, ip)
			}
		}
		muCounts.Unlock()
	}
}

func isOverLimit(ip string, perMin, burst int) bool {
	if perMin <= 0 {
		return false
	}
	now := time.Now()
	muCounts.Lock()
	defer muCounts.Unlock()
	c, ok := counts[ip]
	if !ok || now.Sub(c.WindowFrom) > time.Minute {
		counts[ip] = &counter{Count: 1, WindowFrom: now}
		return false
	}
	if c.Count >= perMin+burst {
		return true
	}
	c.Count++
	return false
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

func RateLimit(perMin, burst int) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if isOverLimit(ip, perMin, burst) {
				log.Printf("Rate limit exceeded for IP: %s", ip)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next(w, r)
		}
	}
}
