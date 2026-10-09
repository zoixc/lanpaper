// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"errors"
	"image"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// regenWorkers bounds how many previews are regenerated at once. Decoding
// is limited by the shared pixel budget (reserveDecodedPixels), so the workers
// cannot exhaust memory together with uploads. Measured on 2 CPUs with eight
// 12 MP JPEGs: 5.3 s with one worker, 2.6 s with two.
const regenWorkers = 2

// regenBudgetRetries and regenBudgetWait bound how long a job waits for the
// shared decode budget to free up, which concurrent uploads may hold.
const (
	regenBudgetRetries = 150
	regenBudgetWait    = 200 * time.Millisecond
)

type regenOutcome int

const (
	regenOK regenOutcome = iota
	regenSkipped
	regenFailed
)

// RegeneratePreviews rebuilds every preview from its stored image. Only one
// run may be active at a time; a second request gets 429.
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

	wallpapers := storage.Global.Snapshot()
	result := RegeneratePreviewsResult{Total: len(wallpapers)}
	var mu sync.Mutex
	jobs := make(chan *storage.Wallpaper)
	var wg sync.WaitGroup
	for i := 0; i < regenWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for snap := range jobs {
				outcome, err := regenerateLink(snap)
				if err != nil {
					log.Printf("RegeneratePreviews: %s: %v", snap.LinkName, err)
				}
				mu.Lock()
				switch outcome {
				case regenSkipped:
					result.Skipped++
				case regenOK:
					result.OK++
				case regenFailed:
					result.Errors++
					if len(result.Failed) < maxFailedItems {
						result.Failed = append(result.Failed, snap.LinkName)
					} else if len(result.Failed) == maxFailedItems {
						result.Failed = append(result.Failed, "...and more")
					}
				}
				mu.Unlock()
			}
		}()
	}
feed:
	for _, snap := range wallpapers {
		select {
		case jobs <- snap:
		case <-r.Context().Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	if r.Context().Err() != nil {
		return
	}
	cleanStalePreviewFiles()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// regenerateLink regenerates one link's preview under the link's lock. The
// snapshot may be stale by the time the job starts, so the current record is
// read again under the lock.
func regenerateLink(snap *storage.Wallpaper) (regenOutcome, error) {
	if !snap.HasImage || isVideo(snap.MIMEType) {
		return regenSkipped, nil
	}
	unlock := storage.LockLinks(snap.LinkName)
	defer unlock()
	wp, exists := storage.Global.Get(snap.LinkName)
	if !exists || !wp.HasImage || isVideo(wp.MIMEType) {
		return regenSkipped, nil
	}
	if err := regenPreview(wp); err != nil {
		return regenFailed, err
	}
	return regenOK, nil
}

// decodeForRegen decodes f, waiting while the shared decode budget is full.
// Only the transient "budget busy" error is retried; any other error (a bad
// image, for example) is returned at once.
func decodeForRegen(f *os.File, ext string) (image.Image, func(), error) {
	var lastErr error
	for attempt := 0; attempt < regenBudgetRetries; attempt++ {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, nil, err
		}
		img, release, err := decodeImage(f, ext)
		if err == nil {
			return img, release, nil
		}
		if !errors.Is(err, errImageBudgetBusy) {
			return nil, nil, err
		}
		lastErr = err
		time.Sleep(regenBudgetWait)
	}
	return nil, nil, lastErr
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
	img, release, err := decodeForRegen(f, ext)
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
			readOnly := false
			if _, exists := storage.Global.Get(name); !exists {
				if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !os.IsNotExist(err) {
					if storage.IsReadOnlyError(err) {
						readOnly = true
					} else {
						log.Printf("Could not remove orphan preview %s: %v", entry.Name(), err)
					}
				}
			}
			unlock()
			if readOnly {
				// Nothing in a read-only directory can be removed; say so once.
				log.Printf("Note: %s is read-only; orphan previews there are kept", dir)
				break
			}
		}
	}
}
