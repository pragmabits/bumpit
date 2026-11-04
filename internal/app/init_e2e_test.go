package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// initCandidateRepository is a repository with a root Go module that embeds
// web/homepage, a line of homepage/v tags that no module owns and two
// directories it may be, a Helm chart with a subchart, and a public and a
// private Node.js package.
func initCandidateRepository(t *testing.T) string {
	t.Helper()

	repositoryPath := initHomepageRepository(t)
	commitNewFile(t, repositoryPath, "deploy/homepage/values.yaml", "replicas: 1\n", "chore: deploy the homepage")
	commitFile(t, repositoryPath, "site.go", "package repository\n\nimport _ \"embed\"\n\n//go:embed web/homepage/index.html\nvar page string\n", "feat: serve the homepage")
	runGit(t, repositoryPath, "tag", "-a", "homepage/v0.1.0", "-m", "homepage v0.1.0")
	commitNewFile(t, repositoryPath, "deploy/chart/Chart.yaml", "apiVersion: v2\nname: chart\nversion: 1.4.0\n", "chore: chart the deploy")
	commitNewFile(t, repositoryPath, "deploy/chart/charts/sub/Chart.yaml", "apiVersion: v2\nname: sub\nversion: 0.1.0\n", "chore: add a subchart")
	commitNewFile(t, repositoryPath, "web/widget/package.json", `{"name": "widget", "version": "2.0.0"}`, "feat: add the widget")
	commitNewFile(t, repositoryPath, "web/private/package.json", `{"name": "private", "version": "1.0.0", "private": true}`, "feat: add a private package")
	runGit(t, repositoryPath, "tag", "-a", "homepage/v0.2.0", "-m", "homepage v0.2.0")
	return repositoryPath
}

// TestCLIEndToEndInit creates bumpit.yaml, lists the candidate modules with
// the commands that declare them, and runs one of those commands.
func TestCLIEndToEndInit(t *testing.T) {
	t.Parallel()

	repositoryPath := initCandidateRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	binaryPath := buildCLI(t)

	expectContains(t, binaryPath, []string{"init", "-r", repositoryPath},
		"created "+configPath,
		"homepage/v: 2 tags, the newest homepage/v0.2.0",
		"bumpit modules add deploy/homepage -r "+repositoryPath+" -T homepage/v -B",
		"bumpit modules add web/homepage -r "+repositoryPath+" -T homepage/v -b .",
		"site.go:5:12 embeds web/homepage/index.html",
		"deploy/chart/Chart.yaml: version 1.4.0",
		"bumpit modules add deploy/chart -r "+repositoryPath+" -B",
		"web/widget/package.json: version 2.0.0",
		"bumpit modules add web/widget -r "+repositoryPath+" -B",
		"bumpit sees only //go:embed",
		"commit the file",
	)
	output, _ := runCLI(binaryPath, "init", "-r", repositoryPath)
	for _, unwanted := range []string{"created", "charts/sub", "web/private", "commit the file"} {
		if strings.Contains(output, unwanted) {
			t.Errorf("the second init printed %q:\n%s", unwanted, output)
		}
	}
	if !strings.Contains(output, configPath+" exists already") {
		t.Errorf("the second init does not say the config exists:\n%s", output)
	}
	expectFile(t, configPath, initialConfig)

	web := filepath.Join(repositoryPath, "web")
	expectContains(t, binaryPath, []string{"init", "-r", web}, "bumpit modules add homepage -r "+web+" -T homepage/v -b ..", "../site.go:5:12 embeds web/homepage/index.html")
	expectContains(t, binaryPath, []string{"modules", "add", "homepage", "-r", web, "-T", "homepage/v", "-b", ".."}, "added web/homepage")
	output, err := runCLI(binaryPath, "init", "-r", repositoryPath)
	if err != nil || strings.Contains(output, "homepage/v:") {
		t.Errorf("init lists the tags of a declared module: %v\n%s", err, output)
	}
}

