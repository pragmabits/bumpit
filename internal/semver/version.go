// Package semver parses, orders and increments versions as Semantic
// Versioning 2.0.0 defines them. A version here carries no prefix: "v1.2.3"
// is a tag, not a semantic version.
package semver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// versionPattern is the regular expression semver.org suggests for a version:
// no leading zeroes in major, minor, patch or a numeric prerelease identifier,
// no empty identifier, and no prefix, since "v1.2.3" is not a semantic version.
// The groups are major, minor, patch, prerelease and build metadata.
var versionPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// preReleasePattern is the prerelease part of versionPattern on its own.
var preReleasePattern = regexp.MustCompile(`^(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*$`)

// BumpLevel is how much of a version a change asks to increment.
type BumpLevel int

// The levels, in increasing order, which MaxBump relies on.
const (
	BumpNone BumpLevel = iota
	BumpPatch
	BumpMinor
	BumpMajor
)

// String names the level: none, patch, minor or major.
func (b BumpLevel) String() string {
	switch b {
	case BumpPatch:
		return "patch"
	case BumpMinor:
		return "minor"
	case BumpMajor:
		return "major"
	default:
		return "none"
	}
}

// MaxBump returns the higher of two levels.
func MaxBump(left, right BumpLevel) BumpLevel {
	if left >= right {
		return left
	}
	return right
}

// Version is a semantic version. The zero Version is 0.0.0.
type Version struct {
	Major         int
	Minor         int
	Patch         int
	PreRelease    string
	BuildMetadata string
}

// Compare orders two versions by SemVer precedence, returning a negative
// number when left is older, zero when both rank the same and a positive
// number when left is newer. Build metadata is ignored, as the spec requires.
func Compare(left, right Version) int {
	if result := compareNumbers(left.Major, right.Major); result != 0 {
		return result
	}
	if result := compareNumbers(left.Minor, right.Minor); result != 0 {
		return result
	}
	if result := compareNumbers(left.Patch, right.Patch); result != 0 {
		return result
	}
	return comparePreRelease(left.PreRelease, right.PreRelease)
}

func comparePreRelease(left, right string) int {
	if left == right {
		return 0
	}
	if left == "" {
		return 1
	}
	if right == "" {
		return -1
	}

	leftIdentifiers := strings.Split(left, ".")
	rightIdentifiers := strings.Split(right, ".")
	for index := 0; index < len(leftIdentifiers) && index < len(rightIdentifiers); index++ {
		if result := comparePreReleaseIdentifier(leftIdentifiers[index], rightIdentifiers[index]); result != 0 {
			return result
		}
	}

	return compareNumbers(len(leftIdentifiers), len(rightIdentifiers))
}

// comparePreReleaseIdentifier orders two prerelease identifiers as the spec
// does: numeric ones numerically, alphanumeric ones in ASCII order, and a
// numeric one below an alphanumeric one. Which kind an identifier is comes
// from the grammar, digits only, so "-1" is alphanumeric.
func comparePreReleaseIdentifier(left, right string) int {
	leftNumeric := isNumericIdentifier(left)
	rightNumeric := isNumericIdentifier(right)

	switch {
	case leftNumeric && rightNumeric:
		return compareNumericIdentifiers(left, right)
	case leftNumeric:
		return -1
	case rightNumeric:
		return 1
	default:
		return strings.Compare(left, right)
	}
}

func isNumericIdentifier(identifier string) bool {
	if identifier == "" {
		return false
	}
	for index := 0; index < len(identifier); index++ {
		if identifier[index] < '0' || identifier[index] > '9' {
			return false
		}
	}
	return true
}

// compareNumericIdentifiers compares two numeric identifiers of any size
// without converting them: with no leading zeroes, the longer is the larger.
func compareNumericIdentifiers(left, right string) int {
	if result := compareNumbers(len(left), len(right)); result != 0 {
		return result
	}
	return strings.Compare(left, right)
}

