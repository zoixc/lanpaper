package middleware

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"lanpaper/config"
)

// staticSecurityHeaders are headers that don't change per-request.
// Kept as a slice so the order is deterministic.
var staticSecurityHeaders = []struct{ key, value string }{
	{"X-Content-Type-Options", "nosniff"},
	{"X-Frame-Options", "DENY"},
	// Token URLs are secrets. Never send their query strings in a Referer,
	// including to other endpoints on the same origin.
	{"Referrer-Policy", "no-referrer"},
	{"Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=(), accelerometer=()"},
	{"X-Download-Options", "noopen"},
	{"Cross-Origin-Resource-Policy", "same-origin"},
	{"Cross-Origin-Opener-Policy", "same-origin"},
	// App assets carry CORP: same-origin. Credentialless is also compatible
	// with approved same-origin asset loads from the admin panel.
	{"Cross-Origin-Embedder-Policy", "credentialless"},
}

// contentSecurityPolicy is the CSP applied to admin/API responses.
// The admin panel loads only same-origin assets (plus data:/blob: URLs used
// by the client-side compressor), so no 'unsafe-inline' and no remote
// sources are needed.
const contentSecurityPolicy = "default-src 'none'; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	"img-src 'self' data: blob:; " +
	"media-src 'self' blob:; " +
	"connect-src 'self'; " +
	"font-src 'self'; " +
	"manifest-src 'self'; " +
	"worker-src 'self' blob:; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none';"

// publicMediaCSP is a relaxed CSP for public media responses so that
// consumers (digital frames, <img>/<video> embeds) are not broken.
const publicMediaCSP = "default-src 'none'; style-src 'none'; script-src 'none'; sandbox;"

// sameOriginRequest rejects cross-site and cross-port requests (CSRF).
// Browsers send Sec-Fetch-Site and/or Origin on unsafe requests. A request
// with neither header is a non-browser API client and still needs Basic Auth.
func sameOriginRequest(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "cross-site", "same-site":
		return false
	case "", "same-origin", "none":
		// Check Origin as well, even if Sec-Fetch-Site says same-origin.
	default:
		return false
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") ||
		u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	originPort := effectivePort(u)
	if originPort == "" {
		return false
	}

	// Trust forwarded headers only from the configured reverse proxy. Always
	// use the rightmost entry: preceding entries may have been set by clients.
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if config.IsTrustedProxy(r.RemoteAddr) {
		if p := rightmostHeader(r.Header.Get("X-Forwarded-Proto")); p == "http" || p == "https" {
			scheme = p
		}
	}
	hosts := []string{r.Host}
	if config.IsTrustedProxy(r.RemoteAddr) {
		if h := rightmostHeader(r.Header.Get("X-Forwarded-Host")); h != "" {
			hosts = append(hosts, h)
		}
	}
	for _, host := range hosts {
		expected, err := url.Parse(scheme + "://" + host)
		if err != nil || expected.Hostname() == "" || expected.User != nil || expected.Path != "" {
			continue
		}
		if strings.EqualFold(u.Scheme, scheme) && strings.EqualFold(u.Hostname(), expected.Hostname()) &&
			originPort == effectivePort(expected) {
			return true
		}
	}
	return false
}

func rightmostHeader(value string) string {
	parts := strings.Split(value, ",")
	return strings.TrimSpace(parts[len(parts)-1])
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	if u.Scheme == "http" {
		return "80"
	}
	return ""
}

// WithSecurity attaches security headers and rejects cross-site
// state-changing requests (CSRF defence).
func WithSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		for _, hh := range staticSecurityHeaders {
			h.Set(hh.key, hh.value)
		}
		if r.TLS != nil || (config.IsTrustedProxy(r.RemoteAddr) && rightmostHeader(r.Header.Get("X-Forwarded-Proto")) == "https") {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", contentSecurityPolicy)

		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !sameOriginRequest(r) {
				// Log rejections: a sudden wave of CSRF 403s means either an
				// attack or a misconfigured reverse proxy — both need a trail.
				log.Printf("Security: rejected cross-origin %s %s (Origin=%q Sec-Fetch-Site=%q Host=%q RemoteAddr=%s) — if this is a reverse proxy, set TRUSTED_PROXY",
					r.Method, r.URL.Path, r.Header.Get("Origin"), r.Header.Get("Sec-Fetch-Site"),
					r.Host, r.RemoteAddr)
				http.Error(w, "Cross-origin request rejected (if you are behind a reverse proxy, configure TRUSTED_PROXY)", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

// WithPublicSecurity attaches a minimal set of security headers suitable for
// public media responses (no CSRF check — GET only).
func WithPublicSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", publicMediaCSP)
		h.Set("X-Download-Options", "noopen")
		// Public media may be embedded cross-origin (smart TVs, frames).
		h.Set("Cross-Origin-Resource-Policy", "cross-origin")
		h.Set("Cache-Control", "no-store")
		next(w, r)
	}
}
