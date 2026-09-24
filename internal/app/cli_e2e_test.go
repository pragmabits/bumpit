package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIEndToEndNextAndExplain(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.2.3", "-m", "Release v1.2.3")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")

	binaryPath := buildCLI(t)

	nextOutput, err := runCLI(binaryPath, "next", "--repo", repositoryPath)
	if err != nil {
		t.Fatalf("next command returned error: %v\noutput: %s", err, nextOutput)
	}
	if strings.TrimSpace(nextOutput) != "v1.3.0" {
		t.Fatalf("unexpected next output: %q", nextOutput)
	}

	explainOutput, err := runCLI(binaryPath, "explain", "--repo", repositoryPath, "--output", "json")
	if err != nil {
		t.Fatalf("explain command returned error: %v\noutput: %s", err, explainOutput)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(explainOutput), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if payload["next_tag"] != "v1.3.0" {
		t.Fatalf("unexpected next_tag: %#v", payload["next_tag"])
	}
}

func TestCLIEndToEndTagUsesConfigMessage(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")

	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	content := []byte("repository: .\ntagMatch: v*\nstartVersion: \"\"\nallowDirty: true\noutput: text\ntagMessage: Release from config\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	binaryPath := buildCLI(t)
	tagOutput, err := runCLI(binaryPath, "tag", "--repo", repositoryPath)
	if err != nil {
		t.Fatalf("tag command returned error: %v\noutput: %s", err, tagOutput)
	}
	if strings.TrimSpace(tagOutput) != "created tag v0.1.0" {
		t.Fatalf("unexpected tag output: %q", tagOutput)
	}

	tagListOutput, err := runGitOutput(repositoryPath, "tag", "-n99", "--list", "v0.1.0")
	if err != nil {
		t.Fatalf("runGitOutput returned error: %v", err)
	}
	if !strings.Contains(tagListOutput, "Release from config") {
		t.Fatalf("unexpected tag annotation: %q", tagListOutput)
	}
}

func TestCLIEndToEndVersion(t *testing.T) {
	t.Parallel()

	t.Run("recorded by the go command", func(t *testing.T) {
		t.Parallel()

		binaryPath := buildCLI(t)
		want := recordedVersion(t, binaryPath)
		assertVersion(t, binaryPath, want)
	})

	t.Run("set at link time", func(t *testing.T) {
		t.Parallel()

		binaryPath := buildCLI(t, "-ldflags", "-X github.com/pragmabits/bumpit/internal/app.buildVersion=v9.8.7")
		assertVersion(t, binaryPath, "v9.8.7")
	})
}

// assertVersion checks that both bumpit version and bumpit --version report
// want.
func assertVersion(t *testing.T, binaryPath, want string) {
	t.Helper()

	versionOutput, err := runCLI(binaryPath, "version")
	if err != nil {
		t.Fatalf("version command returned error: %v\noutput: %s", err, versionOutput)
	}
	if strings.TrimSpace(versionOutput) != want {
		t.Errorf("bumpit version printed %q, want %q", strings.TrimSpace(versionOutput), want)
	}

	flagOutput, err := runCLI(binaryPath, "--version")
	if err != nil {
		t.Fatalf("--version returned error: %v\noutput: %s", err, flagOutput)
	}
	if strings.TrimSpace(flagOutput) != "bumpit version "+want {
		t.Errorf("bumpit --version printed %q, want %q", strings.TrimSpace(flagOutput), "bumpit version "+want)
	}
}

// recordedVersion is the main module version the go command recorded in the
// binary, as go version -m prints it, or "dev" when it recorded none.
func recordedVersion(t *testing.T, binaryPath string) string {
	t.Helper()

	output, err := exec.Command("go", "version", "-m", binaryPath).CombinedOutput()
	if err != nil {
		t.Fatalf("go version -m failed: %v\n%s", err, string(output))
	}
	for line := range strings.SplitSeq(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "mod" && fields[2] != "(devel)" {
			return fields[2]
		}
	}
	return "dev"
}

func TestCLIEndToEndPromotePreRelease(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v0.5.0-beta", "-m", "Release v0.5.0-beta")

	binaryPath := buildCLI(t)
	nextOutput, err := runCLI(binaryPath, "next", "--repo", repositoryPath, "--promote")
	if err != nil {
		t.Fatalf("next command returned error: %v\noutput: %s", err, nextOutput)
	}
	if strings.TrimSpace(nextOutput) != "v0.5.0" {
		t.Fatalf("unexpected next output: %q", nextOutput)
	}
}

func TestCLIEndToEndUsesExplicitPreRelease(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.2.3", "-m", "Release v1.2.3")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")

	binaryPath := buildCLI(t)
	nextOutput, err := runCLI(binaryPath, "next", "--repo", repositoryPath, "--pre", "beta")
	if err != nil {
		t.Fatalf("next command returned error: %v\noutput: %s", err, nextOutput)
	}
	if strings.TrimSpace(nextOutput) != "v1.3.0-beta" {
		t.Fatalf("unexpected next output: %q", nextOutput)
	}
}

