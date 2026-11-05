package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIEndToEndDeclaresSeveralDirectories declares one module with two
// directories, which needs a name, and finds it by either of them.
func TestCLIEndToEndDeclaresSeveralDirectories(t *testing.T) {
	t.Parallel()

	repositoryPath := initTwoDirectoriesRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	binaryPath := buildCLI(t)

	expectFailure(t, binaryPath, []string{"modules", "add", "web/homepage", "deploy/homepage", "-r", repositoryPath}, "several directories need a name")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("the refused add left %s: %v", configPath, err)
	}

	expectContains(t, binaryPath, []string{"modules", "add", "web/homepage", "deploy/homepage", "-e", "homepage", "-r", repositoryPath},
		"added homepage to "+configPath,
		"the modules around it no longer read it (.)",
	)
	expectFile(t, configPath, "modules:\n  - name: homepage\n    directories: [web/homepage, deploy/homepage]\n    shippedBy: []\n")
	expectFailure(t, binaryPath, []string{"modules", "add", "deploy/homepage", "-r", repositoryPath}, "deploy/homepage is declared already")
	expectOutput(t, binaryPath, []string{"next", "-r", repositoryPath, "-g", "homepage", "-d"}, "no release")

	commitFile(t, repositoryPath, "deploy/homepage/nginx.conf", "faster", "fix: serve faster")
	expectOutput(t, binaryPath, []string{"next", "-r", repositoryPath, "-g", "deploy/homepage", "-d"}, "homepage/v0.1.1")

	web := filepath.Join(repositoryPath, "web")
	expectContains(t, binaryPath, []string{"modules", "remove", "homepage", "-r", web}, "removed homepage from "+configPath)
	expectFile(t, configPath, "modules: []\n")
	expectContains(t, binaryPath, []string{"modules", "add", "homepage", "../deploy/homepage", "-e", "site", "-r", web}, "added site to "+configPath)
	expectFile(t, configPath, "modules:\n  - name: site\n    directories: [web/homepage, deploy/homepage]\n    shippedBy: []\n")
}

// TestCLIEndToEndChangesNameAndDirectories changes what a module is called and
// what it owns: its tags stay its own, since its tag prefix is written down
// when the change would move it.
func TestCLIEndToEndChangesNameAndDirectories(t *testing.T) {
	t.Parallel()

	repositoryPath := initTwoDirectoriesRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	binaryPath := buildCLI(t)

	writeFile(t, configPath, "modules:\n  - directories: [web/homepage]\n    shippedBy: []\n")
	expectContains(t, binaryPath, []string{"modules", "set", "web/homepage", "-e", "site", "-r", repositoryPath}, "changed site in "+configPath)
	expectFile(t, configPath, "modules:\n  - name: site\n    directories: [web/homepage]\n    tagPrefix: web/homepage/v\n    shippedBy: []\n")

	expectContains(t, binaryPath, []string{"modules", "set", "site", "-D", "web/homepage", "-D", "deploy/homepage", "-r", repositoryPath}, "changed site in "+configPath)
	expectFile(t, configPath, "modules:\n  - name: site\n    directories: [web/homepage, deploy/homepage]\n    tagPrefix: web/homepage/v\n    shippedBy: []\n")
	expectFailure(t, binaryPath, []string{"modules", "set", "site", "-e", "", "-r", repositoryPath}, "several directories need a name")

	expectContains(t, binaryPath, []string{"modules", "set", "deploy/homepage", "-T", "homepage/v", "-r", repositoryPath}, "changed site in "+configPath)
	expectContains(t, binaryPath, []string{"modules", "set", "site", "-e", "homepage", "-r", repositoryPath}, "changed homepage in "+configPath)
	expectFile(t, configPath, "modules:\n  - name: homepage\n    directories: [web/homepage, deploy/homepage]\n    tagPrefix: homepage/v\n    shippedBy: []\n")

	writeFile(t, configPath, "modules:\n  - name: site\n    directories: [web/homepage]\n    shippedBy: []\n")
	expectContains(t, binaryPath, []string{"modules", "set", "site", "-e", "page", "-r", repositoryPath}, "changed page in "+configPath)
	expectFile(t, configPath, "modules:\n  - name: page\n    directories: [web/homepage]\n    tagPrefix: site/v\n    shippedBy: []\n")
	expectContains(t, binaryPath, []string{"modules", "set", "page", "-e", "", "-r", repositoryPath}, "changed web/homepage in "+configPath)
	expectFile(t, configPath, "modules:\n  - directories: [web/homepage]\n    tagPrefix: site/v\n    shippedBy: []\n")
	expectContains(t, binaryPath, []string{"modules", "set", "web/homepage", "-D", "deploy/homepage", "-r", repositoryPath}, "changed deploy/homepage in "+configPath)
	expectFile(t, configPath, "modules:\n  - directories: [deploy/homepage]\n    tagPrefix: site/v\n    shippedBy: []\n")
}