func compareNumbers(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

// Parse reads a semantic version as the SemVer grammar defines it, surrounding
// space aside. Anything else is an error, including a prefix, as in "v1.2.3",
// and leading zeroes, as in "01.2.3" or "1.2.3-rc.01".
func Parse(input string) (Version, error) {
	matches := versionPattern.FindStringSubmatch(strings.TrimSpace(input))
	if matches == nil {
		return Version{}, fmt.Errorf("invalid semantic version %q", input)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return Version{}, fmt.Errorf("parse major: %w", err)
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return Version{}, fmt.Errorf("parse minor: %w", err)
	}
	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return Version{}, fmt.Errorf("parse patch: %w", err)
	}

	return Version{
		Major:         major,
		Minor:         minor,
		Patch:         patch,
		PreRelease:    matches[4],
		BuildMetadata: matches[5],
	}, nil
}

// String writes the version in SemVer form, without a prefix.
func (v Version) String() string {
	base := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.PreRelease != "" {
		base += "-" + v.PreRelease
	}
	if v.BuildMetadata != "" {
		base += "+" + v.BuildMetadata
	}
	return base
}

// Tag is the name of the tag that carries the version: prefix, then the
// version.
func (v Version) Tag(prefix string) string {
	return prefix + v.String()
}

// Next returns the normal version a change of the given level releases. In
// major version zero a breaking change bumps the minor version: anything may
// change in 0.y.z, and 1.0.0 is the release that declares the public API
// stable, which a commit cannot decide.
func (v Version) Next(level BumpLevel) Version {
	if level == BumpMajor && v.Major == 0 {
		level = BumpMinor
	}

	switch level {
	case BumpMajor:
		return Version{Major: v.Major + 1}
	case BumpMinor:
		return Version{Major: v.Major, Minor: v.Minor + 1}
	case BumpPatch:
		return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}
	default:
		return v
	}
}

// Promote returns the normal version: the version without its prerelease and
// build metadata. For a prerelease, that is the release it precedes.
func (v Version) Promote() Version {
	v.PreRelease = ""
	v.BuildMetadata = ""
	return v
}

// WithPreRelease returns the version with preRelease as its prerelease and no
// build metadata. The prerelease must follow the SemVer grammar; a leading
// hyphen is dropped.
func (v Version) WithPreRelease(preRelease string) (Version, error) {
	preRelease = strings.TrimPrefix(strings.TrimSpace(preRelease), "-")
	if preRelease == "" {
		return Version{}, fmt.Errorf("prerelease cannot be empty")
	}
	if !preReleasePattern.MatchString(preRelease) {
		return Version{}, fmt.Errorf("invalid prerelease %q", preRelease)
	}

	v.PreRelease = preRelease
	v.BuildMetadata = ""
	return v, nil
}

// HasPreRelease reports whether the version is a prerelease.
func (v Version) HasPreRelease() bool {
	return v.PreRelease != ""
}

// PreReleaseLabel is the first identifier of the prerelease: "beta" for
// 1.0.0-beta.2, and empty for a normal version.
func (v Version) PreReleaseLabel() string {
	if v.PreRelease == "" {
		return ""
	}
	parts := strings.Split(v.PreRelease, ".")
	return parts[0]
}

// NextPreRelease returns the next prerelease of the same normal version: the
// last identifier plus one when it is numeric, and ".1" appended otherwise, so
// 1.0.0-beta.2 becomes 1.0.0-beta.3 and 1.0.0-rc.beta becomes 1.0.0-rc.beta.1.
// Either way the result has higher precedence.
func (v Version) NextPreRelease() (Version, error) {
	if v.PreRelease == "" {
		return Version{}, fmt.Errorf("version does not have a prerelease")
	}

	identifiers := strings.Split(v.PreRelease, ".")
	lastIndex := len(identifiers) - 1
	if isNumericIdentifier(identifiers[lastIndex]) {
		number, err := strconv.Atoi(identifiers[lastIndex])
		if err != nil {
			return Version{}, fmt.Errorf("increment prerelease %q: %w", v.PreRelease, err)
		}
		identifiers[lastIndex] = strconv.Itoa(number + 1)
	} else {
		identifiers = append(identifiers, "1")
	}

	v.PreRelease = strings.Join(identifiers, ".")
	v.BuildMetadata = ""
	return v, nil
}
