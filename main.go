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

	// validate() already logs a warning and sets DisableAuth=true when
	// credentials are absent. Emit an additional startup-level notice so it
	// is visible prominently in the log stream.
	if config.Current.DisableAuth {
		if config.Current.AdminUser == "" && config.Current.AdminPass == "" {
			log.Println("Warning: no credentials configured — running WITHOUT authentication.")
		} else {
			log.Println("Warning: authentication disabled (DISABLE_AUTH=true).")
		}
	}

	handlers.InitUploadSemaphore(config.Current.MaxConcurrentUploads)

	for _, d := range []string{"data", "external/images", "static/images", "static/images/previews"} {
		if err := os.MkdirAll(d, 0755); err != nil {
			log.Printf("Warning: failed to create %s: %v", d, err)
		}
	}

	if err := storage.Global.Load(); err != nil {
		if os.IsNotExist(err) {
			log.Println("Info: no existing wallpapers data, starting fresh")
		} else {
			log.Fatalf("FATAL: corrupted storage file: %v", err)
		}
	}

	// Start background cleaners
	go middleware.StartCleaner()
	// CleanExpiredCSRFTokens is a no-op for stateless HMAC tokens;
	// kept for API compatibility, no goroutine needed.
	middleware.CleanExpiredCSRFTokens()

	// Serve static files with long-lived cache for versioned assets.
	// The app uses ?t=<timestamp> cache-busting on dynamic resources.
	staticFS := http.FileServer(http.Dir("static"))
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			staticFS.ServeHTTP(w, r)
		}),
	))
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/health/ready", readyHandler)

	// Admin panel with CSRF protection
	mux.HandleFunc("/admin", middleware.WithSecurity(middleware.CSRFProtection(middleware.MaybeBasicAuth(handlers.Admin))))

	// Read-only API endpoints — guarded by auth when enabled.
	mux.HandleFunc("/api/wallpapers", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Wallpapers)))
	mux.HandleFunc("/api/compression-config", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.GetCompressionConfig)))
	mux.HandleFunc("/api/external-images", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.ExternalImages)))
	mux.HandleFunc("/api/external-image-preview", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.ExternalImagePreview)))

	// State-changing API endpoints with CSRF protection
	mux.HandleFunc("/api/link/", middleware.WithSecurity(middleware.CSRFProtection(middleware.MaybeBasicAuth(handleLinkRoutes))))
	mux.HandleFunc("/api/link", middleware.WithSecurity(middleware.CSRFProtection(middleware.MaybeBasicAuth(handlers.Link))))
	mux.HandleFunc("/api/upload",
		middleware.WithSecurity(middleware.CSRFProtection(middleware.MaybeBasicAuth(
			middleware.RateLimit(func() (int, int) {
				return config.Current.Rate.UploadPerMin, config.Current.Rate.Burst
			})(handlers.Upload),
		))),
	)
	mux.HandleFunc("/api/regenerate-previews",
		middleware.WithSecurity(middleware.CSRFProtection(middleware.MaybeBasicAuth(handlers.RegeneratePreviews))),
	)

	// Public page (no auth/CSRF needed)
	mux.HandleFunc("/", handlers.Public)

	port := config.Current.Port
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	srv := &http.Server{
		Addr:              port,
		Handler:           mux,
		ReadHeaderTimeout: time.Duration(config.HTTPReadHeaderTimeout) * time.Second,
		ReadTimeout:       time.Duration(config.HTTPReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(config.HTTPWriteTimeout) * time.Second,
		IdleTimeout:       time.Duration(config.HTTPIdleTimeout) * time.Second,
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

// handleLinkRoutes routes /api/link/{name}/pin to TogglePin, everything else to Link.
func handleLinkRoutes(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/pin") && r.Method == http.MethodPost {
		handlers.TogglePin(w, r)
	} else {
		handlers.Link(w, r)
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
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
		{"static", "static/images"},
	} {
		if _, err := os.Stat(entry.dir); err != nil {
			checks[entry.key] = check{OK: false, Message: entry.dir + " not accessible"}
			ready = false
		} else {
			checks[entry.key] = check{OK: true}
		}
	}

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
