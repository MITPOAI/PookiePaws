// Package updatecheck checks GitHub releases for newer versions and surfaces
// a non-blocking notice on stderr. The package never installs anything.
package updatecheck

import (
	"strings"

	"golang.org/x/mod/semver"
)

// Normalize trims whitespace, ensures a leading "v", and returns "" if the
// result is not a valid semver string.
func Normalize(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return ""
	}
	return v
}

// IsNewer reports whether `latest` is strictly greater than `current` under
// semantic versioning. Invalid inputs always return false (fail-closed: never
// nag the user about a version we can't parse).
func IsNewer(current, latest string) bool {
	c, l := Normalize(current), Normalize(latest)
	if c == "" || l == "" {
		return false
	}
	return semver.Compare(l, c) > 0
}
