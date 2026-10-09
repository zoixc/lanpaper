// SPDX-License-Identifier: MIT

package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"sync"

	"lanpaper/config"
	"lanpaper/internal/atomicfile"
	appmetrics "lanpaper/internal/metrics"
	"lanpaper/utils"
)

// Wallpaper represents a named wallpaper slot.
type Wallpaper struct {
	ID          string `json:"id"`
	LinkName    string `json:"linkName"`
	Category    string `json:"category"`
	ImageURL    string `json:"imageUrl"`
	Preview     string `json:"preview"`
	HasImage    bool   `json:"hasImage"`
	MIMEType    string `json:"mimeType"`
	SizeBytes   int64  `json:"sizeBytes"`
	ModTime     int64  `json:"modTime"`
	CreatedAt   int64  `json:"createdAt"`
	IsPinned    bool   `json:"isPinned"`
	PinnedAt    int64  `json:"pinnedAt,omitempty"`
	AccessLevel string `json:"accessLevel,omitempty"` // public|local|token|auth
	AccessToken string `json:"accessToken,omitempty"` // secret for token level

	// CurrentVersion numbers the live media file. It starts at 1 with the first
	// upload and increases on every replace and rollback, so a version is never
	// reused and an old bookmarked ?v= URL cannot silently point at new bytes.
	// Records written before versioning existed keep 0, which is read as 1.
	CurrentVersion uint64 `json:"currentVersion,omitempty"`
	// History lists the replaced versions still on disk, newest first. Files
	// live in data/history/{link}/{version}.{ext}; empty when HISTORY_LIMIT=0.
	History []HistoryEntry `json:"history,omitempty"`
	// Items are extra media files served from the same URL (a playlist). The
	// live file is position 0 and is not listed here. Files live in
	// data/items/{link}/{id}.{ext}.
	Items []PlaylistItem `json:"items,omitempty"`
	// Rotate switches between the live file and Items over time. It is a
	// pointer because encoding/json never omits an empty struct: a link that
	// does not use rotation must not gain a "rotate" key in wallpapers.json.
	Rotate *RotateConfig `json:"rotate,omitempty"`

	// Not persisted; derived from MIMEType on Load.
	ImagePath   string `json:"-"`
	PreviewPath string `json:"-"`
	Version     uint64 `json:"-"` // runtime generation for pruning stale snapshots
}

// Store is a thread-safe in-memory store backed by a JSON file.
// sortedSnap caches the sorted slice and is invalidated on any mutation.
type Store struct {
	sync.RWMutex
	// writeMu serializes every change to wallpapers. A writer holds it for the
	// whole read-modify-write-publish sequence, including the disk write, but
	// takes the RWMutex only for the in-memory reads and the final swap. Readers
	// therefore never wait for file I/O or fsync.
	writeMu    sync.Mutex
	wallpapers map[string]*Wallpaper
	sortedSnap []*Wallpaper
	generation uint64
}

const dataFile = "data/wallpapers.json"

// Global is the application-wide wallpaper store.
var Global = &Store{wallpapers: make(map[string]*Wallpaper)}

var (
	ErrNotFound = errors.New("link not found")
	ErrExists   = errors.New("link already exists")
)

// Get returns a copy. Callers can never mutate a stored record outside the
// store lock, nor see a partially updated record.
func (s *Store) Get(id string) (*Wallpaper, bool) {
	s.RLock()
	defer s.RUnlock()
	wp, ok := s.wallpapers[id]
	if !ok || wp == nil {
		return nil, false
	}
	return cloneWallpaper(wp), true
}

// Set is for the one-time startup media migration. Request handlers must use
// the persistent Create/Update/Rename/DeleteEntry operations below.
func (s *Store) Set(id string, wp *Wallpaper) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.Lock()
	defer s.Unlock()
	clone := *wp
	s.generation++
	clone.Version = s.generation
	s.wallpapers[id] = &clone
	s.sortedSnap = nil
}

// writeFile persists a full wallpaper map. It is a variable so tests can slow
// it down and check that readers are not blocked by a write in progress.
var writeFile = atomicWrite

