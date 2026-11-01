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
		"  - directory: web/homepage\n" +
		"    tagPrefix: homepage/v\n" +
		"    paths: [web/brand, web/package.json]\n" +
		"    shippedBy: []\n" +
		"  - directory: docs\n" +
		"    shippedBy: [.]\n"
	config, err := loadConfigText(t, content)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	noCarrier, rootCarrier := []string{}, []string{"."}
	want := []ModuleDeclaration{
		{Directory: "web/homepage", TagPrefix: "homepage/v", Paths: []string{"web/brand", "web/package.json"}, ShippedBy: &noCarrier},
		{Directory: "docs", ShippedBy: &rootCarrier},
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
		{name: "no shippedBy", declaration: "directory: web", message: "modules: web: shippedBy is required: list the modules whose release also carries web, or write shippedBy: []"},
		{name: "null shippedBy", declaration: "directory: web\n    shippedBy: ~", message: "modules: web: shippedBy is required"},
		{name: "unknown key", declaration: "directory: web\n    shippedBy: []\n    excludeFromRoot: true", message: "invalid keys: excludefromroot"},
		{name: "empty directory", declaration: "shippedBy: []", message: "modules: directory cannot be empty"},
		{name: "absolute directory", declaration: "directory: /web\n    shippedBy: []", message: `modules: directory "/web" is absolute`},
		{name: "unclean directory", declaration: "directory: web/homepage/\n    shippedBy: []", message: `modules: directory "web/homepage/" is not clean; write "web/homepage"`},
		{name: "directory outside", declaration: "directory: ../web\n    shippedBy: []", message: `modules: directory "../web" leaves the repository`},
		{name: "pathspec magic", declaration: "directory: \":(exclude)web\"\n    shippedBy: []", message: `modules: directory ":(exclude)web" starts with :`},
		{name: "glob directory", declaration: "directory: web/*\n    shippedBy: []", message: `modules: directory "web/*" holds a glob character`},
		{name: "glob path", declaration: "directory: web\n    paths: [\"deploy/*\"]\n    shippedBy: []", message: `modules: web: paths "deploy/*" holds a glob character`},
		{name: "own directory in paths", declaration: "directory: web\n    paths: [web]\n    shippedBy: []", message: "modules: web: paths lists web, its own directory, which the module reads already"},
		{name: "glob tag prefix", declaration: "directory: web\n    tagPrefix: \"web*\"\n    shippedBy: []", message: `modules: web: tagPrefix "web*" holds a glob character`},
		{name: "root prefix elsewhere", declaration: "directory: web\n    tagPrefix: v\n    shippedBy: []", message: "modules: web: tagPrefix v is the root module's"},
		{name: "shipped by itself", declaration: "directory: web\n    shippedBy: [web]", message: "modules: web: shippedBy lists the module itself"},
		{name: "unclean carrier", declaration: "directory: web\n    shippedBy: [./cmd]", message: `modules: web: shippedBy "./cmd" is not clean; write "cmd"`},
	}
	for _, testCase := range cases {
		_, err := loadConfigText(t, "modules:\n  - "+testCase.declaration+"\n")
		if err == nil || !strings.Contains(err.Error(), testCase.message) {
			t.Errorf("%s: LoadConfig error = %v, want one containing %q", testCase.name, err, testCase.message)
		}
	}

	_, err := loadConfigText(t, "modules:\n  - directory: web\n    shippedBy: []\n  - directory: web\n    shippedBy: []\n")
	if err == nil || !strings.Contains(err.Error(), "modules: web is declared twice") {
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
