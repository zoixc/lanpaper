// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDataDirLockIsExclusive(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("flock is enforced on Linux only")
	}
	dir := filepath.Join(t.TempDir(), "data")
	unlock, err := LockDataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LockDataDir(dir); err == nil {
		unlock()
		t.Fatal("second lock on the same data directory was granted")
	}
	unlock()
	again, err := LockDataDir(dir)
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	again()
}

// The database holds access tokens and session digests, so it must never be
// readable by other users, including its WAL and shared-memory companions.
func TestDatabaseFilesAreOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data", "wallpapers.db")
	store, err := OpenSQLiteWallpaperStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Create(context.Background(), &Wallpaper{ID: "x", LinkName: "x", Category: "other"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Fatalf("%s mode %v grants access to group or others", name, perm)
		}
	}
}
