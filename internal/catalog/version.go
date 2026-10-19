package catalog

import (
	"cmp"
	"regexp"
	"strconv"
	"strings"
)

// version matches date-based catalog versions: YYYY.MM, or YYYY.MM.N for a
// further release in the same month. Same pattern as catalog_version in
// internal/spec.
var version = regexp.MustCompile(`^[0-9]{4}\.(0[1-9]|1[0-2])(\.[1-9][0-9]*)?$`)

// ValidVersion reports whether v is a catalog version such as "2026.10" or
// "2026.10.1".
func ValidVersion(v string) bool {
	return version.MatchString(v)
}

// CompareVersions returns -1, 0 or +1 as catalog version a is older than,
// equal to or newer than b. "2026.10" equals "2026.10.0", so a release
// without a number counts as the month's first. Both must be valid
// versions; for other strings the result is meaningless, but it never panics.
func CompareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := range pa {
		if c := cmp.Compare(pa[i], pb[i]); c != 0 {
			return c
		}
	}
	return 0
}

// versionParts splits a version into year, month and release number.
func versionParts(v string) [3]int {
	var parts [3]int
	for i, s := range strings.SplitN(v, ".", 3) {
		parts[i], _ = strconv.Atoi(s)
	}
	return parts
}
