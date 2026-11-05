package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pragmabits/bumpit/internal/gitx"
)

// TestRepositoryFindingsLeavesOutAModuleWithNoDirectory covers a declaration that
// is nowhere in the repository: nothing can be said of it there.
func TestRepositoryFindingsLeavesOutAModuleWithNoDirectory(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	declarations := []ModuleDeclaration{{Name: "site", ShippedBy: &[]string{}}}
	report, err := repositoryFindings(newRepositorySnapshot(gitx.New(repositoryPath)), declarations)
	if err != nil || len(report.findings) != 0 || len(report.modules) != 2 {
		t.Errorf("repositoryFindings = %+v, %v, want the two Go modules and no finding", report, err)
	}
}

// TestDiscoveryRefusesModuleDeclarations covers what the repository must
// confirm of a declaration.
func TestDiscoveryRefusesModuleDeclarations(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	commitGoMod(t, repositoryPath, "tool", "example.com/repository/tool", "feat(tool): add the tool")
	commitNewFile(t, repositoryPath, "web/homepage/admin/index.html", "admin", "feat: add the admin page")

	cases := []struct {
		name         string
		declarations []ModuleDeclaration
		message      string
	}{
		{name: "a Go module", declarations: []ModuleDeclaration{declare("tool", "", nil)}, message: "modules: tool: the directory holds a go.mod, so it is a Go module: it takes only directories and name"},
		{name: "an untracked directory", declarations: []ModuleDeclaration{declare("docs", "", nil)}, message: "modules: docs: docs matches no tracked file"},
		{name: "an untracked path", declarations: []ModuleDeclaration{declare("web/homepage", "", []string{"deploy/homepage"})}, message: "modules: web/homepage: deploy/homepage matches no tracked file"},
		{name: "a path inside its directory", declarations: []ModuleDeclaration{declare("web/homepage", "", []string{"web/homepage/admin"})}, message: "modules: web/homepage: paths lists web/homepage/admin, inside its own directory: the module reads it already"},
		{name: "a carrier that is no module", declarations: []ModuleDeclaration{declare("web/homepage", "", nil, "web")}, message: "modules: web/homepage: shippedBy lists web, which is no module of the repository"},
		{name: "the prefix of a Go module", declarations: []ModuleDeclaration{declare("web/homepage", "tool/v", nil)}, message: "modules: web/homepage: tag prefix tool/v is taken by another module: it is the one of tool too, so their tags could not be told apart"},
		{name: "a prefix continued by a version", declarations: []ModuleDeclaration{declare("web/homepage", "v1", nil)}, message: "modules: web/homepage: tag prefix v1 continues v: v is the one of ., and what follows it can start a version, so a tag could be a version of both"},
		{name: "an invalid tag name", declarations: []ModuleDeclaration{declare("web/homepage", "home..page/v", nil)}, message: "modules: web/homepage: tag prefix home..page/v does not make a valid tag name"},
		{name: "a declared . beside a root go.mod", declarations: []ModuleDeclaration{declare(".", "", nil)}, message: "modules: .: the directory holds a go.mod"},
		{name: "a tag prefix git takes for an option", declarations: []ModuleDeclaration{declare("web/homepage", "-site/v", nil)}, message: "modules: web/homepage: tag prefix -site/v starts with -: git takes it for an option"},
		{name: "a cycle of carriers", declarations: []ModuleDeclaration{declare("web/brand", "brand/v", nil, "web/homepage"), declare("web/homepage", "", nil, "web/brand")}, message: "modules: web/brand: shippedBy forms a cycle: through web/homepage, the module carries itself"},
	}
	for _, testCase := range cases {
		_, err := BuildReleasePlan(Options{Repository: repositoryPath, Modules: testCase.declarations})
		if err == nil || !strings.Contains(err.Error(), testCase.message) {
			t.Errorf("%s: BuildReleasePlan error = %v, want one containing %q", testCase.name, err, testCase.message)
		}
	}

	nested := []ModuleDeclaration{declare("web/homepage", "", []string{"tool"}), declare("web/homepage/admin", "", nil)}
	if _, err := BuildReleasePlan(Options{Repository: repositoryPath, Modules: nested}); err != nil {
		t.Errorf("a path naming a Go module, and a module nested in another, were refused: %v", err)
	}
}

