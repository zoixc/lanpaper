// SPDX-License-Identifier: MIT

package handlers

import (
	"errors"
	"os"

	"lanpaper/storage"
)

// LibraryService encapsulates history and playlist metadata/file invariants.
type LibraryService struct{ Store *storage.Store }

func NewLibraryService(store *storage.Store) *LibraryService {
	if store == nil {
		store = storage.Global
	}
	return &LibraryService{Store: store}
}

func defaultLibraryService() *LibraryService {
	return NewLibraryService(storage.Global)
}

type LibraryError struct {
	Operation string
	Err       error
}

func (e *LibraryError) Error() string { return e.Operation + ": " + e.Err.Error() }
func (e *LibraryError) Unwrap() error { return e.Err }

func (s *LibraryService) RemoveHistory(name string, version uint64) (*storage.Wallpaper, error) {
	wp, err := s.Store.RemoveHistoryEntry(name, version)
	if err != nil {
		return nil, &LibraryError{"remove history", err}
	}
	return wp, nil
}

func (s *LibraryService) removePlaylistMetadata(w *storage.Wallpaper, id int) (storage.PlaylistItem, error) {
	items, item, ok := storage.WithoutItem(w.Items, id)
	if !ok {
		return storage.PlaylistItem{}, errItemNotFound
	}
	w.Items = items
	return item, nil
}
func (s *LibraryService) cleanupPlaylistItem(name string, item storage.PlaylistItem) error {
	path := storage.ItemPath(name, item.ID, item.Ext)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return &LibraryError{"remove playlist bytes", err}
	}
	_ = os.Remove(storage.ItemsDirPath(name))
	return nil
}
func IsLibraryNotFound(err error) bool {
	return errors.Is(err, storage.ErrNotFound) || errors.Is(err, errItemNotFound)
}
