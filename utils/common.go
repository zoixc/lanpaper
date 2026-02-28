package utils

import "lanpaper/config"

// ExternalBaseDir returns the configured external image directory.
// config.Load() always sets a non-empty default, so no fallback is needed.
func ExternalBaseDir() string {
	return config.Current.ExternalImageDir
}
