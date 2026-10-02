package storage

import (
	"path/filepath"
	"testing"

	"lanpaper/config"
)

func TestPlaylistItemHelpers(t *testing.T) {
	if got := NextItemID(nil); got != 1 {
		t.Fatalf("NextItemID(nil) = %d, want 1", got)
	}
	if got := NextItemID([]PlaylistItem{{ID: 4}, {ID: 2}}); got != 5 {
		t.Fatalf("NextItemID = %d, want 5", got)
	}

	items := AppendItem(nil, PlaylistItem{ID: 1, Ext: "png", SizeBytes: 10})
	items = AppendItem(items, PlaylistItem{ID: 2, Ext: "jpg", SizeBytes: 20})
	if len(items) != 2 || items[1].ID != 2 {
		t.Fatalf("AppendItem: %+v", items)
	}
	if item, ok := ItemByID(items, 2); !ok || item.Ext != "jpg" || item.SizeBytes != 20 {
		t.Fatalf("ItemByID: %+v %v", item, ok)
	}
	if _, ok := ItemByID(items, 9); ok {
		t.Fatal("ItemByID found a missing item")
	}

	kept, removed, ok := WithoutItem(items, 1)
	if !ok || removed.ID != 1 || removed.Ext != "png" || len(kept) != 1 || kept[0].ID != 2 {
		t.Fatalf("WithoutItem: %+v %+v %v", kept, removed, ok)
	}
	if len(items) != 2 {
		t.Fatalf("WithoutItem modified its input: %+v", items)
	}
	if _, _, ok := WithoutItem(items, 9); ok {
		t.Fatal("WithoutItem reported a missing item as removed")
	}
	if _, _, ok := WithoutItem(items, 0); ok {
		t.Fatal("item id 0 is the live file and must not be removable")
	}

	if got := ItemPath("frame", 3, "png"); got != filepath.Join(config.ItemsDir, "frame", "3.png") {
		t.Fatalf("ItemPath = %s", got)
	}
	if got := ItemsDirPath("frame"); got != filepath.Join(config.ItemsDir, "frame") {
		t.Fatalf("ItemsDirPath = %s", got)
	}
}

func TestNormalizeRotateClampsAndKeepsZero(t *testing.T) {
	if rc := NormalizeRotate(RotateConfig{}, false); rc != (RotateConfig{}) {
		t.Fatalf("a record without a playlist gained settings: %+v", rc)
	}
	// A record that only got its first item needs usable defaults.
	if rc := NormalizeRotate(RotateConfig{}, true); rc.Interval != config.DefaultRotateInterval ||
		rc.Order != config.RotateOrderSequential || rc.Enabled {
		t.Fatalf("first item defaults: %+v", rc)
	}
	if rc := NormalizeRotate(RotateConfig{Enabled: true, Interval: 1}, true); rc.Interval != config.MinRotateInterval || !rc.Enabled {
		t.Fatalf("interval below the minimum: %+v", rc)
	}
	if rc := NormalizeRotate(RotateConfig{Enabled: true, Interval: -30}, true); rc.Interval != config.DefaultRotateInterval {
		t.Fatalf("negative interval: %+v", rc)
	}
	if rc := NormalizeRotate(RotateConfig{Enabled: true, Interval: config.MaxRotateInterval * 2}, true); rc.Interval != config.MaxRotateInterval {
		t.Fatalf("interval above the maximum: %+v", rc)
	}
	if rc := NormalizeRotate(RotateConfig{Enabled: true, Order: " Random "}, true); rc.Order != config.RotateOrderRandom {
		t.Fatalf("order was not normalized: %+v", rc)
	}
	if rc := NormalizeRotate(RotateConfig{Enabled: true, Order: "shuffle"}, true); rc.Order != config.RotateOrderSequential {
		t.Fatalf("an unknown order was kept: %+v", rc)
	}
}

