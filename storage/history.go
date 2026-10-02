package storage

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"lanpaper/config"
	"lanpaper/utils"
)

// HistoryEntry is one replaced version of a link's media file. Versions are
// numbered per link and never reused, so ?v=N keeps addressing the same bytes.
type HistoryEntry struct {
	Version   uint64 `json:"version"`
	Ext       string `json:"ext"` // stored extension, e.g. "jpg"
	SizeBytes int64  `json:"sizeBytes"`
	ModTime   int64  `json:"modTime"` // mtime while the file was live
	SavedAt   int64  `json:"savedAt"` // when it was moved into history
}

// HistoryDirPath returns the directory holding a link's archived versions.
func HistoryDirPath(linkName string) string {
	return filepath.Join(config.HistoryDir, linkName)
}

// HistoryPath returns data/history/{link}/{version}.{ext}. linkName must be a
// validated link name and ext a validated stored extension, exactly like
// MediaPath.
func HistoryPath(linkName string, version uint64, ext string) string {
	return filepath.Join(config.HistoryDir, linkName, strconv.FormatUint(version, 10)+"."+ext)
}

// historyBytesTotal tracks the recorded size of every archived version so the
// global budget can be checked with one atomic load instead of scanning the
// store on every upload. It is maintained by the helpers below and recomputed
// by TrimHistoryBudget and Load, so a missed delta self-heals.
var historyBytesTotal atomic.Int64

// HistoryBytes sums the recorded size of a history list.
func HistoryBytes(entries []HistoryEntry) int64 {
	var total int64
	for _, e := range entries {
		if e.SizeBytes > 0 {
			total += e.SizeBytes
		}
	}
	return total
}

// NoteHistoryBytes records the size delta of a history change. Anything that
// assigns Wallpaper.History inside Store.Update must call it with the
// difference, otherwise the budget check drifts.
func NoteHistoryBytes(delta int64) {
	if delta != 0 {
		historyBytesTotal.Add(delta)
	}
}

// HistoryBytesTotal returns the tracked archive size in bytes.
func HistoryBytesTotal() int64 { return historyBytesTotal.Load() }

// ResetHistoryCounters clears the tracked archive size. Used by tests, which
// start every case from an empty store in a fresh temporary directory.
func ResetHistoryCounters() { historyBytesTotal.Store(0) }

// FindHistory returns the entry for a version number.
func FindHistory(entries []HistoryEntry, version uint64) (HistoryEntry, bool) {
	for _, e := range entries {
		if e.Version == version {
			return e, true
		}
	}
	return HistoryEntry{}, false
}

// PrependHistory returns the list with entry first (newest first) trimmed to
// limit entries, plus the trimmed-off entries whose files the caller must
// delete. An entry with the same version number replaces the stored one, so a
// retry can never leave two files for one version. limit is at least 1; a
// disabled history is expressed by not archiving at all.
func PrependHistory(current []HistoryEntry, entry HistoryEntry, limit int) (kept, dropped []HistoryEntry) {
	if limit < 1 {
		limit = 1
	}
	all := make([]HistoryEntry, 0, len(current)+1)
	all = append(all, entry)
	for _, e := range current {
		if e.Version != entry.Version {
			all = append(all, e)
		}
	}
	if len(all) <= limit {
		return all, nil
	}
	return all[:limit], all[limit:]
}

// WithoutHistoryVersion removes one version, returning a fresh slice and
// whether it was present.
func WithoutHistoryVersion(current []HistoryEntry, version uint64) ([]HistoryEntry, bool) {
	kept := make([]HistoryEntry, 0, len(current))
	found := false
	for _, e := range current {
		if e.Version == version {
			found = true
			continue
		}
		kept = append(kept, e)
	}
	if !found {
		return current, false
	}
	return kept, true
}

