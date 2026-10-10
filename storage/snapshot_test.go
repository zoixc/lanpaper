// SPDX-License-Identifier: MIT

package storage

import (
	"fmt"
	"testing"
)

func snapshotStore(n int) *Store {
	s := &Store{legacyJSON: true, wallpapers: make(map[string]*Wallpaper, n)}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("link-%02d", i)
		s.wallpapers[name] = &Wallpaper{ID: name, LinkName: name, CreatedAt: int64(i)}
	}
	return s
}

// Snapshot must list the same records in the same order as GetAll, and hand
// out the stored records themselves rather than copies.
func TestSnapshotMatchesGetAllWithoutCopying(t *testing.T) {
	s := snapshotStore(20)
	all := s.GetAll()
	snap := s.Snapshot()
	if len(all) != len(snap) {
		t.Fatalf("len: GetAll %d, Snapshot %d", len(all), len(snap))
	}
	for i := range snap {
		if snap[i].LinkName != all[i].LinkName {
			t.Fatalf("order differs at %d: %s vs %s", i, snap[i].LinkName, all[i].LinkName)
		}
		if snap[i] != s.wallpapers[snap[i].LinkName] {
			t.Fatalf("Snapshot copied record %s instead of sharing it", snap[i].LinkName)
		}
	}
}

// Reordering the returned slice must not change the order other callers see.
func TestSnapshotSliceIsPrivateToCaller(t *testing.T) {
	s := snapshotStore(5)
	names := func(list []*Wallpaper) []string {
		out := make([]string, len(list))
		for i, wp := range list {
			out[i] = wp.LinkName
		}
		return out
	}
	before := names(s.Snapshot())

	mine := s.Snapshot()
	mine[0], mine[len(mine)-1] = mine[len(mine)-1], mine[0]
	mine = mine[:1]
	_ = mine

	if after := names(s.Snapshot()); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("caller changed the shared order: before %v, after %v", before, after)
	}
}
