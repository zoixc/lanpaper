package middleware

import (
	"net/http"
	"strings"

	"github.com/zoixc/lanpaper/config"
)

func WithSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// Improved CSP - removed unsafe-inline for scripts
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' https: data: blob:; "+
				"media-src 'self' https: data: blob:; "+
				"connect-src 'self'; "+
				"font-src 'self'; "+
				"manifest-src 'self'; "+
				"form-action 'self'; "+
				"base-uri 'self'; "+
				"frame-ancestors 'none';")

		ip := clientIP(r)
		if !strings.HasPrefix(r.URL.Path, "/admin") && !strings.HasPrefix(r.URL.Path, "/api/") {
			if isOverLimit(ip, config.Current.Rate.PublicPerMin, config.Current.Rate.Burst) {
				next(w, r)
				return
			}
		}
		next(w, r)
	}
}
