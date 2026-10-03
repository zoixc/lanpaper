// SPDX-License-Identifier: MIT

package middleware

import (
	"errors"
	"io"
	"log"
	"net/http"
	"runtime/debug"
)

// Recover turns a panic in a handler into a clean 500 instead of a dropped
// connection. net/http already keeps a panicking handler from killing the
// process, but it answers nothing at all: the client sees a reset and the log
// only says "http: panic serving ...".
//
// It sits inside Gzip so a recovered response still goes through the normal
// writer chain (Gzip deliberately does not flush a half-written body after a
// panic, and finish() only runs for the layer that owns the response).
//
// A panic that happens after the status line is on the wire cannot be replaced
// — the headers are gone — so it is only logged. Nothing about the panic is
// ever sent to the client: no value, no type, no stack, no file names.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &panicRecorder{ResponseWriter: w}
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			// http.ErrAbortHandler is the documented way to abandon a response
			// (a client that vanished mid-download, bytes that no longer match
			// the header already sent). It belongs to net/http, which suppresses
			// the log entry and closes the connection instead of inventing a
			// response, so it is re-raised untouched.
			if err, ok := p.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(p)
			}
			// Only the path is logged: on token links the query string carries
			// the secret, and a panic log must not become a token dump.
			log.Printf("Panic serving %s %s: %v\n%s", r.Method, r.URL.Path, p, debug.Stack())
			if rec.started {
				return
			}
			http.Error(rec, "Internal server error", http.StatusInternalServerError)
		}()
		next.ServeHTTP(rec, r)
	})
}

// panicRecorder tracks whether a response has begun. ReadFrom, Flush and
// Unwrap are delegated so public media keeps the sendfile fast path and
// http.NewResponseController still reaches the connection underneath (uploads
// extend their own deadlines through it).
type panicRecorder struct {
	http.ResponseWriter
	started bool
}

func (p *panicRecorder) WriteHeader(code int) {
	p.started = true
	p.ResponseWriter.WriteHeader(code)
}

func (p *panicRecorder) Write(b []byte) (int, error) {
	p.started = true
	return p.ResponseWriter.Write(b)
}

func (p *panicRecorder) ReadFrom(src io.Reader) (int64, error) {
	p.started = true
	if rf, ok := p.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	return io.Copy(struct{ io.Writer }{p.ResponseWriter}, src)
}

func (p *panicRecorder) Flush() {
	if f, ok := p.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (p *panicRecorder) Unwrap() http.ResponseWriter { return p.ResponseWriter }
