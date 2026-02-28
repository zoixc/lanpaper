package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"lanpaper/config"
)

// staticSecurityHeaders are headers that don't change per-request.
// Pre-built once to reduce allocation overhead on every request.
var staticSecurityHeaders = map[string]string{
	"X-Content-Type-Options":       "nosniff",
	"X-Frame-Options":              "DENY",
	"Referrer-Policy":              "strict-origin-when-cross-origin",
	"Permissions-Policy":           "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=(), accelerometer=()",
	"X-Download-Options":           "noopen",
	"Cross-Origin-Resource-Policy": "same-origin",
	"Cross-Origin-Opener-Policy":   "same-origin",
	"Cross-Origin-Embedder-Policy": "require-corp",
}

// generateNonce creates a cryptographically secure nonce (128 bits of entropy).
func generateNonce() (string, error) {
	b := make([]byte, 16) // 128 bits
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// buildCSP constructs a strict Content Security Policy with nonce.
func buildCSP(nonce string) string {
	// Strict CSP following OWASP and W3C recommendations.
	// Reference: https://cheatsheetseries.owasp.org/cheatsheets/Content_Security_Policy_Cheat_Sheet.html
	return "default-src 'none'; " +
		"script-src 'self' 'nonce-" + nonce + "'; " +
		"style-src 'self' 'nonce-" + nonce + "'; " +
		"img-src 'self' https: data: blob:; " +
		"media-src 'self' https: data: blob:; " +
		"connect-src 'self'; " +
		"font-src 'self'; " +
		"manifest-src 'self'; " +
		"worker-src 'self'; " +
		"object-src 'none'; " +
		"base-uri 'self'; " +
		"form-action 'self'; " +
		"frame-ancestors 'none'; " +
		"upgrade-insecure-requests; " +
		"block-all-mixed-content;"
}

// nonceKey is an unexported context key used to pass the CSP nonce to handlers.
type nonceKeyType struct{}

var nonceKey nonceKeyType

// NonceFromRequest retrieves the CSP nonce stored in the request context by
// WithSecurity. Returns an empty string if no nonce is present.
func NonceFromRequest(r *http.Request) string {
	if v := r.Context().Value(nonceKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// WithSecurity attaches security headers and applies public-endpoint rate limiting.
// The generated CSP nonce is stored in the request context so downstream
// handlers can embed it into HTML templates via NonceFromRequest.
func WithSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nonce, err := generateNonce()
		if err != nil {
			nonce = ""
		}

		h := w.Header()
		for key, value := range staticSecurityHeaders {
			h.Set(key, value)
		}

		// HSTS only over TLS.
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}

		if nonce != "" {
			h.Set("Content-Security-Policy", buildCSP(nonce))
			// Propagate the nonce via context so templates can embed it.
			r = r.WithContext(contextWithNonce(r.Context(), nonce))
		}

		// Rate limiting for public endpoints.
		if !strings.HasPrefix(r.URL.Path, "/admin") && !strings.HasPrefix(r.URL.Path, "/api/") {
			if isOverLimit(clientIP(r), config.Current.Rate.PublicPerMin, config.Current.Rate.Burst) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		}

		next(w, r)
	}
}