func TestCLIEndToEndInitJSON(t *testing.T) {
	t.Parallel()

	repositoryPath := initCandidateRepository(t)
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "init", "-r", repositoryPath, "-o", "json")
	if err != nil {
		t.Fatalf("init returned error: %v\n%s", err, output)
	}
	var report InitReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v\n%s", err, output)
	}
	if !report.Created || report.Config != filepath.Join(repositoryPath, "bumpit.yaml") {
		t.Errorf("report = %+v, want the created config", report)
	}
	if len(report.TagLines) != 1 || len(report.TagLines[0].Candidates) != 2 {
		t.Fatalf("tag lines = %+v, want homepage/v with two candidates", report.TagLines)
	}
	homepage := report.TagLines[0].Candidates[1]
	if homepage.Directory != "web/homepage" || homepage.TagPrefix != "homepage/v" || len(homepage.ShippedBy) != 1 || homepage.ShippedBy[0] != "." {
		t.Errorf("candidate = %+v, want web/homepage shipped by .", homepage)
	}
	if len(homepage.Embeds) != 1 || homepage.Embeds[0].Place != "site.go:5:12" || homepage.Embeds[0].Pattern != "web/homepage/index.html" {
		t.Errorf("embeds = %+v, want site.go:5:12", homepage.Embeds)
	}
	if len(report.Manifests) != 2 || report.Manifests[0].Manifest != "deploy/chart/Chart.yaml" || report.Manifests[1].Version != "2.0.0" {
		t.Errorf("manifests = %+v, want the chart and the widget", report.Manifests)
	}
}

func TestCLIEndToEndInitOutsideARepository(t *testing.T) {
	t.Parallel()

	expectFailure(t, buildCLI(t), []string{"init", "-r", t.TempDir()}, "no git repository")
}

// TestCLIEndToEndInitEdges proposes nothing a run owns already or that
// bumpit modules add would refuse, and carries on past a chart it cannot
// read.
func TestCLIEndToEndInitEdges(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "bumpit.yaml", "tagMatch: release-*\n", "chore: configure bumpit")
	runGit(t, repositoryPath, "tag", "-a", "release-1.0.0", "-m", "release")
	runGit(t, repositoryPath, "tag", "-a", "1.0.0", "-m", "bare")
	if err := os.MkdirAll(filepath.Join(repositoryPath, "-old", "legacy"), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	writeFile(t, filepath.Join(repositoryPath, "-old", "legacy", "index.html"), "legacy")
	runGit(t, repositoryPath, "add", "--", "-old/legacy/index.html")
	runGit(t, repositoryPath, "commit", "-m", "feat: keep the legacy site")
	runGit(t, repositoryPath, "tag", "-a", "legacy/v1.0.0", "-m", "legacy")
	commitNewFile(t, repositoryPath, ".github/actions/setup/action.yml", "name: setup\n", "ci: add the setup action")
	runGit(t, repositoryPath, "tag", "-a", "setup/v1.0.0", "-m", "setup")
	commitNewFile(t, repositoryPath, "web/[legacy]/package.json", `{"name": "glob", "version": "1.0.0"}`, "feat: add a package")
	commitNewFile(t, repositoryPath, "web/string/package.json", `{"name": "string", "version": "1.0.0", "private": "true"}`, "feat: add a private package")
	commitNewFile(t, repositoryPath, "testdata/broken/Chart.yaml", "version: [\n", "test: add a broken chart")
	commitNewFile(t, repositoryPath, "deploy/bad/Chart.yaml", "version: [\n", "chore: add a chart")
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "init", "-r", repositoryPath)
	if err != nil {
		t.Fatalf("init returned error: %v\n%s", err, output)
	}
	for _, wanted := range []string{
		"(no prefix): 1 tag, the newest 1.0.0",
		"bumpit modules add ./-old/legacy -r " + repositoryPath + " -T legacy/v -B",
		"bumpit modules add .github/actions/setup -r " + repositoryPath + " -T setup/v -B",
		"web/[legacy]/package.json: version 1.0.0",
		"bumpit modules add would refuse web/[legacy]",
		"glob character",
		"could not read deploy/bad/Chart.yaml",
	} {
		if !strings.Contains(output, wanted) {
			t.Errorf("init output lacks %q:\n%s", wanted, output)
		}
	}
	for _, unwanted := range []string{"release-:", "web/string", "testdata"} {
		if strings.Contains(output, unwanted) {
			t.Errorf("init output holds %q:\n%s", unwanted, output)
		}
	}
	expectContains(t, binaryPath, []string{"modules", "add", "./-old/legacy", "-r", repositoryPath, "-T", "legacy/v", "-B"}, "added -old/legacy")
}
