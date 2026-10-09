// SPDX-License-Identifier: MIT

package recovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"lanpaper/storage"
)

func TestAuditReportsMissingOrphanStaleAndDrift(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"data/media", "data/previews", "data/history/wall", "data/items/wall"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	records := map[string]*storage.Wallpaper{"wall": {LinkName: "wall", HasImage: true, MIMEType: "png", SizeBytes: 4, History: []storage.HistoryEntry{{Version: 1, Ext: "jpg", SizeBytes: 3}}, Items: []storage.PlaylistItem{{ID: 1, Ext: "webp", SizeBytes: 2}}}}
	body, _ := json.Marshal(records)
	if err := os.WriteFile(filepath.Join(root, "data/wallpapers.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(root, "data/media/wall.png")
	if err := os.WriteFile(media, []byte("too-long"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(media, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data/media/orphan.png"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data/previews/.upload-old.webp"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Audit(root)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, issue := range report.Issues {
		codes[issue.Code] = true
	}
	for _, code := range []string{"size-drift", "unsafe-permissions", "orphan-file", "stale-temporary", "missing-preview", "missing-history", "missing-item"} {
		if !codes[code] {
			t.Errorf("missing issue %s: %#v", code, report.Issues)
		}
	}
}

func TestAuditIsReadOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data/media"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "data/media/orphan.png")
	_ = os.WriteFile(path, []byte("keep"), 0o600)
	if _, err := Audit(root); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != "keep" {
		t.Fatalf("audit mutated file: %q %v", body, err)
	}
}
