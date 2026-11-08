package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCLIEndToEndAGitFailureIsNoRefusal covers git failing while bumpit proves an
// edit or rehearses a command it would print: that is a failure to ask, not a
// refusal, so the command fails with it, says nothing is refused and writes
// nothing.
func TestCLIEndToEndAGitFailureIsNoRefusal(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	environment := gitFailingOn(t, "check-ref-format*homepage*")
	withTags := func(t *testing.T) string {
		t.Helper()
		repositoryPath := initHomepageRepository(t)
		runGit(t, repositoryPath, "tag", "-a", "homepage/v1.0.0", "-m", "Release homepage/v1.0.0")
		return repositoryPath
	}

	t.Run("an edit", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initHomepageRepository(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, "modules:\n  - directories: [.]\n")

		output, err := runCLIWith(environment, binaryPath, "modules", "add", "web/homepage", "-e", "homepage", "-r", repositoryPath)
		if err == nil || !strings.Contains(output, gitCannotAnswer) || strings.Contains(output, "refused") {
			t.Errorf("modules add returned %v\n%s", err, output)
		}
		expectFile(t, configPath, "modules:\n  - directories: [.]\n")
	})

	t.Run("init", func(t *testing.T) {
		t.Parallel()
		repositoryPath := withTags(t)

		output, err := runCLIWith(environment, binaryPath, "init", "-r", repositoryPath)
		if err == nil || !strings.Contains(output, gitCannotAnswer) || strings.Contains(output, "refuse") {
			t.Errorf("init returned %v\n%s", err, output)
		}
		if _, statErr := os.Stat(filepath.Join(repositoryPath, "bumpit.yaml")); !os.IsNotExist(statErr) {
			t.Errorf("init wrote bumpit.yaml: %v", statErr)
		}
	})

	t.Run("the check of a repository with no file", func(t *testing.T) {
		t.Parallel()
		repositoryPath := withTags(t)

		output, err := runCLIWith(environment, binaryPath, "modules", "check", "-r", repositoryPath)
		if err == nil || !strings.Contains(output, gitCannotAnswer) || strings.Contains(output, "refuse") || strings.Contains(output, "run: bumpit init") {
			t.Errorf("modules check returned %v\n%s", err, output)
		}
	})

	t.Run("the check", func(t *testing.T) {
		t.Parallel()
		repositoryPath := withTags(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, "modules:\n  - directories: [.]\n")

		output, err := runCLIWith(environment, binaryPath, "modules", "check", "-r", repositoryPath)
		if err == nil || !strings.Contains(output, gitCannotAnswer) || strings.Contains(output, "refuse") {
			t.Errorf("modules check returned %v\n%s", err, output)
		}
		expectFile(t, configPath, "modules:\n  - directories: [.]\n")
	})
}

// TestCLIEndToEndAFileItCannotReadIsNoRefusal covers a Go file bumpit cannot read
// while it rehearses the init a check proposes: that is a failure to ask, as git
// failing is, so the check fails with it rather than say init would be refused.
func TestCLIEndToEndAFileItCannotReadIsNoRefusal(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root reads a directory it has no permission to")
	}
	repositoryPath := initHomepageRepository(t)
	runGit(t, repositoryPath, "tag", "-a", "homepage/v1.0.0", "-m", "Release homepage/v1.0.0")
	commitNewFile(t, repositoryPath, "web/site.go", "package web\n", "feat: serve the site")
	locked := filepath.Join(repositoryPath, "web")
	if err := os.Chmod(locked, 0o600); err != nil {
		t.Fatalf("Chmod returned error: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "modules", "check", "-r", repositoryPath)
	if err == nil || !strings.Contains(output, "permission denied") || strings.Contains(output, "refuse") {
		t.Errorf("modules check returned %v\n%s", err, output)
	}
}

// TestCLIEndToEndARepositoryGitCannotReadSaysWhy covers git failing to find the
// top of the repository, as it does in one it refuses for its owner: the commands
// that need the top say what git said, not only that there is no repository.
func TestCLIEndToEndARepositoryGitCannotReadSaysWhy(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	environment := gitFailingOn(t, "rev-parse*--show-toplevel*")
	repositoryPath := initHomepageRepository(t)
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [.]\n")

	for _, arguments := range [][]string{{"modules", "check"}, {"modules", "add", "web/homepage"}} {
		output, err := runCLIWith(environment, binaryPath, append(arguments, "-r", repositoryPath)...)
		if err == nil || !strings.Contains(output, gitCannotAnswer) {
			t.Errorf("bumpit %v returned %v\n%s", arguments, err, output)
		}
	}
	expectFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [.]\n")
}

// gitCannotAnswer is what the git of gitFailingOn says when it fails.
const gitCannotAnswer = "fatal: cannot answer"

// gitFailingOn is the environment of a bumpit whose git fails, as a git that
// cannot answer does, on the commands its arguments match, a pattern of a shell
// case, and answers the rest as git does.
func gitFailingOn(t *testing.T, arguments string) []string {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the failing git is a shell script")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("LookPath returned error: %v", err)
	}
	directory := t.TempDir()
	script := "#!/bin/sh\ncase \"$*\" in\n  " + arguments + ") echo '" + gitCannotAnswer + "' >&2; exit 128 ;;\nesac\nexec '" + realGit + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(directory, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return append(os.Environ(), "PATH="+directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// runCLIWith is runCLI in environment.
func runCLIWith(environment []string, binaryPath string, arguments ...string) (string, error) {
	command := exec.Command(binaryPath, arguments...)
	command.Env = environment
	output, err := command.CombinedOutput()
	return string(output), err
}
