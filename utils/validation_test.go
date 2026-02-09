package utils

import (
	"testing"
)

func TestIsValidLocalPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"valid relative path", "images/test.jpg", true},
		{"valid subdirectory", "folder/subfolder/image.png", true},
		{"path traversal with ..", "../etc/passwd", false},
		{"path traversal in middle", "images/../../../etc/passwd", false},
		{"absolute path", "/etc/passwd", false},
		{"windows absolute path", "C:\\Windows\\System32", false},
		{"null byte injection", "image\x00.jpg", false},
		{"UNC path", "\\\\server\\share", false},
		{"single file", "image.jpg", true},
		{"empty path", "", true}, // Clean path becomes "."
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidLocalPath(tt.path)
			if result != tt.expected {
				t.Errorf("IsValidLocalPath(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestValidateFileType(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		ext         string
		shouldError bool
	}{
		{
			name:        "valid JPEG",
			data:        []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01},
			ext:         "jpg",
			shouldError: false,
		},
		{
			name:        "valid PNG",
			data:        []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52},
			ext:         "png",
			shouldError: false,
		},
		{
			name:        "valid GIF",
			data:        []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00, 0xFF, 0xFF, 0xFF},
			ext:         "gif",
			shouldError: false,
		},
		{
			name:        "valid WebP",
			data:        []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38, 0x20},
			ext:         "webp",
			shouldError: false,
		},
		{
			name:        "invalid - wrong magic bytes",
			data:        []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			ext:         "jpg",
			shouldError: true,
		},
		{
			name:        "file too small",
			data:        []byte{0xFF, 0xD8},
			ext:         "jpg",
			shouldError: true,
		},
		{
			name:        "unsupported extension",
			data:        []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01},
			ext:         "xyz",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileType(tt.data, tt.ext)
			if tt.shouldError && err == nil {
				t.Errorf("ValidateFileType() should return error for %s", tt.name)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("ValidateFileType() unexpected error for %s: %v", tt.name, err)
			}
		})
	}
}

func TestIsAllowedMimeType(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		expected bool
	}{
		{"valid JPEG", "image/jpeg", true},
		{"valid PNG", "image/png", true},
		{"valid GIF", "image/gif", true},
		{"valid WebP", "image/webp", true},
		{"valid BMP", "image/bmp", true},
		{"valid TIFF", "image/tiff", true},
		{"valid MP4", "video/mp4", true},
		{"valid WebM", "video/webm", true},
		{"invalid SVG", "image/svg+xml", false},
		{"invalid PDF", "application/pdf", false},
		{"invalid executable", "application/x-executable", false},
		{"invalid text", "text/plain", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAllowedMimeType(tt.mimeType)
			if result != tt.expected {
				t.Errorf("IsAllowedMimeType(%q) = %v, want %v", tt.mimeType, result, tt.expected)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple filename", "test.jpg", "test.jpg"},
		{"path with directory", "/path/to/file.png", "file.png"},
		{"windows path", "C:\\Users\\test\\image.jpg", "image.jpg"},
		{"filename with dangerous chars", "test$file`name.jpg", "testfilename.jpg"},
		{"path traversal attempt", "../../../etc/passwd", "passwd"},
		{"multiple dangerous chars", "file$(name)|test.jpg", "filenametest.jpg"},
		{"brackets and braces", "file[test]{name}.jpg", "filetestname.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func BenchmarkIsValidLocalPath(b *testing.B) {
	paths := []string{
		"images/test.jpg",
		"../../../etc/passwd",
		"/absolute/path",
		"folder/subfolder/image.png",
	}

	for i := 0; i < b.N; i++ {
		for _, path := range paths {
			IsValidLocalPath(path)
		}
	}
}

func BenchmarkValidateFileType(b *testing.B) {
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01}

	for i := 0; i < b.N; i++ {
		ValidateFileType(data, "jpg")
	}
}
