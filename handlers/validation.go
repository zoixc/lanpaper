package handlers

import (
	"regexp"
	"strings"
)

// reservedNames contains route prefixes and system paths that cannot be used as link names
// to prevent conflicts with application routing and functionality.
var reservedNames = map[string]bool{
	"admin":    true,
	"api":      true,
	"static":   true,
	"health":   true,
	"external": true,
	"data":     true,
	"images":   true,
	"previews": true,
	"upload":   true,
}

// linkNamePattern allows alphanumeric characters, hyphens, and underscores.
// Length: 1-64 characters.
var linkNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// isValidLinkName validates that a link name:
// 1. Matches the allowed pattern (alphanumeric, -, _)
// 2. Is not a reserved system name
// 3. Does not contain path traversal sequences
// 4. Is not empty or excessively long
func isValidLinkName(name string) bool {
	if name == "" {
		return false
	}
	
	// Check for reserved names (case-insensitive)
	if reservedNames[strings.ToLower(name)] {
		return false
	}
	
	// Check pattern (alphanumeric + hyphen + underscore, 1-64 chars)
	if !linkNamePattern.MatchString(name) {
		return false
	}
	
	// Additional security checks
	if strings.Contains(name, "..") {
		return false
	}
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return false
	}
	
	return true
}
