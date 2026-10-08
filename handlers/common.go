// SPDX-License-Identifier: MIT

package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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
	// Hexadecimal modtime and size, as before — built without fmt, because this
	// runs on every media response, including every 304.
	h.Set("ETag", `"`+strconv.FormatInt(fi.ModTime().UnixNano(), 16)+
		"-"+strconv.FormatInt(fi.Size(), 16)+`"`)
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

// mediaMinBytesPerSec is the slowest client a media response is sized for,
// and maxMediaWriteBudget caps how long one response may hold the connection.
const (
	mediaMinBytesPerSec = 256 << 10 // 256 KiB/s
	maxMediaWriteBudget = 15 * time.Minute
)

// mediaWriteBudget returns how long writing size bytes may take at
// mediaMinBytesPerSec, and whether that is worth extending the deadline for.
// Kept separate from the deadline call so the arithmetic is testable without
// a connection.
func mediaWriteBudget(size int64) (time.Duration, bool) {
	if size <= 0 {
		return 0, false
	}
	budget := time.Duration(size/mediaMinBytesPerSec+1) * time.Second
	if budget <= time.Duration(config.HTTPWriteTimeout)*time.Second {
		return 0, false
	}
	return min(budget, maxMediaWriteBudget), true
}

// extendMediaDeadline lifts the server-wide write timeout for one media
// response. That timeout exists to cut off a stalled request, but a large file
// written to a slow client is not stalled: a 50 MB video at 256 KiB/s needs
// more than the 120 s default and would be truncated mid-file. The deadline is
// sized for the bytes the response has to send and never exceeds
// maxMediaWriteBudget, so a client that stops reading still loses the
// connection.
func extendMediaDeadline(w http.ResponseWriter, size int64) {
	if budget, ok := mediaWriteBudget(size); ok {
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(budget))
	}
}

// extendDeadline lifts the server-wide read/write timeouts for a long-running
// admin request. It is only reached after authentication; public requests
// keep the short server defaults.
func extendDeadline(w http.ResponseWriter, d time.Duration) {
	rc := http.NewResponseController(w)
	deadline := time.Now().Add(d)
	_ = rc.SetReadDeadline(deadline)
	_ = rc.SetWriteDeadline(deadline)
}
