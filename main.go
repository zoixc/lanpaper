package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"lanpaper/config"
	"lanpaper/handlers"
	"lanpaper/middleware"
	"lanpaper/storage"

	"github.com/joho/godotenv"
)

// Version is injected at build time via -ldflags "-X main.Version=..."; falls back to "dev".
var Version = "dev"

func main() {
	_ = godotenv.Load()
	config.Load()

	if config.Current.DisableAuth {
		log.Println("Warning: admin authentication explicitly disabled — protect /admin and /api/* at the reverse proxy.")
	} else if config.Current.AdminUser == "" || config.Current.AdminPass == "" {
		log.Println("Warning: admin credentials missing; admin endpoints will return 503. Set ADMIN_USER and ADMIN_PASS or explicitly set DISABLE_AUTH=true behind an auth proxy.")
	}

	handlers.InitUploadSemaphore(config.Current.MaxConcurrentUploads)

	for _, d := range []string{"data", config.MediaDir, config.PreviewDir} {
		if err := os.MkdirAll(d, 0700); err != nil {
			log.Fatalf("Cannot create data directory %s: %v", d, err)
		}
	}
	// External images are optional; legacy directories should not be created
	// just because an old deployment might have mounted them read-only.
	if err := os.MkdirAll("external/images", 0755); err != nil {
		log.Printf("Warning: external gallery unavailable: %v", err)
	}
	if err := storage.Global.Load(); err != nil {
		log.Fatalf("Cannot load wallpapers (refusing to overwrite metadata): %v", err)
	}
	// Move any leftover files from static/images into data/media.
	storage.MigrateMediaToDataDir()

	go middleware.StartCleaner()

	mux := newMux()

	port := config.Current.Port
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	srv := &http.Server{
		Addr:    port,
		Handler: mux,
		// ReadTimeout covers headers + body; WriteTimeout must exceed the download context timeout.
		ReadTimeout:  time.Duration(config.HTTPReadTimeout) * time.Second,
		WriteTimeout: time.Duration(config.HTTPWriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(config.HTTPIdleTimeout) * time.Second,
		// Cap request header size against slowloris / oversized header DoS.
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		log.Println("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.ShutdownTimeout)*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	log.Printf("Lanpaper %s on %s (max upload %d MB, compression: %d%% quality, %d%% scale)",
		Version, port, config.Current.MaxUploadMB, config.Current.Compression.Quality, config.Current.Compression.Scale)
	log.Printf("Admin: http://localhost%s/admin", port)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("Server stopped.")
}

// newMux is shared with the end-to-end HTTP tests, so tests exercise the
// real authentication, CSRF and routing stack, not just bare handlers.
func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/static/", serveStaticAsset)
	// The service worker must live at the root scope to control /admin and
	// public link URLs; it is served with Service-Worker-Allowed: /.
	mux.HandleFunc("/sw.js", serveServiceWorker)
	// Legacy URL kept working for PWA installs/bookmarks that used /admin.html.
	mux.HandleFunc("/admin.html", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin", http.StatusPermanentRedirect)
	})
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/health/ready", readyHandler)
	mux.HandleFunc("/admin", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Admin)))
	mux.HandleFunc("/api/wallpapers", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Wallpapers)))
	mux.HandleFunc("/api/compression-config", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.GetCompressionConfig)))
	mux.HandleFunc("/api/preview/", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.AdminPreview)))
	mux.HandleFunc("/api/link/", middleware.WithSecurity(middleware.MaybeBasicAuth(handleLinkRoutes)))
	mux.HandleFunc("/api/link", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Link)))
	mux.HandleFunc("/api/upload",
		middleware.WithSecurity(middleware.MaybeBasicAuth(
			middleware.RateLimit(func() (int, int) {
				return config.Current.Rate.UploadPerMin, config.Current.Rate.Burst
			})(handlers.Upload),
		)),
	)
	mux.HandleFunc("/api/external-images", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.ExternalImages)))
	mux.HandleFunc("/api/external-image-preview", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.ExternalImagePreview)))
	mux.HandleFunc("/api/regenerate-previews",
		middleware.WithSecurity(middleware.MaybeBasicAuth(
			middleware.RateLimit(func() (int, int) {
				// Regen is CPU-heavy — reuse the upload budget.
				return config.Current.Rate.UploadPerMin, config.Current.Rate.Burst
			})(handlers.RegeneratePreviews),
		)),
	)
	mux.HandleFunc("/", middleware.WithPublicSecurity(middleware.PublicRateLimit(handlers.Public)))

	return mux
}

// handleLinkRoutes routes /api/link/{name}/pin to TogglePin, everything else to Link
func handleLinkRoutes(w http.ResponseWriter, r *http.Request) {
	// Check if this is a pin toggle request (must be POST to /pin)
	if strings.HasSuffix(r.URL.Path, "/pin") && r.Method == http.MethodPost {
		handlers.TogglePin(w, r)
	} else {
		handlers.Link(w, r)
	}
}

// serveServiceWorker serves the service worker from the root path so its
// scope can cover the whole app (Service-Worker-Allowed: /). Registered at
// /static/sw.js the scope would be limited to /static/ and the SW would
// never control /admin or the public link URLs.
func serveServiceWorker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root, err := os.OpenRoot("static")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	fi, err := root.Lstat("sw.js")
	if err != nil || !fi.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	f, err := root.Open("sw.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.ServeContent(w, r, "sw.js", fi.ModTime(), f)
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "lanpaper",
		"version": Version,
	})
}

func readyHandler(w http.ResponseWriter, _ *http.Request) {
	type check struct {
		OK      bool   `json:"ok"`
		Message string `json:"message,omitempty"`
	}

	checks := make(map[string]check, 3)
	ready := true

	for _, entry := range []struct{ key, dir string }{
		{"storage", "data"},
		{"media", config.MediaDir},
	} {
		if _, err := os.Stat(entry.dir); err != nil {
			checks[entry.key] = check{OK: false, Message: entry.dir + " not accessible"}
			ready = false
		} else {
			checks[entry.key] = check{OK: true}
		}
	}

	// Check disk space
	if freeGB, err := getDiskFreeGB("."); err != nil {
		checks["disk"] = check{OK: false, Message: "cannot check disk space"}
		ready = false
	} else if freeGB < 1 {
		checks["disk"] = check{OK: false, Message: "low disk space"}
		ready = false
	} else {
		checks["disk"] = check{OK: true}
	}

	code := http.StatusOK
	status := "ready"
	if !ready {
		code = http.StatusServiceUnavailable
		status = "not ready"
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "checks": checks})
}

// getDiskFreeGB returns free disk space in GB for the given path.
func getDiskFreeGB(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	return float64(freeBytes) / (1024 * 1024 * 1024), nil
}
