package handlers

import "regexp"

var idRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func isValidLinkName(name string) bool {
	return idRe.MatchString(name)
}
