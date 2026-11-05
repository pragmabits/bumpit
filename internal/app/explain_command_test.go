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

func TestWriteExplanationDescribesDeclaredModules(t *testing.T) {
	t.Parallel()

	declared := ReleasePlan{
		NextTag:      "homepage/v0.1.1",
		HighestBump:  "patch",
		Name:         "web/homepage",
		Directories:  []string{"web/homepage"},
		Paths:        []string{"deploy/homepage"},
		DerivedPaths: []string{"web/brand", "web/package.json"},
		ShippedBy:    []string{"."},
		Analyses:     []CommitDecision{{Hash: "abc1234", Subject: "fix: sharpen the logo", Bump: "patch", Reason: "fix commit", Via: "web/brand"}},
	}
	shipper := ReleasePlan{
		NextTag:     "v0.2.0",
		HighestBump: "minor",
		Module:      "example.com/repository",
		GoFilesIn:   []string{"docs"},
		Ships:       []string{"web/homepage"},
		LeavesOut:   []string{"docs"},
	}

	namedPlan := ReleasePlan{
		NextTag:     "homepage/v0.1.1",
		HighestBump: "patch",
		Name:        "homepage",
		Directories: []string{"web/homepage", "deploy/homepage"},
	}
	namedGoPlan := ReleasePlan{
		NextTag:     "tool/v0.2.0",
		HighestBump: "minor",
		Module:      "example.com/repository/tool",
		Name:        "stack",
	}

	cases := []struct {
		plan  ReleasePlan
		lines []string
	}{
		{plan: namedPlan, lines: []string{"Module: homepage (declared in bumpit.yaml)\nDirectories: web/homepage, deploy/homepage\nCurrent tag:"}},
		{plan: namedGoPlan, lines: []string{"Module: example.com/repository/tool\nName: stack\nCurrent tag:"}},
		{plan: declared, lines: []string{
			"Module: web/homepage (declared in bumpit.yaml)\nPaths: deploy/homepage\nPaths from the Node packages: web/brand, web/package.json\nShipped by: .\nCurrent tag:",
			"- abc1234 fix: sharpen the logo [patch: fix commit] via web/brand\n",
		}},
		{plan: shipper, lines: []string{"Module: example.com/repository\nReads the Go files in: docs\nShips: web/homepage\nLeaves out: docs\nCurrent tag:"}},
	}
	for _, testCase := range cases {
		var text strings.Builder
		if err := writeExplanation(&text, testCase.plan); err != nil {
			t.Fatalf("writeExplanation returned error: %v", err)
		}
		for _, line := range testCase.lines {
			if !strings.Contains(text.String(), line) {
				t.Errorf("explanation lacks %q:\n%s", line, text.String())
			}
		}
	}
}
