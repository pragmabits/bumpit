package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// proofCase is a file with problems, an edit of it, and whether the edit is
// accepted: the proof of an edit refuses what it makes worse, and only that.
type proofCase struct {
	name    string
	content string
	// embed is a pattern the root module embeds, when it embeds any.
	embed   string
	edit    []string
	refused string
}

// TestCLIEndToEndProofComparesBeforeAndAfter covers a file that holds problems:
// one can be fixed at a time, and what an edit adds is refused all the same.
func TestCLIEndToEndProofComparesBeforeAndAfter(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	prefixConflict := "modules:\n  - directories: [web/a]\n    tagPrefix: site/v\n    shippedBy: [.]\n  - directories: [web/b]\n    tagPrefix: site/v\n    shippedBy: [.]\n"
	testCases := []proofCase{
		{
			name:    "fixing one of two problems",
			content: "modules:\n  - directories: [web/a]\n    shippedBy: [ghost]\n  - directories: [web/b]\n    shippedBy: [.]\n    tagPrefix: v1\n",
			edit:    []string{"modules", "set", "web/b", "-T", "b/v"},
		},
		{
			name:    "an edit that adds a problem",
			content: "modules:\n  - directories: [web/a]\n    shippedBy: [ghost]\n  - directories: [web/b]\n    shippedBy: [.]\n",
			edit:    []string{"modules", "set", "web/b", "-b", "phantom"},
			refused: "shippedBy lists phantom",
		},
		{
			name:    "a value as wrong as the one it replaces",
			content: "output: xml\n",
			edit:    []string{"config", "set", "output", "yaml"},
			refused: "unsupported output format",
		},
		{
			name:    "a key beside another that is wrong",
			content: "output: xml\n",
			edit:    []string{"config", "set", "tagMessage", "Release"},
		},
		{
			name:    "another key that the new value contradicts",
			content: "promote: true\n",
			edit:    []string{"config", "set", "preRelease", "beta"},
			refused: "cannot be used together",
		},
		{
			name:    "renaming a module another problem names",
			content: prefixConflict,
			edit:    []string{"modules", "set", "web/b", "-e", "site"},
		},
		{
			name:    "renaming a module that holds a problem",
			content: "modules:\n  - name: page\n    directories: [web/a]\n    shippedBy: [ghost]\n",
			edit:    []string{"modules", "set", "page", "-e", "site"},
		},
		{
			name:    "moving a module with a conflict of prefixes",
			content: prefixConflict,
			edit:    []string{"modules", "set", "web/a", "-D", "docs"},
		},
		{
			name:    "repairing a text beside a problem of the repository",
			content: "modules:\n  - directories: [web/a, web/b]\n    shippedBy: []\n",
			embed:   "web/a/index.html",
			edit:    []string{"modules", "set", "web/a", "-e", "site"},
		},
		{
			name:    "a path no file lies in, beside a text that does not hold",
			content: "modules:\n  - name: broken\n    directories: [web/a, web/a]\n    shippedBy: []\n",
			edit:    []string{"modules", "add", "nothere"},
			refused: "nothere matches no tracked file",
		},
		{
			name:    "a carrier that is no module, beside a text that does not hold",
			content: "modules:\n  - name: broken\n    directories: [web/a, web/a]\n    shippedBy: []\n",
			edit:    []string{"modules", "add", "web/b", "-b", "ghost"},
			refused: "shippedBy lists ghost",
		},
		{
			name:    "a module called as one whose text does not hold",
			content: "modules:\n  - name: web/b\n    directories: [web/a, web/a]\n    shippedBy: []\n",
			edit:    []string{"modules", "add", "web/b"},
			refused: "the name is the directory of another module",
		},
		{
			name:    "a carrier whose text does not hold",
			content: "modules:\n  - name: brand\n    directories: [docs, docs]\n    shippedBy: []\n  - directories: [web/a]\n    shippedBy: []\n",
			edit:    []string{"modules", "set", "web/a", "-b", "brand"},
		},
		{
			name:    "another key beside a carrier whose text does not hold",
			content: "modules:\n  - name: brand\n    directories: [docs, docs]\n    shippedBy: []\n  - directories: [web/a]\n    shippedBy: [brand]\n",
			edit:    []string{"modules", "set", "web/a", "-T", "a/v"},
		},
		{
			name:    "a module beside one with no directory",
			content: "modules:\n  - name: site\n    directories: []\n    shippedBy: []\n",
			edit:    []string{"modules", "add", "docs"},
		},
		{
			name:    "another key of a module with no directory",
			content: "modules:\n  - name: site\n    shippedBy: []\n",
			edit:    []string{"modules", "set", "site", "-T", "site/v"},
		},
		{
			name:    "the first directory of a module with none",
			content: "modules:\n  - name: site\n    directories: []\n    shippedBy: []\n",
			edit:    []string{"modules", "set", "site", "-D", "web/a"},
		},
		{
			name:    "the first directory of a module with none, which another path ships",
			content: "modules:\n  - name: site\n    directories: []\n    shippedBy: []\n",
			embed:   "web/a/index.html",
			edit:    []string{"modules", "set", "site", "-D", "web/a"},
			refused: "embeds web/a/index.html",
		},
		{
			name:    "a carrier as dead as the one it replaces",
			content: "modules:\n  - directories: [web/a]\n    shippedBy: [ghost1]\n",
			edit:    []string{"modules", "set", "web/a", "-b", "ghost2"},
			refused: "shippedBy lists ghost2",
		},
		{
			name:    "a path no file lies in, beside a key that is wrong",
			content: "output: xml\n",
			edit:    []string{"modules", "add", "web/missing"},
			refused: "web/missing matches no tracked file",
		},
		{
			name:    "a carrier that is no module, beside a key that is wrong",
			content: "output: xml\nmodules:\n  - directories: [web/a]\n    shippedBy: [.]\n",
			edit:    []string{"modules", "set", "web/a", "-b", "ghost"},
			refused: "shippedBy lists ghost",
		},
		{
			name:    "the carriers of a cycle, which stays one",
			content: "modules:\n  - directories: [web/a]\n    shippedBy: [web/b]\n  - directories: [web/b]\n    shippedBy: [web/a]\n",
			edit:    []string{"modules", "set", "web/a", "-b", "web/b", "-b", "."},
		},
		{
			name:    "a carrier as dead as the one of another module",
			content: "modules:\n  - directories: [web/a]\n    shippedBy: [ghost]\n",
			edit:    []string{"modules", "add", "web/b", "-b", "ghost"},
			refused: "shippedBy lists ghost",
		},
		{
			name:    "an embed that reaches another module too",
			content: "modules:\n  - directories: [web/a]\n    shippedBy: []\n",
			embed:   "web",
			edit:    []string{"modules", "add", "web/b"},
			refused: "embeds web",
		},
		{
			name:    "a third module with a name given twice",
			content: "modules:\n  - name: site\n    directories: [web/a]\n    shippedBy: [.]\n  - name: site\n    directories: [web/b]\n    shippedBy: [.]\n",
			edit:    []string{"modules", "add", "docs", "-e", "site"},
			refused: "the name is given twice",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repositoryPath := initGoRepository(t)
			commitNewFile(t, repositoryPath, "web/a/index.html", "a", "feat: add a")
			commitNewFile(t, repositoryPath, "web/b/index.html", "b", "feat: add b")
			commitNewFile(t, repositoryPath, "docs/guide.md", "guide", "docs: write the guide")
			if testCase.embed != "" {
				commitFile(t, repositoryPath, "site.go", embeddingSource("repository", testCase.embed), "feat: serve the site")
			}
			configPath := filepath.Join(repositoryPath, "bumpit.yaml")
			writeFile(t, configPath, testCase.content)

			output, err := runCLI(binaryPath, append(testCase.edit, "-r", repositoryPath)...)
			if testCase.refused == "" {
				if err != nil {
					t.Fatalf("the edit returned %v\n%s", err, output)
				}
				return
			}
			if err == nil || !strings.Contains(output, testCase.refused) {
				t.Errorf("the edit returned %v, want a refusal with %q\n%s", err, testCase.refused, output)
			}
			written, readErr := os.ReadFile(configPath)
			if readErr != nil || string(written) != testCase.content {
				t.Errorf("the refused edit changed the file to %s, %v", written, readErr)
			}
		})
	}
}

