package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"lanpaper/config"
	"lanpaper/storage"
	"lanpaper/utils"
)

func isValidLinkName(name string) bool { return utils.IsValidLinkName(name) }

// Report a missing gallery file as 404, while keeping path and permission
// failures indistinguishable to callers as 403.
func writeExternalFileError(w http.ResponseWriter, err error) {
	if errors.Is(err, os.ErrNotExist) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	http.Error(w, "Path outside allowed directory or file unavailable", http.StatusForbidden)
}

func isValidAccessLevel(level string) bool {
	return config.ValidAccessLevels[strings.ToLower(strings.TrimSpace(level))]
}

// mediaCacheControl lets a browser keep media but never reuse it unchecked.
// "no-cache" forces revalidation on every use, and handlers authorize the
// request before http.ServeContent answers 304 Not Modified, so a revoked or
// re-scoped link stops working immediately while unchanged files cost only a
// 304 instead of a full download. "private" keeps shared caches (proxies,
// CDNs) from storing copies at all.
const mediaCacheControl = "private, no-cache"

// publicMediaCacheControl keeps restricted media out of browser caches: a
// stored copy of a token or admin-only link would stay on the device's disk
// after the token is rotated or the access level changes. Public and LAN
// media is revalidated on every use instead (mediaCacheControl).
func publicMediaCacheControl(accessLevel string) string {
	switch storage.NormalizeAccessLevel(accessLevel) {
	case config.AccessPublic, config.AccessLocal:
		return mediaCacheControl
	}
	return "no-store"
}

// setMediaValidators adds an ETag next to the Last-Modified header that
// http.ServeContent derives from modTime. Last-Modified has one-second
// resolution; the ETag changes whenever a file is replaced, even twice in the
// same second, so a revalidation never confirms the previous image.
func setMediaValidators(h http.Header, fi os.FileInfo) {
	h.Set("ETag", fmt.Sprintf(`"%x-%x"`, fi.ModTime().UnixNano(), fi.Size()))
}

func mediaContentType(ext string) string {
	switch ext {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "tif", "tiff":
		return "image/tiff"
	case "mp4", "webm":
		return "video/" + ext
	default:
		return "image/" + ext
	}
}

// generateAccessToken returns a random URL-safe token with 256 bits of
// entropy. crypto/rand.Read never fails (Go 1.24+).
func generateAccessToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
