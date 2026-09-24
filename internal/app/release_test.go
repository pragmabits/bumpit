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

func TestBuildReleasePlanInitialRelease(t *testing.T) {
	t.Parallel()

	for _, subject := range []string{"fix(cli): avoid nil pointer", "feat!: first public API"} {
		t.Run(subject, func(t *testing.T) {
			t.Parallel()

			repositoryPath := initRepository(t)
			commitFile(t, repositoryPath, "first.txt", "first", subject)

			plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
			if err != nil {
				t.Fatalf("BuildReleasePlan returned error: %v", err)
			}

			if plan.NextTag != "v0.1.0" {
				t.Fatalf("unexpected next tag for the initial release: %s", plan.NextTag)
			}
		})
	}
}

func TestBuildReleasePlanBreakingChangeInMajorVersionZero(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v0.1.0", "-m", "Release v0.1.0")
	commitFile(t, repositoryPath, "breaking.txt", "breaking", "feat!: reshape public API")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if plan.NextTag != "v0.2.0" {
		t.Fatalf("unexpected next tag for a breaking change in 0.y.z: %s", plan.NextTag)
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

func TestBuildReleasePlanKeepsCustomPrefix(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "release-1.0.0", "-m", "Release 1.0.0")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath, TagMatch: "release-*"})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if plan.NextTag != "release-1.1.0" {
		t.Fatalf("unexpected next tag: %s", plan.NextTag)
	}
}

func TestBuildReleasePlanVersionsNestedModule(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v9.0.0", "-m", "Root module")
	runGit(t, repositoryPath, "tag", "-a", "cmd/tool/v0.1.0", "-m", "Tool 0.1.0")
	commitFile(t, repositoryPath, "fix.txt", "fix", "fix(tool): handle empty input")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath, TagMatch: "cmd/tool/v*"})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if plan.NextTag != "cmd/tool/v0.1.1" {
		t.Fatalf("unexpected next tag: %s", plan.NextTag)
	}
}

func TestBuildReleasePlanAcceptsStartVersionWithPrefix(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath, StartVersion: "v2.0.0"})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if plan.NextTag != "v2.0.0" {
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

	if plan.NextTag != "v0.5.0-beta.1" {
		t.Fatalf("unexpected next tag: %s", plan.NextTag)
	}
}

func TestBuildReleasePlanKeepsPreReleaseOnItsNormalVersion(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")
	runGit(t, repositoryPath, "tag", "-a", "v1.1.0-beta.2", "-m", "Release v1.1.0-beta.2")

	steps := []struct {
		subject  string
		expected string
	}{
		{"fix(api): adjust response", "v1.1.0-beta.3"},
		{"feat(api): add import endpoint", "v1.1.0-beta.3"},
		{"feat(api)!: drop the v1 routes", "v2.0.0-beta"},
	}

	for index, step := range steps {
		commitFile(t, repositoryPath, "step.txt", step.subject, step.subject)

		plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
		if err != nil {
			t.Fatalf("step %d: BuildReleasePlan returned error: %v", index, err)
		}
		if plan.NextTag != step.expected {
			t.Errorf("after %q: next tag = %s, want %s", step.subject, plan.NextTag, step.expected)
		}
		if plan.BaseTag != "v1.0.0" {
			t.Errorf("after %q: base tag = %q, want v1.0.0", step.subject, plan.BaseTag)
		}
	}
}

func TestBuildReleasePlanNoReleaseWithoutCommitsAfterPreRelease(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")
	runGit(t, repositoryPath, "tag", "-a", "v1.1.0-beta.2", "-m", "Release v1.1.0-beta.2")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if plan.HasRelease {
		t.Fatalf("expected no release, got %s", plan.NextTag)
	}
}

func TestBuildReleasePlanRejectsLowerPreRelease(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.1.0-beta.2", "-m", "Release v1.1.0-beta.2")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath, PreRelease: "alpha"})
	if err == nil {
		t.Fatalf("expected an error for a prerelease below v1.1.0-beta.2, got %s", plan.NextTag)
	}
}

func TestBuildReleasePlanRejectsExistingTag(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	runGit(t, repositoryPath, "checkout", "-b", "sidebranch")
	commitFile(t, repositoryPath, "side.txt", "side", "feat: side feature")
	runGit(t, repositoryPath, "tag", "-a", "v1.1.0", "-m", "Release v1.1.0")
	runGit(t, repositoryPath, "checkout", "-")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat: main feature")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
	if err == nil {
		t.Fatalf("expected an error for a tag that already exists, got %s", plan.NextTag)
	}
}

func TestBuildReleasePlanReleaseAs(t *testing.T) {
	t.Parallel()

	for _, releaseAs := range []string{"1.0.0", "v1.0.0"} {
		t.Run(releaseAs, func(t *testing.T) {
			t.Parallel()

			repositoryPath := initRepository(t)
			commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
			runGit(t, repositoryPath, "tag", "-a", "v0.4.0", "-m", "Release v0.4.0")
			commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): stabilize the API")

			plan, err := BuildReleasePlan(Options{Repository: repositoryPath, ReleaseAs: releaseAs})
			if err != nil {
				t.Fatalf("BuildReleasePlan returned error: %v", err)
			}
			if plan.NextTag != "v1.0.0" {
				t.Fatalf("unexpected next tag: %s", plan.NextTag)
			}
		})
	}
}

func TestBuildReleasePlanRejectsReleaseAs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options Options
	}{
		{name: "below current", options: Options{ReleaseAs: "0.3.0"}},
		{name: "equal to current", options: Options{ReleaseAs: "0.4.0"}},
		{name: "not a version", options: Options{ReleaseAs: "1.0"}},
		{name: "with pre", options: Options{ReleaseAs: "1.0.0", PreRelease: "rc"}},
		{name: "with promote", options: Options{ReleaseAs: "1.0.0", Promote: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repositoryPath := initRepository(t)
			commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
			runGit(t, repositoryPath, "tag", "-a", "v0.4.0", "-m", "Release v0.4.0")
			commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")

			options := test.options
			options.Repository = repositoryPath
			plan, err := BuildReleasePlan(options)
			if err == nil {
				t.Fatalf("expected an error, got %s", plan.NextTag)
			}
		})
	}
}

func TestBuildReleasePlanReportsIgnoredTags(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	runGit(t, repositoryPath, "tag", "-a", "v01.02.03", "-m", "Not a semantic version")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if len(plan.IgnoredTags) != 1 || plan.IgnoredTags[0].Tag != "v01.02.03" || plan.IgnoredTags[0].Reason == "" {
		t.Fatalf("unexpected ignored tags: %#v", plan.IgnoredTags)
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
	arguments := []string{"commit", "-m", subject}
	if body != "" {
		arguments = append(arguments, "-m", body)
	}
	runGit(t, repositoryPath, arguments...)
}

func runGit(t *testing.T, repositoryPath string, arguments ...string) {
	t.Helper()

	command := exec.Command("git", arguments...)
	command.Dir = repositoryPath
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", arguments, err, string(output))
	}
}
