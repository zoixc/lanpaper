package storage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"lanpaper/config"
)

// useGlobalStore swaps the package store for an empty one and restores the
// previous store, counters and working directory afterwards.
func useGlobalStore(t *testing.T) {
	t.Helper()
	testStorageDir(t)
	savedStore, savedBytes := Global, historyBytesTotal.Load()
	Global = &Store{}
	ResetHistoryCounters()
	t.Cleanup(func() {
		Global = savedStore
		historyBytesTotal.Store(savedBytes)
	})
}

func mustGet(t *testing.T, name string) *Wallpaper {
	t.Helper()
	wp, ok := Global.Get(name)
	if !ok {
		t.Fatalf("link %s disappeared from the store", name)
	}
	return wp
}

func writeTestFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPrependHistoryTrimsAndReplaces(t *testing.T) {
	entries := []HistoryEntry{{Version: 3}, {Version: 2}, {Version: 1}}

	kept, dropped := PrependHistory(entries, HistoryEntry{Version: 4}, 3)
	if len(kept) != 3 || kept[0].Version != 4 || kept[1].Version != 3 || kept[2].Version != 2 {
		t.Fatalf("bad trim: %+v", kept)
	}
	if len(dropped) != 1 || dropped[0].Version != 1 {
		t.Fatalf("bad dropped list: %+v", dropped)
	}
	// Repeating a version replaces the stored entry instead of listing it twice,
	// so a retried upload cannot strand a file.
	kept, dropped = PrependHistory(kept, HistoryEntry{Version: 4, Ext: "png"}, 3)
	if len(kept) != 3 || kept[0].Ext != "png" || len(dropped) != 0 {
		t.Fatalf("repeated version not replaced: %+v %+v", kept, dropped)
	}
	// The input is never written to: a record from Store.Get shares its backing
	// array with the stored record.
	if len(entries) != 3 || entries[0].Version != 3 {
		t.Fatalf("input slice was modified: %+v", entries)
	}

	if kept, found := WithoutHistoryVersion(entries, 2); !found || len(kept) != 2 || kept[0].Version != 3 {
		t.Fatalf("WithoutHistoryVersion: %+v %v", kept, found)
	}
	if _, found := WithoutHistoryVersion(entries, 9); found {
		t.Fatal("WithoutHistoryVersion reported a missing version as found")
	}
	if _, found := FindHistory(entries, 1); !found {
		t.Fatal("FindHistory missed a stored version")
	}
	if _, found := FindHistory(entries, 9); found {
		t.Fatal("FindHistory invented a version")
	}
	if got := HistoryBytes([]HistoryEntry{{SizeBytes: 5}, {SizeBytes: 0}, {SizeBytes: -3}, {SizeBytes: 7}}); got != 12 {
		t.Fatalf("HistoryBytes = %d, want 12", got)
	}
}

