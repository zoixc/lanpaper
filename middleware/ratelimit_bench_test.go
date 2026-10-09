package middleware

import (
	"fmt"
	"testing"
	"time"
)

func BenchmarkAllowEventParallel(b *testing.B) {
	keys := make([]string, 4096)
	for i := range keys {
		keys[i] = fmt.Sprintf("10.0.%d.%d", i/256, i%256)
	}
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			allowEvent("public", keys[i%len(keys)], 1<<30, time.Minute)
			i++
		}
	})
}
