package middleware

import (
	"net/http"
	"strings"

	"lanpaper/config"
)

// WithSecurity attaches security headers and applies public-endpoint rate
// limiting. It must wrap every handler that is reachable without auth.
func WithSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		// X-XSS-Protection is deprecated and omitted: modern browsers ignore it
		// and it can introduce XSS vulnerabilities in some edge cases.
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		h.Set("X-Download-Options", "noopen")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		// Note: Cross-Origin-Embedder-Policy: require-corp is intentionally
		// omitted — it breaks loading of external images in the admin panel.
		h.Set("Content-Security-Policy",
			"default-src 'none'; "+
				"script-src 'self'; "+
				"style-src 'self'; "+
				"img-src 'self' https: data: blob:; "+
				"media-src 'self' https: data: blob:; "+
				"connect-src 'self'; "+
				"font-src 'self'; "+
				"manifest-src 'self'; "+
				"form-action 'self'; "+
				"base-uri 'self'; "+
				"frame-ancestors 'none';")

		// Rate-limit public (non-admin, non-API) endpoints only.
		// /api/* and /admin are rate-limited separately (upload middleware).
		if !strings.HasPrefix(r.URL.Path, "/admin") && !strings.HasPrefix(r.URL.Path, "/api/") {
			ip := clientIP(r)
			if isOverLimit(ip, config.Current.Rate.PublicPerMin, config.Current.Rate.Burst) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		}
		next(w, r)
	}
}
