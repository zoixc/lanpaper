package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"net"
	"net/http"

	"lanpaper/config"
	"lanpaper/storage"
	"lanpaper/utils"
)

// AuthorizeLinkAccess checks whether the request may view the wallpaper's
// media according to its AccessLevel.
//
//	public — always allowed
//	local  — client IP is loopback / RFC1918 / link-local / CGNAT / ULA
//	token  — ?token= or X-Access-Token matches the stored secret
//	auth   — requires valid admin Basic Auth
//
// Returns true when access is granted. On denial it writes the appropriate
// status and returns false; the caller must not write further.
func AuthorizeLinkAccess(w http.ResponseWriter, r *http.Request, wp *storage.Wallpaper) bool {
	level := storage.NormalizeAccessLevel(wp.AccessLevel)

	switch level {
	case config.AccessPublic:
		return true

	case config.AccessLocal:
		ipStr := clientIP(r)
		ip := net.ParseIP(ipStr)
		if utils.IsPrivateOrLocalIP(ip) {
			return true
		}
		http.Error(w, "Forbidden: local network only", http.StatusForbidden)
		return false

	case config.AccessToken:
		token := r.URL.Query().Get("token")
		if token == "" {
			token = r.Header.Get("X-Access-Token")
		}
		if wp.AccessToken != "" && secureTokenCompare(token, wp.AccessToken) {
			return true
		}
		// Admin with valid Basic Auth may always preview token-protected links.
		if adminAuthenticated(r) {
			return true
		}
		http.Error(w, "Forbidden: valid access token required", http.StatusForbidden)
		return false

	case config.AccessAuth:
		if adminAuthenticated(r) {
			return true
		}
		if config.Current.DisableAuth {
			// No credentials configured — auth-level links cannot be served
			// publicly. Admin still sees them via /api/preview/ (panel route).
			http.Error(w, "Forbidden: admin authentication required", http.StatusForbidden)
			return false
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="Link"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}

	http.Error(w, "Forbidden", http.StatusForbidden)
	return false
}

// adminAuthenticated reports whether the request carries valid admin credentials.
func adminAuthenticated(r *http.Request) bool {
	if config.Current.DisableAuth {
		return false
	}
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	return secureCompare(user, config.Current.AdminUser) &&
		secureCompare(pass, config.Current.AdminPass)
}

// secureTokenCompare compares tokens in constant time via SHA-256 digests so
// length differences do not short-circuit.
func secureTokenCompare(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}
