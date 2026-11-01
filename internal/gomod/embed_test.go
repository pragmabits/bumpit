package gomod

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestPatternReaches(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		packageDirectory string
		pattern          string
		directory        string
		nested           []string
		want             bool
	}{
		{name: "an ancestor embedded whole", packageDirectory: ".", pattern: "web", directory: "web/homepage", want: true},
		{name: "every entry of the root", packageDirectory: ".", pattern: "*", directory: "web/homepage", want: true},
		{name: "files beside the directory", packageDirectory: ".", pattern: "web/*.css", directory: "web/homepage", want: false},
		{name: "a glob matching the directory", packageDirectory: ".", pattern: "web/h*", directory: "web/homepage", want: true},
		{name: "a path inside", packageDirectory: ".", pattern: "web/homepage/dist", directory: "web/homepage", want: true},
		{name: "the all prefix", packageDirectory: ".", pattern: "all:web", directory: "web/homepage", want: true},
		{name: "from a package in an ancestor", packageDirectory: "web", pattern: "homepage/dist", directory: "web/homepage", want: true},
		{name: "from a package inside", packageDirectory: "web/homepage", pattern: "dist", directory: "web/homepage", want: true},
		{name: "another directory with the same start", packageDirectory: ".", pattern: "website", directory: "web", want: false},
		{name: "a package elsewhere", packageDirectory: "cmd", pattern: "assets", directory: "web/homepage", want: false},
		{name: "into a nested module", packageDirectory: ".", pattern: "web/homepage/admin/dist", directory: "web/homepage", nested: []string{"web/homepage/admin"}, want: false},
		{name: "a glob that may leave the nested module", packageDirectory: ".", pattern: "web/homepage/*/dist", directory: "web/homepage", nested: []string{"web/homepage/admin"}, want: true},
		{name: "beside a nested module", packageDirectory: ".", pattern: "web/homepage/dist", directory: "web/homepage", nested: []string{"web/homepage/admin"}, want: true},
		{name: "a directory starting with _ under the pattern", packageDirectory: ".", pattern: "web", directory: "web/_site", want: false},
		{name: "a directory starting with . under the pattern", packageDirectory: ".", pattern: "*", directory: "web/.well-known", want: false},
		{name: "a directory starting with _ under an all: pattern", packageDirectory: ".", pattern: "all:web", directory: "web/_site", want: true},
		{name: "a directory starting with _ the pattern names", packageDirectory: ".", pattern: "web/_site", directory: "web/_site", want: true},
	}
	for _, testCase := range cases {
		got := PatternReaches(testCase.packageDirectory, testCase.pattern, testCase.directory, testCase.nested)
		if got != testCase.want {
			t.Errorf("%s: PatternReaches(%q, %q, %q, %q) = %v, want %v", testCase.name, testCase.packageDirectory, testCase.pattern, testCase.directory, testCase.nested, got, testCase.want)
		}
	}
}

// TestFileEmbeds covers the patterns a file embeds, and the files that embed
// nothing because no build compiles them: a test file, a file that needs the
// ignore tag, a file the go command cannot read, and a tracked file deleted
// from the working tree.
func TestFileEmbeds(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	files := map[string]string{
		"server.go":      "//go:build linux\n\npackage server\n\nimport \"embed\"\n\n//go:embed \"web/homepage/dist\" `all:static`\nvar site embed.FS\n",
		"generate.go":    "//go:build ignore\n\npackage main\n\nimport _ \"embed\"\n\n//go:embed web/*.css\nvar style string\n",
		"native.go":      "package server\n\n// #include <stdlib.h>\nimport \"C\"\n\nimport _ \"embed\"\n\n//go:embed native/*.h\nvar headers string\n",
		"server_test.go": "package server\n\nimport _ \"embed\"\n\n//go:embed testdata/input\nvar input string\n",
		"plain.go":       "package server\n",
		"template.go":    "package {{.Package}}\n\nimport \"embed\"\n\n//go:embed site\nvar site embed.FS\n",
		"needs.go":       "//go:build ignore && linux\n\npackage main\n\nimport _ \"embed\"\n\n//go:embed site\nvar site string\n",
		"either.go":      "//go:build linux || ignore\n\npackage server\n\nimport _ \"embed\"\n\n//go:embed site\nvar site string\n",
		"unless.go":      "//go:build !ignore\n\npackage server\n\nimport _ \"embed\"\n\n//go:embed site\nvar site string\n",
		"legacy.go":      "// +build ignore\n\npackage main\n\nimport _ \"embed\"\n\n//go:embed site\nvar site string\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile returned error: %v", err)
		}
	}

	cases := []struct {
		file string
		want []string
	}{
		{file: "server.go", want: []string{"web/homepage/dist 7:12", "all:static 7:32"}},
		{file: "generate.go"},
		{file: "native.go", want: []string{"native/*.h 8:12"}},
		{file: "server_test.go"},
		{file: "plain.go"},
		{file: "template.go"},
		{file: "needs.go"},
		{file: "either.go", want: []string{"site 7:12"}},
		{file: "unless.go", want: []string{"site 7:12"}},
		{file: "legacy.go"},
		{file: "deleted.go"},
	}
	for _, testCase := range cases {
		embeds, err := FileEmbeds(filepath.Join(directory, testCase.file))
		if err != nil {
			t.Fatalf("FileEmbeds(%s) returned error: %v", testCase.file, err)
		}
		var got []string
		for _, embed := range embeds {
			got = append(got, embed.Pattern+" "+embed.Place())
		}
		if !slices.Equal(got, testCase.want) {
			t.Errorf("FileEmbeds(%s) = %q, want %q", testCase.file, got, testCase.want)
		}
	}
}

func TestNeverCompiled(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	files := map[string]string{
		"plain.go":    "package server\n",
		"linux.go":    "//go:build linux\n\npackage server\n",
		"ignored.go":  "//go:build ignore\n\npackage main\n",
		"template.go": "package {{.Package}}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile returned error: %v", err)
		}
	}
	for name, want := range map[string]bool{"plain.go": false, "linux.go": false, "ignored.go": true, "template.go": true, "deleted.go": false} {
		if got := NeverCompiled(filepath.Join(directory, name)); got != want {
			t.Errorf("NeverCompiled(%s) = %v, want %v", name, got, want)
		}
	}
}
