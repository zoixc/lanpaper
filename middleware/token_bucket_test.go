// SPDX-License-Identifier: MIT

package middleware

import (
	"testing"
	"time"
)

func TestTokenBucketRefillsContinuouslyAndBoundsBurst(t *testing.T) {
	resetRateCounts()
	t.Cleanup(resetRateCounts)
	const key = "client"
	for i := 0; i < 3; i++ {
		if ok, _ := allowBucketEvent("refill", key, 3, 2, time.Minute); !ok {
			t.Fatalf("initial burst rejected at %d", i)
		}
	}
	if ok, retry := allowBucketEvent("refill", key, 3, 2, time.Minute); ok || retry <= 0 {
		t.Fatalf("empty bucket allowed=%v retry=%v", ok, retry)
	}
	id := "refill:" + key
	sh := shardFor(id)
	sh.mu.Lock()
	sh.counts[id].last = sh.counts[id].last.Add(-30 * time.Second)
	sh.mu.Unlock()
	if ok, _ := allowBucketEvent("refill", key, 3, 2, time.Minute); !ok {
		t.Fatal("one token did not refill after half a minute")
	}
	if ok, _ := allowBucketEvent("refill", key, 3, 2, time.Minute); ok {
		t.Fatal("refill exceeded sustained rate")
	}
}
