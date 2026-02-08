package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	url_ "net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chai2010/webp"
	"github.com/joho/godotenv"
	"github.com/nfnt/resize"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
)

// CONFIG & MODELS

type RateCfg struct {
	PublicPerMin int `json:"public_per_min"`
	AdminPerMin  int `json:"admin_per_min"`
	UploadPerMin int `json:"upload_per_min"`
	Burst        int `json:"burst"`
}

type Config struct {
	Port                 string  `json:"port"`
	Username             string  `json:"username"`
	Password             string  `json:"password"`
	MaxUploadMB          int     `json:"maxUploadMB"`
	Rate                 RateCfg `json:"rate"`
	MaxConcurrentUploads int     `json:"max_concurrent_uploads"`

	// Proxy
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
	ProxyType          string `json:"proxyType,omitempty"`
	ProxyHost          string `json:"proxyHost,omitempty"`
	ProxyPort          string `json:"proxyPort,omitempty"`
	ProxyUsername      string `json:"proxyUsername,omitempty"`
	ProxyPassword      string `json:"proxyPassword,omitempty"`

	// Auth
	DisableAuth bool `json:"disableAuth,omitempty"`

	// Features
	MaxImages int `json:"maxImages,omitempty"`

	// External
	ExternalImageDir string `json:"externalImageDir,omitempty"`
}

type Wallpaper struct {
	ID        string `json:"id"`
	LinkName  string `json:"linkName"`
	ImageURL  string `json:"imageUrl"`
	Preview   string `json:"preview"`
	HasImage  bool   `json:"hasImage"`
	MIMEType  string `json:"mimeType"`
	SizeBytes int64  `json:"sizeBytes"`
	ModTime   int64  `json:"modTime"`
	CreatedAt int64  `json:"createdAt"`

	// Internal paths
	ImagePath   string `json:"-"`
	PreviewPath string `json:"-"`
}

type WallpaperStore struct {
	sync.RWMutex
	wallpapers map[string]*Wallpaper
}

// GLOBALS

var (
	cfg       Config
	store     = &WallpaperStore{wallpapers: make(map[string]*Wallpaper)}
	idRe      = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	uploadSem chan struct{}

	// Rate Limiting
	muCounts sync.Mutex
	counts   = map[string]*counter{}
)

type counter struct {
	Count      int
	WindowFrom time.Time
}

// MAIN

func main() {
	_ = godotenv.Load()
	loadConfig()

	if cfg.MaxConcurrentUploads <= 0 {
		cfg.MaxConcurrentUploads = 2
	}
	uploadSem = make(chan struct{}, cfg.MaxConcurrentUploads)

	// Directories
	dirs := []string{"static/images", "static/images/previews", "data", "external/images"}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			log.Fatalf("Failed to create directory %s: %v", d, err)
		}
	}

	if err := loadWallpapers(); err != nil {
		log.Printf("Warning: failed to load wallpapers: %v", err)
	}

	go startRateLimitCleaner()

	// Routing
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Admin UI
	http.HandleFunc("/admin", withSecurity(adminHandler))

	// API
	http.HandleFunc("/api/wallpapers", withSecurity(maybeBasicAuth(wallpapersHandler)))
	http.HandleFunc("/api/link/", withSecurity(maybeBasicAuth(linkHandler)))
	http.HandleFunc("/api/link", withSecurity(maybeBasicAuth(func(w http.ResponseWriter, r *http.Request) {
		linkHandler(w, r)
	})))

	http.HandleFunc("/api/upload", withSecurity(maybeBasicAuth(uploadHandler)))
	http.HandleFunc("/api/external-images", withSecurity(maybeBasicAuth(externalImagesHandler)))
	http.HandleFunc("/api/external-image-preview", withSecurity(maybeBasicAuth(externalImagePreviewHandler)))

	// Public Access
	http.HandleFunc("/", withSecurity(publicHandler))

	// Start server with graceful shutdown
	port := cfg.Port
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	server := &http.Server{
		Addr:         port,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		log.Println("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()

	log.Printf("Lanpaper server running on %s (max upload %d MB)", port, cfg.MaxUploadMB)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
	log.Println("Server stopped")
}

// HANDLERS

func adminHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "admin.html")
}

func wallpapersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	store.RLock()
	var wallpapers []*Wallpaper
	for _, wp := range store.wallpapers {
		if wp != nil {
			clone := *wp
			wallpapers = append(wallpapers, &clone)
		}
	}
	store.RUnlock()

	sort.Slice(wallpapers, func(i, j int) bool {
		if wallpapers[i].HasImage != wallpapers[j].HasImage {
			return wallpapers[i].HasImage
		}
		if wallpapers[i].HasImage {
			return wallpapers[i].ModTime > wallpapers[j].ModTime
		}
		return wallpapers[i].CreatedAt > wallpapers[j].CreatedAt
	})

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(wallpapers); err != nil {
		log.Printf("Error encoding wallpapers response: %v", err)
	}
}

func linkHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct{ LinkName string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if !idRe.MatchString(req.LinkName) {
			http.Error(w, "Invalid link name", http.StatusBadRequest)
			return
		}
		store.Lock()
		if _, exists := store.wallpapers[req.LinkName]; exists {
			store.Unlock()
			http.Error(w, "Link exists", http.StatusConflict)
			return
		}
		store.wallpapers[req.LinkName] = &Wallpaper{
			ID:        req.LinkName,
			LinkName:  req.LinkName,
			HasImage:  false,
			CreatedAt: time.Now().Unix(),
		}
		store.Unlock()
		if err := saveWallpapers(); err != nil {
			log.Printf("Error saving wallpapers after link creation: %v", err)
		}
		log.Printf("Created link: %s", req.LinkName)
		w.WriteHeader(http.StatusCreated)

	case http.MethodDelete:
		linkName := filepath.Base(r.URL.Path)
		if !idRe.MatchString(linkName) {
			http.Error(w, "Invalid link", http.StatusBadRequest)
			return
		}
		store.Lock()
		wp, exists := store.wallpapers[linkName]
		if exists {
			if wp.HasImage {
				if err := os.Remove(wp.ImagePath); err != nil && !os.IsNotExist(err) {
					log.Printf("Error removing image %s: %v", wp.ImagePath, err)
				}
				if err := os.Remove(wp.PreviewPath); err != nil && !os.IsNotExist(err) {
					log.Printf("Error removing preview %s: %v", wp.PreviewPath, err)
				}
			}
			delete(store.wallpapers, linkName)
		}
		store.Unlock()
		if err := saveWallpapers(); err != nil {
			log.Printf("Error saving wallpapers after link deletion: %v", err)
		}
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	select {
	case uploadSem <- struct{}{}:
		defer func() { <-uploadSem }()
	default:
		http.Error(w, "Too many concurrent uploads", http.StatusTooManyRequests)
		return
	}

	maxBytes := int64(cfg.MaxUploadMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	if err := r.ParseMultipartForm(maxBytes); err != nil {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	linkName := r.FormValue("linkName")
	if !idRe.MatchString(linkName) {
		http.Error(w, "Invalid link name", http.StatusBadRequest)
		return
	}

	store.RLock()
	oldWp, exists := store.wallpapers[linkName]
	store.RUnlock()
	if !exists {
		http.Error(w, "Link does not exist", http.StatusBadRequest)
		return
	}

	var (
		img     image.Image
		ext     string
		err     error
		isVideo bool
	)

	urlStr := r.FormValue("url")

	// Upload logic
	if urlStr != "" {
		if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
			img, ext, err = downloadImage(r.Context(), urlStr)
		} else {
			// Local files from server
			cleanRelPath := filepath.Clean(urlStr)
			if strings.HasPrefix(cleanRelPath, "..") || strings.HasPrefix(cleanRelPath, "/") || strings.HasPrefix(cleanRelPath, "\\") {
				http.Error(w, "Invalid path", http.StatusBadRequest)
				return
			}

			baseDir := cfg.ExternalImageDir
			if baseDir == "" {
				baseDir = "external/images"
			}

			localPath := filepath.Join(baseDir, cleanRelPath)
			ext = strings.ToLower(filepath.Ext(localPath))
			if len(ext) > 1 {
				ext = ext[1:]
			}
			if ext == "mp4" || ext == "webm" {
				isVideo = true
				err = nil
			} else {
				img, ext, err = loadLocalImage(localPath)
			}
		}

		if err != nil {
			log.Printf("Image load error for link %s: %v", linkName, err)
			http.Error(w, "Failed to load image: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		// Uploading from client
		file, _, ferr := r.FormFile("file")
		if ferr != nil {
			http.Error(w, "No file provided", http.StatusBadRequest)
			return
		}
		defer file.Close()

		head := make([]byte, 512)
		if _, err := file.Read(head); err != nil {
			http.Error(w, "Read error", http.StatusBadRequest)
			return
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			log.Printf("Error seeking file: %v", err)
			http.Error(w, "File seek error", http.StatusInternalServerError)
			return
		}

		contentType := http.DetectContentType(head)
		allowed := map[string]string{
			"image/jpeg": "jpg", "image/png": "png",
			"image/gif": "gif", "image/webp": "webp",
			"image/bmp": "bmp", "image/tiff": "tif",
			"video/mp4": "mp4", "video/webm": "webm",
		}

		if e, ok := allowed[contentType]; ok {
			ext = e
			if ext == "mp4" || ext == "webm" {
				isVideo = true
			}
		} else {
			http.Error(w, "Unsupported image format: "+contentType, http.StatusBadRequest)
			return
		}

		if !isVideo {
			img, _, err = image.Decode(file)
			if err != nil {
				http.Error(w, "Invalid image decode", http.StatusBadRequest)
				return
			}
		}
	}

	// Remove old image
	if oldWp != nil && oldWp.HasImage {
		if err := os.Remove(oldWp.ImagePath); err != nil && !os.IsNotExist(err) {
			log.Printf("Error removing old image %s: %v", oldWp.ImagePath, err)
		}
		if err := os.Remove(oldWp.PreviewPath); err != nil && !os.IsNotExist(err) {
			log.Printf("Error removing old preview %s: %v", oldWp.PreviewPath, err)
		}
	}

	// Save new image
	originalPath := filepath.Join("static", "images", linkName+"."+ext)
	previewPath := filepath.Join("static", "images", "previews", linkName+".webp")

	if isVideo {
		// Copy video file
		if urlStr == "" {
			file, _, _ := r.FormFile("file")
			if _, err := file.Seek(0, 0); err != nil {
				log.Printf("Error seeking file for video copy: %v", err)
			}
			out, err := os.Create(originalPath)
			if err != nil {
				log.Printf("Error creating video file %s: %v", originalPath, err)
				http.Error(w, "Failed to save video", http.StatusInternalServerError)
				return
			}
			if _, err := io.Copy(out, file); err != nil {
				log.Printf("Error copying video: %v", err)
			}
			out.Close()
			file.Close()
		} else if !strings.HasPrefix(urlStr, "http") {
			srcPath := filepath.Join(cfg.ExternalImageDir, urlStr)
			if cfg.ExternalImageDir == "" {
				srcPath = filepath.Join("external/images", urlStr)
			}
			in, err := os.Open(srcPath)
			if err == nil {
				out, err := os.Create(originalPath)
				if err != nil {
					log.Printf("Error creating video file %s: %v", originalPath, err)
				} else {
					if _, err := io.Copy(out, in); err != nil {
						log.Printf("Error copying video from external: %v", err)
					}
					out.Close()
				}
				in.Close()
			} else {
				log.Printf("Error opening external video %s: %v", srcPath, err)
			}
		}
		previewPath = "/static/icons/video-placeholder.png"

	} else {
		// Save & pic resize
		if err := saveImage(img, ext, originalPath); err != nil {
			log.Printf("Error saving image %s: %v", originalPath, err)
			http.Error(w, "Save failed", http.StatusInternalServerError)
			return
		}

		thumb := resize.Thumbnail(200, 160, img, resize.Bilinear)
		if err := saveImage(thumb, "webp", previewPath); err != nil {
			log.Printf("Error saving preview %s: %v", previewPath, err)
			if err := os.Remove(originalPath); err != nil {
				log.Printf("Error removing original after preview fail: %v", err)
			}
			http.Error(w, "Preview failed", http.StatusInternalServerError)
			return
		}
		previewPath = "/static/images/previews/" + linkName + ".webp"
	}

	fi, err := os.Stat(originalPath)
	if err != nil {
		log.Printf("Error stating uploaded file %s: %v", originalPath, err)
		http.Error(w, "Failed to stat file", http.StatusInternalServerError)
		return
	}

	createdAt := time.Now().Unix()
	if oldWp != nil {
		createdAt = oldWp.CreatedAt
	}

	wp := &Wallpaper{
		ID:          linkName,
		LinkName:    linkName,
		ImageURL:    "/static/images/" + linkName + "." + ext,
		Preview:     previewPath,
		HasImage:    true,
		MIMEType:    ext,
		SizeBytes:   fi.Size(),
		ModTime:     fi.ModTime().Unix(),
		CreatedAt:   createdAt,
		ImagePath:   originalPath,
		PreviewPath: previewPath,
	}

	if isVideo {
		wp.PreviewPath = ""
	} else {
		wp.PreviewPath = filepath.Join("static", "images", "previews", linkName+".webp")
	}

	store.Lock()
	store.wallpapers[linkName] = wp
	store.Unlock()
	
	if err := saveWallpapers(); err != nil {
		log.Printf("Error saving wallpapers after upload: %v", err)
	}

	if cfg.MaxImages > 0 {
		go func() { pruneOldImages(cfg.MaxImages) }()
	}

	log.Printf("Uploaded: %s (%s, %d KB)", linkName, ext, fi.Size()/1024)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(wp); err != nil {
		log.Printf("Error encoding upload response: %v", err)
	}
}

func externalImagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	root := cfg.ExternalImageDir
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

func externalImagePreviewHandler(w http.ResponseWriter, r *http.Request) {
	pathParam := r.URL.Query().Get("path")
	if pathParam == "" {
		http.NotFound(w, r)
		return
	}

	cleanPath := filepath.Clean(pathParam)
	if strings.Contains(cleanPath, "..") || strings.HasPrefix(cleanPath, "/") || strings.HasPrefix(cleanPath, "\\") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	root := cfg.ExternalImageDir
	if root == "" {
		root = "external/images"
	}
	fullPath := filepath.Join(root, cleanPath)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, fullPath)
}

func publicHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if path == "/" {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	if path == "/admin" || strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/static/") {
		http.NotFound(w, r)
		return
	}

	cleanPath := strings.TrimSuffix(path, "/")
	if len(cleanPath) < 2 {
		http.NotFound(w, r)
		return
	}
	id := cleanPath[1:]

	if !idRe.MatchString(id) {
		http.NotFound(w, r)
		return
	}

	store.RLock()
	wp, exists := store.wallpapers[id]
	store.RUnlock()

	if !exists || !wp.HasImage || wp.ImagePath == "" {
		http.NotFound(w, r)
		return
	}

	fi, err := os.Stat(wp.ImagePath)
	if err != nil || fi.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	mime := "image/" + wp.MIMEType
	if wp.MIMEType == "mp4" || wp.MIMEType == "webm" {
		mime = "video/" + wp.MIMEType
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.%s"`, wp.LinkName, wp.MIMEType))
	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	w.Header().Set("Accept-Ranges", "bytes")

	http.ServeFile(w, r, wp.ImagePath)
}

// MIDDLEWARE

func withSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; "+
				"script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' https: data:; "+
				"media-src 'self' https: data:; "+
				"connect-src 'self'; "+
				"font-src 'self'; "+
				"manifest-src 'self';")

		ip := clientIP(r)
		if !strings.HasPrefix(r.URL.Path, "/admin") && !strings.HasPrefix(r.URL.Path, "/api/") {
			if isOverLimit(ip, cfg.Rate.PublicPerMin, cfg.Rate.Burst) {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
		}
		next(w, r)
	}
}

