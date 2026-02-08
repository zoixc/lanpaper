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
	"time"
)

type Wallpaper struct {
	ID        string `json:"id"`
	LinkName  string `json:"linkName"`
	ImageURL  string `json:"imageUrl"`
	Preview   string `json:"preview"`
	HasImage  bool   `json:"hasImage"`
	MIMEType  string `json:"mimeType"`
	SizeBytes int64  `json:"sizeBytes"`
	ModTime   int64  `json:"modTime"`
	CreatedAt int64  `json:"createdAt"`

	// Internal paths
	ImagePath   string `json:"-"`
	PreviewPath string `json:"-"`
}

type Store struct {
	sync.RWMutex
	wallpapers map[string]*Wallpaper
}

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

func (s *Store) Save() error {
	s.RLock()
	defer s.RUnlock()
	data, err := json.MarshalIndent(s.wallpapers, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal wallpapers: %w", err)
	}
	if err := os.WriteFile("data/wallpapers.json", data, 0644); err != nil {
		return fmt.Errorf("failed to write wallpapers.json: %w", err)
	}
	return nil
}

func (s *Store) Load() error {
	data, err := os.ReadFile("data/wallpapers.json")
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
		if wp.HasImage {
			ext := wp.MIMEType
			wp.ImagePath = filepath.Join("static", "images", wp.LinkName+"."+ext)

			if ext == "mp4" || ext == "webm" {
				wp.PreviewPath = ""
			} else {
				wp.PreviewPath = filepath.Join("static", "images", "previews", wp.LinkName+".webp")
			}
		}
	}

	s.Lock()
	s.wallpapers = m
	s.Unlock()
	return nil
}

func PruneOldImages(max int) {
	Global.Lock()

	var candidates []*Wallpaper
	for _, wp := range Global.wallpapers {
		if wp.HasImage {
			candidates = append(candidates, wp)
		}
	}

	if len(candidates) <= max {
		Global.Unlock()
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

	Global.Unlock()
	if err := Global.Save(); err != nil {
		log.Printf("Error saving wallpapers after pruning: %v", err)
	}
}
