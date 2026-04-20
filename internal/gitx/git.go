package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	Repository string
}

type Commit struct {
	Hash    string
	Subject string
	Body    string
}

func New(repository string) Client {
	return Client{Repository: repository}
}

func (c Client) EnsureRepository() error {
	_, stderr, err := c.run("rev-parse", "--git-dir")
	if err != nil {
		return gitError("rev-parse --git-dir", stderr, err)
	}
	return nil
}

func (c Client) IsDirty() (bool, error) {
	stdout, stderr, err := c.run("status", "--porcelain")
	if err != nil {
		return false, gitError("status --porcelain", stderr, err)
	}
	return strings.TrimSpace(stdout) != "", nil
}

func (c Client) LatestTag(match string) (string, error) {
	args := []string{"describe", "--tags", "--abbrev=0"}
	if match != "" {
		args = append(args, "--match", match)
	}
	args = append(args, "HEAD")

	stdout, stderr, err := c.run(args...)
	if err != nil {
		if strings.Contains(stderr, "No names found") || strings.Contains(stderr, "No tags can describe") {
			return "", nil
		}
		return "", gitError(strings.Join(args, " "), stderr, err)
	}

	return strings.TrimSpace(stdout), nil
}

func (c Client) TagsMergedIntoHEAD(match string) ([]string, error) {
	args := []string{"tag", "--merged", "HEAD", "--sort=-creatordate"}
	if match != "" {
		args = append(args, "--list", match)
	}

	stdout, stderr, err := c.run(args...)
	if err != nil {
		return nil, gitError(strings.Join(args, " "), stderr, err)
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

func (c Client) CommitsSince(tag string) ([]Commit, error) {
	args := []string{"log", "--reverse", "--no-merges", "--format=%H%x1f%s%x1f%b%x1e"}
	if tag != "" {
		args = append(args, fmt.Sprintf("%s..HEAD", tag))
	} else {
		args = append(args, "HEAD")
	}

	stdout, stderr, err := c.run(args...)
	if err != nil {
		return nil, gitError(strings.Join(args, " "), stderr, err)
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

func (c Client) CreateAnnotatedTag(name, message string) error {
	_, stderr, err := c.run("tag", "-a", name, "-m", message)
	if err != nil {
		return gitError("tag -a "+name, stderr, err)
	}
	return nil
}

func (c Client) run(args ...string) (string, string, error) {
	cmd := exec.Command("git", args...)
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
