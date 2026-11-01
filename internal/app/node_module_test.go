package app

import (
	"slices"
	"strings"
	"testing"
)

// TestBuildReleasePlanReadsNodePackages covers a declared module that is a
// member of a Node workspace: it reads the members it depends on and the
// files of the root without listing them in its paths, and a module that
// ships it reads them too.
func TestBuildReleasePlanReadsNodePackages(t *testing.T) {
	t.Parallel()

	repositoryPath := initNodeWorkspace(t)
	homepage := []ModuleDeclaration{declare("web/homepage", "homepage/v", nil)}
	plan := planOf(t, Options{Repository: repositoryPath, Module: "web/homepage", Modules: homepage})
	wantDerived := []string{"web/brand", "web/package.json", "web/pnpm-lock.yaml", "web/pnpm-workspace.yaml"}
	if !slices.Equal(plan.DerivedPaths, wantDerived) {
		t.Fatalf("the homepage derives %q, want %q", plan.DerivedPaths, wantDerived)
	}

	runGit(t, repositoryPath, "tag", "-a", "homepage/v0.1.0", "-m", "Release homepage/v0.1.0")
	commitFile(t, repositoryPath, "web/brand/logo.svg", "sharper", "fix: sharpen the logo")
	plan = planOf(t, Options{Repository: repositoryPath, Module: "web/homepage", Modules: homepage})
	if plan.NextTag != "homepage/v0.1.1" || commitVia(plan, "fix: sharpen the logo") != "web/brand" {
		t.Fatalf("a fix in the brand gave the homepage %s, via %q", plan.NextTag, commitVia(plan, "fix: sharpen the logo"))
	}

	carried := []ModuleDeclaration{declare("deploy", "", nil), declare("web/homepage", "homepage/v", nil, "deploy")}
	runGit(t, repositoryPath, "tag", "-a", "deploy/v0.1.0", "-m", "Release deploy/v0.1.0")
	commitFile(t, repositoryPath, "web/brand/logo.svg", "sharpest", "fix: sharpen the logo again")
	if deploy := planOf(t, Options{Repository: repositoryPath, Module: "deploy", Modules: carried}); deploy.NextTag != "deploy/v0.1.1" || commitVia(deploy, "fix: sharpen the logo again") != "web/homepage" {
		t.Fatalf("the module that ships the homepage gave %s: %+v", deploy.NextTag, deploy.Analyses)
	}
}

// TestBuildReleasePlanLeavesOutWhatNodeDoesNotRead covers a declared module
// nested in another one: listed as left out unless the outer module reads it
// through its package.json.
func TestBuildReleasePlanLeavesOutWhatNodeDoesNotRead(t *testing.T) {
	t.Parallel()

	repositoryPath := initNodeWorkspace(t)
	commitFile(t, repositoryPath, "web/package.json", `{"name": "web", "private": true, "dependencies": {"@orvelis/brand": "workspace:*"}}`, "chore: depend on the brand")
	declarations := []ModuleDeclaration{declare("web", "", nil), declare("web/brand", "", nil), declare("web/homepage", "", nil)}

	web := planOf(t, Options{Repository: repositoryPath, Module: "web", Modules: declarations})
	if !slices.Equal(web.DerivedPaths, []string{"web/brand"}) || !slices.Equal(web.LeavesOut, []string{"web/homepage"}) {
		t.Fatalf("web derives %q and leaves out %q, want web/brand and web/homepage", web.DerivedPaths, web.LeavesOut)
	}
}

// TestListModulesReportsUnreadablePackage covers a package.json that cannot
// be read: the plan of its module fails, and the others are listed.
func TestListModulesReportsUnreadablePackage(t *testing.T) {
	t.Parallel()

	repositoryPath := initNodeWorkspace(t)
	commitFile(t, repositoryPath, "web/homepage/package.json", `{"name": `, "chore: break the package")

	releases, err := ListModules(Options{Repository: repositoryPath, Modules: []ModuleDeclaration{declare("web/homepage", "", nil)}})
	if err != nil {
		t.Fatalf("ListModules returned error: %v", err)
	}
	if len(releases) != 2 || releases[0].Error != "" || !strings.HasPrefix(releases[1].Error, "web/homepage: read web/homepage/package.json:") {
		t.Fatalf("unexpected releases: %+v", releases)
	}
}

// initNodeWorkspace creates a repository whose root module
// example.com/repository holds a pnpm workspace in web: a brand, an app and
// a homepage that depends on the brand.
func initNodeWorkspace(t *testing.T) string {
	t.Helper()

	repositoryPath := initRepository(t)
	commitGoMod(t, repositoryPath, ".", "example.com/repository", "feat: initial library")
	commitNewFile(t, repositoryPath, "web/package.json", `{"name": "web", "private": true}`, "chore: add the workspace")
	commitNewFile(t, repositoryPath, "web/pnpm-workspace.yaml", "packages:\n  - app\n  - brand\n  - homepage\n", "chore: list the packages")
	commitNewFile(t, repositoryPath, "web/pnpm-lock.yaml", "lockfileVersion: '9.0'\n", "chore: lock the packages")
	commitNewFile(t, repositoryPath, "web/brand/package.json", `{"name": "@orvelis/brand", "private": true}`, "feat: add the brand")
	commitNewFile(t, repositoryPath, "web/brand/logo.svg", "logo", "feat: draw the logo")
	commitNewFile(t, repositoryPath, "web/app/package.json", `{"name": "@orvelis/app", "private": true}`, "feat: add the app")
	commitNewFile(t, repositoryPath, "web/homepage/package.json", `{"name": "@orvelis/homepage", "private": true, "dependencies": {"@orvelis/brand": "workspace:*"}}`, "feat: add the homepage")
	commitNewFile(t, repositoryPath, "deploy/Dockerfile", "FROM nginx", "feat: package the homepage")
	return repositoryPath
}
