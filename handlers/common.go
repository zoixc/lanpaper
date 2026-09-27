package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"strings"

	"lanpaper/config"
	"lanpaper/storage"
	"lanpaper/utils"
)

func isValidLinkName(name string) bool { return utils.IsValidLinkName(name) }

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

// generateAccessToken returns a cryptographically random URL-safe token.
func generateAccessToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ensureAccessDefaults fills missing access fields on a wallpaper.
func ensureAccessDefaults(wp *storage.Wallpaper) {
	if wp.AccessLevel == "" {
		wp.AccessLevel = config.AccessPublic
	} else {
		wp.AccessLevel = storage.NormalizeAccessLevel(wp.AccessLevel)
	}
}
