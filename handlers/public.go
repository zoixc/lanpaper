package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"lanpaper/config"
	"lanpaper/middleware"
	"lanpaper/storage"
)

func Public(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		// A CORS preflight is answered here; with CORS_ORIGINS unset the
		// previous 405 stands, because an unconfigured server has nothing to
		// negotiate and must not start answering OPTIONS.
		if middleware.HandleCORSOptions(w, r) {
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.Path

	switch {
	case path == "/":
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	case path == "/admin",
		strings.HasPrefix(path, "/api/"),
		strings.HasPrefix(path, "/static/"):
		http.NotFound(w, r)
		return
	}

	id, ok := publicLinkName(path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	wp, exists := storage.Global.Get(id)
	if !exists || !wp.HasImage || wp.ImagePath == "" {
		http.NotFound(w, r)
		return
	}

	// Enforce per-link access level BEFORE opening the file.
	if !middleware.AuthorizeLinkAccess(w, r, wp) {
		return
	}

	// A version, a playlist item or the rotation clock picks the file; every
	// selector is resolved from the record, never from the query string.
	sel, ok := selectMedia(wp, r.URL.Query())
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Open once for both Stat and ServeContent to avoid a TOCTOU race.
	f, err := storage.OpenMedia(sel.path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		http.NotFound(w, r)
		return
	}

	h := w.Header()
	h.Set("Content-Type", mediaContentType(sel.ext))
	// Link names are restricted to [A-Za-z0-9_-] and extensions to the media
	// allow-list, so this is safe.
	h.Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.%s"`, wp.LinkName, sel.ext))
	h.Set("Cache-Control", publicMediaCacheControl(wp.AccessLevel))
	setMediaValidators(h, fi)
	h.Set("X-Content-Type-Options", "nosniff")

	// The recorder keeps the sendfile fast path (ReadFrom) and the deadline
	// control (Unwrap) of the writer it wraps.
	rec := &hitRecorder{ResponseWriter: w}
	http.ServeContent(rec, r, wp.LinkName+"."+sel.ext, fi.ModTime(), f)
	rec.record(wp.LinkName)
}

// publicLinkName maps a request path to a link name. Besides the canonical
// /{name} it accepts two cosmetic forms that clients insisting on a file
// extension need (some e-ink frames, TV apps and feed readers refuse a URL
// without one):
//
//	/{name}.{ext}   the extension is ignored — the stored media type decides
//	/{name}/latest  an explicit "give me the current file" alias
//
// Neither form widens what is reachable: the extension has to be on the media
// allow-list, so /manifest.json, /favicon.ico, /robots.txt and /sw.js keep
// resolving exactly as before, and the result is still validated as a link
// name, so a path containing a slash or a traversal segment never becomes a
// file path. A bare /latest is not an alias, which keeps a link actually named
// "latest" working.
func publicLinkName(path string) (string, bool) {
	name := strings.TrimSuffix(path, "/")
	if len(name) < 2 {
		return "", false
	}
	name = name[1:]
	name = trimLatestSuffix(name)
	if i := strings.LastIndex(name, "."); i > 0 && config.AllowedMediaExts[strings.ToLower(name[i:])] {
		name = name[:i]
	}
	name = trimLatestSuffix(name)
	if !isValidLinkName(name) {
		return "", false
	}
	return name, true
}

func trimLatestSuffix(name string) string {
	const suffix = "/latest"
	if len(name) > len(suffix) && strings.EqualFold(name[len(name)-len(suffix):], suffix) {
		return name[:len(name)-len(suffix)]
	}
	return name
}

// mediaSelection is the concrete file a public request resolved to.
type mediaSelection struct {
	path string
	ext  string
}

// liveVersion is the version number of the file a link currently serves.
// Records written before versioning existed carry 0, which reads as 1.
func liveVersion(wp *storage.Wallpaper) uint64 {
	if wp.CurrentVersion == 0 {
		return 1
	}
	return wp.CurrentVersion
}

// selectMedia resolves ?v= (an archived version), ?i= (a playlist item id, 0
// for the live file) and time-based rotation into one file. An unusable
// selector is a 404: it never falls back to different bytes than the ones that
// were asked for, because a frame that pinned ?v=3 must not silently start
// showing something else.
func selectMedia(wp *storage.Wallpaper, q url.Values) (mediaSelection, bool) {
	versionStr, itemStr := q.Get("v"), q.Get("i")

	switch {
	case versionStr != "" && itemStr != "":
		return mediaSelection{}, false
	case versionStr != "":
		version, err := strconv.ParseUint(versionStr, 10, 64)
		if err != nil || version == 0 {
			return mediaSelection{}, false
		}
		if version == liveVersion(wp) {
			return liveSelection(wp)
		}
		entry, found := storage.FindHistory(wp.History, version)
		if !found || !validStoredExt(entry.Ext) {
			return mediaSelection{}, false
		}
		return mediaSelection{
			path: storage.HistoryPath(wp.LinkName, entry.Version, entry.Ext),
			ext:  entry.Ext,
		}, true
	case itemStr != "":
		id, err := strconv.Atoi(itemStr)
		if err != nil || id < 0 {
			return mediaSelection{}, false
		}
		if id == 0 {
			return liveSelection(wp)
		}
		item, found := storage.ItemByID(wp.Items, id)
		if !found || !validStoredExt(item.Ext) {
			return mediaSelection{}, false
		}
		return mediaSelection{path: storage.ItemPath(wp.LinkName, item.ID, item.Ext), ext: item.Ext}, true
	}

	// No selector: the rotation clock decides, and a link without a playlist
	// always resolves to the live file (index 0).
	index := wp.PlaylistNow()
	if index <= 0 || index > len(wp.Items) {
		return liveSelection(wp)
	}
	item := wp.Items[index-1]
	if !validStoredExt(item.Ext) {
		return liveSelection(wp)
	}
	return mediaSelection{path: storage.ItemPath(wp.LinkName, item.ID, item.Ext), ext: item.Ext}, true
}

func liveSelection(wp *storage.Wallpaper) (mediaSelection, bool) {
	if wp.ImagePath == "" || !validStoredExt(wp.MIMEType) {
		return mediaSelection{}, false
	}
	return mediaSelection{path: wp.ImagePath, ext: wp.MIMEType}, true
}

func validStoredExt(ext string) bool {
	return config.AllowedMediaExts["."+ext]
}

// hitRecorder counts what a public media response really sent, so the in-memory
// statistics stay exact for range requests, HEAD and 304s. ReadFrom and Unwrap
// are implemented so http.ServeContent keeps the sendfile fast path and
// http.NewResponseController still reaches the connection underneath.
type hitRecorder struct {
	http.ResponseWriter
	status  int
	written int64
}

func (h *hitRecorder) WriteHeader(code int) {
	if h.status == 0 {
		h.status = code
	}
	h.ResponseWriter.WriteHeader(code)
}

func (h *hitRecorder) Write(b []byte) (int, error) {
	if h.status == 0 {
		h.status = http.StatusOK
	}
	n, err := h.ResponseWriter.Write(b)
	h.written += int64(n)
	return n, err
}

func (h *hitRecorder) ReadFrom(src io.Reader) (int64, error) {
	if h.status == 0 {
		h.status = http.StatusOK
	}
	if rf, ok := h.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(src)
		h.written += n
		return n, err
	}
	// Nothing below us can sendfile: copy through a buffer that writes straight
	// to the wrapped writer, so every byte is counted exactly once.
	buf := make([]byte, 32*1024)
	var total int64
	for {
		read, readErr := src.Read(buf)
		if read > 0 {
			written, writeErr := h.ResponseWriter.Write(buf[:read])
			total += int64(written)
			h.written += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != read {
				return total, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return total, nil
			}
			return total, readErr
		}
	}
}

func (h *hitRecorder) Unwrap() http.ResponseWriter { return h.ResponseWriter }

// record stores the counters once the response is complete. A rejected request
// (404, 416) is not a hit; a 304 or a HEAD is, with the bytes it actually sent.
func (h *hitRecorder) record(linkName string) {
	if h.status == 0 {
		h.status = http.StatusOK
	}
	if h.status >= 400 {
		return
	}
	storage.RecordHit(linkName, h.written)
}
