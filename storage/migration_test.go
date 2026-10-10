// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeMigrationSource(t *testing.T, path string, records map[string]*Wallpaper) []byte {
	t.Helper()
	body, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestSQLiteMigrationDryRunBackupAndDowngradeSource(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := filepath.Join(root, "wallpapers.json")
	original := writeMigrationSource(t, source, map[string]*Wallpaper{
		"one": {ID: "one", LinkName: "one", Category: "work", AccessLevel: "public"},
		"two": {ID: "two", LinkName: "two", Category: "other", AccessLevel: "token", AccessToken: "secret"},
	})
	destination := filepath.Join(root, "wallpapers.db")
	options := SQLiteMigrationOptions{Source: source, Destination: destination, Backup: source + ".bak", BatchSize: 1}
	dry, err := MigrateJSONToSQLite(ctx, SQLiteMigrationOptions{Source: source, Destination: destination, DryRun: true})
	if err != nil || !dry.DryRun || !dry.Complete || dry.Total != 2 {
		t.Fatalf("dry run: %+v err=%v", dry, err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("dry run created destination: %v", err)
	}
	report, err := MigrateJSONToSQLite(ctx, options)
	if err != nil || !report.Complete || report.Migrated != 2 {
		t.Fatalf("migration: %+v err=%v", report, err)
	}
	if backup, err := os.ReadFile(options.Backup); err != nil || string(backup) != string(original) {
		t.Fatalf("backup mismatch: %v", err)
	}
	if current, err := os.ReadFile(source); err != nil || string(current) != string(original) {
		t.Fatalf("source changed: %v", err)
	}
	store, err := OpenSQLiteWallpaperStore(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	list, err := store.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("sqlite list: %d err=%v", len(list), err)
	}
}

func TestSQLiteMigrationResumesMatchingCheckpoint(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := filepath.Join(root, "wallpapers.json")
	writeMigrationSource(t, source, map[string]*Wallpaper{
		"a": {ID: "a", LinkName: "a", AccessLevel: "public"},
		"b": {ID: "b", LinkName: "b", AccessLevel: "public"},
		"c": {ID: "c", LinkName: "c", AccessLevel: "public"},
	})
	options := migrationDefaults(SQLiteMigrationOptions{Source: source, Destination: filepath.Join(root, "wallpapers.db"), Backup: source + ".bak", BatchSize: 1})
	_, records, names, checksum, err := readMigrationSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := durableCopy(source, options.Backup); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteWallpaperStore(ctx, options.Destination)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, records[names[0]]); err != nil {
		t.Fatal(err)
	}
	store.Close()
	checkpoint := sqliteMigrationCheckpoint{Version: 1, Checksum: checksum, Source: source, Destination: options.Destination, Backup: options.Backup, Next: 1, Total: 3}
	if err := writeMigrationCheckpoint(options.Checkpoint, checkpoint); err != nil {
		t.Fatal(err)
	}
	options.Resume = true
	report, err := MigrateJSONToSQLite(ctx, options)
	if err != nil || !report.Complete || report.ResumedAt != 1 || report.Migrated != 2 {
		t.Fatalf("resume: %+v err=%v", report, err)
	}
}

func TestSQLiteMigrationRejectsChangedSourceAndRollsBackDestination(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := filepath.Join(root, "wallpapers.json")
	writeMigrationSource(t, source, map[string]*Wallpaper{"a": {ID: "a", LinkName: "a"}})
	options := migrationDefaults(SQLiteMigrationOptions{Source: source, Destination: filepath.Join(root, "wallpapers.db"), Backup: source + ".bak"})
	_, _, _, checksum, _ := readMigrationSource(source)
	if err := durableCopy(source, options.Backup); err != nil {
		t.Fatal(err)
	}
	if err := writeMigrationCheckpoint(options.Checkpoint, sqliteMigrationCheckpoint{Version: 1, Checksum: checksum, Source: source, Destination: options.Destination, Backup: options.Backup, Total: 1}); err != nil {
		t.Fatal(err)
	}
	writeMigrationSource(t, source, map[string]*Wallpaper{"changed": {ID: "changed", LinkName: "changed"}})
	options.Resume = true
	if _, err := MigrateJSONToSQLite(ctx, options); err == nil {
		t.Fatal("resume accepted changed source")
	}
	if err := os.WriteFile(options.Destination, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RollbackSQLiteMigration(options); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(options.Destination); !os.IsNotExist(err) {
		t.Fatalf("destination remains: %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("rollback removed source: %v", err)
	}
	if _, err := os.Stat(options.Backup); err != nil {
		t.Fatalf("rollback removed backup: %v", err)
	}
}
