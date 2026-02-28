package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

// RegeneratePreviews re-generates WebP thumbnails for every stored image entry.
// Only POST is accepted. It runs up to 4 workers concurrently.
func RegeneratePreviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wallpapers := storage.Global.All()

	total := len(wallpapers)
	skipped := 0

	// Collect jobs: only images (not video, not empty)
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

	// Capture context before goroutines so we can pass it in
	ctx := r.Context()

	const workers = 4
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				wp := j.wp
				if err := regenPreview(ctx, wp); err != nil {
					log.Printf("RegeneratePreviews: %s: %v", wp.LinkName, err)
					errCount.Add(1)
					failedMu.Lock()
					failed = append(failed, wp.LinkName)
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
	img, _, _, err := loadLocalImage(ctx, wp.ImagePath)
	if err != nil {
		return err
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

// cleanStalePreviewFiles removes .webp files in previews/ that have no matching storage entry.
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