func TestCLIEndToEndReleaseAs(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v0.4.0", "-m", "Release v0.4.0")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): stabilize the API")

	binaryPath := buildCLI(t)

	for _, flag := range []string{"--release-as", "-V"} {
		nextOutput, err := runCLI(binaryPath, "next", "--repo", repositoryPath, flag, "1.0.0")
		if err != nil {
			t.Fatalf("next %s returned error: %v\noutput: %s", flag, err, nextOutput)
		}
		if strings.TrimSpace(nextOutput) != "v1.0.0" {
			t.Fatalf("unexpected next %s output: %q", flag, nextOutput)
		}
	}
}

func TestCLIEndToEndGoModules(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitFile(t, repositoryPath, "tool/main.txt", "main", "feat(tool): add a flag")

	binaryPath := buildCLI(t)

	commands := []struct {
		arguments []string
		expected  string
	}{
		{[]string{"next", "-r", repositoryPath}, "no release"},
		{[]string{"next", "-r", repositoryPath, "-g", "tool"}, "tool/v0.2.0"},
		{[]string{"latest", "-r", repositoryPath, "--module", "tool"}, "tool/v0.1.0"},
	}
	for _, command := range commands {
		output, err := runCLI(binaryPath, command.arguments...)
		if err != nil {
			t.Fatalf("%v returned error: %v\noutput: %s", command.arguments, err, output)
		}
		if strings.TrimSpace(output) != command.expected {
			t.Errorf("%v printed %q, want %q", command.arguments, strings.TrimSpace(output), command.expected)
		}
	}

	modulesOutput, err := runCLI(binaryPath, "modules", "-r", repositoryPath)
	if err != nil {
		t.Fatalf("modules returned error: %v\noutput: %s", err, modulesOutput)
	}
	for _, text := range []string{"example.com/repository/tool", "tool/v0.2.0", "no release"} {
		if !strings.Contains(modulesOutput, text) {
			t.Errorf("modules output lacks %q:\n%s", text, modulesOutput)
		}
	}

	jsonOutput, err := runCLI(binaryPath, "modules", "-r", repositoryPath, "-o", "json")
	if err != nil {
		t.Fatalf("modules -o json returned error: %v\noutput: %s", err, jsonOutput)
	}
	var releases []ModuleRelease
	if err := json.Unmarshal([]byte(jsonOutput), &releases); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v\noutput: %s", err, jsonOutput)
	}
	if len(releases) != 2 || releases[1].NextTag != "tool/v0.2.0" {
		t.Fatalf("unexpected modules payload: %#v", releases)
	}
}

func TestCLIEndToEndLatest(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.9.0", "-m", "Release v1.9.0")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat: another feature")
	runGit(t, repositoryPath, "tag", "-a", "v1.10.0", "-m", "Release v1.10.0")

	binaryPath := buildCLI(t)

	latestOutput, err := runCLI(binaryPath, "latest", "--repo", repositoryPath)
	if err != nil {
		t.Fatalf("latest command returned error: %v\noutput: %s", err, latestOutput)
	}
	if strings.TrimSpace(latestOutput) != "v1.10.0" {
		t.Fatalf("unexpected latest output: %q", latestOutput)
	}

	jsonOutput, err := runCLI(binaryPath, "latest", "--repo", repositoryPath, "--output", "json")
	if err != nil {
		t.Fatalf("latest command returned error: %v\noutput: %s", err, jsonOutput)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonOutput), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if payload["tag"] != "v1.10.0" {
		t.Fatalf("unexpected tag: %#v", payload["tag"])
	}
	if payload["version"] != "1.10.0" {
		t.Fatalf("unexpected version: %#v", payload["version"])
	}
	if payload["found"] != true {
		t.Fatalf("unexpected found: %#v", payload["found"])
	}
}