// TestCLIEndToEndMovesTheRootModule covers the prefix v, which only the module in
// . takes: a move cannot keep it, so the command asks for another rather than
// refuse a value nobody wrote.
func TestCLIEndToEndMovesTheRootModule(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitNewFile(t, repositoryPath, "a/notes.txt", "notes", "feat: add a")
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	content := "modules:\n  - directories: [.]\n    shippedBy: []\n"
	writeFile(t, configPath, content)
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "modules", "set", ".", "-D", "a", "-r", repositoryPath)
	if err == nil || !strings.Contains(output, "--tag-prefix") || strings.Contains(output, "tagPrefix v") {
		t.Errorf("the move returned %v\n%s", err, output)
	}
	expectFile(t, configPath, content)

	expectContains(t, binaryPath, []string{"modules", "set", ".", "-e", "root", "-r", repositoryPath}, "changed root in "+configPath)
	expectFile(t, configPath, "modules:\n  - name: root\n    directories: [.]\n    tagPrefix: v\n    shippedBy: []\n")
	expectContains(t, binaryPath, []string{"modules", "set", "root", "-D", "a", "-T", "x/v", "-r", repositoryPath}, "changed root in "+configPath)
	expectFile(t, configPath, "modules:\n  - name: root\n    directories: [a]\n    tagPrefix: x/v\n    shippedBy: []\n")
}

// initCarriedRepository is a repository whose root module embeds web/homepage,
// with a Go module in tool.
func initCarriedRepository(t *testing.T) string {
	t.Helper()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/homepage/index.html", "homepage", "feat: add the homepage")
	commitNewFile(t, repositoryPath, "web/brand/logo.svg", "logo", "feat: draw the logo")
	commitFile(t, repositoryPath, "site.go", "package repository\n\nimport _ \"embed\"\n\n//go:embed web/homepage/index.html\nvar page string\n", "feat: serve the homepage")
	return repositoryPath
}

