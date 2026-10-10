// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"lanpaper/config"
	"lanpaper/storage"
)

func TestWallpaperPaginationFilteringAndSorting(t *testing.T) {
	seedWallpapers(t,
		&storage.Wallpaper{ID: "z-video", LinkName: "z-video", Category: "work", HasImage: true, MIMEType: "mp4", SizeBytes: 30, CreatedAt: 1, AccessLevel: config.AccessAuth},
		&storage.Wallpaper{ID: "a-image", LinkName: "a-image", Category: "life", HasImage: true, MIMEType: "png", SizeBytes: 20, CreatedAt: 2, AccessLevel: config.AccessPublic, IsPinned: true},
		&storage.Wallpaper{ID: "m-list", LinkName: "m-list", Category: "work", SizeBytes: 10, CreatedAt: 3, AccessLevel: config.AccessPublic, Items: []storage.PlaylistItem{{ID: 1, Ext: "jpg"}}},
	)
	for _, tc := range []struct{ query, contains, excludes string }{
		{"?kind=video", "z-video", "a-image"},
		{"?kind=image", "a-image", "z-video"},
		{"?kind=playlist", "m-list", "z-video"},
		{"?kind=pinned", "a-image", "m-list"},
		{"?access=auth", "z-video", "m-list"},
		{"?q=life", "a-image", "z-video"},
	} {
		rec := requestWallpapers(t, tc.query)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.contains) || strings.Contains(rec.Body.String(), tc.excludes) {
			t.Errorf("%s: status=%d body=%s", tc.query, rec.Code, rec.Body.String())
		}
	}
	rec := requestWallpapers(t, "?page=2&page_size=1&sort=name&order=asc")
	var response PaginatedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || response.Total != 3 || response.Page != 2 || response.PageSize != 1 || response.TotalPages != 3 || len(response.Data) != 1 {
		t.Fatalf("paginated response: status=%d response=%+v", rec.Code, response)
	}
	// Pinned links stay first, then name ordering applies.
	if response.Data[0].LinkName != "m-list" {
		t.Fatalf("page 2 link=%q", response.Data[0].LinkName)
	}
	for _, query := range []string{"?kind=unknown", "?access=unknown", "?sort=unknown", "?order=sideways"} {
		if rec := requestWallpapers(t, query); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status=%d", query, rec.Code)
		}
	}
}

func TestLegacyWallpaperResponseIsBounded(t *testing.T) {
	seedWallpapers(t)
	for index := 0; index <= CompatibilityResultLimit; index++ {
		name := fmt.Sprintf("w%05d", index)
		storage.Global.Set(name, &storage.Wallpaper{ID: name, LinkName: name, AccessLevel: config.AccessPublic})
	}
	rec := requestWallpapers(t, "")
	if rec.Code != http.StatusOK || rec.Header().Get("X-Lanpaper-Truncated") != "true" {
		t.Fatalf("status=%d headers=%v", rec.Code, rec.Header())
	}
	var response []WallpaperResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response) != CompatibilityResultLimit {
		t.Fatalf("records=%d", len(response))
	}
}
