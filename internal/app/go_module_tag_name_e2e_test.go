package app

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

const noTagCarriesIt = "does not make a valid tag name"

// TestCLIEndToEndAGoModuleNoTagCanCarry covers a Go module in a directory git takes
// in no tag name: a plan of the module stops, rather than write a tag cut at the
// first glob character, read the tags that pattern matches or give git a pattern
// that starts with -, which it reads as an option, and the other modules go on.
func TestCLIEndToEndAGoModuleNoTagCanCarry(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	withModule := func(t *testing.T, directory string) string {
		t.Helper()
		repositoryPath := initGoRepository(t)
		commitGoMod(t, repositoryPath, directory, "example.com/repository/gen", "feat: add the generator")
		return repositoryPath
	}

	for _, directory := range []string{"gen[v2]", "gen v2", "gen.lock", "-gen"} {
		t.Run(directory, func(t *testing.T) {
			t.Parallel()
			repositoryPath := withModule(t, directory)

			for _, command := range []string{"next", "explain", "latest", "tag"} {
				output, err := runCLIIn(repositoryPath, binaryPath, command, "--module="+directory)
				if err == nil || !strings.Contains(output, noTagCarriesIt) {
					t.Errorf("%s returned %v\n%s", command, err, output)
				}
			}
			expectNoNewTag(t, repositoryPath)

			output, err := runCLIIn(repositoryPath, binaryPath, "tag", "--all-modules")
			if err == nil || !strings.Contains(output, "no tag created") || !strings.Contains(output, noTagCarriesIt) {
				t.Errorf("tag --all-modules returned %v\n%s", err, output)
			}
			expectNoNewTag(t, repositoryPath)

			output, err = runCLIIn(repositoryPath, binaryPath, "modules")
			if err != nil || !strings.Contains(output, noTagCarriesIt) {
				t.Errorf("modules returned %v\n%s", err, output)
			}

			output, err = runCLIIn(repositoryPath, binaryPath, "next")
			if err != nil || strings.Contains(output, noTagCarriesIt) {
				t.Errorf("next of the root module returned %v\n%s", err, output)
			}
			output, err = runCLIIn(repositoryPath, binaryPath, "latest", "-g", "tool")
			if err != nil || !strings.Contains(output, "tool/v0.1.0") {
				t.Errorf("latest of another module returned %v\n%s", err, output)
			}
		})
	}

	t.Run("the output of the plan as JSON", func(t *testing.T) {
		t.Parallel()
		repositoryPath := withModule(t, "gen[v2]")

		output, err := runCLIIn(repositoryPath, binaryPath, "modules", "-o", "json")
		var releases []ModuleRelease
		if decodeErr := json.Unmarshal([]byte(output), &releases); err != nil || decodeErr != nil {
			t.Fatalf("modules returned %v, %v\n%s", err, decodeErr, output)
		}
		for _, release := range releases {
			if release.Directory == "gen[v2]" && (release.NextTag != "" || release.HasRelease || !strings.Contains(release.Error, noTagCarriesIt)) {
				t.Errorf("release = %+v", release)
			}
		}
	})

	t.Run("the check", func(t *testing.T) {
		t.Parallel()
		repositoryPath := withModule(t, "gen v2")
		writeFile(t, repositoryPath+"/bumpit.yaml", "modules:\n  - directories: [.]\n")

		report, passed := readCheck(t, binaryPath, repositoryPath)
		item := itemOf(t, report.Blocking, ruleInvalidTagName)
		if passed || item.Subject != "gen v2" || item.Command != "" || !strings.Contains(item.How, "rename its directory") {
			t.Errorf("passed %v, item %+v", passed, item)
		}
		if strings.Contains(item.How, "another prefix") {
			t.Errorf("item = %+v, asks for a key a Go module does not take", item)
		}
		if slices.ContainsFunc(report.Review, func(item CheckItem) bool { return item.Subject == "gen v2" }) {
			t.Errorf("review = %+v, proposes to list a module whose directory is to be renamed", report.Review)
		}
	})

	for _, directory := range []string{"gen v2", "-gen"} {
		t.Run("init beside "+directory, func(t *testing.T) {
			t.Parallel()
			repositoryPath := withModule(t, directory)

			output, err := runCLIIn(repositoryPath, binaryPath, "init", "-o", "json")
			var report InitReport
			if decodeErr := json.Unmarshal([]byte(output), &report); err != nil || decodeErr != nil {
				t.Fatalf("init returned %v, %v\n%s", err, decodeErr, output)
			}
			if !slices.Equal(report.Declared, []string{".", "tool"}) || len(report.Untaggable) != 1 || report.Untaggable[0].Directory != directory || !strings.Contains(report.Untaggable[0].Reason, noTagCarriesIt) {
				t.Errorf("report = %+v, want the module left out of the file, and why", report)
			}

			output, err = runCLIIn(repositoryPath, binaryPath, "init")
			if err != nil || !strings.Contains(output, "no tag can carry "+directory) || !strings.Contains(output, "rename the directory") {
				t.Errorf("init returned %v\n%s", err, output)
			}
			checked, _ := readCheck(t, binaryPath, repositoryPath)
			if item := itemOf(t, checked.Blocking, ruleInvalidTagName); len(checked.Blocking) != 1 || item.Subject != directory {
				t.Errorf("blocking = %+v, want only the directory to rename", checked.Blocking)
			}
		})
	}

	t.Run("a name of it that another module no entry lists has", func(t *testing.T) {
		t.Parallel()
		repositoryPath := withModule(t, "gen v2")
		writeFile(t, repositoryPath+"/bumpit.yaml", "modules:\n  - directories: [.]\n  - name: tool\n    directories: [gen v2]\n")

		report, _ := readCheck(t, binaryPath, repositoryPath)
		if !slices.ContainsFunc(report.Review, func(item CheckItem) bool { return item.Rule == ruleGoModuleUnlisted && item.Subject == "tool" }) {
			t.Errorf("review = %+v, leaves out the Go module in tool, which no entry lists", report.Review)
		}
	})

	t.Run("an edit beside it", func(t *testing.T) {
		t.Parallel()
		repositoryPath := withModule(t, "gen v2")
		writeFile(t, repositoryPath+"/bumpit.yaml", "modules:\n  - directories: [.]\n")

		expectContains(t, binaryPath, []string{"modules", "add", "tool", "-r", repositoryPath}, "added tool")
		expectContains(t, binaryPath, []string{"modules", "add", "gen v2", "-r", repositoryPath}, "added gen v2")
	})
}

// expectNoNewTag checks that the repository holds only the tags initGoRepository
// gives it.
func expectNoNewTag(t *testing.T, repositoryPath string) {
	t.Helper()

	output, err := runGitOutput(repositoryPath, "tag", "-l")
	if err != nil || strings.Join(strings.Fields(output), " ") != "tool/v0.1.0 v0.1.0" {
		t.Errorf("git tag -l returned %v\n%s", err, output)
	}
}