// TestCLIEndToEndRewritesReferences changes the identity of a module that
// another one calls in shippedBy: the edit changes that key alone, and nothing
// else of the other entry, so that no edit is left with a carrier nobody can
// fix first.
func TestCLIEndToEndRewritesReferences(t *testing.T) {
	t.Parallel()

	repositoryPath := initCarriedRepository(t)
	commitNewFile(t, repositoryPath, "web/logo/logo.svg", "logo", "feat: move the logo")
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	binaryPath := buildCLI(t)

	writeFile(t, configPath, "modules:\n  - name: stack\n    directories: [.]\n  - directories: [web/homepage] # the site\n    shippedBy: [stack, web/brand] # carried\n  - directories: [web/brand]\n    shippedBy: []\n")
	expectContains(t, binaryPath, []string{"modules", "set", "stack", "-e", "root", "-r", repositoryPath}, "changed root in "+configPath)
	expectFile(t, configPath, "modules:\n  - name: root\n    directories: [.]\n  - directories: [web/homepage] # the site\n    shippedBy: [root, web/brand] # carried\n  - directories: [web/brand]\n    shippedBy: []\n")

	expectContains(t, binaryPath, []string{"modules", "set", "web/brand", "-D", "web/logo", "-r", repositoryPath}, "changed web/logo in "+configPath)
	expectFile(t, configPath, "modules:\n  - name: root\n    directories: [.]\n  - directories: [web/homepage] # the site\n    shippedBy: [root, web/logo] # carried\n  - directories: [web/logo]\n    tagPrefix: web/brand/v\n    shippedBy: []\n")

	expectContains(t, binaryPath, []string{"modules", "set", "root", "-e", "", "-r", repositoryPath}, "changed . in "+configPath)
	expectFile(t, configPath, "modules:\n  - directories: [.]\n  - directories: [web/homepage] # the site\n    shippedBy: [., web/logo] # carried\n  - directories: [web/logo]\n    tagPrefix: web/brand/v\n    shippedBy: []\n")

	expectContains(t, binaryPath, []string{"modules", "set", "web/logo", "-e", "logo", "-r", repositoryPath}, "changed logo in "+configPath)
	expectFile(t, configPath, "modules:\n  - directories: [.]\n  - directories: [web/homepage] # the site\n    shippedBy: [., logo] # carried\n  - name: logo\n    directories: [web/logo]\n    tagPrefix: web/brand/v\n    shippedBy: []\n")

	blockList := "modules:\n  - name: stack\n    directories: [.]\n  - directories: [web/homepage]\n    shippedBy:\n      - stack\n"
	writeFile(t, configPath, blockList)
	expectFailure(t, binaryPath, []string{"modules", "set", "stack", "-e", "root", "-r", repositoryPath}, "web/homepage: shippedBy is written over several lines")
	expectFile(t, configPath, blockList)

	closedBelow := "modules:\n  - name: stack\n    directories: [.]\n  - directories: [web/homepage]\n    shippedBy: [stack\n      ]\n"
	writeFile(t, configPath, closedBelow)
	expectFailure(t, binaryPath, []string{"modules", "set", "stack", "-e", "root", "-r", repositoryPath}, "web/homepage: shippedBy is written over several lines")
	expectFile(t, configPath, closedBelow)

	writeFile(t, configPath, "modules:\n  - name: stack\n    directories: [.]\n  - directories: [web/homepage]\n    shippedBy: [stack, stack]\n")
	expectContains(t, binaryPath, []string{"modules", "set", "stack", "-e", "root", "-r", repositoryPath}, "changed root in "+configPath)
	expectFile(t, configPath, "modules:\n  - name: root\n    directories: [.]\n  - directories: [web/homepage]\n    shippedBy: [root, root]\n")
}

// TestCLIEndToEndRefusesAnEmptyPath covers a path that comes out empty, as an
// unset variable does in a shell: it names nothing, and least of all --repo.
func TestCLIEndToEndRefusesAnEmptyPath(t *testing.T) {
	t.Parallel()

	repositoryPath := initCarriedRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	content := "modules:\n  - directories: [web/homepage]\n    shippedBy: [.]\n"
	writeFile(t, configPath, content)
	binaryPath := buildCLI(t)

	testCases := [][]string{
		{"modules", "add", "web/brand", "-b", ""},
		{"modules", "add", "web/brand", "-f", ""},
		{"modules", "add", ""},
		{"modules", "set", "web/homepage", "-D", ""},
		{"modules", "set", "web/homepage", "-b", ""},
		{"modules", "set", "", "-T", "site/v"},
		{"modules", "remove", ""},
	}
	for _, arguments := range testCases {
		expectFailure(t, binaryPath, append(arguments, "-r", repositoryPath), "a path cannot be empty")
		expectFile(t, configPath, content)
	}
}