// commit persists next BEFORE publishing it to readers. No handler may report
// success or change access levels in memory if saving fails. The caller must
// hold writeMu and must not hold the RWMutex, so readers keep running during
// the write.
func (s *Store) commit(next map[string]*Wallpaper) error {
	if err := writeFile(dataFile, next); err != nil {
		return err
	}
	s.Lock()
	s.wallpapers = next
	s.generation++
	s.sortedSnap = nil
	s.Unlock()
	return nil
}

func (s *Store) Create(wp *Wallpaper) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.RLock()
	if _, ok := s.wallpapers[wp.LinkName]; ok {
		s.RUnlock()
		return ErrExists
	}
	next := maps.Clone(s.wallpapers)
	if next == nil {
		next = make(map[string]*Wallpaper)
	}
	clone := *wp
	clone.Version = s.generation + 1
	s.RUnlock()
	next[wp.LinkName] = &clone
	return s.commit(next)
}

// Update applies edit to a copy and publishes it only after successful save.
// The returned record is also a copy; never mutate a record from the map.
func (s *Store) Update(id string, edit func(*Wallpaper) error) (*Wallpaper, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.RLock()
	old, ok := s.wallpapers[id]
	if !ok || old == nil {
		s.RUnlock()
		return nil, ErrNotFound
	}
	clone := *old
	if err := edit(&clone); err != nil {
		s.RUnlock()
		return nil, err
	}
	clone.Version = s.generation + 1
	next := maps.Clone(s.wallpapers)
	s.RUnlock()
	next[id] = &clone
	if err := s.commit(next); err != nil {
		return nil, err
	}
	out := clone
	return &out, nil
}

func (s *Store) Rename(oldName, newName string) (*Wallpaper, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.RLock()
	wp, ok := s.wallpapers[oldName]
	if !ok || wp == nil {
		s.RUnlock()
		return nil, ErrNotFound
	}
	if _, exists := s.wallpapers[newName]; exists {
		s.RUnlock()
		return nil, ErrExists
	}
	clone := *wp
	clone.ID, clone.LinkName = newName, newName
	clone.Version = s.generation + 1
	clone.ImageURL = "/" + newName
	if clone.HasImage && clone.MIMEType != "" {
		clone.ImagePath = MediaPath(newName, clone.MIMEType)
		if clone.MIMEType != "mp4" && clone.MIMEType != "webm" {
			clone.PreviewPath = PreviewFilePath(newName)
			clone.Preview = "/api/preview/" + newName
		}
	}
	next := maps.Clone(s.wallpapers)
	s.RUnlock()
	delete(next, oldName)
	next[newName] = &clone
	if err := s.commit(next); err != nil {
		return nil, err
	}
	out := clone
	return &out, nil
}

func (s *Store) DeleteEntry(id string) (*Wallpaper, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.RLock()
	wp, ok := s.wallpapers[id]
	if !ok || wp == nil {
		s.RUnlock()
		return nil, ErrNotFound
	}
	next := maps.Clone(s.wallpapers)
	s.RUnlock()
	delete(next, id)
	if err := s.commit(next); err != nil {
		return nil, err
	}
	clone := *wp
	return &clone, nil
}

func sortSnap(snap []*Wallpaper) {
	sort.Slice(snap, func(i, j int) bool {
		// Pinned links always come first
		if snap[i].IsPinned != snap[j].IsPinned {
			return snap[i].IsPinned
		}
		// Among pinned, sort by PinnedAt (most recent first)
		if snap[i].IsPinned && snap[j].IsPinned {
			return snap[i].PinnedAt > snap[j].PinnedAt
		}
		// Then by image presence
		if snap[i].HasImage != snap[j].HasImage {
			return snap[i].HasImage
		}
		// Then by modification/creation time
		if snap[i].HasImage {
			return snap[i].ModTime > snap[j].ModTime
		}
		return snap[i].CreatedAt > snap[j].CreatedAt
	})
}

