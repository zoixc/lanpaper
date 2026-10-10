// SPDX-License-Identifier: MIT

package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func newDatabaseTestStore(t *testing.T) *Store {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("data", 0o700); err != nil {
		t.Fatal(err)
	}
	s := &Store{wallpapers: make(map[string]*Wallpaper)}
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func sampleWallpaper(name string) *Wallpaper {
	return &Wallpaper{
		ID: name, LinkName: name, Category: "work", AccessLevel: "token", AccessToken: "secret",
		HasImage: true, MIMEType: "png", ImageURL: "/" + name, Preview: "/api/preview/" + name,
		History: []HistoryEntry{{Version: 2, Ext: "png", SizeBytes: 12, ModTime: 3, SavedAt: 4}},
		Items:   []PlaylistItem{{ID: 1, Ext: "webp", SizeBytes: 7, ModTime: 8, AddedAt: 9}},
		Rotate:  &RotateConfig{Enabled: true, Interval: 30, Order: "random"},
	}
}

func reopen(t *testing.T) *Store {
	t.Helper()
	s := &Store{wallpapers: make(map[string]*Wallpaper)}
	if err := s.Load(); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestDatabaseRoundTripAcrossRestart(t *testing.T) {
	s := newDatabaseTestStore(t)
	if err := s.Create(sampleWallpaper("wall")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update("wall", func(wp *Wallpaper) error {
		wp.AccessLevel = "public"
		wp.AccessToken = ""
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	got := reopen(t)
	wp, ok := got.Get("wall")
	if !ok {
		t.Fatal("record lost across restart")
	}
	if wp.AccessLevel != "public" || len(wp.History) != 1 || len(wp.Items) != 1 || wp.Rotate == nil || wp.Rotate.Interval != 30 {
		t.Fatalf("relations not restored: %+v", wp)
	}
	if _, err := os.Stat(filepath.Join("data", "wallpapers.json")); !os.IsNotExist(err) {
		t.Fatalf("a fresh install must not create the legacy JSON file: %v", err)
	}
}

func TestLegacyJSONIsImportedOnceAndKept(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("data", 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]*Wallpaper{"old": sampleWallpaper("old")}
	body, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataFile, body, 0o600); err != nil {
		t.Fatal(err)
	}

	s := &Store{wallpapers: make(map[string]*Wallpaper)}
	if err := s.Load(); err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, ok := s.Get("old"); !ok {
		t.Fatal("legacy record not imported")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if kept, err := os.ReadFile(dataFile); err != nil || !bytes.Equal(kept, body) {
		t.Fatal("legacy JSON must be left untouched")
	}
	backups, _ := filepath.Glob(filepath.Join("data", "wallpapers.json.pre-sqlite-*.bak"))
	if len(backups) != 1 {
		t.Fatalf("expected one verified backup, got %v", backups)
	}

	// Once the database exists the JSON file is ignored, so a stale file can
	// never overwrite newer database state.
	if err := os.WriteFile(dataFile, []byte(`{"stale":{"linkName":"stale"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	again := reopen(t)
	if _, ok := again.Get("stale"); ok {
		t.Fatal("stale JSON was imported over an existing database")
	}
	if _, ok := again.Get("old"); !ok {
		t.Fatal("database record disappeared")
	}
}

func TestInvalidLegacyJSONCreatesNoDatabase(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("data", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataFile, []byte(`{"bad name!":{"linkName":"bad name!"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{wallpapers: make(map[string]*Wallpaper)}
	if err := s.Load(); err == nil {
		t.Fatal("invalid legacy record must stop startup")
	}
	if _, err := os.Stat(databaseFile); !os.IsNotExist(err) {
		t.Fatalf("no database may be created from invalid input: %v", err)
	}
}

func TestRenameKeepsRelationsAndDeleteCascades(t *testing.T) {
	s := newDatabaseTestStore(t)
	if err := s.Create(sampleWallpaper("before")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rename("before", "after"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	got := reopen(t)
	if _, ok := got.Get("before"); ok {
		t.Fatal("old name still present after rename")
	}
	wp, ok := got.Get("after")
	if !ok || len(wp.History) != 1 || len(wp.Items) != 1 {
		t.Fatalf("relations not moved with the rename: %+v", wp)
	}
	if _, err := got.DeleteEntry("after"); err != nil {
		t.Fatal(err)
	}
	if err := got.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := OpenSQLiteWallpaperStore(context.Background(), databaseFile)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"history", "playlist_items", "rotations"} {
		var n int
		if err := db.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s rows left after delete: %d", table, n)
		}
	}
}

func TestFailedCommitLeavesMemoryUnchanged(t *testing.T) {
	s := newDatabaseTestStore(t)
	if err := s.Create(sampleWallpaper("kept")); err != nil {
		t.Fatal(err)
	}
	// Closing the handle makes the next durable write fail. The in-memory
	// state must not show a change the disk never received.
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(sampleWallpaper("lost")); err == nil {
		t.Fatal("expected the write to fail")
	}
	if _, ok := s.Get("lost"); ok {
		t.Fatal("failed create was published to readers")
	}
	if _, ok := s.Get("kept"); !ok {
		t.Fatal("existing record disappeared")
	}
}

func TestSameRecordIgnoresRuntimeVersion(t *testing.T) {
	a := sampleWallpaper("x")
	b := sampleWallpaper("x")
	b.Version = 99
	if !sameRecord(a, b) {
		t.Fatal("runtime version must not count as a change")
	}
	b.AccessLevel = "auth"
	if sameRecord(a, b) {
		t.Fatal("a real field change must be detected")
	}
}

func TestRepairMetadataDatabaseBacksUpAndReplaces(t *testing.T) {
	s := newDatabaseTestStore(t)
	if err := s.Create(sampleWallpaper("broken")); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(sampleWallpaper("fine")); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	records, err := ReadMetadataRecords(root)
	if err != nil {
		t.Fatal(err)
	}
	delete(records, "broken")
	backup := filepath.Join(t.TempDir(), "quarantine", "wallpapers.db.before-repair")
	if err := RepairMetadataDatabase(root, records, backup); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup not written: %v", err)
	}
	after := reopen(t)
	if _, ok := after.Get("broken"); ok {
		t.Fatal("repaired-away record still present")
	}
	if _, ok := after.Get("fine"); !ok {
		t.Fatal("healthy record lost during repair")
	}
	if !reflect.DeepEqual(len(after.GetAll()), 1) {
		t.Fatal("unexpected record count after repair")
	}
}
