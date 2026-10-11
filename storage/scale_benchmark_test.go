// SPDX-License-Identifier: MIT

package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"testing"
)

var scaleSizes = []int{1_000, 10_000, 50_000}

func scaleWallpapers(size int) map[string]*Wallpaper {
	wallpapers := make(map[string]*Wallpaper, size)
	for index := 0; index < size; index++ {
		name := fmt.Sprintf("wall-%05d", index)
		wallpapers[name] = &Wallpaper{
			ID: name, LinkName: name, Category: "other", AccessLevel: "public",
			CreatedAt: int64(index), SizeBytes: int64(index * 1024),
		}
	}
	return wallpapers
}

// benchmarkSerializeCommit does the same work as the old JSON writer's encoding
// step and discards the bytes, so the mutation benchmarks keep their shape
// without disk I/O.
func benchmarkSerializeCommit(data map[string]*Wallpaper) error {
	return json.NewEncoder(io.Discard).Encode(data)
}

// BenchmarkStoreScale measures selectors and the in-memory mutation path with
// the commit replaced by serialization only. Durable SQLite commits are
// measured by BenchmarkDatabaseScale in database_bench_test.go.
func BenchmarkStoreScale(b *testing.B) {
	withCommitHookBench(b, benchmarkSerializeCommit)

	for _, size := range scaleSizes {
		seed := scaleWallpapers(size)
		b.Run(fmt.Sprintf("records=%d/list", size), func(b *testing.B) {
			store := &Store{wallpapers: maps.Clone(seed)}
			b.ReportAllocs()
			for b.Loop() {
				_ = store.GetAll()
			}
		})
		for _, operation := range []string{"create", "update", "rename", "delete"} {
			b.Run(fmt.Sprintf("records=%d/%s", size, operation), func(b *testing.B) {
				store := &Store{wallpapers: make(map[string]*Wallpaper)}
				b.ReportAllocs()
				for b.Loop() {
					b.StopTimer()
					store.wallpapers = maps.Clone(seed)
					store.sortedSnap = nil
					b.StartTimer()
					switch operation {
					case "create":
						_ = store.Create(&Wallpaper{ID: "added", LinkName: "added", Category: "other"})
					case "update":
						_, _ = store.Update("wall-00000", func(wallpaper *Wallpaper) error {
							wallpaper.Category = "work"
							return nil
						})
					case "rename":
						_, _ = store.Rename("wall-00000", "renamed")
					case "delete":
						_, _ = store.DeleteEntry("wall-00000")
					}
				}
			})
		}
	}
}

func withCommitHookBench(b *testing.B, hook func(map[string]*Wallpaper) error) {
	saved := commitHook
	commitHook = hook
	b.Cleanup(func() { commitHook = saved })
}