// GetAll returns a sorted, independent snapshot of the records. Sorting is
// cached, but neither the cache nor any record in the store is exposed.
func (s *Store) GetAll() []*Wallpaper {
	s.RLock()
	if s.sortedSnap != nil {
		snap := cloneSnap(s.sortedSnap)
		s.RUnlock()
		return snap
	}
	s.RUnlock()

	s.Lock()
	defer s.Unlock()
	if s.sortedSnap == nil {
		snap := make([]*Wallpaper, 0, len(s.wallpapers))
		for _, wp := range s.wallpapers {
			if wp != nil {
				snap = append(snap, wp)
			}
		}
		sortSnap(snap)
		s.sortedSnap = snap
	}
	return cloneSnap(s.sortedSnap)
}

// Snapshot returns the records in storage order without copying the records.
// Every write publishes a new copy instead of changing a record in place, so a
// record a caller holds never changes underneath it. The caller may reorder or
// filter the returned slice, but must treat the records as read-only.
func (s *Store) Snapshot() []*Wallpaper {
	s.RLock()
	if s.sortedSnap != nil {
		snap := slices.Clone(s.sortedSnap)
		s.RUnlock()
		return snap
	}
	s.RUnlock()

	s.Lock()
	defer s.Unlock()
	if s.sortedSnap == nil {
		snap := make([]*Wallpaper, 0, len(s.wallpapers))
		for _, wp := range s.wallpapers {
			if wp != nil {
				snap = append(snap, wp)
			}
		}
		sortSnap(snap)
		s.sortedSnap = snap
	}
	return slices.Clone(s.sortedSnap)
}

func cloneSnap(original []*Wallpaper) []*Wallpaper {
	snap := make([]*Wallpaper, len(original))
	for i, wp := range original {
		snap[i] = cloneWallpaper(wp)
	}
	return snap
}

// cloneWallpaper copies a record, slice fields included, so a caller can read
// and modify what it was handed without reaching into the live store. A plain
// struct copy would share the Items and History backing arrays.
func cloneWallpaper(wp *Wallpaper) *Wallpaper {
	clone := *wp
	clone.Items = slices.Clone(wp.Items)
	clone.History = slices.Clone(wp.History)
	return &clone
}

// atomicWrite marshals data to a temp file, flushes it to stable storage and
// renames it atomically, so neither a crash nor a power loss mid-write can
// leave a truncated or empty JSON file behind.
func atomicWrite(path string, data map[string]*Wallpaper) error {
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	err = atomicfile.Write(atomicfile.OSFS{}, path, ".wallpapers-*.json", body)
	if atomicfile.IsCommitted(err) {
		appmetrics.PersistenceFailure()
		log.Printf("Warning: wallpaper metadata was renamed but directory durability could not be confirmed: %v", err)
		return nil
	}
	if err != nil {
		appmetrics.PersistenceFailure()
	}
	return err
}

// MediaPath returns the canonical on-disk path for a link's media file.
func MediaPath(linkName, mimeExt string) string {
	return filepath.Join(config.MediaDir, linkName+"."+mimeExt)
}

// PreviewFilePath returns the canonical on-disk path for a link's WebP preview.
func PreviewFilePath(linkName string) string {
	return filepath.Join(config.PreviewDir, linkName+".webp")
}

// NormalizeAccessLevel defaults missing (legacy) levels to public; unknown
// non-empty values fail closed instead of exposing formerly private media.
func NormalizeAccessLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		return config.AccessPublic
	}
	if config.ValidAccessLevels[level] {
		return level
	}
	return config.AccessAuth
}

// derivePaths fills runtime-only ImagePath/PreviewPath and public URLs.
// Prefers the new data/ layout; falls back to legacy static/images/ if the
// new file is missing but the old one still exists (pre-migration installs).
func derivePaths(wp *Wallpaper) {
	wp.AccessLevel = NormalizeAccessLevel(wp.AccessLevel)
	// Public URL is always the stable link path — never a direct filesystem URL.
	wp.ImageURL = "/" + wp.LinkName
	// Admin preview endpoint (auth-protected); empty when no image.
	if !wp.HasImage || wp.MIMEType == "" {
		wp.ImagePath = ""
		wp.PreviewPath = ""
		wp.Preview = ""
		return
	}

	newImg := MediaPath(wp.LinkName, wp.MIMEType)
	legacyImg := filepath.Join(config.LegacyMedia, wp.LinkName+"."+wp.MIMEType)
	wp.ImagePath = pickExisting(newImg, legacyImg)

	if config.IsVideoExt(wp.MIMEType) {
		wp.PreviewPath = ""
		wp.Preview = ""
		return
	}

	newPrev := PreviewFilePath(wp.LinkName)
	legacyPrev := filepath.Join(config.LegacyMedia, "previews", wp.LinkName+".webp")
	wp.PreviewPath = pickExisting(newPrev, legacyPrev)
	if wp.PreviewPath != "" {
		// Auth-protected admin preview route — never expose via /static/.
		wp.Preview = "/api/preview/" + wp.LinkName
	} else {
		wp.Preview = ""
	}
}