func maybeBasicAuth(next http.HandlerFunc) http.HandlerFunc {
	if cfg.DisableAuth {
		return next
	}
	return basicAuth(next)
}

func basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != cfg.Username || pass != cfg.Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Admin"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// HELPERS

func loadConfig() {
	cfg = Config{
		Port:        "8080",
		MaxUploadMB: 10,
		Rate: RateCfg{
			PublicPerMin: 50,
			AdminPerMin:  0,
			UploadPerMin: 20,
			Burst:        10,
		},
		MaxImages: 100,
		ProxyType: "http",
	}

	if data, err := os.ReadFile("config.json"); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Printf("Warning: failed to parse config.json: %v", err)
		}
	}

	override := func(target *string, keys ...string) {
		for _, k := range keys {
			if v := os.Getenv(k); v != "" {
				*target = v
				return
			}
		}
	}

	override(&cfg.Port, "PORT")
	override(&cfg.Username, "ADMIN_USER", "USERNAME")
	override(&cfg.Password, "ADMIN_PASS", "PASSWORD")
	override(&cfg.ProxyType, "PROXY_TYPE")
	override(&cfg.ProxyHost, "PROXY_HOST", "PROXY_ADDRESS")
	override(&cfg.ProxyPort, "PROXY_PORT")
	override(&cfg.ProxyUsername, "PROXY_USER", "PROXY_USERNAME", "PROXY_LOGIN")
	override(&cfg.ProxyPassword, "PROXY_PASS", "PROXY_PASSWORD")
	override(&cfg.ExternalImageDir, "EXTERNAL_IMAGE_DIR", "IMAGE_FOLDER")

	if v := os.Getenv("MAX_UPLOAD_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxUploadMB = n
		}
	}
	if v := os.Getenv("INSECURE_SKIP_VERIFY"); v == "true" {
		cfg.InsecureSkipVerify = true
	}
	if v := os.Getenv("RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Rate.PublicPerMin = n
			cfg.Rate.AdminPerMin = n
			cfg.Rate.UploadPerMin = n
		}
	}
	if v := os.Getenv("DISABLE_AUTH"); v == "true" {
		cfg.DisableAuth = true
	}
	if v := os.Getenv("MAX_IMAGES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxImages = n
		}
	}
}

