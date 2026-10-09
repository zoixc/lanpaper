// SPDX-License-Identifier: MIT

package middleware

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
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
	{"Cross-Origin-Resource-Policy", "same-origin"},
	{"Cross-Origin-Opener-Policy", "same-origin"},
	// App assets carry CORP: same-origin. Credentialless is also compatible
	// with approved same-origin asset loads from the admin panel.
	{"Cross-Origin-Embedder-Policy", "credentialless"},
}

// contentSecurityPolicy is the CSP applied to admin/API responses.
// The admin panel loads only same-origin assets (plus data: placeholders and
// the blob: URLs used by the client-side image compressor), so no
// 'unsafe-inline' and no remote sources are needed.
const contentSecurityPolicy = "default-src 'none'; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	"img-src 'self' data: blob:; " +
	"media-src 'self'; " +
	"connect-src 'self'; " +
	"font-src 'self'; " +
	"manifest-src 'self'; " +
	"worker-src 'self'; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none';"

// publicMediaCSP applies to public media responses. It does not affect
// consumers that embed the media (<img>/<video>, digital frames); it only
// neutralises the response if it is ever rendered as a document.
const publicMediaCSP = "default-src 'none'; sandbox"

// embeddedMediaCSP is used when ALLOW_EMBED=true: the same "load nothing"
// policy without the sandbox directive, because sandbox also blocks the
// framing that the operator explicitly asked for.
const embeddedMediaCSP = "default-src 'none'"

// corsMaxAge tells a browser how long it may cache a preflight answer, so a
// frame that reloads every minute does not pay for a second request each time.
const corsMaxAge = 600

// corsRequestHeaders are the request headers a cross-origin media read may
// preflight for. Only this list is echoed back, so the endpoint never advertises
// headers it does not understand.
var corsRequestHeaders = map[string]bool{
	"range":             true,
	"if-range":          true,
	"if-none-match":     true,
	"if-modified-since": true,
	"x-access-token":    true,
	"authorization":     true,
	"cache-control":     true,
	"pragma":            true,
}

// applyCORS adds the Access-Control-* headers for a cross-origin media read.
// Nothing is sent when CORS_ORIGINS is unset or the Origin is not allowed, so
// the default deployment keeps behaving exactly as before: no
// Access-Control-Allow-Origin header at all.
func applyCORS(h http.Header, r *http.Request) bool {
	value, ok := config.CORSAllowOrigin(r.Header.Get("Origin"))
	if !ok {
		return false
	}
	h.Set("Access-Control-Allow-Origin", value)
	if value != "*" {
		// A shared cache must not hand one origin's response to another.
		h.Add("Vary", "Origin")
	}
	h.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	h.Set("Access-Control-Expose-Headers", "ETag, Last-Modified, Content-Length, Content-Range, Content-Type")
	h.Set("Access-Control-Max-Age", strconv.Itoa(corsMaxAge))
	return true
}

// HandleCORSOptions answers a CORS preflight for public media. It reports
// whether the request was handled: with CORS disabled, or for an Origin that is
// not allowed, the caller keeps the previous 405 behaviour.
func HandleCORSOptions(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodOptions || !config.CORSConfigured() {
		return false
	}
	h := w.Header()
	if !applyCORS(h, r) {
		return false
	}
	if allowed := filterCORSRequestHeaders(r.Header.Get("Access-Control-Request-Headers")); allowed != "" {
		h.Set("Access-Control-Allow-Headers", allowed)
	}
	// 204 carries no body, so no Content-Length is set: a proxy must not read a
	// preflight answer as the start of a media response.
	w.WriteHeader(http.StatusNoContent)
	return true
}

