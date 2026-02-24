package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lanpaper/config"
	"lanpaper/storage"
	"lanpaper/utils"
)

// maxWalkDepth limits directory recursion depth in ExternalImages.
const maxWalkDepth = 3

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

// Wallpapers handles GET /api/wallpapers.
// Supported query params: ?category=, ?has_image=true|false, ?sort=created|updated, ?order=asc|desc
func Wallpapers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wallpapers := storage.Global.GetAll()

	if cat := r.URL.Query().Get("category"); cat != "" {
		out := wallpapers[:0]
		for _, wp := range wallpapers {
			if strings.EqualFold(wp.Category, cat) {
				out = append(out, wp)
			}
		}
		wallpapers = out
	}
	if hasImg := r.URL.Query().Get("has_image"); hasImg != "" {
		want := hasImg == "true"
		out := wallpapers[:0]
		for _, wp := range wallpapers {
			if wp.HasImage == want {
				out = append(out, wp)
			}
		}
		wallpapers = out
	}

	if sf := r.URL.Query().Get("sort"); sf != "" {
		desc := r.URL.Query().Get("order") != "asc"
		sortWallpapers(wallpapers, sf, desc)
	}

	resp := make([]WallpaperResponse, 0, len(wallpapers))
	for _, wp := range wallpapers {
		resp = append(resp, toResponse(wp))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding wallpapers response: %v", err)
	}
}

func sortWallpapers(wps []*storage.Wallpaper, field string, desc bool) {
	for i := 1; i < len(wps); i++ {
		for j := i; j > 0; j-- {
			var vi, vj int64
			if field == "updated" {
				vi, vj = wps[j].ModTime, wps[j-1].ModTime
			} else {
				vi, vj = wps[j].CreatedAt, wps[j-1].CreatedAt
			}
			if (desc && vi > vj) || (!desc && vi < vj) {
				wps[j], wps[j-1] = wps[j-1], wps[j]
			} else {
				break
			}
		}
	}
}

func toResponse(wp *storage.Wallpaper) WallpaperResponse {
	cat := wp.Category
	if cat == "" {
		switch {
		case wp.MIMEType == "mp4" || wp.MIMEType == "webm":
			cat = "video"
		case wp.HasImage:
			cat = "image"
		default:
			cat = "other"
		}
	}
	return WallpaperResponse{
		ID:        wp.ID,
		LinkName:  wp.LinkName,
		Category:  cat,
		HasImage:  wp.HasImage,
		ImageURL:  wp.ImageURL,
		Preview:   wp.Preview,
		MIMEType:  wp.MIMEType,
		SizeBytes: wp.SizeBytes,
		CreatedAt: wp.CreatedAt,
	}
}

var validCategories = map[string]bool{
	"tech": true, "life": true, "work": true, "other": true,
}

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

	root := config.Current.ExternalImageDir
	if root == "" {
		root = "external/images"
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		log.Printf("Error resolving external image root: %v", err)
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		// Directory may not exist yet — return an empty list.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]string{})
		return
	}

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
				if depth := len(strings.Split(rel, string(filepath.Separator))); depth > maxWalkDepth {
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

	root := config.Current.ExternalImageDir
	if root == "" {
		root = "external/images"
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		log.Printf("Error resolving root directory: %v", err)
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	absPath, err := filepath.Abs(filepath.Join(absRoot, filepath.Clean(pathParam)))
	if err != nil {
		log.Printf("Error resolving preview path: %v", err)
		http.NotFound(w, r)
		return
	}
	if !strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) && absPath != absRoot {
		log.Printf("Security: blocked path traversal in preview: %s -> %s", pathParam, absPath)
		http.Error(w, "Path outside allowed directory", http.StatusForbidden)
		return
	}

	// Resolve symlinks and verify the real target stays inside the gallery.
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !strings.HasPrefix(realPath, realRoot+string(filepath.Separator)) && realPath != realRoot {
		log.Printf("Security: blocked symlink escape in preview: %s -> %s", absPath, realPath)
		http.Error(w, "Path outside allowed directory", http.StatusForbidden)
		return
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, absPath)
}
