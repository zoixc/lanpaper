package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	maxJSONBody = 1 << 20 // 1 MB
)

func Admin(w http.ResponseWriter, r *http.Request) {
	// Always revalidate: the panel must not run a stale UI after an upgrade.
	w.Header().Set("Cache-Control", "no-cache")
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

	wallpapers := storage.Global.GetAllCopy()
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
		want := hasImg == "true"
		out := wallpapers[:0]
		for _, wp := range wallpapers {
			if wp.HasImage == want {
				out = append(out, wp)
			}
		}
		wallpapers = out
	}
	if sf := q.Get("sort"); sf != "" {
		sortWallpapers(wallpapers, sf, q.Get("order") != "asc")
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
	start = (page - 1) * pageSize
	if start > total {
		start = total
	}
	end = start + pageSize
	if end > total {
		end = total
	}
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
	if wp.MIMEType == "mp4" || wp.MIMEType == "webm" {
		return "video"
	}
	if wp.HasImage {
		return "image"
	}
	return "other"
}

func toResponse(wp *storage.Wallpaper) WallpaperResponse {
	ensureAccessDefaults(wp)
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
		AccessLevel: wp.AccessLevel,
	}
	// Only expose the token to the authenticated admin for token-level links.
	if wp.AccessLevel == config.AccessToken && wp.AccessToken != "" {
		resp.AccessToken = wp.AccessToken
	}
	return resp
}

var validCategories = config.ValidCategories

func isValidCategory(cat string) bool { return validCategories[cat] }

// removeFiles deletes image and optional preview files, ignoring not-found errors.
func removeFiles(imagePath, previewPath string) {
	if imagePath != "" {
		if err := os.Remove(imagePath); err != nil && !os.IsNotExist(err) {
			log.Printf("Error removing image %s: %v", imagePath, err)
		}
	}
	if previewPath != "" {
		if err := os.Remove(previewPath); err != nil && !os.IsNotExist(err) {
			log.Printf("Error removing preview %s: %v", previewPath, err)
		}
	}
}

// linkNameFromPath extracts and validates the link name from /api/link/{name}
// or /api/link/{name}/access etc. The suffix (if any) is returned separately.
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