// TestDiscoveryChecksGoCodeShipsDeclaredModule covers a Go module that ships
// a declared module through Go code: a Go file inside it, or a //go:embed
// pattern that reaches it.
func TestDiscoveryChecksGoCodeShipsDeclaredModule(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		files        map[string]string
		declarations []ModuleDeclaration
		message      string
	}{
		{
			name:         "a Go file inside, which is the Go module's own code",
			files:        map[string]string{"web/homepage/serve.go": "package homepage\n"},
			declarations: []ModuleDeclaration{declare("web/homepage", "", nil)},
		},
		{
			name:         "an embed from a package inside",
			files:        map[string]string{"web/homepage/serve.go": "package homepage\n\nimport \"embed\"\n\n//go:embed dist\nvar site embed.FS\n"},
			declarations: []ModuleDeclaration{declare("web/homepage", "", nil)},
			message:      "modules: web/homepage: web/homepage/serve.go:5:12 embeds dist: the module . then ships it; add . to its shippedBy",
		},
		{
			name:         "an embed from the root",
			files:        map[string]string{"site.go": embedSource("web/homepage/dist")},
			declarations: []ModuleDeclaration{declare("web/homepage", "", nil)},
			message:      "modules: web/homepage: site.go:5:12 embeds web/homepage/dist: the module . then ships it; add . to its shippedBy",
		},
		{
			name:         "an embed that misses",
			files:        map[string]string{"site.go": embedSource("web/*.css")},
			declarations: []ModuleDeclaration{declare("web/homepage", "", nil)},
		},
		{
			name:         "a test file",
			files:        map[string]string{"web/homepage/serve_test.go": "package homepage\n"},
			declarations: []ModuleDeclaration{declare("web/homepage", "", nil)},
		},
		{
			name:         "declared as shipped",
			files:        map[string]string{"site.go": embedSource("web/homepage/dist"), "web/homepage/serve.go": "package homepage\n"},
			declarations: []ModuleDeclaration{declare("web/homepage", "", nil, ".")},
		},
		{
			name:         "an embed into the nested module only",
			files:        map[string]string{"site.go": embedSource("web/homepage/dist")},
			declarations: []ModuleDeclaration{declare("web", "", nil), declare("web/homepage", "", nil, ".")},
		},
		{
			name:         "shipped through a carrier",
			files:        map[string]string{"site.go": embedSource("web/homepage/dist")},
			declarations: []ModuleDeclaration{declare("web", "", nil, "."), declare("web/homepage", "", nil, "web")},
		},
	}
	for _, testCase := range cases {
		repositoryPath := initHomepageRepository(t)
		for name, content := range testCase.files {
			commitNewFile(t, repositoryPath, name, content, "chore: add "+name)
		}

		_, err := BuildReleasePlan(Options{Repository: repositoryPath, Modules: testCase.declarations})
		switch {
		case testCase.message == "" && err != nil:
			t.Errorf("%s: BuildReleasePlan returned error: %v", testCase.name, err)
		case testCase.message != "" && (err == nil || !strings.Contains(err.Error(), testCase.message)):
			t.Errorf("%s: BuildReleasePlan error = %v, want one containing %q", testCase.name, err, testCase.message)
		}
	}
}

// embedSource is a Go file of the root package that embeds pattern, at line
// 5 and column 12.
func embedSource(pattern string) string {
	return "package repository\n\nimport \"embed\"\n\n//go:embed " + pattern + "\nvar site embed.FS\n"
}

// TestDiscoveryReadsGoFilesNoBuildCompiles covers Go files that no build
// compiles, which embed nothing and must not stop the discovery: a template
// of a generator, a file marked //go:build ignore, and a tracked file deleted
// from the working tree.
func TestDiscoveryReadsGoFilesNoBuildCompiles(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	commitNewFile(t, repositoryPath, "template.go", "package {{.Package}}\n", "chore: add a template")
	commitNewFile(t, repositoryPath, "web/homepage/serve.go", "//go:build ignore\n\npackage main\n\nimport \"embed\"\n\n//go:embed dist\nvar site embed.FS\n", "chore: add a development server")
	commitNewFile(t, repositoryPath, "extra.go", "package repository\n", "chore: add a file")
	if err := os.Remove(filepath.Join(repositoryPath, "extra.go")); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	declarations := []ModuleDeclaration{declare("web/homepage", "", nil)}
	if _, err := BuildReleasePlan(Options{Repository: repositoryPath, AllowDirty: true, Modules: declarations}); err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}
}

// TestDiscoveryStopsEmbedsAtNestedGoModules covers a //go:embed of a
// directory that holds a nested Go module: the go command does not embed
// the nested module's files, so the declared module inside it is not
// shipped by the embedding module.
func TestDiscoveryStopsEmbedsAtNestedGoModules(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	commitNewFile(t, repositoryPath, "cmd/templates/page.html", "page", "feat: add a template")
	commitNewFile(t, repositoryPath, "main.go", embedSource("cmd"), "feat: embed the templates")
	commitGoMod(t, repositoryPath, "cmd/tool", "example.com/repository/cmd/tool", "feat(tool): add the tool")
	commitNewFile(t, repositoryPath, "cmd/tool/ui/index.html", "ui", "feat(tool): add the interface")

	declarations := []ModuleDeclaration{declare("cmd/tool/ui", "", nil, "cmd/tool")}
	if _, err := BuildReleasePlan(Options{Repository: repositoryPath, Modules: declarations}); err != nil {
		t.Fatalf("an embed that stops at the nested Go module was refused: %v", err)
	}
}

// TestDiscoverRepositoryModulesNamesTheMissingCarrierFirst refuses the
// declaration an earlier Go file ships before a later Go file it cannot
// read stops the check.
func TestDiscoverRepositoryModulesNamesTheMissingCarrierFirst(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("root reads a directory whatever its mode")
	}

	repositoryPath := initRepository(t)
	commitGoMod(t, repositoryPath, ".", "example.com/repository", "feat: initial library")
	commitNewFile(t, repositoryPath, "web/homepage/index.html", "homepage", "feat: add the homepage")
	commitFile(t, repositoryPath, "site.go", "package repository\n\nimport _ \"embed\"\n\n//go:embed web/homepage/index.html\nvar page string\n", "feat: serve the homepage")
	commitNewFile(t, repositoryPath, "web/homepage/zz/late.go", "package zz\n", "feat: add a late package")
	locked := filepath.Join(repositoryPath, "web", "homepage", "zz")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("Chmod returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(locked, 0o755); err != nil {
			t.Errorf("Chmod returned error: %v", err)
		}
	})

	_, _, err := discoverRepositoryModules(gitx.New(repositoryPath), []ModuleDeclaration{declare("web/homepage", "", nil)})
	if err == nil || !strings.Contains(err.Error(), "add . to its shippedBy") {
		t.Fatalf("discoverRepositoryModules returned %v, want the missing carrier", err)
	}
}
