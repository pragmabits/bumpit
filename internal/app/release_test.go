package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBuildReleasePlanMinorFromFeature(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.2.3", "-m", "Release v1.2.3")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if !plan.HasRelease {
		t.Fatalf("expected release plan to produce a release")
	}

	if plan.NextTag != "v1.3.0" {
		t.Fatalf("unexpected next tag: %s", plan.NextTag)
	}

	if plan.HighestBump != "minor" {
		t.Fatalf("unexpected bump: %s", plan.HighestBump)
	}
}

func TestBuildReleasePlanInitialPatchRelease(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "bugfix.txt", "bugfix", "fix(cli): avoid nil pointer")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if plan.NextTag != "v0.0.1" {
		t.Fatalf("unexpected next tag for initial patch release: %s", plan.NextTag)
	}
}

func TestBuildReleasePlanBreakingChange(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.2.3", "-m", "Release v1.2.3")
	commitFileWithBody(t, repositoryPath, "breaking.txt", "breaking", "chore!: reshape public API", "BREAKING CHANGE: remove old endpoint")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if plan.NextTag != "v2.0.0" {
		t.Fatalf("unexpected next tag for breaking change: %s", plan.NextTag)
	}
}

func TestBuildReleasePlanSkipsNonSemverTags(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.2.3", "-m", "Release v1.2.3")
	commitFile(t, repositoryPath, "marker.txt", "marker", "chore: add marker")
	runGit(t, repositoryPath, "tag", "-a", "release-latest", "-m", "Marker tag")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if plan.CurrentTag != "v1.2.3" {
		t.Fatalf("unexpected current tag: %s", plan.CurrentTag)
	}
	if plan.NextTag != "v1.3.0" {
		t.Fatalf("unexpected next tag: %s", plan.NextTag)
	}
}

func TestBuildReleasePlanContinuesPreReleaseLine(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v0.5.0-beta", "-m", "Release v0.5.0-beta")
	commitFile(t, repositoryPath, "feature.txt", "feature", "fix(api): adjust response")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if plan.NextTag != "v0.5.1-beta" {
		t.Fatalf("unexpected next tag: %s", plan.NextTag)
	}
}

func TestBuildReleasePlanPromotesPreRelease(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v0.5.0-beta", "-m", "Release v0.5.0-beta")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath, Promote: true})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if plan.NextTag != "v0.5.0" {
		t.Fatalf("unexpected next tag: %s", plan.NextTag)
	}
}

func TestBuildReleasePlanUsesExplicitPreRelease(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.2.3", "-m", "Release v1.2.3")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath, PreRelease: "rc"})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if plan.NextTag != "v1.3.0-rc" {
		t.Fatalf("unexpected next tag: %s", plan.NextTag)
	}
}

func TestBuildReleasePlanIncrementsExplicitPreReleaseLine(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v0.5.0-beta.1", "-m", "Release v0.5.0-beta.1")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath, PreRelease: "beta"})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if plan.NextTag != "v0.5.0-beta.2" {
		t.Fatalf("unexpected next tag: %s", plan.NextTag)
	}
}

func TestBuildReleasePlanRejectsPromoteOnStableVersion(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.2.3", "-m", "Release v1.2.3")

	_, err := BuildReleasePlan(Options{Repository: repositoryPath, Promote: true})
	if err == nil {
		t.Fatalf("expected error for promote on stable version")
	}
}

func TestBuildReleasePlanRejectsInvalidPreRelease(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.2.3", "-m", "Release v1.2.3")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")

	_, err := BuildReleasePlan(Options{Repository: repositoryPath, PreRelease: "beta!"})
	if err == nil {
		t.Fatalf("expected invalid prerelease error")
	}
}

func TestBuildReleasePlanRejectsPreReleaseAndPromoteTogether(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v0.5.0-beta", "-m", "Release v0.5.0-beta")

	_, err := BuildReleasePlan(Options{Repository: repositoryPath, PreRelease: "beta", Promote: true})
	if err == nil {
		t.Fatalf("expected prerelease/promote conflict error")
	}
}

func initRepository(t *testing.T) string {
	t.Helper()

	directory := t.TempDir()
	runGit(t, directory, "init")
	runGit(t, directory, "config", "user.name", "bumpit")
	runGit(t, directory, "config", "user.email", "bumpit@example.com")
	return directory
}

func commitFile(t *testing.T, repositoryPath, name, content, subject string) {
	t.Helper()
	commitFileWithBody(t, repositoryPath, name, content, subject, "")
}

func commitFileWithBody(t *testing.T, repositoryPath, name, content, subject, body string) {
	t.Helper()

	path := filepath.Join(repositoryPath, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	runGit(t, repositoryPath, "add", name)
	args := []string{"commit", "-m", subject}
	if body != "" {
		args = append(args, "-m", body)
	}
	runGit(t, repositoryPath, args...)
}

func runGit(t *testing.T, repositoryPath string, args ...string) {
	t.Helper()

	command := exec.Command("git", args...)
	command.Dir = repositoryPath
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}
