package gitx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestValidTagName(t *testing.T) {
	t.Parallel()

	client := New(t.TempDir())
	for name, want := range map[string]bool{
		"v0.0.0":              true,
		"homepage/v0.0.0":     true,
		"web/homepage/v0.0.0": true,
		"home..page/v0.0.0":   false,
		"web//v0.0.0":         false,
		"web/.v0.0.0":         false,
		"home page/v0.0.0":    false,
		"-x/v0.0.0":           false,
		"web/-x/v0.0.0":       true,
	} {
		if got, err := client.ValidTagName(name); got != want || err != nil {
			t.Errorf("ValidTagName(%q) = %v, %v; want %v", name, got, err, want)
		}
	}

	if _, err := New(filepath.Join(t.TempDir(), "gone")).ValidTagName("v0.0.0"); err == nil {
		t.Errorf("ValidTagName in a directory that does not exist reported no error")
	} else if _, failed := errors.AsType[*Error](err); !failed {
		t.Errorf("ValidTagName in a directory that does not exist returned %T, no git failure", err)
	}
}

// TestCommitOrderMatchesCommitsSince covers the order CommitOrder gives: the
// one CommitsSince returns, oldest first and merges left out, with or without
// a tag. It also covers IsAncestor on the same history.
func TestCommitOrderMatchesCommitsSince(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	run(t, repository, "init", "-q")
	run(t, repository, "config", "user.name", "bumpit")
	run(t, repository, "config", "user.email", "bumpit@example.com")
	commit(t, repository, "first.txt", "feat: first")
	run(t, repository, "tag", "-a", "v0.1.0", "-m", "Release v0.1.0")
	run(t, repository, "checkout", "-q", "-b", "side")
	commit(t, repository, "side.txt", "feat: on the side")
	run(t, repository, "checkout", "-q", "-")
	commit(t, repository, "main.txt", "fix: on the main line")
	run(t, repository, "merge", "-q", "--no-ff", "-m", "Merge side", "side")

	client := New(repository)
	for revisions, want := range map[[2]string]bool{{"v0.1.0", "HEAD"}: true, {"HEAD", "v0.1.0"}: false, {"v0.1.0", "v0.1.0"}: true} {
		if got, err := client.IsAncestor(revisions[0], revisions[1]); got != want || err != nil {
			t.Errorf("IsAncestor(%s, %s) = %v, %v; want %v", revisions[0], revisions[1], got, err, want)
		}
	}
	if _, err := client.IsAncestor("v9.9.9", "HEAD"); err == nil {
		t.Errorf("IsAncestor of a revision that does not exist reported no error")
	}
	for _, tag := range []string{"", "v0.1.0"} {
		order, err := client.CommitOrder(tag)
		if err != nil {
			t.Fatalf("CommitOrder(%q) returned error: %v", tag, err)
		}
		commits, err := client.CommitsSince(tag)
		if err != nil {
			t.Fatalf("CommitsSince(%q) returned error: %v", tag, err)
		}
		hashes := make([]string, 0, len(commits))
		for _, read := range commits {
			hashes = append(hashes, read.Hash)
		}
		if !slices.Equal(order, hashes) {
			t.Errorf("CommitOrder(%q) = %q, CommitsSince gives %q", tag, order, hashes)
		}
	}
}

// TestTagExists covers a tag that exists, one that does not, and git failing to
// answer, which is no answer that the tag does not exist.
func TestTagExists(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	run(t, repository, "init", "-q")
	run(t, repository, "config", "user.name", "bumpit")
	run(t, repository, "config", "user.email", "bumpit@example.com")
	commit(t, repository, "first.txt", "feat: first")
	run(t, repository, "tag", "-a", "v0.1.0", "-m", "Release v0.1.0")

	client := New(repository)
	for name, want := range map[string]bool{"v0.1.0": true, "v0.2.0": false} {
		if got, err := client.TagExists(name); got != want || err != nil {
			t.Errorf("TagExists(%q) = %v, %v; want %v", name, got, err, want)
		}
	}
	if _, err := New(filepath.Join(t.TempDir(), "gone")).TagExists("v0.1.0"); err == nil {
		t.Errorf("TagExists in a directory that does not exist reported no error")
	} else if _, failed := errors.AsType[*Error](err); !failed {
		t.Errorf("TagExists in a directory that does not exist returned %T, no git failure", err)
	}
}

func commit(t *testing.T, repository, name, subject string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(repository, name), []byte(subject), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	run(t, repository, "add", name)
	run(t, repository, "commit", "-q", "-m", subject)
}

func run(t *testing.T, repository string, arguments ...string) {
	t.Helper()

	command := exec.Command("git", arguments...)
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", arguments, err, output)
	}
}
