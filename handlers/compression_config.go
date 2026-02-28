package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"lanpaper/config"
)

// CompressionConfigResponse is the client-side compression settings payload.
type CompressionConfigResponse struct {
	Quality int `json:"quality"`
	Scale   int `json:"scale"`
}

// GetCompressionConfig handles GET /api/compression-config.
// Cache-Control is intentionally short: compression settings can change
// at runtime via config reload without a server restart.
func GetCompressionConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	if err := json.NewEncoder(w).Encode(CompressionConfigResponse{
		Quality: config.Current.Compression.Quality,
		Scale:   config.Current.Compression.Scale,
	}); err != nil {
		log.Printf("Error encoding compression config response: %v", err)
	}
}
