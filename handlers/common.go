package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"regexp"
	"strings"

	"lanpaper/config"
	"lanpaper/storage"
)

// reservedNames cannot be used as link names — they clash with existing routes.
var reservedNames = map[string]bool{
	"api": true, "admin": true, "static": true,
	"external": true, "data": true, "health": true,
	"sw.js": true, "favicon.ico": true, "robots.txt": true, "sitemap.xml": true,
	"manifest.json": true, "manifest.webmanifest": true,
}

// linkNameRe allows letters, digits, hyphens, underscores.
// The first character must be alphanumeric to avoid names like "-x".
var linkNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func isValidLinkName(name string) bool {
	return len(name) >= 1 && len(name) <= 64 &&
		!reservedNames[strings.ToLower(name)] &&
		linkNameRe.MatchString(name)
}

func isValidAccessLevel(level string) bool {
	return config.ValidAccessLevels[strings.ToLower(strings.TrimSpace(level))]
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
