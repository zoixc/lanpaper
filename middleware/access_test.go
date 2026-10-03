// SPDX-License-Identifier: MIT

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"lanpaper/config"
	"lanpaper/storage"
)

func TestAuthorizeLinkAccessPublic(t *testing.T) {
	wp := &storage.Wallpaper{AccessLevel: config.AccessPublic}
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "8.8.8.8:1234"
	w := httptest.NewRecorder()
	if !AuthorizeLinkAccess(w, r, wp) {
		t.Fatal("public link must allow any IP")
	}
}

func TestAuthorizeLinkAccessLocal(t *testing.T) {
	wp := &storage.Wallpaper{AccessLevel: config.AccessLocal}

	t.Run("private IP allowed", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = "192.168.100.50:9999"
		w := httptest.NewRecorder()
		if !AuthorizeLinkAccess(w, r, wp) {
			t.Fatal("LAN IP should be allowed")
		}
	})

	t.Run("public IP denied", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = "8.8.8.8:9999"
		w := httptest.NewRecorder()
		if AuthorizeLinkAccess(w, r, wp) {
			t.Fatal("public IP must be denied for local links")
		}
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
	})

	t.Run("loopback allowed", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = "127.0.0.1:9999"
		w := httptest.NewRecorder()
		if !AuthorizeLinkAccess(w, r, wp) {
			t.Fatal("loopback should be allowed")
		}
	})
}

func TestAuthorizeLinkAccessToken(t *testing.T) {
	config.Current.DisableAuth = true
	wp := &storage.Wallpaper{
		AccessLevel: config.AccessToken,
		AccessToken: "secret-token-value",
	}

	t.Run("missing token", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = "1.2.3.4:1"
		w := httptest.NewRecorder()
		if AuthorizeLinkAccess(w, r, wp) {
			t.Fatal("must deny without token")
		}
	})

	t.Run("query token", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x?token=secret-token-value", nil)
		r.RemoteAddr = "1.2.3.4:1"
		w := httptest.NewRecorder()
		if !AuthorizeLinkAccess(w, r, wp) {
			t.Fatal("valid query token must allow")
		}
	})

	t.Run("header token", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.Header.Set("X-Access-Token", "secret-token-value")
		r.RemoteAddr = "1.2.3.4:1"
		w := httptest.NewRecorder()
		if !AuthorizeLinkAccess(w, r, wp) {
			t.Fatal("valid header token must allow")
		}
	})

	t.Run("wrong token", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x?token=nope", nil)
		r.RemoteAddr = "1.2.3.4:1"
		w := httptest.NewRecorder()
		if AuthorizeLinkAccess(w, r, wp) {
			t.Fatal("wrong token must deny")
		}
	})
}

func TestAuthorizeLinkAccessAuth(t *testing.T) {
	config.Current.DisableAuth = false
	config.Current.AdminUser = "admin"
	config.Current.AdminPass = "s3cret"
	wp := &storage.Wallpaper{AccessLevel: config.AccessAuth}

	t.Run("no credentials", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = "1.2.3.4:1"
		w := httptest.NewRecorder()
		if AuthorizeLinkAccess(w, r, wp) {
			t.Fatal("must deny without auth")
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("valid basic auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.SetBasicAuth("admin", "s3cret")
		r.RemoteAddr = "1.2.3.4:1"
		w := httptest.NewRecorder()
		if !AuthorizeLinkAccess(w, r, wp) {
			t.Fatal("valid basic auth must allow")
		}
	})

	t.Run("disabled auth cannot serve auth-level", func(t *testing.T) {
		config.Current.DisableAuth = true
		defer func() { config.Current.DisableAuth = false }()
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = "1.2.3.4:1"
		w := httptest.NewRecorder()
		if AuthorizeLinkAccess(w, r, wp) {
			t.Fatal("auth-level must stay closed when global auth is off")
		}
	})
}
