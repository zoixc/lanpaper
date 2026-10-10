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
	appmetrics "lanpaper/internal/metrics"
)

// bucket is a continuously refilled token bucket. capacity bounds bursts and
// refill/span controls the sustained rate. lastSeen supports bounded cleanup.
type bucket struct {
	tokens   float64
	capacity float64
	refill   float64 // tokens per nanosecond
	last     time.Time
	lastSeen time.Time
	idle     time.Duration
}

const rateShards = 64

type rateShard struct {
	mu     sync.Mutex
	counts map[string]*bucket
}

// RateStore owns limiter buckets and can be instantiated independently.
type RateStore struct{ table [rateShards]rateShard }

func NewRateStore() *RateStore { return &RateStore{} }
func (s *RateStore) Clean(now time.Time) {
	for i := range s.table {
		cleanRateShard(&s.table[i], now)
	}
}
func (s *RateStore) Reset() {
	for i := range s.table {
		s.table[i].mu.Lock()
		s.table[i].counts = nil
		s.table[i].mu.Unlock()
	}
}

var defaultRateStore = NewRateStore()

// DefaultRateStore is the process runtime used by compatibility middleware.
func DefaultRateStore() *RateStore { return defaultRateStore }

var rateTable = &defaultRateStore.table // compatibility view for existing tests

func shardIndex(key string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return h % rateShards
}
func shardFor(key string) *rateShard             { return &rateTable[shardIndex(key)] }
func (s *RateStore) shard(key string) *rateShard { return &s.table[shardIndex(key)] }

// Allow is the instance-scoped token-bucket primitive used by injected apps.
func (s *RateStore) Allow(namespace, key string, capacity, refill int, span time.Duration) (bool, time.Duration) {
	now := time.Now()
	id := namespace + ":" + key
	sh := s.shard(id)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	b := currentBucket(sh, id, capacity, refill, span, now)
	if b.tokens < 1 {
		return false, tokenRetry(b)
	}
	b.tokens--
	return true, 0
}

func StartCleaner() {
	ticker := time.NewTicker(time.Duration(config.RateLimitCleanerInterval) * time.Second)
	defer ticker.Stop()
	for now := range ticker.C {
		cleanExpiredCounts(now)
	}
}
func resetRateCounts() {
	for i := range rateTable {
		rateTable[i].mu.Lock()
		rateTable[i].counts = nil
		rateTable[i].mu.Unlock()
	}
}

// CleanRateLimits runs one bounded cleanup pass for composition/maintenance.
func CleanRateLimits(now time.Time) { cleanExpiredCounts(now) }

func cleanExpiredCounts(now time.Time) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("Critical: rate-limit cleaner recovered from a panic: %v\n%s", p, debug.Stack())
		}
	}()
	for i := range rateTable {
		cleanRateShard(&rateTable[i], now)
	}
}

func cleanRateShard(sh *rateShard, now time.Time) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	for key, b := range sh.counts {
		if now.Sub(b.lastSeen) >= b.idle {
			delete(sh.counts, key)
		}
	}
}

func currentBucket(sh *rateShard, key string, capacity, refill int, span time.Duration, now time.Time) *bucket {
	if sh.counts == nil {
		sh.counts = make(map[string]*bucket)
	}
	b := sh.counts[key]
	if b == nil || b.capacity != float64(capacity) {
		b = &bucket{tokens: float64(capacity), capacity: float64(capacity), refill: float64(refill) / float64(span), last: now, lastSeen: now, idle: max(2*span, time.Minute)}
		sh.counts[key] = b
		return b
	}
	elapsed := now.Sub(b.last)
	if elapsed > 0 {
		b.tokens = min(b.capacity, b.tokens+float64(elapsed)*b.refill)
		b.last = now
	}
	b.lastSeen = now
	return b
}
func tokenRetry(b *bucket) time.Duration {
	if b.refill <= 0 {
		return time.Minute
	}
	return max(time.Duration((1-b.tokens)/b.refill), time.Second)
}

func allowBucketEvent(ns, key string, capacity, refill int, span time.Duration) (bool, time.Duration) {
	now := time.Now()
	id := ns + ":" + key
	sh := shardFor(id)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	b := currentBucket(sh, id, capacity, refill, span, now)
	if b.tokens < 1 {
		return false, tokenRetry(b)
	}
	b.tokens--
	return true, 0
}
func allowEvent(ns, key string, limit int, span time.Duration) (bool, time.Duration) {
	return allowBucketEvent(ns, key, limit, limit, span)
}
func budgetExhausted(ns, key string, limit int, span time.Duration) (bool, time.Duration) {
	now := time.Now()
	id := ns + ":" + key
	sh := shardFor(id)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	b := currentBucket(sh, id, limit, limit, span, now)
	if b.tokens < 1 {
		return true, tokenRetry(b)
	}
	return false, 0
}
func recordEvent(ns, key string, span time.Duration) int {
	now := time.Now()
	id := ns + ":" + key
	sh := shardFor(id)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	b := currentBucket(sh, id, authMaxFailures, authMaxFailures, span, now)
	if b.tokens >= 1 {
		b.tokens--
	}
	return int(b.capacity - b.tokens + .999999)
}

func writeTooManyRequests(w http.ResponseWriter, retry time.Duration, msg string) {
	appmetrics.RateRejected()
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
			if ok, retry := allowBucketEvent("public", key, perMin+config.Current.Rate.Burst, perMin, time.Minute); !ok {
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
				if ok, retry := allowBucketEvent("upload", key, perMin+burst, perMin, time.Minute); !ok {
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
