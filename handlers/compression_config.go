// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"lanpaper/config"
)

// CompressionConfigResponse is the client-side compression settings payload.
type CompressionConfigResponse struct {
	Quality int    `json:"quality"`
	Scale   int    `json:"scale"`
	Version string `json:"version"`
}

// GetCompressionConfig handles GET /api/compression-config. The admin panel
// uses it to pre-compress images in the browser and display the running release.
// Version is injected by the composition root so package handlers does not own
// build metadata or depend on package main globals.
func GetCompressionConfig(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(CompressionConfigResponse{
			Quality: config.Current.Compression.Quality,
			Scale:   config.Current.Compression.Scale,
			Version: version,
		}); err != nil {
			log.Printf("Error encoding compression config response: %v", err)
		}
	}
}
