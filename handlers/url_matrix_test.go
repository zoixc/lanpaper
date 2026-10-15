// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"lanpaper/config"
	"lanpaper/storage"
)

// The URL upload has a long list of gates: scheme and length, SSRF rules, DNS,
// redirect policy, status code, size, the content sniffer and the decoder. This
// matrix pins down what a user sees at each of them, so a change to one gate
// cannot silently turn a clear refusal into "no image at this address" or the
// other way round. It is a behaviour inventory: every case is one request
// through Upload.
func TestURLUploadLimitMatrix(t *testing.T) {
	pngBytes := makeSmallPNG(t)
	mp4Bytes := mp4WithBrand("isom", "iso2", "avc1")
	heic := append(append([]byte{0, 0, 0, 24}, []byte("ftyp")...), []byte("heicheicmif1")...)
	heic = append(heic, make([]byte, 64)...)
	// A picture whose header alone is out of bounds: the limits are checked
	// from the header, before any pixel is decoded.
	patchPNG := func(width, height uint32) []byte {
		out := append([]byte{}, pngBytes...)
		binary.BigEndian.PutUint32(out[16:20], width)
		binary.BigEndian.PutUint32(out[20:24], height)
		binary.BigEndian.PutUint32(out[29:33], crc32.ChecksumIEEE(out[12:29]))
		return out
	}
	tooManyPixels := patchPNG(8000, 5000) // 40 MP, over the 36 MP ceiling
	tooWide := patchPNG(config.MaxImageDimension+1, 8)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64"/></svg>`)
	html := []byte(`<!doctype html><html><body><h1>Photo</h1><img src="/a.jpg"></body></html>`)

	type route struct {
		status  int
		body    []byte
		length  int64 // -1: len(body), -2: unknown (chunked)
		block   time.Duration
		redirTo string
	}
	routes := map[string]route{
		"ok.test":         {status: 200, body: pngBytes, length: -1},
		"no-content.test": {status: 200, body: pngBytes, length: -1},
		"slow-header.test": {
			// A server that answers late, but inside the client's
			// ResponseHeaderTimeout: the upload still succeeds.
			status: 200, body: pngBytes, length: -1, block: 2 * time.Second},
		"video.test":      {status: 200, body: mp4Bytes, length: -1},
		"missing.test":    {status: 404, body: html, length: -1},
		"hotlink.test":    {status: 403, body: []byte("hotlinking is not allowed"), length: -1},
		"broken.test":     {status: 500, body: []byte("oops"), length: -1},
		"page.test":       {status: 200, body: html, length: -1},
		"svg.test":        {status: 200, body: svg, length: -1},
		"heic.test":       {status: 200, body: heic, length: -1},
		"empty.test":      {status: 200, body: nil, length: -1},
		"truncated.test":  {status: 200, body: append(append([]byte{}, pngBytes[:40]...), bytes.Repeat([]byte("junk"), 40)...), length: -1},
		"megapixels.test": {status: 200, body: tooManyPixels, length: -1},
		"toowide.test":    {status: 200, body: tooWide, length: -1},
		"lie-size.test":   {status: 200, body: bytes.Repeat([]byte("A"), 3<<20), length: -1},
		"chunked.test":    {status: 200, body: bytes.Repeat([]byte("A"), 3<<20), length: -2},
		"hop1.test":       {status: 302, redirTo: "http://hop2.test/a.png"},
		"hop2.test":       {status: 302, redirTo: "http://hop3.test/a.png"},
		"hop3.test":       {status: 302, redirTo: "http://ok.test/a.png"},
		"loop.test":       {status: 302, redirTo: "http://loop.test/a.png"},
		"page-redir.test": {status: 302, redirTo: "http://page.test/a.jpg"},
		// An AVIF served under a name without an extension: the ISO-BMFF
		// brand decides, exactly as it does for a local file.
		"avif.test": {status: 200, body: avifFixture(t, "alpha.avif"), length: -1},
	}
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		r, ok := routes[req.URL.Hostname()]
		if !ok {
			return nil, fmt.Errorf("no route for %s", req.URL.Hostname())
		}
		if r.block > 0 {
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(r.block):
			}
		}
		if r.redirTo != "" {
			return &http.Response{StatusCode: r.status, Body: io.NopCloser(strings.NewReader("")),
				Header: http.Header{"Location": []string{r.redirTo}}}, nil
		}
		header := http.Header{}
		resp := &http.Response{StatusCode: r.status, Body: io.NopCloser(bytes.NewReader(r.body)), Header: header}
		resp.ContentLength = r.length
		if r.length == -1 {
			resp.ContentLength = int64(len(r.body))
		}
		return resp, nil
	})

	// The deadline on the fetch itself is what stops a slow CDN from holding an
	// upload slot for ever.
	var fetchDeadline time.Duration
	deadlineTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if dl, ok := req.Context().Deadline(); ok {
			fetchDeadline = time.Until(dl)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(pngBytes)),
			ContentLength: int64(len(pngBytes)), Header: http.Header{}}, nil
	})
	service := remoteUploadLink(t, deadlineTransport)
	if _, _, err := service.Fetcher.Fetch(t.Context(), "http://ok.test/a.png", 1<<20); err != nil {
		t.Fatal(err)
	}
	if fetchDeadline < time.Duration(config.DownloadTimeout-5)*time.Second ||
		fetchDeadline > time.Duration(config.DownloadTimeout)*time.Second {
		t.Errorf("fetch deadline is %v, want about %ds from constants.DownloadTimeout", fetchDeadline, config.DownloadTimeout)
	}
	t.Logf("DOWNLOAD_TIMEOUT: a download gets %v before it is cut off; MAX_UPLOAD_MB is %d by default, so a 50 MB file needs about 570 KiB/s",
		time.Duration(config.DownloadTimeout)*time.Second, config.DefaultMaxUploadMB)

	// The download runs inside the request, so the deadlines on that request
	// must outlast the download budget: a shorter one would cut a slow CDN off
	// with a closed connection instead of the 502 that names the cause. The
	// upload handler raises both deadlines to UploadBaseTimeout plus the time
	// the maximum request needs at UploadMinBytesPerSec; the server's own
	// HTTPWriteTimeout is what a response that never reaches the handler is
	// bound by (see the constant's comment in config/constants.go).
	if config.UploadBaseTimeout <= config.DownloadTimeout {
		t.Errorf("UploadBaseTimeout (%ds) must exceed DownloadTimeout (%ds)", config.UploadBaseTimeout, config.DownloadTimeout)
	}
	if config.HTTPWriteTimeout <= config.DownloadTimeout {
		t.Errorf("HTTPWriteTimeout (%ds) must exceed DownloadTimeout (%ds)", config.HTTPWriteTimeout, config.DownloadTimeout)
	}

	cases := []struct {
		name, source string
		wantStatus   int
		wantBody     string
		// realTransport: the case is decided by the SSRF resolver or DNS, so
		// the injected transport must not stand in for it.
		realTransport bool
	}{
		{name: "extension-less, content decides", source: "http://ok.test/photo-1?q=80&w=1336", wantStatus: 200},
		{name: "chunked answer without Content-Length", source: "http://no-content.test/a", wantStatus: 200},
		{name: "slow answer inside the header timeout", source: "http://slow-header.test/a.png", wantStatus: 200},
		{name: "mp4 video by URL", source: "http://video.test/clip.mp4", wantStatus: 200},
		{name: "extension-less AVIF, brand decides", source: "http://avif.test/photo", wantStatus: 200},
		{name: "404 at the CDN (wrong id)", source: "http://missing.test/a.png", wantStatus: 400, wantBody: "Failed to load media"},
		{name: "403 hotlink protection", source: "http://hotlink.test/a.png", wantStatus: 400, wantBody: "Failed to load media"},
		{name: "500 at the CDN", source: "http://broken.test/a.png", wantStatus: 400, wantBody: "Failed to load media"},
		{name: "a web page instead of a file", source: "http://page.test/a.jpg", wantStatus: 400, wantBody: "Invalid or unsupported media file"},
		{name: "SVG is not accepted", source: "http://svg.test/logo.svg", wantStatus: 400, wantBody: "Invalid or unsupported media file"},
		{name: "HEIC is not accepted", source: "http://heic.test/photo.heic", wantStatus: 400, wantBody: "Invalid or unsupported media file"},
		{name: "empty body", source: "http://empty.test/a.png", wantStatus: 400, wantBody: "Invalid or unsupported media file"},
		{name: "truncated image", source: "http://truncated.test/a.png", wantStatus: 400, wantBody: "Invalid image"},
		{name: "40 MP picture (36 MP ceiling)", source: "http://megapixels.test/a.png", wantStatus: 400, wantBody: "Image dimensions too large"},
		{name: "side over 16384 px", source: "http://toowide.test/a.png", wantStatus: 400, wantBody: "Image dimensions too large"},
		{name: "Content-Length over MAX_UPLOAD_MB", source: "http://lie-size.test/a.png", wantStatus: 413, wantBody: "File too large"},
		{name: "streamed past MAX_UPLOAD_MB", source: "http://chunked.test/a.png", wantStatus: 413, wantBody: "File too large"},
		{name: "three redirects are followed", source: "http://hop1.test/a.png", wantStatus: 200},
		{name: "more than five redirects", source: "http://loop.test/a.png", wantStatus: 400, wantBody: "Failed to load media"},
		{name: "redirect to a page", source: "http://page-redir.test/a.png", wantStatus: 400, wantBody: "Invalid or unsupported media file"},
		{name: "too long for the API", source: "http://ok.test/" + strings.Repeat("a", 2100), wantStatus: 400, wantBody: "URL too long"},
		{name: "invalid escape in the address", source: "http://ok.test/%zz", wantStatus: 400, wantBody: "Invalid URL"},
		{name: "not HTTP(S): data URL", source: "data:image/png;base64,iVBORw0KGgo=", wantStatus: 400, wantBody: "Invalid local media path"},
		{name: "not HTTP(S): ftp", source: "ftp://ok.test/a.png", wantStatus: 400, wantBody: "Invalid local media path"},
		{name: "credentials in the address", source: "http://user:secret@ok.test/a.png", realTransport: true, wantStatus: 400, wantBody: "Address is not allowed"},
		{name: "an address in a private range", source: "http://192.168.1.5/cam.jpg", realTransport: true, wantStatus: 400, wantBody: "Address is not allowed"},
		{name: "a metadata hostname", source: "http://metadata.google.internal/latest", realTransport: true, wantStatus: 400, wantBody: "Address is not allowed"},
		{name: "a name that does not resolve", source: "http://no-such-host.test/a.png", realTransport: true, wantStatus: 502, wantBody: "Could not reach the address"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var service *UploadService
			if tc.realTransport {
				service = remoteUploadLink(t, nil)
			} else {
				service = remoteUploadLink(t, transport)
			}
			w := httptest.NewRecorder()
			service.Upload(w, avifRequest(t, "photo", "", tc.source, nil))
			if w.Code != tc.wantStatus {
				t.Errorf("%s: status %d, want %d (%s)", tc.source, w.Code, tc.wantStatus, strings.TrimSpace(w.Body.String()))
			}
			if tc.wantBody != "" && !strings.Contains(w.Body.String(), tc.wantBody) {
				t.Errorf("%s: body %q, want it to mention %q", tc.source, strings.TrimSpace(w.Body.String()), tc.wantBody)
			}
		})
	}
}

