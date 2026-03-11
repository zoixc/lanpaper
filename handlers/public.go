package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"lanpaper/storage"
)

// mimeType returns the correct Content-Type for the stored MIME extension.
func mimeType(ext string) string {
	switch ext {
	case "mp4", "webm":
		return "video/" + ext
	case "jpg":
		// "image/jpg" is non-standard; the correct type is "image/jpeg".
		return "image/jpeg"
	default:
		return "image/" + ext
	}
}

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

	h := w.Header()
	h.Set("Content-Type", mimeType(wp.MIMEType))
	h.Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.%s"`, wp.LinkName, wp.MIMEType))
	// Not immutable: the same URL path can be reassigned to a different image.
	h.Set("Cache-Control", "public, max-age=60, must-revalidate")
	h.Set("X-Content-Type-Options", "nosniff")

	http.ServeContent(w, r, wp.LinkName+"."+wp.MIMEType, fi.ModTime(), f)
}
