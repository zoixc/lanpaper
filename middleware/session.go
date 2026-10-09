// SPDX-License-Identifier: MIT

package middleware

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"lanpaper/config"
)

// Admin sessions. A browser that signs in through the login form gets a random
// token in an HttpOnly cookie instead of a Basic-auth header. Basic auth cannot
// be used by installed PWAs: their standalone window has no password prompt.
// Scripts keep using Basic auth. Only SHA-256 digests of the tokens are stored.
// Sessions live in memory, so a restart signs everyone out.
const (
	sessionCookieName = "lanpaper_session"
	sessionTTL        = 14 * 24 * time.Hour
	maxSessions       = 1000
	maxLoginBody      = 4 << 10
)

var sessionStore = struct {
	sync.Mutex
	expiry map[[sha256.Size]byte]time.Time
}{expiry: make(map[[sha256.Size]byte]time.Time)}

var errTooManySessions = errors.New("too many active sessions")

func tokenDigest(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}

// issueSession creates a session and returns its token.
func issueSession(now time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	sessionStore.Lock()
	defer sessionStore.Unlock()
	if len(sessionStore.expiry) >= maxSessions {
		for digest, exp := range sessionStore.expiry {
			if !now.Before(exp) {
				delete(sessionStore.expiry, digest)
			}
		}
		if len(sessionStore.expiry) >= maxSessions {
			return "", errTooManySessions
		}
	}
	sessionStore.expiry[tokenDigest(token)] = now.Add(sessionTTL)
	return token, nil
}

// sessionValid reports whether the request carries a live session cookie.
func sessionValid(r *http.Request, now time.Time) bool {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || len(c.Value) == 0 || len(c.Value) > 128 {
		return false
	}
	digest := tokenDigest(c.Value)
	sessionStore.Lock()
	defer sessionStore.Unlock()
	exp, ok := sessionStore.expiry[digest]
	if !ok {
		return false
	}
	if !now.Before(exp) {
		delete(sessionStore.expiry, digest)
		return false
	}
	return true
}

func revokeSession(r *http.Request) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return
	}
	sessionStore.Lock()
	delete(sessionStore.expiry, tokenDigest(c.Value))
	sessionStore.Unlock()
}

// HasAdminSession reports whether the request is signed in with the login form.
func HasAdminSession(r *http.Request) bool {
	return sessionValid(r, time.Now())
}

// IsHTTPS reports whether the client reached Lanpaper over HTTPS, directly or
// through the trusted proxy.
func IsHTTPS(r *http.Request) bool {
	return r.TLS != nil || (config.IsTrustedProxy(r.RemoteAddr) && rightmostHeader(r.Header.Get("X-Forwarded-Proto")) == "https")
}

func sessionCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		// Secure only over HTTPS: a Secure cookie set over plain HTTP is dropped
		// by the browser, which would make the login loop on a LAN install.
		Secure:   IsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	}
}

// AdminPage serves the admin page to signed-in users and the login page to
// everyone else. Requests that carry Basic credentials go through
// MaybeBasicAuth, so a wrong password still gets 401 and lockout still applies.
func AdminPage(admin, login http.HandlerFunc) http.HandlerFunc {
	guarded := MaybeBasicAuth(admin)
	return func(w http.ResponseWriter, r *http.Request) {
		required := !config.Current.DisableAuth && config.Current.AdminUser != "" && config.Current.AdminPass != ""
		if required && !HasAdminSession(r) && r.Header.Get("Authorization") == "" {
			login(w, r)
			return
		}
		guarded(w, r)
	}
}

// HandleSession signs in (POST) and out (DELETE) with the login form.
func HandleSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handleSessionLogin(w, r)
	case http.MethodDelete:
		revokeSession(r)
		http.SetCookie(w, sessionCookie(r, "", -1))
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleSessionLogin(w http.ResponseWriter, r *http.Request) {
	if config.Current.DisableAuth || config.Current.AdminUser == "" || config.Current.AdminPass == "" {
		http.Error(w, "Login is not available", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBody)).Decode(&body); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	switch result, retry := verifyAdminPassword(body.Username, body.Password, rateKey(r)); result {
	case authLocked:
		writeTooManyRequests(w, retry, "Too many failed login attempts")
		return
	case authOK:
	default:
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}
	token, err := issueSession(time.Now())
	if err != nil {
		if errors.Is(err, errTooManySessions) {
			http.Error(w, "Too many active sessions; try again later", http.StatusServiceUnavailable)
			return
		}
		log.Printf("Session: could not create a session: %v", err)
		http.Error(w, "Could not sign in", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, sessionCookie(r, token, int(sessionTTL.Seconds())))
	w.WriteHeader(http.StatusNoContent)
}
