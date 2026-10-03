// SPDX-License-Identifier: MIT

package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverAnswersAClean500(t *testing.T) {
	h := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(errors.New("open /data/media/secret.png: permission denied"))
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/photo?token=hunter2", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Internal server error") {
		t.Fatalf("body = %q", body)
	}
	// Nothing from the panic may reach the client: no error value, no path, no
	// stack frame, and no query string (a token link carries its secret there).
	for _, leak := range []string{"secret.png", "permission denied", "hunter2", "panic", "goroutine", ".go:", "middleware."} {
		if strings.Contains(body, leak) {
			t.Fatalf("response leaked %q: %s", leak, body)
		}
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q", ct)
	}
}

func TestRecoverLetsErrAbortHandlerThrough(t *testing.T) {
	// Aborting a response is not a bug: net/http must see the sentinel so it
	// closes the connection quietly instead of answering 500 for a request
	// whose response was deliberately abandoned.
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("half a body"))
		panic(http.ErrAbortHandler)
	}))

	rec := httptest.NewRecorder()
	defer func() {
		p := recover()
		if p == nil {
			t.Fatal("http.ErrAbortHandler was swallowed")
		}
		if err, ok := p.(error); !ok || !errors.Is(err, http.ErrAbortHandler) {
			t.Fatalf("re-raised %v (%T), want http.ErrAbortHandler", p, p)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("the recovery answered %d for an aborted response", rec.Code)
		}
	}()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/photo", nil))
	t.Fatal("the handler did not panic")
}

func TestRecoverLeavesAStartedResponseAlone(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("partial"))
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/photo", nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("a response already on the wire was replaced: status %d", rec.Code)
	}
	if body := rec.Body.String(); body != "partial" {
		t.Fatalf("body = %q, want the bytes written before the panic", body)
	}
}

func TestRecoverIsTransparentWithoutAPanic(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Custom", "kept")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("all good"))
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/photo", nil))

	if rec.Code != http.StatusTeapot || rec.Body.String() != "all good" || rec.Header().Get("X-Custom") != "kept" {
		t.Fatalf("status=%d body=%q header=%q", rec.Code, rec.Body.String(), rec.Header().Get("X-Custom"))
	}
}

// sendfileRecorder stands in for the connection: media is copied with ReadFrom
// (the sendfile fast path) and flushes reach the writer below.
type sendfileRecorder struct {
	*httptest.ResponseRecorder
	fromCalled  bool
	flushCalled bool
}

func (s *sendfileRecorder) ReadFrom(src io.Reader) (int64, error) {
	s.fromCalled = true
	return io.Copy(s.ResponseRecorder, src)
}

func (s *sendfileRecorder) Flush() {
	s.flushCalled = true
	s.ResponseRecorder.Flush()
}

func TestPanicRecorderStaysTransparent(t *testing.T) {
	inner := &sendfileRecorder{ResponseRecorder: httptest.NewRecorder()}
	rec := &panicRecorder{ResponseWriter: inner}

	if got := rec.Unwrap(); got != http.ResponseWriter(inner) {
		t.Fatal("Unwrap must return the wrapped writer, or http.ResponseController cannot reach the connection")
	}

	n, err := rec.ReadFrom(strings.NewReader("media"))
	if err != nil || n != int64(len("media")) {
		t.Fatalf("ReadFrom = %d, %v", n, err)
	}
	if !inner.fromCalled {
		t.Fatal("ReadFrom was not delegated: the sendfile fast path would be lost")
	}
	if !rec.started {
		t.Fatal("a body was written but the recorder does not know the response started")
	}
	if inner.Body.String() != "media" {
		t.Fatalf("body = %q", inner.Body.String())
	}

	rec.Flush()
	if !inner.flushCalled {
		t.Fatal("Flush was not delegated")
	}

	// A late WriteHeader still has to reach the client, once.
	inner2 := &sendfileRecorder{ResponseRecorder: httptest.NewRecorder()}
	rec2 := &panicRecorder{ResponseWriter: inner2}
	rec2.WriteHeader(http.StatusNoContent)
	if inner2.Code != http.StatusNoContent || !rec2.started {
		t.Fatalf("WriteHeader = %d, started %v", inner2.Code, rec2.started)
	}
}

func TestRecoverWorksUnderGzip(t *testing.T) {
	// This is the production order: Gzip(Recover(mux)). The recovered 500 has
	// to travel through the gzip writer, whose finish() only runs for the layer
	// that owns the response.
	h := Gzip(Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))

	req := httptest.NewRequest(http.MethodGet, "/photo", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Internal server error") {
		t.Fatalf("body = %q: the recovered response never reached the client", body)
	}
}
