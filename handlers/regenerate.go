package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"lanpaper/config"
	"lanpaper/storage"
	"lanpaper/utils"
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

var regenerating atomic.Bool

// Regeneration is intentionally serialized: decoding several huge images at
// once (or starting several concurrent requests) can exhaust server memory.
func RegeneratePreviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !regenerating.CompareAndSwap(false, true) {
		http.Error(w, "Preview regeneration already running", http.StatusTooManyRequests)
		return
	}
	defer regenerating.Store(false)
	// A large library can take longer than the default write timeout.
	extendDeadline(w, config.RegenerateTimeout*time.Second)

	wallpapers := storage.Global.GetAll()
	result := RegeneratePreviewsResult{Total: len(wallpapers)}
	for _, snap := range wallpapers {
		if r.Context().Err() != nil {
			return
		}
		if !snap.HasImage || isVideo(snap.MIMEType) {
			result.Skipped++
			continue
		}
		// Use the same per-link lock as upload/rename/delete. The snapshot
		// may be stale by the time this job starts; read the current record.
		unlock := storage.LockLinks(snap.LinkName)
		wp, exists := storage.Global.Get(snap.LinkName)
		if !exists || !wp.HasImage || isVideo(wp.MIMEType) {
			result.Skipped++
			unlock()
			continue
		}
		err := regenPreview(wp)
		unlock()
		if err != nil {
			log.Printf("RegeneratePreviews: %s: %v", snap.LinkName, err)
			result.Errors++
			if len(result.Failed) < maxFailedItems {
				result.Failed = append(result.Failed, snap.LinkName)
			} else if len(result.Failed) == maxFailedItems {
				result.Failed = append(result.Failed, "...and more")
			}
		} else {
			result.OK++
		}
	}
	cleanStalePreviewFiles()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func regenPreview(wp *storage.Wallpaper) error {
	f, err := storage.OpenMedia(wp.ImagePath)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	ext, err := inspectMediaFile(f, wp.ImagePath, fi.Size(), int64(config.Current.MaxUploadMB)<<20)
	if err != nil {
		return err
	}
	img, release, err := decodeImage(f, ext)
	if err != nil {
		return err
	}
	defer release()
	if err := os.MkdirAll(config.PreviewDir, config.DataDirPerm); err != nil {
		return err
	}
	stage, err := stagePath(config.PreviewDir, "webp")
	if err != nil {
		return err
	}
	defer os.Remove(stage)
	if err := savePreview(img, stage); err != nil {
		return err
	}
	previewPath := storage.PreviewFilePath(wp.LinkName)
	pub, err := publishStaged(stage, previewPath, int64(config.Current.MaxUploadMB)<<20)
	if err != nil {
		return err
	}
	_, err = storage.Global.Update(wp.LinkName, func(current *storage.Wallpaper) error {
		if !current.HasImage || current.Version != wp.Version {
			return errors.New("media changed during preview generation")
		}
		current.PreviewPath = previewPath
		current.Preview = "/api/preview/" + wp.LinkName
		return nil
	})
	if err != nil {
		pub.rollback()
		return err
	}
	pub.finish()
	if wp.PreviewPath != "" && wp.PreviewPath != previewPath {
		removeFiles("", wp.PreviewPath)
	}
	return nil
}

// Remove orphan previews only for valid link names. Staging/backup files are
// owned by active uploads and must never be deleted by maintenance.
func cleanStalePreviewFiles() {
	for _, dir := range []string{config.PreviewDir, filepath.Join(config.LegacyMedia, "previews")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".webp" {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".webp")
			if !utils.IsValidLinkName(name) {
				continue
			}
			unlock := storage.LockLinks(name)
			if _, exists := storage.Global.Get(name); !exists {
				if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !os.IsNotExist(err) {
					log.Printf("Could not remove orphan preview %s: %v", entry.Name(), err)
				}
			}
			unlock()
		}
	}
}
