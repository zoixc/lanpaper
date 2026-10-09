// SPDX-License-Identifier: MIT

package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lanpaper/config"
)

func resetExternalImagesCache(t *testing.T, dir string) {
	t.Helper()
	savedDir, savedDepth, savedMB := config.Current.ExternalImageDir, config.Current.MaxWalkDepth, config.Current.MaxUploadMB
	config.Current.ExternalImageDir = dir
	config.Current.MaxWalkDepth = 3
	config.Current.MaxUploadMB = 1
	externalImagesCache.mu.Lock()
	externalImagesCache.body = nil
	externalImagesCache.mu.Unlock()
	t.Cleanup(func() {
		config.Current.ExternalImageDir, config.Current.MaxWalkDepth, config.Current.MaxUploadMB = savedDir, savedDepth, savedMB
		externalImagesCache.mu.Lock()
		externalImagesCache.body = nil
		externalImagesCache.mu.Unlock()
	})
}

func listGallery(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	ExternalImages(rec, httptest.NewRequest(http.MethodGet, "/api/external-images", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	return rec.Body.String()
}

func TestExternalImagesCachesWithinTTL(t *testing.T) {
	dir := t.TempDir()
	resetExternalImagesCache(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if body := listGallery(t); !strings.Contains(body, "a.png") {
		t.Fatalf("first listing = %s", body)
	}

	// A new file inside the TTL is not visible yet: the walk is reused.
	if err := os.WriteFile(filepath.Join(dir, "b.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if body := listGallery(t); strings.Contains(body, "b.png") {
		t.Fatalf("listing was not cached: %s", body)
	}

	// Once the TTL has passed, the next request walks again.
	externalImagesCache.mu.Lock()
	externalImagesCache.at = time.Now().Add(-externalImagesTTL - time.Second)
	externalImagesCache.mu.Unlock()
	if body := listGallery(t); !strings.Contains(body, "b.png") {
		t.Fatalf("expired cache not refreshed: %s", body)
	}
}

// Changing the gallery directory or limits must not serve the old listing.
func TestExternalImagesCacheKeyedByDirectory(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	resetExternalImagesCache(t, first)
	if err := os.WriteFile(filepath.Join(first, "old.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if body := listGallery(t); !strings.Contains(body, "old.png") {
		t.Fatalf("first listing = %s", body)
	}
	config.Current.ExternalImageDir = second
	if body := listGallery(t); strings.Contains(body, "old.png") {
		t.Fatalf("old directory served after a config change: %s", body)
	}
}
