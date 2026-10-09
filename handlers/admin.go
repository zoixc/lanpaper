// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"lanpaper/config"
	"lanpaper/storage"
	"lanpaper/utils"
)

const (
	DefaultPageSize = 50
	MaxPageSize     = 200
	// maxJSONBody limits request bodies on JSON endpoints (requests carry
	// nothing larger than a link name and a category).
	maxJSONBody = 64 << 10 // 64 KB
)

func Admin(w http.ResponseWriter, r *http.Request) {
	// Always revalidate: the panel must not run a stale UI after an upgrade.
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, "admin.html")
}

type WallpaperResponse struct {
	ID          string `json:"id"`
	LinkName    string `json:"linkName"`
	Category    string `json:"category"`
	HasImage    bool   `json:"hasImage"`
	ImageURL    string `json:"imageUrl"`
	Preview     string `json:"preview,omitempty"`
	MIMEType    string `json:"mimeType"`
	SizeBytes   int64  `json:"sizeBytes"`
	ModTime     int64  `json:"modTime"`
	CreatedAt   int64  `json:"createdAt"`
	Pinned      bool   `json:"pinned"`
	PinnedAt    int64  `json:"pinnedAt,omitempty"`
	AccessLevel string `json:"accessLevel"`
	// AccessToken is only included for token-level links so the admin can copy it.
	AccessToken string `json:"accessToken,omitempty"`

	// CurrentVersion numbers the file the URL serves. Archived versions are
	// addressable as /{name}?v=N and restorable via /api/link/{name}/rollback.
	CurrentVersion uint64 `json:"currentVersion,omitempty"`
	// History lists the archived versions, newest first.
	History []storage.HistoryEntry `json:"history,omitempty"`
	// Items are the playlist entries behind the same URL. The live file is not
	// listed; it is position 0.
	Items []storage.PlaylistItem `json:"items,omitempty"`
	// Rotate is present only for links that have a playlist or rotation settings,
	// so links that use neither serialize exactly as before.
	Rotate *storage.RotateConfig `json:"rotate,omitempty"`
	// Stats counts deliveries since the process started and is absent until the
	// link has been requested at least once.
	Stats *storage.AccessStats `json:"stats,omitempty"`
}

type PaginatedResponse struct {
	Data       []WallpaperResponse `json:"data"`
	Total      int                 `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"pageSize"`
	TotalPages int                 `json:"totalPages"`
}

// Wallpapers handles GET /api/wallpapers.
func Wallpapers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read-only listing: the records are not copied, only the slice of pointers
	// (filters and sorting below reorder that slice, never the records).
	wallpapers := storage.Global.Snapshot()
	q := r.URL.Query()

	if cat := q.Get("category"); cat != "" {
		out := wallpapers[:0]
		for _, wp := range wallpapers {
			if strings.EqualFold(wp.Category, cat) {
				out = append(out, wp)
			}
		}
		wallpapers = out
	}
	if hasImg := q.Get("has_image"); hasImg != "" {
		want, ok := queryFlag(hasImg)
		if !ok {
			http.Error(w, "Invalid has_image", http.StatusBadRequest)
			return
		}
		out := wallpapers[:0]
		for _, wp := range wallpapers {
			if wp.HasImage == want {
				out = append(out, wp)
			}
		}
		wallpapers = out
	}
	if sf := q.Get("sort"); sf != "" {
		// Anything but the two documented values is a client error: silently
		// falling back to "created" hid typos and sorted by the wrong field.
		switch strings.ToLower(sf) {
		case "created", "updated":
			sortWallpapers(wallpapers, sf, q.Get("order") != "asc")
		default:
			http.Error(w, "Invalid sort", http.StatusBadRequest)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")

	if pageStr := q.Get("page"); pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			http.Error(w, "Invalid page number", http.StatusBadRequest)
			return
		}
		pageSize := clampPageSize(q.Get("page_size"))
		total := len(wallpapers)
		totalPages := max(1, (total+pageSize-1)/pageSize)
		start, end := pageWindow(page, pageSize, total)
		if err := json.NewEncoder(w).Encode(PaginatedResponse{
			Data: toResponses(wallpapers[start:end]), Total: total,
			Page: page, PageSize: pageSize, TotalPages: totalPages,
		}); err != nil {
			log.Printf("Error encoding paginated response: %v", err)
		}
		return
	}

	if err := json.NewEncoder(w).Encode(toResponses(wallpapers)); err != nil {
		log.Printf("Error encoding wallpapers response: %v", err)
	}
}

// queryFlag reads a boolean query parameter. Unlike formFlag, where a wrong
// value must not break an upload, a filter that cannot be parsed is a client
// error: treating has_image=1 as false returned exactly the links the caller
// asked to exclude.
func queryFlag(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	}
	return false, false
}

func clampPageSize(s string) int {
	if ps, err := strconv.Atoi(s); err == nil && ps > 0 {
		if ps > MaxPageSize {
			return MaxPageSize
		}
		return ps
	}
	return DefaultPageSize
}

