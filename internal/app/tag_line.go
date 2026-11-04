package app

import (
	"slices"
	"strings"

	"github.com/pragmabits/bumpit/internal/semver"
)

// TagLine is the version tags that share a prefix no module of the
// repository owns: the releases of a part of it that bumpit does not know.
type TagLine struct {
	Prefix string `json:"prefix"`
	Count  int    `json:"count"`
	// Newest is the tag of the highest version.
	Newest string `json:"newest"`
	// Candidates are the directories the tags may be the releases of.
	Candidates []Candidate `json:"candidates"`
}

// unownedTagLines groups by prefix the tags that are a prefix followed by a
// semantic version and that are no version of a module whose tag prefix
// prefixes lists, ordered by prefix.
func unownedTagLines(tags, prefixes []string) []TagLine {
	lines := map[string]*TagLine{}
	newest := map[string]semver.Version{}
	for _, tag := range tags {
		prefix, found := versionPrefix(tag)
		if !found || versionOfAnotherModule(tag, prefixes) {
			continue
		}
		version, _ := versionAfterPrefix(tag, prefix)
		line, seen := lines[prefix]
		if !seen {
			line = &TagLine{Prefix: prefix}
			lines[prefix] = line
		}
		line.Count++
		if !seen || semver.Compare(version, newest[prefix]) > 0 {
			line.Newest, newest[prefix] = tag, version
		}
	}

	ordered := make([]TagLine, 0, len(lines))
	for _, line := range lines {
		ordered = append(ordered, *line)
	}
	slices.SortFunc(ordered, func(left, right TagLine) int { return strings.Compare(left.Prefix, right.Prefix) })
	return ordered
}

// versionPrefix is the text of tag before its semantic version: the version
// starts at the first digit after a character that is no digit, or at the
// start, from which the rest of the tag is a version. release-10.0.0 has the
// prefix release-, not release-1. It reports false for a tag that holds no
// version.
func versionPrefix(tag string) (string, bool) {
	for index := range len(tag) {
		if !isDigit(tag[index]) || index > 0 && isDigit(tag[index-1]) {
			continue
		}
		if _, err := semver.Parse(tag[index:]); err == nil {
			return tag[:index], true
		}
	}
	return "", false
}

func isDigit(character byte) bool {
	return character >= '0' && character <= '9'
}

// guessDirectories lists the tracked directories whose last elements are
// the name a tag prefix gives, as homepage/v gives homepage: the directories
// its tags may be the releases of. A directory of a module, or one that
// holds fixtures or dependencies, is none. A prefix that gives no name
// guesses nothing.
func guessDirectories(prefix string, tracked trackedTree, modules []repositoryModule) []string {
	name := prefixName(prefix)
	if name == "" {
		return nil
	}

	var guesses []string
	for directory := range tracked.directories {
		matches := directory == name || strings.HasSuffix(directory, "/"+name)
		if matches && !isModuleDirectory(directory, modules) && !isFixtureDirectory(directory) {
			guesses = append(guesses, directory)
		}
	}
	slices.Sort(guesses)
	return guesses
}

// prefixName is the name a tag prefix gives: the prefix without the
// separators at its end, and without a v that ends it after a separator or
// alone, which starts the version. homepage/v and app-v give homepage and
// app, kv gives kv, and v- gives no name.
func prefixName(prefix string) string {
	const separators = "/-_@."
	name := strings.TrimRight(prefix, separators)
	if before, found := strings.CutSuffix(name, "v"); found && (before == "" || strings.ContainsAny(before[len(before)-1:], separators)) {
		name = strings.TrimRight(before, separators)
	}
	return name
}

func isModuleDirectory(directory string, modules []repositoryModule) bool {
	return slices.ContainsFunc(modules, func(module repositoryModule) bool { return module.directory == directory })
}

// isFixtureDirectory reports a directory that lies in a vendor, testdata or
// node_modules directory: fixtures and dependencies, which are no module of
// the repository.
func isFixtureDirectory(directory string) bool {
	return slices.ContainsFunc(strings.Split(directory, "/"), func(element string) bool {
		return element == "vendor" || element == "testdata" || element == "node_modules"
	})
}

// Candidate is a directory bumpit init proposes to declare as a module, and
// the command that declares it.
type Candidate struct {
	// Directory is relative to the repository init was given.
	Directory string `json:"directory"`
	// TagPrefix is the prefix its tags take, empty for the default one.
	TagPrefix string `json:"tag_prefix,omitempty"`
	// ShippedBy lists the Go modules whose //go:embed patterns reach it,
	// relative to the repository init was given.
	ShippedBy []string `json:"shipped_by"`
	// Embeds are those patterns.
	Embeds []EmbedPlace `json:"embeds,omitempty"`
	// Command is the bumpit modules add that declares it, empty when that
	// command would refuse it.
	Command string `json:"command,omitempty"`
	// Refused is why bumpit modules add would refuse it.
	Refused string `json:"refused,omitempty"`
	// topDirectory is Directory relative to the top of the repository.
	topDirectory string
}

// EmbedPlace is a //go:embed pattern and where it is written.
type EmbedPlace struct {
	// Place is the file, relative to the repository init was given, its
	// line and its column.
	Place   string `json:"place"`
	Pattern string `json:"pattern"`
}
