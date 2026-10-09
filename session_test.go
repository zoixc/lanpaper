// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func loginRequest(t *testing.T, a *testApp, username, password string, extra map[string]string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req, err := http.NewRequest(http.MethodPost, a.server.URL+"/api/session", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func sessionCookieFrom(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == "lanpaper_session" {
			return c
		}
	}
	return nil
}

func (a *testApp) get(path string, cookie *http.Cookie) (int, http.Header, string) {
	a.t.Helper()
	req, err := http.NewRequest(http.MethodGet, a.server.URL+path, nil)
	if err != nil {
		a.t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if rerr != nil {
			break
		}
	}
	return resp.StatusCode, resp.Header, sb.String()
}

// Without a session the admin URL shows the sign-in form. It must not show the
// admin panel, and must not trigger the browser's native password prompt.
func TestAdminShowsLoginFormWithoutSession(t *testing.T) {
	a := setupApp(t)
	status, h, body := a.get("/admin", nil)
	if status != http.StatusOK || !strings.Contains(body, `id="loginForm"`) {
		t.Fatalf("status %d, login form missing", status)
	}
	if strings.Contains(body, "/static/js/app.js") {
		t.Fatal("admin panel leaked to a signed-out visitor")
	}
	if h.Get("WWW-Authenticate") != "" {
		t.Fatalf("login page sent a Basic challenge: %q", h.Get("WWW-Authenticate"))
	}
	if h.Get("Cache-Control") != "no-store" {
		t.Fatalf("login page cache header = %q", h.Get("Cache-Control"))
	}

	// A browser may retain Basic credentials from an older Lanpaper release.
	// They must not reopen the UI after Sign out: browsers do not expose a
	// reliable API for clearing their Basic-auth cache. Basic remains valid for
	// API clients below.
	req, _ := http.NewRequest(http.MethodGet, a.server.URL+"/admin", nil)
	req.SetBasicAuth("admin", "strong-test-password")
	resp, err := a.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), `id="loginForm"`) {
		t.Fatalf("Basic-auth admin request reopened panel: status %d", resp.StatusCode)
	}

	// APIs still answer 401 without credentials and accept Basic for scripts.
	a.expect(http.StatusUnauthorized, "GET", "/api/wallpapers", nil, false, nil)
	a.expect(http.StatusOK, "GET", "/api/wallpapers", nil, true, nil)
}

func TestSessionLoginLogoutFlow(t *testing.T) {
	a := setupApp(t)
	origin := map[string]string{"Origin": a.server.URL}

	if resp := loginRequest(t, a, "admin", "wrong-password", origin); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: status %d", resp.StatusCode)
	} else if resp.Header.Get("WWW-Authenticate") != "" {
		t.Fatal("login failure sent a Basic challenge")
	}

	resp := loginRequest(t, a, "admin", "strong-test-password", origin)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: status %d", resp.StatusCode)
	}
	cookie := sessionCookieFrom(t, resp)
	if cookie == nil {
		t.Fatal("login did not set a session cookie")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatalf("cookie flags: HttpOnly=%v SameSite=%v Path=%q", cookie.HttpOnly, cookie.SameSite, cookie.Path)
	}
	if cookie.Secure {
		t.Fatal("cookie is Secure over plain HTTP; the browser would drop it")
	}

	if status, _, body := a.get("/admin", cookie); status != http.StatusOK || !strings.Contains(body, "/static/js/app.js") {
		t.Fatalf("signed-in admin page: status %d", status)
	}
	if status, _, _ := a.get("/api/wallpapers", cookie); status != http.StatusOK {
		t.Fatalf("signed-in API: status %d", status)
	}
	// Basic credentials keep working for scripts.
	a.expect(http.StatusOK, "GET", "/api/wallpapers", nil, true, nil)

	// Sign out: the same cookie no longer opens anything.
	req, _ := http.NewRequest(http.MethodDelete, a.server.URL+"/api/session", nil)
	req.Header.Set("Origin", a.server.URL)
	req.AddCookie(cookie)
	out, err := a.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	out.Body.Close()
	if out.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: status %d", out.StatusCode)
	}
	if status, _, _ := a.get("/api/wallpapers", cookie); status != http.StatusUnauthorized {
		t.Fatalf("revoked session still accepted: status %d", status)
	}
}

