package handlers

import (
	"encoding/json"
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

// SortField represents valid sort field options for type safety.
type SortField string

const (
	SortFieldCreated SortField = "created"
	SortFieldUpdated SortField = "updated"
)

const (
	// DefaultPageSize is the default number of items per page when pagination is used.
	DefaultPageSize = 50
	// MaxPageSize is the maximum allowed page size to prevent excessive memory usage.
	MaxPageSize = 200
)

func Admin(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "admin.html")
}

type WallpaperResponse struct {
	ID        string `json:"id"`
	LinkName  string `json:"linkName"`
	Category  string `json:"category"`
	HasImage  bool   `json:"hasImage"`
	ImageURL  string `json:"imageUrl"`
	Preview   string `json:"preview,omitempty"`
	MIMEType  string `json:"mimeType"`
	SizeBytes int64  `json:"sizeBytes"`
	CreatedAt int64  `json:"createdAt"`
}

type PaginatedResponse struct {
	Data       []WallpaperResponse `json:"data"`
	Total      int                 `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"pageSize"`
	TotalPages int                 `json:"totalPages"`
}

// Wallpapers handles GET /api/wallpapers with optional pagination.
// Supported query params:
//   - category=<name>: Filter by category
//   - has_image=true|false: Filter by image presence
//   - sort=created|updated: Sort field (default: created)
//   - order=asc|desc: Sort order (default: desc)
//   - page=<number>: Page number for pagination (1-indexed, optional)
//   - page_size=<number>: Items per page (default: 50, max: 200, optional)
//
// Without page parameter, returns all results (backward compatible).
// With page parameter, returns paginated results with metadata.
func Wallpapers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Use GetAllCopy so we can freely sort/filter without touching the cached
	// snapshot or its original pointer values.
	wallpapers := storage.Global.GetAllCopy()

	// Apply filters efficiently
	if cat := r.URL.Query().Get("category"); cat != "" {
		filtered := make([]*storage.Wallpaper, 0, len(wallpapers)/2)
		for _, wp := range wallpapers {
			if strings.EqualFold(wp.Category, cat) {
				filtered = append(filtered, wp)
			}
		}
		wallpapers = filtered
	}
	if hasImg := r.URL.Query().Get("has_image"); hasImg != "" {
		want := hasImg == "true"
		filtered := make([]*storage.Wallpaper, 0, len(wallpapers))
		for _, wp := range wallpapers {
			if wp.HasImage == want {
				filtered = append(filtered, wp)
			}
		}
		wallpapers = filtered
	}

	// Apply sorting on the local copy.
	if sf := r.URL.Query().Get("sort"); sf != "" {
		desc := r.URL.Query().Get("order") != "asc"
		sortWallpapers(wallpapers, sf, desc)
	}

	// Check if pagination is requested
	pageStr := r.URL.Query().Get("page")
	if pageStr != "" {
		// Paginated response
		page, err := strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			http.Error(w, "Invalid page number", http.StatusBadRequest)
			return
		}

		pageSize := DefaultPageSize
		if pageSizeStr := r.URL.Query().Get("page_size"); pageSizeStr != "" {
			if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
				pageSize = ps
				if pageSize > MaxPageSize {
					pageSize = MaxPageSize
				}
			}
		}

		total := len(wallpapers)
		totalPages := (total + pageSize - 1) / pageSize
		if totalPages == 0 {
			totalPages = 1
		}

		start := (page - 1) * pageSize
		end := start + pageSize
		if start >= total {
			start = total
			end = total
		} else if end > total {
			end = total
		}

		pageData := wallpapers[start:end]
		resp := make([]WallpaperResponse, 0, len(pageData))
		for _, wp := range pageData {
			resp = append(resp, toResponse(wp))
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(PaginatedResponse{
			Data:       resp,
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: totalPages,
		}); err != nil {
			log.Printf("Error encoding paginated wallpapers response: %v", err)
		}
	} else {
		// Non-paginated response (backward compatible)
		resp := make([]WallpaperResponse, 0, len(wallpapers))
		for _, wp := range wallpapers {
			resp = append(resp, toResponse(wp))
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("Error encoding wallpapers response: %v", err)
		}
	}
}

