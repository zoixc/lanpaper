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

func benchmarkJSONCommit(_ string, data map[string]*Wallpaper) error {
	return json.NewEncoder(io.Discard).Encode(data)
}

// BenchmarkStoreScale measures selectors and the complete map-copy + JSON
// serialization mutation path. Filesystem durability is intentionally excluded
// here because runner disks obscure the scaling curve; BenchmarkAtomicWriteScale
// below measures the end-to-end durable write separately.
func BenchmarkStoreScale(b *testing.B) {
	originalWrite := writeFile
	writeFile = benchmarkJSONCommit
	b.Cleanup(func() { writeFile = originalWrite })

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
				store := &Store{}
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

func BenchmarkAtomicWriteScale(b *testing.B) {
	for _, size := range scaleSizes {
		data := scaleWallpapers(size)
		b.Run(fmt.Sprintf("records=%d", size), func(b *testing.B) {
			root := b.TempDir()
			b.ReportAllocs()
			for b.Loop() {
				if err := atomicWrite(root+"/wallpapers.json", data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
