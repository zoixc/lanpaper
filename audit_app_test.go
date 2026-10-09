// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"image/color"
	"net/http"
	"testing"

	"lanpaper/config"
)

// A publish key may create a link and add media, but it may not replace the
// live file of a link that already has media. An admin login still may.
func TestAppPublishKeyCannotReplaceExistingMedia(t *testing.T) {
	a := setupFeatureApp(t)
	const key = "publish-key-for-audit-0123456789"
	config.Current.PublishKeys = []string{key}
	config.RefreshDerived()
	red := makePNG(t, color.RGBA{R: 255, A: 255})
	blue := makePNG(t, color.RGBA{B: 255, A: 255})
	green := makePNG(t, color.RGBA{G: 255, A: 255})

	status, _, body := a.request("POST", "/api/link", []byte(`{"linkName":"guarded"}`), false, map[string]string{
		"Content-Type": "application/json", "X-Api-Key": key,
	})
	if status != http.StatusCreated {
		t.Fatalf("create with key: %d %s", status, body)
	}
	// The first file of a new link is allowed: nothing is destroyed.
	a.uploadWithKey(http.StatusOK, "guarded", red, nil, key)

	// Replacing the live file is refused, and the served bytes do not change.
	a.uploadWithKey(http.StatusForbidden, "guarded", blue, nil, key)
	a.uploadWithKey(http.StatusForbidden, "guarded", blue, map[string]string{"autoCreate": "1"}, key)
	if got := a.expect(http.StatusOK, "GET", "/guarded", nil, false, nil); !bytes.Equal(got, red) {
		t.Fatal("publish key replaced the live media of an existing link")
	}

	// Appending a playlist item is still allowed and leaves the live file alone.
	a.uploadWithKey(http.StatusOK, "guarded", green, map[string]string{"mode": "append"}, key)
	if got := a.expect(http.StatusOK, "GET", "/guarded", nil, false, nil); !bytes.Equal(got, red) {
		t.Fatal("append changed the live file")
	}

	// The admin login keeps the power to replace.
	a.uploadFields(http.StatusOK, "guarded", blue, nil)
	if got := a.expect(http.StatusOK, "GET", "/guarded", nil, false, nil); !bytes.Equal(got, blue) {
		t.Fatal("admin replace did not take effect")
	}
}

// The public health probe no longer reports the release version.
func TestHealthDoesNotReportVersion(t *testing.T) {
	a := setupFeatureApp(t)
	body := a.expect(http.StatusOK, "GET", "/health", nil, false, nil)
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("health is not JSON: %v", err)
	}
	if _, ok := out["version"]; ok {
		t.Fatalf("health still reports a version: %s", body)
	}
	if out["status"] != "ok" {
		t.Fatalf("health status = %v", out["status"])
	}
}