// sortWallpapers sorts wallpapers using efficient O(n log n) algorithm.
func sortWallpapers(wps []*storage.Wallpaper, field string, desc bool) {
	sort.Slice(wps, func(i, j int) bool {
		var vi, vj int64
		if field == string(SortFieldUpdated) {
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

// inferCategory derives a display category when none is stored.
func inferCategory(wp *storage.Wallpaper) string {
	if wp.Category != "" {
		return wp.Category
	}
	switch {
	case wp.MIMEType == "mp4" || wp.MIMEType == "webm":
		return "video"
	case wp.HasImage:
		return "image"
	default:
		return "other"
	}
}

func toResponse(wp *storage.Wallpaper) WallpaperResponse {
	return WallpaperResponse{
		ID:        wp.ID,
		LinkName:  wp.LinkName,
		Category:  inferCategory(wp),
		HasImage:  wp.HasImage,
		ImageURL:  wp.ImageURL,
		Preview:   wp.Preview,
		MIMEType:  wp.MIMEType,
		SizeBytes: wp.SizeBytes,
		CreatedAt: wp.CreatedAt,
	}
}

// validCategories is the canonical set of user-assignable category names.
// Sourced from config so it can be extended without touching handler logic.
var validCategories = config.ValidCategories

func isValidCategory(cat string) bool { return validCategories[cat] }

// linkNameFromPath extracts and validates the last URL path segment.
func linkNameFromPath(r *http.Request) (string, bool) {
	name := filepath.Base(strings.TrimSuffix(r.URL.Path, "/"))
	if !isValidLinkName(name) {
		return "", false
	}
	return name, true
}

// removeFiles deletes image and optional preview files, ignoring not-found errors.
func removeFiles(imagePath, previewPath string) {
	if err := os.Remove(imagePath); err != nil && !os.IsNotExist(err) {
		log.Printf("Error removing image %s: %v", imagePath, err)
	}
	if previewPath != "" {
		if err := os.Remove(previewPath); err != nil && !os.IsNotExist(err) {
			log.Printf("Error removing preview %s: %v", previewPath, err)
		}
	}
}

// Link handles POST /api/link, PATCH /api/link/{name}, DELETE /api/link/{name}.
func Link(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			LinkName string `json:"linkName"`
			Category string `json:"category"`
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
		if _, exists := storage.Global.Get(req.LinkName); exists {
			http.Error(w, "Link exists", http.StatusConflict)
			return
		}
		cat := req.Category
		if cat == "" {
			cat = "other"
		}
		storage.Global.Set(req.LinkName, &storage.Wallpaper{
			ID:        req.LinkName,
			LinkName:  req.LinkName,
			Category:  cat,
			CreatedAt: time.Now().Unix(),
		})
		if err := storage.Global.Save(); err != nil {
			log.Printf("Error saving after link creation: %v", err)
		}
		log.Printf("Created link: %s (category: %s)", req.LinkName, cat)
		w.WriteHeader(http.StatusCreated)

	case http.MethodPatch:
		linkName, ok := linkNameFromPath(r)
		if !ok {
			http.Error(w, "Invalid link", http.StatusBadRequest)
			return
		}
		wp, exists := storage.Global.Get(linkName)
		if !exists {
			http.Error(w, "Link not found", http.StatusNotFound)
			return
		}
		var req struct {
			Category *string `json:"category"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Category != nil {
			// Empty string resets to "other" instead of storing a blank value.
			if *req.Category == "" {
				wp.Category = "other"
			} else if !isValidCategory(*req.Category) {
				http.Error(w, "Invalid category", http.StatusBadRequest)
				return
			} else {
				wp.Category = *req.Category
			}
		}
		storage.Global.Set(linkName, wp)
		if err := storage.Global.Save(); err != nil {
			log.Printf("Error saving after link patch: %v", err)
		}
		log.Printf("Patched link: %s (category: %s)", linkName, wp.Category)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(wp); err != nil {
			log.Printf("Error encoding patch response: %v", err)
		}

	case http.MethodDelete:
		linkName, ok := linkNameFromPath(r)
		if !ok {
			http.Error(w, "Invalid link", http.StatusBadRequest)
			return
		}
		wp, exists := storage.Global.Get(linkName)
		if !exists {
			http.Error(w, "Link not found", http.StatusNotFound)
			return
		}
		if wp.HasImage {
			removeFiles(wp.ImagePath, wp.PreviewPath)
		}
		storage.Global.Delete(linkName)
		if err := storage.Global.Save(); err != nil {
			log.Printf("Error saving after link deletion: %v", err)
		}
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func ExternalImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	root := utils.ExternalBaseDir()

	// Resolve the gallery root; a missing directory returns an empty list.
	absRoot, _, err := utils.ValidateAndResolvePath(root, ".")
	if err != nil {
		// Directory may not exist yet — return an empty list.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]string{})
		return
	}
	realRoot, realErr := filepath.EvalSymlinks(absRoot)
	if realErr != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]string{})
		return
	}

	// Use configurable MaxWalkDepth from config instead of hardcoded value
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
				if depth := len(strings.Split(rel, string(filepath.Separator))); depth > maxDepth {
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
		if isAllowedExt(filepath.Ext(d.Name())) {
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

// allowedExts is the set of file extensions served from the external gallery.
var allowedExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".bmp": true, ".tiff": true, ".tif": true,
	".mp4": true, ".webm": true,
}

func isAllowedExt(ext string) bool { return allowedExts[strings.ToLower(ext)] }

func ExternalImagePreview(w http.ResponseWriter, r *http.Request) {
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

	// Use utils.ValidateAndResolvePath to prevent path traversal and symlink escapes.
	absPath, _, err := utils.ValidateAndResolvePath(utils.ExternalBaseDir(), pathParam)
	if err != nil {
		log.Printf("Security: path validation failed for preview %s: %v", pathParam, err)
		http.Error(w, "Path outside allowed directory", http.StatusForbidden)
		return
	}

	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	// Instruct the browser to display the file inline rather than download it.
	h.Set("Content-Disposition", "inline")
	http.ServeFile(w, r, absPath)
}
