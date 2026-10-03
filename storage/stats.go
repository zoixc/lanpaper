// SPDX-License-Identifier: MIT

package storage

import (
	"sync"
	"sync/atomic"
	"time"
)

// AccessStats counts public media deliveries for one link since the process
// started.
//
// The counters are in memory only, which is the whole point: answering "is this
// link used at all, and how much traffic does it cause?" must not add a single
// write to the request path. A restart resets them, and nothing identifying a
// visitor (query string, address, user agent) is kept, so enabling statistics
// cannot turn a LAN wallpaper server into a tracker.
type AccessStats struct {
	Hits    int64 `json:"hits"`
	Bytes   int64 `json:"bytes"`
	LastHit int64 `json:"lastHit,omitempty"`
}

type statCounters struct {
	hits    atomic.Int64
	bytes   atomic.Int64
	lastHit atomic.Int64
}

// maxTrackedLinks bounds the counter map. A link nobody requests costs nothing,
// and once the bound is reached further links are simply not counted instead of
// growing memory without limit.
const maxTrackedLinks = 10_000

var (
	linkStats    sync.Map // link name -> *statCounters
	trackedLinks atomic.Int64
)

// RecordHit counts one delivered public media response. Handlers call it after
// authorization and only for a body that was actually sent.
func RecordHit(linkName string, sent int64) {
	if linkName == "" {
		return
	}
	value, loaded := linkStats.Load(linkName)
	if !loaded {
		if trackedLinks.Load() >= maxTrackedLinks {
			return
		}
		stored, isNew := linkStats.LoadOrStore(linkName, &statCounters{})
		value = stored
		if isNew {
			trackedLinks.Add(1)
		}
	}
	counters, ok := value.(*statCounters)
	if !ok {
		return
	}
	counters.hits.Add(1)
	if sent > 0 {
		counters.bytes.Add(sent)
	}
	counters.lastHit.Store(time.Now().Unix())
}

// StatsFor returns the counters recorded for a link. It reports false when the
// link was never requested, so responses can omit the field entirely.
func StatsFor(linkName string) (AccessStats, bool) {
	value, ok := linkStats.Load(linkName)
	if !ok {
		return AccessStats{}, false
	}
	counters, ok := value.(*statCounters)
	if !ok {
		return AccessStats{}, false
	}
	hits := counters.hits.Load()
	if hits <= 0 {
		return AccessStats{}, false
	}
	return AccessStats{
		Hits:    hits,
		Bytes:   counters.bytes.Load(),
		LastHit: counters.lastHit.Load(),
	}, true
}

// ForgetStats drops the counters of a deleted or pruned link so the map cannot
// accumulate names that no longer exist.
func ForgetStats(linkName string) {
	if _, loaded := linkStats.LoadAndDelete(linkName); loaded {
		trackedLinks.Add(-1)
	}
}

// ResetStats clears every counter. Used by tests.
func ResetStats() {
	linkStats.Range(func(key, _ any) bool {
		linkStats.Delete(key)
		return true
	})
	trackedLinks.Store(0)
}
