package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildReleasePlanScopesRootModuleCommits(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitFile(t, repositoryPath, "tool/main.txt", "main", "feat!: reshape the tool")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}
	if plan.HasRelease {
		t.Fatalf("a commit in the nested module released the root module as %s", plan.NextTag)
	}

	commitFile(t, repositoryPath, "library.txt", "library", "fix: handle empty input")
	plan, err = BuildReleasePlan(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}
	if plan.NextTag != "v0.1.1" || plan.Module != "example.com/repository" {
		t.Fatalf("unexpected plan for the root module: next %s, module %q", plan.NextTag, plan.Module)
	}
}

func TestBuildReleasePlanForNestedModule(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitFile(t, repositoryPath, "library.txt", "library", "feat!: reshape the library")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath, Module: "tool"})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}
	if plan.HasRelease {
		t.Fatalf("a commit in the root module released the tool as %s", plan.NextTag)
	}

	commitFile(t, repositoryPath, "tool/main.txt", "main", "feat(tool): add a flag")
	plan, err = BuildReleasePlan(Options{Repository: repositoryPath, Module: "tool"})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}
	if plan.NextTag != "tool/v0.2.0" {
		t.Fatalf("unexpected next tag for the tool: %s", plan.NextTag)
	}
}

func TestBuildReleasePlanChecksMajorVersionSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		modulePath string
		subject    string
		want       string
	}{
		{name: "breaking change without the suffix", modulePath: "example.com/repository", subject: "feat!: drop the old API"},
		{name: "suffix without a breaking change", modulePath: "example.com/repository/v2", subject: "feat: move to the v2 path"},
		{name: "suffix with a breaking change", modulePath: "example.com/repository/v2", subject: "feat!: move to the v2 path", want: "v2.0.0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repositoryPath := initRepository(t)
			commitGoMod(t, repositoryPath, ".", "example.com/repository", "feat: initial release")
			runGit(t, repositoryPath, "tag", "-a", "v1.2.0", "-m", "Release v1.2.0")
			if test.modulePath == "example.com/repository" {
				commitFile(t, repositoryPath, "api.txt", "api", test.subject)
			} else {
				commitGoMod(t, repositoryPath, ".", test.modulePath, test.subject)
			}

			plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
			if test.want == "" {
				if err == nil {
					t.Fatalf("expected an error, got %s", plan.NextTag)
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildReleasePlan returned error: %v", err)
			}
			if plan.NextTag != test.want {
				t.Fatalf("unexpected next tag: %s", plan.NextTag)
			}
		})
	}
}

func TestBuildReleasePlanMatchTurnsModuleScopeOff(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitFile(t, repositoryPath, "tool/main.txt", "main", "feat(tool): add a flag")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath, TagMatch: "v*"})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}
	if plan.NextTag != "v0.2.0" {
		t.Fatalf("unexpected next tag with an explicit pattern: %s", plan.NextTag)
	}
}

func TestBuildReleasePlanRejectsModuleOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options Options
	}{
		{name: "module with match", options: Options{Module: "tool", TagMatch: "v*"}},
		{name: "unknown module", options: Options{Module: "missing"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repositoryPath := initGoRepository(t)
			commitFile(t, repositoryPath, "tool/main.txt", "main", "feat(tool): add a flag")

			options := test.options
			options.Repository = repositoryPath
			if plan, err := BuildReleasePlan(options); err == nil {
				t.Fatalf("expected an error, got %s", plan.NextTag)
			}
		})
	}
}

func TestBuildReleasePlanReportsDependents(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitFile(t, repositoryPath, "library.txt", "library", "feat: add a helper")

	plan, err := BuildReleasePlan(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("BuildReleasePlan returned error: %v", err)
	}

	want := Dependent{Directory: "tool", Path: "example.com/repository/tool", Requires: "v0.1.0"}
	if plan.NextTag != "v0.2.0" || len(plan.Dependents) != 1 || plan.Dependents[0] != want {
		t.Fatalf("unexpected dependents for %s: %#v", plan.NextTag, plan.Dependents)
	}
}

func TestFindLatestTagForModule(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)

	latest, err := FindLatestTag(LatestOptions{Repository: repositoryPath, Module: "tool"})
	if err != nil {
		t.Fatalf("FindLatestTag returned error: %v", err)
	}
	if latest.Tag != "tool/v0.1.0" {
		t.Fatalf("unexpected latest tag for the tool: %s", latest.Tag)
	}
}

func TestListModules(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitGoMod(t, repositoryPath, "internal/testdata/fixture", "example.com/fixture", "test: add a fixture module")
	commitFile(t, repositoryPath, "tool/main.txt", "main", "feat(tool): add a flag")

	releases, err := ListModules(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("ListModules returned error: %v", err)
	}

	want := []ModuleRelease{
		{Directory: ".", Path: "example.com/repository", CurrentTag: "v0.1.0"},
		{Directory: "tool", Path: "example.com/repository/tool", CurrentTag: "tool/v0.1.0", NextTag: "tool/v0.2.0", HasRelease: true},
	}
	if len(releases) != len(want) {
		t.Fatalf("unexpected modules: %#v", releases)
	}
	for index := range want {
		if releases[index] != want[index] {
			t.Errorf("module %d = %#v, want %#v", index, releases[index], want[index])
		}
	}
}

func TestListModulesPutsRootModuleFirst(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitGoMod(t, repositoryPath, ".", "example.com/repository", "feat: initial library")
	commitGoMod(t, repositoryPath, "cmd/tool", "example.com/repository/cmd/tool", "feat: initial tool")
	commitGoMod(t, repositoryPath, "api", "example.com/repository/api", "feat: initial API")

	releases, err := ListModules(Options{Repository: repositoryPath})
	if err != nil {
		t.Fatalf("ListModules returned error: %v", err)
	}

	var directories []string
	for _, release := range releases {
		directories = append(directories, release.Directory)
	}
	if strings.Join(directories, " ") != ". api cmd/tool" {
		t.Fatalf("unexpected module order: %v", directories)
	}
}

// initGoRepository creates a repository holding the root module
// example.com/repository and the nested module example.com/repository/tool,
// which requires the root one at v0.1.0, both released at v0.1.0.
func initGoRepository(t *testing.T) string {
	t.Helper()

	repositoryPath := initRepository(t)
	commitGoMod(t, repositoryPath, ".", "example.com/repository", "feat: initial library")
	commitGoMod(t, repositoryPath, "tool", "example.com/repository/tool", "feat(tool): initial tool", "example.com/repository v0.1.0")
	runGit(t, repositoryPath, "tag", "-a", "v0.1.0", "-m", "Release v0.1.0")
	runGit(t, repositoryPath, "tag", "-a", "tool/v0.1.0", "-m", "Release tool/v0.1.0")
	return repositoryPath
}

// commitGoMod writes the go.mod of directory, declaring modulePath and
// requiring each "path version" given, and commits it with subject.
func commitGoMod(t *testing.T, repositoryPath, directory, modulePath, subject string, requires ...string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(repositoryPath, directory), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	content := "module " + modulePath + "\n\ngo 1.26\n"
	if len(requires) > 0 {
		content += "\nrequire (\n\t" + strings.Join(requires, "\n\t") + "\n)\n"
	}
	commitFile(t, repositoryPath, filepath.Join(directory, "go.mod"), content, subject)
}
