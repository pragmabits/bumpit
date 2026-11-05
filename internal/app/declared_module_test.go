package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestBuildReleasePlanForDeclaredModule covers a module with no go.mod: its
// own tags and commits, the paths it also reads, and the root module, which
// no longer reads it.
func TestBuildReleasePlanForDeclaredModule(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	homepage := []ModuleDeclaration{declare("web/homepage", "homepage/v", []string{"web/brand"})}

	plan := planOf(t, Options{Repository: repositoryPath, Module: "web/homepage", Modules: homepage})
	if plan.NextTag != "homepage/v0.1.0" || plan.Name != "web/homepage" || !slices.Equal(plan.Directories, []string{"web/homepage"}) || plan.Module != "" {
		t.Fatalf("unexpected first release of the homepage: next %s, name %q, directories %v, module %q", plan.NextTag, plan.Name, plan.Directories, plan.Module)
	}
	unprefixed := []ModuleDeclaration{declare("web/homepage", "", nil)}
	if plan := planOf(t, Options{Repository: repositoryPath, Module: "web/homepage", Modules: unprefixed}); plan.NextTag != "web/homepage/v0.1.0" {
		t.Fatalf("default tag prefix gave %s, want web/homepage/v0.1.0", plan.NextTag)
	}

	root := planOf(t, Options{Repository: repositoryPath, Modules: homepage})
	if root.HasRelease || !slices.Equal(root.LeavesOut, []string{"web/homepage"}) {
		t.Fatalf("a commit in the homepage released the root module: next %s, leaves out %q", root.NextTag, root.LeavesOut)
	}
	if undeclared := planOf(t, Options{Repository: repositoryPath}); undeclared.NextTag != "v0.2.0" {
		t.Fatalf("without the declaration the root module reads the homepage: got %s, want v0.2.0", undeclared.NextTag)
	}

	runGit(t, repositoryPath, "tag", "-a", "homepage/v0.1.0", "-m", "Release homepage/v0.1.0")
	commitFile(t, repositoryPath, "web/brand/logo.svg", "sharper", "fix: sharpen the logo")
	plan = planOf(t, Options{Repository: repositoryPath, Module: "web/homepage", Modules: homepage})
	if plan.NextTag != "homepage/v0.1.1" || commitVia(plan, "fix: sharpen the logo") != "web/brand" {
		t.Fatalf("a fix in web/brand gave the homepage %s, via %q", plan.NextTag, commitVia(plan, "fix: sharpen the logo"))
	}
	if root := planOf(t, Options{Repository: repositoryPath, Modules: homepage}); root.NextTag != "v0.1.1" || commitVia(root, "fix: sharpen the logo") != "" {
		t.Fatalf("the root module, which owns web/brand, gave %s", root.NextTag)
	}
}

