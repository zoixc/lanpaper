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

func main() {
	_ = godotenv.Load()
	config.Load()

	handlers.InitUploadSemaphore(config.Current.MaxConcurrentUploads)

	// Directories
	dirs := []string{"static/images", "static/images/previews", "data", "external/images"}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			log.Fatalf("Failed to create directory %s: %v", d, err)
		}
	}

	if err := storage.Global.Load(); err != nil {
		log.Printf("Warning: failed to load wallpapers: %v", err)
	}

	go middleware.StartCleaner()

	// Routing
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Health check endpoint
	http.HandleFunc("/health", healthCheckHandler)

	// Admin UI
	http.HandleFunc("/admin", middleware.WithSecurity(handlers.Admin))

	// API
	http.HandleFunc("/api/wallpapers", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Wallpapers)))
	http.HandleFunc("/api/link/", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Link)))
	http.HandleFunc("/api/link", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Link)))
	http.HandleFunc("/api/upload", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Upload)))
	http.HandleFunc("/api/external-images", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.ExternalImages)))
	http.HandleFunc("/api/external-image-preview", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.ExternalImagePreview)))

	// Public Access
	http.HandleFunc("/", handlers.Public)

	// Start server with graceful shutdown
	port := config.Current.Port
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

	log.Printf("Lanpaper server running on %s (max upload %d MB)", port, config.Current.MaxUploadMB)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
	log.Println("Server stopped")
}

// healthCheckHandler provides a simple health check endpoint
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	response := map[string]interface{}{
		"status":  "ok",
		"service": "lanpaper",
		"time":    time.Now().Unix(),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding health check response: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}
