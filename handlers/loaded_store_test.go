// SPDX-License-Identifier: MIT

package handlers

import (
	"testing"

	"lanpaper/storage"
)

// loadedStore returns a SQLite-backed store in the current directory.
func loadedStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.NewLoadedStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