// TestCLIEndToEndTakesAnAbsolutePath covers a path that says where it is: the
// commands take it as it is, and refuse it when it lies outside the repository.
func TestCLIEndToEndTakesAnAbsolutePath(t *testing.T) {
	t.Parallel()

	repositoryPath := initCarriedRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	homepage := filepath.Join(repositoryPath, "web", "homepage")
	binaryPath := buildCLI(t)

	expectContains(t, binaryPath, []string{"modules", "add", homepage, "-b", repositoryPath, "-r", repositoryPath}, "added web/homepage to "+configPath)
	expectFile(t, configPath, "modules:\n  - directories: [web/homepage]\n    shippedBy: [.]\n")
	expectContains(t, binaryPath, []string{"modules", "set", homepage, "-T", "site/v", "-r", repositoryPath}, "changed web/homepage in "+configPath)
	expectOutput(t, binaryPath, []string{"next", "-g", homepage, "-r", repositoryPath, "-d"}, "site/v0.1.0")
	expectFailure(t, binaryPath, []string{"modules", "add", t.TempDir(), "-r", repositoryPath}, "leaves the repository")
}

// TestCLIEndToEndThroughALinkToTheRepository covers a working directory reached
// through a link: a path means what it means from the real one, a path that does
// not exist on disk included, which git may still track.
func TestCLIEndToEndThroughALinkToTheRepository(t *testing.T) {
	t.Parallel()

	repositoryPath := initCarriedRepository(t)
	commitNewFile(t, repositoryPath, "docs/guide.md", "guide", "docs: write the guide")
	if err := os.Remove(filepath.Join(repositoryPath, "docs", "guide.md")); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(repositoryPath, link); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}
	binaryPath := buildCLI(t)

	output, err := runCLIIn(link, binaryPath, "modules", "add", "gone")
	if err == nil || !strings.Contains(output, "gone matches no tracked file") {
		t.Errorf("a path nothing lies in returned %v\n%s", err, output)
	}
	output, err = runCLIIn(link, binaryPath, "modules", "add", filepath.Join(link, "gone"))
	if err == nil || !strings.Contains(output, "gone matches no tracked file") {
		t.Errorf("that path, absolute, returned %v\n%s", err, output)
	}
	output, err = runCLIIn(link, binaryPath, "modules", "add", "web/brand", "-f", "docs/guide.md")
	if err != nil {
		t.Errorf("a path tracked and gone from the disk returned %v\n%s", err, output)
	}
	expectFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [web/brand]\n    paths: [docs/guide.md]\n    shippedBy: []\n")
}

// TestCLIEndToEndRefusesANameThatStartsWithADash covers a name a command could
// not take where it reads a module, since it would read a flag. The module is
// still found by its directory, which is how such a file is repaired.
func TestCLIEndToEndRefusesANameThatStartsWithADash(t *testing.T) {
	t.Parallel()

	repositoryPath := initCarriedRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	binaryPath := buildCLI(t)

	expectFailure(t, binaryPath, []string{"modules", "add", "web/brand", "-e", "-x", "-r", repositoryPath}, "starts with -")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("the refused add left %s: %v", configPath, err)
	}

	writeFile(t, configPath, "modules:\n  - name: -x\n    directories: [web/brand]\n    tagPrefix: brand/v\n    shippedBy: []\n")
	report, passed := readCheck(t, binaryPath, repositoryPath)
	if item := itemOf(t, report.Blocking, ruleInvalidName); passed || !report.FileOnly || !strings.Contains(item.What, "starts with -") {
		t.Errorf("passed %v, report %+v", passed, report)
	}
	expectContains(t, binaryPath, []string{"modules", "set", "web/brand", "-e", "brand", "-r", repositoryPath}, "changed brand in "+configPath)
}

