package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// speaksOfTheFile reports an error that sends whoever reads it to the config file.
func speaksOfTheFile(output string) bool {
	return strings.Contains(output, "the file") || strings.Contains(output, "bumpit config")
}

// TestCLIEndToEndAFlagIsNoProblemOfTheFile covers a value the command line gives
// and the file does not hold: the run stops at it, and sends no one to edit a file
// that does not say it.
func TestCLIEndToEndAFlagIsNoProblemOfTheFile(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "allowDirty: true\n")
	binaryPath := buildCLI(t)

	cases := []struct {
		arguments []string
		wanted    string
	}{
		{arguments: []string{"next", "-p", "!!bad"}, wanted: `invalid prerelease "!!bad"`},
		{arguments: []string{"next", "-o", "xml"}, wanted: `unsupported output format "xml"`},
		{arguments: []string{"next", "-s", "nonsense"}, wanted: `invalid semantic version "nonsense"`},
		{arguments: []string{"next", "-p", "beta", "-P"}, wanted: "cannot be used together"},
		{arguments: []string{"next", "-c", "bumpit.yaml", "-r", "/does/not/exist"}, wanted: `repository path "/does/not/exist"`},
		{arguments: []string{"latest", "-o", "xml"}, wanted: `unsupported output format "xml"`},
		{arguments: []string{"modules", "-o", "xml"}, wanted: `unsupported output format "xml"`},
		{arguments: []string{"tag", "-p", "!!bad"}, wanted: `invalid prerelease "!!bad"`},
	}
	for _, testCase := range cases {
		output, err := runCLIIn(repositoryPath, binaryPath, testCase.arguments...)
		if err == nil || !strings.Contains(output, testCase.wanted) || speaksOfTheFile(output) {
			t.Errorf("bumpit %v returned %v\n%s", testCase.arguments, err, output)
		}
	}
}

// TestCLIEndToEndAKeyOfTheFileIsAProblemOfTheFile covers a value the file holds,
// whatever the command line gives: it says how to resolve it in the file. Two keys
// that conflict are a problem of the file only when it holds both.
func TestCLIEndToEndAKeyOfTheFileIsAProblemOfTheFile(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	inTheFile := []struct {
		content   string
		arguments []string
		wanted    string
	}{
		{content: "output: xml\n", arguments: []string{"next"}, wanted: "give it another value with bumpit config set"},
		{content: "output: xml\n", arguments: []string{"next", "-o", "json"}, wanted: "give it another value with bumpit config set"},
		{content: "preRelease: rc\npromote: true\n", arguments: []string{"next"}, wanted: "take out one of them with bumpit config unset"},
	}
	for _, testCase := range inTheFile {
		repositoryPath := initGoRepository(t)
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), testCase.content)
		output, err := runCLIIn(repositoryPath, binaryPath, testCase.arguments...)
		if err == nil || !strings.Contains(output, testCase.wanted) {
			t.Errorf("%q, bumpit %v returned %v\n%s", testCase.content, testCase.arguments, err, output)
		}
	}

	half := []struct {
		content   string
		arguments []string
	}{
		{content: "preRelease: rc\n", arguments: []string{"next", "-P"}},
		{content: "promote: true\n", arguments: []string{"next", "-p", "rc"}},
	}
	for _, testCase := range half {
		repositoryPath := initGoRepository(t)
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), testCase.content)
		output, err := runCLIIn(repositoryPath, binaryPath, testCase.arguments...)
		if err == nil || !strings.Contains(output, "cannot be used together") || speaksOfTheFile(output) {
			t.Errorf("%q, bumpit %v returned %v\n%s", testCase.content, testCase.arguments, err, output)
		}
	}
}

// TestCLIEndToEndCheckLeavesTheRepoFlagOutOfTheFile covers a --repo the file does
// not give: the check reads the file for what it holds, and has nothing to say of
// its repository key.
func TestCLIEndToEndCheckLeavesTheRepoFlagOutOfTheFile(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "conf"), 0o755); err != nil {
		t.Fatalf("Mkdir returned error: %v", err)
	}
	writeFile(t, filepath.Join(directory, "conf", "bumpit.yaml"), "allowDirty: true\n")
	relative, err := filepath.Rel(directory, repositoryPath)
	if err != nil {
		t.Fatalf("Rel returned error: %v", err)
	}

	report, _ := readCheck(t, buildCLI(t), directory, "-c", filepath.Join("conf", "bumpit.yaml"), "-r", relative)
	if countRule(report.Blocking, ruleInvalidKey) != 0 {
		t.Errorf("blocking = %+v, want nothing of a key the file does not hold", report.Blocking)
	}
}

