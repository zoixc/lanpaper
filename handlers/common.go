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

// linkNameRe is compiled once at startup to avoid per-call overhead
var linkNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// isValidLinkName validates link name format and checks against reserved names
func isValidLinkName(name string) bool {
	if len(name) < 1 || len(name) > 255 {
		return false
	}
	if reservedNames[strings.ToLower(name)] {
		return false
	}
	return linkNameRe.MatchString(name)
}
