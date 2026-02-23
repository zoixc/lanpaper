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
)

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

	// Runtime-only fields: not persisted to JSON, derived from MIMEType on Load()
	ImagePath   string `json:"-"`
	PreviewPath string `json:"-"`
}

type Store struct {
	sync.RWMutex
	wallpapers map[string]*Wallpaper
}

const dataFile = "data/wallpapers.json"

var Global = &Store{wallpapers: make(map[string]*Wallpaper)}

func (s *Store) Get(id string) (*Wallpaper, bool) {
	s.RLock()
	defer s.RUnlock()
	wp, exists := s.wallpapers[id]
	return wp, exists
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

func (s *Store) GetAll() []*Wallpaper {
	s.RLock()
	defer s.RUnlock()

	var wallpapers []*Wallpaper
	for _, wp := range s.wallpapers {
		if wp != nil {
			clone := *wp
			wallpapers = append(wallpapers, &clone)
		}
	}

	sort.Slice(wallpapers, func(i, j int) bool {
		if wallpapers[i].HasImage != wallpapers[j].HasImage {
			return wallpapers[i].HasImage
		}
		if wallpapers[i].HasImage {
			return wallpapers[i].ModTime > wallpapers[j].ModTime
		}
		return wallpapers[i].CreatedAt > wallpapers[j].CreatedAt
	})

	return wallpapers
}

// atomicWrite marshals data and writes it atomically via a temp file + rename.
func atomicWrite(path string, data map[string]*Wallpaper) error {
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal wallpapers: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".wallpapers-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}

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
	if wp.MIMEType == "mp4" || wp.MIMEType == "webm" {
		wp.PreviewPath = ""
	} else {
		wp.PreviewPath = filepath.Join("static", "images", "previews", wp.LinkName+".webp")
	}
}

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

// PruneOldImages removes oldest images when exceeding max count.
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

	toDelete := len(candidates) - max
	for i := 0; i < toDelete; i++ {
		wp := candidates[i]
		log.Printf("Pruning old image: %s", wp.ID)

		if err := os.Remove(wp.ImagePath); err != nil && !os.IsNotExist(err) {
			log.Printf("Error pruning image %s: %v", wp.ImagePath, err)
		}
		if wp.PreviewPath != "" && !strings.HasPrefix(wp.PreviewPath, "/static/") {
			if err := os.Remove(wp.PreviewPath); err != nil && !os.IsNotExist(err) {
				log.Printf("Error pruning preview %s: %v", wp.PreviewPath, err)
			}
		}

		wp.HasImage = false
		wp.ImageURL = ""
		wp.Preview = ""
		wp.MIMEType = ""
		wp.SizeBytes = 0
		wp.ImagePath = ""
		wp.PreviewPath = ""
	}

	if err := atomicWrite(dataFile, Global.wallpapers); err != nil {
		log.Printf("Error saving wallpapers after pruning: %v", err)
	}
}
