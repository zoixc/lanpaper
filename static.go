// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

// staticAssets is the allowlist of application files served from static/.
// Arbitrary files or symlinks placed in static/ (particularly the legacy
// static/images directory or files on mounted volumes) are never served.
var staticAssets = map[string]bool{
	"css/style.css":                   true,
	"js/app.js":                       true,
	"js/api.js":                       true,
	"js/login.js":                     true,
	"js/compressor.js":                true,
	"js/export-import.js":             true,
	"js/features.js":                  true,
	"js/feature-domain.js":            true,
	"js/upload-feature.js":            true,
	"js/prepaint.js":                  true,
	"js/settings-menu.js":             true,
	"js/state.js":                     true,
	"sw.js":                           true,
	"manifest.json":                   true,
	"favicon.svg":                     true,
	"logo.svg":                        true,
	"logo-dark.svg":                   true,
	"fonts/golos-text-latin.woff2":    true,
	"fonts/golos-text-cyrillic.woff2": true,
	"fonts/OFL-Golos-Text.txt":        true,
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

// Asset URLs carry a content hash as ?v=. The hashes are computed once when the
// process starts, from the allowlisted files; a request with the current hash
// may be cached forever, because any change to a file changes the URL the page
// uses. A file that cannot be read gets no hash and is served as before.
var staticVersions = computeStaticVersions()

func computeStaticVersions() map[string]string {
	versions := make(map[string]string, len(staticAssets))
	for asset := range staticAssets {
		f, _, err := openStaticAsset(asset)
		if err != nil {
			continue
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err == nil {
			versions[asset] = hex.EncodeToString(h.Sum(nil))[:12]
		}
	}
	return versions
}

func staticVersion(name string) string {
	return staticVersions[name]
}

var staticRefPattern = regexp.MustCompile(`"/static/([A-Za-z0-9_./-]+)"`)

// versionStaticRefs adds ?v=<hash> to every double-quoted allowlisted /static/
// reference in the admin page. Other references are left as they are.
func versionStaticRefs(page []byte) []byte {
	return staticRefPattern.ReplaceAllFunc(page, func(m []byte) []byte {
		name := string(m[len(`"/static/`) : len(m)-1])
		v := staticVersion(name)
		if !staticAssets[name] || v == "" {
			return m
		}
		return []byte(`"/static/` + name + `?v=` + v + `"`)
	})
}

// serveAdminPage serves admin.html for a signed-in user.
func serveAdminPage(w http.ResponseWriter, r *http.Request) {
	serveHTMLPage(w, r, "admin.html")
}

// serveLoginPage serves the sign-in form shown to everyone who is not signed in.
func serveLoginPage(w http.ResponseWriter, r *http.Request) {
	serveHTMLPage(w, r, "login.html")
}

// serveHTMLPage serves one of the application's HTML pages with its static
// references versioned. The page itself is never cached.
func serveHTMLPage(w http.ResponseWriter, r *http.Request, file string) {
	w.Header().Set("Cache-Control", "no-store")
	f, err := os.Open(file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	page, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "Admin page unavailable", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, file, fi.ModTime(), bytes.NewReader(versionStaticRefs(page)))
}

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
	// The manifest has its own media type: Chrome warns about plain
	// application/json, and ServeContent would pick exactly that from ".json".
	if name == "manifest.json" {
		h.Set("Content-Type", "application/manifest+json")
	}
	switch path.Ext(name) {
	case ".svg":
		h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		h.Set("Cache-Control", "public, max-age=86400")
	case ".js", ".css", ".json":
		if v := r.URL.Query().Get("v"); v != "" && v == staticVersion(name) {
			// The URL names this exact content, so the browser never needs to ask again.
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// Unversioned URLs (and any stale hash) always revalidate so upgrades take effect.
			h.Set("Cache-Control", "no-cache")
		}
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