// The address is fetched exactly as it was pasted apart from the HTML escape:
// the query is what makes a CDN return the resized file rather than the
// original, and the port is part of the address.
func TestURLUploadKeepsQueryAndPort(t *testing.T) {
	source := "http://ok.test:8080/photo?q=80&w=1336&auto=format&fit=crop&ixid=ABC%3D"
	body := makeSmallPNG(t)
	seen := ""
	service := remoteUploadLink(t, func(req *http.Request) (*http.Response, error) {
		seen = req.URL.String()
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)),
			ContentLength: int64(len(body)), Header: http.Header{}}, nil
	})
	w := httptest.NewRecorder()
	service.Upload(w, avifRequest(t, "photo", "", source, nil))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if seen != source {
		t.Errorf("fetched %q, want %q", seen, source)
	}
}

// A non-ASCII host is pasted as it is on the site it came from. The panel
// cannot know whether it resolves, so the server decides — and must report an
// address it cannot reach rather than an address without media.
func TestURLUploadUnicodeHostIsUnreachableNotNonMedia(t *testing.T) {
	for _, source := range []string{
		"https://пример.рф/картинка.jpg",
		"https://xn--e1afmkfd.xn--p1ai/photo.jpg",
	} {
		t.Run(source, func(t *testing.T) {
			service := remoteUploadLink(t, nil)
			w := httptest.NewRecorder()
			service.Upload(w, avifRequest(t, "photo", "", source, nil))
			if w.Code != http.StatusBadGateway {
				t.Errorf("status %d %s, want 502 (the name is never resolved)", w.Code, strings.TrimSpace(w.Body.String()))
			}
			if !strings.Contains(w.Body.String(), "Could not reach the address") {
				t.Errorf("body %q, want the unreachable wording", strings.TrimSpace(w.Body.String()))
			}
		})
	}
}

// A media directory that cannot be written to is a server-side storage fault,
// not an unreachable CDN: the link and its media are untouched, and the message
// must not send the user to look at their link.
func TestURLUploadStorageFailureIsNotReportedAsUnreachable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	body := makeSmallPNG(t)
	service := remoteUploadLink(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)),
			ContentLength: int64(len(body)), Header: http.Header{}}, nil
	})
	before, _ := storage.Global.Get("photo")
	if err := os.Chmod(config.MediaDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(config.MediaDir, 0o700) })
	w := httptest.NewRecorder()
	service.Upload(w, avifRequest(t, "photo", "", "http://ok.test/a.png", nil))
	after, _ := storage.Global.Get("photo")
	if after.HasImage != before.HasImage || after.MIMEType != before.MIMEType {
		t.Errorf("link changed after a storage failure: %+v -> %+v", before, after)
	}
	t.Logf("storage failure -> %d %s", w.Code, strings.TrimSpace(w.Body.String()))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status %d %q, want 500 for a storage fault", w.Code, strings.TrimSpace(w.Body.String()))
	}
	if !strings.Contains(w.Body.String(), "Failed to save media") {
		t.Errorf("body %q, want the storage wording, not a download verdict", strings.TrimSpace(w.Body.String()))
	}
}
