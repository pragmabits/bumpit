package semver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
var preReleasePattern = regexp.MustCompile(`^[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*$`)

type BumpLevel int

const (
	BumpNone BumpLevel = iota
	BumpPatch
	BumpMinor
	BumpMajor
)

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

func MaxBump(left, right BumpLevel) BumpLevel {
	if left >= right {
		return left
	}
	return right
}

type Version struct {
	Major         int
	Minor         int
	Patch         int
	PreRelease    string
	BuildMetadata string
}

func Parse(input string) (Version, error) {
	matches := versionPattern.FindStringSubmatch(strings.TrimSpace(input))
	if matches == nil {
		return Version{}, fmt.Errorf("invalid semver tag %q", input)
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

func (v Version) Tag(prefix string) string {
	return prefix + v.String()
}

func (v Version) Next(level BumpLevel) Version {
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

func (v Version) Promote() Version {
	v.PreRelease = ""
	v.BuildMetadata = ""
	return v
}

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

func (v Version) HasPreRelease() bool {
	return v.PreRelease != ""
}

func (v Version) PreReleaseLabel() string {
	if v.PreRelease == "" {
		return ""
	}
	parts := strings.Split(v.PreRelease, ".")
	return parts[0]
}

func (v Version) NextPreRelease() (Version, error) {
	if v.PreRelease == "" {
		return Version{}, fmt.Errorf("version does not have a prerelease")
	}

	parts := strings.Split(v.PreRelease, ".")
	lastPart := parts[len(parts)-1]
	if nextNumber, err := strconv.Atoi(lastPart); err == nil {
		parts[len(parts)-1] = strconv.Itoa(nextNumber + 1)
		v.PreRelease = strings.Join(parts, ".")
		v.BuildMetadata = ""
		return v, nil
	}

	if len(parts) == 1 {
		v.PreRelease = parts[0] + ".1"
		v.BuildMetadata = ""
		return v, nil
	}

	v.BuildMetadata = ""
	return v, nil
}
