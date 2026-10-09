// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lanpaper/config"
	"lanpaper/middleware"
	"lanpaper/storage"
	"lanpaper/utils"
)

// Upload modes for the optional "mode" form field.
const (
	// uploadModeReplace swaps the file behind the URL. It is the default and the
	// only behaviour Lanpaper had before playlists existed.
	uploadModeReplace = "replace"
	// uploadModeAppend adds a playlist item behind the same URL, leaving the
	// live file (and therefore every existing embed) untouched.
	uploadModeAppend = "append"
)

func uploadMode(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", uploadModeReplace:
		return uploadModeReplace, true
	case uploadModeAppend:
		return uploadModeAppend, true
	}
	return "", false
}

// formFlag reads an optional boolean form field ("1", "true", "yes", "on").
// Anything else — including an absent field — is false, so existing clients
// keep the behaviour they had.
func formFlag(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	select {
	case uploadSem <- struct{}{}:
		defer func() { <-uploadSem }()
	default:
		// docs/API.md promises Retry-After on this 429: the slot frees up as
		// soon as one of the running uploads finishes.
		w.Header().Set("Retry-After", "5")
		http.Error(w, "Too many concurrent uploads", http.StatusTooManyRequests)
		return
	}

	maxBytes := int64(config.Current.MaxUploadMB) << 20
	maxRequest := maxBytes + (1 << 20) // multipart headers and form fields
	if r.ContentLength > maxRequest {
		http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
		return
	}
	// Large files over slow links need more than the default read timeout.
	extendDeadline(w, time.Duration(config.UploadBaseTimeout+maxRequest/config.UploadMinBytesPerSec)*time.Second)
	r.Body = http.MaxBytesReader(w, r.Body, maxRequest)
	// The body is streamed: the file goes straight to a temporary file, and a
	// publish key that asks to replace live media is refused before any of its
	// bytes are written (see readUploadForm).
	form, err := readUploadForm(r, maxBytes, func(f *uploadForm) error {
		if publishReplaceDenied(r, f.value("linkName"), f.value("mode")) {
			return errPublishReplace
		}
		return nil
	})
	if err != nil {
		uploadFormError(w, err)
		return
	}
	defer form.close()
	name := form.value("linkName")
	if !isValidLinkName(name) {
		http.Error(w, "Invalid link name", http.StatusBadRequest)
		return
	}
	mode, modeOK := uploadMode(form.value("mode"))
	if !modeOK {
		http.Error(w, "Invalid mode (expected replace or append)", http.StatusBadRequest)
		return
	}
	// autoCreate lets a single request create the link and push its first file,
	// which is what a webhook or a publish key needs. Without the flag an upload
	// to an unknown name is still rejected, so a typo cannot silently create a
	// link and an existing client's behaviour does not change.
	autoCreate := formFlag(form.value("autoCreate"))
	urlStr := form.value("url")
	if len(urlStr) > 2048 {
		http.Error(w, "URL too long", http.StatusBadRequest)
		return
	}
	unlock := storage.LockLinks(name)
	defer unlock()
	prev, exists := storage.Global.Get(name)
	// A publish key may add media — a new link, or a playlist item behind an
	// existing one — but never replace the live file of a link that already
	// has media: that would destroy content the URL is serving, and a leaked
	// key would then be able to deface every link. Admin uploads are unaffected.
	if exists && prev.HasImage && mode == uploadModeReplace &&
		middleware.PublisherFingerprint(r) != "" {
		http.Error(w, "Publish keys cannot replace existing media; use mode=append", http.StatusForbidden)
		return
	}
	createdLink := false
	if !exists {
		if !autoCreate {
			http.Error(w, "Link does not exist", http.StatusBadRequest)
			return
		}
		created, err := createLinkForUpload(name, form)
		if err != nil {
			if errors.Is(err, errInvalidLinkDefaults) {
				http.Error(w, "Invalid access level or category", http.StatusBadRequest)
				return
			}
			writeStoreError(w, err)
			return
		}
		prev = created
		createdLink = true
	}
	// The link is in the store before a single byte of media is decoded, so a
	// rejected file would otherwise leave an empty link in the panel. The link
	// lock is already held here, so the rollback must not take it again.
	linkCommitted := false
	if createdLink {
		defer func() {
			if !linkCommitted {
				rollbackCreatedLink(name)
			}
		}()
	}
	// A playlist item lives next to the live file, so the item count and the
	// target directory are settled before any media is downloaded or decoded.
	itemID := 0
	if mode == uploadModeAppend {
		if !prev.HasImage {
			http.Error(w, "Link has no media to add to", http.StatusBadRequest)
			return
		}
		if len(prev.Items) >= config.Current.PlaylistMax {
			http.Error(w, fmt.Sprintf("Playlist is full (max %d items)", config.Current.PlaylistMax), http.StatusConflict)
			return
		}
		itemID = storage.NextItemID(prev.Items)
		if err := os.MkdirAll(storage.ItemsDirPath(name), config.DataDirPerm); err != nil {
			http.Error(w, "Storage unavailable", http.StatusInternalServerError)
			return
		}
	}
	if err := os.MkdirAll(config.MediaDir, config.DataDirPerm); err != nil {
		http.Error(w, "Storage unavailable", http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(config.PreviewDir, config.DataDirPerm); err != nil {
		http.Error(w, "Storage unavailable", http.StatusInternalServerError)
		return
	}

	var source io.ReadSeeker
	var sourceSize int64
	var sourceName string
	var localPath bool
	if urlStr != "" {
		u, err := url.Parse(urlStr)
		if err != nil {
			http.Error(w, "Invalid URL", http.StatusBadRequest)
			return
		}
		if u.Scheme == "http" || u.Scheme == "https" {
			f, size, err := downloadToTemp(r.Context(), urlStr, maxBytes)
			if err != nil {
				log.Printf("Download rejected: %v", err)
				http.Error(w, "Failed to load media", http.StatusBadRequest)
				return
			}
			defer func() { f.Close(); os.Remove(f.Name()) }()
			source, sourceSize, sourceName = f, size, u.Path
		} else {
			if u.IsAbs() || strings.HasPrefix(urlStr, "//") || !utils.IsValidLocalPath(urlStr) ||
				!config.AllowedMediaExts[strings.ToLower(filepath.Ext(urlStr))] {
				http.Error(w, "Invalid local media path", http.StatusBadRequest)
				return
			}
			f, err := utils.OpenExternalFile(config.Current.ExternalImageDir, urlStr)
			if err != nil {
				writeExternalFileError(w, err)
				return
			}
			defer f.Close()
			fi, err := f.Stat()
			if err != nil {
				http.Error(w, "File unavailable", http.StatusBadRequest)
				return
			}
			source, sourceSize, sourceName, localPath = f, fi.Size(), urlStr, true
		}
	} else {
		if form.file == nil {
			http.Error(w, "No file provided", http.StatusBadRequest)
			return
		}
		fi, err := form.file.Stat()
		if err != nil {
			http.Error(w, "No file provided", http.StatusBadRequest)
			return
		}
		source, sourceSize, sourceName = form.file, fi.Size(), form.fileName
	}

	ext, err := inspectMediaFile(source, sourceName, sourceSize, maxBytes)
	if err != nil {
		if errors.Is(err, errMediaTooLarge) {
			http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
		} else {
			log.Printf("Rejected media: %v", err)
			http.Error(w, "Invalid or unsupported media file", http.StatusBadRequest)
		}
		return
	}
	if localPath {
		nameExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(sourceName)), ".")
		if isVideo(ext) != isVideo(nameExt) {
			http.Error(w, "Media type does not match local file", http.StatusBadRequest)
			return
		}
	}

	video := isVideo(ext)
	lossless := !video && canUseLosslessMode()
	saveExt := storedExt(ext, lossless)
	// Staging happens in the directory the file will end up in, so publishing is
	// always a rename within one directory (no cross-device surprise).
	stageDir := config.MediaDir
	if mode == uploadModeAppend {
		stageDir = storage.ItemsDirPath(name)
	}
	imageStage, err := stagePath(stageDir, saveExt)
	if err != nil {
		http.Error(w, "Storage unavailable", http.StatusInternalServerError)
		return
	}
	defer os.Remove(imageStage)
	previewStage := ""
	if video {
		if err := copyFile(imageStage, source, maxBytes); err != nil {
			writeUploadError(w, err)
			return
		}
	} else {
		if clientGone(r) {
			return
		}
		// Even in lossless mode, fully decode before storing: malformed files
		// must not be published just because their first 512 bytes look valid.
		img, release, err := decodeImage(source, ext)
		if errors.Is(err, errImageBudgetBusy) {
			w.Header().Set("Retry-After", "5")
			http.Error(w, "Image processing busy; retry shortly", http.StatusTooManyRequests)
			return
		}
		if err != nil {
			log.Printf("Rejected image: %v", err)
			http.Error(w, "Invalid image or dimensions too large", http.StatusBadRequest)
			return
		}
		defer release()
		// Decoding a large image takes seconds; the client may be long gone.
		if clientGone(r) {
			return
		}
		if !lossless {
			img = scaleImage(img, config.Current.Compression.Scale)
		}
		// Playlist items get no thumbnail: the panel shows one preview per link,
		// so encoding a WebP for every item would multiply upload CPU and disk
		// use for metadata nobody looks at.
		if mode == uploadModeReplace {
			previewStage, err = stagePath(config.PreviewDir, "webp")
			if err != nil {
				writeUploadError(w, err)
				return
			}
			defer os.Remove(previewStage)
			if err := savePreview(img, previewStage); err != nil {
				writeUploadError(w, err)
				return
			}
		}
		if lossless {
			if _, err := source.Seek(0, io.SeekStart); err != nil {
				writeUploadError(w, err)
				return
			}
			err = copyFile(imageStage, source, maxBytes)
		} else {
			err = saveImage(img, saveExt, imageStage, config.Current.Compression.Quality)
		}
		if err != nil {
			writeUploadError(w, err)
			return
		}
	}

	fi, err := os.Stat(imageStage)
	if err != nil {
		writeUploadError(w, err)
		return
	}
	imagePath := storage.MediaPath(name, saveExt)
	if mode == uploadModeAppend {
		imagePath = storage.ItemPath(name, itemID, saveExt)
	}
	previewPath := ""
	previewURL := ""
	if !video && mode == uploadModeReplace {
		previewPath = storage.PreviewFilePath(name)
		previewURL = "/api/preview/" + name
	}
	txn := &uploadTransaction{}
	if err := txn.staged(); err != nil {
		writeUploadError(w, err)
		return
	}
	defer txn.rollback()
	imagePub, err := txn.publish(imageStage, imagePath, maxBytes)
	if err != nil {
		writeUploadError(w, err)
		return
	}

	if mode == uploadModeAppend {
		// A playlist item never touches the live file, its preview or its
		// history: the URL keeps serving exactly what it served before, and the
		// rotation settings decide when the new file joins in.
		item := storage.PlaylistItem{
			ID:        itemID,
			Ext:       saveExt,
			SizeBytes: fi.Size(),
			ModTime:   fi.ModTime().Unix(),
			AddedAt:   time.Now().Unix(),
		}
		appended, err := storage.Global.Update(name, func(wp *storage.Wallpaper) error {
			wp.Items = storage.AppendItem(wp.Items, item)
			return nil
		})
		if err != nil {
			writeStoreError(w, err)
			return
		}
		if err := txn.commit(); err != nil {
			writeUploadError(w, err)
			return
		}
		_ = txn.finalize()
		linkCommitted = true
		log.Printf("Appended playlist item #%d to %s (%s, %d KB)", itemID, name, saveExt, fi.Size()/1024)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(toResponse(appended))
		return
	}

	if previewStage != "" {
		_, err = txn.publish(previewStage, previewPath, maxBytes)
		if err != nil {
			writeUploadError(w, err)
			return
		}
	}

	newWP := *prev
	newWP.ID, newWP.LinkName = name, name
	newWP.ImageURL = "/" + name
	newWP.Preview = previewURL
	newWP.HasImage = true
	newWP.MIMEType = saveExt
	newWP.SizeBytes = fi.Size()
	newWP.ModTime = fi.ModTime().Unix()
	newWP.ImagePath = imagePath
	newWP.PreviewPath = previewPath
	if newWP.CreatedAt == 0 {
		newWP.CreatedAt = fi.ModTime().Unix()
	}
	updated, err := storage.Global.Update(name, func(wp *storage.Wallpaper) error {
		*wp = newWP
		return nil
	})
	if err != nil {
		writeUploadError(w, err)
		return
	}
	// Version history: the backup publishStaged kept IS the file this upload
	// replaced, so archiving it costs one rename — no extra read, no extra write
	// and no extra disk block for as long as the link stays. It runs after the
	// metadata commit and is best effort: a failed archive must never fail an
	// upload that already succeeded.
	if config.Current.History.Limit > 0 && imagePub.backup != "" {
		archived, err := storage.ArchiveReplaced(prev, imagePub.backup, prev.MIMEType, config.Current.History.Limit)
		if err != nil {
			log.Printf("Upload: could not archive the previous version of %s: %v", name, err)
		} else if archived != nil {
			updated = archived
		}
	}
	// The upload is published and stored: from here on the link is a real one
	// and the deferred rollback must not run.
	if err := txn.commit(); err != nil {
		writeUploadError(w, err)
		return
	}
	_ = txn.finalize()
	linkCommitted = true
	// Cleanup of the previous extension/preview only after commit. In
	// particular, a video replacing an image must not leave a stale preview.
	if prev.HasImage {
		if prev.ImagePath != imagePath {
			removeFiles(prev.ImagePath, "")
		}
		if prev.PreviewPath != previewPath {
			removeFiles("", prev.PreviewPath)
		}
	}
	storage.SchedulePrune(config.Current.MaxImages)
	if fingerprint := middleware.PublisherFingerprint(r); fingerprint != "" {
		log.Printf("Uploaded: %s (%s, %d KB) with publish key %s", name, saveExt, fi.Size()/1024, fingerprint)
	} else {
		log.Printf("Uploaded: %s (%s, %d KB)", name, saveExt, fi.Size()/1024)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toResponse(updated))
}