// ArchiveReplaced moves the backup an upload kept for the file it replaced into
// the link's history directory and records the entry. backupPath is the
// hardlink publishStaged created, so archiving a version costs one rename: no
// extra read, no extra write and no extra disk block while the file is linked.
//
// prev is the record as it was before the upload committed. The entry is
// recorded only if the rename succeeded; on failure the backup is left where it
// is so the caller's normal cleanup still applies.
func ArchiveReplaced(prev *Wallpaper, backupPath, ext string, limit int) (*Wallpaper, error) {
	if prev == nil || !utils.IsValidLinkName(prev.LinkName) {
		return nil, errors.New("invalid link for history archive")
	}
	if limit < 1 {
		return nil, errors.New("version history is disabled")
	}
	if backupPath == "" {
		return nil, errors.New("no replaced file to archive")
	}
	if !prev.HasImage {
		return nil, errors.New("link had no media to archive")
	}
	if !validStoredMediaExt(ext) {
		return nil, fmt.Errorf("invalid stored extension %q", ext)
	}

	version := prev.CurrentVersion
	if version == 0 {
		// Metadata written before versioning existed: its live file is version 1.
		version = 1
	}
	if err := os.MkdirAll(HistoryDirPath(prev.LinkName), config.DataDirPerm); err != nil {
		return nil, err
	}
	dest := HistoryPath(prev.LinkName, version, ext)
	// A retried upload must not fail on what an earlier attempt left behind.
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.Rename(backupPath, dest); err != nil {
		return nil, err
	}

	entry := HistoryEntry{
		Version:   version,
		Ext:       ext,
		SizeBytes: prev.SizeBytes,
		ModTime:   prev.ModTime,
		SavedAt:   time.Now().Unix(),
	}
	if fi, err := os.Lstat(dest); err == nil {
		entry.SizeBytes = fi.Size()
		entry.ModTime = fi.ModTime().Unix()
	}

	var dropped []HistoryEntry
	updated, err := Global.Update(prev.LinkName, func(wp *Wallpaper) error {
		before := HistoryBytes(wp.History)
		kept, over := PrependHistory(wp.History, entry, limit)
		wp.History = kept
		wp.CurrentVersion = version + 1
		dropped = over
		NoteHistoryBytes(HistoryBytes(kept) - before)
		return nil
	})
	if err != nil {
		// Archived but unreferenced: remove it instead of leaving a file no
		// metadata will ever point at again.
		if removeErr := os.Remove(dest); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Printf("History: could not remove unreferenced %s: %v", dest, removeErr)
		}
		return nil, err
	}
	DeleteHistoryFiles(prev.LinkName, dropped)
	return updated, nil
}

// RemoveHistoryEntry drops one archived version from a link's metadata and
// deletes its file. The caller must hold the link lock.
func RemoveHistoryEntry(linkName string, version uint64) (*Wallpaper, error) {
	if !utils.IsValidLinkName(linkName) {
		return nil, errors.New("invalid link name")
	}
	wp, exists := Global.Get(linkName)
	if !exists {
		return nil, ErrNotFound
	}
	entry, found := FindHistory(wp.History, version)
	if !found {
		return nil, ErrNotFound
	}
	updated, err := Global.Update(linkName, func(wp *Wallpaper) error {
		kept, ok := WithoutHistoryVersion(wp.History, version)
		if !ok {
			return ErrNotFound
		}
		NoteHistoryBytes(HistoryBytes(kept) - HistoryBytes(wp.History))
		wp.History = kept
		return nil
	})
	if err != nil {
		return nil, err
	}
	DeleteHistoryFiles(linkName, []HistoryEntry{entry})
	return updated, nil
}

// DeleteHistoryFiles removes archived files and the link's history directory
// once it is empty. A missing file is normal — a rollback may already have
// restored it — and is not logged.
func DeleteHistoryFiles(linkName string, entries []HistoryEntry) {
	if !utils.IsValidLinkName(linkName) || len(entries) == 0 {
		return
	}
	for _, e := range entries {
		if !validStoredMediaExt(e.Ext) {
			continue
		}
		path := HistoryPath(linkName, e.Version, e.Ext)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("History: could not remove %s: %v", path, err)
		}
	}
	// Succeeds only when no other version remains in the directory.
	_ = os.Remove(HistoryDirPath(linkName))
}

// RemoveLinkExtraDirs deletes everything a link keeps outside media/previews:
// archived versions and playlist items. Used when a link is deleted or reset by
// pruning; a missing directory is normal.
func RemoveLinkExtraDirs(linkName string) {
	if !utils.IsValidLinkName(linkName) {
		return
	}
	for _, dir := range []string{HistoryDirPath(linkName), ItemsDirPath(linkName)} {
		if err := os.RemoveAll(dir); err != nil {
			log.Printf("Could not remove %s: %v", dir, err)
		}
	}
}

