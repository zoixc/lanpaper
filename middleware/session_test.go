// SPDX-License-Identifier: MIT

package middleware

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"lanpaper/storage"
)

// useSessionDir runs the test in a fresh directory with an empty session store,
// so sessions written by one test are never seen by another.
func useSessionDir(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("data", 0o700); err != nil {
		t.Fatal(err)
	}
	resetSessionStore()
	db, err := storage.OpenSQLiteWallpaperStore(context.Background(), sessionDBPath)
	if err != nil {
		t.Fatal(err)
	}
	sessionDB = db
	t.Cleanup(func() {
		db.Close()
		sessionDB = nil
	})
	t.Cleanup(resetSessionStore)
}

func resetSessionStore() {
	sessionStore.Lock()
	sessionStore.expiry = make(map[[32]byte]sessionRecord)
	sessionStore.Unlock()
	persistSessions = persistLocked
}

// simulateRestart forgets the in-memory sessions and loads them from disk, as a
// new process would.
func simulateRestart(t *testing.T) {
	t.Helper()
	resetSessionStore()
	if err := LoadSessions(); err != nil {
		t.Fatal(err)
	}
}

func loginAttempt(password, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/session",
		strings.NewReader(`{"username":"admin","password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	HandleSession(rec, req)
	return rec
}

func requestWithToken(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/wallpapers", nil)
	req.AddCookie(sessionCookie(req, token, 0))
	return req
}

// The login form shares the Basic-auth lockout: wrong passwords from one client
// lock that client out, and then even the correct password is refused.
func TestLoginFormSharesLockout(t *testing.T) {
	resetLimiter(t)
	withAdminCredentials(t)
	useSessionDir(t)
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
	useSessionDir(t)
	now := time.Now()
	token, err := issueSession(now)
	if err != nil {
		t.Fatal(err)
	}
	req := requestWithToken(token)
	if !sessionValid(req, now) {
		t.Fatal("fresh session refused")
	}
	if sessionValid(req, now.Add(sessionTTL+time.Second)) {
		t.Fatal("expired session accepted")
	}
}

// The table and the database file hold digests, never the token itself.
func TestSessionTableHoldsDigestsOnly(t *testing.T) {
	useSessionDir(t)
	token, err := issueSession(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	entries := readSessionRows(t)
	if len(entries) != 1 {
		t.Fatalf("session rows = %d, want 1", len(entries))
	}
	want := hex.EncodeToString(func() []byte { d := tokenDigest(token); return d[:] }())
	if entries[0].Digest != want {
		t.Fatalf("stored digest %q, want %q", entries[0].Digest, want)
	}
	if entries[0].ID == "" || entries[0].CreatedAt <= 0 || entries[0].Expires <= entries[0].CreatedAt {
		t.Fatalf("incomplete session metadata: %+v", entries[0])
	}
	for _, name := range []string{sessionDBPath, sessionDBPath + "-wal"} {
		body, err := os.ReadFile(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(body, []byte(token)) {
			t.Fatalf("raw token found in %s", name)
		}
	}
	info, err := os.Stat(sessionDBPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("%s mode %v, want 0600", sessionDBPath, info.Mode().Perm())
	}
}

func TestSessionListMarksOnlyCurrentWithoutSensitiveMetadata(t *testing.T) {
	useSessionDir(t)
	first, err := issueSession(time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	second, err := issueSession(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	list := listSessions(requestWithToken(first), time.Now())
	if len(list) != 2 || list[0].CreatedAt < list[1].CreatedAt {
		t.Fatalf("session list is incomplete or unsorted: %+v", list)
	}
	current := 0
	for _, item := range list {
		if item.ID == "" || item.CreatedAt <= 0 || item.Expires <= item.CreatedAt {
			t.Fatalf("invalid lifecycle metadata: %+v", item)
		}
		if item.Current {
			current++
		}
	}
	if current != 1 || !HasAdminSession(requestWithToken(second)) {
		t.Fatalf("current markers=%d or second session invalid", current)
	}
}

func TestLegacySessionFileIsImportedOnce(t *testing.T) {
	useSessionDir(t)
	token := "legacy-session-token"
	digest := tokenDigest(token)
	expiredDigest := tokenDigest("expired-legacy-token")
	legacy, err := json.Marshal([]persistedSession{
		{ID: "invalid", Digest: hex.EncodeToString(digest[:]), Expires: time.Now().Add(time.Hour).Unix()},
		{Digest: hex.EncodeToString(expiredDigest[:]), Expires: time.Now().Add(-time.Hour).Unix()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacySessionsPath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadSessions(); err != nil {
		t.Fatal(err)
	}
	if !HasAdminSession(requestWithToken(token)) {
		t.Fatal("legacy session was not imported")
	}
	migrated := readSessionRows(t)
	if len(migrated) != 1 || !validSessionID(migrated[0].ID) || migrated[0].CreatedAt <= 0 {
		t.Fatalf("imported rows: %+v", migrated)
	}
	// The file stays as a backup, but a later logout must not be undone by it.
	if err := revokeSession(requestWithToken(token)); err != nil {
		t.Fatal(err)
	}
	simulateRestart(t)
	if HasAdminSession(requestWithToken(token)) {
		t.Fatal("revoked session came back from sessions.json after import")
	}
}

func TestSessionsSurviveRestart(t *testing.T) {
	useSessionDir(t)
	token, err := issueSession(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	simulateRestart(t)
	if !HasAdminSession(requestWithToken(token)) {
		t.Fatal("session lost across a restart")
	}
}

func TestRevokedSessionStaysRevokedAfterRestart(t *testing.T) {
	useSessionDir(t)
	token, err := issueSession(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	req := requestWithToken(token)
	if err := revokeSession(req); err != nil {
		t.Fatal(err)
	}
	simulateRestart(t)
	if HasAdminSession(req) {
		t.Fatal("signed-out session came back after a restart")
	}
}

func TestLogoutReportsPersistenceFailureAndCanBeRetried(t *testing.T) {
	useSessionDir(t)
	token, err := issueSession(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	req := requestWithToken(token)
	req.Method = http.MethodDelete

	persistSessions = func(time.Time) error { return errors.New("disk unavailable") }
	rec := httptest.NewRecorder()
	HandleSession(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("failed durable logout: status %d, want 500", rec.Code)
	}
	if !HasAdminSession(requestWithToken(token)) {
		t.Fatal("failed durable logout removed the in-memory session")
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookieName && cookie.MaxAge < 0 {
			t.Fatal("failed durable logout cleared the cookie, preventing a retry")
		}
	}

	persistSessions = persistLocked
	retry := httptest.NewRecorder()
	HandleSession(retry, req)
	if retry.Code != http.StatusNoContent {
		t.Fatalf("retried logout: status %d, want 204", retry.Code)
	}
	simulateRestart(t)
	if HasAdminSession(requestWithToken(token)) {
		t.Fatal("retried logout was not durable across restart")
	}
}

func TestRevokeAllSessionsIsDurable(t *testing.T) {
	useSessionDir(t)
	first, err := issueSession(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := issueSession(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := revokeAllSessions(); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{first, second} {
		if HasAdminSession(requestWithToken(token)) {
			t.Fatal("revoke-all left a session active")
		}
	}
	simulateRestart(t)
	for _, token := range []string{first, second} {
		if HasAdminSession(requestWithToken(token)) {
			t.Fatal("revoke-all session returned after restart")
		}
	}
}

func TestRevokeAllSessionsRollsBackOnPersistenceFailure(t *testing.T) {
	useSessionDir(t)
	first, err := issueSession(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := issueSession(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	persistSessions = func(time.Time) error { return errors.New("disk unavailable") }
	if err := revokeAllSessions(); err == nil {
		t.Fatal("revoke-all succeeded despite persistence failure")
	}
	for _, token := range []string{first, second} {
		if !HasAdminSession(requestWithToken(token)) {
			t.Fatal("failed revoke-all did not restore every session")
		}
	}
}

func TestExpiredSessionsAreNotRestored(t *testing.T) {
	useSessionDir(t)
	writeSessionRows(t, []persistedSession{{
		Digest:  hex.EncodeToString(func() []byte { d := tokenDigest("old"); return d[:] }()),
		ID:      "stale-id",
		Expires: time.Now().Add(-time.Hour).Unix(),
	}})
	simulateRestart(t)
	if n := len(sessionStore.expiry); n != 0 {
		t.Fatalf("%d expired session(s) restored", n)
	}
}

func TestMalformedSessionsFileIsReported(t *testing.T) {
	useSessionDir(t)
	if err := os.WriteFile(legacySessionsPath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadSessions(); err == nil {
		t.Fatal("damaged sessions file was accepted silently")
	}
}

// A session that cannot be saved is not issued, so the user never gets a
// cookie that would disappear on the next restart.
func TestLoginFailsWhenSessionCannotBeSaved(t *testing.T) {
	useSessionDir(t)
	if err := sessionDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := issueSession(time.Now()); err == nil {
		t.Fatal("session issued although it could not be saved")
	}
	if n := len(sessionStore.expiry); n != 0 {
		t.Fatalf("%d unsaved session(s) kept in memory", n)
	}
}

// readSessionRows returns the rows of the `sessions` table as the server sees
// them after a restart.
func readSessionRows(t *testing.T) []persistedSession {
	t.Helper()
	db, err := storage.OpenSQLiteWallpaperStore(context.Background(), sessionDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.LoadSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := make([]persistedSession, 0, len(rows))
	for _, r := range rows {
		out = append(out, persistedSession{ID: r.ID, Digest: r.Digest, CreatedAt: r.CreatedAt, Expires: r.Expires, Credential: r.Credential})
	}
	return out
}

// writeSessionRows replaces the `sessions` table, as an earlier run would have.
func writeSessionRows(t *testing.T, entries []persistedSession) {
	t.Helper()
	db, err := storage.OpenSQLiteWallpaperStore(context.Background(), sessionDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows := make([]storage.SessionRow, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, storage.SessionRow{Digest: e.Digest, ID: e.ID, CreatedAt: e.CreatedAt, Expires: e.Expires, Credential: e.Credential})
	}
	if err := db.ReplaceSessions(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
}
