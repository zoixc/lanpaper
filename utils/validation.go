package utils

import (
	"bytes"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strings"
)

// windowsAbsPath matches Windows-style absolute paths (e.g. C:\ or D:/).
var windowsAbsPath = regexp.MustCompile(`(?i)^[a-z]:[/\\]`)

// IsValidLocalPath reports whether path is a safe relative path with no
// traversal, absolute, or Windows-style components.
func IsValidLocalPath(path string) bool {
	if strings.Contains(path, "\x00") || windowsAbsPath.MatchString(path) {
		return false
	}
	clean := filepath.Clean(path)
	return !filepath.IsAbs(clean) &&
		!strings.HasPrefix(clean, "..") &&
		!strings.Contains(clean, "/..") &&
		!strings.HasPrefix(clean, "\\\\")
}

var allowedMIMETypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/gif": true,
	"image/webp": true, "image/bmp": true, "image/tiff": true,
	"video/mp4": true, "video/webm": true,
}

// IsAllowedMimeType reports whether mimeType is an accepted upload type.
func IsAllowedMimeType(mimeType string) bool {
	return allowedMIMETypes[strings.ToLower(mimeType)]
}

// magicBytes holds expected file signatures for supported types.
var magicBytes = map[string][]byte{
	"jpg":  {0xFF, 0xD8, 0xFF},
	"png":  {0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
	"gif":  {0x47, 0x49, 0x46, 0x38},
	"webp": {0x52, 0x49, 0x46, 0x46}, // RIFF prefix; WEBP marker at offset 8 checked below
	"bmp":  {0x42, 0x4D},
	"tif":  {0x49, 0x49, 0x2A, 0x00}, // little-endian TIFF
	"tiff": {0x4D, 0x4D, 0x00, 0x2A}, // big-endian TIFF
	"webm": {0x1A, 0x45, 0xDF, 0xA3}, // EBML header
	// mp4 validated via ftyp box check in ValidateFileType
}

// ValidateFileType verifies that data starts with the expected magic bytes
// for expectedExt.
func ValidateFileType(data []byte, expectedExt string) error {
	if len(data) < 16 {
		return fmt.Errorf("file too small to validate")
	}
	ext := strings.ToLower(strings.TrimPrefix(expectedExt, "."))
	if ext == "jpeg" {
		ext = "jpg"
	}
	switch ext {
	case "webp":
		if !bytes.HasPrefix(data, magicBytes["webp"]) {
			return fmt.Errorf("file does not match WebP magic bytes")
		}
		if len(data) >= 12 && string(data[8:12]) != "WEBP" {
			return fmt.Errorf("file has RIFF header but not WEBP format")
		}
		return nil
	case "mp4":
		if len(data) < 12 {
			return fmt.Errorf("file too small for MP4 validation")
		}
		if string(data[4:8]) != "ftyp" {
			return fmt.Errorf("file does not match MP4 structure")
		}
		return nil
	}
	magic, ok := magicBytes[ext]
	if !ok {
		return fmt.Errorf("unsupported file type: %s", ext)
	}
	if !bytes.HasPrefix(data, magic) {
		n := len(magic)
		if len(data) < n {
			n = len(data)
		}
		log.Printf("Security: magic bytes mismatch for %q: expected %v, got %v", ext, magic, data[:n])
		return fmt.Errorf("file content does not match extension %s", ext)
	}
	return nil
}

// dangerousRune drops shell-special characters and replaces spaces with '_'.
// Used by SanitizeFilename for a single-pass replacement.
func dangerousRune(r rune) rune {
	switch r {
	case '$', '`', '|', ';', '[', ']', '(', ')', '&', '<', '>', '"', '\'':
		return -1
	case ' ':
		return '_'
	}
	return r
}

// SanitizeFilename strips path components and removes dangerous characters.
func SanitizeFilename(name string) string {
	return strings.Map(dangerousRune, filepath.Base(name))
}