// TestCLIEndToEndKeepsACarrierTwoModulesAnswerTo covers a shippedBy that gives a
// name two modules answer to, which the file cannot tell apart: an edit that takes
// the name from one of them leaves it, and says so, rather than guess.
func TestCLIEndToEndKeepsACarrierTwoModulesAnswerTo(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	const entries = "modules:\n  - name: tool\n    directories: [web/brand]\n    shippedBy: []\n  - directories: [tool/assets]\n    shippedBy: [tool]\n"
	const renamed = "modules:\n  - name: site\n    directories: [web/brand]\n    tagPrefix: site/v\n    shippedBy: []\n  - directories: [tool/assets]\n    shippedBy: [tool]\n"
	const notice = "the shippedBy of tool/assets keeps tool, since another module is called tool too"
	withoutEmbed := func(t *testing.T) string {
		t.Helper()
		repositoryPath := initGoRepository(t)
		commitNewFile(t, repositoryPath, "web/brand/logo.svg", "logo", "feat: draw the logo")
		commitNewFile(t, repositoryPath, "tool/assets/logo.svg", "logo", "feat: add the logo")
		return repositoryPath
	}

	t.Run("a new name, on what the Go module of that name ships", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, entries)
		expectContains(t, binaryPath, []string{"modules", "set", "web/brand", "-e", "site", "-T", "site/v", "-r", repositoryPath}, "changed site", notice, "give site there")
		expectFile(t, configPath, renamed)
	})

	t.Run("a new name, with no embed to tell", func(t *testing.T) {
		t.Parallel()
		repositoryPath := withoutEmbed(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, entries)
		expectContains(t, binaryPath, []string{"modules", "set", "web/brand", "-e", "site", "-T", "site/v", "-r", repositoryPath}, "changed site", notice, "give site there")
		expectFile(t, configPath, renamed)
	})

	t.Run("a name for the Go module", func(t *testing.T) {
		t.Parallel()
		repositoryPath := withoutEmbed(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, entries)
		expectContains(t, binaryPath, []string{"modules", "add", "./tool", "-e", "stack", "-r", repositoryPath}, "added stack", notice, "give stack there")
		expectFile(t, configPath, entries+"  - name: stack\n    directories: [tool]\n")
	})

	t.Run("a name for the Go module, whose embed then has no carrier", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, entries)
		expectFailure(t, binaryPath, []string{"modules", "add", "./tool", "-e", "stack", "-r", repositoryPath}, "add stack to its shippedBy")
		expectFile(t, configPath, entries)
	})

	t.Run("the entry of a named Go module removed", func(t *testing.T) {
		t.Parallel()
		repositoryPath := withoutEmbed(t)
		commitNewFile(t, repositoryPath, "stack/notes.txt", "notes", "docs: write the notes")
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, "modules:\n  - name: stack\n    directories: [tool]\n  - directories: [stack]\n    shippedBy: []\n  - directories: [web/brand]\n    shippedBy: [stack]\n")
		expectContains(t, binaryPath, []string{"modules", "remove", "tool", "-r", repositoryPath}, "removed stack", "the shippedBy of web/brand keeps stack, since another module is called stack too", "give tool there")
		expectFile(t, configPath, "modules:\n  - directories: [stack]\n    shippedBy: []\n  - directories: [web/brand]\n    shippedBy: [stack]\n")
	})

	t.Run("a new name nobody gives in shippedBy", func(t *testing.T) {
		t.Parallel()
		repositoryPath := withoutEmbed(t)
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - name: tool\n    directories: [web/brand]\n    shippedBy: []\n")
		output, err := runCLI(binaryPath, "modules", "set", "web/brand", "-e", "site", "-T", "site/v", "-r", repositoryPath)
		if err != nil || strings.Contains(output, "shippedBy") {
			t.Errorf("set returned %v\n%s", err, output)
		}
	})
}

