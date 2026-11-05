package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/pragmabits/bumpit/internal/gitx"
)

// homepageDirectories are the two directories of the module these tests read.
var homepageDirectories = []string{"web/homepage", "deploy/homepage"}

// initTwoDirectoriesRepository is initHomepageRepository with a second
// directory that belongs to the homepage, released as homepage/v0.1.0.
func initTwoDirectoriesRepository(t *testing.T) string {
	t.Helper()

	repositoryPath := initHomepageRepository(t)
	commitNewFile(t, repositoryPath, "deploy/homepage/nginx.conf", "server", "chore: serve the homepage")
	runGit(t, repositoryPath, "tag", "-a", "homepage/v0.1.0", "-m", "Release homepage/v0.1.0")
	return repositoryPath
}

// TestModuleOwnsEveryDirectory covers a module with two directories: a commit
// in either releases it, and the module around them reads neither.
func TestModuleOwnsEveryDirectory(t *testing.T) {
	t.Parallel()

	repositoryPath := initTwoDirectoriesRepository(t)
	homepage := []ModuleDeclaration{named("homepage", homepageDirectories)}

	commitFile(t, repositoryPath, "deploy/homepage/nginx.conf", "faster", "fix: serve faster")
	plan := planOf(t, Options{Repository: repositoryPath, Module: "homepage", Modules: homepage})
	if plan.NextTag != "homepage/v0.1.1" || commitVia(plan, "fix: serve faster") != "" || !slices.Equal(plan.Directories, homepageDirectories) {
		t.Fatalf("a fix in the second directory: next %s, via %q, directories %v", plan.NextTag, commitVia(plan, "fix: serve faster"), plan.Directories)
	}

	commitFile(t, repositoryPath, "web/homepage/index.html", "bigger", "feat: grow the homepage")
	if plan := planOf(t, Options{Repository: repositoryPath, Module: "deploy/homepage", Modules: homepage}); plan.NextTag != "homepage/v0.2.0" {
		t.Fatalf("a feature in the first directory gave %s", plan.NextTag)
	}

	root := planOf(t, Options{Repository: repositoryPath, Modules: homepage})
	if root.HasRelease || !slices.Equal(root.LeavesOut, homepageDirectories) {
		t.Fatalf("the root module: next %s, leaves out %v", root.NextTag, root.LeavesOut)
	}
}

// TestModuleQueriesEachDirectory covers a side branch whose change to one
// directory a merge discarded while it kept a change to the other: git leaves
// it out of that directory's history only when each has a query of its own.
func TestModuleQueriesEachDirectory(t *testing.T) {
	t.Parallel()

	repositoryPath := initTwoDirectoriesRepository(t)
	runGit(t, repositoryPath, "checkout", "-q", "-b", "side")
	commitFile(t, repositoryPath, "deploy/homepage/nginx.conf", "rewritten", "feat!: rewrite the server")
	commitFile(t, repositoryPath, "web/homepage/index.html", "sharper", "fix: sharpen the homepage")
	runGit(t, repositoryPath, "checkout", "-q", "-")
	runGit(t, repositoryPath, "merge", "-q", "--no-ff", "--no-commit", "side")
	runGit(t, repositoryPath, "checkout", "HEAD", "--", "deploy/homepage/nginx.conf")
	runGit(t, repositoryPath, "commit", "-q", "-m", "Merge side, keeping the server")

	plan := planOf(t, Options{Repository: repositoryPath, Module: "homepage", Modules: []ModuleDeclaration{named("homepage", homepageDirectories)}})
	if plan.NextTag != "homepage/v0.1.1" || commitDecision(plan, "feat!: rewrite the server").Subject != "" {
		t.Fatalf("the module read a change the merge discarded: next %s, %+v", plan.NextTag, plan.Analyses)
	}
}

