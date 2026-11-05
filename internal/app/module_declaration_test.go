package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadConfigReadsModuleDeclarations(t *testing.T) {
	t.Parallel()

	content := "modules:\n" +
		"  - directories: [web/homepage]\n" +
		"    tagPrefix: homepage/v\n" +
		"    paths: [web/brand, web/package.json]\n" +
		"    shippedBy: []\n" +
		"  - directories: [docs]\n" +
		"    shippedBy: [.]\n"
	config, err := loadConfigText(t, content)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	noCarrier, rootCarrier := []string{}, []string{"."}
	want := []ModuleDeclaration{
		{Directories: []string{"web/homepage"}, TagPrefix: "homepage/v", Paths: []string{"web/brand", "web/package.json"}, ShippedBy: &noCarrier},
		{Directories: []string{"docs"}, ShippedBy: &rootCarrier},
	}
	if !reflect.DeepEqual(config.Modules, want) {
		t.Fatalf("modules = %#v, want %#v", config.Modules, want)
	}
	if prefix := config.Modules[1].prefix(); prefix != "docs/v" {
		t.Fatalf("default tag prefix of docs = %q, want docs/v", prefix)
	}
}

func TestLoadConfigRejectsModuleDeclarations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		declaration string
		message     string
	}{
		{name: "unknown key", declaration: "directories: [web]\n    shippedBy: []\n    excludeFromRoot: true", message: "invalid keys: excludefromroot"},
		{name: "empty directory", declaration: "shippedBy: []", message: "modules: directories cannot be empty"},
		{name: "absolute directory", declaration: "directories: [/web]\n    shippedBy: []", message: `modules: /web: directory "/web" is absolute`},
		{name: "unclean directory", declaration: "directories: [web/homepage/]\n    shippedBy: []", message: `directory "web/homepage/" is not clean: bumpit compares paths as they are written; write "web/homepage"`},
		{name: "directory outside", declaration: "directories: [../web]\n    shippedBy: []", message: `directory "../web" leaves the repository`},
		{name: "pathspec magic", declaration: "directories: [\":(exclude)web\"]\n    shippedBy: []", message: `directory ":(exclude)web" starts with :`},
		{name: "glob directory", declaration: "directories: [\"web/*\"]\n    shippedBy: []", message: `directory "web/*" holds a glob character`},
		{name: "glob path", declaration: "directories: [web]\n    paths: [\"deploy/*\"]\n    shippedBy: []", message: `modules: web: paths "deploy/*" holds a glob character`},
		{name: "own directory in paths", declaration: "directories: [web]\n    paths: [web]\n    shippedBy: []", message: "modules: web: paths lists web, its own directory: the module reads it already"},
		{name: "glob tag prefix", declaration: "directories: [web]\n    tagPrefix: \"web*\"\n    shippedBy: []", message: `modules: web: tagPrefix "web*" holds a glob character`},
		{name: "root prefix elsewhere", declaration: "directories: [web]\n    tagPrefix: v\n    shippedBy: []", message: "modules: web: tagPrefix v is the root module's"},
		{name: "shipped by itself", declaration: "directories: [web]\n    shippedBy: [web]", message: "modules: web: shippedBy lists the module itself"},
		{name: "unclean carrier", declaration: "directories: [web]\n    shippedBy: [./cmd]", message: `modules: web: shippedBy "./cmd" is not clean: bumpit compares paths as they are written; write "cmd"`},
	}
	for _, testCase := range cases {
		_, err := loadConfigText(t, "modules:\n  - "+testCase.declaration+"\n")
		if err == nil || !strings.Contains(err.Error(), testCase.message) {
			t.Errorf("%s: LoadConfig error = %v, want one containing %q", testCase.name, err, testCase.message)
		}
	}

	_, err := loadConfigText(t, "modules:\n  - directories: [web]\n    shippedBy: []\n  - directories: [web]\n    shippedBy: []\n")
	if err == nil || !strings.Contains(err.Error(), "web is declared twice") {
		t.Errorf("a directory declared twice: LoadConfig error = %v", err)
	}
}

// loadConfigText loads content as the bumpit.yaml of an empty directory.
func loadConfigText(t *testing.T, content string) (Config, error) {
	t.Helper()

	directory := t.TempDir()
	path := filepath.Join(directory, "bumpit.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	config, _, err := LoadConfig(path, Config{Repository: ".", Output: "text"})
	return config, err
}
