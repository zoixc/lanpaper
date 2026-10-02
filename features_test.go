package main

import (
	"bytes"
	"encoding/json"
	"image/color"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"lanpaper/config"
	"lanpaper/handlers"
	"lanpaper/storage"
)

// setupFeatureApp enables the version-history and playlist features on top of
// the shared end-to-end harness. setupApp leaves them at their zero values, so
// every pre-existing test keeps running against the old behaviour.
func setupFeatureApp(t *testing.T) *testApp {
	t.Helper()
	a := setupApp(t)
	config.Current.History = config.HistoryConfig{Limit: 3, MaxMB: 512}
	config.Current.PlaylistMax = config.DefaultPlaylistMax
	config.Current.AllowEmbed = false
	config.Current.CORSOrigins = nil
	config.Current.PublishKeys = nil
	config.RefreshDerived()
	// The parsed snapshots are process-wide, so a test that installs publish
	// keys or CORS origins must not hand them to the next one.
	t.Cleanup(func() {
		config.Current.AllowEmbed = false
		config.Current.CORSOrigins = nil
		config.Current.PublishKeys = nil
		config.RefreshDerived()
	})
	// Both counters are process-wide; each test starts from an empty store in a
	// temporary directory, so they have to start from zero too.
	storage.ResetStats()
	storage.ResetHistoryCounters()
	if err := os.MkdirAll(config.ItemsDir, 0755); err != nil {
		t.Fatal(err)
	}
	return a
}

func jsonHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/json"}
}

