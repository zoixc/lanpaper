// SPDX-License-Identifier: MIT

package handlers

import (
	"errors"
	"testing"

	"lanpaper/storage"
)

func TestUploadServiceUsesInjectedStore(t *testing.T) {
	store := &storage.Store{}
	service := NewUploadService(store)
	if service.Store != store {
		t.Fatal("upload service replaced injected store")
	}
	if NewUploadService(nil).Store != storage.Global {
		t.Fatal("nil store did not use compatibility adapter")
	}
}

func TestUploadErrorPreservesCauseAndStage(t *testing.T) {
	cause := errors.New("disk full")
	err := &UploadError{Stage: "publish media", Err: cause}
	if !errors.Is(err, cause) {
		t.Fatal("typed upload error lost cause")
	}
	if got := err.Error(); got != "upload publish media: disk full" {
		t.Fatalf("error=%q", got)
	}
}
