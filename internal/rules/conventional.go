// Package rules reads a commit as a Conventional Commit and tells how much of
// a version it asks to increment.
package rules

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pragmabits/bumpit/internal/semver"
)

// headerPattern is a Conventional Commits header: a type, an optional scope,
// an optional breaking marker, then the required colon, space and description.
var headerPattern = regexp.MustCompile(`^([a-zA-Z]+)(\([^()\r\n]+\))?(!)?: \S`)

// footerPattern is a Conventional Commits footer, which follows the git
// trailer format: a token, where BREAKING CHANGE is the one token allowed to
// hold a space, followed by ": " or " #".
var footerPattern = regexp.MustCompile(`^(BREAKING CHANGE|[A-Za-z][A-Za-z0-9-]*)(?:: | #)`)

var paragraphSeparator = regexp.MustCompile(`\n[ \t]*\n`)

// Analysis is what one commit asks for: its type, the bump, and why.
type Analysis struct {
	Type     string           `json:"type,omitempty"`
	Level    semver.BumpLevel `json:"-"`
	Bump     string           `json:"bump"`
	Reason   string           `json:"reason"`
	Breaking bool             `json:"breaking"`
}

// Analyze reads the bump a commit asks for. Only a conventional commit asks
// for one: a breaking change, from a BREAKING CHANGE footer or the ! marker,
// is major; a feat is minor, and so is a Deprecated footer, since SemVer
// requires a minor release for a deprecation; fix, perf and refactor are patch.
func Analyze(subject, body string) Analysis {
	matches := headerPattern.FindStringSubmatch(strings.TrimSpace(subject))
	if matches == nil {
		return Analysis{
			Level:  semver.BumpNone,
			Bump:   semver.BumpNone.String(),
			Reason: "non-conventional commit",
		}
	}

	commitType := strings.ToLower(matches[1])
	tokens := footerTokens(body)
	if hasBreakingChange(tokens) {
		return breakingAnalysis(commitType, "breaking change footer")
	}
	if matches[3] == "!" {
		return breakingAnalysis(commitType, "breaking change marker in commit header")
	}

	level, reason := typeLevel(commitType)
	if hasDeprecation(tokens) && level < semver.BumpMinor {
		level, reason = semver.BumpMinor, "deprecation footer"
	}

	return Analysis{
		Type:   commitType,
		Level:  level,
		Bump:   level.String(),
		Reason: reason,
	}
}

func breakingAnalysis(commitType, reason string) Analysis {
	return Analysis{
		Type:     commitType,
		Level:    semver.BumpMajor,
		Bump:     semver.BumpMajor.String(),
		Reason:   reason,
		Breaking: true,
	}
}

func typeLevel(commitType string) (semver.BumpLevel, string) {
	reason := fmt.Sprintf("%s commit", commitType)
	switch commitType {
	case "feat":
		return semver.BumpMinor, reason
	case "fix", "perf", "refactor":
		return semver.BumpPatch, reason
	default:
		return semver.BumpNone, reason
	}
}

// footerTokens returns the tokens of the footers in body, read the way git
// reads trailers: the footers are the last paragraph, when it opens with one,
// and a line of it that is not a footer continues the value above it.
func footerTokens(body string) []string {
	paragraphs := paragraphSeparator.Split(strings.TrimSpace(body), -1)
	lines := strings.Split(paragraphs[len(paragraphs)-1], "\n")
	if !footerPattern.MatchString(lines[0]) {
		return nil
	}

	var tokens []string
	for _, line := range lines {
		if match := footerPattern.FindStringSubmatch(line); match != nil {
			tokens = append(tokens, match[1])
		}
	}
	return tokens
}

// hasBreakingChange reports a BREAKING CHANGE footer, or its synonym
// BREAKING-CHANGE. Unlike every other token, it must be uppercase.
func hasBreakingChange(tokens []string) bool {
	for _, token := range tokens {
		if token == "BREAKING CHANGE" || token == "BREAKING-CHANGE" {
			return true
		}
	}
	return false
}

// hasDeprecation reports a Deprecated footer, a token Conventional Commits
// reads in any case.
func hasDeprecation(tokens []string) bool {
	for _, token := range tokens {
		if strings.EqualFold(token, "Deprecated") {
			return true
		}
	}
	return false
}