// TestCLIEndToEndNamesGoModules lists Go modules in bumpit.yaml: an entry takes
// a name and nothing else, the carriers that call the module follow its name,
// and removing the entry gives the module its directory back.
func TestCLIEndToEndNamesGoModules(t *testing.T) {
	t.Parallel()

	repositoryPath := initCarriedRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "modules", "add", ".", "-r", repositoryPath)
	if err != nil {
		t.Fatalf("add of a Go module returned error: %v\n%s", err, output)
	}
	if strings.Contains(output, "no -b") {
		t.Errorf("add of a Go module says what no carrier rests on:\n%s", output)
	}
	expectFile(t, configPath, "modules:\n  - directories: [.]\n")

	expectContains(t, binaryPath, []string{"modules", "add", "web/homepage", "-b", ".", "-r", repositoryPath}, "added web/homepage")
	expectContains(t, binaryPath, []string{"modules", "add", "web/brand", "-b", "tool", "-r", repositoryPath}, "added web/brand")
	expectFile(t, configPath, "modules:\n  - directories: [.]\n  - directories: [web/homepage]\n    shippedBy: [.]\n  - directories: [web/brand]\n    shippedBy: [tool]\n")

	expectContains(t, binaryPath, []string{"modules", "add", "tool", "-e", "stack", "-r", repositoryPath}, "added stack to "+configPath)
	expectFile(t, configPath, "modules:\n  - directories: [.]\n  - directories: [web/homepage]\n    shippedBy: [.]\n  - directories: [web/brand]\n    shippedBy: [stack]\n  - name: stack\n    directories: [tool]\n")

	expectFailure(t, binaryPath, []string{"modules", "set", "stack", "-D", "web/homepage", "-r", repositoryPath}, "the directory of a Go module is where its go.mod is")
	expectFailure(t, binaryPath, []string{"modules", "set", "stack", "-T", "tools/v", "-r", repositoryPath}, "takes only directories and name")

	expectContains(t, binaryPath, []string{"modules", "set", "web/brand", "-b", "stack", "-r", repositoryPath}, "changed web/brand")
	expectContains(t, binaryPath, []string{"modules", "set", "web/brand", "-b", "tool", "-r", repositoryPath}, "changed web/brand")
	expectFile(t, configPath, "modules:\n  - directories: [.]\n  - directories: [web/homepage]\n    shippedBy: [.]\n  - directories: [web/brand]\n    shippedBy: [stack]\n  - name: stack\n    directories: [tool]\n")

	expectContains(t, binaryPath, []string{"modules", "remove", "stack", "-r", repositoryPath}, "removed stack from "+configPath)
	expectFile(t, configPath, "modules:\n  - directories: [.]\n  - directories: [web/homepage]\n    shippedBy: [.]\n  - directories: [web/brand]\n    shippedBy: [tool]\n")

	expectFailure(t, binaryPath, []string{"modules", "add", "tool", "-b", ".", "-r", repositoryPath}, "takes only directories and name")
	expectFailure(t, binaryPath, []string{"modules", "remove", "tool", "-r", repositoryPath}, "tool is not declared")
}

// TestCLIEndToEndRefusesAReferenceToTwoModules covers a name that, from --repo,
// is also the directory of another module, and how to say the directory.
func TestCLIEndToEndRefusesAReferenceToTwoModules(t *testing.T) {
	t.Parallel()

	repositoryPath := initCarriedRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	binaryPath := buildCLI(t)
	web := filepath.Join(repositoryPath, "web")

	writeFile(t, configPath, "modules:\n  - name: brand\n    directories: [web/homepage]\n    shippedBy: [.]\n  - directories: [web/brand]\n    shippedBy: []\n")
	expectFailure(t, binaryPath, []string{"modules", "set", "brand", "-T", "site/v", "-r", web}, "write ./brand")
	expectFailure(t, binaryPath, []string{"next", "-g", "brand", "-r", web, "-d"}, "write ./brand")
	expectContains(t, binaryPath, []string{"modules", "set", "./brand", "-T", "logo/v", "-r", web}, "changed web/brand in "+configPath)
	expectOutput(t, binaryPath, []string{"next", "-g", "./brand", "-r", web, "-d"}, "logo/v0.1.0")
	expectOutput(t, binaryPath, []string{"next", "-g", "brand", "-r", repositoryPath, "-d"}, "brand/v0.1.0")
}

