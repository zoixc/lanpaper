// SPDX-License-Identifier: MIT

package utils

import (
	"regexp"
	"strings"
)

// escapedAmpersand is the HTML escape of "&". It is what a URL copied out of a
// page source, a markdown file or a chat log carries instead of the separator
// itself. The panel normalizes the same artifact before sending it
// (static/js/feature-domain.js, normalizeMediaUrl); the API does it here as
// well, so a script that passes such a link gets the file it pointed at.
var escapedAmpersand = regexp.MustCompile(`(?i)&amp;`)

// NormalizeRemoteURL turns a user-supplied remote address into the address the
// server should actually request. Only the escape is rewritten, and that is
// not cosmetic for the image CDNs built on imgix (Unsplash, for one): they
// ignore parameters they do not recognize, so "?q=80&amp;w=1336&amp;fit=crop"
// silently drops q, w and fit and answers with the full-size original — a
// different file from the one the user picked, and often a much larger one.
func NormalizeRemoteURL(raw string) string {
	return escapedAmpersand.ReplaceAllLiteralString(strings.TrimSpace(raw), "&")
}
