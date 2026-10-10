// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lanpaper/config"
	"lanpaper/storage"
)

// historyResponse is the body of GET /api/link/{name}/history. Live describes
// the file the URL serves right now in the same shape as an archived version,
// so a client can render one list without special cases.
type historyResponse struct {
	LinkName       string                 `json:"linkName"`
	CurrentVersion uint64                 `json:"currentVersion"`
	Live           storage.HistoryEntry   `json:"live"`
	History        []storage.HistoryEntry `json:"history"`
	Limit          int                    `json:"limit"`
	Bytes          int64                  `json:"bytes"`
}

// LinkHistory handles GET /api/link/{name}/history.
func (s *LibraryService) LinkHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name, ok := linkNameFromSubPath(r.URL.Path, "/history")
	if !ok {
		http.Error(w, "Invalid or missing link name", http.StatusBadRequest)
		return
	}
	wp, exists := s.Store.Get(name)
	if !exists {
		http.Error(w, "Link not found", http.StatusNotFound)
		return
	}
	resp := historyResponse{
		LinkName:       name,
		CurrentVersion: liveVersion(wp),
		History:        wp.History,
		Limit:          config.Current.History.Limit,
		Bytes:          storage.HistoryBytes(wp.History),
	}
	if resp.History == nil {
		resp.History = []storage.HistoryEntry{}
	}
	if wp.HasImage {
		resp.Live = storage.HistoryEntry{
			Version:   liveVersion(wp),
			Ext:       wp.MIMEType,
			SizeBytes: wp.SizeBytes,
			ModTime:   wp.ModTime,
			SavedAt:   wp.ModTime,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// linkNameFromSubPath extracts the link name from /api/link/{name}{suffix}.
// The sub-resource routes are dispatched by suffix in main.go, so the name comes
// from the path and is validated before anything is built from it.
func linkNameFromSubPath(path, suffix string) (string, bool) {
	name := strings.TrimPrefix(path, "/api/link/")
	name = strings.TrimSuffix(name, suffix)
	name = strings.Trim(name, "/")
	if name == "" || strings.Contains(name, "/") {
		return "", false
	}
	return name, isValidLinkName(name)
}

// rollbackRequest is the body of POST /api/link/{name}/rollback.
type rollbackRequest struct {
	Version uint64 `json:"version"`
}

// errVersionFileMissing means the metadata lists a version whose file is gone.
var errVersionFileMissing = errors.New("archived file is missing")

// RollbackLink handles POST /api/link/{name}/rollback and restores an archived
// version as the file the URL serves. The version that was live is archived by
// the same operation, so a rollback is itself reversible.
func (s *LibraryService) RollbackLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name, ok := linkNameFromSubPath(r.URL.Path, "/rollback")
	if !ok {
		http.Error(w, "Invalid or missing link name", http.StatusBadRequest)
		return
	}
	var req rollbackRequest
	if !decodeLinkJSON(w, r, &req) {
		return
	}
	if req.Version == 0 {
		http.Error(w, "Invalid version", http.StatusBadRequest)
		return
	}
	// Restoring an image re-encodes its thumbnail, which needs more than the
	// default write timeout for a large file.
	extendDeadline(w, time.Duration(config.UploadBaseTimeout)*time.Second)

	unlock := storage.LockLinks(name)
	defer unlock()
	wp, exists := s.Store.Get(name)
	if !exists {
		http.Error(w, "Link not found", http.StatusNotFound)
		return
	}
	entry, found := storage.FindHistory(wp.History, req.Version)
	if !found {
		http.Error(w, "Version not found", http.StatusNotFound)
		return
	}
	updated, err := s.rollbackToVersion(wp, entry)
	if err != nil {
		if errors.Is(err, errVersionFileMissing) {
			http.Error(w, "Version not found", http.StatusNotFound)
			return
		}
		log.Printf("Rollback of %s to version %d failed: %v", name, req.Version, err)
		writeStoreError(w, err)
		return
	}
	log.Printf("Rolled back %s to version %d (%s)", name, req.Version, entry.Ext)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toResponse(updated))
}

