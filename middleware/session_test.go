// SPDX-License-Identifier: MIT

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func loginAttempt(password, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/session",
		strings.NewReader(`{"username":"admin","password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	HandleSession(rec, req)
	return rec
}

// The login form shares the Basic-auth lockout: wrong passwords from one client
// lock that client out, and then even the correct password is refused.
func TestLoginFormSharesLockout(t *testing.T) {
	resetLimiter(t)
	withAdminCredentials(t)
	const remote = "198.51.100.23:4000"
	for i := 0; i < authMaxFailures; i++ {
		if rec := loginAttempt("wrong", remote); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d, want 401", i+1, rec.Code)
		}
	}
	if rec := loginAttempt("correct horse", remote); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("locked client: status %d, want 429", rec.Code)
	}
	// Another client is not affected.
	if rec := loginAttempt("correct horse", "198.51.100.24:4000"); rec.Code != http.StatusNoContent {
		t.Fatalf("other client: status %d, want 204", rec.Code)
	}
}

// An expired session is refused even though its cookie is still sent.
func TestExpiredSessionIsRefused(t *testing.T) {
	withAdminCredentials(t)
	now := time.Now()
	token, err := issueSession(now)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/wallpapers", nil)
	req.AddCookie(sessionCookie(req, token, 0))
	if !sessionValid(req, now) {
		t.Fatal("fresh session refused")
	}
	if sessionValid(req, now.Add(sessionTTL+time.Second)) {
		t.Fatal("expired session accepted")
	}
}

// Session tokens are stored only as digests.
func TestSessionStoresDigestsOnly(t *testing.T) {
	token, err := issueSession(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	sessionStore.Lock()
	defer sessionStore.Unlock()
	if _, ok := sessionStore.expiry[tokenDigest(token)]; !ok {
		t.Fatal("session not stored under its digest")
	}
}