func TestArchiveReplacedConsumesTheUploadBackup(t *testing.T) {
	useGlobalStore(t)
	if err := Global.Create(&Wallpaper{
		ID: "w", LinkName: "w", Category: "other", AccessLevel: config.AccessPublic,
		HasImage: true, MIMEType: "png", SizeBytes: 3, ImagePath: MediaPath("w", "png"),
	}); err != nil {
		t.Fatal(err)
	}
	prev := mustGet(t, "w")

	// publishStaged leaves a hardlink of the replaced file behind; archiving it
	// must be a rename, not a copy.
	backup := filepath.Join(config.MediaDir, ".upload-backup.png")
	writeTestFile(t, backup, []byte("old"))
	archived, err := ArchiveReplaced(prev, backup, "png", 2)
	if err != nil {
		t.Fatal(err)
	}
	if archived.CurrentVersion != 2 || len(archived.History) != 1 {
		t.Fatalf("bad archive result: %+v", archived)
	}
	if entry := archived.History[0]; entry.Version != 1 || entry.Ext != "png" || entry.SizeBytes != 3 || entry.SavedAt == 0 {
		t.Fatalf("bad history entry: %+v", entry)
	}
	if got, err := os.ReadFile(HistoryPath("w", 1, "png")); err != nil || !bytes.Equal(got, []byte("old")) {
		t.Fatalf("archived bytes wrong: %q %v", got, err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("the backup was not consumed by the archive")
	}
	if HistoryBytesTotal() != 3 {
		t.Fatalf("archive counter = %d, want 3", HistoryBytesTotal())
	}

	// A record whose version predates versioning is read as version 1.
	legacy := mustGet(t, "w")
	legacy.CurrentVersion = 0
	writeTestFile(t, backup, []byte("new"))
	if _, err := ArchiveReplaced(legacy, backup, "png", 2); err != nil {
		t.Fatal(err)
	}
	// Refused instead of silently ignored.
	if _, err := ArchiveReplaced(prev, "", "png", 2); err == nil {
		t.Fatal("an empty backup was accepted")
	}
	if _, err := ArchiveReplaced(prev, backup, "png", 0); err == nil {
		t.Fatal("disabled history was accepted")
	}
	if _, err := ArchiveReplaced(prev, backup, "exe", 2); err == nil {
		t.Fatal("an invalid extension was accepted")
	}
	if _, err := ArchiveReplaced(nil, backup, "png", 2); err == nil {
		t.Fatal("a nil record was accepted")
	}
	empty := &Wallpaper{ID: "e", LinkName: "e"}
	if _, err := ArchiveReplaced(empty, backup, "png", 2); err == nil {
		t.Fatal("a link without media was archived")
	}
}

func TestRemoveHistoryEntryDeletesTheFile(t *testing.T) {
	useGlobalStore(t)
	if err := Global.Create(&Wallpaper{
		ID: "w", LinkName: "w", Category: "other", HasImage: true, MIMEType: "png",
		History: []HistoryEntry{{Version: 2, Ext: "png", SizeBytes: 4}, {Version: 1, Ext: "jpg", SizeBytes: 6}},
	}); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, HistoryPath("w", 2, "png"), []byte("four"))
	writeTestFile(t, HistoryPath("w", 1, "jpg"), []byte("six666"))
	historyBytesTotal.Store(10)

	updated, err := RemoveHistoryEntry("w", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.History) != 1 || updated.History[0].Version != 1 {
		t.Fatalf("bad record after removal: %+v", updated.History)
	}
	if _, err := os.Stat(HistoryPath("w", 2, "png")); !os.IsNotExist(err) {
		t.Fatalf("the file of a removed version survived: %v", err)
	}
	if HistoryBytesTotal() != 6 {
		t.Fatalf("archive counter = %d, want 6", HistoryBytesTotal())
	}
	if _, err := RemoveHistoryEntry("w", 2); err != ErrNotFound {
		t.Fatalf("removing a missing version: %v", err)
	}
	if _, err := RemoveHistoryEntry("../etc", 1); err == nil {
		t.Fatal("an invalid link name was accepted")
	}
	// The directory survives while another version is still archived.
	if _, err := os.Stat(HistoryDirPath("w")); err != nil {
		t.Fatalf("history directory disappeared too early: %v", err)
	}
	if _, err := RemoveHistoryEntry("w", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(HistoryDirPath("w")); !os.IsNotExist(err) {
		t.Fatalf("the empty history directory survived: %v", err)
	}
	if HistoryBytesTotal() != 0 {
		t.Fatalf("archive counter = %d, want 0", HistoryBytesTotal())
	}
}

func TestTrimHistoryBudgetDropsOldestFirst(t *testing.T) {
	useGlobalStore(t)
	const versionMB = 2 // MiB per archived version
	const version = int64(versionMB) << 20

	for _, link := range []string{"a", "b"} {
		if err := Global.Create(&Wallpaper{
			ID: link, LinkName: link, Category: "other", HasImage: true, MIMEType: "png",
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Link b archived earlier than link a, so b is trimmed first.
	addVersions := func(link string, savedAt int64) {
		t.Helper()
		if _, err := Global.Update(link, func(wp *Wallpaper) error {
			wp.History = []HistoryEntry{
				{Version: 1, Ext: "png", SizeBytes: version, SavedAt: savedAt + 1},
				{Version: 2, Ext: "png", SizeBytes: version, SavedAt: savedAt + 2},
			}
			NoteHistoryBytes(2 * version)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	addVersions("a", 100)
	addVersions("b", 50)
	if HistoryBytesTotal() != 4*version {
		t.Fatalf("archive counter = %d", HistoryBytesTotal())
	}

	TrimHistoryBudget(0)             // disabled
	TrimHistoryBudget(-1)            // disabled
	TrimHistoryBudget(4 * versionMB) // exactly at the budget: nothing is trimmed
	if len(mustGet(t, "a").History) != 2 || len(mustGet(t, "b").History) != 2 {
		t.Fatal("a budget that is not exceeded trimmed anyway")
	}

	// One version over the budget (8 MiB archived, 6 MiB allowed): the oldest
	// archive of the link that archived first goes, and nothing else.
	TrimHistoryBudget(3 * versionMB)
	if history := mustGet(t, "b").History; len(history) != 1 || history[0].Version != 2 {
		t.Fatalf("the oldest version was not dropped first: %+v", history)
	}
	if len(mustGet(t, "a").History) != 2 {
		t.Fatalf("newer versions were dropped: %+v", mustGet(t, "a").History)
	}
	if HistoryBytesTotal() != 3*version {
		t.Fatalf("archive counter = %d after the first trim", HistoryBytesTotal())
	}

	// The budget is shared by every link, so trimming continues across them:
	// 6 MiB -> b.v2 (4 MiB) -> a.v1 (2 MiB), which is what a 2 MiB budget allows.
	TrimHistoryBudget(versionMB)
	if history := mustGet(t, "b").History; len(history) != 0 {
		t.Fatalf("link b kept versions: %+v", history)
	}
	if history := mustGet(t, "a").History; len(history) != 1 || history[0].Version != 2 {
		t.Fatalf("link a kept the wrong versions: %+v", history)
	}
	if HistoryBytesTotal() != version {
		t.Fatalf("archive counter = %d after the second trim", HistoryBytesTotal())
	}

	// A missed delta self-heals: the pass recomputes the total before deciding,
	// so an inflated counter is corrected instead of trimming good versions.
	NoteHistoryBytes(1 << 30)
	TrimHistoryBudget(64)
	if HistoryBytesTotal() != version {
		t.Fatalf("the counter did not self-heal: %d", HistoryBytesTotal())
	}
}

func TestMoveLinkExtraDirsIsReversible(t *testing.T) {
	useGlobalStore(t)
	writeTestFile(t, HistoryPath("old", 1, "png"), []byte("one"))
	writeTestFile(t, ItemPath("old", 2, "jpg"), []byte("two"))

	moved, err := MoveLinkExtraDirs("old", "new")
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 2 {
		t.Fatalf("moved %#v", moved)
	}
	if got, err := os.ReadFile(HistoryPath("new", 1, "png")); err != nil || string(got) != "one" {
		t.Fatalf("archived version did not move: %q %v", got, err)
	}
	if got, err := os.ReadFile(ItemPath("new", 2, "jpg")); err != nil || string(got) != "two" {
		t.Fatalf("playlist item did not move: %q %v", got, err)
	}
	if _, err := os.Stat(HistoryDirPath("old")); !os.IsNotExist(err) {
		t.Fatalf("the old directory survived: %v", err)
	}

	UndoMovedDirs(moved)
	if got, err := os.ReadFile(HistoryPath("old", 1, "png")); err != nil || string(got) != "one" {
		t.Fatalf("rollback did not restore the archive: %q %v", got, err)
	}

	// A link with neither directory is not an error.
	if moved, err := MoveLinkExtraDirs("plain", "other"); err != nil || len(moved) != 0 {
		t.Fatalf("moving nothing: %#v %v", moved, err)
	}
	// An occupied destination is refused instead of merged, and nothing moves.
	if err := os.MkdirAll(HistoryDirPath("taken"), 0o755); err != nil {
		t.Fatal(err)
	}
	moved, err = MoveLinkExtraDirs("old", "taken")
	if err == nil {
		t.Fatal("moving onto an existing directory succeeded")
	}
	if len(moved) != 0 {
		t.Fatalf("a refused move reported moved directories: %#v", moved)
	}
	if _, err := os.Stat(HistoryDirPath("old")); err != nil {
		t.Fatalf("a refused move still removed the source: %v", err)
	}
	// Removing the extra directories of a link never touches the media itself.
	writeTestFile(t, MediaPath("old", "png"), []byte("live"))
	RemoveLinkExtraDirs("old")
	if _, err := os.Stat(HistoryDirPath("old")); !os.IsNotExist(err) {
		t.Fatalf("history survived RemoveLinkExtraDirs: %v", err)
	}
	if got, err := os.ReadFile(MediaPath("old", "png")); err != nil || string(got) != "live" {
		t.Fatalf("RemoveLinkExtraDirs touched the media: %q %v", got, err)
	}
}

func TestLoadSanitizesHistoryAndPlaylist(t *testing.T) {
	useGlobalStore(t)
	body := `{
	  "good": {"id":"good","linkName":"good","category":"other","hasImage":true,"mimeType":"png","currentVersion":4,
	           "history":[{"version":3,"ext":"png","sizeBytes":10},{"version":3,"ext":"jpg","sizeBytes":99},
	                      {"version":2,"ext":"exe","sizeBytes":10},{"version":0,"ext":"png","sizeBytes":1}],
	           "items":[{"id":1,"ext":"png","sizeBytes":5},{"id":1,"ext":"jpg","sizeBytes":5},
	                     {"id":0,"ext":"png"},{"id":7,"ext":"../evil"},{"id":2,"ext":"jpg","sizeBytes":-5}],
	           "rotate":{"enabled":true,"interval":1,"order":"shuffle"}},
	  "clash": {"id":"clash","linkName":"clash","category":"other","hasImage":true,"mimeType":"png","currentVersion":3,
	            "history":[{"version":3,"ext":"png","sizeBytes":1},{"version":5,"ext":"png","sizeBytes":1}]},
	  "plain": {"id":"plain","linkName":"plain","category":"other","hasImage":false}
	}`
	writeTestFile(t, dataFile, []byte(body))
	if err := Global.Load(); err != nil {
		t.Fatal(err)
	}

	good := mustGet(t, "good")
	if len(good.History) != 1 || good.History[0].Version != 3 || good.History[0].Ext != "png" {
		t.Fatalf("history was not sanitized: %+v", good.History)
	}
	if len(good.Items) != 2 || good.Items[0].ID != 1 || good.Items[1].ID != 2 || good.Items[1].SizeBytes != 0 {
		t.Fatalf("playlist was not sanitized: %+v", good.Items)
	}
	if good.Rotate == nil || good.Rotate.Interval != config.MinRotateInterval ||
		good.Rotate.Order != config.RotateOrderSequential || !good.Rotate.Enabled {
		t.Fatalf("rotation was not normalized: %+v", good.Rotate)
	}
	if HistoryBytesTotal() != 12 {
		t.Fatalf("archive counter after load = %d, want 12", HistoryBytesTotal())
	}

	// A live version that is also archived is repaired, so a future archive can
	// never overwrite a file that is still listed.
	clash := mustGet(t, "clash")
	if clash.CurrentVersion != 6 {
		t.Fatalf("clashing version was not repaired: %d", clash.CurrentVersion)
	}

	// A link that never used the features keeps its zero values, so enabling them
	// does not rewrite wallpapers.json for every existing link.
	plain := mustGet(t, "plain")
	if plain.Rotate != nil || len(plain.History) != 0 || len(plain.Items) != 0 || plain.CurrentVersion != 0 {
		t.Fatalf("a plain link changed shape: %+v", plain)
	}
	// A link that never used the features serializes exactly as before, so an
	// upgrade does not rewrite the shape of an existing wallpapers.json.
	encoded, err := json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"history", "items", "rotate", "currentVersion"} {
		if bytes.Contains(encoded, []byte(field)) {
			t.Fatalf("an unused feature changed the stored shape: %s", encoded)
		}
	}
}