// MoveLinkExtraDirs renames a link's history and playlist directories as part
// of a link rename. Missing directories are normal and not an error. It returns
// the pairs it moved so the caller can roll them back.
func MoveLinkExtraDirs(oldName, newName string) ([][2]string, error) {
	var moved [][2]string
	for _, base := range []string{config.HistoryDir, config.ItemsDir} {
		from := filepath.Join(base, oldName)
		to := filepath.Join(base, newName)
		fi, statErr := os.Lstat(from)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return moved, statErr
		}
		if !fi.Mode().IsDir() {
			return moved, fmt.Errorf("%s is not a directory", from)
		}
		if _, statErr := os.Lstat(to); !os.IsNotExist(statErr) {
			return moved, fmt.Errorf("destination directory %s already exists", to)
		}
		if mkErr := os.MkdirAll(base, config.DataDirPerm); mkErr != nil {
			return moved, mkErr
		}
		if renErr := os.Rename(from, to); renErr != nil {
			return moved, renErr
		}
		moved = append(moved, [2]string{from, to})
	}
	return moved, nil
}

// UndoMovedDirs reverses MoveLinkExtraDirs after a failed rename.
func UndoMovedDirs(moved [][2]string) {
	for _, pair := range moved {
		if err := os.Rename(pair[1], pair[0]); err != nil {
			log.Printf("Error rolling back directory rename %s: %v", pair[0], err)
		}
	}
}

// TrimHistoryBudget removes the oldest archived versions across all links until
// the recorded total fits into maxMB (0 disables the budget). Like
// PruneOldImages it rechecks each record under the per-link lock, so a
// concurrent upload is never overwritten by a stale pass.
func TrimHistoryBudget(maxMB int) {
	if maxMB <= 0 {
		return
	}
	budget := int64(maxMB) << 20
	if historyBytesTotal.Load() <= budget {
		return
	}

	snap := Global.GetAll()
	// Recompute from the snapshot first: the check above may have been based on
	// a stale counter, and this pass is the self-healing point.
	var total int64
	for _, wp := range snap {
		total += HistoryBytes(wp.History)
	}
	historyBytesTotal.Store(total)
	if total <= budget {
		return
	}

	type victim struct {
		link       string
		generation uint64
		entry      HistoryEntry
	}
	all := make([]victim, 0, 64)
	for _, wp := range snap {
		for _, e := range wp.History {
			all = append(all, victim{link: wp.LinkName, generation: wp.Version, entry: e})
		}
	}
	// Oldest archive first, so the versions a user just replaced survive.
	sort.Slice(all, func(i, j int) bool {
		if all[i].entry.SavedAt != all[j].entry.SavedAt {
			return all[i].entry.SavedAt < all[j].entry.SavedAt
		}
		if all[i].link != all[j].link {
			return all[i].link < all[j].link
		}
		return all[i].entry.Version < all[j].entry.Version
	})

	type group struct {
		link       string
		generation uint64
		entries    []HistoryEntry
	}
	var order []string
	groups := make(map[string]*group)
	for _, v := range all {
		if total <= budget {
			break
		}
		g, ok := groups[v.link]
		if !ok {
			g = &group{link: v.link, generation: v.generation}
			groups[v.link] = g
			order = append(order, v.link)
		}
		g.entries = append(g.entries, v.entry)
		if v.entry.SizeBytes > 0 {
			total -= v.entry.SizeBytes
		}
	}

	for _, link := range order {
		g := groups[link]
		// dropped is only honoured once the metadata commit succeeded: a failed
		// commit leaves the entries referenced, and deleting their files would
		// strand them.
		var removed []HistoryEntry
		func() {
			unlock := LockLinks(link)
			defer unlock()
			var dropped []HistoryEntry
			_, err := Global.Update(link, func(wp *Wallpaper) error {
				if wp.Version != g.generation {
					return errStale
				}
				dropped = nil
				before := HistoryBytes(wp.History)
				kept := wp.History
				for _, e := range g.entries {
					next, ok := WithoutHistoryVersion(kept, e.Version)
					if !ok {
						return errStale
					}
					kept = next
					dropped = append(dropped, e)
				}
				wp.History = kept
				NoteHistoryBytes(HistoryBytes(kept) - before)
				return nil
			})
			if err != nil {
				if !errors.Is(err, errStale) && !errors.Is(err, ErrNotFound) {
					log.Printf("History: could not trim %s: %v", link, err)
				}
				return
			}
			removed = dropped
		}()
		if len(removed) > 0 {
			log.Printf("History budget: dropped %d archived version(s) of %s", len(removed), link)
			DeleteHistoryFiles(link, removed)
		}
	}
}