// TestBuildReleasePlanForShippedModule covers a module whose release carries
// a declared one: it reads the commits of the declared module, and a breaking
// change there is minor for it.
func TestBuildReleasePlanForShippedModule(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	shipped := []ModuleDeclaration{declare("web/homepage", "homepage/v", []string{"web/brand"}, ".")}

	root := planOf(t, Options{Repository: repositoryPath, Modules: shipped})
	if root.NextTag != "v0.2.0" || commitVia(root, "feat: add the homepage") != "web/homepage" || !slices.Equal(root.Ships, []string{"web/homepage"}) {
		t.Fatalf("the root module that ships the homepage gave %s, via %q, ships %q", root.NextTag, commitVia(root, "feat: add the homepage"), root.Ships)
	}

	runGit(t, repositoryPath, "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	runGit(t, repositoryPath, "tag", "-a", "homepage/v1.0.0", "-m", "Release homepage/v1.0.0")
	commitFile(t, repositoryPath, "web/homepage/index.html", "moved", "feat!: move every page to a new address")

	root = planOf(t, Options{Repository: repositoryPath, Modules: shipped})
	decision := commitDecision(root, "feat!: move every page to a new address")
	if root.NextTag != "v1.1.0" || decision.Bump != "minor" || decision.Breaking || decision.Reason != "breaking change marker in commit header, of a module this one ships: minor here" || decision.Via != "web/homepage" {
		t.Fatalf("a breaking change of the homepage gave the root module %s: %+v", root.NextTag, decision)
	}
	if homepage := planOf(t, Options{Repository: repositoryPath, Module: "web/homepage", Modules: shipped}); homepage.NextTag != "homepage/v2.0.0" {
		t.Fatalf("a breaking change of the homepage at v1 gave it %s, want homepage/v2.0.0", homepage.NextTag)
	}
}

// TestBuildReleasePlanShipsTransitively covers a module carried by a module
// that is carried in turn.
func TestBuildReleasePlanShipsTransitively(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	chain := []ModuleDeclaration{
		declare("web/brand", "brand/v", nil, "web/homepage"),
		declare("web/homepage", "homepage/v", nil, "."),
	}
	commitFile(t, repositoryPath, "web/brand/logo.svg", "sharper", "fix: sharpen the logo")

	root := planOf(t, Options{Repository: repositoryPath, Modules: chain})
	if commitVia(root, "fix: sharpen the logo") != "web/brand" {
		t.Fatalf("the root module does not read the brand through the homepage: %+v", root.Analyses)
	}

}

// TestBuildReleasePlanOwnsNestedDirectories covers the module that owns a
// file: the deepest one, Go or declared.
func TestBuildReleasePlanOwnsNestedDirectories(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	runGit(t, repositoryPath, "tag", "-a", "web/v0.1.0", "-m", "Release web/v0.1.0")
	commitGoMod(t, repositoryPath, "web/wasm", "example.com/repository/web/wasm", "feat(wasm): add the module")
	commitNewFile(t, repositoryPath, "cmd/tool/assets/icon.svg", "icon", "feat: draw an icon")
	commitGoMod(t, repositoryPath, "cmd/tool", "example.com/repository/cmd/tool", "feat(tool): add the module")
	runGit(t, repositoryPath, "tag", "-a", "cmd/tool/v0.1.0", "-m", "Release cmd/tool/v0.1.0")
	runGit(t, repositoryPath, "tag", "-a", "cmd/tool/assets/v0.1.0", "-m", "Release cmd/tool/assets/v0.1.0")
	runGit(t, repositoryPath, "tag", "-a", "web/wasm/v0.1.0", "-m", "Release web/wasm/v0.1.0")
	runGit(t, repositoryPath, "tag", "-a", "web/v0.1.1", "-m", "Release web/v0.1.1")
	commitFile(t, repositoryPath, "web/homepage/index.html", "new", "fix: fix the homepage")
	commitFile(t, repositoryPath, "web/wasm/main.go", "package main", "fix(wasm): fix the module")
	commitFile(t, repositoryPath, "cmd/tool/assets/icon.svg", "new", "fix: redraw the icon")

	nested := []ModuleDeclaration{
		declare("web", "", nil),
		declare("web/homepage", "", nil),
		declare("cmd/tool/assets", "", nil),
	}
	if web := planOf(t, Options{Repository: repositoryPath, Module: "web", Modules: nested}); web.HasRelease {
		t.Fatalf("commits in modules nested in web released it: %+v", web.Analyses)
	}
	if tool := planOf(t, Options{Repository: repositoryPath, Module: "cmd/tool", Modules: nested}); tool.HasRelease {
		t.Fatalf("a commit in the declared module nested in cmd/tool released it: %+v", tool.Analyses)
	}

	reader := []ModuleDeclaration{declare("web", "", []string{"web/wasm"}), declare("web/homepage", "", nil), declare("cmd/tool/assets", "", nil)}
	web := planOf(t, Options{Repository: repositoryPath, Module: "web", Modules: reader})
	if commitVia(web, "fix(wasm): fix the module") != "web/wasm" || commitDecision(web, "fix: fix the homepage").Subject != "" {
		t.Fatalf("web, which lists web/wasm in its paths, read %+v", web.Analyses)
	}
}

// TestBuildReleasePlanQueriesEachPart covers a side branch whose change to
// the homepage a merge discarded: git leaves it out of the homepage's own
// history, and reading the paths of the homepage must not bring it back.
func TestBuildReleasePlanQueriesEachPart(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	runGit(t, repositoryPath, "tag", "-a", "homepage/v0.1.0", "-m", "Release homepage/v0.1.0")
	runGit(t, repositoryPath, "checkout", "-q", "-b", "side")
	commitFile(t, repositoryPath, "web/homepage/index.html", "rewritten", "feat!: rewrite the homepage")
	commitFile(t, repositoryPath, "web/brand/logo.svg", "sharper", "fix: sharpen the logo")
	runGit(t, repositoryPath, "checkout", "-q", "-")
	runGit(t, repositoryPath, "merge", "-q", "--no-ff", "--no-commit", "side")
	runGit(t, repositoryPath, "checkout", "HEAD", "--", "web/homepage/index.html")
	runGit(t, repositoryPath, "commit", "-q", "-m", "Merge side, keeping the homepage")

	homepage := []ModuleDeclaration{declare("web/homepage", "homepage/v", []string{"web/brand"})}
	plan := planOf(t, Options{Repository: repositoryPath, Module: "web/homepage", Modules: homepage})
	if plan.NextTag != "homepage/v0.1.1" || commitDecision(plan, "feat!: rewrite the homepage").Subject != "" {
		t.Fatalf("the homepage read a change the merge discarded: next %s, %+v", plan.NextTag, plan.Analyses)
	}
}

// TestBuildReleasePlanReadsBothSidesOfARename covers a file moved from the
// root module into a declared one: the commit changes both.
func TestBuildReleasePlanReadsBothSidesOfARename(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	runGit(t, repositoryPath, "tag", "-a", "homepage/v0.1.0", "-m", "Release homepage/v0.1.0")
	commitNewFile(t, repositoryPath, "assets/banner.svg", "banner", "chore: add a banner")
	runGit(t, repositoryPath, "tag", "-a", "v0.2.0", "-m", "Release v0.2.0")
	runGit(t, repositoryPath, "mv", "assets/banner.svg", "web/homepage/banner.svg")
	runGit(t, repositoryPath, "commit", "-q", "-m", "fix: move the banner to the homepage")

	homepage := []ModuleDeclaration{declare("web/homepage", "homepage/v", nil)}
	if root := planOf(t, Options{Repository: repositoryPath, Modules: homepage}); root.NextTag != "v0.2.1" {
		t.Fatalf("the root module, which lost the banner, gave %s", root.NextTag)
	}
	if plan := planOf(t, Options{Repository: repositoryPath, Module: "web/homepage", Modules: homepage}); plan.NextTag != "homepage/v0.1.1" {
		t.Fatalf("the homepage, which gained the banner, gave %s", plan.NextTag)
	}
}

// initHomepageRepository creates a repository whose root module
// example.com/repository is released at v0.1.0, with a brand and a homepage
// under web, and a feature of the homepage after the release.
func initHomepageRepository(t *testing.T) string {
	t.Helper()

	repositoryPath := initRepository(t)
	commitGoMod(t, repositoryPath, ".", "example.com/repository", "feat: initial library")
	commitNewFile(t, repositoryPath, "web/brand/logo.svg", "logo", "chore: draw the logo")
	runGit(t, repositoryPath, "tag", "-a", "v0.1.0", "-m", "Release v0.1.0")
	commitNewFile(t, repositoryPath, "web/homepage/index.html", "homepage", "feat: add the homepage")
	return repositoryPath
}

// commitNewFile is commitFile for a file whose directories may not exist.
func commitNewFile(t *testing.T, repositoryPath, name, content, subject string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(repositoryPath, filepath.Dir(name)), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	commitFile(t, repositoryPath, name, content, subject)
}

// declare is the declaration of the module in directory, shipped by the
// modules in carriers.
func declare(directory, tagPrefix string, paths []string, carriers ...string) ModuleDeclaration {
	shippedBy := append([]string{}, carriers...)
	return ModuleDeclaration{Directories: []string{directory}, TagPrefix: tagPrefix, Paths: paths, ShippedBy: &shippedBy}
}

func planOf(t *testing.T, opts Options) ReleasePlan {
	t.Helper()

	plan, err := BuildReleasePlan(opts)
	if err != nil {
		t.Fatalf("BuildReleasePlan(%+v) returned error: %v", opts, err)
	}
	return plan
}

// commitDecision is the decision of the plan on the commit with subject, the
// zero one when the plan did not read it.
func commitDecision(plan ReleasePlan, subject string) CommitDecision {
	index := slices.IndexFunc(plan.Analyses, func(decision CommitDecision) bool { return decision.Subject == subject })
	if index < 0 {
		return CommitDecision{}
	}
	return plan.Analyses[index]
}

func commitVia(plan ReleasePlan, subject string) string {
	return commitDecision(plan, subject).Via
}

// TestListModulesListsDeclaredModules covers a declared module among Go
// modules: listed in the order of its directory, with no module path, and
// leaving the holds of the Go modules as they are.
func TestListModulesListsDeclaredModules(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "docs/index.md", "docs", "feat: write the docs")
	commitFile(t, repositoryPath, "library.txt", "library", "feat: add a helper")
	commitFile(t, repositoryPath, "tool/main.txt", "main", "feat(tool): add a flag")

	releases, err := ListModules(Options{Repository: repositoryPath, Modules: []ModuleDeclaration{declare("docs", "", nil)}})
	if err != nil {
		t.Fatalf("ListModules returned error: %v", err)
	}

	labels := make([]string, 0, len(releases))
	for _, release := range releases {
		labels = append(labels, release.label())
	}
	if !slices.Equal(labels, []string{".", "docs", "tool"}) {
		t.Fatalf("modules listed as %q", labels)
	}
	if docs := releases[1]; !slices.Equal(docs.Directories, []string{"docs"}) || docs.Directory != "" || docs.Path != "" || docs.NextTag != "docs/v0.1.0" || docs.Hold != nil {
		t.Fatalf("unexpected release of the declared module: %+v", docs)
	}
	if root := releases[0]; root.NextTag != "v0.2.0" || root.Directory != "." || root.Directories != nil || root.Name != "" {
		t.Fatalf("unexpected release of the root module: %+v", root)
	}
	if tool := releases[2]; tool.Hold == nil || tool.Hold.Requirements[0].Directory != "." {
		t.Fatalf("the hold of the tool changed: %+v", tool.Hold)
	}
}

