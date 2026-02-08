package handlers

import (
	"regexp"
	"strings"
)

// Reserved names that cannot be used as link names
var reservedNames = map[string]bool{
	"api":      true,
	"admin":    true,
	"static":   true,
	"external": true,
	"data":     true,
	"health":   true,
}

// isValidLinkName validates link name format and checks against reserved names
func isValidLinkName(name string) bool {
	// Check length (1-255 characters)
	if len(name) < 1 || len(name) > 255 {
		return false
	}

	// Check for reserved names
	if reservedNames[strings.ToLower(name)] {
		return false
	}

	// Allow only alphanumeric, hyphen, underscore
	// Must start with alphanumeric
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`, name)
	return matched
}
