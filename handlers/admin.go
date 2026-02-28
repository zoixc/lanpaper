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

const (
	DefaultPageSize = 50
	MaxPageSize     = 200
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
	ModTime   int64  `json:"modTime"`
	CreatedAt int64  `json:"createdAt"`
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

func sortWallpapers(wps []*storage.Wallpaper, field string, desc bool) {
	sort.Slice(wps, func(i, j int) bool {
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
	return WallpaperResponse{
		ID:        wp.ID,
		LinkName:  wp.LinkName,
		Category:  inferCategory(wp),
		HasImage:  wp.HasImage,
		ImageURL:  wp.ImageURL,
		Preview:   wp.Preview,
		MIMEType:  wp.MIMEType,
		SizeBytes: wp.SizeBytes,
		ModTime:   wp.ModTime,
		CreatedAt: wp.CreatedAt,
	}
}

var validCategories = config.ValidCategories

func isValidCategory(cat string) bool { return validCategories[cat] }

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

// linkNameFromPath extracts and validates the link name from /api/link/{name}.
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
		newWp := &storage.Wallpaper{
			ID:        req.LinkName,
			LinkName:  req.LinkName,
			Category:  cat,
			CreatedAt: time.Now().Unix(),
		}
		storage.Global.Set(req.LinkName, newWp)
		if err := storage.Global.Save(); err != nil {
			log.Printf("Error saving after link creation: %v", err)
		}
		log.Printf("Created link: %s (category: %s)", req.LinkName, cat)
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
		storage.Global.Set(linkName, wp)
		if err := storage.Global.Save(); err != nil {
			log.Printf("Error saving after link patch: %v", err)
		}
		log.Printf("Patched link: %s (category: %s)", linkName, wp.Category)
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
	absPath, _, err := utils.ValidateAndResolvePath(utils.ExternalBaseDir(), pathParam)
	if err != nil {
		log.Printf("Security: path validation failed for preview %s: %v", pathParam, err)
		http.Error(w, "Path outside allowed directory", http.StatusForbidden)
		return
	}
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Disposition", "inline")
	h.Set("Cache-Control", "public, max-age=300")
	http.ServeFile(w, r, absPath)
}

func jsonEmpty(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("[]\n"))
}