// TestModuleExcludesWhatIsNestedInEachDirectory covers a module nested in the
// second directory, which owns its files.
func TestModuleExcludesWhatIsNestedInEachDirectory(t *testing.T) {
	t.Parallel()

	repositoryPath := initTwoDirectoriesRepository(t)
	commitNewFile(t, repositoryPath, "deploy/homepage/admin/panel.html", "panel", "chore: add the panel")
	runGit(t, repositoryPath, "tag", "-a", "homepage/v0.1.1", "-m", "Release homepage/v0.1.1")
	commitFile(t, repositoryPath, "deploy/homepage/admin/panel.html", "sharper", "fix: sharpen the panel")

	modules := []ModuleDeclaration{named("homepage", homepageDirectories), declare("deploy/homepage/admin", "admin/v", nil)}
	plan := planOf(t, Options{Repository: repositoryPath, Module: "homepage", Modules: modules})
	if plan.HasRelease || !slices.Equal(plan.LeavesOut, []string{"deploy/homepage/admin"}) {
		t.Fatalf("a commit in a nested module: next %s, leaves out %v", plan.NextTag, plan.LeavesOut)
	}
}

// TestModuleLeavesOutWhatNoPathCovers covers a module around both directories
// of another whose paths name only one of them: what it does not read is the
// other directory.
func TestModuleLeavesOutWhatNoPathCovers(t *testing.T) {
	t.Parallel()

	repositoryPath := initTwoDirectoriesRepository(t)
	around := ModuleDeclaration{Name: "around", Directories: []string{"web", "deploy"}, Paths: []string{"deploy/homepage"}, ShippedBy: &[]string{}}
	modules := []ModuleDeclaration{named("homepage", homepageDirectories), around}
	plan := planOf(t, Options{Repository: repositoryPath, Module: "around", Modules: modules})
	if !slices.Equal(plan.LeavesOut, []string{"web/homepage"}) {
		t.Fatalf("leaves out %v, want the directory no path covers", plan.LeavesOut)
	}
}

// TestModuleGivesEachGoModuleItsCode covers two directories with different Go
// modules around them: each Go module reads the .go files of the directory it
// is the nearest module around.
func TestModuleGivesEachGoModuleItsCode(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/homepage/serve.go", "package homepage\n", "feat: serve the homepage")
	commitNewFile(t, repositoryPath, "tool/assets/assets.go", "package assets\n", "feat: add the assets")
	modules := []ModuleDeclaration{named("site", []string{"web/homepage", "tool/assets"})}

	root := planOf(t, Options{Repository: repositoryPath, Modules: modules})
	if !slices.Equal(root.GoFilesIn, []string{"web/homepage"}) || root.NextTag != "v0.2.0" {
		t.Errorf("the root module: go files in %v, next %s", root.GoFilesIn, root.NextTag)
	}
	tool := planOf(t, Options{Repository: repositoryPath, Module: "tool", Modules: modules})
	if !slices.Equal(tool.GoFilesIn, []string{"tool/assets"}) || tool.NextTag != "tool/v0.2.0" {
		t.Errorf("the tool module: go files in %v, next %s", tool.GoFilesIn, tool.NextTag)
	}
}

// TestModuleIsShippedThroughAnyDirectory covers //go:embed patterns that reach
// one directory of a module: each Go module they lie in must carry it.
func TestModuleIsShippedThroughAnyDirectory(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/homepage/index.html", "homepage", "feat: add the homepage")
	commitNewFile(t, repositoryPath, "tool/assets/logo.svg", "logo", "feat: add the logo")
	commitFile(t, repositoryPath, "site.go", "package repository\n\nimport _ \"embed\"\n\n//go:embed web/homepage/index.html\nvar page string\n", "feat: serve the homepage")
	commitFile(t, repositoryPath, "tool/logo.go", "package tool\n\nimport _ \"embed\"\n\n//go:embed assets/logo.svg\nvar logo string\n", "feat: embed the logo")
	client := gitx.New(repositoryPath)
	directories := []string{"web/homepage", "tool/assets"}

	if _, _, err := discoverRepositoryModules(client, []ModuleDeclaration{named("site", directories, ".")}); err == nil || !strings.Contains(err.Error(), "modules: site: tool/logo.go:5:12 embeds assets/logo.svg: the module tool then ships it; add tool to its shippedBy") {
		t.Errorf("the second directory shipped by another Go module: error = %v", err)
	}
	if _, _, err := discoverRepositoryModules(client, []ModuleDeclaration{named("site", directories, "tool")}); err == nil || !strings.Contains(err.Error(), "add . to its shippedBy") {
		t.Errorf("the first directory shipped by the root module: error = %v", err)
	}
	if _, _, err := discoverRepositoryModules(client, []ModuleDeclaration{named("site", directories, ".", "tool")}); err != nil {
		t.Errorf("both carriers named: error = %v", err)
	}
}

