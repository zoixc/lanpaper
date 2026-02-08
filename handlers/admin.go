package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zoixc/lanpaper/config"
	"github.com/zoixc/lanpaper/storage"
	"github.com/zoixc/lanpaper/utils"
)

func Admin(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "admin.html")
}

func Wallpapers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wallpapers := storage.Global.GetAll()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(wallpapers); err != nil {
		log.Printf("Error encoding wallpapers response: %v", err)
	}
}

func Link(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct{ LinkName string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if !isValidLinkName(req.LinkName) {
			http.Error(w, "Invalid link name", http.StatusBadRequest)
			return
		}
		if _, exists := storage.Global.Get(req.LinkName); exists {
			http.Error(w, "Link exists", http.StatusConflict)
			return
		}

		newWp := &storage.Wallpaper{
			ID:        req.LinkName,
			LinkName:  req.LinkName,
			HasImage:  false,
			CreatedAt: time.Now().Unix(),
		}
		storage.Global.Set(req.LinkName, newWp)

		if err := storage.Global.Save(); err != nil {
			log.Printf("Error saving wallpapers after link creation: %v", err)
		}
		log.Printf("Created link: %s", req.LinkName)
		w.WriteHeader(http.StatusCreated)

	case http.MethodDelete:
		linkName := filepath.Base(r.URL.Path)
		if !isValidLinkName(linkName) {
			http.Error(w, "Invalid link", http.StatusBadRequest)
			return
		}

		wp, exists := storage.Global.Get(linkName)
		if exists {
			if wp.HasImage {
				if err := os.Remove(wp.ImagePath); err != nil && !os.IsNotExist(err) {
					log.Printf("Error removing image %s: %v", wp.ImagePath, err)
				}
				if err := os.Remove(wp.PreviewPath); err != nil && !os.IsNotExist(err) {
					log.Printf("Error removing preview %s: %v", wp.PreviewPath, err)
				}
			}
			storage.Global.Delete(linkName)
		}

		if err := storage.Global.Save(); err != nil {
			log.Printf("Error saving wallpapers after link deletion: %v", err)
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

	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" ||
			ext == ".webp" || ext == ".bmp" || ext == ".tiff" || ext == ".tif" ||
			ext == ".mp4" || ext == ".webm" {

			relPath, err := filepath.Rel(root, path)
			if err == nil {
				relPath = filepath.ToSlash(relPath)
				files = append(files, relPath)
			}
		}
		return nil
	})

	if err != nil {
		log.Printf("Error walking external images directory: %v", err)
	}

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

	// Enhanced path validation
	if !utils.IsValidLocalPath(pathParam) {
		log.Printf("Security: blocked invalid preview path: %s", pathParam)
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	root := config.Current.ExternalImageDir
	if root == "" {
		root = "external/images"
	}

	// Resolve to absolute paths and validate
	absRoot, err := filepath.Abs(root)
	if err != nil {
		log.Printf("Error resolving root directory: %v", err)
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	fullPath := filepath.Join(absRoot, filepath.Clean(pathParam))
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		log.Printf("Error resolving preview path: %v", err)
		http.NotFound(w, r)
		return
	}

	// Ensure path is within allowed directory
	if !strings.HasPrefix(absPath, absRoot) {
		log.Printf("Security: blocked path traversal in preview: %s -> %s", pathParam, absPath)
		http.Error(w, "Path outside allowed directory", http.StatusForbidden)
		return
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	// Set security headers for served files
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, absPath)
}
