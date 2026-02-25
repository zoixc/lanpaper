package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Wallpaper represents a named wallpaper slot.
type Wallpaper struct {
	ID        string `json:"id"`
	LinkName  string `json:"linkName"`
	Category  string `json:"category"`
	ImageURL  string `json:"imageUrl"`
	Preview   string `json:"preview"`
	HasImage  bool   `json:"hasImage"`
	MIMEType  string `json:"mimeType"`
	SizeBytes int64  `json:"sizeBytes"`
	ModTime   int64  `json:"modTime"`
	CreatedAt int64  `json:"createdAt"`

	// Runtime-only fields: not persisted; derived from MIMEType on Load.
	ImagePath   string `json:"-"`
	PreviewPath string `json:"-"`
}

// Store is a thread-safe in-memory store backed by a JSON file.
type Store struct {
	sync.RWMutex
	wallpapers map[string]*Wallpaper
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
}

func (s *Store) Delete(id string) {
	s.Lock()
	defer s.Unlock()
	delete(s.wallpapers, id)
}

// GetAll returns a snapshot of all wallpapers sorted: images first (newest
// ModTime), then empty slots (newest CreatedAt).
// Returns pointers to the original wallpapers - callers must not modify.
// For mutable copies, use GetAllCopy.
func (s *Store) GetAll() []*Wallpaper {
	s.RLock()
	snap := make([]*Wallpaper, 0, len(s.wallpapers))
	for _, wp := range s.wallpapers {
		if wp != nil {
			snap = append(snap, wp)
		}
	}
	s.RUnlock()

	sort.Slice(snap, func(i, j int) bool {
		if snap[i].HasImage != snap[j].HasImage {
			return snap[i].HasImage
		}
		if snap[i].HasImage {
			return snap[i].ModTime > snap[j].ModTime
		}
		return snap[i].CreatedAt > snap[j].CreatedAt
	})
	return snap
}

// GetAllCopy returns a deep copy of all wallpapers for cases where
// mutation is needed without affecting the original data.
func (s *Store) GetAllCopy() []*Wallpaper {
	s.RLock()
	snap := make([]*Wallpaper, 0, len(s.wallpapers))
	for _, wp := range s.wallpapers {
		if wp != nil {
			clone := *wp
			snap = append(snap, &clone)
		}
	}
	s.RUnlock()

	sort.Slice(snap, func(i, j int) bool {
		if snap[i].HasImage != snap[j].HasImage {
			return snap[i].HasImage
		}
		if snap[i].HasImage {
			return snap[i].ModTime > snap[j].ModTime
		}
		return snap[i].CreatedAt > snap[j].CreatedAt
	})
	return snap
}

// atomicWrite marshals data and writes it via a temp-file + rename so that a
// crash mid-write never produces a truncated JSON file.
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

// derivePaths fills runtime-only ImagePath/PreviewPath from persisted fields.
func derivePaths(wp *Wallpaper) {
	if !wp.HasImage || wp.MIMEType == "" {
		return
	}
	wp.ImagePath = filepath.Join("static", "images", wp.LinkName+"."+wp.MIMEType)
	if wp.MIMEType != "mp4" && wp.MIMEType != "webm" {
		wp.PreviewPath = filepath.Join("static", "images", "previews", wp.LinkName+".webp")
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
	for _, wp := range m {
		derivePaths(wp)
	}
	s.Lock()
	s.wallpapers = m
	s.Unlock()
	return nil
}

// PruneOldImages removes the oldest images when the count exceeds max,
// keeping the newest max entries. Link slots are preserved (HasImage=false).
func PruneOldImages(max int) {
	Global.Lock()
	defer Global.Unlock()

	var candidates []*Wallpaper
	for _, wp := range Global.wallpapers {
		if wp.HasImage {
			candidates = append(candidates, wp)
		}
	}
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
		*wp = Wallpaper{ID: wp.ID, LinkName: wp.LinkName, Category: wp.Category, CreatedAt: wp.CreatedAt}
	}

	if err := atomicWrite(dataFile, Global.wallpapers); err != nil {
		log.Printf("Error saving after pruning: %v", err)
	}
}