// TestModuleReadsThePackagesOfEachDirectory covers the Node packages of a
// module: what the package.json of any directory reads outside the module, and
// not one directory of the module because another depends on it.
func TestModuleReadsThePackagesOfEachDirectory(t *testing.T) {
	t.Parallel()

	repositoryPath := initTwoDirectoriesRepository(t)
	commitNewFile(t, repositoryPath, "web/brand/package.json", `{"name": "brand"}`, "chore: package the brand")
	commitNewFile(t, repositoryPath, "web/homepage/package.json", `{"name": "homepage", "dependencies": {"server": "file:../../deploy/homepage"}}`, "chore: package the homepage")
	commitNewFile(t, repositoryPath, "deploy/homepage/package.json", `{"name": "server", "dependencies": {"brand": "file:../../web/brand"}}`, "chore: package the server")
	modules := []ModuleDeclaration{named("homepage", homepageDirectories)}

	plan := planOf(t, Options{Repository: repositoryPath, Module: "homepage", Modules: modules})
	if !slices.Equal(plan.DerivedPaths, []string{"web/brand"}) {
		t.Fatalf("derived paths %v, want the brand and neither directory of the module", plan.DerivedPaths)
	}

	commitFile(t, repositoryPath, "deploy/homepage/package.json", `{"name": `, "chore: break the package")
	_, err := BuildReleasePlan(Options{Repository: repositoryPath, Module: "homepage", Modules: modules})
	if err == nil || !strings.HasPrefix(err.Error(), "homepage: read deploy/homepage/package.json:") {
		t.Fatalf("an unreadable package in the second directory: error = %v", err)
	}
}

func TestDiscoveryRefusesSeveralDirectories(t *testing.T) {
	t.Parallel()

	repositoryPath := initTwoDirectoriesRepository(t)
	client := gitx.New(repositoryPath)

	testCases := []struct {
		name         string
		declarations []ModuleDeclaration
		message      string
	}{
		{name: "a directory no file lies in", declarations: []ModuleDeclaration{named("site", []string{"web/homepage", "deploy/missing"})}, message: "modules: site: deploy/missing matches no tracked file"},
		{name: "a path inside the second directory", declarations: []ModuleDeclaration{{Name: "site", Directories: homepageDirectories, Paths: []string{"deploy/homepage/nginx.conf"}, ShippedBy: &[]string{}}}, message: "paths lists deploy/homepage/nginx.conf, inside its own directory"},
	}
	for _, testCase := range testCases {
		_, _, err := discoverRepositoryModules(client, testCase.declarations)
		if err == nil || !strings.Contains(err.Error(), testCase.message) {
			t.Errorf("%s: error = %v, want one containing %q", testCase.name, err, testCase.message)
		}
	}
}

func TestLoadConfigRefusesSeveralDirectories(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
		message string
	}{
		{name: "the same directory in two modules", content: "  - name: a\n    directories: [web, docs]\n    shippedBy: []\n  - name: b\n    directories: [docs, deploy]\n    shippedBy: []\n", message: "docs is declared twice"},
		{name: "the same directory twice in one", content: "  - name: a\n    directories: [web, web]\n    shippedBy: []\n", message: "modules: a: web is listed twice"},
		{name: "a directory inside another", content: "  - name: a\n    directories: [web, web/homepage]\n    shippedBy: []\n", message: "modules: a: web/homepage lies in web: the module owns it already"},
		{name: "the root beside another", content: "  - name: a\n    directories: [., web]\n    shippedBy: []\n", message: "modules: a: web lies in .: the module owns it already"},
		{name: "an own directory in paths", content: "  - name: a\n    directories: [web, docs]\n    paths: [docs]\n    shippedBy: []\n", message: "modules: a: paths lists docs, its own directory"},
	}
	for _, testCase := range cases {
		_, err := loadConfigText(t, "modules:\n"+testCase.content)
		if err == nil || !strings.Contains(err.Error(), testCase.message) {
			t.Errorf("%s: LoadConfig error = %v, want one containing %q", testCase.name, err, testCase.message)
		}
	}
}
