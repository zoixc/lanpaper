package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"lanpaper/config"
	"lanpaper/storage"
)

// RegeneratePreviewsResult is the JSON response for /api/regenerate-previews.
type RegeneratePreviewsResult struct {
	Total   int      `json:"total"`
	OK      int      `json:"ok"`
	Skipped int      `json:"skipped"`
	Errors  int      `json:"errors"`
	Failed  []string `json:"failed,omitempty"`
}

const maxFailedItems = 100

// RegeneratePreviews re-generates WebP thumbnails for every stored image entry.
// Only POST is accepted. Worker count scales with available CPUs (capped at 8).
func RegeneratePreviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wallpapers := storage.Global.GetAllCopy()
	total := len(wallpapers)
	skipped := 0

	type job struct{ wp *storage.Wallpaper }
	jobs := make(chan job, total)
	for _, wp := range wallpapers {
		if wp == nil || !wp.HasImage || isVideo(wp.MIMEType) {
			skipped++
			continue
		}
		jobs <- job{wp: wp}
	}
	close(jobs)

	var (
		okCount  atomic.Int32
		errCount atomic.Int32
		failedMu sync.Mutex
		failed   []string
	)

	ctx := r.Context()
	workers := min(max(runtime.NumCPU(), 1), 8)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if ctx.Err() != nil {
					return
				}
				wp := j.wp
				if err := regenPreview(ctx, wp); err != nil {
					log.Printf("RegeneratePreviews: %s: %v", wp.LinkName, err)
					errCount.Add(1)
					failedMu.Lock()
					if len(failed) < maxFailedItems {
						failed = append(failed, wp.LinkName)
					} else if len(failed) == maxFailedItems {
						failed = append(failed, "...and more")
					}
					failedMu.Unlock()
				} else {
					okCount.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	if err := storage.Global.Save(); err != nil {
		log.Printf("RegeneratePreviews: save storage: %v", err)
	}
	cleanStalePreviewFiles()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(RegeneratePreviewsResult{
		Total:   total,
		OK:      int(okCount.Load()),
		Skipped: skipped,
		Errors:  int(errCount.Load()),
		Failed:  failed,
	})
}

func regenPreview(ctx context.Context, wp *storage.Wallpaper) error {
	img, _, fileData, err := loadLocalImage(ctx, wp.ImagePath)
	if err != nil {
		return err
	}
	// Lossless path: loadLocalImage returns nil img — decode from raw bytes.
	if img == nil {
		if len(fileData) == 0 {
			return nil
		}
		if img, _, err = image.Decode(bytes.NewReader(fileData)); err != nil {
			return err
		}
	}
	prevPath := previewFilePath(wp.LinkName)
	if err := atomicSaveImage(thumbnail(img, config.ThumbnailMaxWidth, config.ThumbnailMaxHeight), "webp", prevPath); err != nil {
		return err
	}
	wp.PreviewPath = prevPath
	wp.Preview = previewURLPath(wp.LinkName)
	storage.Global.Set(wp.LinkName, wp)
	return nil
}

// cleanStalePreviewFiles removes .webp files in previews/ with no matching storage entry.
func cleanStalePreviewFiles() {
	previewDir := filepath.Join("static", "images", "previews")
	entries, err := os.ReadDir(previewDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".webp" {
			continue
		}
		linkName := e.Name()[:len(e.Name())-len(ext)]
		if _, exists := storage.Global.Get(linkName); !exists {
			path := filepath.Join(previewDir, e.Name())
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				log.Printf("cleanStalePreviewFiles: remove %s: %v", path, err)
			}
		}
	}
}