func pageWindow(page, pageSize, total int) (start, end int) {
	// Check before multiplying: a very large page must return an empty
	// slice, not overflow to a negative index and panic.
	if page-1 > total/pageSize {
		return total, total
	}
	start = (page - 1) * pageSize
	end = min(start+pageSize, total)
	return
}

func toResponses(wps []*storage.Wallpaper) []WallpaperResponse {
	out := make([]WallpaperResponse, len(wps))
	for i, wp := range wps {
		out[i] = toResponse(wp)
	}
	return out
}

// sortWallpapers sorts by the requested field while always keeping pinned
// entries at the top, consistent with the default storage ordering.
func sortWallpapers(wps []*storage.Wallpaper, field string, desc bool) {
	sort.SliceStable(wps, func(i, j int) bool {
		// Pinned entries always sort first regardless of the requested field.
		if wps[i].IsPinned != wps[j].IsPinned {
			return wps[i].IsPinned
		}
		var vi, vj int64
		if field == "updated" {
			vi, vj = wps[i].ModTime, wps[j].ModTime
		} else {
			vi, vj = wps[i].CreatedAt, wps[j].CreatedAt
		}
		if desc {
			return vi > vj
		}
		return vi < vj
	})
}

func inferCategory(wp *storage.Wallpaper) string {
	if wp.Category != "" {
		return wp.Category
	}
	if config.IsVideoExt(wp.MIMEType) {
		return "video"
	}
	if wp.HasImage {
		return "image"
	}
	return "other"
}

// toResponse builds the admin view of a record. It only reads wp: records are
// shared with concurrent readers (see storage.Store.Snapshot), so it must not
// write to them.
func toResponse(wp *storage.Wallpaper) WallpaperResponse {
	accessLevel := storage.NormalizeAccessLevel(wp.AccessLevel)
	resp := WallpaperResponse{
		ID:          wp.ID,
		LinkName:    wp.LinkName,
		Category:    inferCategory(wp),
		HasImage:    wp.HasImage,
		ImageURL:    wp.ImageURL,
		Preview:     wp.Preview,
		MIMEType:    wp.MIMEType,
		SizeBytes:   wp.SizeBytes,
		ModTime:     wp.ModTime,
		CreatedAt:   wp.CreatedAt,
		Pinned:      wp.IsPinned,
		PinnedAt:    wp.PinnedAt,
		AccessLevel: accessLevel,
	}
	// Only expose the token to the authenticated admin for token-level links.
	if accessLevel == config.AccessToken && wp.AccessToken != "" {
		resp.AccessToken = wp.AccessToken
	}
	// Version, playlist and statistics are additive: a link that uses none of
	// them produces the payload it always produced.
	switch {
	case wp.CurrentVersion > 0:
		resp.CurrentVersion = wp.CurrentVersion
	case wp.HasImage:
		// Records written before versioning existed serve version 1.
		resp.CurrentVersion = 1
	}
	resp.History = wp.History
	resp.Items = wp.Items
	if (wp.Rotate != nil && wp.Rotate.Enabled) || len(wp.Items) > 0 {
		resp.Rotate = storage.NormalizeRotatePtr(wp.Rotate, len(wp.Items) > 0)
	}
	if stats, ok := storage.StatsFor(wp.LinkName); ok {
		resp.Stats = &stats
	}
	return resp
}

func isValidCategory(cat string) bool { return config.ValidCategories[cat] }

// removeFiles deletes image and optional preview files, ignoring not-found errors.
func removeFiles(imagePath, previewPath string) {
	removeFile(imagePath, "image")
	removeFile(previewPath, "preview")
}

// removeFile deletes one media file and reports whether it is gone. A file on a
// read-only filesystem (a legacy static/images copy inside a read_only container)
// cannot be removed by the server; that is a deployment matter, so it is logged
// as a note rather than as a failed delete.
func removeFile(path, kind string) bool {
	if path == "" {
		return true
	}
	err := os.Remove(path)
	switch {
	case err == nil || os.IsNotExist(err):
		return true
	case storage.IsReadOnlyError(err):
		log.Printf("Note: %s %s is on a read-only filesystem and was left in place", kind, path)
	default:
		log.Printf("Error removing %s %s: %v", kind, path, err)
	}
	return false
}

// linkNameFromPath extracts and validates the link name from /api/link/{name},
// the path shared by PATCH, DELETE and the pin/rollback/history sub-routes.
func linkNameFromPath(path string) (string, bool) {
	name := strings.TrimPrefix(path, "/api/link/")
	name = strings.Trim(name, "/")
	if name == "" || strings.Contains(name, "/") {
		return "", false
	}
	if !isValidLinkName(name) {
		return "", false
	}
	return name, true
}