func TestRevokeAllSessionsFlow(t *testing.T) {
	a := setupApp(t)
	origin := map[string]string{"Origin": a.server.URL}
	first := sessionCookieFrom(t, loginRequest(t, a, "admin", "strong-test-password", origin))
	second := sessionCookieFrom(t, loginRequest(t, a, "admin", "strong-test-password", origin))
	if first == nil || second == nil || first.Value == second.Value {
		t.Fatal("two independent sessions were not issued")
	}

	// The session list exposes lifecycle metadata, marks only the caller and
	// never exposes bearer tokens or digests.
	req, _ := http.NewRequest(http.MethodGet, a.server.URL+"/api/sessions", nil)
	req.AddCookie(first)
	listed, err := a.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var sessions []struct {
		ID        string `json:"id"`
		CreatedAt int64  `json:"createdAt"`
		Expires   int64  `json:"expires"`
		Current   bool   `json:"current"`
	}
	if err := json.NewDecoder(listed.Body).Decode(&sessions); err != nil {
		listed.Body.Close()
		t.Fatal(err)
	}
	listed.Body.Close()
	if listed.StatusCode != http.StatusOK || len(sessions) != 2 {
		t.Fatalf("session list: status=%d sessions=%d", listed.StatusCode, len(sessions))
	}
	current := 0
	for _, item := range sessions {
		if item.ID == "" || item.CreatedAt <= 0 || item.Expires <= item.CreatedAt {
			t.Fatalf("invalid session metadata: %+v", item)
		}
		if item.Current {
			current++
		}
	}
	if current != 1 {
		t.Fatalf("current session markers=%d, want 1", current)
	}

	// Unlike single-session logout, revoke-all must itself be authenticated.
	req, _ = http.NewRequest(http.MethodDelete, a.server.URL+"/api/sessions", nil)
	req.Header.Set("Origin", a.server.URL)
	unauthorized, err := a.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated revoke-all: status %d, want 401", unauthorized.StatusCode)
	}

	// Cross-site requests are rejected before they can revoke any session.
	req, _ = http.NewRequest(http.MethodDelete, a.server.URL+"/api/sessions", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.AddCookie(first)
	crossSite, err := a.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	crossSite.Body.Close()
	if crossSite.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site revoke-all: status %d, want 403", crossSite.StatusCode)
	}
	if status, _, _ := a.get("/api/wallpapers", second); status != http.StatusOK {
		t.Fatalf("cross-site revoke-all changed sessions: status %d", status)
	}

	req, _ = http.NewRequest(http.MethodDelete, a.server.URL+"/api/sessions", nil)
	req.Header.Set("Origin", a.server.URL)
	req.AddCookie(first)
	out, err := a.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	out.Body.Close()
	if out.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke-all: status %d", out.StatusCode)
	}
	cleared := sessionCookieFrom(t, out)
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Fatal("revoke-all did not clear the current browser cookie")
	}
	for _, cookie := range []*http.Cookie{first, second} {
		if status, _, _ := a.get("/api/wallpapers", cookie); status != http.StatusUnauthorized {
			t.Fatalf("revoked session still accepted: status %d", status)
		}
	}
}

// A login posted from another site is refused like any other cross-site
// state change.
func TestSessionLoginRefusesCrossSite(t *testing.T) {
	a := setupApp(t)
	resp := loginRequest(t, a, "admin", "strong-test-password", map[string]string{
		"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site login: status %d", resp.StatusCode)
	}
	if sessionCookieFrom(t, resp) != nil {
		t.Fatal("cross-site login issued a session")
	}
}
