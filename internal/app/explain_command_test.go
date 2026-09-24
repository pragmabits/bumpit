package app

import (
	"strings"
	"testing"
)

func TestWriteExplanationListsModuleAndDependents(t *testing.T) {
	t.Parallel()

	plan := ReleasePlan{
		CurrentTag:  "v0.1.0",
		BaseTag:     "v0.1.0",
		NextTag:     "v0.2.0",
		HighestBump: "minor",
		Module:      "example.com/repository",
		Dependents:  []Dependent{{Directory: "tool", Path: "example.com/repository/tool", Requires: "v0.1.0"}},
	}

	var text strings.Builder
	if err := writeExplanation(&text, plan); err != nil {
		t.Fatalf("writeExplanation returned error: %v", err)
	}

	lines := []string{
		"Module: example.com/repository",
		"Dependents requiring an older version:",
		"- tool (example.com/repository/tool) requires v0.1.0",
	}
	for _, line := range lines {
		if !strings.Contains(text.String(), line+"\n") {
			t.Errorf("explanation lacks %q:\n%s", line, text.String())
		}
	}
}

func TestWriteExplanationListsBaseReleaseAndIgnoredTags(t *testing.T) {
	t.Parallel()

	plan := ReleasePlan{
		CurrentTag:  "v1.1.0-beta.2",
		BaseTag:     "v1.0.0",
		NextTag:     "v1.1.0-beta.3",
		HighestBump: "minor",
		IgnoredTags: []IgnoredTag{{Tag: "v01.02.03", Reason: "invalid semver"}},
	}

	var text strings.Builder
	if err := writeExplanation(&text, plan); err != nil {
		t.Fatalf("writeExplanation returned error: %v", err)
	}

	for _, line := range []string{"Base release: v1.0.0", "Ignored tags:", "- v01.02.03: invalid semver"} {
		if !strings.Contains(text.String(), line+"\n") {
			t.Errorf("explanation lacks %q:\n%s", line, text.String())
		}
	}
}
