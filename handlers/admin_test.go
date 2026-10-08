// SPDX-License-Identifier: MIT

package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"lanpaper/config"
	"lanpaper/storage"
)

// seedWallpapers replaces the global store with the given records. A store
// persists through data/, so the test runs in a temporary working directory.
func seedWallpapers(t *testing.T, wps ...*storage.Wallpaper) {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(config.MediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	previous := storage.Global
	storage.Global = &storage.Store{}
	t.Cleanup(func() { storage.Global = previous })
	for _, wp := range wps {
		if err := storage.Global.Create(wp); err != nil {
			t.Fatalf("seed %s: %v", wp.LinkName, err)
		}
	}
}

func requestWallpapers(t *testing.T, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/wallpapers"+rawQuery, nil)
	rec := httptest.NewRecorder()
	Wallpapers(rec, req)
	return rec
}

// has_image=1 used to mean "false": every value but the literal "true" filtered
// for links WITHOUT media, so a caller asking for images got the opposite.
func TestHasImageFilterAcceptsOneAndZero(t *testing.T) {
	seedWallpapers(t,
		&storage.Wallpaper{ID: "with", LinkName: "with", HasImage: true, AccessLevel: config.AccessPublic},
		&storage.Wallpaper{ID: "without", LinkName: "without", AccessLevel: config.AccessPublic},
	)
	for _, tc := range []struct{ query, want string }{
		{"?has_image=1", `"linkName":"with"`},
		{"?has_image=true", `"linkName":"with"`},
		{"?has_image=0", `"linkName":"without"`},
		{"?has_image=false", `"linkName":"without"`},
	} {
		rec := requestWallpapers(t, tc.query)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d, want 200", tc.query, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, tc.want) {
			t.Errorf("%s: body %s does not contain %s", tc.query, body, tc.want)
		}
	}
	// Anything else is a client error, not a silent "false".
	for _, query := range []string{"?has_image=maybe", "?has_image=2"} {
		if rec := requestWallpapers(t, query); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", query, rec.Code)
		}
	}
}

// sort=zzz used to sort by creation date and answer 200: the caller could not
// tell that the sort field was ignored.
func TestSortParameterIsValidated(t *testing.T) {
	seedWallpapers(t, &storage.Wallpaper{ID: "one", LinkName: "one", AccessLevel: config.AccessPublic})
	if rec := requestWallpapers(t, "?sort=updated&order=asc"); rec.Code != http.StatusOK {
		t.Fatalf("sort=updated: status %d, want 200", rec.Code)
	}
	if rec := requestWallpapers(t, "?sort=zzz"); rec.Code != http.StatusBadRequest {
		t.Fatalf("sort=zzz: status %d, want 400", rec.Code)
	}
}

// A POST to /api/link/{name} is a method mismatch: the path is routed and
// answers PATCH and DELETE, so 405 is the answer a client can act on.
func TestPostToLinkNameIsMethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	Link(rec, httptest.NewRequest(http.MethodPost, "/api/link/photo", strings.NewReader("{}")))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/link/photo: status %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, "PATCH") || !strings.Contains(allow, "DELETE") {
		t.Fatalf("Allow = %q, want PATCH and DELETE", allow)
	}
	// A path that is not a link name at all stays a 404.
	rec = httptest.NewRecorder()
	Link(rec, httptest.NewRequest(http.MethodPost, "/api/link/a/b/c", strings.NewReader("{}")))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /api/link/a/b/c: status %d, want 404", rec.Code)
	}
}
