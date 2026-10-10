// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"lanpaper/config"
	"lanpaper/storage"
)

func decodeLinkJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	if err := decoder.Decode(dst); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		http.Error(w, "Link not found", http.StatusNotFound)
	case errors.Is(err, storage.ErrExists):
		http.Error(w, "Link name already taken", http.StatusConflict)
	default:
		log.Printf("Error persisting link: %v", err)
		http.Error(w, "Storage unavailable", http.StatusInternalServerError)
	}
}

// Link handles POST /api/link, PATCH/DELETE /api/link/{name}.
func Link(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if r.URL.Path != "/api/link" && r.URL.Path != "/api/link/" {
			// A POST to /api/link/{name} is a method mismatch, not a missing
			// route: that path exists and answers PATCH and DELETE.
			if _, ok := linkNameFromPath(r.URL.Path); ok {
				w.Header().Set("Allow", "PATCH, DELETE")
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			http.NotFound(w, r)
			return
		}
		var req struct {
			LinkName    string `json:"linkName"`
			Category    string `json:"category"`
			AccessLevel string `json:"accessLevel"`
		}
		if !decodeLinkJSON(w, r, &req) {
			return
		}
		if !isValidLinkName(req.LinkName) {
			http.Error(w, "Invalid link name", http.StatusBadRequest)
			return
		}
		if req.Category != "" && !isValidCategory(req.Category) {
			http.Error(w, "Invalid category", http.StatusBadRequest)
			return
		}
		if req.AccessLevel != "" && !isValidAccessLevel(req.AccessLevel) {
			http.Error(w, "Invalid access level", http.StatusBadRequest)
			return
		}
		level := storage.NormalizeAccessLevel(req.AccessLevel)
		cat := req.Category
		if cat == "" {
			cat = "other"
		}
		wp := &storage.Wallpaper{
			ID: req.LinkName, LinkName: req.LinkName, Category: cat,
			CreatedAt: time.Now().Unix(), AccessLevel: level,
		}
		if level == config.AccessToken {
			wp.AccessToken = generateAccessToken()
		}
		unlock := storage.LockLinks(req.LinkName)
		defer unlock()
		if err := storage.Global.Create(wp); err != nil {
			writeStoreError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(toResponse(wp))

	case http.MethodPatch:
		name, ok := linkNameFromPath(r.URL.Path)
		if !ok {
			http.Error(w, "Invalid or missing link name", http.StatusBadRequest)
			return
		}
		var req struct {
			NewLinkName *string      `json:"newLinkName"`
			Category    *string      `json:"category"`
			AccessLevel *string      `json:"accessLevel"`
			RotateToken bool         `json:"rotateToken"`
			Rotate      *rotatePatch `json:"rotate"`
			RemoveItem  *int         `json:"removeItem"`
		}
		if !decodeLinkJSON(w, r, &req) {
			return
		}
		if req.NewLinkName != nil {
			if !isValidLinkName(*req.NewLinkName) {
				http.Error(w, "Invalid new link name", http.StatusBadRequest)
				return
			}
			if *req.NewLinkName == name {
				wp, exists := storage.Global.Get(name)
				if !exists {
					http.Error(w, "Link not found", http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(toResponse(wp))
				return
			}
			wp, err := renameLink(name, *req.NewLinkName)
			if err != nil {
				writeStoreError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(toResponse(wp))
			return
		}
		if req.Category != nil && *req.Category != "" && !isValidCategory(*req.Category) {
			http.Error(w, "Invalid category", http.StatusBadRequest)
			return
		}
		if req.AccessLevel != nil && !isValidAccessLevel(*req.AccessLevel) {
			http.Error(w, "Invalid access level", http.StatusBadRequest)
			return
		}
		// Playlist rotation and item removal are validated before anything is
		// locked, so a bad request costs no I/O.
		order := ""
		if req.Rotate != nil {
			if req.Rotate.Order != nil {
				order = strings.ToLower(strings.TrimSpace(*req.Rotate.Order))
				if order != config.RotateOrderSequential && order != config.RotateOrderRandom {
					http.Error(w, "Invalid rotation order", http.StatusBadRequest)
					return
				}
			}
			if req.Rotate.Interval != nil {
				interval := *req.Rotate.Interval
				if interval != 0 && (interval < config.MinRotateInterval || interval > config.MaxRotateInterval) {
					http.Error(w, fmt.Sprintf("Rotation interval must be %d-%d seconds (0 restores the default)",
						config.MinRotateInterval, config.MaxRotateInterval), http.StatusBadRequest)
					return
				}
			}
		}
		removeID := 0
		if req.RemoveItem != nil {
			removeID = *req.RemoveItem
			if removeID <= 0 {
				http.Error(w, "Invalid playlist item", http.StatusBadRequest)
				return
			}
		}
		unlock := storage.LockLinks(name)
		defer unlock()
		var removedItem storage.PlaylistItem
		wp, err := storage.Global.Update(name, func(wp *storage.Wallpaper) error {
			if req.Category != nil {
				wp.Category = *req.Category
				if wp.Category == "" {
					wp.Category = "other"
				}
			}
			if req.AccessLevel != nil {
				newLevel := storage.NormalizeAccessLevel(*req.AccessLevel)
				oldLevel := storage.NormalizeAccessLevel(wp.AccessLevel)
				if newLevel == config.AccessToken && (oldLevel != config.AccessToken || wp.AccessToken == "" || req.RotateToken) {
					wp.AccessToken = generateAccessToken()
				} else if newLevel != config.AccessToken {
					wp.AccessToken = ""
				}
				wp.AccessLevel = newLevel
			} else if req.RotateToken {
				if storage.NormalizeAccessLevel(wp.AccessLevel) != config.AccessToken {
					return errNotTokenLink
				}
				wp.AccessToken = generateAccessToken()
			}
			if req.Rotate != nil {
				// Merge into the stored settings: a PATCH that only toggles
				// "enabled" must not reset the interval or the order.
				var rotate storage.RotateConfig
				if wp.Rotate != nil {
					rotate = *wp.Rotate
				}
				if req.Rotate.Enabled != nil {
					rotate.Enabled = *req.Rotate.Enabled
				}
				if req.Rotate.Interval != nil {
					rotate.Interval = *req.Rotate.Interval
				}
				if order != "" {
					rotate.Order = order
				}
				wp.Rotate = storage.NormalizeRotatePtr(&rotate, len(wp.Items) > 0)
			}
			if req.RemoveItem != nil {
				var mutationErr error
				removedItem, mutationErr = defaultLibraryService().removePlaylistMetadata(wp, removeID)
				if mutationErr != nil {
					return mutationErr
				}
			}
			return nil
		})
		if errors.Is(err, errNotTokenLink) {
			http.Error(w, "Link is not token-protected", http.StatusBadRequest)
			return
		}
		if errors.Is(err, errItemNotFound) {
			http.Error(w, "Playlist item not found", http.StatusNotFound)
			return
		}
		if err != nil {
			writeStoreError(w, err)
			return
		}
		// The item file is removed only after the metadata commit, so a failed
		// save can never leave a listed item without its bytes.
		if removedItem.ID > 0 {
			if removeErr := defaultLibraryService().cleanupPlaylistItem(name, removedItem); removeErr != nil {
				log.Printf("Could not remove playlist item from %s: %v", name, removeErr)
			}
			log.Printf("Removed playlist item #%d from %s", removedItem.ID, name)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(toResponse(wp))

	case http.MethodDelete:
		name, ok := linkNameFromPath(r.URL.Path)
		if !ok {
			http.Error(w, "Invalid or missing link name", http.StatusBadRequest)
			return
		}
		unlock := storage.LockLinks(name)
		defer unlock()
		wp, err := storage.Global.DeleteEntry(name)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		// First commit the deletion. If file cleanup fails, the media remains
		// unreachable through HTTP and the error is logged.
		if wp.HasImage {
			removeFiles(wp.ImagePath, wp.PreviewPath)
			if wp.MIMEType != "" {
				removeFiles(filepath.Join(config.LegacyMedia, name+"."+wp.MIMEType),
					filepath.Join(config.LegacyMedia, "previews", name+".webp"))
			}
		}
		// Archived versions, playlist items and the access counters belong to the
		// link and must not outlive it.
		storage.RemoveLinkExtraDirs(name)
		storage.ForgetStats(name)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

var errNotTokenLink = errors.New("not a token-protected link")

// errItemNotFound is returned by a PATCH that removes a playlist item id the
// link does not have.
var errItemNotFound = errors.New("playlist item not found")

// rotatePatch is the PATCH body for playlist rotation. Every field is optional
// and a missing field keeps the value the link already has, which is why they
// are pointers: false, 0 and "" are meaningful values, not "unset".
type rotatePatch struct {
	Enabled  *bool   `json:"enabled"`
	Interval *int    `json:"interval"`
	Order    *string `json:"order"`
}

// renameLink holds both names for the whole operation. A failed metadata save
// rolls file renames back instead of leaving the old URL pointing to nothing.
func renameLink(oldName, newName string) (*storage.Wallpaper, error) {
	unlock := storage.LockLinks(oldName, newName)
	defer unlock()
	wp, exists := storage.Global.Get(oldName)
	if !exists {
		return nil, storage.ErrNotFound
	}
	if _, exists := storage.Global.Get(newName); exists {
		return nil, storage.ErrExists
	}
	type movedFile struct{ from, to string }
	var moved []movedFile
	rollback := func() {
		for _, m := range slices.Backward(moved) {
			if err := os.Rename(m.to, m.from); err != nil {
				log.Printf("Error rolling back rename of %s: %v", m.from, err)
			}
		}
	}
	move := func(from, to string, required bool) error {
		if from == "" || from == to {
			return nil
		}
		fi, err := os.Lstat(from)
		if os.IsNotExist(err) && !required {
			return nil
		}
		if err != nil {
			return fmt.Errorf("source file: %w", err)
		}
		if !fi.Mode().IsRegular() {
			return errors.New("media is not a regular file")
		}
		if _, err := os.Lstat(to); !os.IsNotExist(err) {
			return fmt.Errorf("destination file unavailable: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(to), config.DataDirPerm); err != nil {
			return err
		}
		if err := os.Rename(from, to); err != nil {
			return err
		}
		moved = append(moved, movedFile{from, to})
		return nil
	}
	if wp.HasImage {
		if err := move(wp.ImagePath, storage.MediaPath(newName, wp.MIMEType), true); err != nil {
			rollback()
			return nil, err
		}
		if !isVideo(wp.MIMEType) {
			if err := move(wp.PreviewPath, storage.PreviewFilePath(newName), false); err != nil {
				rollback()
				return nil, err
			}
		}
	}
	// Archived versions and playlist items move as whole directories. Most links
	// have neither, and a missing directory is not an error.
	movedDirs, err := storage.MoveLinkExtraDirs(oldName, newName)
	if err != nil {
		storage.UndoMovedDirs(movedDirs)
		rollback()
		return nil, err
	}
	renamed, err := storage.Global.Rename(oldName, newName)
	if err != nil {
		storage.UndoMovedDirs(movedDirs)
		rollback()
		return nil, err
	}
	// Счётчики обращений переезжают вместе с адресом: иначе ссылка после
	// переименования показывала бы нулевую статистику, а под старым именем
	// осталась бы «статистика призрака».
	storage.RenameStats(oldName, newName)
	log.Printf("Renamed link: %s -> %s", oldName, newName)
	return renamed, nil
}

// TogglePin handles POST /api/link/{name}/pin.
func TogglePin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/pin") {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name, ok := linkNameFromPath(strings.TrimSuffix(r.URL.Path, "/pin"))
	if !ok {
		http.Error(w, "Invalid link name", http.StatusBadRequest)
		return
	}
	unlock := storage.LockLinks(name)
	defer unlock()
	wp, err := storage.Global.Update(name, func(wp *storage.Wallpaper) error {
		wp.IsPinned = !wp.IsPinned
		if wp.IsPinned {
			wp.PinnedAt = time.Now().Unix()
		} else {
			wp.PinnedAt = 0
		}
		return nil
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toResponse(wp))
}
