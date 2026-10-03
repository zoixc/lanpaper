// SPDX-License-Identifier: MIT

package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAcceptsGzip(t *testing.T) {
	for header, want := range map[string]bool{
		"gzip":                    true,
		"GZip":                    true,
		"deflate, gzip;q=0.5, br": true,
		"gzip ; q=1.0":            true,
		"gzip;q=0":                false,
		"gzip; q=0.000":           false,
		"br, deflate":             false,
		"x-gzip":                  false,
		"":                        false,
	} {
		if got := acceptsGzip(header); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}

func serveGzip(t *testing.T, h http.HandlerFunc, reqHeaders map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	for k, v := range reqHeaders {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	Gzip(h).ServeHTTP(rec, req)
	return rec
}

func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGzipCompressesLargeTextWrittenInChunks(t *testing.T) {
	want := strings.Repeat(`{"linkName":"wallpaper","category":"image"},`, 100)
	rec := serveGzip(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"v1"`)
		for i := 0; i < len(want); i += 50 {
			_, _ = io.WriteString(w, want[i:min(i+50, len(want))])
		}
	}, nil)
	h := rec.Header()
	if h.Get("Content-Encoding") != "gzip" || h.Get("Vary") != "Accept-Encoding" || h.Get("ETag") != `W/"v1"` {
		t.Fatalf("unexpected headers: %v", h)
	}
	if got := gunzip(t, rec.Body.Bytes()); string(got) != want {
		t.Fatal("decompressed body differs")
	}
	if rec.Body.Len() >= len(want)/4 {
		t.Fatalf("poor compression: %d of %d bytes", rec.Body.Len(), len(want))
	}
}

func TestGzipLeavesOtherResponsesUntouched(t *testing.T) {
	large := strings.Repeat("a", 4096)
	cases := []struct {
		name    string
		handler http.HandlerFunc
		req     map[string]string
		vary    bool
	}{
		{"small body", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ok":true}`)
		}, nil, true},
		{"small declared length", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/css")
			w.Header().Set("Content-Length", "10")
			_, _ = io.WriteString(w, "body{a:b}\n")
		}, nil, true},
		{"media", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, large)
		}, nil, false},
		{"no content type", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, large)
		}, nil, false},
		{"already encoded", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Encoding", "br")
			_, _ = io.WriteString(w, large)
		}, nil, false},
		{"client refuses gzip", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, large)
		}, map[string]string{"Accept-Encoding": "gzip;q=0"}, true},
		{"range request", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, large)
		}, map[string]string{"Range": "bytes=0-1"}, false},
		{"not modified", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusNotModified)
		}, nil, false},
	}
	for _, tc := range cases {
		rec := serveGzip(t, tc.handler, tc.req)
		h := rec.Header()
		if h.Get("Content-Encoding") == "gzip" {
			t.Errorf("%s: compressed", tc.name)
		}
		if (h.Get("Vary") != "") != tc.vary {
			t.Errorf("%s: Vary = %q", tc.name, h.Get("Vary"))
		}
		if tc.name == "media" && rec.Body.String() != large {
			t.Errorf("%s: body altered", tc.name)
		}
	}
}

func TestGzipKeepsStatusAndHeadRequests(t *testing.T) {
	body := strings.Repeat("not found ", 200)
	rec := serveGzip(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, body)
	}, nil)
	if rec.Code != http.StatusNotFound || rec.Header().Get("Content-Encoding") != "gzip" ||
		string(gunzip(t, rec.Body.Bytes())) != body {
		t.Fatalf("status or body lost: %d %v", rec.Code, rec.Header())
	}

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec = httptest.NewRecorder()
	Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", "5000")
	})).ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") != "" || rec.Header().Get("Content-Length") != "5000" {
		t.Fatalf("HEAD response changed: %v", rec.Header())
	}
}

// readFromRecorder reports whether the sendfile-capable ReadFrom path was used.
type readFromRecorder struct {
	*httptest.ResponseRecorder
	readFrom bool
}

func (r *readFromRecorder) ReadFrom(src io.Reader) (int64, error) {
	r.readFrom = true
	return io.Copy(r.ResponseRecorder, src)
}

func TestGzipKeepsReadFromForUncompressedBodies(t *testing.T) {
	for _, tc := range []struct {
		contentType string
		wantDirect  bool
	}{{"video/mp4", true}, {"text/css", false}} {
		rec := &readFromRecorder{ResponseRecorder: httptest.NewRecorder()}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		payload := strings.Repeat("x", 8192)
		Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", tc.contentType)
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			// The copy http.ServeContent performs (a LimitedReader has no WriteTo).
			_, _ = io.CopyN(w, strings.NewReader(payload), int64(len(payload)))
		})).ServeHTTP(rec, req)
		if rec.readFrom != tc.wantDirect {
			t.Errorf("%s: underlying ReadFrom used = %v, want %v", tc.contentType, rec.readFrom, tc.wantDirect)
		}
		got := rec.Body.Bytes()
		if !tc.wantDirect {
			got = gunzip(t, got)
		}
		if string(got) != payload {
			t.Errorf("%s: body differs", tc.contentType)
		}
	}
}

func TestGzipFlushAndResponseControllerOverRealConnection(t *testing.T) {
	srv := httptest.NewServer(Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Uploads extend their deadlines through the wrapper.
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(time.Minute)); err != nil {
			t.Errorf("SetWriteDeadline through wrapper: %v", err)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "first ")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("Flush through wrapper: %v", err)
		}
		_, _ = io.WriteString(w, "second")
	})))
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL) // the transport requests and decodes gzip
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !resp.Uncompressed || string(body) != "first second" {
		t.Fatalf("flushed response: uncompressed=%v body=%q", resp.Uncompressed, body)
	}
}

func TestGzipWritesNothingAfterPanic(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	func() {
		defer func() { _ = recover() }()
		Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"partial":`)
			panic(http.ErrAbortHandler)
		})).ServeHTTP(rec, req)
	}()
	if rec.Body.Len() != 0 || rec.Flushed {
		t.Fatalf("partial body sent after panic: %q", rec.Body.String())
	}
}

func TestGzipMediaType(t *testing.T) {
	for in, want := range map[string]string{
		"text/css; charset=utf-8": "text/css",
		" Application/JSON ":      "application/json",
		"image/svg+xml":           "image/svg+xml",
		"":                        "",
	} {
		if got := mediaType(in); got != want {
			t.Errorf("mediaType(%q) = %q, want %q", in, got, want)
		}
	}
}
