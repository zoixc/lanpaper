// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lanpaper/config"
	"lanpaper/storage"
)

func importLinks(t *testing.T, body string) (*httptest.ResponseRecorder, importResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/import/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	BulkImportLinks(rec, req)
	var response importResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &response)
	return rec, response
}

func TestBulkImportValidatesEntireBatchBeforeMutation(t *testing.T) {
	seedWallpapers(t)
	rec, response := importLinks(t, `{"records":[{"linkName":"good"},{"linkName":"bad name"}]}`)
	if rec.Code != http.StatusUnprocessableEntity || response.Invalid != 1 {
		t.Fatalf("status=%d response=%+v", rec.Code, response)
	}
	if _, exists := storage.Global.Get("good"); exists {
		t.Fatal("valid prefix was persisted before validation completed")
	}
}

func TestBulkImportDryRunAndPerRecordReport(t *testing.T) {
	seedWallpapers(t, &storage.Wallpaper{ID: "present", LinkName: "present", Category: "other"})
	rec, response := importLinks(t, `{"dryRun":true,"records":[{"linkName":"present"},{"linkName":"new","category":"work"}]}`)
	if rec.Code != http.StatusOK || !response.DryRun || response.Skipped != 1 {
		t.Fatalf("status=%d response=%+v", rec.Code, response)
	}
	if response.Results[0].Status != "exists" || response.Results[1].Status != "ready" {
		t.Fatalf("results=%+v", response.Results)
	}
	if _, exists := storage.Global.Get("new"); exists {
		t.Fatal("dry run mutated storage")
	}
}

func TestBulkImportCreatesOneBatchAndRotatesTokenSecrets(t *testing.T) {
	seedWallpapers(t)
	rec, response := importLinks(t, `{"records":[{"linkName":"one"},{"linkName":"secret","accessLevel":"token","accessToken":"must-not-import"}]}`)
	if rec.Code != http.StatusCreated || response.Created != 2 {
		t.Fatalf("status=%d response=%+v body=%s", rec.Code, response, rec.Body.String())
	}
	secret, exists := storage.Global.Get("secret")
	if !exists || secret.AccessLevel != config.AccessToken || secret.AccessToken == "" || secret.AccessToken == "must-not-import" {
		t.Fatalf("token secret was not freshly generated: %+v", secret)
	}
}
