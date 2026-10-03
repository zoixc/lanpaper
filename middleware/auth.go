// SPDX-License-Identifier: MIT

package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"log"
	"net/http"
	"time"

	"lanpaper/config"
)

// Brute-force protection for admin credentials. After authMaxFailures wrong
// username/password pairs from one client (IPv6: one /64) within
// authFailWindow, that client is locked out until the window ends. While a
// client is locked out its credentials are not evaluated at all, so even a
// correct guess is rejected and the lockout cannot be used as an oracle.
// Requests without credentials are never counted.
const (
	authMaxFailures = 10
	authFailWindow  = 15 * time.Minute
)

type authResult int

const (
	authMissing authResult = iota // no Basic credentials in the request
	authOK
	authInvalid
	authLocked
)

// checkAdminCredentials is the only place where admin credentials are
// verified, for both the admin API and admin-protected public links.
func checkAdminCredentials(r *http.Request) (authResult, time.Duration) {
	if config.Current.DisableAuth || config.Current.AdminUser == "" || config.Current.AdminPass == "" {
		return authMissing, 0
	}
	user, pass, ok := r.BasicAuth()
	if !ok {
		return authMissing, 0
	}
	key := rateKey(r)
	if locked, retry := budgetExhausted("authfail", key, authMaxFailures, authFailWindow); locked {
		return authLocked, retry
	}
	// Evaluate both comparisons so the response time does not reveal
	// whether the username alone was correct.
	userOK := secureCompare(user, config.Current.AdminUser)
	passOK := secureCompare(pass, config.Current.AdminPass)
	if userOK && passOK {
		return authOK, 0
	}
	if n := recordEvent("authfail", key, authFailWindow); n >= authMaxFailures {
		log.Printf("Security: %s locked out after %d failed login attempts", key, n)
	} else {
		log.Printf("Failed auth attempt from %s", key)
	}
	return authInvalid, 0
}

// MaybeBasicAuth protects admin routes. Missing credentials fail closed (503)
// unless authentication is explicitly delegated to a proxy (DISABLE_AUTH).
func MaybeBasicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if config.Current.DisableAuth {
			next(w, r)
			return
		}
		if config.Current.AdminUser == "" || config.Current.AdminPass == "" {
			http.Error(w, "Admin credentials not configured", http.StatusServiceUnavailable)
			return
		}
		switch result, retry := checkAdminCredentials(r); result {
		case authOK:
			next(w, r)
		case authLocked:
			writeTooManyRequests(w, retry, "Too many failed login attempts")
		default:
			w.Header().Set("WWW-Authenticate", `Basic realm="Admin", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		}
	}
}

// secureCompare compares two secrets in constant time by hashing first,
// so differing lengths cannot short-circuit the comparison.
func secureCompare(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}
