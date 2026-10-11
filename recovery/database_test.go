// SPDX-License-Identifier: MIT

package recovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lanpaper/storage"
)

// With the SQLite backend, recovery must read the database, not the legacy
// JSON file, and must name the database in its findings.
func TestAuditReadsDatabaseWhenPresent(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, d := range []string{"data/media", "data/previews"} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store := &storage.Store{}
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(&storage.Wallpaper{ID: "wall", LinkName: "wall", HasImage: true, MIMEType: "png", SizeBytes: 4}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := Audit(root)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	body, _ := json.Marshal(report)
	if !strings.Contains(string(body), `"code":"missing-media"`) {
		t.Fatalf("audit did not see the stored record: %s", body)
	}
	if strings.Contains(string(body), "data/wallpapers.json") {
		t.Fatalf("findings must not reference the legacy file: %s", body)
	}
	if _, err := os.Stat(filepath.Join(root, "data/wallpapers.json")); !os.IsNotExist(err) {
		t.Fatal("audit must not create the legacy JSON file")
	}
}
