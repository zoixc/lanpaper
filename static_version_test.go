// SPDX-License-Identifier: MIT

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVersionStaticRefs(t *testing.T) {
	v := staticVersion("js/app.js")
	if v == "" {
		t.Fatal("no version computed for js/app.js")
	}
	in := `<script src="/static/js/app.js" defer></script>` +
		`<img src="/static/images/legacy.png">` +
		`<script src="/static/js/unknown.js"></script>` +
		`fetch('/static/i18n/' + code)`
	out := string(versionStaticRefs([]byte(in)))
	if !strings.Contains(out, `"/static/js/app.js?v=`+v+`"`) {
		t.Fatalf("allowlisted asset not versioned: %s", out)
	}
	if !strings.Contains(out, `"/static/images/legacy.png"`) || !strings.Contains(out, `"/static/js/unknown.js"`) {
		t.Fatalf("non-allowlisted reference was changed: %s", out)
	}
	if !strings.Contains(out, `'/static/i18n/' + code`) {
		t.Fatalf("runtime URL was changed: %s", out)
	}
}

func TestStaticAssetCacheDependsOnVersion(t *testing.T) {
	v := staticVersion("js/app.js")
	cases := []struct {
		query, want string
	}{
		{"?v=" + v, "public, max-age=31536000, immutable"},
		{"", "no-cache"},
		{"?v=0000000000ff", "no-cache"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		serveStaticAsset(rec, httptest.NewRequest(http.MethodGet, "/static/js/app.js"+c.query, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%q: status %d", c.query, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != c.want {
			t.Fatalf("%q: Cache-Control = %q, want %q", c.query, got, c.want)
		}
	}
}

func TestAdminPageIsNeverCachedAndIsVersioned(t *testing.T) {
	rec := httptest.NewRecorder()
	serveAdminPage(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("admin page cache header = %q", rec.Header().Get("Cache-Control"))
	}
	if !strings.Contains(rec.Body.String(), "/static/js/app.js?v="+staticVersion("js/app.js")) {
		t.Fatal("admin page does not reference the versioned script")
	}
}
