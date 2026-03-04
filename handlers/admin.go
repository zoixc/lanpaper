package handlers

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"

	"lanpaper/config"
	"lanpaper/storage"
)

//go:embed templates/*
var templateFS embed.FS

var adminTemplate *template.Template

func init() {
	var err error
	adminTemplate, err = template.New("admin").Funcs(template.FuncMap{
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, nil
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, nil
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
	}).ParseFS(templateFS, "templates/admin.html")
	if err != nil {
		log.Fatalf("Failed to parse admin template: %v", err)
	}
}

func Admin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := struct {
		Wallpapers      []*storage.Wallpaper
		ValidCategories []string
	}{
		Wallpapers:      storage.Global.ListAll(),
		ValidCategories: getValidCategoryList(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := adminTemplate.Execute(w, data); err != nil {
		log.Printf("Error rendering admin template: %v", err)
	}
}

func getValidCategoryList() []string {
	var categories []string
	for cat := range config.ValidCategories {
		categories = append(categories, cat)
	}
	return categories
}

func ListWallpapers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(storage.Global.ListAll()); err != nil {
		log.Printf("Error encoding wallpapers list: %v", err)
	}
}

func CreateLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		LinkName string `json:"linkName"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if !isValidLinkName(req.LinkName) {
		http.Error(w, "Invalid link name. Use only: a-z, 0-9, dash, underscore. Length: 1-64", http.StatusBadRequest)
		return
	}

	if _, exists := storage.Global.Get(req.LinkName); exists {
		http.Error(w, "Link already exists", http.StatusConflict)
		return
	}

	wp := &storage.Wallpaper{
		ID:       req.LinkName,
		LinkName: req.LinkName,
		HasImage: false,
	}

	storage.Global.Set(req.LinkName, wp)

	if err := storage.Global.Save(); err != nil {
		log.Printf("Error saving after create: %v — rolling back", err)
		storage.Global.Delete(req.LinkName)
		http.Error(w, "Failed to persist link", http.StatusInternalServerError)
		return
	}

	log.Printf("Created link: %s", req.LinkName)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(wp); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func DeleteLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	linkName := strings.TrimPrefix(r.URL.Path, "/api/link/")
	if linkName == "" {
		http.Error(w, "Link name required", http.StatusBadRequest)
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
		log.Printf("Error saving after delete: %v", err)
		http.Error(w, "Failed to persist deletion", http.StatusInternalServerError)
		return
	}

	log.Printf("Deleted link: %s", linkName)
	w.WriteHeader(http.StatusNoContent)
}

func GetCompressionConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(config.Current.Compression); err != nil {
		log.Printf("Error encoding compression config: %v", err)
	}
}
