package middleware

import (
	"log"
	"net/http"

	"lanpaper/config"
)

func MaybeBasicAuth(next http.HandlerFunc) http.HandlerFunc {
	if config.Current.DisableAuth {
		return next
	}
	return BasicAuth(next)
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
