package middleware

import (
	"log"
	"net/http"

	"lanpaper/config"
)

// MaybeBasicAuth applies Basic Auth only when auth is enabled.
// The check is performed per-request so that runtime config changes are respected.
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
		if !ok || user != config.Current.AdminUser || pass != config.Current.AdminPass {
			log.Printf("Failed authentication attempt from %s", clientIP(r))
			w.Header().Set("WWW-Authenticate", `Basic realm="Admin"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
