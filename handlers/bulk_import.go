// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"lanpaper/config"
	"lanpaper/storage"
)

const maxImportBatch = 100

type importRecord struct {
	LinkName    string `json:"linkName"`
	Category    string `json:"category,omitempty"`
	AccessLevel string `json:"accessLevel,omitempty"`
}

type importRequest struct {
	Records []importRecord `json:"records"`
	DryRun  bool           `json:"dryRun"`
}

type importResult struct {
	Index    int    `json:"index"`
	LinkName string `json:"linkName"`
	Status   string `json:"status"`
	Code     string `json:"code,omitempty"`
}

type importResponse struct {
	DryRun  bool           `json:"dryRun"`
	Total   int            `json:"total"`
	Created int            `json:"created"`
	Skipped int            `json:"skipped"`
	Invalid int            `json:"invalid"`
	Results []importResult `json:"results"`
}

// BulkImportLinks validates an entire bounded batch before one durable store
// commit. A client first submits every batch with dryRun=true, then repeats the
// valid batches to import; disconnecting between batches is safe cancellation.
func BulkImportLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req importRequest
	if !decodeLinkJSON(w, r, &req) {
		return
	}
	if len(req.Records) == 0 || len(req.Records) > maxImportBatch {
		w.Header().Set("X-Error-Code", "IMPORT_BATCH_SIZE")
		http.Error(w, "Import batch must contain 1 to 100 records", http.StatusBadRequest)
		return
	}

	response := importResponse{DryRun: req.DryRun, Total: len(req.Records), Results: make([]importResult, len(req.Records))}
	seen := make(map[string]bool, len(req.Records))
	wallpapers := make([]*storage.Wallpaper, 0, len(req.Records))
	now := time.Now().Unix()
	for index, record := range req.Records {
		result := importResult{Index: index, LinkName: record.LinkName, Status: "ready"}
		switch {
		case !isValidLinkName(record.LinkName):
			result.Status, result.Code = "invalid", "INVALID_LINK_NAME"
		case seen[record.LinkName]:
			result.Status, result.Code = "invalid", "DUPLICATE_LINK_NAME"
		case record.Category != "" && !isValidCategory(record.Category):
			result.Status, result.Code = "invalid", "INVALID_CATEGORY"
		case record.AccessLevel != "" && !isValidAccessLevel(record.AccessLevel):
			result.Status, result.Code = "invalid", "INVALID_ACCESS_LEVEL"
		}
		seen[record.LinkName] = true
		if result.Status == "invalid" {
			response.Invalid++
			response.Results[index] = result
			continue
		}
		category := record.Category
		if category == "" {
			category = "other"
		}
		level := storage.NormalizeAccessLevel(record.AccessLevel)
		wallpaper := &storage.Wallpaper{
			ID: record.LinkName, LinkName: record.LinkName, Category: category,
			CreatedAt: now, AccessLevel: level,
		}
		if level == config.AccessToken {
			// Import files never carry secrets; token links receive a fresh one.
			wallpaper.AccessToken = generateAccessToken()
		}
		wallpapers = append(wallpapers, wallpaper)
		response.Results[index] = result
	}
	if response.Invalid > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Error-Code", "IMPORT_VALIDATION_FAILED")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	for index := range response.Results {
		if _, exists := storage.Global.Get(response.Results[index].LinkName); exists {
			response.Results[index].Status = "exists"
			response.Skipped++
		}
	}
	if req.DryRun {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
		return
	}
	if err := r.Context().Err(); err != nil {
		w.Header().Set("X-Error-Code", "IMPORT_CANCELLED")
		http.Error(w, "Import cancelled", http.StatusRequestTimeout)
		return
	}
	created, existing, err := storage.Global.CreateBatch(wallpapers)
	if err != nil {
		if errors.Is(err, storage.ErrExists) {
			w.Header().Set("X-Error-Code", "IMPORT_CONFLICT")
			http.Error(w, "Import conflict", http.StatusConflict)
			return
		}
		writeStoreError(w, err)
		return
	}
	createdSet := make(map[string]bool, len(created))
	existingSet := make(map[string]bool, len(existing))
	for _, name := range created {
		createdSet[name] = true
	}
	for _, name := range existing {
		existingSet[name] = true
	}
	response.Created, response.Skipped = len(created), len(existing)
	for index := range response.Results {
		name := response.Results[index].LinkName
		if createdSet[name] {
			response.Results[index].Status = "created"
		}
		if existingSet[name] {
			response.Results[index].Status = "exists"
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}
