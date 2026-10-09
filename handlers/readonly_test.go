// SPDX-License-Identifier: MIT

package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"lanpaper/storage"
)

func TestIsReadOnlyErrorClassifiesEROFS(t *testing.T) {
	wrapped := &os.PathError{Op: "remove", Path: "x", Err: syscall.EROFS}
	if !storage.IsReadOnlyError(wrapped) {
		t.Fatal("EROFS inside PathError must be read-only")
	}
	if storage.IsReadOnlyError(&os.PathError{Op: "remove", Path: "x", Err: syscall.EACCES}) {
		t.Fatal("EACCES is a permission problem, not read-only")
	}
	if storage.IsReadOnlyError(fmt.Errorf("other: %w", os.ErrNotExist)) {
		t.Fatal("ENOENT must not be read-only")
	}
}

// TestRemoveFileOnReadOnlyMount needs a directory that really is read-only.
// It is skipped unless LANPAPER_RO_DIR points at one that holds a file named
// test.webp, e.g. inside `unshare -rm` after a bind remount with `ro`.
func TestRemoveFileOnReadOnlyMount(t *testing.T) {
	dir := os.Getenv("LANPAPER_RO_DIR")
	if dir == "" {
		t.Skip("set LANPAPER_RO_DIR to a read-only directory containing test.webp")
	}
	path := filepath.Join(dir, "test.webp")
	if removeFile(path, "preview") {
		t.Fatal("removal on a read-only mount must report that the file is still there")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file must still exist: %v", err)
	}
}
