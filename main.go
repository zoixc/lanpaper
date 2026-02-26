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
		if config.Current.AdminUser == "" && config.Current.AdminPass == "" {
			log.Println("Warning: no credentials provided — authentication disabled.")
		} else {
			log.Println("Warning: authentication disabled (DISABLE_AUTH=true).")
		}
	}

	handlers.InitUploadSemaphore(config.Current.MaxConcurrentUploads)

	for _, d := range []string{"data", "external/images", "static/images/previews"} {
		if err := os.MkdirAll(d, 0755); err != nil {
			log.Printf("Warning: failed to create %s: %v", d, err)
		}
	}

	if err := storage.Global.Load(); err != nil {
		log.Printf("Warning: failed to load wallpapers: %v", err)
	}

	go middleware.StartCleaner()

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/health/ready", readyHandler)
	mux.HandleFunc("/admin", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Admin)))
	mux.HandleFunc("/api/wallpapers", middleware.WithSecurity(handlers.Wallpapers))
	mux.HandleFunc("/api/compression-config", middleware.WithSecurity(handlers.GetCompressionConfig))
	mux.HandleFunc("/api/link/", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Link)))
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
	mux.HandleFunc("/", handlers.Public)

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

	log.Printf("Lanpaper %s on %s (max upload %d MB, compression: %d%% quality, %d%% scale)", Version, port, config.Current.MaxUploadMB, config.Current.Compression.Quality, config.Current.Compression.Scale)
	log.Printf("Admin: http://localhost%s/admin", port)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("Server stopped.")
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

	checks := make(map[string]check, 2)
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
