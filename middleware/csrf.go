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
	csrfFormField   = "csrf_token"
	csrfMaxAge      = 24 * 60 * 60 // 24 hours
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

// getOrCreateCSRFToken retrieves existing token from cookie or creates new one.
func getOrCreateCSRFToken(r *http.Request) (string, error) {
	cookie, err := r.Cookie(csrfCookieName)
	if err == nil && cookie.Value != "" {
		csrfTokensMu.RLock()
		if tok, exists := csrfTokens[cookie.Value]; exists {
			// Check if token is still valid (not expired)
			if time.Since(tok.createdAt) < csrfMaxAge*time.Second {
				csrfTokensMu.RUnlock()
				return cookie.Value, nil
			}
			// Token expired, will create new one
			csrfTokensMu.RUnlock()
			csrfTokensMu.Lock()
			delete(csrfTokens, cookie.Value)
			csrfTokensMu.Unlock()
		} else {
			csrfTokensMu.RUnlock()
		}
	}

	// Generate new token
	token, err := generateCSRFToken()
	if err != nil {
		return "", err
	}

	csrfTokensMu.Lock()
	csrfTokens[token] = &csrfToken{
		token:     token,
		createdAt: time.Now(),
	}
	csrfTokensMu.Unlock()

	return token, nil
}

// validateCSRFToken checks if the provided token matches the expected token.
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

	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}

// setCSRFCookie sets a secure CSRF token cookie.
func setCSRFCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   csrfMaxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// CSRFProtection returns middleware that protects against CSRF attacks.
// It validates CSRF tokens on all state-changing requests (POST, PUT, PATCH, DELETE).
func CSRFProtection(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Generate or retrieve CSRF token
		token, err := getOrCreateCSRFToken(r)
		if err != nil {
			log.Printf("Error generating CSRF token: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Set CSRF cookie on every request to keep it fresh
		setCSRFCookie(w, token, r.TLS != nil)

		// For state-changing methods, validate the token
		if r.Method == http.MethodPost || r.Method == http.MethodPut ||
			r.Method == http.MethodPatch || r.Method == http.MethodDelete {

			// Try header first, then form field
			providedToken := r.Header.Get(csrfHeaderName)
			if providedToken == "" {
				providedToken = r.FormValue(csrfFormField)
			}

			if !validateCSRFToken(token, providedToken) {
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
// Should be called periodically (e.g., from a background goroutine).
func CleanExpiredCSRFTokens() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		csrfTokensMu.Lock()
		for key, tok := range csrfTokens {
			if now.Sub(tok.createdAt) > csrfMaxAge*time.Second {
				delete(csrfTokens, key)
			}
		}
		csrfTokensMu.Unlock()
	}
}
