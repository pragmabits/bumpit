package app

import (
	"strings"
	"testing"

	"github.com/pragmabits/bumpit/internal/gitx"
)

// goEntry is the entry bumpit.yaml gives the Go module in directory, with a
// name when name is not empty.
func goEntry(name, directory string) ModuleDeclaration {
	return ModuleDeclaration{Name: name, Directories: []string{directory}}
}

// TestGoEntryComplementsTheGoModule covers an entry on a directory that holds a
// go.mod: the module stays the one bumpit found, its tags and its checks with
// it, and the entry gives it a name.
func TestGoEntryComplementsTheGoModule(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitFile(t, repositoryPath, "tool/main.txt", "main", "feat(tool): add a flag")
	entries := []ModuleDeclaration{goEntry("", "."), goEntry("stack", "tool")}

	releases, err := ListModules(Options{Repository: repositoryPath, Modules: entries})
	if err != nil {
		t.Fatalf("ListModules returned error: %v", err)
	}
	if len(releases) != 2 || releases[0].Path != "example.com/repository" || releases[1].Path != "example.com/repository/tool" {
		t.Fatalf("modules listed as %+v", releases)
	}

	for _, written := range []string{"stack", "tool"} {
		plan := planOf(t, Options{Repository: repositoryPath, Module: written, Modules: entries})
		if plan.NextTag != "tool/v0.2.0" || plan.Name != "stack" || plan.Module != "example.com/repository/tool" {
			t.Errorf("--module %s: next %s, name %q, module %q", written, plan.NextTag, plan.Name, plan.Module)
		}
	}
	if plan := planOf(t, Options{Repository: repositoryPath, Modules: entries}); plan.Name != "" || plan.Module != "example.com/repository" {
		t.Errorf("the root module: name %q, module %q", plan.Name, plan.Module)
	}
}

// TestGoEntryNameLeavesTheTagPrefix covers a Go module in a major version
// directory: its tags keep the prefix the go command reads, whatever its name.
func TestGoEntryNameLeavesTheTagPrefix(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitGoMod(t, repositoryPath, "sub/v2", "example.com/repository/sub/v2", "feat: add the module")
	runGit(t, repositoryPath, "tag", "-a", "sub/v2.0.0", "-m", "Release sub/v2.0.0")
	commitNewFile(t, repositoryPath, "sub/v2/notes.txt", "notes", "fix: write the notes")

	plan := planOf(t, Options{Repository: repositoryPath, Module: "versioned", Modules: []ModuleDeclaration{goEntry("versioned", "sub/v2")}})
	if plan.NextTag != "sub/v2.0.1" {
		t.Errorf("a named Go module gave %s, want sub/v2.0.1", plan.NextTag)
	}
}

// TestShippedByCallsANamedGoModule covers a carrier that is a Go module with a
// name: //go:embed asks for the name, and shippedBy gives it.
func TestShippedByCallsANamedGoModule(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/homepage/index.html", "homepage", "feat: add the homepage")
	commitFile(t, repositoryPath, "site.go", "package repository\n\nimport _ \"embed\"\n\n//go:embed web/homepage/index.html\nvar page string\n", "feat: serve the homepage")
	client := gitx.New(repositoryPath)

	unshipped := []ModuleDeclaration{goEntry("platform", "."), declare("web/homepage", "", nil)}
	if _, _, err := discoverRepositoryModules(client, unshipped); err == nil || !strings.Contains(err.Error(), "add platform to its shippedBy") {
		t.Errorf("an embed of a named Go module: error = %v", err)
	}
	shipped := []ModuleDeclaration{goEntry("platform", "."), declare("web/homepage", "", nil, "platform")}
	if _, _, err := discoverRepositoryModules(client, shipped); err != nil {
		t.Errorf("a carrier called by its name: error = %v", err)
	}
	byDirectory := []ModuleDeclaration{goEntry("platform", "."), declare("web/homepage", "", nil, ".")}
	if _, _, err := discoverRepositoryModules(client, byDirectory); err == nil || !strings.Contains(err.Error(), "shippedBy lists ., which is no module of the repository") {
		t.Errorf("a carrier called by the directory of a named module: error = %v", err)
	}
}

func TestDiscoveryRefusesGoEntries(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "docs/guide.md", "guide", "docs: write the guide")
	client := gitx.New(repositoryPath)

	prefixed := goEntry("", "tool")
	prefixed.TagPrefix = "tools/v"
	withPaths := goEntry("", "tool")
	withPaths.Paths = []string{"docs"}
	testCases := []struct {
		name         string
		declarations []ModuleDeclaration
		message      string
	}{
		{name: "a tag prefix", declarations: []ModuleDeclaration{prefixed}, message: "modules: tool: the directory holds a go.mod, so it is a Go module: it takes only directories and name"},
		{name: "paths", declarations: []ModuleDeclaration{withPaths}, message: "modules: tool: the directory holds a go.mod"},
		{name: "shippedBy", declarations: []ModuleDeclaration{declare("tool", "", nil)}, message: "modules: tool: the directory holds a go.mod"},
		{name: "several directories", declarations: []ModuleDeclaration{named("site", []string{"tool", "docs"})}, message: "modules: site: tool holds a go.mod: a Go module has one directory"},
		{name: "a name that is the directory of another Go module", declarations: []ModuleDeclaration{goEntry("tool", ".")}, message: "modules: tool: the name is the directory of another module"},
		{name: "no go.mod and no shippedBy", declarations: []ModuleDeclaration{goEntry("", "docs")}, message: "modules: docs: shippedBy is required: bumpit cannot always tell whether the release of another module carries it; list the modules that carry it, or write shippedBy: []"},
		{name: "the same directory twice", declarations: []ModuleDeclaration{goEntry("", "tool"), goEntry("", "tool")}, message: "tool is declared twice"},
	}
	for _, testCase := range testCases {
		_, _, err := discoverRepositoryModules(client, testCase.declarations)
		if err == nil || !strings.Contains(err.Error(), testCase.message) {
			t.Errorf("%s: error = %v, want one containing %q", testCase.name, err, testCase.message)
		}
	}
}

// TestDiscoveryLeavesGoModulesWithNoEntry covers a go.mod bumpit.yaml does not
// list: it is a module all the same, beside the entries of the others.
func TestDiscoveryLeavesGoModulesWithNoEntry(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	modules, _, err := discoverRepositoryModules(gitx.New(repositoryPath), []ModuleDeclaration{goEntry("platform", ".")})
	if err != nil {
		t.Fatalf("discoverRepositoryModules returned error: %v", err)
	}
	if len(modules) != 2 || modules[0].name != "platform" || modules[1].name != "tool" {
		t.Fatalf("modules = %+v", modules)
	}
}

// TestRunWithATagPatternLeavesTheModulesUnread covers the runs that read tags
// by pattern alone: they never discover the modules, so a declaration that
// lacks shippedBy, which only the repository can tell, does not stop them.
func TestRunWithATagPatternLeavesTheModulesUnread(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "docs/guide.md", "guide", "docs: write the guide")
	modules := []ModuleDeclaration{goEntry("", "docs")}

	if _, err := BuildReleasePlan(Options{Repository: repositoryPath, TagMatch: "v*", Modules: modules}); err != nil {
		t.Errorf("a run with a tag pattern: error = %v", err)
	}
	if _, err := BuildReleasePlan(Options{Repository: repositoryPath, Modules: modules}); err == nil || !strings.Contains(err.Error(), "shippedBy is required") {
		t.Errorf("a run that discovers the modules: error = %v", err)
	}
}
