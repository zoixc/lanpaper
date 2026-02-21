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

const Version = "v0.8.7"

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

	// Router
	mux := http.NewServeMux()

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Health check
	mux.HandleFunc("/health", healthCheckHandler)

	// Admin UI (NO AUTH - для тестов)
	mux.HandleFunc("/admin", handlers.Admin)

	// API - PUBLIC для тестов (убрал middleware.WithSecurity)
	mux.HandleFunc("/api/wallpapers", handlers.Wallpapers)
	mux.HandleFunc("/api/tags", handlers.Tags)
	mux.HandleFunc("/api/link/", handlers.Link)
	mux.HandleFunc("/api/link", handlers.Link)
	mux.HandleFunc("/api/upload", handlers.Upload)
	mux.HandleFunc("/api/external-images", handlers.ExternalImages)
	mux.HandleFunc("/api/external-image-preview", handlers.ExternalImagePreview)

	// Public pages
	mux.HandleFunc("/", handlers.Public)

	// Server config
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

	log.Printf("Lanpaper %s running on %s (max upload %d MB)", Version, port, config.Current.MaxUploadMB)
	log.Printf("Admin: http://localhost%s/admin", port)
	log.Printf("API endpoints ready")

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
		"uptime":  time.Since(time.Now().Add(-5 * time.Minute)).String(), // пример
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding health check response: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
}
