package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIEndToEndEditsModules declares, changes and removes a module with
// bumpit modules add, set and remove, against the built binary.
func TestCLIEndToEndEditsModules(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	binaryPath := buildCLI(t)

	expectContains(t, binaryPath, []string{"modules", "add", "web/homepage", "-r", repositoryPath, "-T", "homepage/v", "-f", "web/brand", "-B"}, "added web/homepage to "+configPath, "commit the file")
	expectFile(t, configPath, "modules:\n  - directory: web/homepage\n    tagPrefix: homepage/v\n    paths: [web/brand]\n    shippedBy: []\n")
	expectOutput(t, binaryPath, []string{"next", "-r", repositoryPath, "-g", "web/homepage", "-d"}, "homepage/v0.1.0")

	expectFailure(t, binaryPath, []string{"modules", "add", "web/homepage", "-r", repositoryPath, "-B"}, "web/homepage is declared already")
	expectFailure(t, binaryPath, []string{"modules", "add", "web/brand", "-r", repositoryPath}, "--shipped-by or --shipped-by-none")
	expectFailure(t, binaryPath, []string{"modules", "add", "web/brand", "-r", repositoryPath, "-B", "-b", "."}, "--shipped-by-none")
	expectFailure(t, binaryPath, []string{"modules", "set", "web/homepage", "-r", repositoryPath, "-f", "web/brand", "-N"}, "--no-paths")
	expectFailure(t, binaryPath, []string{"modules", "set", "web/homepage", "-r", repositoryPath}, "nothing to change")
	expectFailure(t, binaryPath, []string{"modules", "set", "web/homepage", "-r", repositoryPath, "--no-paths=false", "--shipped-by-none=false"}, "nothing to change")
	expectFailure(t, binaryPath, []string{"modules", "add", "web/brand", "-r", repositoryPath, "--shipped-by-none=false"}, "--shipped-by or --shipped-by-none")

	web := filepath.Join(repositoryPath, "web")
	expectContains(t, binaryPath, []string{"modules", "set", "homepage", "-r", web, "-b", "..", "-N"}, "changed web/homepage in "+configPath)
	expectFile(t, configPath, "modules:\n  - directory: web/homepage\n    tagPrefix: homepage/v\n    shippedBy: [.]\n")

	expectFailure(t, binaryPath, []string{"modules", "add", "web/missing", "-r", repositoryPath, "-B"}, "web/missing matches no tracked file")
	expectFile(t, configPath, "modules:\n  - directory: web/homepage\n    tagPrefix: homepage/v\n    shippedBy: [.]\n")

	otherPath := filepath.Join(repositoryPath, "other.yaml")
	writeFile(t, otherPath, "output: text\n")
	expectFailure(t, binaryPath, []string{"modules", "add", "web/brand", "-r", repositoryPath, "-c", otherPath, "-B"}, otherPath+" declares no modules")

	expectContains(t, binaryPath, []string{"modules", "remove", "web/homepage", "-r", repositoryPath}, "removed web/homepage from "+configPath)
	expectFile(t, configPath, "modules: []\n")
	expectFailure(t, binaryPath, []string{"modules", "remove", "web/homepage", "-r", repositoryPath}, "web/homepage is not declared")

	expectFailure(t, binaryPath, []string{"modules", "web"}, `unknown command "web"`)
}

// TestCLIEndToEndEditsConfig reads, sets and unsets the keys of the config
// with bumpit config get, set and unset, against the built binary.
func TestCLIEndToEndEditsConfig(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	binaryPath := buildCLI(t)

	expectFailure(t, binaryPath, []string{"config", "get", "tagMatch", "-r", repositoryPath}, "holds no bumpit.yaml")
	expectContains(t, binaryPath, []string{"config", "set", "tagMatch", "w*", "-r", repositoryPath}, "set tagMatch in "+configPath, "commit the file")
	expectFile(t, configPath, "tagMatch: w*\n")
	expectOutput(t, binaryPath, []string{"config", "get", "tagmatch", "-r", repositoryPath}, "w*")

	expectContains(t, binaryPath, []string{"config", "set", "tagMessage", "true", "-r", repositoryPath}, "set tagMessage")
	expectContains(t, binaryPath, []string{"config", "set", "allowDirty", "true", "-r", repositoryPath}, "set allowDirty")
	expectFile(t, configPath, "tagMatch: w*\ntagMessage: \"true\"\nallowDirty: true\n")

	expectFailure(t, binaryPath, []string{"config", "set", "allowDirty", "yes", "-r", repositoryPath}, "allowDirty takes true or false")
	expectFailure(t, binaryPath, []string{"config", "set", "modules", "[]", "-r", repositoryPath}, "bumpit modules")
	expectFailure(t, binaryPath, []string{"config", "set", "color", "red", "-r", repositoryPath}, "unknown key color")
	expectFailure(t, binaryPath, []string{"config", "set", "output", "yaml", "-r", repositoryPath}, "output")
	expectFile(t, configPath, "tagMatch: w*\ntagMessage: \"true\"\nallowDirty: true\n")

	writeFile(t, configPath, "tagMatch: w*\ntagMessage: \"true\"\nallowDirty: true\noutput: xml\n")
	expectContains(t, binaryPath, []string{"config", "set", "output", "json", "-r", repositoryPath}, "set output")
	expectContains(t, binaryPath, []string{"config", "unset", "output", "-r", repositoryPath}, "unset output")
	expectFile(t, configPath, "tagMatch: w*\ntagMessage: \"true\"\nallowDirty: true\n")

	expectContains(t, binaryPath, []string{"config", "set", "repository", ".", "-r", repositoryPath}, "set repository")
	expectContains(t, binaryPath, []string{"config", "unset", "repository", "-r", repositoryPath}, "unset repository")
	expectContains(t, binaryPath, []string{"config", "unset", "tagMatch", "-r", repositoryPath}, "unset tagMatch in "+configPath)
	expectFailure(t, binaryPath, []string{"config", "get", "tagMatch", "-r", repositoryPath}, "tagMatch is not set in "+configPath)
	expectFailure(t, binaryPath, []string{"config", "unset", "tagMatch", "-r", repositoryPath}, "tagMatch is not set in "+configPath)

	otherPath := filepath.Join(t.TempDir(), "other.yaml")
	writeFile(t, otherPath, "output: text\n")
	expectContains(t, binaryPath, []string{"config", "set", "output", "json", "-c", otherPath, "-r", repositoryPath}, "set output in "+otherPath)
	expectFile(t, otherPath, "output: json\n")

	expectFailure(t, binaryPath, []string{"config", "tagMatch"}, `unknown command "tagMatch"`)
}

