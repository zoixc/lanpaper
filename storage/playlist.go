package storage

import (
	"encoding/binary"
	"hash/fnv"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lanpaper/config"
)

// PlaylistItem is one extra media file served from the same URL as the live
// file. Items never get their own preview: the admin panel shows one thumbnail
// per link, and generating a WebP for every playlist entry would multiply the
// CPU and disk cost of an upload for metadata nobody looks at.
type PlaylistItem struct {
	ID        int    `json:"id"`
	Ext       string `json:"ext"`
	SizeBytes int64  `json:"sizeBytes"`
	ModTime   int64  `json:"modTime"`
	AddedAt   int64  `json:"addedAt"`
}

// RotateConfig controls automatic switching between the live file and Items.
type RotateConfig struct {
	Enabled  bool   `json:"enabled,omitempty"`
	Interval int    `json:"interval,omitempty"` // seconds between switches
	Order    string `json:"order,omitempty"`    // sequential (default) | random
}

// ItemsDirPath returns the directory holding a link's playlist items.
func ItemsDirPath(linkName string) string {
	return filepath.Join(config.ItemsDir, linkName)
}

// ItemPath returns data/items/{link}/{id}.{ext}. linkName must be a validated
// link name and ext a validated stored extension, exactly like MediaPath.
func ItemPath(linkName string, id int, ext string) string {
	return filepath.Join(config.ItemsDir, linkName, strconv.Itoa(id)+"."+ext)
}

// NextItemID returns an unused item id. Ids are never reused while an item
// exists, so a cached ?i= URL cannot silently switch to different bytes.
func NextItemID(items []PlaylistItem) int {
	next := 1
	for _, it := range items {
		if it.ID >= next {
			next = it.ID + 1
		}
	}
	return next
}

// ItemByID finds a playlist item by its id.
func ItemByID(items []PlaylistItem, id int) (PlaylistItem, bool) {
	for _, it := range items {
		if it.ID == id {
			return it, true
		}
	}
	return PlaylistItem{}, false
}

// AppendItem returns a fresh slice with item added. Records handed out by
// Store.Get share their backing array with the stored record, so an in-place
// append could publish a partially written item to a concurrent reader.
func AppendItem(current []PlaylistItem, item PlaylistItem) []PlaylistItem {
	out := make([]PlaylistItem, 0, len(current)+1)
	out = append(out, current...)
	return append(out, item)
}

// WithoutItem removes an item by id, returning a fresh slice, the removed item
// and whether it existed. The caller deletes the file after the metadata commit.
func WithoutItem(current []PlaylistItem, id int) ([]PlaylistItem, PlaylistItem, bool) {
	removed, found := ItemByID(current, id)
	if !found {
		return current, PlaylistItem{}, false
	}
	out := make([]PlaylistItem, 0, len(current)-1)
	for _, it := range current {
		if it.ID != id {
			out = append(out, it)
		}
	}
	return out, removed, true
}

// NormalizeRotate clamps rotation settings to the supported ranges. A record
// that never used rotation keeps its zero value, so enabling the feature does
// not grow wallpapers.json for every existing link.
func NormalizeRotate(rc RotateConfig, hasItems bool) RotateConfig {
	if !rc.Enabled && rc.Interval == 0 && rc.Order == "" && !hasItems {
		return RotateConfig{}
	}
	switch strings.ToLower(strings.TrimSpace(rc.Order)) {
	case config.RotateOrderRandom:
		rc.Order = config.RotateOrderRandom
	default:
		rc.Order = config.RotateOrderSequential
	}
	if rc.Interval <= 0 {
		rc.Interval = config.DefaultRotateInterval
	} else if rc.Interval < config.MinRotateInterval {
		rc.Interval = config.MinRotateInterval
	}
	if rc.Interval > config.MaxRotateInterval {
		rc.Interval = config.MaxRotateInterval
	}
	return rc
}

// PlaylistIndex picks which media a public request should serve: 0 is the live
// file, 1..n are Items in stored order. The choice is derived from the clock
// (sequential) or from a stable hash of the link and the time window (random),
// so rotation needs no goroutine, no state and no extra request-time I/O, and
// two replicas reading the same data directory always agree.
func (wp *Wallpaper) PlaylistIndex(now int64) int {
	count := len(wp.Items) + 1
	if count < 2 || !wp.Rotate.Enabled {
		return 0
	}
	interval := wp.Rotate.Interval
	if interval < config.MinRotateInterval {
		interval = config.DefaultRotateInterval
	}
	window := now / int64(interval)
	if window < 0 {
		window = 0
	}
	if wp.Rotate.Order == config.RotateOrderRandom {
		// FNV over the link name and the window: cheap, allocation-free and
		// stable for the whole window, so the image does not flicker per hit.
		h := fnv.New32a()
		_, _ = h.Write([]byte(wp.LinkName))
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], uint64(window))
		_, _ = h.Write(buf[:])
		return int(h.Sum32() % uint32(count))
	}
	return int(window % int64(count))
}

// PlaylistNow is PlaylistIndex for the current time; handlers use it so the
// whole request agrees on one window.
func (wp *Wallpaper) PlaylistNow() int {
	return wp.PlaylistIndex(time.Now().Unix())
}
