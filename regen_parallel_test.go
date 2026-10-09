// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"net/http"
	"os"
	"testing"

	"lanpaper/handlers"
	"lanpaper/storage"
)

func TestAppRegeneratePreviewsParallel(t *testing.T) {
	a := setupApp(t)
	for i := 0; i < 6; i++ {
		a.uploadFields(http.StatusOK, fmt.Sprintf("pic-%d", i), makePNG(t, color.RGBA{uint8(40 * i), 90, 200, 255}),
			map[string]string{"autoCreate": "true"})
	}
	// A text-only link has nothing to regenerate and is skipped.
	if err := storage.Global.Create(&storage.Wallpaper{LinkName: "text-only"}); err != nil {
		t.Fatal(err)
	}
	// A link whose stored image no longer decodes is reported as failed.
	a.uploadFields(http.StatusOK, "broken", makePNG(t, color.RGBA{1, 2, 3, 255}),
		map[string]string{"autoCreate": "true"})
	if err := os.WriteFile("data/media/broken.png", []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		os.Remove(storage.PreviewFilePath(fmt.Sprintf("pic-%d", i)))
	}

	body := a.expect(http.StatusOK, "POST", "/api/regenerate-previews", nil, true, nil)
	var res handlers.RegeneratePreviewsResult
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("bad response %s: %v", body, err)
	}
	if res.Total != 8 || res.OK != 6 || res.Skipped != 1 || res.Errors != 1 ||
		len(res.Failed) != 1 || res.Failed[0] != "broken" {
		t.Fatalf("result = %+v", res)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(storage.PreviewFilePath(fmt.Sprintf("pic-%d", i))); err != nil {
			t.Fatalf("preview %d not regenerated: %v", i, err)
		}
	}
}
