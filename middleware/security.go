package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"lanpaper/config"
)

// staticSecurityHeaders are headers that don't change per-request.
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

// CSP is split into two static halves; the nonce is inserted between them.
// This avoids repeated string concatenation on every request.
const (
	cspPrefix = "default-src 'none'; " +
		"script-src 'self' 'nonce-"
	cspInfix = "'; " +
		"style-src 'self' 'nonce-"
	cspSuffix = "'; " +
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
)

func buildCSP(nonce string) string {
	return strings.Join([]string{cspPrefix, cspInfix, cspSuffix}, nonce)
}

func generateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

type nonceKeyType struct{}

var nonceKey nonceKeyType

// NonceFromRequest retrieves the CSP nonce stored in the request context.
// Returns an empty string if no nonce is present.
func NonceFromRequest(r *http.Request) string {
	if v := r.Context().Value(nonceKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// WithSecurity attaches security headers and applies public-endpoint rate
// limiting. The CSP nonce is stored in the request context for templates.
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
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}
		if nonce != "" {
			h.Set("Content-Security-Policy", buildCSP(nonce))
			r = r.WithContext(contextWithNonce(r.Context(), nonce))
		}

		if !strings.HasPrefix(r.URL.Path, "/admin") && !strings.HasPrefix(r.URL.Path, "/api/") {
			if isOverLimit(clientIP(r), config.Current.Rate.PublicPerMin, config.Current.Rate.Burst) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		}

		next(w, r)
	}
}