// Link handles POST /api/link, PATCH /api/link/{name}, DELETE /api/link/{name}.
func Link(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	}
	switch r.Method {
	case http.MethodPost:
		var req struct {
			LinkName    string `json:"linkName"`
			Category    string `json:"category"`
			AccessLevel string `json:"accessLevel"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
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
		if _, exists := storage.Global.Get(req.LinkName); exists {
			http.Error(w, "Link exists", http.StatusConflict)
			return
		}
		cat := req.Category
		if cat == "" {
			cat = "other"
		}
		level := storage.NormalizeAccessLevel(req.AccessLevel)
		newWp := &storage.Wallpaper{
			ID:          req.LinkName,
			LinkName:    req.LinkName,
			Category:    cat,
			CreatedAt:   time.Now().Unix(),
			AccessLevel: level,
		}
		if level == config.AccessToken {
			tok, err := generateAccessToken()
			if err != nil {
				http.Error(w, "Failed to generate access token", http.StatusInternalServerError)
				return
			}
			newWp.AccessToken = tok
		}
		storage.Global.Set(req.LinkName, newWp)
		if err := storage.Global.Save(); err != nil {
			log.Printf("Error saving after link creation: %v", err)
		}
		log.Printf("Created link: %s (category: %s, access: %s)", req.LinkName, cat, level)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(toResponse(newWp)); err != nil {
			log.Printf("Error encoding link creation response: %v", err)
		}

	case http.MethodPatch:
		linkName, ok := linkNameFromPath(r.URL.Path)
		if !ok {
			http.Error(w, "Invalid or missing link name", http.StatusBadRequest)
			return
		}

		var req struct {
			NewLinkName *string `json:"newLinkName"`
			Category    *string `json:"category"`
			AccessLevel *string `json:"accessLevel"`
			RotateToken bool    `json:"rotateToken"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// --- Rename ---
		if req.NewLinkName != nil {
			newName := *req.NewLinkName
			if !isValidLinkName(newName) {
				http.Error(w, "Invalid new link name", http.StatusBadRequest)
				return
			}
			if newName == linkName {
				wp, exists := storage.Global.Get(linkName)
				if !exists {
					http.Error(w, "Link not found", http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(toResponse(wp))
				return
			}
			if _, exists := storage.Global.Get(newName); exists {
				http.Error(w, "Link name already taken", http.StatusConflict)
				return
			}

			wpOld, exists := storage.Global.Get(linkName)
			if !exists {
				http.Error(w, "Link not found", http.StatusNotFound)
				return
			}
			if wpOld.HasImage && wpOld.MIMEType != "" {
				oldImg := wpOld.ImagePath
				if oldImg == "" {
					oldImg = storage.MediaPath(linkName, wpOld.MIMEType)
				}
				newImg := storage.MediaPath(newName, wpOld.MIMEType)
				if err := renameFile(oldImg, newImg); err != nil {
					log.Printf("Error renaming image file %s -> %s: %v", oldImg, newImg, err)
					http.Error(w, "Failed to rename image file", http.StatusInternalServerError)
					return
				}
				// Also try legacy location.
				_ = renameFile(
					filepath.Join(config.LegacyMedia, linkName+"."+wpOld.MIMEType),
					storage.MediaPath(newName, wpOld.MIMEType),
				)
				if wpOld.MIMEType != "mp4" && wpOld.MIMEType != "webm" {
					oldPrev := wpOld.PreviewPath
					if oldPrev == "" {
						oldPrev = storage.PreviewFilePath(linkName)
					}
					newPrev := storage.PreviewFilePath(newName)
					if err := renameFile(oldPrev, newPrev); err != nil {
						log.Printf("Warning: could not rename preview %s -> %s: %v", oldPrev, newPrev, err)
					}
					_ = renameFile(
						filepath.Join(config.LegacyMedia, "previews", linkName+".webp"),
						storage.PreviewFilePath(newName),
					)
				}
			}

			wp, ok := storage.Global.Rename(linkName, newName)
			if !ok {
				http.Error(w, "Rename failed", http.StatusInternalServerError)
				return
			}

			// Update URLs and runtime paths to reflect the new name.
			if wp.HasImage && wp.MIMEType != "" {
				wp.ImageURL = "/" + newName
				wp.ImagePath = storage.MediaPath(newName, wp.MIMEType)
				if wp.MIMEType != "mp4" && wp.MIMEType != "webm" {
					wp.Preview = "/api/preview/" + newName
					wp.PreviewPath = storage.PreviewFilePath(newName)
				}
				storage.Global.Set(newName, wp)
			}

			if err := storage.Global.Save(); err != nil {
				log.Printf("Error saving after rename: %v", err)
			}
			log.Printf("Renamed link: %s -> %s", linkName, newName)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(toResponse(wp))
			return
		}

		// --- Category / access patch ---
		wp, exists := storage.Global.Get(linkName)
		if !exists {
			http.Error(w, "Link not found", http.StatusNotFound)
			return
		}
		if req.Category != nil {
			switch {
			case *req.Category == "":
				wp.Category = "other"
			case !isValidCategory(*req.Category):
				http.Error(w, "Invalid category", http.StatusBadRequest)
				return
			default:
				wp.Category = *req.Category
			}
		}
		if req.AccessLevel != nil {
			if !isValidAccessLevel(*req.AccessLevel) {
				http.Error(w, "Invalid access level", http.StatusBadRequest)
				return
			}
			newLevel := storage.NormalizeAccessLevel(*req.AccessLevel)
			prev := storage.NormalizeAccessLevel(wp.AccessLevel)
			wp.AccessLevel = newLevel
			if newLevel == config.AccessToken && (prev != config.AccessToken || wp.AccessToken == "" || req.RotateToken) {
				tok, err := generateAccessToken()
				if err != nil {
					http.Error(w, "Failed to generate access token", http.StatusInternalServerError)
					return
				}
				wp.AccessToken = tok
			}
			if newLevel != config.AccessToken {
				wp.AccessToken = ""
			}
		} else if req.RotateToken {
			if storage.NormalizeAccessLevel(wp.AccessLevel) != config.AccessToken {
				http.Error(w, "Link is not token-protected", http.StatusBadRequest)
				return
			}
			tok, err := generateAccessToken()
			if err != nil {
				http.Error(w, "Failed to generate access token", http.StatusInternalServerError)
				return
			}
			wp.AccessToken = tok
		}
		storage.Global.Set(linkName, wp)
		if err := storage.Global.Save(); err != nil {
			log.Printf("Error saving after link patch: %v", err)
		}
		log.Printf("Patched link: %s (category: %s, access: %s)", linkName, wp.Category, wp.AccessLevel)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(toResponse(wp)); err != nil {
			log.Printf("Error encoding patch response: %v", err)
		}

	case http.MethodDelete:
		linkName, ok := linkNameFromPath(r.URL.Path)
		if !ok {
			http.Error(w, "Invalid or missing link name", http.StatusBadRequest)
			return
		}
		wp, exists := storage.Global.Get(linkName)
		if !exists {
			http.Error(w, "Link not found", http.StatusNotFound)
			return
		}
		if wp.HasImage {
			removeFiles(wp.ImagePath, wp.PreviewPath)
			// Clean legacy paths too.
			if wp.MIMEType != "" {
				removeFiles(
					filepath.Join(config.LegacyMedia, linkName+"."+wp.MIMEType),
					filepath.Join(config.LegacyMedia, "previews", linkName+".webp"),
				)
			}
		}
		storage.Global.Delete(linkName)
		if err := storage.Global.Save(); err != nil {
			log.Printf("Error saving after link deletion: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// renameFile renames src to dst, creating the destination directory as needed.
// Missing source is not an error (returns nil).
func renameFile(src, dst string) error {
	if src == "" || dst == "" || src == dst {
		return nil
	}
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// TogglePin handles POST /api/link/{name}/pin to toggle pin status.
func TogglePin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/link/")
	path = strings.TrimSuffix(path, "/pin")
	linkName := strings.Trim(path, "/")

	if linkName == "" || !isValidLinkName(linkName) {
		http.Error(w, "Invalid link name", http.StatusBadRequest)
		return
	}

	wp, exists := storage.Global.Get(linkName)
	if !exists {
		http.Error(w, "Link not found", http.StatusNotFound)
		return
	}

	wp.IsPinned = !wp.IsPinned
	if wp.IsPinned {
		wp.PinnedAt = time.Now().Unix()
	} else {
		wp.PinnedAt = 0
	}

	storage.Global.Set(linkName, wp)
	if err := storage.Global.Save(); err != nil {
		log.Printf("Error saving after pin toggle: %v", err)
	}

	action := "unpinned"
	if wp.IsPinned {
		action = "pinned"
	}
	log.Printf("Link %s: %s", linkName, action)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(toResponse(wp)); err != nil {
		log.Printf("Error encoding pin toggle response: %v", err)
	}
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

	// Prefer the WebP preview; fall back to the full media file (videos).
	path := wp.PreviewPath
	contentType := "image/webp"
	if path == "" || !fileExists(path) {
		path = wp.ImagePath
		if path == "" || !fileExists(path) {
			http.NotFound(w, r)
			return
		}
		contentType = "image/" + wp.MIMEType
		if wp.MIMEType == "mp4" || wp.MIMEType == "webm" {
			contentType = "video/" + wp.MIMEType
		}
	}

	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		http.NotFound(w, r)
		return
	}

	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", "inline")
	h.Set("Cache-Control", "private, max-age=60, must-revalidate")
	h.Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filepath.Base(path), fi.ModTime(), f)
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func ExternalImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	root := utils.ExternalBaseDir()
	absRoot, _, err := utils.ValidateAndResolvePath(root, ".")
	if err != nil {
		jsonEmpty(w)
		return
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		jsonEmpty(w)
		return
	}

	maxDepth := config.Current.MaxWalkDepth
	var files []string
	_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			if rel, relErr := filepath.Rel(absRoot, path); relErr == nil && rel != "." {
				if len(strings.Split(rel, string(filepath.Separator))) > maxDepth {
					return filepath.SkipDir
				}
			}
			return nil
		}
		realPath, symlinkErr := filepath.EvalSymlinks(path)
		if symlinkErr != nil {
			return nil
		}
		if !strings.HasPrefix(realPath, realRoot+string(filepath.Separator)) && realPath != realRoot {
			log.Printf("Security: skipping symlink escape: %s -> %s", path, realPath)
			return nil
		}
		if config.AllowedMediaExts[strings.ToLower(filepath.Ext(d.Name()))] {
			if relPath, relErr := filepath.Rel(absRoot, path); relErr == nil {
				files = append(files, filepath.ToSlash(relPath))
			}
		}
		return nil
	})

	if files == nil {
		files = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(files); err != nil {
		log.Printf("Error encoding external images response: %v", err)
	}
}

func ExternalImagePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pathParam := r.URL.Query().Get("path")
	if pathParam == "" {
		http.NotFound(w, r)
		return
	}
	if !utils.IsValidLocalPath(pathParam) {
		log.Printf("Security: blocked invalid preview path: %s", pathParam)
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	// Reject paths whose extension is not an allowed media type.
	ext := strings.ToLower(filepath.Ext(pathParam))
	if !config.AllowedMediaExts[ext] {
		http.Error(w, "Unsupported file type", http.StatusBadRequest)
		return
	}
	absPath, _, err := utils.ValidateAndResolvePath(utils.ExternalBaseDir(), pathParam)
	if err != nil {
		log.Printf("Security: path validation failed for preview %s: %v", pathParam, err)
		http.Error(w, "Path outside allowed directory", http.StatusForbidden)
		return
	}
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Disposition", "inline")
	h.Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, absPath)
}

func jsonEmpty(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("[]\n"))
}
