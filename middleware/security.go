package middleware

import (
	"context"
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
	// require-corp breaks loading of static assets (images/JS/CSS) from 'self'
	// unless every response carries CORP: same-origin, which http.FileServer
	// does not set. Use credentialless to allow same-origin assets without
	// requiring CORP on every sub-resource.
	"Cross-Origin-Embedder-Policy": "credentialless",
}

func buildCSP(nonce string) string {
	if nonce == "" {
		return "default-src 'none'; " +
			"script-src 'self'; " +
			"style-src 'self'; " +
			"img-src 'self' https: data: blob:; " +
			"media-src 'self' https: data: blob:; " +
			"connect-src 'self'; " +
			"font-src 'self'; " +
			"manifest-src 'self'; " +
			"worker-src 'self' blob:; " +
			"object-src 'none'; " +
			"base-uri 'self'; " +
			"form-action 'self'; " +
			"frame-ancestors 'none';"
	}
	return "default-src 'none'; " +
		"script-src 'self' 'nonce-" + nonce + "'; " +
		"style-src 'self' 'nonce-" + nonce + "'; " +
		"img-src 'self' https: data: blob:; " +
		"media-src 'self' https: data: blob:; " +
		"connect-src 'self'; " +
		"font-src 'self'; " +
		"manifest-src 'self'; " +
		"worker-src 'self' blob:; " +
		"object-src 'none'; " +
		"base-uri 'self'; " +
		"form-action 'self'; " +
		"frame-ancestors 'none';"
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
		nonce, _ := generateNonce() // If err != nil, nonce is ""

		h := w.Header()
		for key, value := range staticSecurityHeaders {
			h.Set(key, value)
		}
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}

		h.Set("Content-Security-Policy", buildCSP(nonce))
		if nonce != "" {
			r = r.WithContext(context.WithValue(r.Context(), nonceKey, nonce))
		}

		// Apply public rate-limit only to routes that aren't admin or API.
		if !strings.HasPrefix(r.URL.Path, "/admin") && !strings.HasPrefix(r.URL.Path, "/api/") {
			if isOverLimit(clientIP(r), config.Current.Rate.PublicPerMin, config.Current.Rate.Burst) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		}

		next(w, r)
	}
}
