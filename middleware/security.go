package middleware

import (
	"net/http"
	"strings"

	"lanpaper/config"
)

func WithSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Basic security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		w.Header().Set("X-Download-Options", "noopen")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		// Note: Cross-Origin-Embedder-Policy: require-corp is intentionally omitted
		// as it breaks loading of external images in the admin panel.

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

		// Rate limiting for public endpoints only
		ip := clientIP(r)
		if !strings.HasPrefix(r.URL.Path, "/admin") && !strings.HasPrefix(r.URL.Path, "/api/") {
			if isOverLimit(ip, config.Current.Rate.PublicPerMin, config.Current.Rate.Burst) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		}
		next(w, r)
	}
}