// filterCORSRequestHeaders keeps the allowed subset of a preflight's
// Access-Control-Request-Headers list, canonically cased and de-duplicated.
func filterCORSRequestHeaders(list string) string {
	if strings.TrimSpace(list) == "" {
		return ""
	}
	parts := strings.Split(list, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, item := range parts {
		name := strings.ToLower(strings.TrimSpace(item))
		if name == "" || seen[name] || !corsRequestHeaders[name] {
			continue
		}
		seen[name] = true
		out = append(out, http.CanonicalHeaderKey(name))
	}
	return strings.Join(out, ", ")
}

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
	trusted := config.IsTrustedProxy(r.RemoteAddr)
	scheme := "http"
	schemeKnown := true
	if r.TLS != nil {
		scheme = "https"
	} else if trusted {
		switch p := rightmostHeader(r.Header.Get("X-Forwarded-Proto")); p {
		case "http", "https":
			scheme = p
		default:
			// A proxy that terminates TLS without forwarding the original
			// scheme leaves it unknown: the browser sees https while the
			// connection to us is plain http.
			schemeKnown = false
		}
	}
	hosts := []string{r.Host}
	if trusted {
		if h := rightmostHeader(r.Header.Get("X-Forwarded-Host")); h != "" {
			hosts = append(hosts, h)
		}
	}
	for _, host := range hosts {
		expected, err := url.Parse(scheme + "://" + host)
		if err != nil || expected.Hostname() == "" || expected.User != nil || expected.Path != "" {
			continue
		}
		if !strings.EqualFold(u.Hostname(), expected.Hostname()) {
			continue
		}
		if strings.EqualFold(u.Scheme, scheme) && originPort == effectivePort(expected) {
			return true
		}
		// Unknown external scheme behind a trusted proxy: the same host may
		// have been reached over https while the connection to us is plain
		// http, so the scheme — and with it the default port — is the one the
		// proxy hides. The browser itself says the request comes from this very
		// origin, and host and port fit the other scheme, so accept it; without
		// this, a TLS-terminating proxy that does not send X-Forwarded-Proto
		// turns every panel POST into a 403. Cross-site and same-site requests
		// were already refused by the Sec-Fetch-Site switch above, so a browser
		// cannot reach this line from a page on another origin.
		if !schemeKnown && r.Header.Get("Sec-Fetch-Site") == "same-origin" {
			other := "https"
			if scheme == "https" {
				other = "http"
			}
			if alt, err := url.Parse(other + "://" + host); err == nil &&
				strings.EqualFold(u.Scheme, other) && originPort == effectivePort(alt) {
				return true
			}
		}
	}
	return false
}

// rightmostHeader returns the last entry of a comma-separated header value.
func rightmostHeader(value string) string {
	if i := strings.LastIndexByte(value, ','); i >= 0 {
		value = value[i+1:]
	}
	return strings.TrimSpace(value)
}

// peerAddress returns the IP part of an address of the form "ip:port",
// or the input unchanged when it carries no port. Used only in log lines,
// where the operator needs the exact value for TRUSTED_PROXY.
func peerAddress(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
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

// setHSTS enables HTTP Strict Transport Security for HTTPS requests, either
// served directly or forwarded by the trusted reverse proxy. Plain-HTTP LAN
// deployments are unaffected.
func setHSTS(h http.Header, r *http.Request) {
	if IsHTTPS(r) {
		h.Set("Strict-Transport-Security", "max-age=31536000")
	}
}

// WithSecurity attaches security headers and rejects cross-site
// state-changing requests (CSRF defence).
func WithSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		for _, hh := range staticSecurityHeaders {
			h.Set(hh.key, hh.value)
		}
		setHSTS(h, r)
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", contentSecurityPolicy)

		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !sameOriginRequest(r) {
				// Log rejections: a sudden wave of CSRF 403s means either an
				// attack or a misconfigured reverse proxy — both need a trail.
				// Everything the decision used is printed, because the usual
				// causes are exactly these values: a Host the browser never
				// asked for, a proxy that is not trusted yet, or a proxy that
				// terminates TLS without saying so (X-Forwarded-Proto), which
				// leaves Origin https:// against a scheme of http.
				log.Printf("Security: rejected cross-origin %s %s (Origin=%q Sec-Fetch-Site=%q Host=%q X-Forwarded-Host=%q X-Forwarded-Proto=%q trustPeer=%v RemoteAddr=%s): if this is a reverse proxy, add %s to TRUSTED_PROXY (a comma-separated list of IPs and CIDRs is allowed) and make it pass the original Host header; a proxy that terminates TLS should also send X-Forwarded-Proto=https, because without it only requests the browser itself marks as same-origin are accepted",
					r.Method, r.URL.Path, r.Header.Get("Origin"), r.Header.Get("Sec-Fetch-Site"),
					r.Host, r.Header.Get("X-Forwarded-Host"), r.Header.Get("X-Forwarded-Proto"),
					config.IsTrustedProxy(r.RemoteAddr), r.RemoteAddr, peerAddress(r.RemoteAddr))
				http.Error(w, "Cross-origin request rejected (if you are behind a reverse proxy, configure TRUSTED_PROXY)", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

// WithPublicSecurity attaches a minimal set of security headers suitable for
// public media responses (no CSRF check — GET only).
//
// ALLOW_EMBED=true relaxes exactly two of them (X-Frame-Options and the CSP
// sandbox) so dashboards and digital frames can put the media in an <iframe>.
// The relaxation is opt-in because it removes the protection against a public
// link being framed by a hostile page.
func WithPublicSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if config.Current.AllowEmbed {
			h.Set("Content-Security-Policy", embeddedMediaCSP)
		} else {
			h.Set("X-Frame-Options", "DENY")
			h.Set("Content-Security-Policy", publicMediaCSP)
		}
		setHSTS(h, r)
		// Public media may be embedded cross-origin (smart TVs, frames).
		h.Set("Cross-Origin-Resource-Policy", "cross-origin")
		h.Set("Cache-Control", "no-store")
		applyCORS(h, r)
		next(w, r)
	}
}
