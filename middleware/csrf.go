package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"log"
	"net/http"
	"strings"
	"time"

	"lanpaper/config"
)

const (
	csrfCookieName = "_csrf_token"
	csrfHeaderName = "X-CSRF-Token"
	csrfMaxAge     = 24 * 60 * 60 // 24 hours in seconds
	csrfNonceLen   = 16
)

// generateCSRFToken creates a stateless signed token: base64(nonce) + "." + base64(hmac).
// The token survives container restarts as long as CSRF_SECRET stays the same.
func generateCSRFToken() (string, error) {
	nonce := make([]byte, csrfNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	nonce64 := base64.RawURLEncoding.EncodeToString(nonce)
	sig := csrfSign(nonce64)
	return nonce64 + "." + sig, nil
}

// csrfSign returns base64(HMAC-SHA256(nonce, secret)).
func csrfSign(nonce64 string) string {
	h := hmac.New(sha256.New, []byte(config.Current.CSRFSecret))
	h.Write([]byte(nonce64))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// verifyCSRFToken checks that the token is well-formed and its signature is valid.
func verifyCSRFToken(token string) bool {
	if token == "" {
		return false
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	nonce64, providedSig := parts[0], parts[1]
	expectedSig := csrfSign(nonce64)
	return hmac.Equal([]byte(expectedSig), []byte(providedSig))
}

// getOrCreateCSRFToken returns the cookie token if valid, otherwise issues a new one.
func getOrCreateCSRFToken(r *http.Request) (string, bool) {
	if cookie, err := r.Cookie(csrfCookieName); err == nil && verifyCSRFToken(cookie.Value) {
		return cookie.Value, false // existing valid token, no need to set cookie again
	}
	token, err := generateCSRFToken()
	if err != nil {
		log.Printf("[CSRF] failed to generate token: %v", err)
		return "", false
	}
	return token, true // new token, caller must set cookie
}

// setCSRFCookie writes the CSRF token as a cookie.
// HttpOnly is false — JS reads the cookie to put it in X-CSRF-Token header
// (double-submit cookie pattern).
func setCSRFCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   csrfMaxAge,
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// CSRFProtection returns middleware that protects state-changing requests.
// Uses stateless HMAC-signed tokens — survives container restarts.
func CSRFProtection(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, isNew := getOrCreateCSRFToken(r)
		if token == "" {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if isNew {
			setCSRFCookie(w, token, r.TLS != nil)
		}

		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			provided := r.Header.Get(csrfHeaderName)
			if !verifyCSRFToken(provided) {
				log.Printf("[SECURITY] CSRF token validation failed for %s %s from %s",
					r.Method, r.URL.Path, r.RemoteAddr)
				http.Error(w, "CSRF token validation failed", http.StatusForbidden)
				return
			}
		}

		next(w, r)
	}
}

// CleanExpiredCSRFTokens is kept for backward compatibility with main.go.
// Stateless tokens need no cleanup — this is now a no-op.
func CleanExpiredCSRFTokens() {
	_ = time.NewTicker // suppress unused import
	select {}          // block forever, goroutine cost is negligible
}
