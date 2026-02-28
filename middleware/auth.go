package middleware

import (
	"crypto/subtle"
	"log"
	"net/http"

	"lanpaper/config"
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
		if !ok || !secureCompare(user, config.Current.AdminUser) || !secureCompare(pass, config.Current.AdminPass) {
			log.Printf("Failed auth attempt from %s", clientIP(r))
			w.Header().Set("WWW-Authenticate", `Basic realm="Admin"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func secureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
