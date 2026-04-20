package rules

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pragmabits/bumpit/internal/semver"
)

var headerPattern = regexp.MustCompile(`^([a-zA-Z]+)(\([^)]+\))?(!)?:`)

type Analysis struct {
	Type     string           `json:"type,omitempty"`
	Level    semver.BumpLevel `json:"-"`
	Bump     string           `json:"bump"`
	Reason   string           `json:"reason"`
	Breaking bool             `json:"breaking"`
}

func Analyze(subject, body string) Analysis {
	body = strings.TrimSpace(body)
	subject = strings.TrimSpace(subject)

	if hasBreakingChange(body) {
		return Analysis{
			Level:    semver.BumpMajor,
			Bump:     semver.BumpMajor.String(),
			Reason:   "breaking change footer",
			Breaking: true,
		}
	}

	matches := headerPattern.FindStringSubmatch(subject)
	if matches == nil {
		return Analysis{
			Level:  semver.BumpNone,
			Bump:   semver.BumpNone.String(),
			Reason: "non-conventional commit",
		}
	}

	commitType := strings.ToLower(matches[1])
	if matches[3] == "!" {
		return Analysis{
			Type:     commitType,
			Level:    semver.BumpMajor,
			Bump:     semver.BumpMajor.String(),
			Reason:   "breaking change marker in commit header",
			Breaking: true,
		}
	}

	level := semver.BumpNone
	switch commitType {
	case "feat":
		level = semver.BumpMinor
	case "fix", "perf", "refactor":
		level = semver.BumpPatch
	}

	return Analysis{
		Type:   commitType,
		Level:  level,
		Bump:   level.String(),
		Reason: fmt.Sprintf("%s commit", commitType),
	}
}

func hasBreakingChange(body string) bool {
	return strings.Contains(body, "BREAKING CHANGE:") || strings.Contains(body, "BREAKING-CHANGE:")
}
