package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// gzipMinSize is the smallest body worth compressing.
const gzipMinSize = 1024

// gzipLevel favours server resources over ratio. Measured on the app's own
// assets, BestSpeed needs about half the CPU of level 5 and a quarter less
// memory per writer, for 4-5 percentage points of ratio (style.css 27% vs
// 22% of the original, a 1000-link API response 9% vs 8%).
const gzipLevel = gzip.BestSpeed

var gzipWriters = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(io.Discard, gzipLevel)
	return w
}}

// compressibleTypes are text formats. Media (JPEG, PNG, WebP, video) is
// already compressed and is always streamed untouched, which keeps sendfile
// and range requests working.
var compressibleTypes = map[string]bool{
	"text/html":                 true,
	"text/css":                  true,
	"text/plain":                true,
	"text/javascript":           true,
	"application/javascript":    true,
	"application/json":          true,
	"application/manifest+json": true,
	"image/svg+xml":             true,
}

// Gzip compresses text responses for clients that accept gzip.
//
// BREACH needs attacker-chosen text reflected into the same compressed body
// as a secret. No response here reflects request input: JSON is rendered
// from stored state that only the authenticated admin can change, and query
// parameters only select, sort or page through it.
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Byte ranges address the identity encoding, and HEAD has no body.
		if r.Method == http.MethodHead || r.Header.Get("Range") != "" {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w, accepts: acceptsGzip(r.Header.Get("Accept-Encoding"))}
		next.ServeHTTP(gw, r)
		// Not deferred: after a panic nothing buffered may be sent as if the
		// response were complete.
		gw.finish()
	})
}

// acceptsGzip reports whether an Accept-Encoding header allows gzip.
func acceptsGzip(header string) bool {
	for part := range strings.SplitSeq(header, ",") {
		coding, params, _ := strings.Cut(part, ";")
		if !strings.EqualFold(strings.TrimSpace(coding), "gzip") {
			continue
		}
		for param := range strings.SplitSeq(params, ";") {
			k, v, _ := strings.Cut(param, "=")
			if strings.EqualFold(strings.TrimSpace(k), "q") {
				q, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
				return err == nil && q > 0
			}
		}
		return true
	}
	return false
}

type gzipState uint8

const (
	// gzipPending: status recorded, the body is buffered until it is known
	// whether it reaches gzipMinSize.
	gzipPending gzipState = iota
	gzipPassthrough
	gzipActive
)

type gzipResponseWriter struct {
	http.ResponseWriter
	accepts bool
	code    int // 0 until the handler sets a final status
	state   gzipState
	buf     []byte
	gz      *gzip.Writer
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	if g.code != 0 {
		return
	}
	if code >= 100 && code < 200 {
		g.ResponseWriter.WriteHeader(code) // informational, not final
		return
	}
	g.code = code
	h := g.Header()
	if !compressibleTypes[mediaType(h.Get("Content-Type"))] || !bodyAllowed(code) || h.Get("Content-Encoding") != "" {
		g.passthrough()
		return
	}
	// Caches must key text responses by encoding, compressed or not.
	h.Add("Vary", "Accept-Encoding")
	switch n, err := strconv.ParseInt(h.Get("Content-Length"), 10, 64); {
	case !g.accepts, err == nil && n < gzipMinSize:
		g.passthrough()
	case err == nil:
		g.compress()
	}
	// Unknown length: stay pending and decide on the buffered body.
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if g.code == 0 {
		g.WriteHeader(http.StatusOK)
	}
	switch g.state {
	case gzipPassthrough:
		return g.ResponseWriter.Write(b)
	case gzipActive:
		return g.gz.Write(b)
	}
	g.buf = append(g.buf, b...)
	if len(g.buf) >= gzipMinSize {
		if err := g.compress(); err != nil {
			return 0, err
		}
	}
	return len(b), nil
}

// ReadFrom keeps the sendfile fast path of http.ServeContent for bodies that
// are not compressed.
func (g *gzipResponseWriter) ReadFrom(src io.Reader) (int64, error) {
	if g.code == 0 {
		g.WriteHeader(http.StatusOK)
	}
	if rf, ok := g.ResponseWriter.(io.ReaderFrom); ok && g.state == gzipPassthrough {
		return rf.ReadFrom(src)
	}
	return io.Copy(struct{ io.Writer }{g}, src)
}

// Flush sends everything written so far; a flushed body is compressed.
func (g *gzipResponseWriter) Flush() {
	if g.code == 0 {
		g.WriteHeader(http.StatusOK)
	}
	if g.state == gzipPending {
		_ = g.compress()
	}
	if g.gz != nil {
		_ = g.gz.Flush()
	}
	_ = http.NewResponseController(g.ResponseWriter).Flush()
}

// Unwrap lets http.ResponseController reach the connection, e.g. for the
// per-request deadlines that uploads extend.
func (g *gzipResponseWriter) Unwrap() http.ResponseWriter { return g.ResponseWriter }

func (g *gzipResponseWriter) passthrough() {
	g.state = gzipPassthrough
	g.ResponseWriter.WriteHeader(g.code)
}

func (g *gzipResponseWriter) compress() error {
	h := g.Header()
	h.Del("Content-Length")
	h.Del("Accept-Ranges")
	if etag := h.Get("ETag"); etag != "" && !strings.HasPrefix(etag, "W/") {
		h.Set("ETag", "W/"+etag) // not byte-identical to the identity body
	}
	h.Set("Content-Encoding", "gzip")
	g.state = gzipActive
	g.ResponseWriter.WriteHeader(g.code)
	g.gz = gzipWriters.Get().(*gzip.Writer)
	g.gz.Reset(g.ResponseWriter)
	buf := g.buf
	g.buf = nil
	_, err := g.gz.Write(buf)
	return err
}

// finish completes the response after the handler has returned.
func (g *gzipResponseWriter) finish() {
	switch g.state {
	case gzipPending:
		if g.code == 0 {
			return // nothing written; net/http sends an empty 200
		}
		g.passthrough() // body stayed below gzipMinSize
		if len(g.buf) > 0 {
			_, _ = g.ResponseWriter.Write(g.buf)
		}
	case gzipActive:
		_ = g.gz.Close()
		g.gz.Reset(io.Discard)
		gzipWriters.Put(g.gz)
		g.gz = nil
	}
}

// mediaType returns the lower-case type/subtype of a Content-Type value
// without the allocations of mime.ParseMediaType (this runs per response).
func mediaType(contentType string) string {
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = contentType[:i]
	}
	return strings.ToLower(strings.TrimSpace(contentType))
}

func bodyAllowed(code int) bool {
	return code != http.StatusNoContent && code != http.StatusNotModified &&
		code != http.StatusPartialContent
}
