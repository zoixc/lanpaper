// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const resumeFixture = `{
  "a": {"linkName":"a","category":"other"},
  "b": {"linkName":"b","category":"nature"},
  "c": {"linkName":"c","category":"other"}
}`

func writeResumeFixture(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(dataFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func loadedNames(t *testing.T) map[string]bool {
	t.Helper()
	db, err := OpenSQLiteWallpaperStore(context.Background(), databaseFile)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	all, err := db.loadAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool, len(all))
	for name := range all {
		names[name] = true
	}
	return names
}

func backupCount(t *testing.T) int {
	t.Helper()
	matches, err := filepath.Glob(dataFile + ".pre-sqlite-*.bak")
	if err != nil {
		t.Fatal(err)
	}
	return len(matches)
}

// An import that is interrupted after its checkpoint was written resumes from
// that checkpoint and finishes the job, without a second backup.
func TestImportResumesAfterInterruptedRun(t *testing.T) {
	testStorageDir(t)
	writeResumeFixture(t, resumeFixture)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := importLegacyMetadataAt(cancelled, databaseFile, dataFile); err == nil {
		t.Fatal("cancelled import reported success")
	}
	if !fileExists(databaseFile + ".migration.json") {
		t.Fatal("interrupted import left no checkpoint to resume from")
	}

	if err := importLegacyMetadataAt(context.Background(), databaseFile, dataFile); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if names := loadedNames(t); len(names) != 3 || !names["a"] || !names["b"] || !names["c"] {
		t.Fatalf("resumed import records = %v, want a, b, c", names)
	}
	if fileExists(databaseFile + ".migration.json") {
		t.Fatal("checkpoint kept after a completed import")
	}
	if n := backupCount(t); n != 1 {
		t.Fatalf("backups after resume = %d, want exactly the first one", n)
	}
	if !fileExists(dataFile) {
		t.Fatal("legacy JSON was removed by the import")
	}
}

// If the process stopped after one batch was committed but before the
// checkpoint moved on, the resumed run must not duplicate that batch.
func TestImportResumeSkipsCommittedBatch(t *testing.T) {
	testStorageDir(t)
	writeResumeFixture(t, resumeFixture)

	backup := dataFile + ".pre-sqlite-manual.bak"
	if err := durableCopy(dataFile, backup); err != nil {
		t.Fatal(err)
	}
	_, _, names, checksum, err := readMigrationSource(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenSQLiteWallpaperStore(context.Background(), databaseFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(context.Background(), &Wallpaper{ID: names[0], LinkName: names[0], Category: "other"}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	checkpoint := sqliteMigrationCheckpoint{
		Version: 1, Checksum: checksum, Source: dataFile, Destination: databaseFile,
		Total: len(names), Backup: backup, Next: 1,
	}
	if err := writeMigrationCheckpoint(databaseFile+".migration.json", checkpoint); err != nil {
		t.Fatal(err)
	}

	if err := importLegacyMetadataAt(context.Background(), databaseFile, dataFile); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if got := loadedNames(t); len(got) != 3 {
		t.Fatalf("records after resume = %v, want 3", got)
	}
}

// A checkpoint pins the checksum of the JSON it was made from. If the file
// changed in the meantime, the import is refused instead of mixing sources.
func TestImportResumeRefusesChangedSource(t *testing.T) {
	testStorageDir(t)
	writeResumeFixture(t, resumeFixture)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_ = importLegacyMetadataAt(cancelled, databaseFile, dataFile)

	writeResumeFixture(t, `{"a":{"linkName":"a","category":"work"}}`)
	if err := importLegacyMetadataAt(context.Background(), databaseFile, dataFile); err == nil {
		t.Fatal("resume accepted a source that changed after the checkpoint")
	}
}

// A checkpoint without its source file must not be silently ignored either.
func TestImportResumeRefusesMissingSource(t *testing.T) {
	testStorageDir(t)
	writeResumeFixture(t, resumeFixture)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_ = importLegacyMetadataAt(cancelled, databaseFile, dataFile)
	if err := os.Remove(dataFile); err != nil {
		t.Fatal(err)
	}
	if err := importLegacyMetadataAt(context.Background(), databaseFile, dataFile); err == nil {
		t.Fatal("checkpoint with a missing source was accepted")
	}
}
