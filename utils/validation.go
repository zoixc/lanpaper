package utils

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
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

// GetRealIP extracts real IP from request considering proxy headers
func GetRealIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}

// Magic bytes signatures for file type validation
var magicBytes = map[string][]byte{
	"jpg":  {0xFF, 0xD8, 0xFF},
	"png":  {0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
	"gif":  {0x47, 0x49, 0x46, 0x38},
	"webp": {0x52, 0x49, 0x46, 0x46}, // RIFF (WebP starts with RIFF)
	"bmp":  {0x42, 0x4D},
	"tif":  {0x49, 0x49, 0x2A, 0x00}, // Little-endian TIFF
	"tiff": {0x4D, 0x4D, 0x00, 0x2A}, // Big-endian TIFF
	"mp4":  {0x00, 0x00, 0x00}, // MP4 starts with ftyp at offset 4
	"webm": {0x1A, 0x45, 0xDF, 0xA3}, // EBML header for WebM/Matroska
}

// ValidateFileType checks if file content matches expected type using magic bytes
func ValidateFileType(data []byte, expectedExt string) error {
	if len(data) < 16 {
		return fmt.Errorf("file too small to validate")
	}

	expectedExt = strings.ToLower(strings.TrimPrefix(expectedExt, "."))

	// Normalize extensions
	if expectedExt == "jpeg" {
		expectedExt = "jpg"
	}

	magic, exists := magicBytes[expectedExt]
	if !exists {
		return fmt.Errorf("unsupported file type: %s", expectedExt)
	}

	// Special case for WebP (needs RIFF and WEBP check)
	if expectedExt == "webp" {
		if !bytes.HasPrefix(data, magic) {
			return fmt.Errorf("file does not match WebP magic bytes")
		}
		// Check for WEBP signature at offset 8
		if len(data) >= 12 && string(data[8:12]) != "WEBP" {
			return fmt.Errorf("file has RIFF header but not WEBP format")
		}
		return nil
	}

	// Special case for MP4 (ftyp box check)
	if expectedExt == "mp4" {
		if len(data) < 12 {
			return fmt.Errorf("file too small for MP4 validation")
		}
		// Check for ftyp at offset 4
		if string(data[4:8]) != "ftyp" {
			return fmt.Errorf("file does not match MP4 structure")
		}
		return nil
	}

	// Standard magic bytes check
	if !bytes.HasPrefix(data, magic) {
		log.Printf("Security: file extension '%s' does not match magic bytes. Expected: %v, Got: %v",
			expectedExt, magic, data[:min(len(magic), len(data))])
		return fmt.Errorf("file content does not match extension %s", expectedExt)
	}

	return nil
}

// IsAllowedMimeType checks if MIME type is allowed
func IsAllowedMimeType(mimeType string) bool {
	allowed := []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
		"image/bmp",
		"image/tiff",
		"video/mp4",
		"video/webm",
	}

	for _, a := range allowed {
		if mimeType == a {
			return true
		}
	}
	return false
}

// SanitizeFilename removes dangerous characters from filename
func SanitizeFilename(filename string) string {
	// Remove path separators
	filename = filepath.Base(filename)

	// Remove dangerous characters
	dangerous := []string{"..", "~", "$", "`", "|", ";", "&", "<", ">", "(", ")", "{", "}", "[", "]"}
	for _, char := range dangerous {
		filename = strings.ReplaceAll(filename, char, "")
	}

	return filename
}

func min(a, b int) int {
	if a < b {
			return a
	}
	return b
}