// TestCLIEndToEndEditKeepsTheFile edits a config reached through a symbolic
// link: the link stays, and the file it points to keeps its mode.
func TestCLIEndToEndEditKeepsTheFile(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	targetPath := filepath.Join(repositoryPath, "shared.yaml")
	writeFile(t, targetPath, "output: text\n")
	if err := os.Chmod(targetPath, 0o600); err != nil {
		t.Fatalf("Chmod returned error: %v", err)
	}
	linkPath := filepath.Join(repositoryPath, "bumpit.yaml")
	if err := os.Symlink("shared.yaml", linkPath); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}
	binaryPath := buildCLI(t)

	expectContains(t, binaryPath, []string{"config", "set", "output", "json", "-r", repositoryPath}, "set output in "+linkPath)
	expectFile(t, targetPath, "output: json\n")
	if info, err := os.Lstat(linkPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("bumpit.yaml is no longer a symbolic link: %v, %v", info, err)
	}
	if info, err := os.Stat(targetPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("shared.yaml has the mode %v, %v, want 0600", info.Mode().Perm(), err)
	}
	entries, err := os.ReadDir(repositoryPath)
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".yaml") && entry.Name() != "shared.yaml" && entry.Name() != "bumpit.yaml" {
			t.Errorf("the edit left %s behind", entry.Name())
		}
	}
}

func expectFailure(t *testing.T, binaryPath string, arguments []string, text string) {
	t.Helper()

	output, err := runCLI(binaryPath, arguments...)
	if err == nil {
		t.Errorf("%v succeeded, want a failure with %q\noutput: %s", arguments, text, output)
		return
	}
	if !strings.Contains(output, text) {
		t.Errorf("%v output lacks %q:\n%s", arguments, text, output)
	}
}

func expectFile(t *testing.T, path, want string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(content) != want {
		t.Errorf("%s holds\n%q\nwant\n%q", path, content, want)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}

// TestCLIEndToEndEditsModulesAsDeclared changes a declaration that writes an
// empty list of paths, from a directory whose own config does not load.
func TestCLIEndToEndEditsModulesAsDeclared(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	writeFile(t, configPath, "modules:\n  - directory: web/homepage\n    paths: []\n    shippedBy: []\n")
	web := filepath.Join(repositoryPath, "web")
	writeFile(t, filepath.Join(web, "bumpit.yaml"), "output: xml\n")
	binaryPath := buildCLI(t)

	expectContains(t, binaryPath, []string{"modules", "set", "web/homepage", "-r", repositoryPath, "-T", "site/v"}, "changed web/homepage")
	expectFile(t, configPath, "modules:\n  - directory: web/homepage\n    tagPrefix: site/v\n    shippedBy: []\n")
	expectContains(t, binaryPath, []string{"modules", "add", "brand", "-r", web, "-B"}, "added web/brand to "+configPath)
}

// TestCLIEndToEndEditResolvesThroughTheLink edits a config reached through
// a link to another directory, whose repository the runs resolve from the
// directory of the link, and a link to a file that does not exist yet.
func TestCLIEndToEndEditResolvesThroughTheLink(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	if err := os.Mkdir(filepath.Join(repositoryPath, "sub"), 0o755); err != nil {
		t.Fatalf("Mkdir returned error: %v", err)
	}
	shared := t.TempDir()
	targetPath := filepath.Join(shared, "bumpit.yaml")
	writeFile(t, targetPath, "repository: sub\n")
	linkPath := filepath.Join(repositoryPath, "bumpit.yaml")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}
	binaryPath := buildCLI(t)

	expectContains(t, binaryPath, []string{"config", "set", "output", "json", "-r", repositoryPath}, "set output in "+linkPath)
	expectFile(t, targetPath, "repository: sub\noutput: json\n")

	pendingPath := filepath.Join(shared, "pending.yaml")
	otherPath := filepath.Join(t.TempDir(), "bumpit.yaml")
	if err := os.Symlink(pendingPath, otherPath); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}
	expectContains(t, binaryPath, []string{"config", "set", "output", "json", "-r", filepath.Dir(otherPath)}, "set output in "+otherPath)
	expectFile(t, pendingPath, "output: json\n")
	if info, err := os.Lstat(otherPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link to a file that did not exist is no longer a link: %v, %v", info, err)
	}
}
