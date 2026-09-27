package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"lanpaper/middleware"
	"lanpaper/storage"
)

func Public(w http.ResponseWriter, r *http.Request) {
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

	// Enforce per-link access level BEFORE opening the file.
	if !middleware.AuthorizeLinkAccess(w, r, wp) {
		return
	}

	// Open once for both Stat and ServeContent to avoid a TOCTOU race.
	f, err := storage.OpenMedia(wp.ImagePath)
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

	mime := mediaContentType(wp.MIMEType)

	h := w.Header()
	h.Set("Content-Type", mime)
	// Link names are restricted to [A-Za-z0-9_-] so this is safe.
	h.Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.%s"`, wp.LinkName, wp.MIMEType))
	// Permissions and content can change at any time. A shared cache (or a
	// service worker) must not replay an old public image after access changes.
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")

	http.ServeContent(w, r, wp.LinkName+"."+wp.MIMEType, fi.ModTime(), f)
}