// buildUploadForm assembles a multipart upload body with extra form fields, so
// the mode and autoCreate options can be exercised.
func buildUploadForm(t *testing.T, linkName string, file []byte, fields map[string]string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("linkName", linkName); err != nil {
		t.Fatal(err)
	}
	for key, value := range fields {
		if err := mw.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := mw.CreateFormFile("file", "picture.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(file); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), mw.FormDataContentType()
}

// uploadFields posts an authenticated upload with extra form fields.
func (a *testApp) uploadFields(want int, name string, file []byte, fields map[string]string) []byte {
	a.t.Helper()
	body, contentType := buildUploadForm(a.t, name, file, fields)
	return a.expect(want, "POST", "/api/upload", body, true, map[string]string{"Content-Type": contentType})
}

// uploadWithKey posts an upload authenticated by a publish key instead of the
// admin login. An empty key sends no credential at all.
func (a *testApp) uploadWithKey(want int, name string, file []byte, fields map[string]string, key string) []byte {
	a.t.Helper()
	body, contentType := buildUploadForm(a.t, name, file, fields)
	headers := map[string]string{"Content-Type": contentType}
	if key != "" {
		headers["X-Api-Key"] = key
	}
	return a.expect(want, "POST", "/api/upload", body, false, headers)
}

// link reads one record back through the admin API.
func (a *testApp) link(name string) handlers.WallpaperResponse {
	a.t.Helper()
	var links []handlers.WallpaperResponse
	if err := json.Unmarshal(a.expect(http.StatusOK, "GET", "/api/wallpapers", nil, true, nil), &links); err != nil {
		a.t.Fatal(err)
	}
	for _, link := range links {
		if link.LinkName == name {
			return link
		}
	}
	a.t.Fatalf("link %s is missing from /api/wallpapers", name)
	return handlers.WallpaperResponse{}
}

// historyBody mirrors GET /api/link/{name}/history.
type historyBody struct {
	LinkName       string                 `json:"linkName"`
	CurrentVersion uint64                 `json:"currentVersion"`
	Live           storage.HistoryEntry   `json:"live"`
	History        []storage.HistoryEntry `json:"history"`
	Limit          int                    `json:"limit"`
	Bytes          int64                  `json:"bytes"`
}

func TestAppVersionHistoryRollbackAndDelete(t *testing.T) {
	a := setupFeatureApp(t)
	config.Current.History.Limit = 2
	red := makePNG(t, color.RGBA{R: 255, A: 255})
	blue := makePNG(t, color.RGBA{B: 255, A: 255})
	green := makePNG(t, color.RGBA{G: 255, A: 255})
	yellow := makePNG(t, color.RGBA{R: 255, G: 255, A: 255})

	a.createLink("wall")
	var first handlers.WallpaperResponse
	if err := json.Unmarshal(a.upload(http.StatusOK, "wall", red, ""), &first); err != nil {
		t.Fatal(err)
	}
	if first.CurrentVersion != 1 || len(first.History) != 0 {
		t.Fatalf("first upload must be version 1 without history: %+v", first)
	}

	// Replacing a file archives the bytes it replaced, at no extra copy.
	var second handlers.WallpaperResponse
	if err := json.Unmarshal(a.upload(http.StatusOK, "wall", blue, ""), &second); err != nil {
		t.Fatal(err)
	}
	if second.CurrentVersion != 2 || len(second.History) != 1 || second.History[0].Version != 1 {
		t.Fatalf("replace did not archive the previous version: %+v", second)
	}
	if got, err := os.ReadFile(filepath.Join(config.HistoryDir, "wall", "1.png")); err != nil || !bytes.Equal(got, red) {
		t.Fatalf("archived file missing or wrong: %v", err)
	}
	if body := a.expect(http.StatusOK, "GET", "/wall", nil, false, nil); !bytes.Equal(body, blue) {
		t.Fatal("the URL must keep serving the newest file")
	}
	if body := a.expect(http.StatusOK, "GET", "/wall?v=1", nil, false, nil); !bytes.Equal(body, red) {
		t.Fatal("?v=1 did not serve the archived version")
	}
	// The archived version keeps its own validators, so a frame can cache it.
	if _, h, _ := a.request("GET", "/wall?v=1", nil, false, nil); h.Get("ETag") == "" {
		t.Fatal("archived version has no ETag")
	}

	var third handlers.WallpaperResponse
	if err := json.Unmarshal(a.upload(http.StatusOK, "wall", green, ""), &third); err != nil {
		t.Fatal(err)
	}
	if third.CurrentVersion != 3 || len(third.History) != 2 {
		t.Fatalf("bad history after the third upload: %+v", third)
	}
	var fourth handlers.WallpaperResponse
	if err := json.Unmarshal(a.upload(http.StatusOK, "wall", yellow, ""), &fourth); err != nil {
		t.Fatal(err)
	}
	if fourth.CurrentVersion != 4 || len(fourth.History) != 2 || fourth.History[0].Version != 3 {
		t.Fatalf("history was not trimmed to the limit: %+v", fourth)
	}
	a.expect(http.StatusNotFound, "GET", "/wall?v=1", nil, false, nil)
	if _, err := os.Stat(filepath.Join(config.HistoryDir, "wall", "1.png")); !os.IsNotExist(err) {
		t.Fatalf("the trimmed version file was not deleted: %v", err)
	}
	if body := a.expect(http.StatusOK, "GET", "/wall?v=2", nil, false, nil); !bytes.Equal(body, blue) {
		t.Fatal("?v=2 must still serve the second version")
	}

	// A rollback swaps the archived file back in and archives what was live, so
	// the rollback itself can be rolled back.
	var rolled handlers.WallpaperResponse
	if err := json.Unmarshal(a.expect(http.StatusOK, "POST", "/api/link/wall/rollback",
		[]byte(`{"version":2}`), true, jsonHeaders()), &rolled); err != nil {
		t.Fatal(err)
	}
	if rolled.CurrentVersion != 5 || rolled.MIMEType != "png" || !rolled.HasImage {
		t.Fatalf("bad rollback response: %+v", rolled)
	}
	if body := a.expect(http.StatusOK, "GET", "/wall", nil, false, nil); !bytes.Equal(body, blue) {
		t.Fatal("rollback did not restore the requested version")
	}
	if body := a.expect(http.StatusOK, "GET", "/wall?v=4", nil, false, nil); !bytes.Equal(body, yellow) {
		t.Fatal("the file that was live is not archived by the rollback")
	}
	a.expect(http.StatusNotFound, "GET", "/wall?v=2", nil, false, nil)
	if _, h, preview := a.request("GET", "/api/preview/wall", nil, true, nil); h.Get("Content-Type") != "image/webp" || len(preview) < 16 {
		t.Fatalf("rollback left no thumbnail: %v", h)
	}

	var listed historyBody
	if err := json.Unmarshal(a.expect(http.StatusOK, "GET", "/api/link/wall/history", nil, true, nil), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.LinkName != "wall" || listed.CurrentVersion != 5 || listed.Live.Version != 5 || listed.Live.Ext != "png" {
		t.Fatalf("bad history listing: %+v", listed)
	}
	if len(listed.History) != 2 || listed.Bytes <= 0 || listed.Limit != 2 {
		t.Fatalf("bad history listing body: %+v", listed)
	}

	// A single version can be dropped to reclaim its disk space.
	var dropped handlers.WallpaperResponse
	if err := json.Unmarshal(a.expect(http.StatusOK, "DELETE", "/api/link/wall/history/4", nil, true, nil), &dropped); err != nil {
		t.Fatal(err)
	}
	if len(dropped.History) != 1 {
		t.Fatalf("version was not removed from the record: %+v", dropped)
	}
	if _, err := os.Stat(filepath.Join(config.HistoryDir, "wall", "4.png")); !os.IsNotExist(err) {
		t.Fatalf("the deleted version file is still on disk: %v", err)
	}
	a.expect(http.StatusNotFound, "GET", "/wall?v=4", nil, false, nil)
	a.expect(http.StatusNotFound, "POST", "/api/link/wall/rollback", []byte(`{"version":4}`), true, jsonHeaders())
	a.expect(http.StatusNotFound, "DELETE", "/api/link/wall/history/4", nil, true, nil)
	a.expect(http.StatusBadRequest, "POST", "/api/link/wall/rollback", []byte(`{"version":0}`), true, jsonHeaders())
	a.expect(http.StatusBadRequest, "POST", "/api/link/wall/rollback", []byte(`{"version":"x"}`), true, jsonHeaders())

	// Removing the last version also removes the directory it lived in.
	a.expect(http.StatusOK, "DELETE", "/api/link/wall/history/3", nil, true, nil)
	if _, err := os.Stat(filepath.Join(config.HistoryDir, "wall")); !os.IsNotExist(err) {
		t.Fatalf("the history directory survived: %v", err)
	}
	if left := storage.HistoryBytesTotal(); left != 0 {
		t.Fatalf("archive byte counter drifted to %d", left)
	}

	// Renaming a link moves its archive with it.
	a.upload(http.StatusOK, "wall", red, "")
	a.upload(http.StatusOK, "wall", green, "")
	var before historyBody
	if err := json.Unmarshal(a.expect(http.StatusOK, "GET", "/api/link/wall/history", nil, true, nil), &before); err != nil {
		t.Fatal(err)
	}
	if len(before.History) == 0 {
		t.Fatal("nothing archived to move")
	}
	// The newest archive holds the red upload that the green one replaced.
	archived := before.History[0].Version
	a.expect(http.StatusOK, "PATCH", "/api/link/wall", []byte(`{"newLinkName":"moved"}`), true, jsonHeaders())
	if _, err := os.Stat(filepath.Join(config.HistoryDir, "moved")); err != nil {
		t.Fatalf("history directory was not renamed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.HistoryDir, "wall")); !os.IsNotExist(err) {
		t.Fatalf("the old history directory survived the rename: %v", err)
	}
	if body := a.expect(http.StatusOK, "GET", "/moved?v="+strconv.FormatUint(archived, 10), nil, false, nil); !bytes.Equal(body, red) {
		t.Fatal("archived version is unreachable after a rename")
	}
	a.expect(http.StatusNotFound, "GET", "/wall", nil, false, nil)

	// Deleting a link deletes its archive.
	a.expect(http.StatusNoContent, "DELETE", "/api/link/moved", nil, true, nil)
	if _, err := os.Stat(filepath.Join(config.HistoryDir, "moved")); !os.IsNotExist(err) {
		t.Fatalf("history directory survived link deletion: %v", err)
	}
}

func TestAppHistoryDisabledKeepsOldBehaviour(t *testing.T) {
	a := setupApp(t) // HISTORY_LIMIT stays 0
	red := makePNG(t, color.RGBA{R: 255, A: 255})
	blue := makePNG(t, color.RGBA{B: 255, A: 255})
	a.createLink("plain")
	a.upload(http.StatusOK, "plain", red, "")
	var replaced handlers.WallpaperResponse
	if err := json.Unmarshal(a.upload(http.StatusOK, "plain", blue, ""), &replaced); err != nil {
		t.Fatal(err)
	}
	if len(replaced.History) != 0 {
		t.Fatalf("history was written although it is disabled: %+v", replaced)
	}
	if _, err := os.Stat(config.HistoryDir); !os.IsNotExist(err) {
		t.Fatalf("disabled history created %s: %v", config.HistoryDir, err)
	}
	// The version the URL serves is always addressable, so ?v=1 keeps working;
	// anything else is a 404 because nothing was archived.
	if body := a.expect(http.StatusOK, "GET", "/plain?v=1", nil, false, nil); !bytes.Equal(body, blue) {
		t.Fatal("?v=1 must serve the live file")
	}
	a.expect(http.StatusNotFound, "GET", "/plain?v=2", nil, false, nil)
	a.expect(http.StatusNotFound, "GET", "/plain?v=0", nil, false, nil)
	var listed historyBody
	if err := json.Unmarshal(a.expect(http.StatusOK, "GET", "/api/link/plain/history", nil, true, nil), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Limit != 0 || len(listed.History) != 0 || listed.Live.Version != 1 {
		t.Fatalf("bad listing for a link without history: %+v", listed)
	}
}

func TestAppPlaylistAppendRotateAndRemove(t *testing.T) {
	a := setupFeatureApp(t)
	red := makePNG(t, color.RGBA{R: 255, A: 255})
	blue := makePNG(t, color.RGBA{B: 255, A: 255})
	green := makePNG(t, color.RGBA{G: 255, A: 255})

	a.createLink("frame")
	a.createLink("empty")
	a.upload(http.StatusOK, "frame", red, "")

	// Appending leaves the live file — and therefore every existing embed —
	// exactly as it was.
	var appended handlers.WallpaperResponse
	if err := json.Unmarshal(a.uploadFields(http.StatusOK, "frame", blue, map[string]string{"mode": "append"}), &appended); err != nil {
		t.Fatal(err)
	}
	if len(appended.Items) != 1 || appended.Items[0].ID != 1 || appended.Items[0].Ext != "png" {
		t.Fatalf("bad append response: %+v", appended)
	}
	if appended.CurrentVersion != 1 || len(appended.History) != 0 {
		t.Fatalf("append must not touch the version history: %+v", appended)
	}
	if body := a.expect(http.StatusOK, "GET", "/frame", nil, false, nil); !bytes.Equal(body, red) {
		t.Fatal("append changed what the URL serves")
	}
	if got, err := os.ReadFile(filepath.Join(config.ItemsDir, "frame", "1.png")); err != nil || !bytes.Equal(got, blue) {
		t.Fatalf("playlist item was not stored: %v", err)
	}
	// A playlist item is addressable by id; an unknown id or a conflicting pair
	// of selectors is a 404, never a silent fallback.
	if body := a.expect(http.StatusOK, "GET", "/frame?i=1", nil, false, nil); !bytes.Equal(body, blue) {
		t.Fatal("?i=1 did not serve the playlist item")
	}
	if body := a.expect(http.StatusOK, "GET", "/frame?i=0", nil, false, nil); !bytes.Equal(body, red) {
		t.Fatal("?i=0 must serve the live file")
	}
	a.expect(http.StatusNotFound, "GET", "/frame?i=7", nil, false, nil)
	a.expect(http.StatusNotFound, "GET", "/frame?i=-1", nil, false, nil)
	a.expect(http.StatusNotFound, "GET", "/frame?i=x", nil, false, nil)
	a.expect(http.StatusNotFound, "GET", "/frame?v=1&i=1", nil, false, nil)

	// Appending needs media to append to, and respects the configured cap.
	a.uploadFields(http.StatusBadRequest, "empty", red, map[string]string{"mode": "append"})
	a.uploadFields(http.StatusBadRequest, "frame", red, map[string]string{"mode": "nope"})
	config.Current.PlaylistMax = 1
	a.uploadFields(http.StatusConflict, "frame", green, map[string]string{"mode": "append"})
	config.Current.PlaylistMax = config.DefaultPlaylistMax
	a.uploadFields(http.StatusOK, "frame", green, map[string]string{"mode": "append"})

	// Rotation stays off until it is switched on.
	var patched handlers.WallpaperResponse
	if err := json.Unmarshal(a.expect(http.StatusOK, "PATCH", "/api/link/frame",
		[]byte(`{"rotate":{"enabled":true,"interval":60}}`), true, jsonHeaders()), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Rotate == nil || !patched.Rotate.Enabled || patched.Rotate.Interval != 60 ||
		patched.Rotate.Order != config.RotateOrderSequential {
		t.Fatalf("rotation settings were not stored: %+v", patched.Rotate)
	}
	wp, ok := storage.Global.Get("frame")
	if !ok {
		t.Fatal("link disappeared")
	}
	want := wp.PlaylistNow()
	served := a.expect(http.StatusOK, "GET", "/frame", nil, false, nil)
	if expected := [][]byte{red, blue, green}[want]; !bytes.Equal(served, expected) {
		t.Fatalf("rotation served the wrong item (index %d)", want)
	}
	// A pinned item survives rotation, which is what a "show this one" link needs.
	pinned := a.expect(http.StatusOK, "GET", "/frame?i=2", nil, false, nil)
	if !bytes.Equal(pinned, green) {
		t.Fatal("?i= must ignore rotation")
	}

	// Removing an item deletes its file and its URL.
	var removed handlers.WallpaperResponse
	if err := json.Unmarshal(a.expect(http.StatusOK, "PATCH", "/api/link/frame",
		[]byte(`{"removeItem":1}`), true, jsonHeaders()), &removed); err != nil {
		t.Fatal(err)
	}
	if len(removed.Items) != 1 || removed.Items[0].ID != 2 {
		t.Fatalf("bad removal response: %+v", removed)
	}
	if _, err := os.Stat(filepath.Join(config.ItemsDir, "frame", "1.png")); !os.IsNotExist(err) {
		t.Fatalf("the removed item file is still on disk: %v", err)
	}
	a.expect(http.StatusNotFound, "GET", "/frame?i=1", nil, false, nil)
	a.expect(http.StatusNotFound, "PATCH", "/api/link/frame", []byte(`{"removeItem":9}`), true, jsonHeaders())
	a.expect(http.StatusBadRequest, "PATCH", "/api/link/frame", []byte(`{"removeItem":0}`), true, jsonHeaders())
	a.expect(http.StatusBadRequest, "PATCH", "/api/link/frame", []byte(`{"rotate":{"order":"shuffle"}}`), true, jsonHeaders())
	a.expect(http.StatusBadRequest, "PATCH", "/api/link/frame", []byte(`{"rotate":{"interval":1}}`), true, jsonHeaders())

	// Deleting the link removes the whole playlist directory.
	a.expect(http.StatusNoContent, "DELETE", "/api/link/frame", nil, true, nil)
	if _, err := os.Stat(filepath.Join(config.ItemsDir, "frame")); !os.IsNotExist(err) {
		t.Fatalf("playlist directory survived link deletion: %v", err)
	}
}

func TestAppUploadAutoCreate(t *testing.T) {
	a := setupFeatureApp(t)
	red := makePNG(t, color.RGBA{R: 255, A: 255})

	// Without the flag an unknown name is still rejected, so a typo cannot
	// silently create a link.
	a.uploadFields(http.StatusBadRequest, "missing", red, nil)
	if _, exists := storage.Global.Get("missing"); exists {
		t.Fatal("a rejected upload created a link")
	}

	var created handlers.WallpaperResponse
	if err := json.Unmarshal(a.uploadFields(http.StatusOK, "hook", red, map[string]string{"autoCreate": "1"}), &created); err != nil {
		t.Fatal(err)
	}
	if created.LinkName != "hook" || !created.HasImage || created.ImageURL != "/hook" || created.AccessLevel != config.AccessPublic {
		t.Fatalf("bad auto-created link: %+v", created)
	}
	if body := a.expect(http.StatusOK, "GET", "/hook", nil, false, nil); !bytes.Equal(body, red) {
		t.Fatal("auto-created link does not serve its media")
	}
	// The requested access level and category are validated, not trusted.
	a.uploadFields(http.StatusBadRequest, "scoped", red, map[string]string{"autoCreate": "1", "accessLevel": "root"})
	a.uploadFields(http.StatusBadRequest, "scoped", red, map[string]string{"autoCreate": "1", "category": "nope"})
	var tokened handlers.WallpaperResponse
	if err := json.Unmarshal(a.uploadFields(http.StatusOK, "scoped", red,
		map[string]string{"autoCreate": "1", "accessLevel": "token"}), &tokened); err != nil {
		t.Fatal(err)
	}
	if tokened.AccessLevel != config.AccessToken || tokened.AccessToken == "" {
		t.Fatalf("auto-created token link has no token: %+v", tokened)
	}
	a.expect(http.StatusForbidden, "GET", "/scoped", nil, false, nil)
	if body := a.expect(http.StatusOK, "GET", "/scoped?token="+tokened.AccessToken, nil, false, nil); !bytes.Equal(body, red) {
		t.Fatal("token link does not serve its media")
	}
}

func TestAppPublicAliasesCORSEmbedAndStats(t *testing.T) {
	a := setupFeatureApp(t)
	red := makePNG(t, color.RGBA{R: 255, A: 255})
	a.createLink("photo")
	a.upload(http.StatusOK, "photo", red, "")

	// A cosmetic extension or /latest resolves to the same link, and the stored
	// media type — never the URL — decides the Content-Type.
	for _, path := range []string{"/photo", "/photo.png", "/photo.jpg", "/photo/latest", "/photo.PNG/", "/photo.jpg/latest"} {
		status, h, body := a.request("GET", path, nil, false, nil)
		if status != http.StatusOK || !bytes.Equal(body, red) {
			t.Fatalf("%s: status %d, %d bytes", path, status, len(body))
		}
		if h.Get("Content-Type") != "image/png" || h.Get("Content-Disposition") != `inline; filename="photo.png"` {
			t.Fatalf("%s: wrong media headers: %v", path, h)
		}
	}
	// Reserved names and non-media extensions keep resolving exactly as before.
	for _, path := range []string{"/manifest.json", "/favicon.ico", "/robots.txt", "/sitemap.xml", "/photo.txt", "/photo/other", "/api/nope", "/static/photo.png"} {
		a.expect(http.StatusNotFound, "GET", path, nil, false, nil)
	}
	// An alias cannot bypass the access level of the link it resolves to.
	a.expect(http.StatusOK, "PATCH", "/api/link/photo", []byte(`{"accessLevel":"auth"}`), true, jsonHeaders())
	a.expect(http.StatusUnauthorized, "GET", "/photo.jpg", nil, false, nil)
	a.expect(http.StatusOK, "PATCH", "/api/link/photo", []byte(`{"accessLevel":"public"}`), true, jsonHeaders())

	// OPTIONS stays 405 until CORS is configured.
	a.expect(http.StatusMethodNotAllowed, "OPTIONS", "/photo", nil, false, map[string]string{"Origin": "https://frame.example"})

	config.Current.CORSOrigins = []string{"https://frame.example"}
	config.RefreshDerived()
	if _, h, _ := a.request("GET", "/photo", nil, false, map[string]string{"Origin": "https://frame.example"}); h.Get("Access-Control-Allow-Origin") != "https://frame.example" ||
		h.Get("Vary") != "Origin" || h.Get("Access-Control-Expose-Headers") == "" {
		t.Fatalf("CORS headers missing: %v", h)
	}
	if _, h, _ := a.request("GET", "/photo", nil, false, map[string]string{"Origin": "https://frame.example"}); h.Get("X-Frame-Options") != "DENY" ||
		h.Get("Cross-Origin-Resource-Policy") != "cross-origin" {
		t.Fatalf("CORS changed unrelated hardening: %v", h)
	}
	if _, h, _ := a.request("GET", "/photo", nil, false, map[string]string{"Origin": "https://evil.example"}); h.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("an unlisted origin got CORS headers: %v", h)
	}
	if _, h, _ := a.request("GET", "/photo", nil, false, nil); h.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("a request without Origin got CORS headers: %v", h)
	}
	status, h, body := a.request("OPTIONS", "/photo", nil, false, map[string]string{
		"Origin":                         "https://frame.example",
		"Access-Control-Request-Method":  "GET",
		"Access-Control-Request-Headers": "range, x-access-token, x-evil",
	})
	if status != http.StatusNoContent || len(body) != 0 {
		t.Fatalf("preflight: %d, %d bytes", status, len(body))
	}
	if h.Get("Access-Control-Allow-Methods") != "GET, HEAD, OPTIONS" || h.Get("Access-Control-Allow-Headers") != "Range, X-Access-Token" {
		t.Fatalf("preflight headers wrong: %v", h)
	}
	if h.Get("Access-Control-Max-Age") != "600" {
		t.Fatalf("preflight is not cacheable: %v", h)
	}
	a.expect(http.StatusMethodNotAllowed, "OPTIONS", "/photo", nil, false, map[string]string{"Origin": "https://evil.example"})

	// ALLOW_EMBED relaxes exactly the two directives that block framing.
	config.Current.AllowEmbed = true
	if _, h, _ := a.request("GET", "/photo", nil, false, nil); h.Get("X-Frame-Options") != "" ||
		h.Get("Content-Security-Policy") != "default-src 'none'" {
		t.Fatalf("embed mode did not relax framing: %v", h)
	}
	if _, h, _ := a.request("GET", "/photo", nil, false, nil); h.Get("X-Content-Type-Options") != "nosniff" ||
		h.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("embed mode dropped unrelated hardening: %v", h)
	}
	// The admin panel is never framable, whatever ALLOW_EMBED says.
	if _, h, _ := a.request("GET", "/admin", nil, true, nil); h.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("the admin panel became framable: %v", h)
	}
	config.Current.AllowEmbed = false

	// Statistics count what was really delivered, and nothing else. The requests
	// above already served this link, so the counters start from a clean slate.
	storage.ResetStats()
	if stats := a.link("photo").Stats; stats != nil {
		t.Fatalf("stats appeared before the link was served: %+v", stats)
	}
	_, h, _ := a.request("GET", "/photo", nil, false, nil)
	etag := h.Get("ETag")
	if etag == "" {
		t.Fatal("no ETag to revalidate with")
	}
	a.request("GET", "/photo", nil, false, map[string]string{"Range": "bytes=0-3"})
	a.request("HEAD", "/photo", nil, false, nil)
	a.expect(http.StatusNotModified, "GET", "/photo", nil, false, map[string]string{"If-None-Match": etag})
	a.expect(http.StatusNotFound, "GET", "/photo?i=9", nil, false, nil)
	stats := a.link("photo").Stats
	if stats == nil || stats.Hits != 4 {
		t.Fatalf("bad hit count: %+v", stats)
	}
	if want := int64(len(red)) + 4; stats.Bytes != want {
		t.Fatalf("byte counter = %d, want %d (full body + range)", stats.Bytes, want)
	}
	if stats.LastHit == 0 {
		t.Fatal("last hit was not recorded")
	}

	// A request that is refused is not a hit.
	a.createLink("secret")
	a.expect(http.StatusOK, "PATCH", "/api/link/secret", []byte(`{"accessLevel":"auth"}`), true, jsonHeaders())
	a.upload(http.StatusOK, "secret", red, "")
	a.expect(http.StatusUnauthorized, "GET", "/secret", nil, false, nil)
	if stats := a.link("secret").Stats; stats != nil {
		t.Fatalf("a refused request was counted: %+v", stats)
	}
}

func TestAppPublishKeys(t *testing.T) {
	a := setupFeatureApp(t)
	const key = "publish-key-for-tests-0123456789"
	config.Current.PublishKeys = []string{key, "too-short"}
	config.RefreshDerived()
	red := makePNG(t, color.RGBA{R: 255, A: 255})
	blue := makePNG(t, color.RGBA{B: 255, A: 255})

	// A publish key can create a link and push media into it with no admin login.
	status, _, body := a.request("POST", "/api/link", []byte(`{"linkName":"pushed"}`), false, map[string]string{
		"Content-Type": "application/json", "X-Api-Key": key,
	})
	if status != http.StatusCreated {
		t.Fatalf("create with publish key: %d %s", status, body)
	}
	a.uploadWithKey(http.StatusOK, "pushed", red, nil, key)
	if got := a.expect(http.StatusOK, "GET", "/pushed", nil, false, nil); !bytes.Equal(got, red) {
		t.Fatal("media published with a key is not served")
	}
	// The Bearer form works too, and so does autoCreate in one request.
	status, _, body = a.request("POST", "/api/link", []byte(`{"linkName":"bearer"}`), false, map[string]string{
		"Content-Type": "application/json", "Authorization": "Bearer " + key,
	})
	if status != http.StatusCreated {
		t.Fatalf("create with bearer key: %d %s", status, body)
	}
	a.uploadWithKey(http.StatusOK, "one-shot", blue, map[string]string{"autoCreate": "1"}, key)
	if got := a.expect(http.StatusOK, "GET", "/one-shot", nil, false, nil); !bytes.Equal(got, blue) {
		t.Fatal("auto-created publish did not work")
	}

	// A key is not an admin session: everything that can expose or destroy the
	// library still needs the login.
	a.expect(http.StatusUnauthorized, "PATCH", "/api/link/pushed", []byte(`{"accessLevel":"public"}`), false,
		map[string]string{"Content-Type": "application/json", "X-Api-Key": key})
	a.expect(http.StatusUnauthorized, "DELETE", "/api/link/pushed", nil, false, map[string]string{"X-Api-Key": key})
	a.expect(http.StatusUnauthorized, "POST", "/api/link/pushed/pin", nil, false, map[string]string{"X-Api-Key": key})
	a.expect(http.StatusUnauthorized, "POST", "/api/link/pushed/rollback", []byte(`{"version":1}`), false,
		map[string]string{"Content-Type": "application/json", "X-Api-Key": key})
	a.expect(http.StatusUnauthorized, "GET", "/api/wallpapers", nil, false, map[string]string{"X-Api-Key": key})
	if _, exists := storage.Global.Get("pushed"); !exists {
		t.Fatal("a rejected admin operation deleted the link")
	}
	// The admin login still works on the same routes.
	a.expect(http.StatusOK, "PATCH", "/api/link/pushed", []byte(`{"category":"tech"}`), true, jsonHeaders())

	// A key that was rejected at parse time (too short) is not a credential, and
	// neither is a wrong one. Nothing is accepted without a key either.
	a.uploadWithKey(http.StatusUnauthorized, "pushed", blue, nil, "too-short")
	a.uploadWithKey(http.StatusUnauthorized, "pushed", blue, nil, "wrong-key-but-long-enough")
	a.uploadWithKey(http.StatusUnauthorized, "pushed", blue, nil, "")

	// Guessing is rate limited with the admin brute-force budget, and the lockout
	// is not an oracle: even the right key is refused while it lasts. The wrong
	// keys above already consumed part of the budget, so the loop accepts either
	// answer until the window is exhausted.
	locked := 0
	for range 20 {
		status, h, _ := a.request("POST", "/api/upload", nil, false,
			map[string]string{"X-Api-Key": "wrong-key-but-long-enough"})
		switch status {
		case http.StatusUnauthorized:
		case http.StatusTooManyRequests:
			if h.Get("Retry-After") == "" {
				t.Fatalf("a lockout without Retry-After: %v", h)
			}
			locked++
		default:
			t.Fatalf("guessing a publish key: %d", status)
		}
	}
	if locked == 0 {
		t.Fatal("publish key guessing is not rate limited")
	}
	a.uploadWithKey(http.StatusTooManyRequests, "pushed", blue, nil, key)
	// Public links keep working while a publisher is locked out.
	if got := a.expect(http.StatusOK, "GET", "/pushed", nil, false, nil); !bytes.Equal(got, red) {
		t.Fatal("a publish-key lockout broke public serving")
	}
}
