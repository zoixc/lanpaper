package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/chai2010/webp"

	"lanpaper/config"
	"lanpaper/handlers"
	"lanpaper/storage"
)

type testApp struct {
	t       *testing.T
	server  *httptest.Server
	client  *http.Client
	rootDir string
}

func setupApp(t *testing.T) *testApp {
	t.Helper()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rootDir := t.TempDir()
	if err := os.Chdir(rootDir); err != nil {
		t.Fatal(err)
	}
	originalConfig, originalStore := config.Current, storage.Global
	t.Cleanup(func() {
		config.Current, storage.Global = originalConfig, originalStore
		if err := os.Chdir(originalDir); err != nil {
			t.Error(err)
		}
	})
	config.Current = config.Config{
		AdminUser: "admin", AdminPass: "strong-test-password", MaxUploadMB: 2,
		MaxConcurrentUploads: 2, ExternalImageDir: "external/images", MaxWalkDepth: 3,
		Compression: config.CompressionConfig{Quality: 100, Scale: 100},
		Rate:        config.RateConfig{PublicPerMin: 10000, UploadPerMin: 10000, Burst: 100},
	}
	storage.Global = &storage.Store{}
	handlers.InitUploadSemaphore(config.Current.MaxConcurrentUploads)
	for _, dir := range []string{"data/media", "data/previews", "external/images", "static/css", "static/images", "static/i18n"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"admin.html", "static/sw.js", "static/css/style.css", "static/i18n/en.json"} {
		src, err := os.ReadFile(filepath.Join(originalDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, src, 0644); err != nil {
			t.Fatal(err)
		}
	}
	srv := httptest.NewServer(newMux())
	t.Cleanup(srv.Close)
	return &testApp{t: t, server: srv, client: srv.Client(), rootDir: rootDir}
}

func (a *testApp) request(method, path string, body []byte, auth bool, headers map[string]string) (int, http.Header, []byte) {
	a.t.Helper()
	req, err := http.NewRequest(method, a.server.URL+path, bytes.NewReader(body))
	if err != nil {
		a.t.Fatal(err)
	}
	if auth {
		req.SetBasicAuth("admin", "strong-test-password")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer resp.Body.Close()
	result, err := io.ReadAll(resp.Body)
	if err != nil {
		a.t.Fatal(err)
	}
	return resp.StatusCode, resp.Header, result
}

func (a *testApp) expect(want int, method, path string, body []byte, auth bool, headers map[string]string) []byte {
	a.t.Helper()
	status, _, result := a.request(method, path, body, auth, headers)
	if status != want {
		a.t.Fatalf("%s %s: status %d, want %d: %s", method, path, status, want, result)
	}
	return result
}

func (a *testApp) createLink(name string) handlers.WallpaperResponse {
	a.t.Helper()
	result := a.expect(http.StatusCreated, "POST", "/api/link", []byte(`{"linkName":"`+name+`"}`), true,
		map[string]string{"Content-Type": "application/json"})
	var wp handlers.WallpaperResponse
	if err := json.Unmarshal(result, &wp); err != nil {
		a.t.Fatal(err)
	}
	return wp
}

func makePNG(t *testing.T, colorValue color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.Set(x, y, colorValue)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func multipartUpload(t *testing.T, linkName string, file []byte, url string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("linkName", linkName); err != nil {
		t.Fatal(err)
	}
	if url != "" {
		if err := mw.WriteField("url", url); err != nil {
			t.Fatal(err)
		}
	} else {
		w, err := mw.CreateFormFile("file", "picture.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(file); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), mw.FormDataContentType()
}

func (a *testApp) upload(want int, name string, file []byte, url string) []byte {
	a.t.Helper()
	body, contentType := multipartUpload(a.t, name, file, url)
	return a.expect(want, "POST", "/api/upload", body, true, map[string]string{"Content-Type": contentType})
}

func TestAppAuthenticationStaticAssetsAndCSRF(t *testing.T) {
	a := setupApp(t)
	for _, route := range []string{"/admin", "/api/wallpapers", "/api/preview/x"} {
		a.expect(http.StatusUnauthorized, "GET", route, nil, false, nil)
	}
	a.expect(http.StatusForbidden, "POST", "/api/link", []byte(`{"linkName":"wrong"}`), true,
		map[string]string{"Content-Type": "application/json", "Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"})
	a.expect(http.StatusForbidden, "POST", "/api/link", []byte(`{"linkName":"wrong"}`), true,
		map[string]string{"Content-Type": "application/json", "Origin": a.server.URL + "9"})
	if _, ok := storage.Global.Get("wrong"); ok {
		t.Fatal("CSRF request mutated the store")
	}
	_, h, admin := a.request("GET", "/admin", nil, true, nil)
	if !bytes.Contains(admin, []byte("/static/js/app.js")) || h.Get("Cache-Control") != "no-store" {
		t.Fatalf("admin UI missing scripts or no-store: headers=%v", h)
	}
	status, h, css := a.request("GET", "/static/css/style.css", nil, false, nil)
	if status != http.StatusOK || len(css) < 100 || h.Get("Cross-Origin-Resource-Policy") != "same-origin" {
		t.Fatalf("static CSS unavailable or insecure: %d %v", status, h)
	}
	for _, path := range []string{"/static/images/secret.png", "/static/does-not-exist.css", "/static/.gitkeep"} {
		a.expect(http.StatusNotFound, "GET", path, nil, false, nil)
	}
	if err := os.WriteFile("static/images/secret.png", []byte("do not serve"), 0644); err != nil {
		t.Fatal(err)
	}
	a.expect(http.StatusNotFound, "GET", "/static/images/secret.png", nil, false, nil)
	if err := os.Remove("static/css/style.css"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../data/wallpapers.json", "static/css/style.css"); err != nil {
		t.Fatal(err)
	}
	a.expect(http.StatusNotFound, "GET", "/static/css/style.css", nil, false, nil)
	_, h, sw := a.request("GET", "/sw.js", nil, false, nil)
	// Match any cache generation: the version bumps whenever the precache
	// list changes, and the tests must not track that number by hand.
	if !bytes.Contains(sw, []byte("lanpaper-static-v")) || h.Get("Service-Worker-Allowed") != "/" {
		t.Fatalf("service worker missing or wrong scope: %v", h)
	}
	a.expect(http.StatusMethodNotAllowed, "POST", "/static/sw.js", nil, false, nil)
}

func TestAppWebPUploadPreservesOriginalAndCompressedMode(t *testing.T) {
	a := setupApp(t)
	a.createLink("webp-picture")
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	img.SetRGBA(0, 0, color.RGBA{R: 120, G: 60, A: 128})
	var encoded bytes.Buffer
	if err := webp.Encode(&encoded, img, &webp.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	original := encoded.Bytes()
	var uploaded handlers.WallpaperResponse
	if err := json.Unmarshal(a.upload(http.StatusOK, "webp-picture", original, ""), &uploaded); err != nil {
		t.Fatal(err)
	}
	if uploaded.MIMEType != "webp" {
		t.Fatalf("WebP MIME changed in lossless mode: %+v", uploaded)
	}
	status, h, body := a.request("GET", "/webp-picture", nil, false, nil)
	if status != http.StatusOK || h.Get("Content-Type") != "image/webp" || !bytes.Equal(body, original) {
		t.Fatalf("WebP bytes not preserved: HTTP %d, %v", status, h)
	}

	config.Current.Compression.Quality = 85
	a.upload(http.StatusOK, "webp-picture", original, "")
	status, h, body = a.request("GET", "/webp-picture", nil, false, nil)
	if status != http.StatusOK || h.Get("Content-Type") != "image/webp" || len(body) < 16 {
		t.Fatalf("compressed WebP unavailable: HTTP %d, %v", status, h)
	}
	status, h, preview := a.request("GET", "/api/preview/webp-picture", nil, true, nil)
	if status != http.StatusOK || h.Get("Content-Type") != "image/webp" || len(preview) < 16 {
		t.Fatalf("compressed WebP thumbnail unavailable: HTTP %d, %v", status, h)
	}
}

func TestAppUploadAccessRenameAndDelete(t *testing.T) {
	a := setupApp(t)
	red := makePNG(t, color.RGBA{R: 255, A: 255})
	created := a.createLink("photo")
	if created.LinkName != "photo" || created.HasImage {
		t.Fatalf("bad link: %+v", created)
	}
	var uploaded handlers.WallpaperResponse
	if err := json.Unmarshal(a.upload(http.StatusOK, "photo", red, ""), &uploaded); err != nil {
		t.Fatal(err)
	}
	if !uploaded.HasImage || uploaded.Preview != "/api/preview/photo" || uploaded.MIMEType != "png" {
		t.Fatalf("bad upload response: %+v", uploaded)
	}
	body := a.expect(http.StatusOK, "GET", "/photo", nil, false, nil)
	if !bytes.Equal(body, red) {
		t.Fatal("lossless mode changed uploaded PNG")
	}
	status, h, _ := a.request("HEAD", "/photo", nil, false, nil)
	if status != http.StatusOK || h.Get("Cache-Control") != "no-store" || h.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("public media must not be cached/leak tokens: %d %v", status, h)
	}
	status, _, chunk := a.request("GET", "/photo", nil, false, map[string]string{"Range": "bytes=0-3"})
	if status != http.StatusPartialContent || !bytes.Equal(chunk, red[:4]) {
		t.Fatalf("range failed: %d %v", status, chunk)
	}
	_, h, preview := a.request("GET", "/api/preview/photo", nil, true, nil)
	if h.Get("Content-Type") != "image/webp" || len(preview) < 16 {
		t.Fatalf("missing thumbnail: %v, %d bytes", h, len(preview))
	}
	a.expect(http.StatusUnauthorized, "GET", "/api/preview/photo", nil, false, nil)
	a.expect(http.StatusNotFound, "GET", "/static/images/photo.png", nil, false, nil)

	var tokenLink handlers.WallpaperResponse
	raw := a.expect(http.StatusOK, "PATCH", "/api/link/photo", []byte(`{"accessLevel":"token"}`), true,
		map[string]string{"Content-Type": "application/json", "Origin": a.server.URL})
	if err := json.Unmarshal(raw, &tokenLink); err != nil || tokenLink.AccessToken == "" {
		t.Fatalf("token not generated: %s %v", raw, err)
	}
	a.expect(http.StatusForbidden, "GET", "/photo", nil, false, nil)
	a.expect(http.StatusOK, "GET", "/photo?token="+tokenLink.AccessToken, nil, false, nil)
	a.expect(http.StatusOK, "GET", "/photo", nil, true, nil)

	a.expect(http.StatusOK, "PATCH", "/api/link/photo", []byte(`{"accessLevel":"auth"}`), true,
		map[string]string{"Content-Type": "application/json"})
	a.expect(http.StatusUnauthorized, "GET", "/photo", nil, false, nil)
	a.expect(http.StatusOK, "GET", "/photo", nil, true, nil)
	a.expect(http.StatusOK, "POST", "/api/link/photo/pin", nil, true, nil)
	a.expect(http.StatusOK, "PATCH", "/api/link/photo", []byte(`{"newLinkName":"other"}`), true,
		map[string]string{"Content-Type": "application/json"})
	a.expect(http.StatusNotFound, "GET", "/photo", nil, true, nil)
	a.expect(http.StatusOK, "GET", "/other", nil, true, nil)
	a.expect(http.StatusNoContent, "DELETE", "/api/link/other", nil, true, nil)
	a.expect(http.StatusNotFound, "GET", "/other", nil, true, nil)
	if _, err := os.Stat("data/media/other.png"); !os.IsNotExist(err) {
		t.Fatalf("media left after deletion: %v", err)
	}
}

// Turn the metadata file into a directory: the atomic JSON rename must fail
// AFTER the new image has been staged/published. The old bytes and in-memory
// record must be restored before the 500 response is sent.
func blockMetadata(t *testing.T) func() {
	t.Helper()
	path := filepath.Join("data", "wallpapers.json")
	backup := path + ".bak"
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	restore := func() {
		once.Do(func() {
			if err := os.Remove(path); err != nil {
				t.Error(err)
			}
			if err := os.Rename(backup, path); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(restore)
	return restore
}

func TestAppRollsBackOnMetadataFailures(t *testing.T) {
	a := setupApp(t)
	a.createLink("keep")
	red := makePNG(t, color.RGBA{R: 255, A: 255})
	blue := makePNG(t, color.RGBA{B: 255, A: 255})
	a.upload(http.StatusOK, "keep", red, "")
	before, err := storage.Global.Get("keep")
	if !err {
		t.Fatal("lost link")
	}
	oldPreview, errRead := os.ReadFile("data/previews/keep.webp")
	if errRead != nil {
		t.Fatal(errRead)
	}
	restore := blockMetadata(t)
	a.upload(http.StatusInternalServerError, "keep", blue, "")
	if got, err := os.ReadFile("data/media/keep.png"); err != nil || !bytes.Equal(got, red) {
		t.Fatalf("image not rolled back: %v", err)
	}
	if got, err := os.ReadFile("data/previews/keep.webp"); err != nil || !bytes.Equal(got, oldPreview) {
		t.Fatalf("preview not rolled back: %v", err)
	}
	if after, ok := storage.Global.Get("keep"); !ok || after.SizeBytes != before.SizeBytes || after.AccessLevel != before.AccessLevel {
		t.Fatalf("metadata changed after failed upload: %+v", after)
	}
	a.expect(http.StatusInternalServerError, "PATCH", "/api/link/keep", []byte(`{"newLinkName":"broken"}`), true,
		map[string]string{"Content-Type": "application/json"})
	if _, err := os.Stat("data/media/broken.png"); !os.IsNotExist(err) {
		t.Fatalf("rename was not rolled back: %v", err)
	}
	a.expect(http.StatusInternalServerError, "DELETE", "/api/link/keep", nil, true, nil)
	a.expect(http.StatusOK, "GET", "/keep", nil, false, nil)
	a.expect(http.StatusOK, "POST", "/api/regenerate-previews", nil, true, nil)
	if got, err := os.ReadFile("data/previews/keep.webp"); err != nil || !bytes.Equal(got, oldPreview) {
		t.Fatalf("regeneration did not roll back preview: %v", err)
	}
	restore()
	for _, pattern := range []string{"data/media/.upload-*", "data/media/.tmp-*", "data/previews/.upload-*", "data/.wallpapers-*.json"} {
		matches, _ := filepath.Glob(pattern)
		if len(matches) != 0 {
			t.Fatalf("staged files leaked on error: %q = %v", pattern, matches)
		}
	}
	if err := storage.Global.Load(); err != nil {
		t.Fatalf("persisted JSON unusable: %v", err)
	}
	a.expect(http.StatusOK, "GET", "/keep", nil, false, nil)
}

func TestAppRejectsTraversalSymlinksAndSizeLimit(t *testing.T) {
	a := setupApp(t)
	a.createLink("safe")
	pngBytes := makePNG(t, color.RGBA{G: 255, A: 255})
	if err := os.WriteFile("external/images/valid.png", pngBytes, 0644); err != nil {
		t.Fatal(err)
	}
	a.upload(http.StatusOK, "safe", nil, "valid.png")
	a.upload(http.StatusBadRequest, "safe", nil, "../data/wallpapers.json")
	a.upload(http.StatusBadRequest, "safe", nil, "file:///etc/passwd")
	a.upload(http.StatusBadRequest, "safe", nil, "http://127.0.0.1/admin")
	if err := os.Symlink("../../../data/wallpapers.json", "external/images/escape.png"); err != nil {
		t.Fatal(err)
	}
	a.upload(http.StatusForbidden, "safe", nil, "escape.png")
	if err := os.WriteFile("data/media/safe.png", []byte("old placeholder"), 0644); err != nil {
		t.Fatal(err)
	}
	// Replace the only copy with a symlink: the reader must not follow it.
	if err := os.Remove("data/media/safe.png"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../wallpapers.json", "data/media/safe.png"); err != nil {
		t.Fatal(err)
	}
	a.expect(http.StatusNotFound, "GET", "/safe", nil, false, nil)
	if err := os.Remove("data/media/safe.png"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("data/media/safe.png", pngBytes, 0644); err != nil {
		t.Fatal(err)
	}
	config.Current.MaxUploadMB = 1
	large := make([]byte, (1<<20)+1)
	copy(large, pngBytes)
	a.upload(http.StatusRequestEntityTooLarge, "safe", large, "")
	if got := a.expect(http.StatusOK, "GET", "/safe", nil, false, nil); !bytes.Equal(got, pngBytes) {
		t.Fatal("invalid upload changed existing image")
	}
	if _, err := os.Stat("data/previews/safe.webp"); err != nil {
		t.Fatal("existing thumbnail removed: ", err)
	}
	if matches, _ := filepath.Glob("data/media/.download-*"); len(matches) != 0 {
		t.Fatalf("temporary download leaked: %v", matches)
	}
}

func TestStartupRefusesCorruptMetadata(t *testing.T) {
	if os.Getenv("LANPAPER_TEST_STARTUP") == "1" {
		main() // should exit non-zero before listening
		return
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data", "wallpapers.json"), []byte(`{broken`), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStartupRefusesCorruptMetadata$")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "LANPAPER_TEST_STARTUP=1")
	result, err := cmd.CombinedOutput()
	if err == nil || !bytes.Contains(result, []byte("Cannot load wallpapers")) {
		t.Fatalf("server started over damaged metadata or wrong failure: %v %s", err, result)
	}
	if got, err := os.ReadFile(filepath.Join(root, "data", "wallpapers.json")); err != nil || string(got) != `{broken` {
		t.Fatalf("startup overwrote damaged metadata: %v %s", err, got)
	}
}

func TestStaticAllowlistCoversApplicationAssets(t *testing.T) {
	if err := filepath.WalkDir("static", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := filepath.ToSlash(strings.TrimPrefix(path, "static/"))
		if strings.HasPrefix(name, "images/") || strings.HasPrefix(name, ".") {
			return nil // legacy mount and source control placeholders must be private
		}
		if !staticAssets[name] {
			t.Errorf("application asset is not in the allowlist: %s", name)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for name := range staticAssets {
		if _, err := os.Stat(filepath.Join("static", name)); err != nil {
			t.Errorf("allowlisted asset missing: %s: %v", name, err)
		}
	}
}

func TestAppExternalGalleryListing(t *testing.T) {
	a := setupApp(t)
	pngBytes := makePNG(t, color.RGBA{B: 255, A: 255})
	files := map[string][]byte{
		"a.png":                      pngBytes,
		".hidden.png":                pngBytes,
		".hiddendir/b.png":           pngBytes,
		"d1/d2/d3/deep.png":          pngBytes, // depth 3: allowed with MaxWalkDepth 3
		"d1/d2/d3/d4/too-deep.png":   pngBytes,
		"notes.txt":                  []byte("not media"),
		"clips/holiday.mp4":          []byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00isomiso2"),
		"clips/.partial/unused.webm": pngBytes,
	}
	for name, data := range files {
		path := filepath.Join("external/images", name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	big, err := os.Create("external/images/big.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := big.Truncate(int64(config.Current.MaxUploadMB)<<20 + 1); err != nil {
		t.Fatal(err)
	}
	big.Close()
	for link, target := range map[string]string{
		"external/images/escape.png":      "../../data/wallpapers.json", // leaves the gallery
		"external/images/inside-link.png": "a.png",                      // stays inside
		"external/images/dirlink":         "d1",                         // directory symlink: not descended
	} {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	var listed []string
	if err := json.Unmarshal(a.expect(http.StatusOK, "GET", "/api/external-images", nil, true, nil), &listed); err != nil {
		t.Fatal(err)
	}
	want := []string{"a.png", "clips/holiday.mp4", "d1/d2/d3/deep.png", "inside-link.png"}
	if strings.Join(listed, ",") != strings.Join(want, ",") {
		t.Fatalf("gallery listing = %q, want %q", listed, want)
	}
	// A missing gallery directory yields an empty list, not an error.
	config.Current.ExternalImageDir = "does-not-exist"
	if got := strings.TrimSpace(string(a.expect(http.StatusOK, "GET", "/api/external-images", nil, true, nil))); got != "[]" {
		t.Fatalf("missing gallery = %s, want []", got)
	}
}
