// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/webp"
	"lanpaper/config"
	"lanpaper/storage"
	"lanpaper/utils"
)

func avifFixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "internal", "avifcodec", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func avifRequest(t *testing.T, name, mode, source string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for k, v := range map[string]string{"linkName": name, "mode": mode} {
		if err := form.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if source != "" {
		if err := form.WriteField("url", source); err != nil {
			t.Fatal(err)
		}
	} else {
		part, err := form.CreateFormFile("file", "photo.avif")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/upload", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	return req
}

func TestAVIFUploadFileURLGalleryAndStorageModes(t *testing.T) {
	data := avifFixture(t, "alpha.avif")
	for _, tc := range []struct {
		name, source   string
		quality, scale int
		ext            string
	}{
		{"original", "", 100, 100, "avif"},
		{"webp", "", 85, 100, "webp"},
		{"scaled", "", 85, 50, "webp"},
		{"url", "https://images.unsplash.com/photo-example?auto=format&w=32", 85, 100, "webp"},
		{"gallery", "photo.avif", 100, 100, "avif"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupRemoteTest(t)
			originalStore := storage.Global
			storage.Global = loadedStore(t)
			t.Cleanup(func() { storage.Global = originalStore })
			config.Current.Compression = config.CompressionConfig{Quality: tc.quality, Scale: tc.scale}
			if err := storage.Global.Create(&storage.Wallpaper{ID: "photo", LinkName: "photo", AccessLevel: config.AccessPublic}); err != nil {
				t.Fatal(err)
			}
			service := NewUploadService(storage.Global)
			fetched := false
			service.Fetcher = &RemoteFetcher{TempDir: config.MediaDir, Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				fetched = true
				if req.URL.String() != tc.source {
					t.Fatal("URL parameters changed")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Header: http.Header{"Content-Type": []string{"application/octet-stream"}}}, nil
			})}
			if tc.source == "photo.avif" {
				config.Current.ExternalImageDir = t.TempDir()
				if err := os.WriteFile(filepath.Join(config.Current.ExternalImageDir, tc.source), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			w := httptest.NewRecorder()
			service.Upload(w, avifRequest(t, "photo", "", tc.source, data))
			if w.Code != http.StatusOK {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if strings.HasPrefix(tc.source, "https:") && !fetched {
				t.Fatal("remote fetch skipped")
			}
			wp, ok := storage.Global.Get("photo")
			if !ok || !wp.HasImage || wp.MIMEType != tc.ext {
				t.Fatalf("stored metadata: %+v", wp)
			}
			stored, err := os.ReadFile(wp.ImagePath)
			if err != nil {
				t.Fatal(err)
			}
			if tc.ext == "avif" {
				if !bytes.Equal(stored, data) {
					t.Fatal("original AVIF bytes changed")
				}
				if mediaContentType(wp.MIMEType) != "image/avif" {
					t.Fatal("wrong AVIF response MIME")
				}
			} else {
				img, err := webp.Decode(bytes.NewReader(stored))
				if err != nil {
					t.Fatal(err)
				}
				if img.Bounds().Dx() != 32*tc.scale/100 || img.Bounds().Dy() != 24*tc.scale/100 {
					t.Fatal(img.Bounds())
				}
				_, _, _, alpha := img.At(0, 0).RGBA()
				if alpha == 0xffff || alpha == 0 {
					t.Fatal("transparency lost")
				}
			}
			// The original/converted media is served by the existing public route,
			// including extension aliases; the MIME follows storage, not the URL.
			served := httptest.NewRecorder()
			Public(served, httptest.NewRequest(http.MethodGet, "/photo.avif", nil))
			if served.Code != 200 || served.Header().Get("Content-Type") != mediaContentType(tc.ext) || !bytes.Equal(served.Body.Bytes(), stored) {
				t.Fatalf("public media: %d %s", served.Code, served.Header().Get("Content-Type"))
			}
			preview, err := os.ReadFile(wp.PreviewPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := webp.Decode(bytes.NewReader(preview)); err != nil {
				t.Fatal(err)
			}
			// Regeneration must also work when the original remains AVIF.
			if err := regenPreview(context.Background(), wp); err != nil {
				t.Fatal(err)
			}
			// Failure must never replace the existing media/metadata.
			response := httptest.NewRecorder()
			service.Upload(response, avifRequest(t, "photo", "", "", data[:100]))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("corruption accepted: %d", response.Code)
			}
			after, _ := os.ReadFile(wp.ImagePath)
			if !bytes.Equal(stored, after) {
				t.Fatal("failed upload replaced existing file")
			}
			if release, err := reserveDecodedBytes(maxDecodedBytesInFlight); err != nil {
				t.Fatal("leaked memory reservation", err)
			} else {
				release()
			}
		})
	}
}

func TestAVIFSharesBudgetsAndCleansUpOnFailure(t *testing.T) {
	data := avifFixture(t, "alpha.avif")
	first, err := reserveDecodedBytes(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := decodeImage(t.Context(), bytes.NewReader(data), "avif"); !errors.Is(err, errImageBudgetBusy) {
		t.Fatalf("AVIF bypassed shared budget: %v", err)
	}
	first()
	img, release, err := decodeImage(t.Context(), bytes.NewReader(data), "avif")
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Empty() {
		t.Fatal("no image")
	}
	if _, err := reserveDecodedPixels(1, 4); !errors.Is(err, errImageBudgetBusy) {
		t.Fatal("normal decode bypassed AVIF reservation")
	}
	release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := decodeImage(ctx, bytes.NewReader(data), "avif"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	corrupt := bytes.Clone(data)
	idx := bytes.Index(corrupt, []byte("mdat"))
	for i := idx + 4; i < len(corrupt); i++ {
		corrupt[i] = 0
	}
	if _, _, err := decodeImage(t.Context(), bytes.NewReader(corrupt), "avif"); err == nil {
		t.Fatal("invalid stream accepted")
	}
	if done, err := reserveDecodedPixels(config.MaxImagePixels, config.MaxImagePixels*8); err != nil {
		t.Fatal("leaked reservation", err)
	} else {
		done()
	}
}

func TestAVIFSignatureAndVideoSeparation(t *testing.T) {
	for _, data := range [][]byte{mp4WithBrand("avif", "mif1"), mp4WithBrand("mif1", "avif"), mp4WithBrand("avis", "avif")} {
		ext, err := inspectMediaFile(bytes.NewReader(data), "no-extension", int64(len(data)), 1<<20)
		if err != nil || ext != "avif" {
			t.Fatalf("AVIF not detected: %s %v", ext, err)
		}
		if err := utils.ValidateFileType(data, "avif"); err != nil {
			t.Fatal(err)
		}
	}
	data := mp4WithBrand("isom", "heic")
	if ext, err := inspectMediaFile(bytes.NewReader(data), "a.mp4", int64(len(data)), 1<<20); err == nil {
		t.Fatalf("HEIC accepted as %s", ext)
	}
	for _, size := range []uint32{8, 15, 17, 512, 0xffffffff} {
		data := mp4WithBrand("avif")
		binary.BigEndian.PutUint32(data, size)
		if utils.IsAVIF(data) {
			t.Fatalf("malformed ftyp size %d accepted", size)
		}
	}
	if storedExt("avif", false) != "webp" || storedExt("avif", true) != "avif" || !storeUploadedBytes("avif", true) || storeUploadedBytes("avif", false) {
		t.Fatal("wrong storage policy")
	}
}

func TestAVIFAppendOriginalAndRejectAnimation(t *testing.T) {
	data := avifFixture(t, "alpha.avif")
	animation := avifFixture(t, "test.avifs")
	setupRemoteTest(t)
	config.Current.PlaylistMax = 10
	originalStore := storage.Global
	storage.Global = loadedStore(t)
	t.Cleanup(func() { storage.Global = originalStore })
	if err := storage.Global.Create(&storage.Wallpaper{ID: "photo", LinkName: "photo", AccessLevel: config.AccessPublic}); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"", "append"} {
		w := httptest.NewRecorder()
		Upload(w, avifRequest(t, "photo", mode, "", data))
		if w.Code != 200 {
			t.Fatalf("%s %d %s", mode, w.Code, w.Body.String())
		}
	}
	before, _ := storage.Global.Get("photo")
	w := httptest.NewRecorder()
	Upload(w, avifRequest(t, "photo", "", "", animation))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "Unsupported AVIF feature") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	after, _ := storage.Global.Get("photo")
	if before.ImagePath != after.ImagePath || len(before.Items) != len(after.Items) {
		t.Fatal("failed upload mutated playlist")
	}
}

func TestAVIFHistoryRollback(t *testing.T) {
	data := avifFixture(t, "alpha.avif")
	setupRemoteTest(t)
	originalStore := storage.Global
	storage.Global = loadedStore(t)
	t.Cleanup(func() { storage.Global = originalStore })
	config.Current.History = config.HistoryConfig{Limit: 3}
	if err := storage.Global.Create(&storage.Wallpaper{ID: "photo", LinkName: "photo", AccessLevel: config.AccessPublic}); err != nil {
		t.Fatal(err)
	}
	for _, quality := range []int{100, 85} {
		config.Current.Compression.Quality = quality
		w := httptest.NewRecorder()
		Upload(w, avifRequest(t, "photo", "", "", data))
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	wp, _ := storage.Global.Get("photo")
	if len(wp.History) != 1 || wp.History[0].Ext != "avif" {
		t.Fatalf("AVIF history lost: %+v", wp.History)
	}
	restored, err := NewLibraryService(storage.Global).rollbackToVersion(wp, wp.History[0])
	if err != nil {
		t.Fatal(err)
	}
	if restored.MIMEType != "avif" {
		t.Fatalf("restored %s", restored.MIMEType)
	}
	got, err := os.ReadFile(restored.ImagePath)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("original bytes not restored", err)
	}
	if err := regenPreview(context.Background(), restored); err != nil {
		t.Fatal(err)
	}
}

func TestAVIFHistoryUsesInjectedStore(t *testing.T) {
	data := avifFixture(t, "alpha.avif")
	setupRemoteTest(t)
	store := loadedStore(t)
	previous := storage.Global
	storage.Global = &storage.Store{}
	t.Cleanup(func() { storage.Global = previous })
	config.Current.History = config.HistoryConfig{Limit: 3}
	if err := store.Create(&storage.Wallpaper{ID: "photo", LinkName: "photo", AccessLevel: config.AccessPublic}); err != nil {
		t.Fatal(err)
	}
	service := NewUploadService(store)
	for _, quality := range []int{100, 85} {
		config.Current.Compression.Quality = quality
		w := httptest.NewRecorder()
		service.Upload(w, avifRequest(t, "photo", "", "", data))
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	wp, _ := store.Get("photo")
	if len(wp.History) != 1 || wp.History[0].Ext != "avif" {
		t.Fatalf("history written to wrong store: %+v", wp.History)
	}
	if _, ok := storage.Global.Get("photo"); ok {
		t.Fatal("upload leaked into global store")
	}
}
