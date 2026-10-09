// SPDX-License-Identifier: MIT

package middleware

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"lanpaper/config"
)

// Admin sessions. A browser that signs in through the login form gets a random
// token in an HttpOnly cookie instead of a Basic-auth header. Basic auth cannot
// be used by installed PWAs: their standalone window has no password prompt.
// Scripts keep using Basic auth.
//
// Only SHA-256 digests of the tokens are stored, in memory and in
// data/sessions.json. The file holds digests and expiry times, so reading it
// does not reveal a usable token. Sessions survive a restart.
const (
	sessionCookieName = "lanpaper_session"
	sessionTTL        = 14 * 24 * time.Hour
	maxSessions       = 1000
	maxLoginBody      = 4 << 10
)

// sessionsPath is a variable so tests can point it at a temporary directory.
var sessionsPath = filepath.Join("data", "sessions.json")

var sessionStore = struct {
	sync.Mutex
	expiry map[[sha256.Size]byte]time.Time
}{expiry: make(map[[sha256.Size]byte]time.Time)}

var errTooManySessions = errors.New("too many active sessions")

func tokenDigest(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}

// persistedSession is one entry of data/sessions.json.
type persistedSession struct {
	Digest  string `json:"digest"`  // hex SHA-256 of the token
	Expires int64  `json:"expires"` // Unix seconds
}

// LoadSessions restores sessions saved by an earlier run. Expired and malformed
// entries are dropped. A missing file means no sessions yet.
func LoadSessions() error {
	data, err := os.ReadFile(sessionsPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var entries []persistedSession
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("%s: %w", sessionsPath, err)
	}
	now := time.Now()
	sessionStore.Lock()
	defer sessionStore.Unlock()
	for _, e := range entries {
		raw, err := hex.DecodeString(e.Digest)
		if err != nil || len(raw) != sha256.Size {
			continue
		}
		exp := time.Unix(e.Expires, 0)
		if !now.Before(exp) || len(sessionStore.expiry) >= maxSessions {
			continue
		}
		var digest [sha256.Size]byte
		copy(digest[:], raw)
		sessionStore.expiry[digest] = exp
	}
	return nil
}

// persistSessions is replaceable in tests so disk failures can be injected
// deterministically. Production always points it at persistLocked. The caller
// holds sessionStore.Lock.
var persistSessions = persistLocked

// persistLocked writes the live sessions to disk atomically. The caller holds
// sessionStore.Lock.
func persistLocked(now time.Time) error {
	entries := make([]persistedSession, 0, len(sessionStore.expiry))
	for digest, exp := range sessionStore.expiry {
		if now.Before(exp) {
			entries = append(entries, persistedSession{Digest: hex.EncodeToString(digest[:]), Expires: exp.Unix()})
		}
	}
	body, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	dir := filepath.Dir(sessionsPath)
	if err := os.MkdirAll(dir, config.DataDirPerm); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sessions-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		return fail(err)
	}
	if _, err := tmp.Write(body); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, sessionsPath); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// issueSession creates a session, saves it, and returns its token. If the save
// fails, the session is not created.
func issueSession(now time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	digest := tokenDigest(token)

	sessionStore.Lock()
	defer sessionStore.Unlock()
	if len(sessionStore.expiry) >= maxSessions {
		for d, exp := range sessionStore.expiry {
			if !now.Before(exp) {
				delete(sessionStore.expiry, d)
			}
		}
		if len(sessionStore.expiry) >= maxSessions {
			return "", errTooManySessions
		}
	}
	sessionStore.expiry[digest] = now.Add(sessionTTL)
	if err := persistSessions(now); err != nil {
		delete(sessionStore.expiry, digest)
		return "", err
	}
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

// revokeSession forgets the request's session only after the updated store is
// durably saved. If persistence fails, the in-memory entry is restored: the
// caller can report failure and the user can retry instead of receiving a
// false-success response whose revoked session returns after a restart.
func revokeSession(r *http.Request) error {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil // signing out without a session is idempotent
	}
	digest := tokenDigest(c.Value)
	sessionStore.Lock()
	defer sessionStore.Unlock()
	exp, exists := sessionStore.expiry[digest]
	if !exists {
		return nil
	}
	delete(sessionStore.expiry, digest)
	if err := persistSessions(time.Now()); err != nil {
		sessionStore.expiry[digest] = exp
		return err
	}
	return nil
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
// everyone else. The browser UI deliberately accepts sessions only, not Basic
// auth: browsers cache Basic credentials and provide no dependable way for a
// web page to forget them, which used to make the Sign out button immediately
// sign the user back in. Basic auth remains available on /api/* for scripts.
func AdminPage(admin, login http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if config.Current.DisableAuth {
			admin(w, r)
			return
		}
		if config.Current.AdminUser == "" || config.Current.AdminPass == "" {
			http.Error(w, "Admin credentials not configured", http.StatusServiceUnavailable)
			return
		}
		if !HasAdminSession(r) {
			login(w, r)
			return
		}
		admin(w, r)
	}
}

// HandleSession signs in (POST) and out (DELETE) with the login form.
func HandleSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handleSessionLogin(w, r)
	case http.MethodDelete:
		if err := revokeSession(r); err != nil {
			log.Printf("Session: could not save sessions after sign-out: %v", err)
			http.Error(w, "Could not sign out; try again", http.StatusInternalServerError)
			return
		}
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