func saveWallpapers() error {
	store.RLock()
	defer store.RUnlock()
	data, err := json.MarshalIndent(store.wallpapers, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal wallpapers: %w", err)
	}
	if err := os.WriteFile("data/wallpapers.json", data, 0644); err != nil {
		return fmt.Errorf("failed to write wallpapers.json: %w", err)
	}
	return nil
}

func loadWallpapers() error {
	data, err := os.ReadFile("data/wallpapers.json")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	m := make(map[string]*Wallpaper)
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	for _, wp := range m {
		if wp.HasImage {
			ext := wp.MIMEType
			wp.ImagePath = filepath.Join("static", "images", wp.LinkName+"."+ext)

			if ext == "mp4" || ext == "webm" {
				wp.PreviewPath = ""
			} else {
				wp.PreviewPath = filepath.Join("static", "images", "previews", wp.LinkName+".webp")
			}
		}
	}

	store.Lock()
	store.wallpapers = m
	store.Unlock()
	return nil
}

func saveImage(img image.Image, format, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create image file: %w", err)
	}
	defer out.Close()

	switch format {
	case "jpg", "jpeg":
		return jpeg.Encode(out, img, &jpeg.Options{Quality: 85})
	case "png":
		return png.Encode(out, img)
	case "gif":
		return gif.Encode(out, img, &gif.Options{NumColors: 256})
	case "webp":
		return webp.Encode(out, img, &webp.Options{Quality: 85})
	default:
		return jpeg.Encode(out, img, &jpeg.Options{Quality: 85})
	}
}

