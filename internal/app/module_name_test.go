package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/pragmabits/bumpit/internal/gitx"
)

// named is the declaration of a module with a name of its own, shipped by the
// modules in carriers.
func named(name string, directories []string, carriers ...string) ModuleDeclaration {
	shippedBy := append([]string{}, carriers...)
	return ModuleDeclaration{Name: name, Directories: directories, ShippedBy: &shippedBy}
}

// TestModuleNameNamesTheModule covers a module with a name of its own: it takes
// its tag prefix from the name, and --module finds it by the name or by its
// directory.
func TestModuleNameNamesTheModule(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	site := []ModuleDeclaration{named("site", []string{"web/homepage"})}

	for _, written := range []string{"site", "web/homepage"} {
		plan := planOf(t, Options{Repository: repositoryPath, Module: written, Modules: site})
		if plan.NextTag != "site/v0.1.0" || plan.Name != "site" {
			t.Errorf("--module %s: next %s, name %q", written, plan.NextTag, plan.Name)
		}
	}

	prefixed := []ModuleDeclaration{{Name: "site", Directories: []string{"web/homepage"}, TagPrefix: "homepage/v", ShippedBy: &[]string{}}}
	if plan := planOf(t, Options{Repository: repositoryPath, Module: "site", Modules: prefixed}); plan.NextTag != "homepage/v0.1.0" {
		t.Errorf("a tag prefix beside a name gave %s", plan.NextTag)
	}

	_, err := BuildReleasePlan(Options{Repository: repositoryPath, Module: "missing", Modules: site})
	if err == nil || !strings.Contains(err.Error(), "no module missing") {
		t.Errorf("a module that does not exist: error = %v", err)
	}
}

// TestModuleNameIsWhatShippedByCalls covers a carrier with a name of its own:
// shippedBy calls it by that name, and its directory is no longer what names
// it.
func TestModuleNameIsWhatShippedByCalls(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	runGit(t, repositoryPath, "tag", "-a", "site/v0.1.0", "-m", "Release site/v0.1.0")
	commitFile(t, repositoryPath, "web/brand/logo.svg", "sharper", "fix: sharpen the logo")

	declarations := []ModuleDeclaration{
		named("site", []string{"web/homepage"}, "."),
		declare("web/brand", "brand/v", nil, "site"),
	}
	plan := planOf(t, Options{Repository: repositoryPath, Module: "site", Modules: declarations})
	if plan.NextTag != "site/v0.1.1" || !slices.Equal(plan.Ships, []string{"web/brand"}) {
		t.Fatalf("the carrier gave %s, ships %v", plan.NextTag, plan.Ships)
	}
	if root := planOf(t, Options{Repository: repositoryPath, Modules: declarations}); !slices.Equal(root.Ships, []string{"web/brand", "site"}) {
		t.Errorf("the root module ships %v", root.Ships)
	}

	byDirectory := []ModuleDeclaration{
		named("site", []string{"web/homepage"}),
		declare("web/brand", "brand/v", nil, "web/homepage"),
	}
	_, _, err := discoverRepositoryModules(gitx.New(repositoryPath), byDirectory)
	if err == nil || !strings.Contains(err.Error(), "shippedBy lists web/homepage, which is no module of the repository") {
		t.Errorf("a carrier called by its directory: error = %v", err)
	}
}

// TestDiscoveryRefusesANameThatIsADirectory covers the rule the text of a
// declaration cannot tell: a name cannot be the directory of another module,
// which the commands could not tell from it.
func TestDiscoveryRefusesANameThatIsADirectory(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/homepage/index.html", "homepage", "feat: add the homepage")
	commitNewFile(t, repositoryPath, "docs/guide.md", "guide", "docs: write the guide")

	testCases := []struct {
		name         string
		declarations []ModuleDeclaration
	}{
		{name: "a Go module with no entry", declarations: []ModuleDeclaration{named("tool", []string{"web/homepage"})}},
		{name: "a declared module", declarations: []ModuleDeclaration{named("docs", []string{"web/homepage"}), declare("docs", "docs/v", nil)}},
		{name: "a name with a slash", declarations: []ModuleDeclaration{named("web/homepage", []string{"docs"}), declare("web/homepage", "site/v", nil)}},
	}
	for _, testCase := range testCases {
		_, _, err := discoverRepositoryModules(gitx.New(repositoryPath), testCase.declarations)
		if err == nil || !strings.Contains(err.Error(), "is the directory of another module") {
			t.Errorf("%s: error = %v", testCase.name, err)
		}
	}

	own := []ModuleDeclaration{named("docs", []string{"docs"})}
	if _, _, err := discoverRepositoryModules(gitx.New(repositoryPath), own); err != nil {
		t.Errorf("a name that is the module's own directory: error = %v", err)
	}
}

func TestLoadConfigRefusesModuleNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		declaration string
		message     string
	}{
		{name: "the root", declaration: "name: .\n    directories: [web]\n    shippedBy: []", message: `name "." is a directory`},
		{name: "unclean", declaration: "name: ./site\n    directories: [web]\n    shippedBy: []", message: `name "./site" is not clean`},
		{name: "absolute", declaration: "name: /site\n    directories: [web]\n    shippedBy: []", message: `name "/site" is absolute`},
		{name: "outside", declaration: "name: ../site\n    directories: [web]\n    shippedBy: []", message: `name "../site" leaves the repository`},
		{name: "glob", declaration: "name: \"site*\"\n    directories: [web]\n    shippedBy: []", message: `name "site*" holds a glob character`},
		{name: "several directories and no name", declaration: "directories: [web, docs]\n    shippedBy: []", message: "modules: web, docs: several directories need a name"},
		{name: "shipped by its own name", declaration: "name: site\n    directories: [web]\n    shippedBy: [site]", message: "modules: site: shippedBy lists the module itself"},
		{name: "shipped by its own directory", declaration: "name: site\n    directories: [web]\n    shippedBy: [web]", message: "modules: site: shippedBy lists the module itself"},
	}
	for _, testCase := range cases {
		_, err := loadConfigText(t, "modules:\n  - "+testCase.declaration+"\n")
		if err == nil || !strings.Contains(err.Error(), testCase.message) {
			t.Errorf("%s: LoadConfig error = %v, want one containing %q", testCase.name, err, testCase.message)
		}
	}

	_, err := loadConfigText(t, "modules:\n  - name: site\n    directories: [web]\n    shippedBy: []\n  - name: site\n    directories: [docs]\n    shippedBy: []\n")
	if err == nil || !strings.Contains(err.Error(), "modules: site: the name is given twice") {
		t.Errorf("a name given twice: LoadConfig error = %v", err)
	}
}
