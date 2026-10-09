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

// AuthRealm is the Basic-auth realm of every password prompt in the app: the
// admin password also opens auth-level links, and two different realms would
// make the browser ask for the same credentials twice.
const AuthRealm = `Basic realm="Admin", charset="UTF-8"`

// checkAdminCredentials is the only place where admin credentials are
// verified, for both the admin API and admin-protected public links.
func checkAdminCredentials(r *http.Request) (authResult, time.Duration) {
	if config.Current.DisableAuth || config.Current.AdminUser == "" || config.Current.AdminPass == "" {
		return authMissing, 0
	}
	if HasAdminSession(r) {
		return authOK, 0
	}
	// Fetch Metadata identifies browser traffic. Never let an origin-wide
	// cached Basic credential become a fallback after the browser session was
	// revoked; non-browser API clients omit this browser-controlled header and
	// may continue to send Basic credentials preemptively.
	if r.Header.Get("Sec-Fetch-Site") != "" {
		return authMissing, 0
	}
	user, pass, ok := r.BasicAuth()
	if !ok {
		return authMissing, 0
	}
	return verifyAdminPassword(user, pass, rateKey(r))
}

// verifyAdminPassword checks a username and password from client key. It is
// shared by Basic auth and the login form, so both count failures and enforce
// the same lockout.
func verifyAdminPassword(user, pass, key string) (authResult, time.Duration) {
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
			// A browser uses the session flow and must never be prompted into
			// caching origin-wide Basic credentials. Script clients still receive
			// the challenge that describes the supported authentication scheme.
			if r.Header.Get("Sec-Fetch-Site") == "" {
				w.Header().Set("WWW-Authenticate", AuthRealm)
			}
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
