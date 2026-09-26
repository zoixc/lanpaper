package middleware

import (
	"crypto/subtle"
	"log"
	"net/http"

	"lanpaper/config"
)

// Brute-force protection for Basic Auth: after AuthFailPerMin+AuthFailBurst
// failed login attempts from one IP within a minute, further failures are
// answered with 429. Successful logins are never counted, so a legitimate
// user who eventually types the right password is never locked out.
const (
	authFailPerMin = 10
	authFailBurst  = 5
)

// MaybeBasicAuth applies Basic Auth only when auth is enabled.
// Checked per-request so runtime config changes take effect immediately.
func MaybeBasicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if config.Current.DisableAuth {
			next(w, r)
			return
		}
		BasicAuth(next)(w, r)
	}
}

func BasicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if ok && secureCompare(user, config.Current.AdminUser) && secureCompare(pass, config.Current.AdminPass) {
			next(w, r)
			return
		}
		ip := clientIP(r)
		if isOverLimitNS("authfail", ip, authFailPerMin, authFailBurst) {
			log.Printf("Auth failure rate limit exceeded for IP: %s", ip)
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Too many failed login attempts", http.StatusTooManyRequests)
			return
		}
		log.Printf("Failed auth attempt from %s", ip)
		w.Header().Set("WWW-Authenticate", `Basic realm="Admin"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func secureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
