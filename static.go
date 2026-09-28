package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Serve only application assets, never arbitrary files/symlinks placed in
// static/ (particularly legacy static/images and files on mounted volumes).
var staticAssets = map[string]bool{
	"css/style.css": true,
	"js/app.js": true, "js/compressor.js": true, "js/export-import.js": true, "js/settings-menu.js": true,
	"sw.js": true, "manifest.json": true,
	"favicon.svg": true, "logo.svg": true, "logo-dark.svg": true,
	"fonts/Rubik.ttf":                   true,
	"fonts/Disket/Disket-Mono-Bold.ttf": true, "fonts/Disket/Disket-Mono-Regular.ttf": true,
	"i18n/en.json": true, "i18n/ru.json": true, "i18n/de.json": true,
	"i18n/fr.json": true, "i18n/it.json": true, "i18n/es.json": true,
	"icons/apple-touch-icon.png": true, "icons/icon-192.png": true,
	"icons/icon-512.png": true, "icons/icon-maskable-512.png": true,
}

func serveStaticAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/static/")
	if !staticAssets[name] {
		http.NotFound(w, r)
		return
	}
	root, err := os.OpenRoot("static")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	// Also disallow final symlinks: a known asset path must not be linked to
	// a different, access-controlled file inside the static tree.
	fi, err := root.Lstat(name)
	if err != nil || !fi.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	f, err := root.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, err = f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	if strings.HasSuffix(name, ".svg") {
		h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	}
	if strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".css") || strings.HasSuffix(name, ".json") {
		h.Set("Cache-Control", "no-cache")
	} else {
		h.Set("Cache-Control", "public, max-age=86400")
	}
	http.ServeContent(w, r, filepath.Base(name), fi.ModTime(), f)
}
