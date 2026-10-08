// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// staticAssets is the allowlist of application files served from static/.
// Arbitrary files or symlinks placed in static/ (particularly the legacy
// static/images directory or files on mounted volumes) are never served.
var staticAssets = map[string]bool{
	"css/style.css":                   true,
	"js/app.js":                       true,
	"js/compressor.js":                true,
	"js/export-import.js":             true,
	"js/prepaint.js":                  true,
	"js/settings-menu.js":             true,
	"sw.js":                           true,
	"manifest.json":                   true,
	"favicon.svg":                     true,
	"logo.svg":                        true,
	"logo-dark.svg":                   true,
	"fonts/manrope-latin.woff2":       true,
	"fonts/manrope-cyrillic.woff2":    true,
	"fonts/unbounded-latin-500.woff2": true,
	"fonts/OFL-Manrope.txt":           true,
	"fonts/OFL-Unbounded.txt":         true,
	"i18n/en.json":                    true,
	"i18n/ru.json":                    true,
	"i18n/de.json":                    true,
	"i18n/fr.json":                    true,
	"i18n/it.json":                    true,
	"i18n/es.json":                    true,
	"icons/apple-touch-icon.png":      true,
	"icons/icon-192.png":              true,
	"icons/icon-512.png":              true,
	"icons/icon-maskable-512.png":     true,
}

var errNotRegular = errors.New("not a regular file")

// openStaticAsset opens a file below static/ through os.Root, so the path
// cannot escape the directory. A final symlink is rejected as well: a known
// asset name must not be redirected to another file inside the static tree
// (for example legacy, access-controlled media in static/images). Comparing
// the Lstat result with the opened descriptor also defeats a swap between
// the check and the open.
func openStaticAsset(name string) (*os.File, os.FileInfo, error) {
	root, err := os.OpenRoot("static")
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	linfo, err := root.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !linfo.Mode().IsRegular() {
		return nil, nil, errNotRegular
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || !os.SameFile(linfo, fi) {
		f.Close()
		return nil, nil, errNotRegular
	}
	return f, fi, nil
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
	f, fi, err := openStaticAsset(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	switch path.Ext(name) {
	case ".svg":
		h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		h.Set("Cache-Control", "public, max-age=86400")
	case ".js", ".css", ".json":
		// Not content-hashed: always revalidate so upgrades take effect.
		h.Set("Cache-Control", "no-cache")
	case ".woff2":
		// Not in Go's built-in MIME table; do not depend on /etc/mime.types.
		h.Set("Content-Type", "font/woff2")
		h.Set("Cache-Control", "public, max-age=86400")
	case ".txt":
		h.Set("Content-Type", "text/plain; charset=utf-8")
		h.Set("Cache-Control", "public, max-age=86400")
	default:
		h.Set("Cache-Control", "public, max-age=86400")
	}
	http.ServeContent(w, r, path.Base(name), fi.ModTime(), f)
}

// serveServiceWorker serves the service worker from the root path so its
// scope can cover the whole app (Service-Worker-Allowed: /). Registered at
// /static/sw.js the scope would be limited to /static/ and the SW would
// never control /admin or the public link URLs.
func serveServiceWorker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	f, fi, err := openStaticAsset("sw.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Service-Worker-Allowed", "/")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	http.ServeContent(w, r, "sw.js", fi.ModTime(), f)
}

// robotsBody keeps crawlers off the media URLs. Everything below "/" is either
// an admin route or a mutable link whose bytes change without the URL changing,
// so an indexed copy is stale by design — and every crawl re-downloads a file
// the operator is paying bandwidth for.
const robotsBody = "User-agent: *\nDisallow: /\n"

// serveRobotsTxt answers the one path a crawler asks for before anything else.
// It replaces the 404 the reserved name produced, which only made bots retry.
func serveRobotsTxt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=3600")
	h.Set("X-Content-Type-Options", "nosniff")
	// A zero modTime means no Last-Modified: the body never changes, so there
	// is nothing to revalidate against.
	http.ServeContent(w, r, "robots.txt", time.Time{}, strings.NewReader(robotsBody))
}