// TestBuildReleasePlanForDeclaredRoot covers a repository with no go.mod
// whose root bumpit.yaml declares, beside a module nested in it: the root
// module owns every file but the nested module's, and is the default module.
func TestBuildReleasePlanForDeclaredRoot(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitNewFile(t, repositoryPath, "site/index.html", "page", "feat: add the first page")
	commitNewFile(t, repositoryPath, "docs/guide.md", "guide", "docs: write the guide")
	runGit(t, repositoryPath, "tag", "-a", "v0.1.0", "-m", "Release v0.1.0")
	commitFile(t, repositoryPath, "site/index.html", "second", "feat: add the second page")
	declarations := []ModuleDeclaration{declare(".", "", []string{"site/index.html"}), declare("site", "", nil)}

	if root := planOf(t, Options{Repository: repositoryPath, Modules: declarations}); root.NextTag != "v0.2.0" || commitVia(root, "feat: add the second page") != "site/index.html" {
		t.Fatalf("the declared root module, which reads site/index.html, gave %s: %+v", root.NextTag, root.Analyses)
	}
	declarations[0] = declare(".", "", nil)
	if root := planOf(t, Options{Repository: repositoryPath, Modules: declarations}); root.HasRelease || !slices.Equal(root.LeavesOut, []string{"site"}) {
		t.Fatalf("a commit in site released the declared root module as %s", root.NextTag)
	}
	if site := planOf(t, Options{Repository: repositoryPath, Module: "site", Modules: declarations}); site.NextTag != "site/v0.1.0" {
		t.Fatalf("the module in site gave %s, want site/v0.1.0", site.NextTag)
	}
}

