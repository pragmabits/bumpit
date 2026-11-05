package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFindModuleSource covers where the modules of a run come from, a question
// bumpit asks when it runs, when it checks and when it edits.
func TestFindModuleSource(t *testing.T) {
	t.Parallel()

	top := t.TempDir()
	topPath := filepath.Join(top, "bumpit.yaml")
	writeFile(t, topPath, "tagMatch: v*\n")
	otherPath := filepath.Join(top, "other.yaml")
	writeFile(t, otherPath, "tagMatch: v*\n")
	if err := os.Mkdir(filepath.Join(top, "web"), 0o755); err != nil {
		t.Fatalf("Mkdir returned error: %v", err)
	}
	belowPath := filepath.Join(top, "web", "bumpit.yaml")
	writeFile(t, belowPath, "tagMatch: v*\n")

	withModules := &Config{Modules: []ModuleDeclaration{{Directories: []string{"web"}}}}
	emptyList := &Config{Modules: []ModuleDeclaration{}}
	silent := &Config{}
	testCases := []struct {
		name       string
		explicit   bool
		config     *Config
		configPath string
		want       moduleSource
		fromConfig bool
	}{
		{name: "no config", want: moduleSource{top: top, topConfig: topPath}},
		{name: "--config with modules", explicit: true, config: withModules, configPath: otherPath, want: moduleSource{top: top, topConfig: topPath, given: true}, fromConfig: true},
		{name: "--config with an empty list", explicit: true, config: emptyList, configPath: otherPath, want: moduleSource{top: top, topConfig: topPath, given: true}, fromConfig: true},
		{name: "--config silent on the modules", explicit: true, config: silent, configPath: otherPath, want: moduleSource{top: top, topConfig: topPath}},
		{name: "--config that is the one at the top", explicit: true, config: silent, configPath: topPath, want: moduleSource{top: top, topConfig: topPath, atTop: true}, fromConfig: true},
		{name: "the config at the top", config: withModules, configPath: topPath, want: moduleSource{top: top, topConfig: topPath, atTop: true}, fromConfig: true},
		{name: "a config below the top with modules", config: withModules, configPath: belowPath, want: moduleSource{top: top, topConfig: topPath, belowTop: true}},
		{name: "a config below the top with an empty list", config: emptyList, configPath: belowPath, want: moduleSource{top: top, topConfig: topPath}},
		{name: "a config below the top silent on the modules", config: silent, configPath: belowPath, want: moduleSource{top: top, topConfig: topPath}},
	}
	for _, testCase := range testCases {
		got, err := findModuleSource(testCase.explicit, func() (string, error) { return top, nil }, testCase.config, testCase.configPath)
		if err != nil || got != testCase.want || got.fromConfig() != testCase.fromConfig {
			t.Errorf("%s: findModuleSource = %+v, %v, want %+v", testCase.name, got, err, testCase.want)
		}
	}

	outside := func() (string, error) { return "", errors.New("not a git repository") }
	got, err := findModuleSource(false, outside, withModules, belowPath)
	if err != nil || got != (moduleSource{}) {
		t.Errorf("outside a repository: findModuleSource = %+v, %v, want nothing to compare with", got, err)
	}
}

// TestCLIEndToEndConfigAtTheTopInALoopOfLinks covers a config at the top nobody
// can read: a run with a --config that declares modules has no need of it, while
// the commands that edit the modules and the check stop at it.
func TestCLIEndToEndConfigAtTheTopInALoopOfLinks(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	topPath := filepath.Join(repositoryPath, "bumpit.yaml")
	if err := os.Symlink("bumpit.yaml", topPath); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}
	other := filepath.Join(repositoryPath, "other.yaml")
	writeFile(t, other, "modules: []\n")
	binaryPath := buildCLI(t)

	if output, err := runCLI(binaryPath, "next", "-c", other, "-d"); err != nil {
		t.Errorf("a run returned %v\n%s", err, output)
	}
	expectFailure(t, binaryPath, []string{"modules", "add", "web/homepage", "-c", other}, "too many levels of symbolic links")
	expectFailure(t, binaryPath, []string{"modules", "check", "-c", other}, "too many levels of symbolic links")
}

// TestCLIEndToEndConfigThatDeclaresModulesOutsideARepository covers a config with
// no top to compare it with: what stops a run is that there is no repository.
func TestCLIEndToEndConfigThatDeclaresModulesOutsideARepository(t *testing.T) {
	t.Parallel()

	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "bumpit.yaml"), "modules:\n  - directories: [web]\n    shippedBy: []\n")

	output, err := runCLI(buildCLI(t), "next", "-r", outside, "-d")
	if err == nil || strings.Contains(output, "declares modules") {
		t.Errorf("a run returned %v\n%s", err, output)
	}
}