// pickExisting returns primary if it exists, otherwise fallback if it exists,
// otherwise primary (so new uploads write to the canonical location).
func pickExisting(primary, fallback string) string {
	if _, err := os.Stat(primary); err == nil {
		return primary
	}
	if fallback != "" {
		if _, err := os.Stat(fallback); err == nil {
			return fallback
		}
	}
	return primary
}

// MigrateMediaToDataDir moves legacy static/images files into data/media and
// data/previews. Safe to call repeatedly; skips missing sources.
func MigrateMediaToDataDir() {
	if err := os.MkdirAll(config.MediaDir, config.DataDirPerm); err != nil {
		log.Printf("Warning: cannot create %s: %v", config.MediaDir, err)
		return
	}
	if err := os.MkdirAll(config.PreviewDir, config.DataDirPerm); err != nil {
		log.Printf("Warning: cannot create %s: %v", config.PreviewDir, err)
		return
	}

	Global.RLock()
	ids := make([]string, 0, len(Global.wallpapers))
	for id := range Global.wallpapers {
		ids = append(ids, id)
	}
	Global.RUnlock()

	moved := 0
	for _, id := range ids {
		wp, ok := Global.Get(id)
		if !ok || wp == nil || !wp.HasImage || wp.MIMEType == "" {
			continue
		}
		changed := false

		dstImg := MediaPath(wp.LinkName, wp.MIMEType)
		srcImg := filepath.Join(config.LegacyMedia, wp.LinkName+"."+wp.MIMEType)
		if moveIfNeeded(srcImg, dstImg) {
			wp.ImagePath = dstImg
			changed = true
			moved++
		}

		if !config.IsVideoExt(wp.MIMEType) {
			dstPrev := PreviewFilePath(wp.LinkName)
			srcPrev := filepath.Join(config.LegacyMedia, "previews", wp.LinkName+".webp")
			if moveIfNeeded(srcPrev, dstPrev) {
				wp.PreviewPath = dstPrev
				changed = true
			}
		}

		if changed {
			derivePaths(wp)
			Global.Set(id, wp)
		}
	}
	if moved > 0 {
		// Paths are runtime-only (json:"-"); no metadata save is needed.
		log.Printf("Migrated %d media file(s) from static/images to data/media", moved)
	}
	if left := countLegacyLeftovers(); left > 0 {
		log.Printf("Warning: %d file(s) remain in %s after migration (read-only mount or a failed move). "+
			"Mount that directory writable to finish the move, or delete the files once their links have data/ copies.",
			left, config.LegacyMedia)
	}
}

// countLegacyLeftovers counts regular files still in the legacy static/images
// tree (top level and previews/). Symlinks and directories are not counted.
func countLegacyLeftovers() int {
	count := 0
	for _, dir := range []string{config.LegacyMedia, filepath.Join(config.LegacyMedia, "previews")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Type().IsRegular() {
				count++
			}
		}
	}
	return count
}

