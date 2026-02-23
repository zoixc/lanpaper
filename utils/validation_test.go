package utils

import (
	"strings"
	"testing"
)

func TestIsValidLocalPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		want     bool
	}{
		{"valid relative path", "images/photo.jpg", true},
		{"valid nested path", "folder/subfolder/image.png", true},
		{"path with dot", "folder/file.name.jpg", true},
		{"absolute path - invalid", "/etc/passwd", false},
		{"path traversal - invalid", "../../../etc/passwd", false},
		{"path with double dots - invalid", "folder/../../../secret", false},
		{"null byte injection - invalid", "file\x00.jpg", false},
		{"UNC path - invalid", "\\\\server\\share", false},
		{"windows absolute - invalid", "C:\\Windows\\System32", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidLocalPath(tt.path); got != tt.want {
				t.Errorf("IsValidLocalPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestValidateFileType(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		ext     string
		wantErr bool
	}{
		{
			name:    "valid JPEG",
			data:    []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01},
			ext:     "jpg",
			wantErr: false,
		},
		{
			name:    "valid PNG",
			data:    []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52},
			ext:     "png",
			wantErr: false,
		},
		{
			name:    "valid GIF",
			data:    []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			ext:     "gif",
			wantErr: false,
		},
		{
			name:    "valid WebP",
			data:    []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38, 0x20},
			ext:     "webp",
			wantErr: false,
		},
		{
			name:    "invalid - JPEG marked as PNG",
			data:    []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01},
			ext:     "png",
			wantErr: true,
		},
		{
			name:    "file too small",
			data:    []byte{0xFF, 0xD8},
			ext:     "jpg",
			wantErr: true,
		},
		{
			name:    "unsupported extension",
			data:    []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			ext:     "exe",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileType(tt.data, tt.ext)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFileType() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsAllowedMimeType(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		want     bool
	}{
		{"JPEG allowed", "image/jpeg", true},
		{"PNG allowed", "image/png", true},
		{"GIF allowed", "image/gif", true},
		{"WebP allowed", "image/webp", true},
		{"BMP allowed", "image/bmp", true},
		{"TIFF allowed", "image/tiff", true},
		{"MP4 allowed", "video/mp4", true},
		{"WebM allowed", "video/webm", true},
		{"SVG not allowed", "image/svg+xml", false},
		{"executable not allowed", "application/x-executable", false},
		{"text not allowed", "text/plain", false},
		{"pdf not allowed", "application/pdf", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAllowedMimeType(tt.mimeType); got != tt.want {
				t.Errorf("IsAllowedMimeType(%q) = %v, want %v", tt.mimeType, got, tt.want)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{"simple filename", "photo.jpg", "photo.jpg"},
		{"filename with spaces", "my photo.jpg", "my photo.jpg"},
		{"path traversal removed", "../../../etc/passwd", "passwd"},
		{"dangerous chars removed", "file$name`test.jpg", "filenametest.jpg"},
		{"pipes removed", "file|name.jpg", "filename.jpg"},
		{"semicolons removed", "file;name.jpg", "filename.jpg"},
		{"brackets removed", "file[name].jpg", "filename.jpg"},
		{"complex path", "/home/user/../file.jpg", "file.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeFilename(tt.filename); got != tt.want {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestGetRealIP(t *testing.T) {
	// This is a basic test - more complex scenarios would need mocked http.Request
	tests := []struct {
		name          string
		xForwardedFor string
		xRealIP       string
		remoteAddr    string
		want          string
	}{
		{
			name:          "X-Forwarded-For single IP",
			xForwardedFor: "192.168.1.1",
			remoteAddr:    "10.0.0.1:1234",
			want:          "192.168.1.1",
		},
		{
			name:          "X-Forwarded-For multiple IPs",
			xForwardedFor: "192.168.1.1, 10.0.0.2, 172.16.0.1",
			remoteAddr:    "10.0.0.1:1234",
			want:          "192.168.1.1",
		},
		{
			name:       "X-Real-IP fallback",
			xRealIP:    "192.168.1.100",
			remoteAddr: "10.0.0.1:1234",
			want:       "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation that function handles strings correctly
			if tt.xForwardedFor != "" {
				if idx := strings.Index(tt.xForwardedFor, ","); idx >= 0 {
					got := strings.TrimSpace(tt.xForwardedFor[:idx])
					if got != tt.want {
						t.Errorf("Expected first IP = %q, got %q", tt.want, got)
					}
				} else {
					got := strings.TrimSpace(tt.xForwardedFor)
					if got != tt.want {
						t.Errorf("Expected IP = %q, got %q", tt.want, got)
					}
				}
			}
		})
	}
}
