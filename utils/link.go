// SPDX-License-Identifier: MIT

package utils

import (
	"regexp"
	"strings"
)

// Link names are used in URL routes AND as on-disk filenames. Apply the same
// validation when loading persisted metadata as when handling API requests.
var linkNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

var reservedLinkNames = map[string]bool{
	"api": true, "admin": true, "static": true,
	"external": true, "data": true, "health": true,
	"sw.js": true, "favicon.ico": true, "robots.txt": true, "sitemap.xml": true,
	"manifest.json": true, "manifest.webmanifest": true,
}

func IsValidLinkName(name string) bool {
	return len(name) >= 1 && len(name) <= 64 &&
		!reservedLinkNames[strings.ToLower(name)] && linkNameRE.MatchString(name)
}
