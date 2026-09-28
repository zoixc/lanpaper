package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strings"

	"lanpaper/config"
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
