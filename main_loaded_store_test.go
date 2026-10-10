// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"lanpaper/storage"
)

// loadedTestStore returns a SQLite-backed store in the current directory.
func loadedTestStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.NewLoadedStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
