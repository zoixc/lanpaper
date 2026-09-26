package middleware

import (
	"net"
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
	{"Referrer-Policy", "strict-origin-when-cross-origin"},
	{"Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=(), accelerometer=()"},
	{"X-Download-Options", "noopen"},
	{"Cross-Origin-Resource-Policy", "same-origin"},
	{"Cross-Origin-Opener-Policy", "same-origin"},
	// require-corp breaks loading of static assets (images/JS/CSS) from 'self'
	// unless every response carries CORP: same-origin, which http.FileServer
	// does not set. Use credentialless to allow same-origin assets without
	// requiring CORP on every sub-resource.
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

// sameOriginRequest reports whether a state-changing request originates
// from this application and not from another website (CSRF protection).
//
// Sec-Fetch-Site (set by all modern browsers) is authoritative: it is
// unaffected by reverse proxies that rewrite the Host header. The Origin
// header is used as a fallback for older browsers; it is compared against
// the request Host. X-Forwarded-Host is honoured ONLY when the immediate
// peer is the configured TrustedProxy — otherwise an attacker could spoof
// it to bypass CSRF checks.
//
// Non-browser clients (curl, scripts) send neither header and are allowed
// through — they are protected by Basic Auth instead.
func sameOriginRequest(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "cross-site":
		return false
		// "same-site" (sibling subdomain) or absent: check Origin below.
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser client
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false
	}
	originHost := strings.ToLower(u.Hostname())

	for _, host := range knownHosts(r) {
		if strings.EqualFold(originHost, host) {
			return true
		}
	}
	return false
}

// knownHosts returns the hostnames the app is reachable under: the request
// Host plus, when the peer is a trusted proxy, every X-Forwarded-Host entry.
func knownHosts(r *http.Request) []string {
	hosts := make([]string, 0, 2)
	addHost := func(hostport string) {
		if hostport == "" {
			return
		}
		if h, _, err := net.SplitHostPort(hostport); err == nil {
			hostport = h
		}
		hosts = append(hosts, strings.ToLower(hostport))
	}
	addHost(r.Host)
	// Only trust X-Forwarded-Host from a configured reverse proxy.
	// Without this gate an attacker sets X-Forwarded-Host: evil.com together
	// with Origin: https://evil.com and bypasses CSRF on older browsers.
	if config.IsTrustedProxy(r.RemoteAddr) {
		for _, v := range r.Header.Values("X-Forwarded-Host") {
			for _, h := range strings.Split(v, ",") {
				addHost(strings.TrimSpace(h))
			}
		}
	}
	return hosts
}

// WithSecurity attaches security headers and rejects cross-site
// state-changing requests (CSRF defence).
func WithSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			// Safe methods: nothing to protect.
		default:
			if !sameOriginRequest(r) {
				http.Error(w, "Cross-origin request rejected", http.StatusForbidden)
				return
			}
		}

		h := w.Header()
		for _, hh := range staticSecurityHeaders {
			h.Set(hh.key, hh.value)
		}
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}
		h.Set("Content-Security-Policy", contentSecurityPolicy)

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
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", publicMediaCSP)
		h.Set("X-Download-Options", "noopen")
		// Public media may be embedded cross-origin (smart TVs, frames).
		h.Set("Cross-Origin-Resource-Policy", "cross-origin")
		next(w, r)
	}
}
