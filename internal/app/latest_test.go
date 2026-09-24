package app

import "testing"

func TestFindLatestTagIgnoresLexicographicOrder(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.9.0", "-m", "Release v1.9.0")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat: another feature")
	runGit(t, repositoryPath, "tag", "-a", "v1.10.0", "-m", "Release v1.10.0")

	latest, err := FindLatestTag(LatestOptions{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("FindLatestTag returned error: %v", err)
	}

	if !latest.Found {
		t.Fatalf("expected a tag to be found")
	}
	if latest.Tag != "v1.10.0" {
		t.Fatalf("unexpected latest tag: %s", latest.Tag)
	}
	if latest.Version != "1.10.0" {
		t.Fatalf("unexpected latest version: %s", latest.Version)
	}
}

func TestFindLatestTagIgnoresCreationOrder(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v2.0.0", "-m", "Release v2.0.0")
	runGit(t, repositoryPath, "tag", "-a", "v1.4.0", "-m", "Backport tag created later")

	latest, err := FindLatestTag(LatestOptions{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("FindLatestTag returned error: %v", err)
	}

	if latest.Tag != "v2.0.0" {
		t.Fatalf("unexpected latest tag: %s", latest.Tag)
	}
}

func TestFindLatestTagPrefersStableOverPreRelease(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	runGit(t, repositoryPath, "tag", "-a", "v1.0.0-rc.2", "-m", "Release v1.0.0-rc.2")

	latest, err := FindLatestTag(LatestOptions{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("FindLatestTag returned error: %v", err)
	}

	if latest.Tag != "v1.0.0" {
		t.Fatalf("unexpected latest tag: %s", latest.Tag)
	}
}

func TestFindLatestTagSkipsNonSemverTags(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.2.3", "-m", "Release v1.2.3")
	runGit(t, repositoryPath, "tag", "-a", "release-latest", "-m", "Marker tag")

	latest, err := FindLatestTag(LatestOptions{Repository: repositoryPath, TagMatch: "*"})
	if err != nil {
		t.Fatalf("FindLatestTag returned error: %v", err)
	}

	if latest.Tag != "v1.2.3" {
		t.Fatalf("unexpected latest tag: %s", latest.Tag)
	}
}

func TestFindLatestTagWithCustomPrefix(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "release-1.0.0", "-m", "Release 1.0.0")
	runGit(t, repositoryPath, "tag", "-a", "release-1.2.0", "-m", "Release 1.2.0")

	latest, err := FindLatestTag(LatestOptions{Repository: repositoryPath, TagMatch: "release-*"})
	if err != nil {
		t.Fatalf("FindLatestTag returned error: %v", err)
	}

	if latest.Tag != "release-1.2.0" || latest.Version != "1.2.0" {
		t.Fatalf("unexpected latest tag: %s (%s)", latest.Tag, latest.Version)
	}
}

func TestFindLatestTagForNestedModule(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v9.0.0", "-m", "Root module")
	runGit(t, repositoryPath, "tag", "-a", "cmd/tool/v0.1.0", "-m", "Tool 0.1.0")
	runGit(t, repositoryPath, "tag", "-a", "cmd/tool/v0.2.0", "-m", "Tool 0.2.0")

	latest, err := FindLatestTag(LatestOptions{Repository: repositoryPath, TagMatch: "cmd/tool/v*"})
	if err != nil {
		t.Fatalf("FindLatestTag returned error: %v", err)
	}

	if latest.Tag != "cmd/tool/v0.2.0" || latest.Version != "0.2.0" {
		t.Fatalf("unexpected latest tag: %s (%s)", latest.Tag, latest.Version)
	}
}

func TestFindLatestTagIgnoresLeadingZeroes(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	runGit(t, repositoryPath, "tag", "-a", "v01.02.03", "-m", "Not a semantic version")

	latest, err := FindLatestTag(LatestOptions{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("FindLatestTag returned error: %v", err)
	}

	if latest.Tag != "v1.0.0" {
		t.Fatalf("unexpected latest tag: %s", latest.Tag)
	}
}

func TestFindLatestTagReportsMissingTag(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")

	latest, err := FindLatestTag(LatestOptions{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("FindLatestTag returned error: %v", err)
	}

	if latest.Found {
		t.Fatalf("expected no tag to be found, got %s", latest.Tag)
	}
}

func TestFindLatestTagIgnoresUnreachableTagsByDefault(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	runGit(t, repositoryPath, "checkout", "-b", "sidebranch")
	commitFile(t, repositoryPath, "side.txt", "side", "feat: side feature")
	runGit(t, repositoryPath, "tag", "-a", "v2.0.0", "-m", "Release v2.0.0")
	runGit(t, repositoryPath, "checkout", "-")

	latest, err := FindLatestTag(LatestOptions{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("FindLatestTag returned error: %v", err)
	}
	if latest.Tag != "v1.0.0" {
		t.Fatalf("unexpected latest tag reachable from HEAD: %s", latest.Tag)
	}

	latestFromAll, err := FindLatestTag(LatestOptions{Repository: repositoryPath, All: true})
	if err != nil {
		t.Fatalf("FindLatestTag returned error: %v", err)
	}
	if latestFromAll.Tag != "v2.0.0" {
		t.Fatalf("unexpected latest tag across the repository: %s", latestFromAll.Tag)
	}
}

func TestBuildReleasePlanUsesHighestSemverTag(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "base.txt", "base", "feat: initial release")
	runGit(t, repositoryPath, "tag", "-a", "v1.10.0", "-m", "Release v1.10.0")
	runGit(t, repositoryPath, "tag", "-a", "v1.9.0", "-m", "Tag created after the newer release")
	commitFile(t, repositoryPath, "feature.txt", "feature", "feat(api): add export endpoint")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	if plan.CurrentTag != "v1.10.0" {
		t.Fatalf("unexpected current tag: %s", plan.CurrentTag)
	}
	if plan.NextTag != "v1.11.0" {
		t.Fatalf("unexpected next tag: %s", plan.NextTag)
	}
}
