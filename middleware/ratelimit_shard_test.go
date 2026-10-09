// SPDX-License-Identifier: MIT

package middleware

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// Sharding must not change the budget: each key gets exactly its limit, even
// when many goroutines hit many keys at once.
func TestShardedLimitIsExactUnderConcurrency(t *testing.T) {
	resetRateCounts()
	t.Cleanup(resetRateCounts)
	const keys, limit, callers = 200, 5, 8
	allowed := make([]int, keys)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for c := 0; c < callers; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < keys; k++ {
				if ok, _ := allowEvent("test", fmt.Sprintf("client-%d", k), limit, time.Minute); ok {
					mu.Lock()
					allowed[k]++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	for k, n := range allowed {
		if n != limit {
			t.Fatalf("client-%d allowed %d events, want %d", k, n, limit)
		}
	}
}

// The cleaner must reach every shard, not just the first one.
func TestCleanExpiredCountsCoversAllShards(t *testing.T) {
	resetRateCounts()
	t.Cleanup(resetRateCounts)
	for k := 0; k < 500; k++ {
		recordEvent("gc", fmt.Sprintf("ip-%d", k), time.Minute)
	}
	populated := 0
	for i := range rateTable {
		if len(rateTable[i].counts) > 0 {
			populated++
		}
	}
	if populated < 2 {
		t.Fatalf("keys landed in %d shard(s); sharding is not spreading them", populated)
	}
	cleanExpiredCounts(time.Now().Add(2 * time.Minute))
	for i := range rateTable {
		if n := len(rateTable[i].counts); n != 0 {
			t.Fatalf("shard %d still holds %d expired counters", i, n)
		}
	}
}
