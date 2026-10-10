// SPDX-License-Identifier: MIT

package storage

import (
	"fmt"
	"os"
	"testing"
)

// BenchmarkSingleUpdate measures one durable metadata change in a library of
// n records. With SQLite the cost should stay flat as n grows; the JSON
// backend rewrites the whole file each time (BenchmarkSingleUpdateJSON).
func BenchmarkSingleUpdate(b *testing.B) {
	for _, n := range []int{100, 1000, 3000} {
		b.Run(fmt.Sprintf("sqlite/records=%d", n), func(b *testing.B) {
			s := benchStore(b, n, true)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				toggleAccess(b, s, i)
			}
		})
		b.Run(fmt.Sprintf("json/records=%d", n), func(b *testing.B) {
			s := benchStore(b, n, false)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				toggleAccess(b, s, i)
			}
		})
	}
}

func benchStore(b *testing.B, n int, sqlite bool) *Store {
	b.Helper()
	b.Chdir(b.TempDir())
	if err := os.MkdirAll("data", 0o700); err != nil {
		b.Fatal(err)
	}
	s := &Store{wallpapers: make(map[string]*Wallpaper)}
	if sqlite {
		if err := s.Load(); err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { _ = s.Close() })
	}
	batch := make([]*Wallpaper, 0, n)
	for i := 0; i < n; i++ {
		batch = append(batch, &Wallpaper{ID: fmt.Sprintf("w%05d", i), LinkName: fmt.Sprintf("w%05d", i), Category: "nature", AccessLevel: "public", HasImage: true, MIMEType: "png", CreatedAt: int64(i)})
	}
	if _, _, err := s.CreateBatch(batch); err != nil {
		b.Fatal(err)
	}
	return s
}

func toggleAccess(b *testing.B, s *Store, i int) {
	b.Helper()
	level := "public"
	if i%2 == 0 {
		level = "local"
	}
	if _, err := s.Update("w00000", func(wp *Wallpaper) error { wp.AccessLevel = level; return nil }); err != nil {
		b.Fatal(err)
	}
}
