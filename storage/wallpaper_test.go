// SPDX-License-Identifier: MIT

package storage

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"lanpaper/config"
)

func testStorageDir(t *testing.T) {
	t.Helper()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Error(err)
		}
	})
	for _, dir := range []string{"data/media", "data/previews", "static/images/previews"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStoreCommitsAtomicallyAndReturnsCopies(t *testing.T) {
	testStorageDir(t)
	s := &Store{legacyJSON: true}
	if err := s.Create(&Wallpaper{ID: "a", LinkName: "a", Category: "other", AccessLevel: config.AccessToken, AccessToken: "secret"}); err != nil {
		t.Fatal(err)
	}
	read, _ := s.Get("a")
	read.Category = "modified"
	list := s.GetAll()
	list[0].AccessToken = "changed"
	if fresh, _ := s.Get("a"); fresh.Category != "other" || fresh.AccessToken != "secret" {
		t.Fatalf("mutable pointer escaped Store: %+v", fresh)
	}
	original, err := os.ReadFile(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dataFile, dataFile+".orig"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dataFile, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update("a", func(w *Wallpaper) error { w.Category = "work"; return nil }); err == nil {
		t.Fatal("Store.Update returned success despite failed disk commit")
	}
	if err := s.Create(&Wallpaper{ID: "b", LinkName: "b"}); err == nil {
		t.Fatal("Store.Create returned success despite failed disk commit")
	}
	if _, err := s.DeleteEntry("a"); err == nil {
		t.Fatal("Store.DeleteEntry returned success despite failed disk commit")
	}
	if _, err := s.Rename("a", "c"); err == nil {
		t.Fatal("Store.Rename returned success despite failed disk commit")
	}
	if _, exists := s.Get("b"); exists {
		t.Fatal("failed create appeared in memory")
	}
	if _, exists := s.Get("c"); exists {
		t.Fatal("failed rename appeared in memory")
	}
	if fresh, exists := s.Get("a"); !exists || fresh.Category != "other" || fresh.AccessToken != "secret" {
		t.Fatalf("failed transaction changed memory: %+v", fresh)
	}
	if err := os.Remove(dataFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dataFile+".orig", dataFile); err != nil {
		t.Fatal(err)
	}
	if persisted, err := os.ReadFile(dataFile); err != nil || !bytes.Equal(persisted, original) {
		t.Fatalf("failed transaction modified disk: %v", err)
	}
	loaded := &Store{legacyJSON: true}
	if err := loaded.Load(); err != nil {
		t.Fatal(err)
	}
	if wp, ok := loaded.Get("a"); !ok || wp.Category != "other" {
		t.Fatalf("reloaded state incorrect: %+v", wp)
	}
}

// A snapshot that shares its Items or History backing array with the live
// record is not a snapshot: mutating what the caller was handed would rewrite
// the store behind its back.
func TestStoreSnapshotsDoNotShareSlices(t *testing.T) {
	testStorageDir(t)
	s := &Store{legacyJSON: true}
	created := &Wallpaper{
		ID: "p", LinkName: "p", Category: "other", AccessLevel: config.AccessPublic,
		Items:   []PlaylistItem{{ID: 1, Ext: "jpg"}, {ID: 2, Ext: "png"}},
		History: []HistoryEntry{{Version: 1, Ext: "jpg", SizeBytes: 10}},
	}
	if err := s.Create(created); err != nil {
		t.Fatal(err)
	}

	read, ok := s.Get("p")
	if !ok {
		t.Fatal("created record is missing")
	}
	read.Items[0].Ext = "tampered"
	read.History[0].Ext = "tampered"

	snap := s.GetAll()
	if len(snap) != 1 {
		t.Fatalf("unexpected snapshot size: %d", len(snap))
	}
	snap[0].Items[1].ID = 99
	snap[0].Items = append(snap[0].Items, PlaylistItem{ID: 3, Ext: "gif"})

	fresh, _ := s.Get("p")
	if len(fresh.Items) != 2 || fresh.Items[0].Ext != "jpg" || fresh.Items[1].ID == 99 {
		t.Fatalf("playlist items escaped the store: %+v", fresh.Items)
	}
	if len(fresh.History) != 1 || fresh.History[0].Ext != "jpg" {
		t.Fatalf("history escaped the store: %+v", fresh.History)
	}
}

func TestStoreLoadRefusesCorruptAndUnsafeData(t *testing.T) {
	testStorageDir(t)
	s := &Store{legacyJSON: true}
	if err := s.Create(&Wallpaper{LinkName: "keep", Category: "other"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, body string }{
		{"malformed", `{broken`},
		{"null root", `null`},
		{"invalid link key", `{"../escape":{"hasImage":true,"mimeType":"png"}}`},
		{"nil record", `{"keep":null}`},
		{"unsupported media type", `{"keep":{"hasImage":true,"mimeType":"html"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(dataFile, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			if err := s.Load(); err == nil {
				t.Fatal("Load silently dropped or replaced malformed metadata")
			}
			if _, ok := s.Get("keep"); !ok {
				t.Fatal("failed Load replaced existing state")
			}
			if unchanged, _ := os.ReadFile(dataFile); string(unchanged) != tc.body {
				t.Fatal("invalid metadata was rewritten")
			}
		})
	}
	// A legacy record may have untrusted ID/paths; the validated map key is
	// the only authority for filenames and URLs, and unknown access fails shut.
	body := `{"valid":{"id":"../../secret","linkName":"../bad","hasImage":true,"mimeType":"png","accessLevel":"unknown"}}`
	if err := os.WriteFile(dataFile, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if wp, ok := s.Get("valid"); !ok || wp.ID != "valid" || wp.LinkName != "valid" ||
		wp.ImagePath != MediaPath("valid", "png") || wp.AccessLevel != config.AccessAuth {
		t.Fatalf("unsafe persisted field escaped validation: %+v", wp)
	}
}

func TestConcurrentUpdatesAndSnapshots(t *testing.T) {
	testStorageDir(t)
	s := &Store{legacyJSON: true}
	if err := s.Create(&Wallpaper{LinkName: "counter", Category: "other"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 25 {
				if _, err := s.Update("counter", func(wp *Wallpaper) error { wp.SizeBytes++; return nil }); err != nil {
					t.Errorf("concurrent update: %v", err)
					return
				}
				for _, snapshot := range s.GetAll() {
					snapshot.SizeBytes = -1 // must not mutate store or cache
				}
			}
		})
	}
	wg.Wait()
	if wp, _ := s.Get("counter"); wp.SizeBytes != 200 {
		t.Fatalf("lost updates or snapshot escaped cache: %d", wp.SizeBytes)
	}
	loaded := &Store{legacyJSON: true}
	if err := loaded.Load(); err != nil {
		t.Fatal(err)
	}
	if wp, _ := loaded.Get("counter"); wp.SizeBytes != 200 {
		t.Fatalf("lost persisted updates: %d", wp.SizeBytes)
	}
}

func TestLegacyMigrationNeverFollowsFileSymlink(t *testing.T) {
	testStorageDir(t)
	oldGlobal := Global
	Global = &Store{legacyJSON: true}
	t.Cleanup(func() { Global = oldGlobal })
	outside := filepath.Join(t.TempDir(), "private.png")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(config.LegacyMedia, "legacy.png")); err != nil {
		t.Fatal(err)
	}
	if err := Global.Create(&Wallpaper{ID: "legacy", LinkName: "legacy", HasImage: true, MIMEType: "png"}); err != nil {
		t.Fatal(err)
	}
	MigrateMediaToDataDir()
	if _, err := os.Stat(filepath.Join(config.MediaDir, "legacy.png")); !os.IsNotExist(err) {
		t.Fatalf("legacy symlink was migrated or copied: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(config.LegacyMedia, "legacy.png")); err != nil {
		t.Fatal("legacy symlink was removed: ", err)
	}
	if f, err := OpenMedia(filepath.Join(config.LegacyMedia, "legacy.png")); err == nil {
		f.Close()
		t.Fatal("OpenMedia followed planted symlink")
	}
}

func TestPruneKeepsLinksAndTheirAccessControls(t *testing.T) {
	testStorageDir(t)
	oldGlobal := Global
	Global = &Store{legacyJSON: true}
	t.Cleanup(func() { Global = oldGlobal })
	for index, name := range []string{"old", "new"} {
		path := MediaPath(name, "png")
		if err := os.WriteFile(path, []byte(fmt.Sprint(index)), 0600); err != nil {
			t.Fatal(err)
		}
		if err := Global.Create(&Wallpaper{ID: name, LinkName: name, HasImage: true, MIMEType: "png",
			ImagePath: path, ModTime: int64(index + 1), AccessLevel: config.AccessToken, AccessToken: "token-" + name}); err != nil {
			t.Fatal(err)
		}
	}
	PruneOldImages(1)
	old, _ := Global.Get("old")
	fresh, _ := Global.Get("new")
	if old == nil || old.HasImage || old.AccessLevel != config.AccessToken || old.AccessToken != "token-old" ||
		fresh == nil || !fresh.HasImage {
		t.Fatalf("prune deleted links or access settings: old=%+v new=%+v", old, fresh)
	}
	if _, err := os.Stat(MediaPath("old", "png")); !os.IsNotExist(err) {
		t.Fatalf("old media not removed: %v", err)
	}
}
