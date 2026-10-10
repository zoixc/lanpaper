// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteWallpaperStoreConformance(t *testing.T) {
	root := t.TempDir()
	runWallpaperStoreConformance(t, func(t *testing.T) WallpaperStore {
		store, err := OpenSQLiteWallpaperStore(context.Background(), filepath.Join(root, "metadata.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		return store
	})
}

func TestSQLiteNormalizedRelationsIntegrityAndBackup(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := OpenSQLiteWallpaperStore(ctx, filepath.Join(root, "metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	wallpaper := &Wallpaper{
		ID: "wall", LinkName: "wall", Category: "work", AccessLevel: "token", AccessToken: "secret",
		HasImage: true, MIMEType: "png", ImageURL: "/wall", Preview: "/api/preview/wall",
		History: []HistoryEntry{{Version: 2, Ext: "png", SizeBytes: 12, ModTime: 3, SavedAt: 4}},
		Items:   []PlaylistItem{{ID: 1, Ext: "webp", SizeBytes: 7, ModTime: 8, AddedAt: 9}},
		Rotate:  &RotateConfig{Enabled: true, Interval: 30, Order: "random"},
	}
	if err := store.Create(ctx, wallpaper); err != nil {
		t.Fatal(err)
	}
	loaded, exists, err := store.Get(ctx, "wall")
	if err != nil || !exists {
		t.Fatalf("get: exists=%v err=%v", exists, err)
	}
	if len(loaded.History) != 1 || loaded.History[0].SavedAt != 4 || len(loaded.Items) != 1 || loaded.Rotate == nil || loaded.Rotate.Order != "random" {
		t.Fatalf("relations did not round trip: %+v", loaded)
	}
	renamed, err := store.Rename(ctx, "wall", "renamed")
	if err != nil || renamed.Preview != "/api/preview/renamed" || renamed.ImagePath != MediaPath("renamed", "png") {
		t.Fatalf("rename paths: wallpaper=%+v err=%v", renamed, err)
	}
	if err := store.IntegrityCheck(ctx); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(root, "backups", "metadata.db")
	if err := store.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(backup); err != nil || info.Size() == 0 {
		t.Fatalf("backup: info=%v err=%v", info, err)
	}

	copyStore, err := OpenSQLiteWallpaperStore(ctx, backup)
	if err != nil {
		t.Fatal(err)
	}
	defer copyStore.Close()
	if value, ok, err := copyStore.Get(ctx, "renamed"); err != nil || !ok || value.AccessToken != "secret" {
		t.Fatalf("backup record: value=%+v ok=%v err=%v", value, ok, err)
	}
}

func TestSQLiteForeignKeysAndWAL(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteWallpaperStore(ctx, filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var foreignKeys int
	if err := store.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatalf("foreign_keys=%d err=%v", foreignKeys, err)
	}
	var journal string
	if err := store.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil || journal != "wal" {
		t.Fatalf("journal_mode=%q err=%v", journal, err)
	}
}
