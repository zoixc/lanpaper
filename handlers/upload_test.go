package handlers

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chai2010/webp"

	"lanpaper/config"
	"lanpaper/storage"
)

func TestDecodedImageBudgetIsShared(t *testing.T) {
	release, err := reserveDecodedPixels(config.MaxImagePixels)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	if _, err := reserveDecodedPixels(config.MaxImagePixels); !errors.Is(err, errImageBudgetBusy) {
		t.Fatalf("concurrent large image unexpectedly accepted: %v", err)
	}
	if _, err := reserveDecodedPixels(0); !errors.Is(err, errImageBudgetBusy) {
		t.Fatalf("invalid reservation accepted: %v", err)
	}
	release()
	release = nil
	second, err := reserveDecodedPixels(config.MaxImagePixels)
	if err != nil {
		t.Fatalf("memory budget not released: %v", err)
	}
	second()
}

func TestWebPDecodesWithoutCDecoderAndRejectsOversizedDimensions(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	img.SetRGBA(0, 0, color.RGBA{R: 120, G: 60, A: 128})
	for _, tc := range []struct {
		name    string
		options *webp.Options
	}{
		{"lossy with alpha", &webp.Options{Quality: 85}},
		{"lossless", &webp.Options{Lossless: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := webp.Encode(&buf, img, tc.options); err != nil {
				t.Fatal(err)
			}
			file := bytes.NewReader(buf.Bytes())
			ext, err := inspectMediaFile(file, "no-extension", int64(file.Len()), 1<<20)
			if err != nil || ext != "webp" {
				t.Fatalf("valid WebP not detected: %s %v", ext, err)
			}
			decoded, release, err := decodeImage(file, ext)
			if err != nil {
				t.Fatalf("streaming WebP decoder failed: %v", err)
			}
			defer release()
			if decoded.Bounds().Dx() != 16 || decoded.Bounds().Dy() != 16 {
				t.Fatalf("incorrect WebP dimensions: %s", decoded.Bounds())
			}
		})
	}

	pngBytes := makeSmallPNG(t)
	// IHDR width is at [16:20]; recalculate its CRC to make an
	// otherwise-valid PNG that declares an excessive pixel dimension.
	binary.BigEndian.PutUint32(pngBytes[16:20], config.MaxImageDimension+1)
	binary.BigEndian.PutUint32(pngBytes[29:33], crc32.ChecksumIEEE(pngBytes[12:29]))
	if _, release, err := decodeImage(bytes.NewReader(pngBytes), "png"); err == nil {
		if release != nil {
			release()
		}
		t.Fatal("oversized image was decoded")
	}
}

func makeSmallPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func remoteUploadRequest(t *testing.T, name string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("linkName", name); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("url", "http://public.test/image.png"); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestUploadSemaphoreBoundsConcurrentRemoteDownloads(t *testing.T) {
	setupRemoteTest(t)
	if err := os.MkdirAll(config.PreviewDir, 0755); err != nil {
		t.Fatal(err)
	}
	previous := storage.Global
	storage.Global = &storage.Store{}
	t.Cleanup(func() { storage.Global = previous })
	oldSem := uploadSem
	InitUploadSemaphore(1)
	t.Cleanup(func() { uploadSem = oldSem })
	for _, name := range []string{"first", "second"} {
		if err := storage.Global.Create(&storage.Wallpaper{ID: name, LinkName: name, AccessLevel: config.AccessPublic}); err != nil {
			t.Fatal(err)
		}
	}
	started, finish := make(chan struct{}), make(chan struct{})
	pngBytes := makeSmallPNG(t)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-finish
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer proxy.Close()
	configureProxy(t, proxy)

	firstReq := remoteUploadRequest(t, "first")
	secondReq := remoteUploadRequest(t, "second")
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		Upload(w, firstReq)
		result <- w
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		close(finish)
		t.Fatal("first upload never reached remote proxy")
	}
	w := httptest.NewRecorder()
	Upload(w, secondReq)
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "concurrent") {
		close(finish)
		t.Fatalf("upload limit ignored: %d %q", w.Code, w.Body.String())
	}
	close(finish)
	select {
	case first := <-result:
		if first.Code != http.StatusOK {
			t.Fatalf("first upload failed: %d %s", first.Code, first.Body.String())
		}
		var wp WallpaperResponse
		if err := json.Unmarshal(first.Body.Bytes(), &wp); err != nil || !wp.HasImage {
			t.Fatalf("first upload incomplete: %+v %v", wp, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first upload never completed")
	}
	if wp, ok := storage.Global.Get("second"); !ok || wp.HasImage {
		t.Fatalf("rate-limited upload changed second link: %+v", wp)
	}
}

// mp4WithBrand builds a minimal ftyp box with the given major brand, as real
// cameras/phones produce (often WITHOUT any "mp4*" brand).
func mp4WithBrand(major string, compat ...string) []byte {
	brands := major + "\x00\x00\x00\x00" + strings.Join(compat, "")
	box := make([]byte, 0, 8+len(brands)+64)
	size := len(brands) + 8
	box = append(box, byte(size>>24), byte(size>>16), byte(size>>8), byte(size))
	box = append(box, []byte("ftyp")...)
	box = append(box, []byte(brands)...)
	return append(box, make([]byte, 64)...)
}

// TestInspectMediaFileAcceptsAllMP4Brands guards against the WHATWG sniffer
// gap in net/http: it only matches ftyp boxes containing an "mp4*" brand, so
// isom/iso2/avc1/M4V videos (typical camera/phone output) were rejected —
// especially URL downloads, whose temp files carry no extension fallback.
func TestInspectMediaFileAcceptsAllMP4Brands(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"isom only (camera files)", mp4WithBrand("isom", "iso2", "avc1"), "mp4"},
		{"mp42 brand", mp4WithBrand("mp42", "isom", "iso2", "mp41"), "mp4"},
		{"M4V brand", mp4WithBrand("M4V ", "M4V "), "mp4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Nameless temp file: no extension to fall back to.
			ext, err := inspectMediaFile(bytes.NewReader(tt.data), ".download-123", int64(len(tt.data)), 1<<20)
			if err != nil || ext != tt.want {
				t.Errorf("inspectMediaFile() = (%q, %v), want (%q, nil)", ext, err, tt.want)
			}
		})
	}

	// Garbage must still be rejected, even named like a video.
	garbage := []byte("<html>definitely not a video file at all, padding padding</html>")
	if _, err := inspectMediaFile(bytes.NewReader(garbage), "evil.mp4", int64(len(garbage)), 1<<20); err == nil {
		t.Error("garbage with .mp4 name must be rejected")
	}
}
