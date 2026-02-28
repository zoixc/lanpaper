package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"lanpaper/config"
)

// CompressionConfigResponse represents the client-side compression settings
type CompressionConfigResponse struct {
	Quality int `json:"quality"` // 1-100, JPEG quality
	Scale   int `json:"scale"`   // 1-100, percentage of max dimensions
}

// GetCompressionConfig returns the server's compression configuration for the client.
// GET /api/compression-config
// Public endpoint - no authentication required.
func GetCompressionConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	res := CompressionConfigResponse{
		Quality: config.Current.Compression.Quality,
		Scale:   config.Current.Compression.Scale,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour
	if err := json.NewEncoder(w).Encode(res); err != nil {
		log.Printf("Error encoding compression config response: %v", err)
	}
}
