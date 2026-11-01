package nodejs

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestInputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		files     map[string]string
		directory string
		want      []string
	}{
		{
			name: "a pnpm workspace",
			files: map[string]string{
				"web/package.json":          `{"name": "web", "private": true}`,
				"web/pnpm-workspace.yaml":   "packages:\n  - app\n  - brand\n  - homepage\n",
				"web/pnpm-lock.yaml":        "lockfileVersion: '9.0'\n",
				"web/brand/package.json":    `{"name": "@orvelis/brand"}`,
				"web/homepage/package.json": `{"name": "@orvelis/homepage", "dependencies": {"@orvelis/brand": "workspace:*", "astro": "^5.0.0"}}`,
				"web/app/package.json":      `{"name": "@orvelis/app"}`,
			},
			directory: "web/homepage",
			want:      []string{"web/brand", "web/package.json", "web/pnpm-lock.yaml", "web/pnpm-workspace.yaml"},
		},
		{
			name: "every kind of local specification",
			files: map[string]string{
				"package.json":            `{"name": "root", "workspaces": ["packages/*"]}`,
				"package-lock.json":       "{}",
				"packages/a/package.json": `{"name": "@x/a"}`,
				"packages/b/package.json": `{"name": "b"}`,
				"packages/c/package.json": `{"name": "c"}`,
				"packages/d/package.json": `{"name": "d"}`,
				"packages/e/package.json": `{"name": "e"}`,
				"tools/archive.tgz":       "archive",
				"packages/site/package.json": `{"name": "site",
					"dependencies": {"brand": "workspace:@x/a@*", "sibling": "workspace:../b"},
					"devDependencies": {"w": "file:../c", "l": "link:../d", "tarball": "file:../../tools/archive.tgz"},
					"peerDependencies": {"e": "^1.0.0"},
					"optionalDependencies": {"remote": "github:x/remote", "registry": "npm:left-pad@1"}}`,
			},
			directory: "packages/site",
			want:      []string{"package-lock.json", "package.json", "packages/a", "packages/b", "packages/c", "packages/d", "packages/e", "tools/archive.tgz"},
		},
		{
			name: "a transitive dependency",
			files: map[string]string{
				"package.json":            `{"workspaces": {"packages": ["packages/*"]}}`,
				"yarn.lock":               "",
				".yarnrc.yml":             "nodeLinker: node-modules\n",
				".yarn/releases/yarn.cjs": "release",
				"packages/a/package.json": `{"name": "a", "dependencies": {"b": "workspace:^"}}`,
				"packages/b/package.json": `{"name": "b", "dependencies": {"c": "workspace:^"}}`,
				"packages/c/package.json": `{"name": "c"}`,
			},
			directory: "packages/a",
			want:      []string{".yarn", ".yarnrc.yml", "package.json", "packages/b", "packages/c", "yarn.lock"},
		},
		{
			name: "an override of the root that points at a package",
			files: map[string]string{
				"package.json":              `{"workspaces": ["apps/*"], "overrides": {"left": {"right": "file:./vendor/right"}}, "pnpm": {"overrides": {"x": "link:./vendor/x"}}}`,
				"vendor/right/package.json": `{"name": "right"}`,
				"vendor/x/package.json":     `{"name": "x"}`,
				"apps/site/package.json":    `{"name": "site"}`,
			},
			directory: "apps/site",
			want:      []string{"package.json", "vendor/right", "vendor/x"},
		},
		{
			name: "nested roots, a root without packages, and configuration above the root",
			files: map[string]string{
				".yarnrc.yml":                    "enableGlobalCache: false\n",
				"repo/package.json":              `{"workspaces": ["inner"]}`,
				"repo/bun.lock":                  "{}",
				"repo/inner/pnpm-workspace.yaml": "onlyBuiltDependencies: []\n",
				"repo/inner/.pnpmfile.mjs":       "export default {}\n",
				"repo/inner/patches/a.patch":     "patch",
				"repo/inner/site/package.json":   `{"name": "site"}`,
			},
			directory: "repo/inner/site",
			want:      []string{".yarnrc.yml", "repo/bun.lock", "repo/inner/.pnpmfile.mjs", "repo/inner/patches", "repo/inner/pnpm-workspace.yaml", "repo/package.json"},
		},
		{
			name: "a package elsewhere with the name of a dependency, read too",
			files: map[string]string{
				"web/package.json":           `{"workspaces": ["site"]}`,
				"web/site/package.json":      `{"name": "site", "dependencies": {"shared": "^1.0.0"}}`,
				"legacy/shared/package.json": "\uFEFF{\"name\": \"shared\"}",
			},
			directory: "web/site",
			want:      []string{"legacy/shared", "web/package.json"},
		},
		{
			name: "a dependency on an ancestor, and the module that is the root",
			files: map[string]string{
				"package.json":   `{"name": "root", "workspaces": ["*"], "dependencies": {"a": "workspace:*"}}`,
				"pnpm-lock.yaml": "",
				"a/package.json": `{"name": "a", "dependencies": {"root": "workspace:*"}}`,
			},
			directory: ".",
			want:      []string{"a"},
		},
		{
			name: "a directory with no package.json",
			files: map[string]string{
				"package.json":  `{"workspaces": ["*"]}`,
				"docs/guide.md": "guide",
			},
			directory: "docs",
		},
		{
			name: "an unrelated package.json that cannot be read",
			files: map[string]string{
				"package.json":           `{"workspaces": ["site"]}`,
				"site/package.json":      `{"name": "site", "dependencies": {"react": "^19.0.0"}}`,
				"templates/package.json": `{"name": {{.Name}}}`,
			},
			directory: "site",
			want:      []string{"package.json"},
		},
	}
	for _, testCase := range cases {
		top, tracked := writeTree(t, testCase.files)
		repository := Read(top, tracked)
		got, err := repository.Inputs(testCase.directory)
		if err != nil {
			t.Errorf("%s: Inputs(%s) returned error: %v", testCase.name, testCase.directory, err)
			continue
		}
		if !slices.Equal(got, testCase.want) {
			t.Errorf("%s: Inputs(%s) = %q, want %q", testCase.name, testCase.directory, got, testCase.want)
		}
	}
}

