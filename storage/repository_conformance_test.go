// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"errors"
	"os"
	"testing"
)

type wallpaperStoreFactory func(*testing.T) WallpaperStore

func runWallpaperStoreConformance(t *testing.T, factory wallpaperStoreFactory) {
	t.Helper()
	store := factory(t)
	ctx := context.Background()
	if err := store.Create(ctx, &Wallpaper{ID: "one", LinkName: "one", Category: "other"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	wallpaper, exists, err := store.Get(ctx, "one")
	if err != nil || !exists || wallpaper.LinkName != "one" {
		t.Fatalf("get: wallpaper=%+v exists=%v err=%v", wallpaper, exists, err)
	}
	wallpaper.Category = "outside"
	fresh, _, _ := store.Get(ctx, "one")
	if fresh.Category == "outside" {
		t.Fatal("Get exposed mutable repository state")
	}
	if _, err := store.Update(ctx, "one", func(value *Wallpaper) error {
		value.Category = "work"
		return nil
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := store.Rename(ctx, "one", "renamed"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	list, err := store.List(ctx)
	if err != nil || len(list) != 1 || list[0].LinkName != "renamed" {
		t.Fatalf("list: list=%+v err=%v", list, err)
	}
	if _, err := store.Delete(ctx, "renamed"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, exists, _ := store.Get(ctx, "renamed"); exists {
		t.Fatal("deleted record remains visible")
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.Create(cancelled, &Wallpaper{ID: "cancelled", LinkName: "cancelled"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled create: %v", err)
	}

	sentinel := errors.New("rollback")
	if err := store.Transact(ctx, func(tx WallpaperTransaction) error {
		if err := tx.Create(&Wallpaper{ID: "rolled-back", LinkName: "rolled-back"}); err != nil {
			return err
		}
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("transaction error: %v", err)
	}
	if _, exists, _ := store.Get(ctx, "rolled-back"); exists {
		t.Fatal("failed transaction published changes")
	}

	if err := store.Transact(ctx, func(tx WallpaperTransaction) error {
		if err := tx.Create(&Wallpaper{ID: "a", LinkName: "a"}); err != nil {
			return err
		}
		if err := tx.Create(&Wallpaper{ID: "b", LinkName: "b"}); err != nil {
			return err
		}
		_, err := tx.Update("a", func(value *Wallpaper) error { value.Category = "life"; return nil })
		return err
	}); err != nil {
		t.Fatalf("committed transaction: %v", err)
	}
	list, err = store.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("transaction list: list=%+v err=%v", list, err)
	}
	categories := make(map[string]string, len(list))
	for _, value := range list {
		categories[value.LinkName] = value.Category
	}
	if categories["a"] != "life" || categories["b"] != "" {
		t.Fatalf("transaction records: %+v", categories)
	}
}

func TestJSONWallpaperStoreConformance(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("data", 0o755); err != nil {
		t.Fatal(err)
	}
	runWallpaperStoreConformance(t, func(t *testing.T) WallpaperStore {
		return NewJSONWallpaperStore(&Store{})
	})
}

func TestJSONWallpaperStoreTransactionCommitsOnce(t *testing.T) {
	store := &Store{}
	repository := NewJSONWallpaperStore(store)
	original := writeFile
	commits := 0
	writeFile = func(_ string, _ map[string]*Wallpaper) error { commits++; return nil }
	t.Cleanup(func() { writeFile = original })
	if err := repository.Transact(context.Background(), func(tx WallpaperTransaction) error {
		if err := tx.Create(&Wallpaper{ID: "a", LinkName: "a"}); err != nil {
			return err
		}
		return tx.Create(&Wallpaper{ID: "b", LinkName: "b"})
	}); err != nil {
		t.Fatal(err)
	}
	if commits != 1 {
		t.Fatalf("commits=%d, want one", commits)
	}
}
