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

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
)

var (
	Version   = "dev"
	startTime = time.Now()
)

func main() {
	_ = godotenv.Load()
	config.Load()

	// Startup warnings
	if config.Current.DisableAuth {
		if config.Current.AdminUser == "" && config.Current.AdminPass == "" {
			log.Println("Warning: No credentials provided. Authentication is automatically disabled.")
		} else {
			log.Println("Warning: Authentication is disabled (DISABLE_AUTH=true).")
		}
	}

	handlers.InitUploadSemaphore(config.Current.MaxConcurrentUploads)

	dirs := []string{"data", "external/images", "static/images/previews"}
	for _, d := range dirs {
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
	mux.HandleFunc("/health", healthCheckHandler)

	mux.HandleFunc("/admin", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Admin)))

	mux.HandleFunc("/api/wallpapers", middleware.WithSecurity(handlers.Wallpapers))
	mux.HandleFunc("/api/link/", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Link)))
	mux.HandleFunc("/api/link", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Link)))

	// Rate limit values are read per-request inside the middleware closure,
	// so they always reflect the current config without needing a restart.
	uploadRate := middleware.RateLimit(
		func() (int, int) {
			return config.Current.Rate.UploadPerMin, config.Current.Rate.Burst
		},
	)
	mux.HandleFunc("/api/upload",
		middleware.WithSecurity(
			middleware.MaybeBasicAuth(
				uploadRate(handlers.Upload),
			),
		),
	)
	mux.HandleFunc("/api/external-images", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.ExternalImages)))
	mux.HandleFunc("/api/external-image-preview", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.ExternalImagePreview)))

	mux.HandleFunc("/", handlers.Public)

	port := config.Current.Port
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	server := &http.Server{
		Addr:         port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

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

	log.Printf("Lanpaper %s running on %s (max upload %d MB)", Version, port, config.Current.MaxUploadMB)
	log.Printf("Admin: http://localhost%s/admin", port)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
	log.Println("Server stopped")
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	response := map[string]interface{}{
		"status":  "ok",
		"service": "lanpaper",
		"version": Version,
		"time":    time.Now().Unix(),
		"uptime":  time.Since(startTime).String(),
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding health check response: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}
