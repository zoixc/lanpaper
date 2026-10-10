// SPDX-License-Identifier: MIT

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"lanpaper/config"
	"lanpaper/storage"
)

func resetLimiter(t *testing.T) {
	t.Helper()
	reset := func() {
		resetRateCounts()
	}
	reset()
	t.Cleanup(reset)
}

func withAdminCredentials(t *testing.T) {
	t.Helper()
	saved := config.Current
	t.Cleanup(func() { config.Current = saved })
	config.Current.DisableAuth = false
	config.Current.AdminUser = "admin"
	config.Current.AdminPass = "correct horse"
}

func adminRequest(remote, user, pass string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/wallpapers", nil)
	r.RemoteAddr = remote
	if user != "" || pass != "" {
		r.SetBasicAuth(user, pass)
	}
	return r
}

func serveAdmin(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	MaybeBasicAuth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })(w, r)
	return w
}

func TestBasicAuthLockoutRejectsEvenCorrectPassword(t *testing.T) {
	resetLimiter(t)
	withAdminCredentials(t)
	const attacker = "203.0.113.7:4000"

	if w := serveAdmin(adminRequest(attacker, "admin", "correct horse")); w.Code != http.StatusNoContent {
		t.Fatalf("valid login before lockout: %d", w.Code)
	}
	for i := range authMaxFailures {
		if w := serveAdmin(adminRequest(attacker, "admin", "guess-"+strconv.Itoa(i))); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d, want 401", i, w.Code)
		}
	}
	w := serveAdmin(adminRequest(attacker, "admin", "correct horse"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("correct password during lockout: status %d, want 429 (guesses must not be evaluated)", w.Code)
	}
	if secs, err := strconv.Atoi(w.Header().Get("Retry-After")); err != nil || secs < 1 {
		t.Fatalf("Retry-After = %q", w.Header().Get("Retry-After"))
	}
	// Other clients are unaffected.
	if w := serveAdmin(adminRequest("198.51.100.9:1", "admin", "correct horse")); w.Code != http.StatusNoContent {
		t.Fatalf("other client locked out: %d", w.Code)
	}
}

func TestBasicAuthDoesNotCountMissingCredentials(t *testing.T) {
	resetLimiter(t)
	withAdminCredentials(t)
	for range authMaxFailures * 3 {
		w := serveAdmin(adminRequest("192.0.2.1:1", "", ""))
		if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
			t.Fatalf("missing credentials: status %d, headers %v", w.Code, w.Header())
		}
	}
	if w := serveAdmin(adminRequest("192.0.2.1:1", "admin", "correct horse")); w.Code != http.StatusNoContent {
		t.Fatalf("browser challenge requests must not lock the admin out: %d", w.Code)
	}
}

func TestBrowserCachedBasicDoesNotSurviveSessionLogout(t *testing.T) {
	resetLimiter(t)
	withAdminCredentials(t)
	r := adminRequest("192.0.2.2:1", "admin", "correct horse")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w := serveAdmin(r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("browser cached Basic fallback: status %d, want 401", w.Code)
	}
	if challenge := w.Header().Get("WWW-Authenticate"); challenge != "" {
		t.Fatalf("browser received Basic challenge after session logout: %q", challenge)
	}
	// The same explicit credentials remain valid for a non-browser API client.
	if w := serveAdmin(adminRequest("192.0.2.2:1", "admin", "correct horse")); w.Code != http.StatusNoContent {
		t.Fatalf("script Basic auth was rejected: %d", w.Code)
	}
}

func TestLinkAccessSharesBruteForceProtection(t *testing.T) {
	resetLimiter(t)
	withAdminCredentials(t)
	const attacker = "203.0.113.8:1"
	links := []*storage.Wallpaper{
		{AccessLevel: config.AccessAuth},
		{AccessLevel: config.AccessToken, AccessToken: "link-token"},
	}
	for i := range authMaxFailures {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = attacker
		r.SetBasicAuth("admin", "guess-"+strconv.Itoa(i))
		if AuthorizeLinkAccess(httptest.NewRecorder(), r, links[i%2]) {
			t.Fatal("wrong credentials granted link access")
		}
	}
	for _, wp := range links {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = attacker
		r.SetBasicAuth("admin", "correct horse")
		w := httptest.NewRecorder()
		if AuthorizeLinkAccess(w, r, wp) || w.Code != http.StatusTooManyRequests {
			t.Fatalf("%s link: public links must not be an unthrottled password oracle (status %d)", wp.AccessLevel, w.Code)
		}
	}
	// The admin API sees the same lockout.
	if w := serveAdmin(adminRequest(attacker, "admin", "correct horse")); w.Code != http.StatusTooManyRequests {
		t.Fatalf("admin API not locked after link guesses: %d", w.Code)
	}
	// A valid link token is not a password guess and keeps working.
	r := httptest.NewRequest(http.MethodGet, "/x?token=link-token", nil)
	r.RemoteAddr = attacker
	if !AuthorizeLinkAccess(httptest.NewRecorder(), r, links[1]) {
		t.Fatal("valid token rejected during admin lockout")
	}
}

func TestRateKeyGroupsIPv6Prefix(t *testing.T) {
	tests := map[string]string{
		"192.0.2.10:80":                 "192.0.2.10",
		"[2001:db8:1:2:aaaa::1]:80":     "2001:db8:1:2::/64",
		"[2001:db8:1:2:bbbb::2]:443":    "2001:db8:1:2::/64",
		"[::ffff:198.51.100.1]:8080":    "198.51.100.1",
		"not-an-address":                "not-an-address",
		"[2001:db8:1:3:aaaa::1]:80":     "2001:db8:1:3::/64",
		"[fe80::1234:5678:9abc:def0]:1": "fe80::/64",
	}
	for remote, want := range tests {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		if got := rateKey(r); got != want {
			t.Errorf("rateKey(%s) = %q, want %q", remote, got, want)
		}
	}
}

func TestPublicRateLimitSendsRetryAfter(t *testing.T) {
	resetLimiter(t)
	saved := config.Current
	t.Cleanup(func() { config.Current = saved })
	config.Current.Rate = config.RateConfig{PublicPerMin: 2, Burst: 1}
	h := PublicRateLimit(func(w http.ResponseWriter, r *http.Request) {})
	codes := make([]int, 0, 4)
	var last *httptest.ResponseRecorder
	for range 4 {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = "192.0.2.50:1"
		last = httptest.NewRecorder()
		h(last, r)
		codes = append(codes, last.Code)
	}
	if codes[2] != http.StatusOK || codes[3] != http.StatusTooManyRequests {
		t.Fatalf("status codes %v, want 3x200 then 429", codes)
	}
	if last.Header().Get("Retry-After") == "" {
		t.Fatal("429 without Retry-After")
	}
}
