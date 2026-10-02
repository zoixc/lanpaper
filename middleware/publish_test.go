package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lanpaper/config"
)

const testPublishKey = "middleware-test-publish-key"

// withPublishKeys installs publish keys for one test and restores both the
// config and the parsed snapshot afterwards.
func withPublishKeys(t *testing.T, keys ...string) {
	t.Helper()
	saved := config.Current
	config.Current.PublishKeys = keys
	config.RefreshDerived()
	t.Cleanup(func() {
		config.Current = saved
		config.RefreshDerived()
	})
}

func TestPublishKeyIsScopedToPublishing(t *testing.T) {
	resetLimiter(t)
	// Registered first so its cleanup runs last and leaves a consistent snapshot.
	withPublishKeys(t, testPublishKey)
	withAdminCredentials(t)

	calls := []string{}
	upload := PublishOrAdmin(AllowPublishUpload, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "upload:"+PublisherFingerprint(r))
		w.WriteHeader(http.StatusNoContent)
	})
	create := PublishOrAdmin(AllowPublishCreateLink, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "create:"+PublisherFingerprint(r))
		w.WriteHeader(http.StatusCreated)
	})

	send := func(h http.HandlerFunc, method, path, key, user, pass string) int {
		r := httptest.NewRequest(method, path, nil)
		r.RemoteAddr = "192.0.2.1:1234"
		if key != "" {
			r.Header.Set("X-Api-Key", key)
		}
		if user != "" {
			r.SetBasicAuth(user, pass)
		}
		w := httptest.NewRecorder()
		h(w, r)
		return w.Code
	}

	// The key replaces the admin login for publishing only.
	if code := send(upload, http.MethodPost, "/api/upload", testPublishKey, "", ""); code != http.StatusNoContent {
		t.Fatalf("upload with a publish key: %d", code)
	}
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "upload:") || len(calls[0]) != len("upload:")+config.KeyFingerprintLen {
		t.Fatalf("handler did not see the key fingerprint: %q", calls)
	}
	if fp := calls[0][len("upload:"):]; strings.Contains(testPublishKey, fp) {
		t.Fatalf("the fingerprint %q leaks the key", fp)
	}
	// Authorization: Bearer is accepted as well.
	if code := send(upload, http.MethodPost, "/api/upload", "", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("upload without credentials: %d", code)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/upload", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("Authorization", "Bearer "+testPublishKey)
	w := httptest.NewRecorder()
	upload(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("bearer key: %d", w.Code)
	}

	// Admin credentials keep working on the same route.
	if code := send(upload, http.MethodPost, "/api/upload", "", "admin", "correct horse"); code != http.StatusNoContent {
		t.Fatalf("upload with admin credentials: %d", code)
	}
	// A wrong key is refused instead of being ignored.
	if code := send(upload, http.MethodPost, "/api/upload", "not-the-right-key-at-all", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("upload with a wrong key: %d", code)
	}
	if code := send(upload, http.MethodPost, "/api/upload", "", "admin", "wrong password"); code != http.StatusUnauthorized {
		t.Fatalf("upload with a wrong password: %d", code)
	}

	// Creating a link is publishable; renaming, re-scoping, pinning, rolling back
	// and deleting are not, and fall back to the admin login.
	if code := send(create, http.MethodPost, "/api/link", testPublishKey, "", ""); code != http.StatusCreated {
		t.Fatalf("create with a publish key: %d", code)
	}
	for _, tc := range []struct{ method, path string }{
		{http.MethodPatch, "/api/link/photo"},
		{http.MethodDelete, "/api/link/photo"},
		{http.MethodPost, "/api/link/photo/pin"},
		{http.MethodPost, "/api/link/photo/rollback"},
		{http.MethodGet, "/api/link/photo/history"},
	} {
		if code := send(create, tc.method, tc.path, testPublishKey, "", ""); code != http.StatusUnauthorized {
			t.Fatalf("%s %s with a publish key: %d, want 401", tc.method, tc.path, code)
		}
		if code := send(create, tc.method, tc.path, testPublishKey, "admin", "correct horse"); code == http.StatusUnauthorized {
			t.Fatalf("%s %s lost admin access: %d", tc.method, tc.path, code)
		}
	}
}

func TestPublishKeyGuessingIsRateLimited(t *testing.T) {
	resetLimiter(t)
	// Registered first so its cleanup runs last and leaves a consistent snapshot.
	withPublishKeys(t, testPublishKey)
	withAdminCredentials(t)

	handler := PublishOrAdmin(AllowPublishUpload, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	send := func(key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/upload", nil)
		r.RemoteAddr = "192.0.2.1:1234"
		r.Header.Set("X-Api-Key", key)
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}

	for range authMaxFailures {
		if code := send("guessing-a-publish-key").Code; code != http.StatusUnauthorized {
			t.Fatalf("wrong key: %d, want 401", code)
		}
	}
	locked := send("guessing-a-publish-key")
	if locked.Code != http.StatusTooManyRequests || locked.Header().Get("Retry-After") == "" {
		t.Fatalf("guessing is not rate limited: %d %v", locked.Code, locked.Header())
	}
	// The lockout is not an oracle: the right key is refused while it lasts, and
	// an admin login from the same client is refused too because the budget is
	// checked before any credential is evaluated.
	if code := send(testPublishKey).Code; code != http.StatusTooManyRequests {
		t.Fatalf("correct key during lockout: %d, want 429", code)
	}
	// Another client is unaffected.
	other := httptest.NewRequest(http.MethodPost, "/api/upload", nil)
	other.RemoteAddr = "198.51.100.7:4321"
	other.Header.Set("X-Api-Key", testPublishKey)
	otherRecorder := httptest.NewRecorder()
	handler(otherRecorder, other)
	if otherRecorder.Code != http.StatusNoContent {
		t.Fatalf("a second client was locked out: %d", otherRecorder.Code)
	}
}

func TestPublishKeyIsIgnoredWhenNoneAreConfigured(t *testing.T) {
	resetLimiter(t)
	withPublishKeys(t) // no keys
	withAdminCredentials(t)

	handler := PublishOrAdmin(AllowPublishUpload, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	send := func(key, user, pass string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/upload", nil)
		r.RemoteAddr = "192.0.2.1:1234"
		if key != "" {
			r.Header.Set("X-Api-Key", key)
		}
		if user != "" {
			r.SetBasicAuth(user, pass)
		}
		w := httptest.NewRecorder()
		handler(w, r)
		return w.Code
	}

	// An installation without PUBLISH_KEYS behaves exactly as before: a stray
	// key header is not a credential and does not consume the failure budget.
	if code := send("whatever-the-client-sends", "admin", "correct horse"); code != http.StatusNoContent {
		t.Fatalf("admin login with a stray key header: %d", code)
	}
	if code := send("whatever-the-client-sends", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("stray key header was accepted: %d", code)
	}
	for range authMaxFailures + 1 {
		send("whatever-the-client-sends", "", "")
	}
	if code := send("", "admin", "correct horse"); code != http.StatusNoContent {
		t.Fatalf("stray key headers locked the client out: %d", code)
	}
}
