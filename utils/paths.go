package utils

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateAndResolvePath ensures targetPath is within baseDir, resolving
// symlinks to prevent escapes. Returns absolute and real paths, or an error.
func ValidateAndResolvePath(baseDir, targetPath string) (absPath, realPath string, err error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", "", fmt.Errorf("resolving base dir: %w", err)
	}
	absPath, err = filepath.Abs(filepath.Join(absBase, filepath.Clean(targetPath)))
	if err != nil {
		return "", "", fmt.Errorf("resolving target path: %w", err)
	}
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return "", "", fmt.Errorf("path traversal detected: %s escapes %s", targetPath, baseDir)
	}
	realPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", "", fmt.Errorf("resolving symlinks: %w", err)
	}
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		// Base doesn't exist yet — absPath containment already checked above.
		return absPath, "", nil
	}
	if !strings.HasPrefix(realPath, realBase+string(filepath.Separator)) && realPath != realBase {
		return "", "", fmt.Errorf("symlink escape detected: %s -> %s escapes %s", absPath, realPath, baseDir)
	}
	return absPath, realPath, nil
}

// MustBeInDirectory is a convenience wrapper around ValidateAndResolvePath
// that returns only the security error.
func MustBeInDirectory(baseDir, targetPath string) error {
	_, _, err := ValidateAndResolvePath(baseDir, targetPath)
	return err
}
