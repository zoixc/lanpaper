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
	Skipped int      `json:"skipped"` // videos or no-image entries
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

	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}
	if workers > 8 {
		workers = 8
	}

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				// Check context cancellation immediately
				if ctx.Err() != nil {
					return
				}
				wp := j.wp
				if err := regenPreview(ctx, wp); err != nil {
					log.Printf("RegeneratePreviews: %s: %v", wp.LinkName, err)
					errCount.Add(1)
					// Limit failed array to prevent memory exhaustion
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
	// loadLocalImage returns nil img when canUseLosslessMode is true.
	// In that case we decode from the returned fileData bytes directly.
	img, _, fileData, err := loadLocalImage(ctx, wp.ImagePath)
	if err != nil {
		return err
	}
	if img == nil {
		// Lossless path: fileData holds the raw bytes — decode for thumbnail.
		if len(fileData) == 0 {
			return nil // nothing to do
		}
		img, _, err = image.Decode(bytes.NewReader(fileData))
		if err != nil {
			return err
		}
	}
	previewPath := filepath.Join("static", "images", "previews", wp.LinkName+".webp")
	thumb := thumbnail(img, config.ThumbnailMaxWidth, config.ThumbnailMaxHeight)
	if err := saveImage(thumb, "webp", previewPath); err != nil {
		return err
	}
	wp.PreviewPath = previewPath
	wp.Preview = "/static/images/previews/" + wp.LinkName + ".webp"
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
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				log.Printf("cleanStalePreviewFiles: remove %s: %v", path, removeErr)
			}
		}
	}
}