// TestCLIEndToEndARelativeRepoBesideAConfigElsewhere covers a --repo relative to
// where the command runs, with a --config in another directory that gives no
// repository: the runs read it as the check does, from where the command runs.
func TestCLIEndToEndARelativeRepoBesideAConfigElsewhere(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "conf"), 0o755); err != nil {
		t.Fatalf("Mkdir returned error: %v", err)
	}
	configPath := filepath.Join("conf", "bumpit.yaml")
	writeFile(t, filepath.Join(directory, configPath), "allowDirty: true\n")
	relative, err := filepath.Rel(directory, repositoryPath)
	if err != nil {
		t.Fatalf("Rel returned error: %v", err)
	}
	binaryPath := buildCLI(t)

	for _, arguments := range [][]string{{"latest"}, {"next"}, {"explain"}, {"modules"}} {
		command := append(arguments, "-c", configPath, "-r", relative)
		if output, err := runCLIIn(directory, binaryPath, command...); err != nil {
			t.Errorf("bumpit %v returned %v\n%s", command, err, output)
		}
	}
	output, err := runCLIIn(directory, binaryPath, "latest", "-c", configPath, "-r", relative)
	if err != nil || strings.TrimSpace(output) != "v0.1.0" {
		t.Errorf("latest returned %v\n%s", err, output)
	}

	output, err = runCLIIn(directory, binaryPath, "next", "-c", configPath, "-r", "nowhere")
	if err == nil || !strings.Contains(output, `repository path "nowhere"`) || speaksOfTheFile(output) {
		t.Errorf("a --repo that leads nowhere is said as it was written: %v\n%s", err, output)
	}

	// With no --repo the repository is still the directory of the config.
	writeFile(t, filepath.Join(repositoryPath, "other.yaml"), "allowDirty: true\n")
	output, err = runCLIIn(directory, binaryPath, "latest", "-c", filepath.Join(repositoryPath, "other.yaml"))
	if err != nil || strings.TrimSpace(output) != "v0.1.0" {
		t.Errorf("latest with no --repo returned %v\n%s", err, output)
	}
}

// TestCLIEndToEndARepositoryKeyThatLeadsNowhere covers the key the repository comes
// from: what cannot find the repository because of it says so of the key, and how
// to resolve it, in a repository or out of one.
func TestCLIEndToEndARepositoryKeyThatLeadsNowhere(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	const content = "repository: ../gone\n"

	t.Run("a check outside a repository", func(t *testing.T) {
		t.Parallel()
		outside := t.TempDir()
		configPath := filepath.Join(outside, "bumpit.yaml")
		writeFile(t, configPath, content)

		report, passed := readCheck(t, binaryPath, outside)
		item := itemOf(t, report.Blocking, ruleInvalidKey)
		if passed || !report.FileOnly || item.Subject != "repository" || item.Command != "bumpit config unset repository" {
			t.Fatalf("passed %v, report %+v", passed, report)
		}
		expectFile(t, configPath, content)

		if output, err := runCLIIn(outside, binaryPath, strings.Fields(item.Command)[1:]...); err != nil {
			t.Fatalf("%s returned %v\n%s", item.Command, err, output)
		}
		output, err := runCLIIn(outside, binaryPath, "modules", "check")
		if err == nil || !strings.Contains(output, "no git repository") {
			t.Errorf("once the key is out, what is left is that there is no repository: %v\n%s", err, output)
		}
	})

	t.Run("a check of another key outside a repository", func(t *testing.T) {
		t.Parallel()
		outside := t.TempDir()
		writeFile(t, filepath.Join(outside, "bumpit.yaml"), "output: xml\n")

		report, passed := readCheck(t, binaryPath, outside)
		if item := itemOf(t, report.Blocking, ruleInvalidKey); passed || item.Subject != "output" {
			t.Errorf("passed %v, report %+v", passed, report)
		}
	})

	t.Run("init", func(t *testing.T) {
		t.Parallel()
		for _, directory := range []string{t.TempDir(), initGoRepository(t)} {
			configPath := filepath.Join(directory, "bumpit.yaml")
			writeFile(t, configPath, content)

			output, err := runCLIIn(directory, binaryPath, "init")
			if err == nil || strings.Contains(output, "no git repository") {
				t.Errorf("init in %s returned %v\n%s", directory, err, output)
			}
			for _, wanted := range []string{"repository: repository path", "bumpit config unset", "bumpit modules check lists all there is to resolve"} {
				if !strings.Contains(output, wanted) {
					t.Errorf("init in %s lacks %q:\n%s", directory, wanted, output)
				}
			}
			expectFile(t, configPath, content)
		}
	})

	t.Run("an edit of the modules", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initGoRepository(t)
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), content)

		output, err := runCLIIn(repositoryPath, binaryPath, "modules", "add", "tool")
		if err == nil || strings.Contains(output, "no git repository") || !strings.Contains(output, "repository: repository path") {
			t.Errorf("add returned %v\n%s", err, output)
		}
		expectContains(t, binaryPath, []string{"modules", "add", "tool", "-r", repositoryPath}, "added tool")
	})
}
