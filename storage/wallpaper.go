package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"lanpaper/config"
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

	// Not persisted; derived from MIMEType on Load.
	ImagePath   string `json:"-"`
	PreviewPath string `json:"-"`
}

// Store is a thread-safe in-memory store backed by a JSON file.
// sortedSnap caches the sorted slice and is invalidated on any mutation.
type Store struct {
	sync.RWMutex
	wallpapers map[string]*Wallpaper
	sortedSnap []*Wallpaper
}

const dataFile = "data/wallpapers.json"

// Global is the application-wide wallpaper store.
var Global = &Store{wallpapers: make(map[string]*Wallpaper)}

func (s *Store) Get(id string) (*Wallpaper, bool) {
	s.RLock()
	defer s.RUnlock()
	wp, ok := s.wallpapers[id]
	return wp, ok
}

func (s *Store) Set(id string, wp *Wallpaper) {
	s.Lock()
	defer s.Unlock()
	s.wallpapers[id] = wp
	s.sortedSnap = nil
}

func (s *Store) Delete(id string) {
	s.Lock()
	defer s.Unlock()
	delete(s.wallpapers, id)
	s.sortedSnap = nil
}

// Rename atomically renames oldName -> newName in the store.
// Returns false if oldName not found or newName already exists.
func (s *Store) Rename(oldName, newName string) (*Wallpaper, bool) {
	s.Lock()
	defer s.Unlock()
	wp, ok := s.wallpapers[oldName]
	if !ok {
		return nil, false
	}
	if _, exists := s.wallpapers[newName]; exists {
		return nil, false
	}
	wp.ID = newName
	wp.LinkName = newName
	s.wallpapers[newName] = wp
	delete(s.wallpapers, oldName)
	s.sortedSnap = nil
	return wp, true
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

// GetAll returns a sorted snapshot: pinned first, then images (newest ModTime),
// then empty slots (newest CreatedAt). Callers must not modify the returned pointers.
// The result is cached until the store is mutated.
func (s *Store) GetAll() []*Wallpaper {
	s.RLock()
	if s.sortedSnap != nil {
		snap := s.sortedSnap
		s.RUnlock()
		return snap
	}
	s.RUnlock()

	// Cache miss: build under write lock to prevent duplicate work.
	s.Lock()
	defer s.Unlock()
	if s.sortedSnap != nil {
		return s.sortedSnap
	}
	snap := make([]*Wallpaper, 0, len(s.wallpapers))
	for _, wp := range s.wallpapers {
		if wp != nil {
			snap = append(snap, wp)
		}
	}
	sortSnap(snap)
	s.sortedSnap = snap
	return snap
}

// GetAllCopy returns a deep copy for cases where mutation is needed.
func (s *Store) GetAllCopy() []*Wallpaper {
	original := s.GetAll()
	snap := make([]*Wallpaper, len(original))
	for i, wp := range original {
		clone := *wp
		snap[i] = &clone
	}
	return snap
}

// atomicWrite marshals data to a temp file and renames it atomically,
// so a crash mid-write never produces a truncated JSON file.
func atomicWrite(path string, data map[string]*Wallpaper) error {
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".wallpapers-*.json")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}

// Save persists the current state to disk atomically.
func (s *Store) Save() error {
	s.RLock()
	defer s.RUnlock()
	return atomicWrite(dataFile, s.wallpapers)
}

// MediaPath returns the canonical on-disk path for a link's media file.
func MediaPath(linkName, mimeExt string) string {
	return filepath.Join(config.MediaDir, linkName+"."+mimeExt)
}

// PreviewFilePath returns the canonical on-disk path for a link's WebP preview.
func PreviewFilePath(linkName string) string {
	return filepath.Join(config.PreviewDir, linkName+".webp")
}

// NormalizeAccessLevel returns a valid access level, defaulting to public.
func NormalizeAccessLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	if config.ValidAccessLevels[level] {
		return level
	}
	return config.AccessPublic
}

// derivePaths fills runtime-only ImagePath/PreviewPath and public URLs.
// Prefers the new data/ layout; falls back to legacy static/images/ if the
// new file is missing but the old one still exists (pre-migration installs).
func derivePaths(wp *Wallpaper) {
	if wp.AccessLevel == "" {
		wp.AccessLevel = config.AccessPublic
	} else {
		wp.AccessLevel = NormalizeAccessLevel(wp.AccessLevel)
	}
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

	if wp.MIMEType == "mp4" || wp.MIMEType == "webm" {
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
	if err := os.MkdirAll(config.MediaDir, 0755); err != nil {
		log.Printf("Warning: cannot create %s: %v", config.MediaDir, err)
		return
	}
	if err := os.MkdirAll(config.PreviewDir, 0755); err != nil {
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

		if wp.MIMEType != "mp4" && wp.MIMEType != "webm" {
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
		if err := Global.Save(); err != nil {
			log.Printf("Warning: save after media migration: %v", err)
		}
		log.Printf("Migrated %d media file(s) from static/images to data/media", moved)
	}
}

func moveIfNeeded(src, dst string) bool {
	if src == dst {
		return false
	}
	if _, err := os.Stat(src); err != nil {
		return false
	}
	if _, err := os.Stat(dst); err == nil {
		// Destination already present — drop the legacy copy.
		_ = os.Remove(src)
		return true
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
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
	in, err := os.Open(src)
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
	for key, wp := range m {
		if wp == nil {
			log.Printf("Warning: skipping nil wallpaper entry for key %q in storage", key)
			delete(m, key)
			continue
		}
		if wp.LinkName == "" {
			wp.LinkName = key
		}
		if wp.ID == "" {
			wp.ID = key
		}
		derivePaths(wp)
	}
	s.Lock()
	s.wallpapers = m
	s.sortedSnap = nil
	s.Unlock()
	return nil
}

// PruneOldImages removes the oldest non-pinned images when count exceeds max,
// preserving empty slots and pinned entries.
// File I/O is performed outside the lock to avoid blocking Get/Set during disk operations.
func PruneOldImages(max int) {
	Global.Lock()
	var candidates []*Wallpaper
	for _, wp := range Global.wallpapers {
		// Skip nil, empty slots, and pinned entries — they are never pruned.
		if wp != nil && wp.HasImage && !wp.IsPinned {
			clone := *wp
			candidates = append(candidates, &clone)
		}
	}
	Global.Unlock()

	if len(candidates) <= max {
		return
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ModTime < candidates[j].ModTime
	})

	for _, wp := range candidates[:len(candidates)-max] {
		log.Printf("Pruning old image: %s", wp.ID)
		if err := os.Remove(wp.ImagePath); err != nil && !os.IsNotExist(err) {
			log.Printf("Error pruning image %s: %v", wp.ImagePath, err)
		}
		if wp.PreviewPath != "" {
			if err := os.Remove(wp.PreviewPath); err != nil && !os.IsNotExist(err) {
				log.Printf("Error pruning preview %s: %v", wp.PreviewPath, err)
			}
		}
		Global.Set(wp.ID, &Wallpaper{
			ID:          wp.ID,
			LinkName:    wp.LinkName,
			Category:    wp.Category,
			CreatedAt:   wp.CreatedAt,
			IsPinned:    wp.IsPinned,
			PinnedAt:    wp.PinnedAt,
			AccessLevel: NormalizeAccessLevel(wp.AccessLevel),
			AccessToken: wp.AccessToken,
		})
	}

	if err := Global.Save(); err != nil {
		log.Printf("Error saving after pruning: %v", err)
	}
}
