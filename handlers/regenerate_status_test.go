// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegeneratePreviewsProgressEndpoint(t *testing.T) {
	previous := currentRegenerationStatus()
	t.Cleanup(func() { setRegenerationStatus(previous) })
	setRegenerationStatus(RegeneratePreviewsStatus{Running: true, Total: 12, Completed: 5, OK: 4, Errors: 1})
	rec := httptest.NewRecorder()
	RegeneratePreviews(rec, httptest.NewRequest(http.MethodGet, "/api/regenerate-previews", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status=%d headers=%v", rec.Code, rec.Header())
	}
	var status RegeneratePreviewsStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Total != 12 || status.Completed != 5 || status.OK != 4 || status.Errors != 1 {
		t.Fatalf("status=%+v", status)
	}
}

func TestRegeneratePreviewsMethodContract(t *testing.T) {
	rec := httptest.NewRecorder()
	RegeneratePreviews(rec, httptest.NewRequest(http.MethodDelete, "/api/regenerate-previews", strings.NewReader("")))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, POST" {
		t.Fatalf("status=%d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}
