package app

import (
	"strings"

	"github.com/pragmabits/bumpit/internal/semver"
)

// globMetacharacters are the characters that end the literal head of a git
// tag pattern.
const globMetacharacters = `*?[\`

// tagPrefix is the text a tag pattern fixes before its first wildcard, which
// every tag the pattern matches starts with: "v" for "v*", "release-" for
// "release-*", "cmd/tool/v" for "cmd/tool/v*".
func tagPrefix(pattern string) string {
	if index := strings.IndexAny(pattern, globMetacharacters); index >= 0 {
		return pattern[:index]
	}
	return pattern
}

// versionAfterPrefix reads the semantic version after prefix. A tag carries a
// prefix and a version, and only the version is SemVer. A pattern that opens
// with a wildcard fixes no prefix, and then the customary "v" is accepted.
func versionAfterPrefix(value, prefix string) (semver.Version, error) {
	remainder := strings.TrimPrefix(strings.TrimSpace(value), prefix)
	if prefix == "" {
		remainder = strings.TrimPrefix(remainder, "v")
	}
	return semver.Parse(remainder)
}

// prefixOfTag is the prefix the next tag carries: the one the pattern fixes,
// or, when it fixes none, the "v" the current tag carries.
func prefixOfTag(currentTag, prefix string) string {
	if prefix == "" && strings.HasPrefix(currentTag, "v") {
		return "v"
	}
	return prefix
}