// TestCLIEndToEndCheckFindsWhatARepairLeft covers a repair the proof accepts
// beside a problem of the repository: the check that follows says it.
func TestCLIEndToEndCheckFindsWhatARepairLeft(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/a/index.html", "a", "feat: add a")
	commitNewFile(t, repositoryPath, "web/b/index.html", "b", "feat: add b")
	commitFile(t, repositoryPath, "site.go", embeddingSource("repository", "web/a/index.html"), "feat: serve a")
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [web/a, web/b]\n    shippedBy: []\n")
	binaryPath := buildCLI(t)

	expectContains(t, binaryPath, []string{"modules", "set", "web/a", "-e", "site", "-r", repositoryPath}, "changed site")
	report, passed := readCheck(t, binaryPath, repositoryPath)
	if item := itemOf(t, report.Blocking, ruleEmbedWithoutCarrier); passed || item.Subject != "site" {
		t.Errorf("passed %v, item %+v", passed, item)
	}
}

// TestCLIEndToEndRefusalOfAddNamesNoOtherCommand covers a refusal of a module
// that was not written: it says what to do in any case, and no command that
// could not find the module.
func TestCLIEndToEndRefusalOfAddNamesNoOtherCommand(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	commitFile(t, repositoryPath, "site.go", embeddingSource("repository", "web/homepage/index.html"), "feat: serve the homepage")
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "modules", "add", "web/homepage", "-r", repositoryPath)
	if err == nil || !strings.Contains(output, "add . to its shippedBy") || strings.Contains(output, "bumpit modules set") {
		t.Errorf("add returned %v\n%s", err, output)
	}
}
