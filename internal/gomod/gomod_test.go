package gomod

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModuleTagPrefix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		directory string
		path      string
		expected  string
	}{
		{".", "example.com/repository", "v"},
		{"cmd/tool", "example.com/repository/cmd/tool", "cmd/tool/v"},
		{"sub/v2", "example.com/repository/sub/v2", "sub/v"},
		{"sub", "example.com/repository/sub/v2", "sub/v"},
		{"v2", "example.com/repository/v2", "v"},
		{".", "example.com/repository/v2", "v"},
		{".", "gopkg.in/yaml.v3", "v"},
	}

	for _, testCase := range testCases {
		module := Module{Directory: testCase.directory, Path: testCase.path}
		if prefix := module.TagPrefix(); prefix != testCase.expected {
			t.Errorf("TagPrefix() of %s in %q = %q, want %q", testCase.path, testCase.directory, prefix, testCase.expected)
		}
	}
}

func TestModuleCheckVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path    string
		version string
		valid   bool
	}{
		{"example.com/repository", "v0.3.0", true},
		{"example.com/repository", "v1.4.0", true},
		{"example.com/repository", "v2.0.0", false},
		{"example.com/repository/v2", "v2.1.0", true},
		{"example.com/repository/v2", "v2.0.0-beta", true},
		{"example.com/repository/v2", "v1.9.0", false},
		{"example.com/repository/v2", "v3.0.0", false},
		{"gopkg.in/yaml.v3", "v3.0.1", true},
	}

	for _, testCase := range testCases {
		err := Module{Path: testCase.path}.CheckVersion(testCase.version)
		if valid := err == nil; valid != testCase.valid {
			t.Errorf("CheckVersion(%s) for %s: error = %v, want valid = %t", testCase.version, testCase.path, err, testCase.valid)
		}
	}
}

func TestLoad(t *testing.T) {
	t.Parallel()

	top := t.TempDir()
	if err := os.Mkdir(filepath.Join(top, "tool"), 0o755); err != nil {
		t.Fatalf("Mkdir returned error: %v", err)
	}
	content := "// The tool.\nmodule example.com/repository/tool\n\ngo 1.26\n\nrequire (\n\texample.com/repository v0.1.0\n\texample.com/other v1.2.3 // indirect\n)\n"
	if err := os.WriteFile(filepath.Join(top, "tool", "go.mod"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	module, err := Load(top, "tool")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if module.Directory != "tool" || module.Path != "example.com/repository/tool" {
		t.Fatalf("unexpected module: %#v", module)
	}
	if module.Requires["example.com/repository"] != "v0.1.0" || module.Requires["example.com/other"] != "v1.2.3" {
		t.Fatalf("unexpected requirements: %#v", module.Requires)
	}
}

func TestLoadRejectsMissingModulePath(t *testing.T) {
	t.Parallel()

	top := t.TempDir()
	if err := os.WriteFile(filepath.Join(top, "go.mod"), []byte("go 1.26\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if module, err := Load(top, "."); err == nil {
		t.Fatalf("expected an error for a go.mod without a module path, got %#v", module)
	}
}

func TestIgnored(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		directory string
		expected  bool
	}{
		{".", false},
		{"cmd/tool", false},
		{"testdata", true},
		{"internal/testdata/fixture", true},
		{"vendor/example.com/other", true},
		{".hidden/tool", true},
		{"_examples/tool", true},
	}

	for _, testCase := range testCases {
		if ignored := Ignored(testCase.directory); ignored != testCase.expected {
			t.Errorf("Ignored(%q) = %t, want %t", testCase.directory, ignored, testCase.expected)
		}
	}
}
