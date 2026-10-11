// SPDX-License-Identifier: MIT

package storage

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// withFakeWriter replaces the disk writer for one test. The fake records the
// last map it was asked to persist and never touches the filesystem.
func withFakeWriter(t *testing.T, fn func(path string, data map[string]*Wallpaper) error) {
	t.Helper()
	saved := writeFile
	writeFile = fn
	t.Cleanup(func() { writeFile = saved })
}

func newTestStore(names ...string) *Store {
	s := &Store{legacyJSON: true, wallpapers: make(map[string]*Wallpaper)}
	for _, n := range names {
		s.wallpapers[n] = &Wallpaper{ID: n, LinkName: n}
	}
	return s
}

// A write in progress must not block readers. Before the fix, the write ran
// under the store's write lock and Get waited for the whole fsync.
func TestReadersNotBlockedByPendingWrite(t *testing.T) {
	s := newTestStore("alpha")
	entered := make(chan struct{})
	release := make(chan struct{})
	withFakeWriter(t, func(string, map[string]*Wallpaper) error {
		close(entered)
		<-release
		return nil
	})

	done := make(chan error, 1)
	go func() { done <- s.Create(&Wallpaper{LinkName: "beta"}) }()
	<-entered

	read := make(chan struct{})
	go func() {
		s.Get("alpha")
		s.Snapshot()
		close(read)
	}()
	select {
	case <-read:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("reader blocked while a write was waiting on disk")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("beta"); !ok {
		t.Fatal("create did not publish after the write finished")
	}
}

// Concurrent writers must all land: none may overwrite another's change.
func TestConcurrentCreatesAreNotLost(t *testing.T) {
	s := newTestStore()
	var mu sync.Mutex
	var persisted int
	withFakeWriter(t, func(_ string, data map[string]*Wallpaper) error {
		mu.Lock()
		persisted = len(data)
		mu.Unlock()
		return nil
	})

	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := s.Create(&Wallpaper{LinkName: fmt.Sprintf("link-%03d", i)}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()

	if got := len(s.Snapshot()); got != n {
		t.Fatalf("in memory: %d links, want %d", got, n)
	}
	mu.Lock()
	defer mu.Unlock()
	if persisted != n {
		t.Fatalf("last persisted map had %d links, want %d", persisted, n)
	}
}

// A failed write must leave memory exactly as it was.
func TestFailedWriteDoesNotPublish(t *testing.T) {
	s := newTestStore("keep")
	withFakeWriter(t, func(string, map[string]*Wallpaper) error {
		return errors.New("disk full")
	})

	if err := s.Create(&Wallpaper{LinkName: "ghost"}); err == nil {
		t.Fatal("expected write error")
	}
	if _, ok := s.Get("ghost"); ok {
		t.Fatal("failed create was published")
	}
	if _, err := s.Update("keep", func(w *Wallpaper) error { w.Category = "x"; return nil }); err == nil {
		t.Fatal("expected write error from update")
	}
	if wp, _ := s.Get("keep"); wp.Category == "x" {
		t.Fatal("failed update was published")
	}
	if _, err := s.DeleteEntry("keep"); err == nil {
		t.Fatal("expected write error from delete")
	}
	if _, ok := s.Get("keep"); !ok {
		t.Fatal("failed delete was published")
	}
}