func loadLocalImage(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("file not found on server")
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return nil, "", err
	}
	if format == "jpeg" {
		format = "jpg"
	}
	return img, format, nil
}

func downloadImage(ctx context.Context, urlStr string) (image.Image, string, error) {
	parsedURL, err := url_.Parse(urlStr)
	if err != nil || !parsedURL.IsAbs() || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, "", fmt.Errorf("invalid URL")
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	if cfg.ProxyHost != "" {
		proxyURL := &url_.URL{
			Scheme: cfg.ProxyType,
			Host:   net.JoinHostPort(cfg.ProxyHost, cfg.ProxyPort),
		}
		if cfg.ProxyUsername != "" {
			proxyURL.User = url_.UserPassword(cfg.ProxyUsername, cfg.ProxyPassword)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Lanpaper/1.0)")
	req.Header.Set("Accept", "image/*,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("network error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	limitReader := io.LimitReader(resp.Body, int64(cfg.MaxUploadMB)<<20)
	buf, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, "", fmt.Errorf("read error: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, "", fmt.Errorf("decode error")
	}

	ext := "jpg"
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		if strings.Contains(ct, "png") {
			ext = "png"
		} else if strings.Contains(ct, "gif") {
			ext = "gif"
		} else if strings.Contains(ct, "webp") {
			ext = "webp"
		}
	}
	return img, ext, nil
}

// pruneOldImages removing old images if quota exceeded
func pruneOldImages(max int) {
	store.Lock()

	var candidates []*Wallpaper
	for _, wp := range store.wallpapers {
		if wp.HasImage {
			candidates = append(candidates, wp)
		}
	}

	if len(candidates) <= max {
		store.Unlock()
		return
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ModTime < candidates[j].ModTime
	})

	toDelete := len(candidates) - max
	for i := 0; i < toDelete; i++ {
		wp := candidates[i]
		log.Printf("Pruning old image: %s", wp.ID)

		if err := os.Remove(wp.ImagePath); err != nil && !os.IsNotExist(err) {
			log.Printf("Error pruning image %s: %v", wp.ImagePath, err)
		}
		if wp.PreviewPath != "" && !strings.HasPrefix(wp.PreviewPath, "/static/") {
			if err := os.Remove(wp.PreviewPath); err != nil && !os.IsNotExist(err) {
				log.Printf("Error pruning preview %s: %v", wp.PreviewPath, err)
			}
		}

		wp.HasImage = false
		wp.ImageURL = ""
		wp.Preview = ""
		wp.MIMEType = ""
		wp.SizeBytes = 0
		wp.ImagePath = ""
		wp.PreviewPath = ""
	}

	store.Unlock()
	if err := saveWallpapers(); err != nil {
		log.Printf("Error saving wallpapers after pruning: %v", err)
	}
}

func clientIP(r *http.Request) string {
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		return strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func isOverLimit(ip string, perMin, burst int) bool {
	if perMin <= 0 {
		return false
	}
	now := time.Now()
	muCounts.Lock()
	defer muCounts.Unlock()
	c, ok := counts[ip]
	if !ok || now.Sub(c.WindowFrom) > time.Minute {
		counts[ip] = &counter{Count: 1, WindowFrom: now}
		return false
	}
	if c.Count >= perMin+burst {
		return true
	}
	c.Count++
	return false
}

func startRateLimitCleaner() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		muCounts.Lock()
		now := time.Now()
		for ip, c := range counts {
			if now.Sub(c.WindowFrom) > 5*time.Minute {
				delete(counts, ip)
			}
		}
		muCounts.Unlock()
	}
}
