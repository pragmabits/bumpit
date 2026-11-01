package app

import (
	"bytes"
	"regexp"
	"testing"
)

// hashPattern matches the short commit hashes of a JSON payload, which change
// with every run of the test.
var hashPattern = regexp.MustCompile(`"hash": "[0-9a-f]+"`)

// TestJSONOfGoModulesStaysTheSame pins the whole JSON that next, explain and
// modules print in a repository of Go modules alone, so that a field added
// for modules declared in bumpit.yaml cannot leak into it.
func TestJSONOfGoModulesStaysTheSame(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitFile(t, repositoryPath, "tool/main.txt", "main", "feat(tool): add a flag")
	commitFile(t, repositoryPath, "library.txt", "library", "fix: handle empty input")

	rootPlan, err := BuildReleasePlan(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}
	toolPlan, err := BuildReleasePlan(Options{Repository: repositoryPath, Module: "tool"})
	if err != nil {
		t.Fatalf("BuildReleasePlan for the tool returned error: %v", err)
	}
	releases, err := ListModules(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("ListModules returned error: %v", err)
	}

	payloads := []struct {
		name  string
		value any
		want  string
	}{
		{name: "root plan", value: rootPlan, want: goldenRootPlan},
		{name: "tool plan", value: toolPlan, want: goldenToolPlan},
		{name: "modules", value: releases, want: goldenModules},
	}
	for _, payload := range payloads {
		var written bytes.Buffer
		if err := writeJSON(&written, payload.value); err != nil {
			t.Fatalf("writeJSON returned error: %v", err)
		}
		got := hashPattern.ReplaceAllString(written.String(), `"hash": "<hash>"`)
		if got != payload.want {
			t.Errorf("JSON of the %s changed:\n%s", payload.name, got)
		}
	}
}

const goldenRootPlan = `{
  "current_tag": "v0.1.0",
  "current_version": "0.1.0",
  "base_tag": "v0.1.0",
  "next_tag": "v0.1.1",
  "next_version": "0.1.1",
  "has_release": true,
  "highest_bump": "patch",
  "commit_count": 1,
  "relevant_commit_count": 1,
  "commits": [
    {
      "hash": "<hash>",
      "subject": "fix: handle empty input",
      "type": "fix",
      "bump": "patch",
      "reason": "fix commit",
      "breaking": false
    }
  ],
  "module": "example.com/repository",
  "dependents": [
    {
      "directory": "tool",
      "path": "example.com/repository/tool",
      "requires": "v0.1.0"
    }
  ]
}
`

const goldenToolPlan = `{
  "current_tag": "tool/v0.1.0",
  "current_version": "0.1.0",
  "base_tag": "tool/v0.1.0",
  "next_tag": "tool/v0.2.0",
  "next_version": "0.2.0",
  "has_release": true,
  "highest_bump": "minor",
  "commit_count": 1,
  "relevant_commit_count": 1,
  "commits": [
    {
      "hash": "<hash>",
      "subject": "feat(tool): add a flag",
      "type": "feat",
      "bump": "minor",
      "reason": "feat commit",
      "breaking": false
    }
  ],
  "module": "example.com/repository/tool"
}
`

const goldenModules = `[
  {
    "directory": ".",
    "path": "example.com/repository",
    "current_tag": "v0.1.0",
    "next_tag": "v0.1.1",
    "has_release": true
  },
  {
    "directory": "tool",
    "path": "example.com/repository/tool",
    "current_tag": "tool/v0.1.0",
    "next_tag": "tool/v0.2.0",
    "has_release": true,
    "hold": {
      "requirements": [
        {
          "directory": ".",
          "path": "example.com/repository",
          "requires": "v0.1.0",
          "newest": "v0.1.1"
        }
      ]
    }
  }
]
`
