// SPDX-License-Identifier: MIT

package middleware

import (
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
// Admin credentials go through the same brute-force protection as the admin
// API. On denial the status is written here and false is returned; the caller
// must not write further.
func AuthorizeLinkAccess(w http.ResponseWriter, r *http.Request, wp *storage.Wallpaper) bool {
	switch storage.NormalizeAccessLevel(wp.AccessLevel) {
	case config.AccessPublic:
		return true

	case config.AccessLocal:
		if utils.IsPrivateOrLocalIP(net.ParseIP(clientIP(r))) {
			return true
		}
		http.Error(w, "Forbidden: local network only", http.StatusForbidden)
		return false

	case config.AccessToken:
		token := r.URL.Query().Get("token")
		if token == "" {
			token = r.Header.Get("X-Access-Token")
		}
		if wp.AccessToken != "" && token != "" && secureCompare(token, wp.AccessToken) {
			return true
		}
		// Admin with valid Basic Auth may always preview token-protected links.
		switch result, retry := checkAdminCredentials(r); result {
		case authOK:
			return true
		case authLocked:
			writeTooManyRequests(w, retry, "Too many failed login attempts")
			return false
		}
		http.Error(w, "Forbidden: valid access token required", http.StatusForbidden)
		return false

	case config.AccessAuth:
		if config.Current.DisableAuth {
			// Authentication is delegated to a proxy that does not protect
			// public link URLs, so auth-level links cannot be served here.
			// The admin still sees them via /api/preview/ (panel route).
			http.Error(w, "Forbidden: admin authentication required", http.StatusForbidden)
			return false
		}
		switch result, retry := checkAdminCredentials(r); result {
		case authOK:
			return true
		case authLocked:
			writeTooManyRequests(w, retry, "Too many failed login attempts")
			return false
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="Link", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}

	http.Error(w, "Forbidden", http.StatusForbidden)
	return false
}