func moveIfNeeded(src, dst string) bool {
	if src == dst {
		return false
	}
	srcInfo, err := os.Lstat(src)
	if err != nil || !srcInfo.Mode().IsRegular() {
		// Never migrate a planted symlink or device from a legacy mount.
		return false
	}
	if dstInfo, err := os.Lstat(dst); err == nil {
		if !dstInfo.Mode().IsRegular() {
			log.Printf("Warning: cannot migrate to non-regular file %s", dst)
			return false
		}
		// Destination already present — drop the legacy copy.
		_ = os.Remove(src)
		return true
	} else if !os.IsNotExist(err) {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(dst), config.DataDirPerm); err != nil {
		log.Printf("Warning: mkdir for %s: %v", dst, err)
		return false
	}
	if err := os.Rename(src, dst); err != nil {
		// Cross-device rename may fail; fall back to copy+remove.
		if copyErr := copyFileContents(src, dst); copyErr != nil {
			log.Printf("Warning: migrate %s -> %s: %v", src, dst, copyErr)
			return false
		}
		_ = os.Remove(src)
	}
	return true
}

func copyFileContents(src, dst string) error {
	in, err := OpenMedia(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(dst), ".mig-*")
	if err != nil {
		return err
	}
	tmp := out.Name()
	_, copyErr := out.ReadFrom(in)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func validStoredMediaExt(ext string) bool {
	return config.AllowedMediaExts["."+ext]
}

// sanitizeMediaLists drops persisted history and playlist entries this server
// could not have written (an invalid extension would otherwise be turned into a
// file path) and normalizes rotation settings.
//
// Unlike the identity and media-type checks in Load, a bad value here is
// ignored rather than fatal: these fields are optional extras added later, and
// refusing to start — or worse, wiping a link — because of a corrupt counter
// would degrade the whole server for a feature it may not even use.
func sanitizeMediaLists(wp *Wallpaper) {
	if len(wp.History) > 0 {
		kept := make([]HistoryEntry, 0, len(wp.History))
		seen := make(map[uint64]bool, len(wp.History))
		for _, entry := range wp.History {
			if entry.Version == 0 || seen[entry.Version] || !validStoredMediaExt(entry.Ext) {
				log.Printf("Warning: dropping invalid history entry of %s (version %d)", wp.LinkName, entry.Version)
				continue
			}
			seen[entry.Version] = true
			if entry.SizeBytes < 0 {
				entry.SizeBytes = 0
			}
			kept = append(kept, entry)
		}
		wp.History = kept
	}
	if len(wp.Items) > 0 {
		kept := make([]PlaylistItem, 0, len(wp.Items))
		seen := make(map[int]bool, len(wp.Items))
		for _, item := range wp.Items {
			if item.ID <= 0 || seen[item.ID] || !validStoredMediaExt(item.Ext) {
				log.Printf("Warning: dropping invalid playlist item of %s (id %d)", wp.LinkName, item.ID)
				continue
			}
			seen[item.ID] = true
			if item.SizeBytes < 0 {
				item.SizeBytes = 0
			}
			kept = append(kept, item)
		}
		wp.Items = kept
	}
	wp.Rotate = NormalizeRotatePtr(wp.Rotate, len(wp.Items) > 0)
	// Repair a version number that collides with an archived one, so a future
	// archive can never overwrite a file that is still listed.
	if _, clash := FindHistory(wp.History, wp.CurrentVersion); clash {
		highest := wp.CurrentVersion
		for _, entry := range wp.History {
			if entry.Version > highest {
				highest = entry.Version
			}
		}
		log.Printf("Warning: %s had version %d both live and archived; live is now %d",
			wp.LinkName, wp.CurrentVersion, highest+1)
		wp.CurrentVersion = highest + 1
	}
}

// Load reads wallpapers from disk. A missing file is treated as first run.
func (s *Store) Load() error {
	data, err := os.ReadFile(dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	m := make(map[string]*Wallpaper)
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if m == nil {
		return fmt.Errorf("invalid wallpaper object in %s", dataFile)
	}
	var archived int64
	for key, wp := range m {
		if wp == nil || !utils.IsValidLinkName(key) || (wp.HasImage && !validStoredMediaExt(wp.MIMEType)) {
			// Silently dropping a bad entry would permanently erase it on the
			// next save. Stop startup so an operator can repair the file.
			return fmt.Errorf("invalid wallpaper entry for key %q in %s", key, dataFile)
		}
		// Never build file paths from untrusted persisted ID/LinkName fields.
		// The validated map key is the canonical identifier.
		wp.ID, wp.LinkName = key, key
		sanitizeMediaLists(wp)
		archived += HistoryBytes(wp.History)
		derivePaths(wp)
	}
	// The archive budget is checked from a counter, so it has to start from the
	// value already on disk.
	historyBytesTotal.Store(archived)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.Lock()
	s.generation++
	for _, wp := range m {
		wp.Version = s.generation
	}
	s.wallpapers = m
	s.sortedSnap = nil
	s.Unlock()
	return nil
}

// SchedulePrune coalesces concurrent requests into at most one pending pass.
// A stream of uploads cannot spawn an unbounded number of prune goroutines.
var (
	pruneOnce     sync.Once
	pruneRequests = make(chan pruneRequest, 1)
	errStale      = errors.New("record changed while pruning")
)

// pruneRequest carries everything the background pass needs, so the worker
// goroutine never reads config.Current behind the caller's back.
type pruneRequest struct {
	maxImages int
	historyMB int
}

func SchedulePrune(max int) {
	// Both settings are read here, in the caller's goroutine: the worker must
	// not touch config.Current, because a request that changes configuration
	// (or a test that does) would race with it.
	historyMB := config.Current.History.MaxMB
	// With history enabled the same background pass also enforces the archive
	// budget, so it has to run even when MAX_IMAGES is 0 (unlimited links).
	if max <= 0 && config.Current.History.Limit <= 0 {
		return
	}
	pruneOnce.Do(func() {
		go func() {
			for req := range pruneRequests {
				runPrunePass(req)
			}
		}()
	})
	select {
	case pruneRequests <- pruneRequest{maxImages: max, historyMB: historyMB}:
	default:
	}
}

// PruneOldImages removes the oldest non-pinned images above max. The candidate
// snapshot is rechecked under the per-link lock before changing metadata;
// neither an upload nor an access change can be overwritten by a stale pass.
func PruneOldImages(max int) {
	if max <= 0 {
		return
	}
	var candidates []*Wallpaper
	for _, wp := range Global.GetAll() {
		if wp.HasImage && !wp.IsPinned {
			candidates = append(candidates, wp)
		}
	}
	if len(candidates) <= max {
		return
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ModTime < candidates[j].ModTime
	})
	for _, candidate := range candidates[:len(candidates)-max] {
		func() {
			unlock := LockLinks(candidate.ID)
			defer unlock()
			_, err := Global.Update(candidate.ID, func(wp *Wallpaper) error {
				if !wp.HasImage || wp.IsPinned || wp.Version != candidate.Version {
					return errStale
				}
				// The archived versions belong to the media being dropped, so
				// they leave the budget with it.
				NoteHistoryBytes(-HistoryBytes(wp.History))
				*wp = Wallpaper{
					ID: wp.ID, LinkName: wp.LinkName, Category: wp.Category,
					CreatedAt: wp.CreatedAt, AccessLevel: wp.AccessLevel,
					AccessToken: wp.AccessToken,
				}
				return nil
			})
			if err != nil {
				if !errors.Is(err, errStale) && !errors.Is(err, ErrNotFound) {
					log.Printf("Error persisting prune of %s: %v", candidate.ID, err)
				}
				return
			}
			log.Printf("Pruning old image: %s", candidate.ID)
			for _, path := range []string{candidate.ImagePath, candidate.PreviewPath} {
				if path == "" {
					continue
				}
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					log.Printf("Error removing pruned media %s: %v", path, err)
				}
			}
			// Archived versions, playlist items and access counters of a link
			// that no longer has media must not outlive it.
			RemoveLinkExtraDirs(candidate.ID)
			ForgetStats(candidate.ID)
		}()
	}
}

// runPrunePass performs one background maintenance pass. It runs on a goroutine
// nobody restarts, so a panic must not escape: one bad record would otherwise
// take the whole server down between two uploads. The pass is idempotent — the
// next request schedules it again.
func runPrunePass(req pruneRequest) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("Critical: background prune recovered from a panic: %v\n%s", p, debug.Stack())
		}
	}()
	PruneOldImages(req.maxImages)
	TrimHistoryBudget(req.historyMB)
}
