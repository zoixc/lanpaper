package handlers

import (
	"regexp"
	"strings"
)

// reservedNames cannot be used as link names — they clash with existing routes.
var reservedNames = map[string]bool{
	"api": true, "admin": true, "static": true,
	"external": true, "data": true, "health": true,
}

var linkNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func isValidLinkName(name string) bool {
	return len(name) >= 1 && len(name) <= 255 &&
		!reservedNames[strings.ToLower(name)] &&
		linkNameRe.MatchString(name)
}
