// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"maps"
)

// WallpaperStore is the context-aware metadata boundary used by application
// services. Implementations must return independent records and make Transact
// atomic: callback failure or context cancellation publishes no changes.
type WallpaperStore interface {
	Get(context.Context, string) (*Wallpaper, bool, error)
	List(context.Context) ([]*Wallpaper, error)
	Create(context.Context, *Wallpaper) error
	Update(context.Context, string, func(*Wallpaper) error) (*Wallpaper, error)
	Rename(context.Context, string, string) (*Wallpaper, error)
	Delete(context.Context, string) (*Wallpaper, error)
	Transact(context.Context, func(WallpaperTransaction) error) error
}

// WallpaperTransaction exposes CRUD over one isolated metadata snapshot.
type WallpaperTransaction interface {
	Get(string) (*Wallpaper, bool)
	Create(*Wallpaper) error
	Update(string, func(*Wallpaper) error) (*Wallpaper, error)
	Rename(string, string) (*Wallpaper, error)
	Delete(string) (*Wallpaper, error)
	List() []*Wallpaper
}

// JSONWallpaperStore adapts the existing durable JSON Store without changing
// its on-disk format. Direct mutations retain their established behavior;
// Transact performs one full-file durable commit.
type JSONWallpaperStore struct{ Store *Store }

func NewJSONWallpaperStore(store *Store) *JSONWallpaperStore {
	if store == nil {
		store = Global
	}
	return &JSONWallpaperStore{Store: store}
}

func contextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (s *JSONWallpaperStore) Get(ctx context.Context, id string) (*Wallpaper, bool, error) {
	if err := contextError(ctx); err != nil {
		return nil, false, err
	}
	wallpaper, exists := s.Store.Get(id)
	return wallpaper, exists, nil
}

func (s *JSONWallpaperStore) List(ctx context.Context) ([]*Wallpaper, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return s.Store.GetAll(), nil
}

func (s *JSONWallpaperStore) Create(ctx context.Context, wallpaper *Wallpaper) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	return s.Store.Create(wallpaper)
}

func (s *JSONWallpaperStore) Update(ctx context.Context, id string, edit func(*Wallpaper) error) (*Wallpaper, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return s.Store.Update(id, edit)
}

func (s *JSONWallpaperStore) Rename(ctx context.Context, oldName, newName string) (*Wallpaper, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return s.Store.Rename(oldName, newName)
}

func (s *JSONWallpaperStore) Delete(ctx context.Context, id string) (*Wallpaper, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return s.Store.DeleteEntry(id)
}

type jsonTransaction struct {
	wallpapers map[string]*Wallpaper
	generation uint64
}

func cloneMap(source map[string]*Wallpaper) map[string]*Wallpaper {
	cloned := make(map[string]*Wallpaper, len(source))
	for id, wallpaper := range source {
		cloned[id] = cloneWallpaper(wallpaper)
	}
	return cloned
}

func (tx *jsonTransaction) Get(id string) (*Wallpaper, bool) {
	wallpaper, exists := tx.wallpapers[id]
	if !exists || wallpaper == nil {
		return nil, false
	}
	return cloneWallpaper(wallpaper), true
}

func (tx *jsonTransaction) Create(wallpaper *Wallpaper) error {
	if _, exists := tx.wallpapers[wallpaper.LinkName]; exists {
		return ErrExists
	}
	clone := cloneWallpaper(wallpaper)
	clone.Version = tx.generation
	tx.wallpapers[clone.LinkName] = clone
	return nil
}

func (tx *jsonTransaction) Update(id string, edit func(*Wallpaper) error) (*Wallpaper, error) {
	wallpaper, exists := tx.wallpapers[id]
	if !exists || wallpaper == nil {
		return nil, ErrNotFound
	}
	clone := cloneWallpaper(wallpaper)
	if err := edit(clone); err != nil {
		return nil, err
	}
	clone.Version = tx.generation
	tx.wallpapers[id] = clone
	return cloneWallpaper(clone), nil
}

func (tx *jsonTransaction) Rename(oldName, newName string) (*Wallpaper, error) {
	wallpaper, exists := tx.wallpapers[oldName]
	if !exists || wallpaper == nil {
		return nil, ErrNotFound
	}
	if _, exists := tx.wallpapers[newName]; exists {
		return nil, ErrExists
	}
	clone := cloneWallpaper(wallpaper)
	clone.ID, clone.LinkName, clone.ImageURL = newName, newName, "/"+newName
	clone.Version = tx.generation
	if clone.HasImage && clone.MIMEType != "" {
		clone.ImagePath = MediaPath(newName, clone.MIMEType)
		if clone.MIMEType != "mp4" && clone.MIMEType != "webm" {
			clone.PreviewPath = PreviewFilePath(newName)
			clone.Preview = "/api/preview/" + newName
		}
	}
	delete(tx.wallpapers, oldName)
	tx.wallpapers[newName] = clone
	return cloneWallpaper(clone), nil
}

func (tx *jsonTransaction) Delete(id string) (*Wallpaper, error) {
	wallpaper, exists := tx.wallpapers[id]
	if !exists || wallpaper == nil {
		return nil, ErrNotFound
	}
	delete(tx.wallpapers, id)
	return cloneWallpaper(wallpaper), nil
}

func (tx *jsonTransaction) List() []*Wallpaper {
	list := make([]*Wallpaper, 0, len(tx.wallpapers))
	for _, wallpaper := range tx.wallpapers {
		list = append(list, cloneWallpaper(wallpaper))
	}
	sortSnap(list)
	return list
}

func (s *JSONWallpaperStore) Transact(ctx context.Context, apply func(WallpaperTransaction) error) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	s.Store.writeMu.Lock()
	defer s.Store.writeMu.Unlock()
	s.Store.RLock()
	tx := &jsonTransaction{wallpapers: cloneMap(s.Store.wallpapers), generation: s.Store.generation + 1}
	s.Store.RUnlock()
	if err := apply(tx); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	return s.Store.commit(maps.Clone(tx.wallpapers))
}

var _ WallpaperStore = (*JSONWallpaperStore)(nil)
