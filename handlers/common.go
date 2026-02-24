package handlers

import (
	"regexp"
	"strings"
)

// reservedNames cannot be used as link names because they clash with URL routes.
var reservedNames = map[string]bool{
	"api": true, "admin": true, "static": true,
	"external": true, "data": true, "health": true,
}

// linkNameRe is compiled once at startup.
var linkNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// isValidLinkName checks format and reserved-name constraints.
func isValidLinkName(name string) bool {
	return len(name) >= 1 && len(name) <= 255 &&
		!reservedNames[strings.ToLower(name)] &&
		linkNameRe.MatchString(name)
}