// AdminPreview serves the thumbnail (or full media for videos) for the admin
// panel. Always requires admin auth (mounted behind MaybeBasicAuth). This is
// the ONLY way to load previews — they are no longer under /static/.
func AdminPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/preview/")
	name = strings.Trim(name, "/")
	if !isValidLinkName(name) {
		http.NotFound(w, r)
		return
	}
	wp, exists := storage.Global.Get(name)
	if !exists || !wp.HasImage {
		http.NotFound(w, r)
		return
	}

	// Open the preview without following a planted symlink; fall back to
	// the original media (including videos) if a thumbnail is missing.
	f, err := storage.OpenMedia(wp.PreviewPath)
	contentType := "image/webp"
	if err != nil {
		f, err = storage.OpenMedia(wp.ImagePath)
		contentType = mediaContentType(wp.MIMEType)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", "inline")
	h.Set("Cache-Control", mediaCacheControl)
	setMediaValidators(h, fi)
	/* Видео-ссылка отдаётся здесь целиком, поэтому у большого файла на
	   медленном канале должно быть столько же времени, сколько у публичной
	   отдачи: иначе превью обрывалось бы на середине. */
	extendMediaDeadline(w, fi.Size())
	http.ServeContent(w, r, filepath.Base(f.Name()), fi.ModTime(), f)
}

// externalImagesTTL bounds how stale the gallery listing may be. The picker is
// opened rarely and a walk of a large mount is expensive, so repeated requests
// within this window reuse one walk. A file added to the gallery shows up in
// the picker at most this long after it appears on disk.
const externalImagesTTL = 10 * time.Second

// externalImagesCache holds the last encoded listing. mu is held for the whole
// walk, so concurrent requests on an expired cache wait for one walk instead of
// each starting their own.
var externalImagesCache struct {
	mu   sync.Mutex
	key  string
	at   time.Time
	body []byte
}

// ExternalImages enumerates gallery files. The walk runs inside an os.Root,
// so symlinked files are listed only if they resolve inside the gallery, and
// symlinked directories are never descended into. Hard caps bound disk walk
// time and JSON memory when a mounted gallery is huge.
func ExternalImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	dir := config.Current.ExternalImageDir
	key := dir + "\x00" + strconv.Itoa(config.Current.MaxWalkDepth) + "\x00" + strconv.Itoa(config.Current.MaxUploadMB)

	externalImagesCache.mu.Lock()
	defer externalImagesCache.mu.Unlock()
	if externalImagesCache.body == nil || externalImagesCache.key != key || time.Since(externalImagesCache.at) > externalImagesTTL {
		body, err := listExternalImages(dir)
		if err != nil {
			jsonEmpty(w)
			return
		}
		externalImagesCache.key, externalImagesCache.at, externalImagesCache.body = key, time.Now(), body
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(externalImagesCache.body)
}

// listExternalImages walks the gallery and returns the encoded JSON array.
func listExternalImages(dir string) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	gallery := root.FS()

	const maxEntries = 20000
	const maxFiles = 5000
	visited := 0
	maxDepth := config.Current.MaxWalkDepth
	maxSize := int64(config.Current.MaxUploadMB) << 20
	files := make([]string, 0)
	_ = fs.WalkDir(gallery, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// A gallery may hold a directory the process cannot read (a mount
			// with stricter modes, a file that vanished mid-walk). Skipping it
			// keeps the reachable files listed; returning the error would turn
			// one unreadable folder into a 500 for the whole picker.
			return nil
		}
		visited++
		if visited > maxEntries || len(files) >= maxFiles {
			return fs.SkipAll
		}
		if d.IsDir() {
			if p != "." && (strings.HasPrefix(d.Name(), ".") || strings.Count(p, "/")+1 > maxDepth) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || !config.AllowedMediaExts[strings.ToLower(path.Ext(p))] {
			return nil
		}
		// Stat follows a symlink only within the root and reports the target.
		fi, err := fs.Stat(gallery, p)
		if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxSize {
			return nil
		}
		files = append(files, p)
		return nil
	})
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(files); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ExternalImagePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("path")
	if !utils.IsValidLocalPath(name) || !config.AllowedMediaExts[strings.ToLower(filepath.Ext(name))] {
		http.Error(w, "Invalid media path", http.StatusBadRequest)
		return
	}
	f, err := utils.OpenExternalFile(config.Current.ExternalImageDir, name)
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
	ext, err := inspectMediaFile(f, name, fi.Size(), int64(config.Current.MaxUploadMB)<<20)
	if err != nil {
		http.Error(w, "Invalid media file", http.StatusBadRequest)
		return
	}
	h := w.Header()
	h.Set("Content-Type", mediaContentType(ext))
	h.Set("Content-Disposition", "inline")
	h.Set("Cache-Control", mediaCacheControl)
	setMediaValidators(h, fi)
	extendMediaDeadline(w, fi.Size())
	http.ServeContent(w, r, filepath.Base(name), fi.ModTime(), f)
}

func jsonEmpty(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("[]\n"))
}