// clientGone reports whether the client cancelled the request while the upload
// was being processed. Decoding and encoding would otherwise run to completion
// for a response nobody is waiting for, holding a slot of the decode budget.
func clientGone(r *http.Request) bool {
	return r.Context().Err() != nil
}

// errInvalidLinkDefaults rejects an auto-created link whose requested access
// level or category is not on the allow-list.
var errInvalidLinkDefaults = errors.New("invalid access level or category")

// createLinkForUpload creates the link an upload targets when the client set
// autoCreate, so a webhook or a publish key can push media in one request
// instead of create-then-upload. The caller holds the link lock.
func createLinkForUpload(name string, form *uploadForm) (*storage.Wallpaper, error) {
	rawLevel := strings.TrimSpace(form.value("accessLevel"))
	if rawLevel != "" && !isValidAccessLevel(rawLevel) {
		return nil, errInvalidLinkDefaults
	}
	level := storage.NormalizeAccessLevel(rawLevel)
	category := strings.TrimSpace(form.value("category"))
	if category != "" && !isValidCategory(category) {
		return nil, errInvalidLinkDefaults
	}
	if category == "" {
		category = "other"
	}
	wp := &storage.Wallpaper{
		ID: name, LinkName: name, Category: category, ImageURL: "/" + name,
		CreatedAt: time.Now().Unix(), AccessLevel: level,
	}
	if level == config.AccessToken {
		wp.AccessToken = generateAccessToken()
	}
	if err := storage.Global.Create(wp); err != nil {
		return nil, err
	}
	created, exists := storage.Global.Get(name)
	if !exists {
		// The store accepted the entry but cannot read it back: drop it rather
		// than fail the upload with a link nobody asked for left behind.
		_, _ = storage.Global.DeleteEntry(name)
		return nil, storage.ErrNotFound
	}
	return created, nil
}

// rollbackCreatedLink deletes a link that autoCreate had to create for an
// upload that then failed. The caller holds the link lock; taking it again here
// would deadlock, so the cleanup is limited to the store, the per-link
// directories and the counters of a link that never served a byte.
func rollbackCreatedLink(name string) {
	if _, err := storage.Global.DeleteEntry(name); err != nil {
		log.Printf("Upload: could not roll back the auto-created link %s: %v", name, err)
		return
	}
	storage.RemoveLinkExtraDirs(name)
	storage.ForgetStats(name)
}

func writeUploadError(w http.ResponseWriter, err error) {
	log.Printf("Upload failed: %v", err)
	if errors.Is(err, errMediaTooLarge) {
		http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
	} else {
		http.Error(w, "Failed to save media", http.StatusInternalServerError)
	}
}
