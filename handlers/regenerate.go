package handlers

import (
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
	Total    int      `json:"total"`
	OK       int      `json:"ok"`
	Skipped  int      `json:"skipped"` // videos or no-image entries
	Errors   int      `json:"errors"`
	Failed   []string `json:"failed,omitempty"`
}

// RegeneratePreviews re-generates WebP thumbnails for every stored image entry.
// Only POST is accepted. It runs up to 4 workers concurrently.
func RegeneratePreviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wallpapers := storage.Global.All()

	type job struct {
		wp *storage.Wallpaper
	}

	jobs := make(chan job, len(wallpapers))
	for _, wp := range wallpapers {
		if wp != nil && wp.HasImage && !isVideo(wp.MIMEType) {
			jobs <- job{wp: wp}
		}
	}
	close(jobs)

	var (
		total   = len(wallpapers)
		skipped int
		var okCount, errCount atomic.Int32
		failedMu sync.Mutex
		failed  []string
	)

	for _, wp := range wallpapers {
		if wp == nil || !wp.HasImage || isVideo(wp.MIMEType) {
			skipped++
		}
	}

	const workers = 4
	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				wp := j.wp
				// Load the original image from disk
				img, _, _, err := loadLocalImage(r.Context(), wp.ImagePath)
				if err != nil {
					log.Printf("RegeneratePreviews: load %s: %v", wp.ImagePath, err)
					errCount.Add(1)
					failedMu.Lock()
					failed = append(failed, wp.LinkName)
					failedMu.Unlock()
					continue
				}

				previewPath := filepath.Join("static", "images", "previews", wp.LinkName+".webp")
				thumb := thumbnail(img, config.ThumbnailMaxWidth, config.ThumbnailMaxHeight)
				if err := saveImage(thumb, "webp", previewPath); err != nil {
					log.Printf("RegeneratePreviews: save preview %s: %v", wp.LinkName, err)
					errCount.Add(1)
					failedMu.Lock()
					failed = append(failed, wp.LinkName)
					failedMu.Unlock()
					continue
				}

				// Update the stored preview path/URL
				wp.PreviewPath = previewPath
				wp.Preview = "/static/images/previews/" + wp.LinkName + ".webp"
				storage.Global.Set(wp.LinkName, wp)
				okCount.Add(1)
			}
		}()
	}

	wg.Wait()

	if err := storage.Global.Save(); err != nil {
		log.Printf("RegeneratePreviews: save storage: %v", err)
	}

	// Clean up stale preview files that no longer have a corresponding entry
	cleanStalePreviewFiles()

	result := RegeneratePreviewsResult{
		Total:   total,
		OK:      int(okCount.Load()),
		Skipped: skipped,
		Errors:  int(errCount.Load()),
		Failed:  failed,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("RegeneratePreviews: encode response: %v", err)
	}
}

func isVideo(mimeType string) bool {
	return mimeType == "mp4" || mimeType == "webm" ||
		mimeType == "video/mp4" || mimeType == "video/webm"
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
		extension := filepath.Ext(e.Name())
		if extension != ".webp" {
			continue
		}
		linkName := e.Name()[:len(e.Name())-len(extension)]
		if _, exists := storage.Global.Get(linkName); !exists {
			path := filepath.Join(previewDir, e.Name())
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				log.Printf("cleanStalePreviewFiles: remove %s: %v", path, removeErr)
			}
		}
	}
}
