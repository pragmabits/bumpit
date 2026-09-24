// Package gitx runs the git commands bumpit reads a repository with and
// creates its tags with. It never writes anything else and never pushes.
package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Client runs git in Repository, or in the working directory when it is empty.
type Client struct {
	Repository string
}

// Commit is one commit as bumpit reads it.
type Commit struct {
	Hash    string
	Subject string
	Body    string
}

// New returns a client for the repository at the given path.
func New(repository string) Client {
	return Client{Repository: repository}
}

// EnsureRepository fails when the path is not inside a git repository.
func (c Client) EnsureRepository() error {
	_, stderr, err := c.run("rev-parse", "--git-dir")
	if err != nil {
		return gitError("rev-parse --git-dir", stderr, err)
	}
	return nil
}

// IsDirty reports uncommitted changes, untracked files included.
func (c Client) IsDirty() (bool, error) {
	stdout, stderr, err := c.run("status", "--porcelain")
	if err != nil {
		return false, gitError("status --porcelain", stderr, err)
	}
	return strings.TrimSpace(stdout) != "", nil
}

// Tags returns every tag matching the glob pattern match, newest first by
// creation date, an order that says nothing about their versions.
func (c Client) Tags(match string) ([]string, error) {
	return c.listTags(nil, match)
}

// TagsMergedIntoHEAD is Tags restricted to the tags reachable from HEAD.
func (c Client) TagsMergedIntoHEAD(match string) ([]string, error) {
	return c.listTags([]string{"--merged", "HEAD"}, match)
}

func (c Client) listTags(filters []string, match string) ([]string, error) {
	arguments := append([]string{"tag"}, filters...)
	arguments = append(arguments, "--sort=-creatordate")
	if match != "" {
		arguments = append(arguments, "--list", match)
	}

	stdout, stderr, err := c.run(arguments...)
	if err != nil {
		return nil, gitError(strings.Join(arguments, " "), stderr, err)
	}

	var tags []string
	for tag := range strings.SplitSeq(stdout, "\n") {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
	}

	return tags, nil
}

// CommitsSince returns the commits after tag, oldest first, or every commit
// when tag is empty. Given pathspecs, only the commits touching them count.
func (c Client) CommitsSince(tag string, pathspecs ...string) ([]Commit, error) {
	arguments := []string{"log", "--reverse", "--no-merges", "--format=%H%x1f%s%x1f%b%x1e"}
	if tag != "" {
		arguments = append(arguments, fmt.Sprintf("%s..HEAD", tag))
	} else {
		arguments = append(arguments, "HEAD")
	}
	if len(pathspecs) > 0 {
		arguments = append(append(arguments, "--"), pathspecs...)
	}

	stdout, stderr, err := c.run(arguments...)
	if err != nil {
		return nil, gitError(strings.Join(arguments, " "), stderr, err)
	}

	var commits []Commit
	for record := range strings.SplitSeq(stdout, "\x1e") {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		fields := strings.Split(record, "\x1f")
		if len(fields) < 3 {
			continue
		}

		commits = append(commits, Commit{
			Hash:    strings.TrimSpace(fields[0]),
			Subject: strings.TrimSpace(fields[1]),
			Body:    strings.TrimSpace(fields[2]),
		})
	}

	return commits, nil
}

// Top returns the absolute path of the top directory of the working tree.
func (c Client) Top() (string, error) {
	stdout, stderr, err := c.run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", gitError("rev-parse --show-toplevel", stderr, err)
	}
	return strings.TrimSpace(stdout), nil
}

// TrackedFiles returns the tracked files matching pathspec, relative to the
// top directory whichever directory the client runs in.
func (c Client) TrackedFiles(pathspec string) ([]string, error) {
	stdout, stderr, err := c.run("ls-files", "--full-name", "-z", "--", pathspec)
	if err != nil {
		return nil, gitError("ls-files "+pathspec, stderr, err)
	}

	var files []string
	for file := range strings.SplitSeq(stdout, "\x00") {
		if file != "" {
			files = append(files, file)
		}
	}
	return files, nil
}

// TagExists reports whether a tag with the given name exists, on any branch.
func (c Client) TagExists(name string) (bool, error) {
	_, stderr, err := c.run("rev-parse", "-q", "--verify", "refs/tags/"+name)
	if err != nil {
		if stderr == "" {
			return false, nil
		}
		return false, gitError("rev-parse -q --verify refs/tags/"+name, stderr, err)
	}
	return true, nil
}

// CreateAnnotatedTag creates an annotated tag at HEAD, locally.
func (c Client) CreateAnnotatedTag(name, message string) error {
	_, stderr, err := c.run("tag", "-a", name, "-m", message)
	if err != nil {
		return gitError("tag -a "+name, stderr, err)
	}
	return nil
}

func (c Client) run(arguments ...string) (string, string, error) {
	cmd := exec.Command("git", arguments...)
	if c.Repository != "" {
		cmd.Dir = c.Repository
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), strings.TrimSpace(stderr.String()), err
}

func gitError(command, stderr string, err error) error {
	if stderr == "" {
		return fmt.Errorf("git %s: %w", command, err)
	}
	return fmt.Errorf("git %s: %s", command, stderr)
}
