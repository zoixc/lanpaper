// SPDX-License-Identifier: MIT

package handlers

import (
	"errors"
	"testing"

	"lanpaper/storage"
)

func TestLibraryServicePlaylistMutationIsAtomicInMetadata(t *testing.T) {
	service := NewLibraryService(&storage.Store{})
	wall := &storage.Wallpaper{Items: []storage.PlaylistItem{{ID: 1, Ext: "png"}, {ID: 2, Ext: "jpg"}}}
	removed, err := service.removePlaylistMetadata(wall, 1)
	if err != nil {
		t.Fatal(err)
	}
	if removed.ID != 1 || len(wall.Items) != 1 || wall.Items[0].ID != 2 {
		t.Fatalf("removed=%#v remaining=%#v", removed, wall.Items)
	}
	before := len(wall.Items)
	if _, err := service.removePlaylistMetadata(wall, 99); !errors.Is(err, errItemNotFound) {
		t.Fatalf("error=%v", err)
	}
	if len(wall.Items) != before {
		t.Fatal("failed mutation changed playlist")
	}
}

func TestLibraryErrorPreservesStoreCause(t *testing.T) {
	cause := storage.ErrNotFound
	err := &LibraryError{Operation: "remove history", Err: cause}
	if !errors.Is(err, cause) || !IsLibraryNotFound(err) {
		t.Fatal("typed library error lost not-found cause")
	}
}
