package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"lanpaper/config"
)

// generateNonce creates a cryptographically secure nonce (128 bits of entropy)
func generateNonce() (string, error) {
	b := make([]byte, 16) // 128 bits
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// buildCSP constructs a strict Content Security Policy with nonce
func buildCSP(nonce string) string {
	// Strict CSP following OWASP and W3C 2026 recommendations
	// Reference: https://cheatsheetseries.owasp.org/cheatsheets/Content_Security_Policy_Cheat_Sheet.html
	return "default-src 'none'; " +
		"script-src 'self' 'nonce-" + nonce + "'; " + // No unsafe-eval, no unsafe-inline
		"style-src 'self' 'nonce-" + nonce + "'; " + // Allow nonce-based inline styles only
		"img-src 'self' https: data: blob:; " + // Images from HTTPS sources
		"media-src 'self' https: data: blob:; " + // Media from HTTPS sources
		"connect-src 'self'; " + // API calls to same origin only
		"font-src 'self'; " + // Fonts from same origin
		"manifest-src 'self'; " + // PWA manifest
		"worker-src 'self'; " + // Service workers from same origin
		"object-src 'none'; " + // Block Flash, Java applets, etc.
		"base-uri 'self'; " + // Prevent base tag injection
		"form-action 'self'; " + // Forms submit to same origin only
		"frame-ancestors 'none'; " + // Prevent clickjacking (no iframes)
		"upgrade-insecure-requests; " + // Auto-upgrade HTTP to HTTPS
		"block-all-mixed-content;" // Block mixed content
}

// WithSecurity attaches security headers and applies public-endpoint rate limiting.
// Must wrap every handler reachable without authentication.
func WithSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Generate unique nonce for this request
		nonce, err := generateNonce()
		if err != nil {
			// Fallback to strict CSP without nonce if generation fails
			nonce = ""
		}

		// Store nonce in request context for use in templates
		// Note: In production, inject nonce into HTML templates
		if nonce != "" {
			w.Header().Set("X-Nonce", nonce) // Custom header for JS to read
		}

		// Security headers following OWASP recommendations
		h := w.Header()
		h.Set("Content-Security-Policy", buildCSP(nonce))
		h.Set("X-Content-Type-Options", "nosniff") // Prevent MIME sniffing
		h.Set("X-Frame-Options", "DENY") // Prevent clickjacking (legacy)
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin") // Limit referrer info
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=(), accelerometer=()") // Disable unnecessary features
		h.Set("X-Download-Options", "noopen") // IE: prevent opening downloads in browser context
		h.Set("Cross-Origin-Resource-Policy", "same-origin") // CORP: isolate resources
		h.Set("Cross-Origin-Opener-Policy", "same-origin") // COOP: isolate browsing context
		h.Set("Cross-Origin-Embedder-Policy", "require-corp") // COEP: require CORP for cross-origin resources

		// Strict-Transport-Security (HSTS) - only if using HTTPS
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload") // 2 years
		}

		// Rate limiting for public endpoints
		if !strings.HasPrefix(r.URL.Path, "/admin") && !strings.HasPrefix(r.URL.Path, "/api/") {
			if isOverLimit(clientIP(r), config.Current.Rate.PublicPerMin, config.Current.Rate.Burst) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		}

		next(w, r)
	}
}
