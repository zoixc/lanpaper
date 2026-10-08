// SPDX-License-Identifier: MIT

package handlers

import (
	"net/url"
	"testing"
	"time"

	"lanpaper/config"
	"lanpaper/storage"
)

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad query %q: %v", raw, err)
	}
	return values
}

func TestPublicLinkNameAcceptsAliasesOnly(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"/photo", "photo"},
		{"/photo/", "photo"},
		{"/photo.jpg", "photo"},
		{"/photo.JPG", "photo"},
		{"/photo.jpeg", "photo"},
		{"/photo.webp", "photo"},
		{"/photo.webm", "photo"},
		{"/photo/latest", "photo"},
		{"/photo/LATEST", "photo"},
		{"/photo/latest/", "photo"},
		{"/photo.jpg/latest", "photo"},
		{"/photo/latest.jpg", "photo"},
		// A link may itself be named "latest"; the alias never eats the whole path.
		{"/latest", "latest"},
		{"/latest.jpg", "latest"},
		{"/my-photo2", "my-photo2"},
	} {
		got, ok := publicLinkName(tc.path)
		if !ok || got != tc.want {
			t.Errorf("publicLinkName(%q) = %q, %v; want %q, true", tc.path, got, ok, tc.want)
		}
	}

	for _, path := range []string{
		"", "/", "//",
		// Non-media extensions are not aliases: they must keep answering 404 so
		// /manifest.json, /robots.txt and /favicon.ico behave as before.
		"/manifest.json", "/favicon.ico", "/robots.txt", "/sw.js", "/photo.txt", "/photo.json",
		// Anything deeper than the optional suffixes is not a link name.
		"/photo/other", "/photo.jpg/extra", "/photo/latest/other",
		// Traversal, reserved names and invalid characters stay rejected.
		"/../etc/passwd", "/photo/../../x", "/-bad", "/bad name", "/photo?", "/api/link", "/admin",
		"/data/media/x", "/static/css/style.css", "/health",
	} {
		if got, ok := publicLinkName(path); ok {
			t.Errorf("publicLinkName(%q) = %q, want a rejection", path, got)
		}
	}
}

func TestSelectMediaResolvesVersionsAndItems(t *testing.T) {
	wp := &storage.Wallpaper{
		ID: "w", LinkName: "w", HasImage: true, MIMEType: "png",
		ImagePath:      storage.MediaPath("w", "png"),
		CurrentVersion: 3,
		History:        []storage.HistoryEntry{{Version: 2, Ext: "jpg"}, {Version: 1, Ext: "png"}},
		Items:          []storage.PlaylistItem{{ID: 5, Ext: "webm"}, {ID: 6, Ext: "png"}},
	}

	for _, tc := range []struct{ query, path, ext string }{
		{"", wp.ImagePath, "png"},
		{"v=3", wp.ImagePath, "png"}, // the live version is the live file
		{"v=2", storage.HistoryPath("w", 2, "jpg"), "jpg"},
		{"v=1", storage.HistoryPath("w", 1, "png"), "png"},
		{"i=0", wp.ImagePath, "png"}, // 0 is the live file
		{"i=5", storage.ItemPath("w", 5, "webm"), "webm"},
		{"i=6", storage.ItemPath("w", 6, "png"), "png"},
	} {
		sel, ok := selectMedia(wp, mustQuery(t, tc.query))
		if !ok || sel.path != tc.path || sel.ext != tc.ext {
			t.Errorf("selectMedia(%q) = %+v, %v; want %s/%s", tc.query, sel, ok, tc.path, tc.ext)
		}
	}

	// An unusable selector is a 404, never a silent switch to other bytes.
	for _, query := range []string{
		"v=0", "v=4", "v=99", "v=x", "v=-1", "v=2&i=5", "v=3&i=0",
		"i=1", "i=4", "i=7", "i=x", "i=-1",
	} {
		if sel, ok := selectMedia(wp, mustQuery(t, query)); ok {
			t.Errorf("selectMedia(%q) = %+v, want a rejection", query, sel)
		}
	}
	// "v=" and "i=" alone parse as an empty value, which means "no selector".
	if sel, ok := selectMedia(wp, mustQuery(t, "v=")); !ok || sel.path != wp.ImagePath {
		t.Errorf("an empty version selector: %+v %v", sel, ok)
	}

	// Rotation walks the whole playlist and stays inside it.
	wp.Rotate = storage.NormalizeRotatePtr(
		&storage.RotateConfig{Enabled: true, Interval: config.MinRotateInterval}, true)
	allowed := map[string]bool{
		wp.ImagePath:                     true,
		storage.ItemPath("w", 5, "webm"): true,
		storage.ItemPath("w", 6, "png"):  true,
	}
	for range 200 {
		sel, ok := selectMedia(wp, mustQuery(t, ""))
		if !ok || !allowed[sel.path] {
			t.Fatalf("rotation returned %+v %v", sel, ok)
		}
	}
	// An explicit selector still wins over rotation.
	if sel, ok := selectMedia(wp, mustQuery(t, "v=2")); !ok || sel.path != storage.HistoryPath("w", 2, "jpg") {
		t.Fatalf("rotation overrode ?v=: %+v %v", sel, ok)
	}

	// A record without media cannot be served, with or without a selector.
	for _, query := range []string{"", "v=1", "i=0"} {
		if sel, ok := selectMedia(&storage.Wallpaper{ID: "x", LinkName: "x"}, mustQuery(t, query)); ok {
			t.Errorf("selectMedia(%q) on an empty record = %+v", query, sel)
		}
	}
}

func TestLiveVersionTreatsLegacyRecordsAsVersionOne(t *testing.T) {
	if got := liveVersion(&storage.Wallpaper{}); got != 1 {
		t.Fatalf("liveVersion = %d, want 1", got)
	}
	if got := liveVersion(&storage.Wallpaper{CurrentVersion: 7}); got != 7 {
		t.Fatalf("liveVersion = %d, want 7", got)
	}
}

// A media download must be given time for the bytes it has to send. The
// server-wide write timeout cuts off a stalled request, but a 50 MB video at
// 256 KiB/s is not stalled — it simply needs longer, and the response used to
// be truncated mid-file.
func TestMediaWriteBudgetFollowsTheFileSize(t *testing.T) {
	for _, tc := range []struct {
		name   string
		size   int64
		want   bool
		budget time.Duration
	}{
		{"empty", 0, false, 0},
		{"small image", 1 << 20, false, 0}, // 1 MB: ~5 s, covered by the default
		{"just above the default", 40 << 20, true, 161 * time.Second},
		{"50 MB video at 256 KiB/s", 50 << 20, true, 201 * time.Second},
		{"huge file is capped", 512 << 20, true, maxMediaWriteBudget},
	} {
		budget, ok := mediaWriteBudget(tc.size)
		if ok != tc.want || budget != tc.budget {
			t.Errorf("%s: mediaWriteBudget(%d) = %v, %v; want %v, %v",
				tc.name, tc.size, budget, ok, tc.budget, tc.want)
		}
		if budget > maxMediaWriteBudget {
			t.Errorf("%s: budget %v exceeds the cap %v", tc.name, budget, maxMediaWriteBudget)
		}
	}
}
