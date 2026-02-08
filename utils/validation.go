package utils

import (
	"path/filepath"
	"strings"
)

// IsValidLocalPath validates that a path doesn't contain dangerous patterns
func IsValidLocalPath(path string) bool {
	// Check for null bytes
	if strings.Contains(path, "\x00") {
		return false
	}

	cleanPath := filepath.Clean(path)

	// Reject absolute paths
	if filepath.IsAbs(cleanPath) {
		return false
	}

	// Reject paths trying to escape (..)
	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, "/..") {
		return false
	}

	// Reject UNC paths on Windows
	if strings.HasPrefix(cleanPath, "\\\\") {
		return false
	}

	return true
}

// ClientIP extracts the real client IP from request headers
func ClientIP(r interface{ Header() interface{ Get(string) string }; RemoteAddr() string }) string {
	headers := r.Header()
	if xr := headers.Get("X-Real-IP"); xr != "" {
		return xr
	}
	if xf := headers.Get("X-Forwarded-For"); xf != "" {
		return strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	// This is a simplified version - in real code we'd need proper type assertions
	return r.RemoteAddr()
}
