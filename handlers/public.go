package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"lanpaper/storage"
)

func Public(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if path == "/" {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	if path == "/admin" || strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/static/") {
		http.NotFound(w, r)
		return
	}

	cleanPath := strings.TrimSuffix(path, "/")
	if len(cleanPath) < 2 {
		http.NotFound(w, r)
		return
	}
	id := cleanPath[1:]

	if !isValidLinkName(id) {
		http.NotFound(w, r)
		return
	}

	wp, exists := storage.Global.Get(id)
	if !exists || !wp.HasImage || wp.ImagePath == "" {
		http.NotFound(w, r)
		return
	}

	// Open the file once — use the returned *os.File for both Stat and
	// ServeContent, eliminating the extra syscall and the TOCTOU window
	// between a separate Stat + Open pair.
	f, err := os.Open(wp.ImagePath)
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

	mime := "image/" + wp.MIMEType
	if wp.MIMEType == "mp4" || wp.MIMEType == "webm" {
		mime = "video/" + wp.MIMEType
	}

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.%s"`, wp.LinkName, wp.MIMEType))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	http.ServeContent(w, r, wp.LinkName+"."+wp.MIMEType, fi.ModTime(), f)
}