// rollbackToVersion swaps an archived file with the live one and records both
// moves in one metadata commit. The caller holds the link lock.
func (s *LibraryService) rollbackToVersion(wp *storage.Wallpaper, entry storage.HistoryEntry) (*storage.Wallpaper, error) {
	if !isValidLinkName(wp.LinkName) || !validStoredExt(entry.Ext) {
		return nil, errors.New("invalid rollback target")
	}
	src := storage.HistoryPath(wp.LinkName, entry.Version, entry.Ext)
	fi, err := os.Lstat(src)
	if os.IsNotExist(err) {
		return nil, errVersionFileMissing
	}
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("archived media is not a regular file")
	}

	liveExt := wp.MIMEType
	livePath := wp.ImagePath
	if livePath == "" {
		livePath = storage.MediaPath(wp.LinkName, liveExt)
	}
	version := liveVersion(wp)
	dst := storage.MediaPath(wp.LinkName, entry.Ext)

	// 1. Move the live file aside. Staging it as a sibling keeps every rename
	//    inside one directory, so no step can fail on a device boundary.
	stash := ""
	if wp.HasImage && validStoredExt(liveExt) {
		stash, err = stagePath(filepath.Dir(livePath), liveExt)
		if err != nil {
			return nil, err
		}
		defer func() {
			if stash != "" {
				_ = os.Remove(stash)
			}
		}()
		if err := os.Rename(livePath, stash); err != nil {
			stash = ""
			return nil, err
		}
	}
	restore := func() {
		if stash == "" {
			return
		}
		if err := os.Rename(stash, livePath); err != nil {
			log.Printf("Critical: could not restore %s: %v", livePath, err)
		}
	}

	// 2. Put the archived version where the URL looks for it.
	if err := os.MkdirAll(filepath.Dir(dst), config.DataDirPerm); err != nil {
		restore()
		return nil, err
	}
	if err := os.Rename(src, dst); err != nil {
		restore()
		return nil, err
	}

	// 3. Archive the file that was live, so this rollback can be rolled back.
	archived := false
	if stash != "" {
		archivePath := storage.HistoryPath(wp.LinkName, version, liveExt)
		if mkErr := os.MkdirAll(storage.HistoryDirPath(wp.LinkName), config.DataDirPerm); mkErr != nil {
			log.Printf("Rollback: cannot keep the previous version of %s: %v", wp.LinkName, mkErr)
		} else if renErr := os.Rename(stash, archivePath); renErr != nil {
			log.Printf("Rollback: cannot keep the previous version of %s: %v", wp.LinkName, renErr)
		} else {
			stash = ""
			archived = true
		}
	}

	size, modTime := entry.SizeBytes, entry.ModTime
	if restored, statErr := os.Lstat(dst); statErr == nil {
		size = restored.Size()
		modTime = restored.ModTime().Unix()
	}
	displaced := storage.HistoryEntry{
		Version:   version,
		Ext:       liveExt,
		SizeBytes: wp.SizeBytes,
		ModTime:   wp.ModTime,
		SavedAt:   time.Now().Unix(),
	}
	limit := config.Current.History.Limit

	var dropped []storage.HistoryEntry
	updated, err := s.Store.Update(wp.LinkName, func(cur *storage.Wallpaper) error {
		before := storage.HistoryBytes(cur.History)
		// The restored version leaves the archive...
		kept, _ := storage.WithoutHistoryVersion(cur.History, entry.Version)
		// ...and the file that was live joins it, if it could be kept.
		if archived && limit > 0 {
			var over []storage.HistoryEntry
			kept, over = storage.PrependHistory(kept, displaced, limit)
			dropped = over
		}
		cur.History = kept
		cur.CurrentVersion = version + 1
		cur.HasImage = true
		cur.MIMEType = entry.Ext
		cur.ImagePath = dst
		cur.SizeBytes = size
		cur.ModTime = modTime
		if config.IsVideoExt(entry.Ext) {
			// A video is its own preview; the old thumbnail must not survive.
			cur.PreviewPath = ""
			cur.Preview = ""
		} else {
			cur.PreviewPath = storage.PreviewFilePath(cur.LinkName)
			cur.Preview = "/api/preview/" + cur.LinkName
		}
		storage.NoteHistoryBytes(storage.HistoryBytes(kept) - before)
		return nil
	})
	if err != nil {
		// Undo the swap so the disk and the metadata agree again.
		if renameErr := os.Rename(dst, src); renameErr != nil {
			log.Printf("Critical: could not undo the rollback of %s: %v", wp.LinkName, renameErr)
		}
		if archived {
			if renameErr := os.Rename(storage.HistoryPath(wp.LinkName, version, liveExt), livePath); renameErr != nil {
				log.Printf("Critical: could not restore the live file of %s: %v", wp.LinkName, renameErr)
			}
			stash = ""
		} else {
			restore()
		}
		return nil, err
	}

	if config.IsVideoExt(entry.Ext) {
		removeFiles("", wp.PreviewPath)
	} else if regenErr := regenPreview(context.Background(), updated); regenErr != nil {
		// The URL already serves the restored file; only the panel thumbnail is
		// stale until the next regeneration, so this is logged, not returned.
		log.Printf("Rollback: preview regeneration failed for %s: %v", wp.LinkName, regenErr)
	} else if fresh, ok := s.Store.Get(wp.LinkName); ok {
		updated = fresh
	}
	storage.DeleteHistoryFiles(wp.LinkName, dropped)
	return updated, nil
}

// DeleteHistoryVersion handles DELETE /api/link/{name}/history/{version} and
// drops one archived version, freeing its disk space immediately.
func (s *LibraryService) DeleteHistoryVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name, version, ok := historyVersionFromPath(r.URL.Path)
	if !ok {
		http.Error(w, "Invalid link name or version", http.StatusBadRequest)
		return
	}
	unlock := storage.LockLinks(name)
	defer unlock()
	updated, err := s.RemoveHistory(name, version)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	log.Printf("Deleted archived version %d of %s", version, name)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toResponse(updated))
}

// historyVersionFromPath splits /api/link/{name}/history/{version}.
func historyVersionFromPath(path string) (string, uint64, bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/api/link/"), "/")
	if len(parts) != 3 || parts[1] != "history" {
		return "", 0, false
	}
	version, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil || version == 0 || !isValidLinkName(parts[0]) {
		return "", 0, false
	}
	return parts[0], version, true
}
