package utils

import "lanpaper/config"

// ExternalBaseDir returns the configured external image directory path.
// This eliminates code duplication across handlers.
func ExternalBaseDir() string {
	if d := config.Current.ExternalImageDir; d != "" {
		return d
	}
	return "external/images"
}
