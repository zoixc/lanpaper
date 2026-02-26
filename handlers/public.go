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

	// Open once for both Stat and ServeContent to avoid a TOCTOU race.
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

	h := w.Header()
	h.Set("Content-Type", mime)
	h.Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.%s"`, wp.LinkName, wp.MIMEType))
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("X-Content-Type-Options", "nosniff")

	http.ServeContent(w, r, wp.LinkName+"."+wp.MIMEType, fi.ModTime(), f)
}