// TestBuildReleasePlanReadsGoCodeInDeclaredDirectory covers Go code of a Go
// module that lives in a declared directory: it is the Go module's own code,
// read with its own level, while the rest of the directory reaches the Go
// module only through shippedBy.
func TestBuildReleasePlanReadsGoCodeInDeclaredDirectory(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitGoMod(t, repositoryPath, ".", "example.com/repository", "feat: initial library")
	commitNewFile(t, repositoryPath, "web/handler.go", "package web\n", "feat: serve the site")
	commitNewFile(t, repositoryPath, "web/index.html", "page", "feat: add the page")
	runGit(t, repositoryPath, "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	runGit(t, repositoryPath, "tag", "-a", "web/v1.0.0", "-m", "Release web/v1.0.0")

	for _, carriers := range [][]string{nil, {"."}} {
		repository := cloneRepository(t, repositoryPath)
		declarations := []ModuleDeclaration{declare("web", "", nil, carriers...)}
		commitFile(t, repository, "web/index.html", "layout", "feat!: change the layout")
		root := planOf(t, Options{Repository: repository, Modules: declarations})
		want := map[bool]string{false: "", true: "v1.1.0"}[len(carriers) > 0]
		if root.NextTag != want {
			t.Fatalf("shipped by %q, a breaking change of the page gave the root module %q, want %q", carriers, root.NextTag, want)
		}

		commitFile(t, repository, "web/handler.go", "package web\n\nfunc Serve() {}\n", "feat!: remove the handler")
		_, err := BuildReleasePlan(Options{Repository: repository, Modules: declarations})
		if err == nil || !strings.Contains(err.Error(), "cannot release v2.0.0") {
			t.Fatalf("shipped by %q, a breaking change of the root module's Go code in web gave %v, want the v2.0.0 refusal", carriers, err)
		}
		if web := planOf(t, Options{Repository: repository, Module: "web", Modules: declarations}); web.NextTag != "web/v2.0.0" {
			t.Fatalf("shipped by %q, web gave %s, want web/v2.0.0", carriers, web.NextTag)
		}
	}
}

// cloneRepository copies the repository at repositoryPath into a new
// directory, its tags included.
func cloneRepository(t *testing.T, repositoryPath string) string {
	t.Helper()

	clone := t.TempDir()
	runGit(t, clone, "clone", "-q", repositoryPath, ".")
	runGit(t, clone, "config", "user.name", "bumpit")
	runGit(t, clone, "config", "user.email", "bumpit@example.com")
	return clone
}

// TestBuildReleasePlanLeavesOutDeclaredTagsWithoutRootModule covers the plan
// of a repository with no root module, which reads tags by v* alone: the
// versions of a declared module whose prefix starts with v are not its tags.
func TestBuildReleasePlanLeavesOutDeclaredTagsWithoutRootModule(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitNewFile(t, repositoryPath, "vite/index.html", "page", "feat: add the page")
	runGit(t, repositoryPath, "tag", "-a", "vite/v0.1.0", "-m", "Release vite/v0.1.0")
	commitFile(t, repositoryPath, "vite/index.html", "second", "fix: fix the page")
	declarations := []ModuleDeclaration{declare("vite", "", nil)}

	if plan := planOf(t, Options{Repository: repositoryPath, Modules: declarations}); plan.NextTag != "v0.1.0" || len(plan.IgnoredTags) != 0 {
		t.Fatalf("the plan with no root module gave %s, ignored %v", plan.NextTag, plan.IgnoredTags)
	}
	latest, err := FindLatestTag(LatestOptions{Repository: repositoryPath, Modules: declarations})
	if err != nil || latest.Found {
		t.Fatalf("FindLatestTag with no root module = %+v, %v; want no tag", latest, err)
	}
}

// TestBuildReleasePlanListsWhatItLeavesOut covers the declared modules a
// module does not read: not one it reads through its paths, and one nested
// in a module it ships, which that module's directory leaves out.
func TestBuildReleasePlanListsWhatItLeavesOut(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitGoMod(t, repositoryPath, ".", "example.com/repository", "feat: initial library")
	commitNewFile(t, repositoryPath, "web/index.html", "page", "feat: add the page")
	commitNewFile(t, repositoryPath, "web/brand/logo.svg", "logo", "feat: draw the logo")
	commitNewFile(t, repositoryPath, "web/app/index.html", "app", "feat: add the app")

	reader := []ModuleDeclaration{declare("web", "", []string{"web/brand"}), declare("web/brand", "", nil), declare("web/app", "", nil)}
	if web := planOf(t, Options{Repository: repositoryPath, Module: "web", Modules: reader}); !slices.Equal(web.LeavesOut, []string{"web/app"}) {
		t.Fatalf("web, which reads web/brand through its paths, leaves out %q, want only web/app", web.LeavesOut)
	}

	shipped := []ModuleDeclaration{declare("web", "", nil, "."), declare("web/app", "", nil)}
	root := planOf(t, Options{Repository: repositoryPath, Modules: shipped})
	if !slices.Equal(root.Ships, []string{"web"}) || !slices.Equal(root.LeavesOut, []string{"web/app"}) {
		t.Fatalf("the root module ships %q and leaves out %q, want web and web/app", root.Ships, root.LeavesOut)
	}
}

// TestBuildReleasePlanContinuesDeclaredPreRelease covers a prerelease of a
// module read with several queries: whether a releasable commit came after
// the current tag, through its directory or through a path.
func TestBuildReleasePlanContinuesDeclaredPreRelease(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	homepage := []ModuleDeclaration{declare("web/homepage", "homepage/v", []string{"web/brand"})}
	runGit(t, repositoryPath, "tag", "-a", "homepage/v1.0.0", "-m", "Release homepage/v1.0.0")
	commitFile(t, repositoryPath, "web/homepage/index.html", "second", "feat: add a section")
	runGit(t, repositoryPath, "tag", "-a", "homepage/v1.1.0-beta.1", "-m", "Release homepage/v1.1.0-beta.1")

	if plan := planOf(t, Options{Repository: repositoryPath, Module: "web/homepage", Modules: homepage}); plan.HasRelease {
		t.Fatalf("no commit after the prerelease gave %s", plan.NextTag)
	}
	commitFile(t, repositoryPath, "web/brand/logo.svg", "sharper", "fix: sharpen the logo")
	if plan := planOf(t, Options{Repository: repositoryPath, Module: "web/homepage", Modules: homepage}); plan.NextTag != "homepage/v1.1.0-beta.2" {
		t.Fatalf("a fix through a path after the prerelease gave %s, want homepage/v1.1.0-beta.2", plan.NextTag)
	}
}

// TestBuildReleasePlanLeavesOutGoFilesNoBuildCompiles covers the .go files of
// a declared directory that no build compiles, a file marked //go:build
// ignore and a file under testdata: they are no code of the Go module, which
// does not read them as its own, and explain names the declared directories
// whose Go files it reads.
func TestBuildReleasePlanLeavesOutGoFilesNoBuildCompiles(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitGoMod(t, repositoryPath, ".", "example.com/repository", "feat: initial library")
	commitNewFile(t, repositoryPath, "web/handler.go", "package web\n", "feat: serve the site")
	commitNewFile(t, repositoryPath, "web/serve.go", "//go:build ignore\n\npackage main\n", "feat: add a development server")
	commitNewFile(t, repositoryPath, "web/testdata/fixture.go", "package fixture\n", "test: add a fixture")
	runGit(t, repositoryPath, "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	commitFile(t, repositoryPath, "web/serve.go", "//go:build ignore\n\npackage main\n\nfunc main() {}\n", "feat!: rework the development server")
	commitFile(t, repositoryPath, "web/testdata/fixture.go", "package fixture\n\nvar x = 1\n", "fix: fix the fixture")

	declarations := []ModuleDeclaration{declare("web", "", nil)}
	root := planOf(t, Options{Repository: repositoryPath, Modules: declarations})
	if root.HasRelease || !slices.Equal(root.GoFilesIn, []string{"web"}) {
		t.Fatalf("files no build compiles released the root module as %s; Go files in %q", root.NextTag, root.GoFilesIn)
	}
}

// TestBuildReleasePlanReadsDerivedPathsLiterally covers a derived path whose
// name holds a glob character: git reads it as the name it is.
func TestBuildReleasePlanReadsDerivedPathsLiterally(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitNewFile(t, repositoryPath, "web/site/package.json", `{"name": "site", "dependencies": {"legacy": "file:../[legacy]"}}`, "feat: add the site")
	commitNewFile(t, repositoryPath, "web/[legacy]/package.json", `{"name": "legacy"}`, "feat: add the legacy package")
	commitNewFile(t, repositoryPath, "web/l", "notes", "docs: write notes")
	runGit(t, repositoryPath, "tag", "-a", "web/site/v0.1.0", "-m", "Release web/site/v0.1.0")
	commitFile(t, repositoryPath, "web/l", "more", "fix: fix the notes")

	declarations := []ModuleDeclaration{declare("web/site", "", nil)}
	if site := planOf(t, Options{Repository: repositoryPath, Module: "web/site", Modules: declarations}); site.HasRelease {
		t.Fatalf("a commit to web/l, which the glob web/[legacy] would match, released the site as %s", site.NextTag)
	}
}
