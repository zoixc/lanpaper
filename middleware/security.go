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

// buildCSP constructs a Content-Security-Policy header value.
// When nonce is non-empty it is injected into script-src and style-src
// (double-submit cookie pattern used by the admin frontend).
func buildCSP(nonce string) string {
	scriptSrc := "'self'"
	styleSrc := "'self'"
	if nonce != "" {
		scriptSrc = "'self' 'nonce-" + nonce + "'"
		styleSrc = "'self' 'nonce-" + nonce + "'"
	}
	return "default-src 'none'; " +
		"script-src " + scriptSrc + "; " +
		"style-src " + styleSrc + "; " +
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
func NonceFromRequest(r *http.Request) string {
	if v := r.Context().Value(nonceKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// isHTTPS reports whether the request arrived over HTTPS, either directly or
// via a trusted reverse proxy forwarding X-Forwarded-Proto.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if config.IsTrustedProxy(r.RemoteAddr) {
		return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	}
	return false
}

// WithSecurity attaches security headers and applies per-route rate limiting.
// The CSP nonce is stored in the request context for templates.
// Rate limit namespaces:
//   - /admin  → AdminPerMin  (panel HTML + read-only API calls from the UI)
//   - /api/*  → PublicPerMin (external API consumers)
//   - other   → PublicPerMin (public wallpaper pages)
func WithSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nonce, _ := generateNonce()

		h := w.Header()
		for key, value := range staticSecurityHeaders {
			h.Set(key, value)
		}
		// Set HSTS when HTTPS is detected, including behind a trusted proxy.
		if isHTTPS(r) {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}
		h.Set("Content-Security-Policy", buildCSP(nonce))
		if nonce != "" {
			r = r.WithContext(context.WithValue(r.Context(), nonceKey, nonce))
		}

		ip := clientIP(r)

		switch {
		case strings.HasPrefix(r.URL.Path, "/admin"):
			// Admin panel uses its own quota so that browsing the UI does not
			// consume the tighter upload-rate allowance.
			if isOverLimitNS("admin", ip, config.Current.Rate.AdminPerMin, config.Current.Rate.Burst) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		case strings.HasPrefix(r.URL.Path, "/api/"):
			if isOverLimitNS("api", ip, config.Current.Rate.PublicPerMin, config.Current.Rate.Burst) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		default:
			if isOverLimit(ip, config.Current.Rate.PublicPerMin, config.Current.Rate.Burst) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		}

		next(w, r)
	}
}
