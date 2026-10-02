package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"

	"lanpaper/config"
)

// Publish keys let an automation (a webhook, a script on a phone, a home
// assistant routine) replace the media behind a link without holding the admin
// password. They are deliberately narrower than admin credentials:
//
//	POST /api/upload — replace or append the media of an existing link
//	POST /api/link   — create a link to upload into
//
// Everything else (renaming, re-scoping access, deleting, pinning, history,
// rotation settings) still requires the admin login, so a leaked publish key
// can push content but can neither expose nor destroy the library.
//
// Failed key guesses share the admin brute-force policy: after authMaxFailures
// wrong keys from one client (IPv6: one /64) within authFailWindow the client
// is locked out, and while it is locked out even a correct key is rejected so
// the lockout cannot be used as an oracle. A request without a key is never
// counted, so the admin panel and public links are unaffected.
type publishKeyContextKey struct{}

// PublisherFingerprint returns the log fingerprint of the publish key that
// authorized this request, or "" when admin credentials (or DISABLE_AUTH) did.
// It is a truncated hash, never the key itself.
func PublisherFingerprint(r *http.Request) string {
	fingerprint, _ := r.Context().Value(publishKeyContextKey{}).(string)
	return fingerprint
}

// publishKeyFromRequest reads an API key from X-Api-Key or from
// Authorization: Bearer. Basic credentials are left to MaybeBasicAuth, so a
// request can never satisfy both schemes with one header.
func publishKeyFromRequest(r *http.Request) string {
	if key := strings.TrimSpace(r.Header.Get("X-Api-Key")); key != "" {
		return key
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return strings.TrimSpace(auth[len(prefix):])
	}
	return ""
}

// AllowPublishUpload is the publish-key policy for POST /api/upload.
func AllowPublishUpload(r *http.Request) bool {
	return r.Method == http.MethodPost
}

// AllowPublishCreateLink is the publish-key policy for POST /api/link: creating
// a link only. Any path carrying a link name (rename, delete, pin, history,
// rollback) stays admin-only.
func AllowPublishCreateLink(r *http.Request) bool {
	return r.Method == http.MethodPost && (r.URL.Path == "/api/link" || r.URL.Path == "/api/link/")
}

// PublishOrAdmin accepts either a valid publish key for the operations allow
// permits, or admin credentials for everything else. It replaces
// MaybeBasicAuth on the two publishable routes and behaves exactly like it
// when no key is presented or no key is configured.
func PublishOrAdmin(allow func(*http.Request) bool, next http.HandlerFunc) http.HandlerFunc {
	admin := MaybeBasicAuth(next)
	return func(w http.ResponseWriter, r *http.Request) {
		key := publishKeyFromRequest(r)
		// Without a key — or on a server that has none configured — this is
		// exactly MaybeBasicAuth, so an unrelated X-Api-Key header can never
		// lock an admin out of an installation that does not use the feature.
		if key == "" || !config.PublishKeysConfigured() {
			admin(w, r)
			return
		}
		ipKey := rateKey(r)
		if locked, retry := budgetExhausted("publishfail", ipKey, authMaxFailures, authFailWindow); locked {
			writeTooManyRequests(w, retry, "Too many failed API key attempts")
			return
		}
		fingerprint, ok := config.MatchPublishKey(key)
		switch {
		case !ok:
			if n := recordEvent("publishfail", ipKey, authFailWindow); n >= authMaxFailures {
				log.Printf("Security: %s locked out after %d failed publish-key attempts", ipKey, n)
			} else {
				log.Printf("Security: rejected an invalid publish key from %s (%s %s)", ipKey, r.Method, r.URL.Path)
			}
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
		case allow(r):
			ctx := context.WithValue(r.Context(), publishKeyContextKey{}, fingerprint)
			next(w, r.WithContext(ctx))
		default:
			// A valid key does not widen the admin surface: this operation still
			// needs the admin login.
			admin(w, r)
		}
	}
}
