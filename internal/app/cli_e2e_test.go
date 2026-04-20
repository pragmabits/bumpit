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

	binaryPath := buildCLI(t)
	versionOutput, err := runCLI(binaryPath, "version")
	if err != nil {
		t.Fatalf("version command returned error: %v\noutput: %s", err, versionOutput)
	}
	if strings.TrimSpace(versionOutput) != "dev" {
		t.Fatalf("unexpected version output: %q", versionOutput)
	}
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

func buildCLI(t *testing.T) string {
	t.Helper()

	projectRoot := projectRoot(t)
	binaryPath := filepath.Join(t.TempDir(), "bumpit")

	command := exec.Command("go", "build", "-o", binaryPath, "./cmd/bumpit")
	command.Dir = projectRoot
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(output))
	}

	return binaryPath
}

func runCLI(binaryPath string, args ...string) (string, error) {
	command := exec.Command(binaryPath, args...)
	output, err := command.CombinedOutput()
	return string(output), err
}

func runGitOutput(repositoryPath string, args ...string) (string, error) {
	command := exec.Command("git", args...)
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
