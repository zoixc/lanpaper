// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lanpaper/config"
	"lanpaper/storage"
)

// remoteUploadLink creates the link an upload replaces and returns an upload
// service whose fetcher answers from memory. No DNS and no socket: the SSRF
// rules are exercised separately, with the real transport, in the tests below.
func remoteUploadLink(t *testing.T, respond roundTripFunc) *UploadService {
	t.Helper()
	setupRemoteTest(t)
	originalStore := storage.Global
	storage.Global = loadedStore(t)
	t.Cleanup(func() { storage.Global = originalStore })
	if err := storage.Global.Create(&storage.Wallpaper{
		ID: "photo", LinkName: "photo", AccessLevel: config.AccessPublic}); err != nil {
		t.Fatal(err)
	}
	service := NewUploadService(storage.Global)
	if respond != nil {
		service.Fetcher = &RemoteFetcher{TempDir: config.MediaDir, Transport: respond}
	}
	return service
}

func mediaResponse(status int, body []byte) roundTripFunc {
	return func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    status,
			Body:          io.NopCloser(bytes.NewReader(body)),
			ContentLength: int64(len(body)),
			Header:        http.Header{"Content-Type": []string{"image/png"}},
		}, nil
	}
}

// A CDN address carries no file extension at all — images.unsplash.com serves
// the file under its photo id — and the type is decided by the content. The
// escaped separators a link keeps when it is copied out of a page source or a
// markdown file are rewritten before the request: imgix ignores parameters it
// does not recognize, so "?q=80&amp;w=1336" would fetch the full-size original
// instead of the picture the user picked.
func TestRemoteUploadAcceptsExtensionlessAddressAndRealAmpersands(t *testing.T) {
	const source = "https://images.unsplash.com/photo-1791391279415-fefa6e26f0e2?q=80&amp;w=1336&amp;auto=format"
	const wantURL = "https://images.unsplash.com/photo-1791391279415-fefa6e26f0e2?q=80&w=1336&auto=format"
	pngBytes := makeSmallPNG(t)
	requested := ""
	service := remoteUploadLink(t, func(req *http.Request) (*http.Response, error) {
		requested = req.URL.String()
		return mediaResponse(http.StatusOK, pngBytes)(req)
	})

	w := httptest.NewRecorder()
	service.Upload(w, avifRequest(t, "photo", "", source, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("extension-less URL refused: %d %s", w.Code, w.Body.String())
	}
	if requested != wantURL {
		t.Fatalf("fetched %q, want %q", requested, wantURL)
	}
	wp, ok := storage.Global.Get("photo")
	if !ok || !wp.HasImage || wp.MIMEType != "png" {
		t.Fatalf("stored metadata: %+v", wp)
	}
}

// "The server could not reach the address" and "the address answered, but not
// with media" are different problems: the first is DNS, a timeout, TLS or a
// host blocked for the server's own country — the second is a wrong link. The
// panel shows the server's words, so the two must not share an answer.
func TestRemoteUploadSeparatesUnreachableAddressFromNonMedia(t *testing.T) {
	timeout := errors.New("dial tcp 8.8.8.8:443: i/o timeout")
	service := remoteUploadLink(t, func(*http.Request) (*http.Response, error) { return nil, timeout })

	w := httptest.NewRecorder()
	service.Upload(w, avifRequest(t, "photo", "", "https://images.unsplash.com/photo-1?w=100", nil))
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "Could not reach the address") {
		t.Fatalf("unreachable address answered %d %q", w.Code, w.Body.String())
	}

	service = remoteUploadLink(t, mediaResponse(http.StatusNotFound, []byte("no such object")))
	w = httptest.NewRecorder()
	service.Upload(w, avifRequest(t, "photo", "", "https://images.unsplash.com/photo-2?w=100", nil))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Failed to load media") {
		t.Fatalf("missing media answered %d %q", w.Code, w.Body.String())
	}
}

// The refusals the SSRF rules make are not connectivity failures either: the
// address was never dialled. They get their own answer, which the panel turns
// into "the server does not download addresses like this", instead of the
// "no image at this address" a media problem gets.
func TestRemoteUploadRefusalsAreNotReportedAsUnreachable(t *testing.T) {
	for _, tc := range []struct {
		name, source string
	}{
		{"private address", "http://private.test/a.png"},
		{"metadata host", "http://metadata.google.internal/a.png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The real transport and the fake resolver: nothing is dialled for
			// either address, so the test stays offline.
			service := remoteUploadLink(t, nil)
			w := httptest.NewRecorder()
			service.Upload(w, avifRequest(t, "photo", "", tc.source, nil))
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Address is not allowed") {
				t.Fatalf("refused address answered %d %q", w.Code, w.Body.String())
			}
		})
	}
}

// A host that does not resolve is the everyday form of "the server cannot
// reach it" (a filtered DNS, a CDN that is not answering from this country)
// and must not be dressed up as a missing picture.
func TestRemoteUploadReportsDNSFailureAsUnreachable(t *testing.T) {
	service := remoteUploadLink(t, nil)
	w := httptest.NewRecorder()
	service.Upload(w, avifRequest(t, "photo", "", "http://no-such-host.test/a.png", nil))
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "Could not reach the address") {
		t.Fatalf("unresolvable host answered %d %q", w.Code, w.Body.String())
	}
}