// TestCLIEndToEndListsNamedModules covers what the commands print of modules
// with names and several directories. A module with neither, and a Go module
// with no name, read as they always have.
func TestCLIEndToEndListsNamedModules(t *testing.T) {
	t.Parallel()

	repositoryPath := initTwoDirectoriesRepository(t)
	commitGoMod(t, repositoryPath, "tool", "example.com/repository/tool", "feat(tool): add the tool")
	commitNewFile(t, repositoryPath, "docs/guide.md", "guide", "feat: write the guide")
	commitFile(t, repositoryPath, "bumpit.yaml", "modules:\n  - name: homepage\n    directories: [web/homepage, deploy/homepage]\n    shippedBy: []\n  - name: stack\n    directories: [tool]\n  - directories: [docs]\n    shippedBy: []\n", "chore: configure bumpit")
	commitFile(t, repositoryPath, "deploy/homepage/nginx.conf", "faster", "fix: serve faster")
	binaryPath := buildCLI(t)

	expectContains(t, binaryPath, []string{"modules", "-r", repositoryPath},
		"web/homepage, deploy/homepage",
		"homepage (no go.mod)",
		"stack (example.com/repository/tool)",
		"docs ",
		"no go.mod",
		"homepage/v0.1.1",
	)
	expectContains(t, binaryPath, []string{"modules", "-r", repositoryPath, "-o", "json"},
		`"name": "homepage"`,
		`"directories": [`,
		`"name": "stack"`,
		`"directory": "tool"`,
	)
	expectContains(t, binaryPath, []string{"explain", "-r", repositoryPath, "-g", "homepage"},
		"Module: homepage (declared in bumpit.yaml)",
		"Directories: web/homepage, deploy/homepage",
	)
	expectContains(t, binaryPath, []string{"explain", "-r", repositoryPath, "-g", "stack"},
		"Module: example.com/repository/tool",
		"Name: stack",
	)
	expectContains(t, binaryPath, []string{"explain", "-r", repositoryPath},
		"Leaves out: docs, web/homepage, deploy/homepage",
	)
	expectContains(t, binaryPath, []string{"tag", "-M", "-r", repositoryPath},
		"homepage  created tag homepage/v0.1.1",
		"stack     created tag tool/v0.1.0",
	)
}

// TestCLIEndToEndSaysWhichModulesStopReading covers the notice of an add with
// no -b: it names the modules around each directory, and none when none is.
func TestCLIEndToEndSaysWhichModulesStopReading(t *testing.T) {
	t.Parallel()

	repositoryPath := initCarriedRepository(t)
	commitNewFile(t, repositoryPath, "tool/assets/logo.svg", "logo", "feat: add the logo")
	binaryPath := buildCLI(t)
	expectContains(t, binaryPath, []string{"modules", "add", "web/brand", "tool/assets", "-e", "brand", "-r", repositoryPath},
		"no -b, so no module ships brand, and the modules around it no longer read it (., tool), but bumpit sees only //go:embed",
	)

	withoutRoot := initRepository(t)
	commitNewFile(t, withoutRoot, "docs/guide.md", "guide", "docs: write the guide")
	expectContains(t, binaryPath, []string{"modules", "add", "docs", "-r", withoutRoot},
		"no -b, so no module ships docs, but bumpit sees only //go:embed",
	)

	besideAProblem := initCarriedRepository(t)
	writeFile(t, filepath.Join(besideAProblem, "bumpit.yaml"), "modules:\n  - name: broken\n    directories: [web/homepage, web/homepage]\n    shippedBy: [.]\n")
	expectContains(t, binaryPath, []string{"modules", "add", "web/brand", "-r", besideAProblem},
		"no -b, so no module ships web/brand, and the modules around it no longer read it (.)",
	)
}