func TestInputsRefusesWhatItMustRead(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		files     map[string]string
		directory string
		message   string
	}{
		{
			name:      "its own package.json",
			files:     map[string]string{"site/package.json": `{"name": `},
			directory: "site",
			message:   "read site/package.json:",
		},
		{
			name:      "the package.json of an ancestor",
			files:     map[string]string{"package.json": `{"workspaces": `, "site/package.json": `{"name": "site"}`},
			directory: "site",
			message:   "read package.json:",
		},
		{
			name:      "the package.json a path points at",
			files:     map[string]string{"site/package.json": `{"dependencies": {"b": "file:../b"}}`, "b/package.json": `[`},
			directory: "site",
			message:   "read b/package.json:",
		},
		{
			name:      "the pnpm-workspace.yaml of a root",
			files:     map[string]string{"pnpm-workspace.yaml": "packages: [", "site/package.json": `{"name": "site"}`},
			directory: "site",
			message:   "read pnpm-workspace.yaml:",
		},
	}
	for _, testCase := range cases {
		top, tracked := writeTree(t, testCase.files)
		_, err := Read(top, tracked).Inputs(testCase.directory)
		if err == nil || !strings.HasPrefix(err.Error(), testCase.message) {
			t.Errorf("%s: Inputs error = %v, want one starting with %q", testCase.name, err, testCase.message)
		}
	}
}

// writeTree writes files under a new directory and returns it with the
// files, which stand for the tracked files of a repository.
func writeTree(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()

	top := t.TempDir()
	tracked := make([]string, 0, len(files))
	for name, content := range files {
		file := filepath.Join(top, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatalf("MkdirAll returned error: %v", err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile returned error: %v", err)
		}
		tracked = append(tracked, name)
	}
	sort.Strings(tracked)
	return top, tracked
}
