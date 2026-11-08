package app

import (
	"strings"

	"github.com/pragmabits/bumpit/internal/semver"
)

// globMetacharacters are the characters that open a wildcard in a git glob,
// as a tag pattern or a pathspec is: the literal head of a pattern ends at the
// first one.
const globMetacharacters = `*?[\`

// firstTagName is the name of the first tag prefix makes, of version 0.0.0: git
// takes every other one the prefix makes when it takes that one, since what
// follows the prefix is a version.
func firstTagName(prefix string) string {
	return prefix + "0.0.0"
}

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

// versionOfAnotherModule reports whether tag is a version of a module whose
// prefix prefixes lists: the prefix followed by a semantic version. Such a tag
// matches the pattern of the module being read only because that prefix
// starts with its own, and it belongs to the module with the more precise
// prefix, as a file belongs to the module with the deepest directory.
func versionOfAnotherModule(tag string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if !strings.HasPrefix(tag, prefix) {
			continue
		}
		if _, err := versionAfterPrefix(tag, prefix); err == nil {
			return true
		}
	}
	return false
}

// prefixOfTag is the prefix the next tag carries: the one the pattern fixes,
// or, when it fixes none, the "v" the current tag carries.
func prefixOfTag(currentTag, prefix string) string {
	if prefix == "" && strings.HasPrefix(currentTag, "v") {
		return "v"
	}
	return prefix
}
