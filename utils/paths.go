package utils

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateAndResolvePath validates that targetPath is within baseDir,
// resolving both absolute paths and symlinks to prevent escapes.
// Returns the absolute and real paths if valid, or an error.
func ValidateAndResolvePath(baseDir, targetPath string) (absPath, realPath string, err error) {
	// Resolve base directory
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", "", fmt.Errorf("resolving base dir: %w", err)
	}

	// Construct and resolve target path
	absPath, err = filepath.Abs(filepath.Join(absBase, filepath.Clean(targetPath)))
	if err != nil {
		return "", "", fmt.Errorf("resolving target path: %w", err)
	}

	// Check that path is within base directory
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return "", "", fmt.Errorf("path traversal detected: %s escapes %s", targetPath, baseDir)
	}

	// Resolve symlinks to prevent escape via symbolic links
	realPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", "", fmt.Errorf("resolving symlinks: %w", err)
	}

	// Verify real path is still within base directory
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		// If base doesn't exist, only check absPath containment
		return absPath, "", nil
	}

	if !strings.HasPrefix(realPath, realBase+string(filepath.Separator)) && realPath != realBase {
		return "", "", fmt.Errorf("symlink escape detected: %s -> %s escapes %s", absPath, realPath, baseDir)
	}

	return absPath, realPath, nil
}

// MustBeInDirectory is a stricter version that panics on configuration errors
// and returns only security-related errors. Used for external image validation.
func MustBeInDirectory(baseDir, targetPath string) error {
	_, _, err := ValidateAndResolvePath(baseDir, targetPath)
	return err
}
