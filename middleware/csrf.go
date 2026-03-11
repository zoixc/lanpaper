package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	csrfTokenLength = 32
	csrfCookieName  = "_csrf_token"
	csrfHeaderName  = "X-CSRF-Token"
	csrfMaxAge      = 24 * 60 * 60 // 24 hours in seconds
	csrfTTL         = csrfMaxAge * time.Second
	// maxCSRFTokens caps in-memory token storage to prevent memory exhaustion.
	maxCSRFTokens = 50000
)

type csrfToken struct {
	token     string
	createdAt time.Time
}

var (
	csrfTokens   = make(map[string]*csrfToken)
	csrfTokensMu sync.RWMutex
)

// generateCSRFToken creates a cryptographically secure random token.
func generateCSRFToken() (string, error) {
	b := make([]byte, csrfTokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// getOrCreateCSRFToken retrieves an existing token from cookie or creates a new one.
func getOrCreateCSRFToken(r *http.Request) (string, error) {
	if cookie, err := r.Cookie(csrfCookieName); err == nil && cookie.Value != "" {
		csrfTokensMu.RLock()
		tok, exists := csrfTokens[cookie.Value]
		csrfTokensMu.RUnlock()
		if exists {
			if time.Since(tok.createdAt) < csrfTTL {
				return cookie.Value, nil
			}
			// Token expired — delete and fall through to generate a new one.
			csrfTokensMu.Lock()
			delete(csrfTokens, cookie.Value)
			csrfTokensMu.Unlock()
		}
	}

	token, err := generateCSRFToken()
	if err != nil {
		return "", err
	}

	csrfTokensMu.Lock()
	defer csrfTokensMu.Unlock()

	// Refuse to grow beyond cap — prevents memory exhaustion via GET flooding.
	if len(csrfTokens) >= maxCSRFTokens {
		log.Printf("[SECURITY] CSRF token store full (%d), dropping new token request from %s",
			maxCSRFTokens, r.RemoteAddr)
		return "", http.ErrNoCookie
	}

	csrfTokens[token] = &csrfToken{token: token, createdAt: time.Now()}
	return token, nil
}

// validateCSRFToken checks whether the provided token matches the stored one.
func validateCSRFToken(expected, provided string) bool {
	if expected == "" || provided == "" {
		return false
	}
	csrfTokensMu.RLock()
	_, exists := csrfTokens[expected]
	csrfTokensMu.RUnlock()
	if !exists {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}

// setCSRFCookie sets the CSRF token cookie.
// HttpOnly is false so the JS frontend can read it for the X-CSRF-Token header
// (double-submit cookie pattern). The cookie itself is not the secret — the
// header is; stealing only the cookie does not help without script execution.
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

// CSRFProtection returns middleware that protects against CSRF attacks.
// Validates token on all state-changing requests (POST, PUT, PATCH, DELETE).
// Token is read exclusively from the X-CSRF-Token header (double-submit cookie
// pattern). The form field fallback has been removed: r.FormValue() on a
// multipart body consumes r.Body before the handler can call
// ParseMultipartForm, silently breaking file uploads.
func CSRFProtection(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := getOrCreateCSRFToken(r)
		if err != nil {
			if err == http.ErrNoCookie {
				http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			log.Printf("Error generating CSRF token: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		setCSRFCookie(w, token, r.TLS != nil)

		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if !validateCSRFToken(token, r.Header.Get(csrfHeaderName)) {
				log.Printf("[SECURITY] CSRF token validation failed for %s %s from %s",
					r.Method, r.URL.Path, r.RemoteAddr)
				http.Error(w, "CSRF token validation failed", http.StatusForbidden)
				return
			}
		}

		next(w, r)
	}
}

// CleanExpiredCSRFTokens removes expired CSRF tokens from memory.
// Runs every 5 minutes to bound memory usage.
func CleanExpiredCSRFTokens() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		csrfTokensMu.Lock()
		for key, tok := range csrfTokens {
			if now.Sub(tok.createdAt) > csrfTTL {
				delete(csrfTokens, key)
			}
		}
		csrfTokensMu.Unlock()
	}
}
