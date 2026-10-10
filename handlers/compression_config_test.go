// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"lanpaper/config"
)

func TestCompressionConfigIncludesRunningVersion(t *testing.T) {
	previous := config.Current
	t.Cleanup(func() { config.Current = previous })
	config.Current.Compression.Quality = 82
	config.Current.Compression.Scale = 75

	recorder := httptest.NewRecorder()
	GetCompressionConfig("1.2.3").ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/compression-config", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response CompressionConfigResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Quality != 82 || response.Scale != 75 || response.Version != "1.2.3" {
		t.Fatalf("response = %+v", response)
	}
}

func TestCompressionConfigRejectsNonGET(t *testing.T) {
	recorder := httptest.NewRecorder()
	GetCompressionConfig("1.2.3").ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/compression-config", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}
