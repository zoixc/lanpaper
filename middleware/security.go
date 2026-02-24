package middleware

import (
	"net/http"
	"strings"

	"lanpaper/config"
)

const csp = "default-src 'none'; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	"img-src 'self' https: data: blob:; " +
	"media-src 'self' https: data: blob:; " +
	"connect-src 'self'; " +
	"font-src 'self'; " +
	"manifest-src 'self'; " +
	"form-action 'self'; " +
	"base-uri 'self'; " +
	"frame-ancestors 'none';"

// WithSecurity attaches security headers and applies public-endpoint rate limiting.
// Must wrap every handler reachable without authentication.
func WithSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		// X-XSS-Protection is omitted: deprecated and can introduce XSS in some browsers.
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		h.Set("X-Download-Options", "noopen")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		// Cross-Origin-Embedder-Policy: require-corp is intentionally omitted —
		// it breaks loading of external images in the admin panel.
		h.Set("Content-Security-Policy", csp)

		// /api/* and /admin are rate-limited separately (upload middleware).
		if !strings.HasPrefix(r.URL.Path, "/admin") && !strings.HasPrefix(r.URL.Path, "/api/") {
			if isOverLimit(clientIP(r), config.Current.Rate.PublicPerMin, config.Current.Rate.Burst) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		}
		next(w, r)
	}
}
