package middleware

import (
	"log"
	"net/http"

	"lanpaper/config"
)

// MaybeBasicAuth checks DisableAuth on every request, not at startup.
// This ensures env variables like DISABLE_AUTH=true are always respected.
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
		if config.Current.AdminUser == "" || config.Current.AdminPass == "" {
			log.Printf("Auth: admin credentials not configured, denying access from %s", clientIP(r))
			w.Header().Set("WWW-Authenticate", `Basic realm="Admin"`)
			http.Error(w, "Unauthorized: admin credentials not set", http.StatusUnauthorized)
			return
		}

		user, pass, ok := r.BasicAuth()
		if !ok || user != config.Current.AdminUser || pass != config.Current.AdminPass {
			log.Printf("Auth: failed authentication attempt from %s", clientIP(r))
			w.Header().Set("WWW-Authenticate", `Basic realm="Admin"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