func TestPlaylistIndexRotatesWithoutState(t *testing.T) {
	wp := &Wallpaper{LinkName: "frame", Items: []PlaylistItem{{ID: 1}, {ID: 2}}}
	positions := 3 // live file + two items

	// Without rotation the live file is always served, even with items present.
	for _, now := range []int64{0, 1, 60, 1 << 40} {
		if got := wp.PlaylistIndex(now); got != 0 {
			t.Fatalf("PlaylistIndex(%d) = %d without rotation", now, got)
		}
	}

	wp.Rotate = RotateConfig{Enabled: true, Interval: 60, Order: config.RotateOrderSequential}
	if got := wp.PlaylistIndex(0); got != 0 {
		t.Fatalf("first window: %d", got)
	}
	// One window, one image: two hits inside the same minute agree.
	if wp.PlaylistIndex(0) != wp.PlaylistIndex(59) {
		t.Fatal("the selection changed inside one window")
	}
	for window, want := range []int{0, 1, 2, 0, 1, 2} {
		if got := wp.PlaylistIndex(int64(window) * 60); got != want {
			t.Fatalf("window %d: got %d, want %d", window, got, want)
		}
	}
	seen := map[int]bool{}
	for now := range int64(60 * 60) {
		got := wp.PlaylistIndex(now)
		if got < 0 || got >= positions {
			t.Fatalf("PlaylistIndex(%d) = %d, out of range", now, got)
		}
		seen[got] = true
	}
	if len(seen) != positions {
		t.Fatalf("sequential rotation did not reach every position: %v", seen)
	}

	// An interval below the minimum falls back to the default instead of
	// switching on every request.
	wp.Rotate.Interval = 1
	if wp.PlaylistIndex(0) != wp.PlaylistIndex(config.DefaultRotateInterval-1) {
		t.Fatal("an invalid interval rotated on every second")
	}

	wp.Rotate = RotateConfig{Enabled: true, Interval: 60, Order: config.RotateOrderRandom}
	other := &Wallpaper{LinkName: "other", Items: wp.Items, Rotate: wp.Rotate}
	random, differs := map[int]bool{}, false
	for window := range 200 {
		now := int64(window) * 60
		got := wp.PlaylistIndex(now)
		if got < 0 || got >= positions {
			t.Fatalf("random PlaylistIndex = %d, out of range", got)
		}
		if wp.PlaylistIndex(now) != got {
			t.Fatal("random selection is not stable inside a window")
		}
		random[got] = true
		if other.PlaylistIndex(now) != got {
			differs = true
		}
	}
	if len(random) < 2 {
		t.Fatalf("random order never varied: %v", random)
	}
	if !differs {
		t.Fatal("random order is identical for every link")
	}

	// Rotation is off while there is nothing to rotate to.
	wp.Items = nil
	for _, now := range []int64{0, 60, 120} {
		if got := wp.PlaylistIndex(now); got != 0 {
			t.Fatalf("PlaylistIndex(%d) = %d with an empty playlist", now, got)
		}
	}
	// A clock before the epoch cannot produce a negative index.
	wp.Items = []PlaylistItem{{ID: 1}}
	if got := wp.PlaylistIndex(-1 << 40); got < 0 || got > len(wp.Items) {
		t.Fatalf("negative clock: %d", got)
	}
}

func TestPlaylistNowMatchesPlaylistIndex(t *testing.T) {
	wp := &Wallpaper{LinkName: "frame", Items: []PlaylistItem{{ID: 1}, {ID: 2}},
		Rotate: NormalizeRotate(RotateConfig{Enabled: true, Interval: config.MinRotateInterval}, true)}
	// A handler resolves a whole request against one window, so PlaylistNow must
	// always return a position this link can actually serve.
	for range 50 {
		if now := wp.PlaylistNow(); now < 0 || now > len(wp.Items) {
			t.Fatalf("PlaylistNow = %d", now)
		}
	}
}