func TestCLIEndToEndShortFlags(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.2.3", "-m", "Release v1.2.3")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")

	binaryPath := buildCLI(t)

	latestOutput, err := runCLI(binaryPath, "latest", "-r", repositoryPath, "-t", "v*")
	if err != nil {
		t.Fatalf("latest command returned error: %v\noutput: %s", err, latestOutput)
	}
	if strings.TrimSpace(latestOutput) != "v1.2.3" {
		t.Fatalf("unexpected latest output: %q", latestOutput)
	}

	nextOutput, err := runCLI(binaryPath, "next", "-r", repositoryPath, "-p", "beta")
	if err != nil {
		t.Fatalf("next command returned error: %v\noutput: %s", err, nextOutput)
	}
	if strings.TrimSpace(nextOutput) != "v1.3.0-beta" {
		t.Fatalf("unexpected next output: %q", nextOutput)
	}

	explainOutput, err := runCLI(binaryPath, "explain", "-r", repositoryPath, "-o", "json")
	if err != nil {
		t.Fatalf("explain command returned error: %v\noutput: %s", err, explainOutput)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(explainOutput), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if payload["next_tag"] != "v1.3.0" {
		t.Fatalf("unexpected next_tag: %#v", payload["next_tag"])
	}

	tagOutput, err := runCLI(binaryPath, "tag", "-r", repositoryPath, "-m", "Release from short flag")
	if err != nil {
		t.Fatalf("tag command returned error: %v\noutput: %s", err, tagOutput)
	}
	if strings.TrimSpace(tagOutput) != "created tag v1.3.0" {
		t.Fatalf("unexpected tag output: %q", tagOutput)
	}

	annotationOutput, err := runGitOutput(repositoryPath, "tag", "-n99", "--list", "v1.3.0")
	if err != nil {
		t.Fatalf("runGitOutput returned error: %v", err)
	}
	if !strings.Contains(annotationOutput, "Release from short flag") {
		t.Fatalf("unexpected tag annotation: %q", annotationOutput)
	}
}

func TestCLIEndToEndLatestNoPrefix(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.4.0-rc.2", "-m", "Release v1.4.0-rc.2")

	binaryPath := buildCLI(t)

	tagOutput, err := runCLI(binaryPath, "latest", "--repo", repositoryPath)
	if err != nil {
		t.Fatalf("latest command returned error: %v\noutput: %s", err, tagOutput)
	}
	if strings.TrimSpace(tagOutput) != "v1.4.0-rc.2" {
		t.Fatalf("unexpected latest output: %q", tagOutput)
	}

	versionOutput, err := runCLI(binaryPath, "latest", "--repo", repositoryPath, "--no-prefix")
	if err != nil {
		t.Fatalf("latest command returned error: %v\noutput: %s", err, versionOutput)
	}
	if strings.TrimSpace(versionOutput) != "1.4.0-rc.2" {
		t.Fatalf("unexpected no-prefix output: %q", versionOutput)
	}

	shortOutput, err := runCLI(binaryPath, "latest", "-n", "-r", repositoryPath)
	if err != nil {
		t.Fatalf("latest command returned error: %v\noutput: %s", err, shortOutput)
	}
	if strings.TrimSpace(shortOutput) != "1.4.0-rc.2" {
		t.Fatalf("unexpected short flag output: %q", shortOutput)
	}
}

func TestCLIEndToEndLatestNoPrefixWithoutTags(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")

	binaryPath := buildCLI(t)
	latestOutput, err := runCLI(binaryPath, "latest", "--repo", repositoryPath, "--no-prefix")
	if err != nil {
		t.Fatalf("latest command returned error: %v\noutput: %s", err, latestOutput)
	}
	if strings.TrimSpace(latestOutput) != "no tag" {
		t.Fatalf("unexpected latest output: %q", latestOutput)
	}
}

func TestCLIEndToEndLatestWithoutTags(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")

	binaryPath := buildCLI(t)
	latestOutput, err := runCLI(binaryPath, "latest", "--repo", repositoryPath)
	if err != nil {
		t.Fatalf("latest command returned error: %v\noutput: %s", err, latestOutput)
	}
	if strings.TrimSpace(latestOutput) != "no tag" {
		t.Fatalf("unexpected latest output: %q", latestOutput)
	}
}

// buildCLI builds bumpit into a temporary directory, passing buildFlags to go
// build, and returns the binary's path.
func buildCLI(t *testing.T, buildFlags ...string) string {
	t.Helper()

	projectRoot := projectRoot(t)
	binaryPath := filepath.Join(t.TempDir(), "bumpit")

	arguments := append(append([]string{"build"}, buildFlags...), "-o", binaryPath, ".")
	command := exec.Command("go", arguments...)
	command.Dir = projectRoot
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(output))
	}

	return binaryPath
}

func runCLI(binaryPath string, arguments ...string) (string, error) {
	command := exec.Command(binaryPath, arguments...)
	output, err := command.CombinedOutput()
	return string(output), err
}

func runGitOutput(repositoryPath string, arguments ...string) (string, error) {
	command := exec.Command("git", arguments...)
	command.Dir = repositoryPath
	output, err := command.CombinedOutput()
	return string(output), err
}

func projectRoot(t *testing.T) string {
	t.Helper()

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}

	return filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
}
