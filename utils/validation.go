package utils

import (
	"bytes"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strings"
)

// windowsAbsPath matches Windows-style absolute paths like C:\ or D:/
var windowsAbsPath = regexp.MustCompile(`(?i)^[a-z]:[/\\]`)

// IsValidLocalPath validates that a path doesn't contain dangerous patterns.
func IsValidLocalPath(path string) bool {
	if strings.Contains(path, "\x00") {
		return false
	}

	// filepath.IsAbs returns false for Windows paths on Linux, so check explicitly.
	if windowsAbsPath.MatchString(path) {
		return false
	}

	cleanPath := filepath.Clean(path)

	if filepath.IsAbs(cleanPath) {
		return false
	}

	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, "/\..") {
		return false
	}

	// Reject UNC paths on Windows.
	if strings.HasPrefix(cleanPath, "\\\\") {
		return false
	}

	return true
}

// Magic bytes signatures for file type validation.
var magicBytes = map[string][]byte{
	"jpg":  {0xFF, 0xD8, 0xFF},
	"png":  {0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
	"gif":  {0x47, 0x49, 0x46, 0x38},
	"webp": {0x52, 0x49, 0x46, 0x46}, // RIFF (WebP starts with RIFF)
	"bmp":  {0x42, 0x4D},
	"tif":  {0x49, 0x49, 0x2A, 0x00}, // Little-endian TIFF
	"tiff": {0x4D, 0x4D, 0x00, 0x2A}, // Big-endian TIFF
	"webm": {0x1A, 0x45, 0xDF, 0xA3}, // EBML header for WebM/Matroska
	// mp4 is handled separately via ftyp box check
}

// ValidateFileType checks if file content matches expected type using magic bytes.
func ValidateFileType(data []byte, expectedExt string) error {
	if len(data) < 16 {
		return fmt.Errorf("file too small to validate")
	}

	expectedExt = strings.ToLower(strings.TrimPrefix(expectedExt, "."))

	if expectedExt == "jpeg" {
		expectedExt = "jpg"
	}

	// WebP needs both RIFF header and WEBP marker at offset 8.
	if expectedExt == "webp" {
		if !bytes.HasPrefix(data, magicBytes["webp"]) {
			return fmt.Errorf("file does not match WebP magic bytes")
		}
		if len(data) >= 12 && string(data[8:12]) != "WEBP" {
			return fmt.Errorf("file has RIFF header but not WEBP format")
		}
		return nil
	}

	// MP4: validate ftyp box at offset 4.
	if expectedExt == "mp4" {
		if len(data) < 12 {
			return fmt.Errorf("file too small for MP4 validation")
		}
		if string(data[4:8]) != "ftyp" {
			return fmt.Errorf("file does not match MP4 structure")
		}
		return nil
	}

	magic, exists := magicBytes[expectedExt]
	if !exists {
		return fmt.Errorf("unsupported file type: %s", expectedExt)
	}

	if !bytes.HasPrefix(data, magic) {
		maxShow := len(magic)
		if len(data) < maxShow {
			maxShow = len(data)
		}
		log.Printf("Security: file extension '%s' does not match magic bytes. Expected: %v, Got: %v",
			expectedExt, magic, data[:maxShow])
		return fmt.Errorf("file content does not match extension %s", expectedExt)
	}

	return nil
}

// SanitizeFilename strips path separators from a filename and is used solely
// for safe logging — the actual saved filename is always the link name.
func SanitizeFilename(filename string) string {
	return filepath.Base(filename)
}
