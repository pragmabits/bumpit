package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// initCandidateRepository is a repository with a root Go module that embeds
// web/homepage, a line of homepage/v tags that no module owns and two
// directories it may be, a Helm chart with a subchart, and a public and a
// private Node.js package.
func initCandidateRepository(t *testing.T) string {
	t.Helper()

	repositoryPath := initHomepageRepository(t)
	commitNewFile(t, repositoryPath, "deploy/homepage/values.yaml", "replicas: 1\n", "chore: deploy the homepage")
	commitFile(t, repositoryPath, "site.go", "package repository\n\nimport _ \"embed\"\n\n//go:embed web/homepage/index.html\nvar page string\n", "feat: serve the homepage")
	runGit(t, repositoryPath, "tag", "-a", "homepage/v0.1.0", "-m", "homepage v0.1.0")
	commitNewFile(t, repositoryPath, "deploy/chart/Chart.yaml", "apiVersion: v2\nname: chart\nversion: 1.4.0\n", "chore: chart the deploy")
	commitNewFile(t, repositoryPath, "deploy/chart/charts/sub/Chart.yaml", "apiVersion: v2\nname: sub\nversion: 0.1.0\n", "chore: add a subchart")
	commitNewFile(t, repositoryPath, "web/widget/package.json", `{"name": "widget", "version": "2.0.0"}`, "feat: add the widget")
	commitNewFile(t, repositoryPath, "web/private/package.json", `{"name": "private", "version": "1.0.0", "private": true}`, "feat: add a private package")
	runGit(t, repositoryPath, "tag", "-a", "homepage/v0.2.0", "-m", "homepage v0.2.0")
	return repositoryPath
}

// TestCLIEndToEndInit creates bumpit.yaml with the Go modules, proposes one
// module per tag line with the command that declares it, and runs that command.
func TestCLIEndToEndInit(t *testing.T) {
	t.Parallel()

	repositoryPath := initCandidateRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	binaryPath := buildCLI(t)

	expectContains(t, binaryPath, []string{"init", "-r", repositoryPath},
		"created "+configPath,
		"listed the Go modules . in "+configPath,
		"homepage/v: 2 tags, the newest homepage/v0.2.0",
		"bumpit modules add deploy/homepage web/homepage -r "+repositoryPath+" -e homepage -b .\n",
		"site.go:5:12 embeds web/homepage/index.html",
		"leave out a directory that is no part of the module",
		"deploy/chart/Chart.yaml: version 1.4.0",
		"bumpit modules add deploy/chart -r "+repositoryPath+"\n",
		"web/widget/package.json: version 2.0.0",
		"bumpit modules add web/widget -r "+repositoryPath+"\n",
		"bumpit sees only //go:embed",
		"commit the file",
	)
	output, _ := runCLI(binaryPath, "init", "-r", repositoryPath)
	for _, unwanted := range []string{"created", "listed the Go modules", "charts/sub", "web/private", "commit the file"} {
		if strings.Contains(output, unwanted) {
			t.Errorf("the second init printed %q:\n%s", unwanted, output)
		}
	}
	if !strings.Contains(output, configPath+" exists already: left as it is") {
		t.Errorf("the second init does not say the config stays:\n%s", output)
	}
	expectFile(t, configPath, initialConfig+"modules:\n  - directories: [.]\n")

	web := filepath.Join(repositoryPath, "web")
	expectContains(t, binaryPath, []string{"init", "-r", web}, "bumpit modules add ../deploy/homepage homepage -r "+web+" -e homepage -b ..\n", "../site.go:5:12 embeds web/homepage/index.html")
	expectContains(t, binaryPath, []string{"modules", "add", "../deploy/homepage", "homepage", "-r", web, "-e", "homepage", "-b", ".."}, "added homepage")
	output, err := runCLI(binaryPath, "init", "-r", repositoryPath)
	if err != nil || strings.Contains(output, "homepage/v:") {
		t.Errorf("init lists the tags of a declared module: %v\n%s", err, output)
	}
}

func TestCLIEndToEndInitJSON(t *testing.T) {
	t.Parallel()

	repositoryPath := initCandidateRepository(t)
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "init", "-r", repositoryPath, "-o", "json")
	if err != nil {
		t.Fatalf("init returned error: %v\n%s", err, output)
	}
	var report InitReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v\n%s", err, output)
	}
	if !report.Created || report.Config != filepath.Join(repositoryPath, "bumpit.yaml") || !slices.Equal(report.Declared, []string{"."}) {
		t.Errorf("report = %+v, want the created config and the root module", report)
	}
	if len(report.TagLines) != 1 || report.TagLines[0].Candidate == nil {
		t.Fatalf("tag lines = %+v, want homepage/v with a candidate", report.TagLines)
	}
	homepage := report.TagLines[0].Candidate
	if homepage.Name != "homepage" || !slices.Equal(homepage.Directories, []string{"deploy/homepage", "web/homepage"}) || homepage.TagPrefix != "" || !slices.Equal(homepage.ShippedBy, []string{"."}) {
		t.Errorf("candidate = %+v, want homepage in both directories, shipped by .", homepage)
	}
	if len(homepage.Embeds) != 1 || homepage.Embeds[0].Place != "site.go:5:12" || homepage.Embeds[0].Pattern != "web/homepage/index.html" {
		t.Errorf("embeds = %+v, want site.go:5:12", homepage.Embeds)
	}
	if len(report.Manifests) != 2 || report.Manifests[0].Manifest != "deploy/chart/Chart.yaml" || report.Manifests[1].Version != "2.0.0" {
		t.Errorf("manifests = %+v, want the chart and the widget", report.Manifests)
	}
}

// TestCLIEndToEndInitListsGoModules covers what init declares: the Go modules
// no entry lists, in the file as it is, and nothing the second time.
func TestCLIEndToEndInitListsGoModules(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	testCases := []struct {
		name    string
		content string
		want    string
		listed  string
	}{
		{
			name:    "keeping the rest of the file",
			content: "# ours\noutput: text # plain\n",
			want:    "# ours\noutput: text # plain\nmodules:\n  - directories: [.]\n  - directories: [tool]\n",
			listed:  "listed the Go modules ., tool in ",
		},
		{
			name:    "under an empty list",
			content: "modules: [] # none yet\n",
			want:    "modules: # none yet\n  - directories: [.]\n  - directories: [tool]\n",
			listed:  "listed the Go modules ., tool in ",
		},
		{
			name:    "those no entry lists",
			content: "modules:\n  - name: platform\n    directories: [.]\n",
			want:    "modules:\n  - name: platform\n    directories: [.]\n  - directories: [tool]\n",
			listed:  "listed the Go modules tool in ",
		},
	}
	for _, testCase := range testCases {
		repositoryPath := initGoRepository(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, testCase.content)

		output, err := runCLI(binaryPath, "init", "-r", repositoryPath)
		if err != nil || !strings.Contains(output, testCase.listed+configPath) || !strings.Contains(output, "commit the file") || strings.Contains(output, "left as it is") {
			t.Errorf("%s: init returned %v\n%s", testCase.name, err, output)
		}
		expectFile(t, configPath, testCase.want)

		output, err = runCLI(binaryPath, "init", "-r", repositoryPath)
		if err != nil || strings.Contains(output, "listed the Go modules") || !strings.Contains(output, "left as it is") {
			t.Errorf("%s: the second init returned %v\n%s", testCase.name, err, output)
		}
		expectFile(t, configPath, testCase.want)
	}
}

// TestCLIEndToEndInitSaysTheGoModulesAsTheFileWritesThem covers what it lists from
// a subdirectory: as the file writes them, while the commands are for --repo.
func TestCLIEndToEndInitSaysTheGoModulesAsTheFileWritesThem(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/index.html", "site", "feat: add the site")
	web := filepath.Join(repositoryPath, "web")

	output, err := runCLI(buildCLI(t), "init", "-r", web, "-o", "json")
	if err != nil {
		t.Fatalf("init returned %v\n%s", err, output)
	}
	var report InitReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v\n%s", err, output)
	}
	if !slices.Equal(report.Declared, []string{".", "tool"}) {
		t.Errorf("declared = %v, want what the file lists", report.Declared)
	}
	expectFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), initialConfig+"modules:\n  - directories: [.]\n  - directories: [tool]\n")
}

// TestCLIEndToEndInitStopsOnAProblem covers a file that holds a problem: what
// init would write or propose over it could not be promised, so it stops with
// the problem and leaves the file.
func TestCLIEndToEndInitStopsOnAProblem(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	testCases := []struct {
		name    string
		content string
		message string
	}{
		{name: "an invalid key", content: "output: xml\n", message: "output"},
		{name: "a module no check accepts", content: "modules:\n  - directories: [docs]\n", message: "shippedBy is required"},
		{name: "a key no version of bumpit reads", content: "colour: red\n", message: "invalid keys: colour"},
	}
	for _, testCase := range testCases {
		repositoryPath := initGoRepository(t)
		commitNewFile(t, repositoryPath, "docs/guide.md", "guide", "docs: write the guide")
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, testCase.content)

		expectFailure(t, binaryPath, []string{"init", "-r", repositoryPath}, testCase.message)
		expectFile(t, configPath, testCase.content)
	}
}

// TestCLIEndToEndInitReadsTheConfigOfTheRepository covers what makes the
// commands init prints mean what init saw: the config in --repo, whose
// repository key and whose faults they will meet.
func TestCLIEndToEndInitReadsTheConfigOfTheRepository(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)

	repositoryPath := initHomepageRepository(t)
	commitNewFile(t, repositoryPath, "sub/notes.txt", "notes", "docs: write the notes")
	runGit(t, repositoryPath, "tag", "-a", "homepage/v0.1.0", "-m", "homepage")
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "repository: sub\n")
	output, err := runCLIIn(repositoryPath, binaryPath, "init")
	if err != nil || !strings.Contains(output, "bumpit modules add ../web/homepage -e homepage\n") {
		t.Fatalf("init beside a repository key returned %v\n%s", err, output)
	}
	if output, err := runCLIIn(repositoryPath, binaryPath, "modules", "add", "../web/homepage", "-e", "homepage"); err != nil {
		t.Errorf("the command init printed returned %v\n%s", err, output)
	}

	unreadable := initHomepageRepository(t)
	web := filepath.Join(unreadable, "web")
	writeFile(t, filepath.Join(web, "bumpit.yaml"), "colour: red\n")
	expectFailure(t, binaryPath, []string{"init", "-r", web}, "invalid keys: colour")
	if _, err := os.Stat(filepath.Join(unreadable, "bumpit.yaml")); !os.IsNotExist(err) {
		t.Errorf("the refused init left a config: %v", err)
	}
}

// TestCLIEndToEndInitProposesOneModulePerTagLine covers how a tag line becomes
// one proposal: its directories, the name its prefix gives, and the flags that
// proposal needs beyond them.
func TestCLIEndToEndInitProposesOneModulePerTagLine(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	testCases := []struct {
		name   string
		tag    string
		files  []string
		wanted []string
	}{
		{
			name:   "a directory that already says the name",
			tag:    "docs/v1.0.0",
			files:  []string{"docs/guide.md"},
			wanted: []string{"bumpit modules add docs -r %s\n"},
		},
		{
			name:   "a directory the name differs from",
			tag:    "homepage/v1.0.0",
			files:  []string{"web/homepage/index.html"},
			wanted: []string{"bumpit modules add web/homepage -r %s -e homepage\n"},
		},
		{
			name:   "a prefix the name does not give",
			tag:    "site-v1.0.0",
			files:  []string{"site/index.html"},
			wanted: []string{"bumpit modules add site -r %s -T site-v\n"},
		},
		{
			name:   "a directory inside another",
			tag:    "homepage/v1.0.0",
			files:  []string{"web/homepage/homepage/index.html"},
			wanted: []string{"bumpit modules add web/homepage -r %s -e homepage\n", "left out web/homepage/homepage: it lies in web/homepage"},
		},
		{
			name:   "a directory no module can have",
			tag:    "homepage/v1.0.0",
			files:  []string{"web/[x]/homepage/index.html", "deploy/homepage/nginx.conf"},
			wanted: []string{"bumpit modules add deploy/homepage -r %s -e homepage\n", "left out web/[x]/homepage:", "glob character"},
		},
		{
			name:   "a name a module has, with one directory",
			tag:    "tool-v1.0.0",
			files:  []string{"web/tool/index.html"},
			wanted: []string{"bumpit modules add web/tool -r %s -T tool-v\n"},
		},
		{
			name:   "a name a module has, with two directories",
			tag:    "tool-v1.0.0",
			files:  []string{"web/tool/index.html", "deploy/tool/nginx.conf"},
			wanted: []string{"bumpit modules add would refuse deploy/tool, web/tool:", "the name is the directory of another module"},
		},
	}
	for _, testCase := range testCases {
		repositoryPath := initGoRepository(t)
		for _, file := range testCase.files {
			commitNewFile(t, repositoryPath, file, "content", "feat: add "+file)
		}
		runGit(t, repositoryPath, "tag", "-a", testCase.tag, "-m", testCase.tag)

		output, err := runCLI(binaryPath, "init", "-r", repositoryPath)
		if err != nil {
			t.Errorf("%s: init returned %v\n%s", testCase.name, err, output)
			continue
		}
		for _, wanted := range testCase.wanted {
			if !strings.Contains(output, strings.ReplaceAll(wanted, "%s", repositoryPath)) {
				t.Errorf("%s: init output lacks %q:\n%s", testCase.name, wanted, output)
			}
		}
	}
}

// printedCommands are the bumpit commands an output prints, each as the words
// after bumpit.
func printedCommands(output string) [][]string {
	var commands [][]string
	for _, line := range strings.Split(output, "\n") {
		words := strings.Fields(line)
		if len(words) > 3 && words[0] == "bumpit" && words[1] == "modules" && words[2] == "add" && words[3] != "would" {
			commands = append(commands, words[1:])
		}
	}
	return commands
}

// runPrinted runs, in the order printed, every command the output of init
// prints, which the invariant of init says all succeed.
func runPrinted(t *testing.T, binaryPath, output string) {
	t.Helper()

	runPrintedIn(t, "", binaryPath, output)
}

// runPrintedIn is runPrinted from directory, where init ran.
func runPrintedIn(t *testing.T, directory, binaryPath, output string) {
	t.Helper()

	commands := printedCommands(output)
	if len(commands) == 0 {
		t.Fatalf("init printed no command:\n%s", output)
	}
	for _, command := range commands {
		if commandOutput, err := runCLIIn(directory, binaryPath, command...); err != nil {
			t.Errorf("bumpit %s returned %v:\n%s\nfrom the output of init:\n%s", strings.Join(command, " "), err, commandOutput, output)
		}
	}
}

// TestCLIEndToEndInitPrintsOnlyWhatIsAccepted runs every command init prints,
// in order. Nothing init proposes may be refused by bumpit when followed.
func TestCLIEndToEndInitPrintsOnlyWhatIsAccepted(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)

	t.Run("from the top", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initCandidateRepository(t)
		output, err := runCLI(binaryPath, "init", "-r", repositoryPath)
		if err != nil {
			t.Fatalf("init returned %v\n%s", err, output)
		}
		runPrinted(t, binaryPath, output)
		output, _ = runCLI(binaryPath, "init", "-r", repositoryPath)
		if commands := printedCommands(output); len(commands) != 0 {
			t.Errorf("init still proposes %v:\n%s", commands, output)
		}
	})

	t.Run("from a subdirectory", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initCandidateRepository(t)
		output, err := runCLI(binaryPath, "init", "-r", filepath.Join(repositoryPath, "web"))
		if err != nil {
			t.Fatalf("init returned %v\n%s", err, output)
		}
		runPrinted(t, binaryPath, output)
	})

	t.Run("with no --repo and no file yet, from the top", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initCandidateRepository(t)
		output, err := runCLIIn(repositoryPath, binaryPath, "init")
		if err != nil {
			t.Fatalf("init returned %v\n%s", err, output)
		}
		runPrintedIn(t, repositoryPath, binaryPath, output)
	})

	t.Run("with no --repo and no file yet, from a subdirectory", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initCandidateRepository(t)
		web := filepath.Join(repositoryPath, "web")
		output, err := runCLIIn(web, binaryPath, "init")
		if err != nil {
			t.Fatalf("init returned %v\n%s", err, output)
		}
		runPrintedIn(t, web, binaryPath, output)
	})

	t.Run("with proposals that conflict", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initGoRepository(t)
		commitNewFile(t, repositoryPath, "web/homepage/index.html", "homepage", "feat: add the homepage")
		commitNewFile(t, repositoryPath, "deploy/homepage/nginx.conf", "server", "chore: serve the homepage")
		runGit(t, repositoryPath, "tag", "-a", "homepage/v1.0.0", "-m", "homepage")
		runGit(t, repositoryPath, "tag", "-a", "homepage-v2.0.0", "-m", "homepage")
		commitNewFile(t, repositoryPath, "deploy/both/Chart.yaml", "apiVersion: v2\nname: both\nversion: 1.0.0\n", "chore: chart it")
		commitNewFile(t, repositoryPath, "deploy/both/package.json", `{"name": "both", "version": "1.0.0"}`, "chore: package it")
		output, err := runCLI(binaryPath, "init", "-r", repositoryPath)
		if err != nil {
			t.Fatalf("init returned %v\n%s", err, output)
		}
		runPrinted(t, binaryPath, output)
	})

	t.Run("with a name that from --repo is another module", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initRepository(t)
		commitGoMod(t, repositoryPath, ".", "example.com/repository", "feat: initial library")
		commitGoMod(t, repositoryPath, "web/stack", "example.com/repository/web/stack", "feat: add the stack")
		commitNewFile(t, repositoryPath, "web/homepage/index.html", "homepage", "feat: add the homepage")
		commitFile(t, repositoryPath, "site.go", "package repository\n\nimport _ \"embed\"\n\n//go:embed web/homepage/index.html\nvar page string\n", "feat: serve the homepage")
		runGit(t, repositoryPath, "tag", "-a", "homepage/v1.0.0", "-m", "homepage")
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - name: stack\n    directories: [.]\n")
		web := filepath.Join(repositoryPath, "web")
		output, err := runCLI(binaryPath, "init", "-r", web)
		if err != nil {
			t.Fatalf("init returned %v\n%s", err, output)
		}
		if !strings.Contains(output, "-b ..\n") {
			t.Errorf("init does not call the carrier by its path:\n%s", output)
		}
		runPrinted(t, binaryPath, output)
	})
}

// TestCLIEndToEndInitWritesNothingWhenItCannotPropose covers a failure while it
// makes up the proposals: it stops before it writes, whatever it read until then.
func TestCLIEndToEndInitWritesNothingWhenItCannotPropose(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "pkg/site.go", "package pkg\n", "feat: add the site")
	commitNewFile(t, repositoryPath, "pkg/x/index.html", "x", "feat: add x")
	runGit(t, repositoryPath, "tag", "-a", "x/v1.0.0", "-m", "x")
	if err := os.RemoveAll(filepath.Join(repositoryPath, "pkg")); err != nil {
		t.Fatalf("RemoveAll returned error: %v", err)
	}
	writeFile(t, filepath.Join(repositoryPath, "pkg"), "now a file")

	expectFailure(t, buildCLI(t), []string{"init", "-r", repositoryPath}, "not a directory")
	if _, err := os.Stat(filepath.Join(repositoryPath, "bumpit.yaml")); !os.IsNotExist(err) {
		t.Errorf("the init that failed left a config: %v", err)
	}
}

// TestCLIEndToEndInitStopsAtModulesBelowTheTop covers a config in --repo that
// declares modules, which every run from there stops at: init stops too, rather
// than write the file at the top and propose commands no run could follow.
func TestCLIEndToEndInitStopsAtModulesBelowTheTop(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/index.html", "site", "feat: add the site")
	web := filepath.Join(repositoryPath, "web")
	writeFile(t, filepath.Join(web, "bumpit.yaml"), "modules:\n  - directories: [docs]\n    shippedBy: []\n")
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "init", "-r", web)
	if err == nil || !strings.Contains(output, "declares modules") || !strings.Contains(output, "bumpit modules check -r "+web+" lists all there is to resolve") {
		t.Errorf("init returned %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(repositoryPath, "bumpit.yaml")); !os.IsNotExist(err) {
		t.Errorf("the refused init left a config: %v", err)
	}
}

// TestCLIEndToEndInitThroughALinkToNoFileYet covers a bumpit.yaml that is a link
// to a file that does not exist yet: init creates that file and keeps the link,
// as the commands that edit the config do.
func TestCLIEndToEndInitThroughALinkToNoFileYet(t *testing.T) {
	t.Parallel()

	repositoryPath := initCandidateRepository(t)
	linkPath := filepath.Join(repositoryPath, "bumpit.yaml")
	if err := os.Symlink("shared.yaml", linkPath); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "init", "-r", repositoryPath)
	if err != nil || !strings.Contains(output, "created "+linkPath) || !strings.Contains(output, "bumpit modules add deploy/homepage web/homepage") {
		t.Fatalf("init returned %v\n%s", err, output)
	}
	expectFile(t, filepath.Join(repositoryPath, "shared.yaml"), initialConfig+"modules:\n  - directories: [.]\n")
	if target, err := os.Readlink(linkPath); err != nil || target != "shared.yaml" {
		t.Errorf("the link is now %q, %v", target, err)
	}
	runPrinted(t, binaryPath, output)
}

// TestCLIEndToEndInitSaysWhyItWouldRefuse covers a proposal the command would
// refuse: it says what is wrong of the module, since nothing was edited.
func TestCLIEndToEndInitSaysWhyItWouldRefuse(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/tool/index.html", "content", "feat: add the page")
	commitNewFile(t, repositoryPath, "deploy/tool/nginx.conf", "content", "chore: serve the page")
	runGit(t, repositoryPath, "tag", "-a", "tool-v1.0.0", "-m", "tool")

	output, err := runCLI(buildCLI(t), "init", "-r", repositoryPath)
	if err != nil || !strings.Contains(output, "the name is the directory of another module") {
		t.Fatalf("init returned %v\n%s", err, output)
	}
	if strings.Contains(output, "left as it was") {
		t.Errorf("init speaks of an edit it did not make:\n%s", output)
	}
}

// TestCLIEndToEndInitRehearsesItsCommands covers a file the editor of the
// commands cannot change: init says so, and prints no command it would refuse.
func TestCLIEndToEndInitRehearsesItsCommands(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	runGit(t, repositoryPath, "tag", "-a", "homepage/v1.0.0", "-m", "homepage")
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	content := "modules: [{directories: [web/brand], shippedBy: []}]\n"
	writeFile(t, configPath, content)
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "init", "-r", repositoryPath)
	if err != nil {
		t.Fatalf("init returned %v\n%s", err, output)
	}
	for _, wanted := range []string{"could not list the Go modules . in " + configPath, "modules is written as a list in [...]", "bumpit modules add would refuse web/homepage:"} {
		if !strings.Contains(output, wanted) {
			t.Errorf("init output lacks %q:\n%s", wanted, output)
		}
	}
	if commands := printedCommands(output); len(commands) != 0 {
		t.Errorf("init printed %v, which it would refuse:\n%s", commands, output)
	}
	expectFile(t, configPath, content)

	commitNewFile(t, repositoryPath, "deploy/both/Chart.yaml", "apiVersion: v2\nname: both\nversion: 1.0.0\n", "chore: chart it")
	commitNewFile(t, repositoryPath, "deploy/both/package.json", `{"name": "both", "version": "1.0.0"}`, "chore: package it")
	output, err = runCLI(binaryPath, "init", "-r", repositoryPath)
	if err != nil || strings.Count(output, "bumpit modules add would refuse deploy/both:") != 2 {
		t.Errorf("a proposal refused takes no directory from the next, which says why of its own: %v\n%s", err, output)
	}
}

// TestCLIEndToEndAGoModuleNoEntryCanName covers a Go module in a directory the file
// cannot name: it takes none of the others with it, and nothing sends anyone to
// write an entry every run would refuse, or says that it is versioned as any other,
// since no tag can carry that directory. bumpit still finds it by its go.mod.
func TestCLIEndToEndAGoModuleNoEntryCanName(t *testing.T) {
	t.Parallel()

	const directory = "gen[v2]"
	repositoryPath := initGoRepository(t)
	commitGoMod(t, repositoryPath, directory, "example.com/repository/gen", "feat: add the generator")
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	binaryPath := buildCLI(t)

	output, err := runCLIIn(repositoryPath, binaryPath, "init", "-o", "json")
	var report InitReport
	if decodeErr := json.Unmarshal([]byte(output), &report); err != nil || decodeErr != nil {
		t.Fatalf("init returned %v, %v\n%s", err, decodeErr, output)
	}
	if !slices.Equal(report.Declared, []string{".", "tool"}) || len(report.Unlisted) != 0 {
		t.Errorf("report = %+v, want . and tool listed", report)
	}
	if len(report.Unnameable) != 1 || report.Unnameable[0].Directory != directory || !strings.Contains(report.Unnameable[0].Reason, "glob character") {
		t.Errorf("unnameable = %+v", report.Unnameable)
	}
	content, err := os.ReadFile(configPath)
	if err != nil || !strings.Contains(string(content), "directories: [.]") || !strings.Contains(string(content), "directories: [tool]") || strings.Contains(string(content), "gen") {
		t.Errorf("the file is %s, %v", content, err)
	}

	output, err = runCLIIn(repositoryPath, binaryPath, "init")
	if err != nil || !strings.Contains(output, "no entry can name "+directory) || !strings.Contains(output, "rename the directory") {
		t.Errorf("init returned %v\n%s", err, output)
	}
	if strings.Contains(output, "edit it by hand") || strings.Contains(output, "versioned all the same") {
		t.Errorf("init promises what it cannot:\n%s", output)
	}

	reviewed, passed := readCheck(t, binaryPath, repositoryPath)
	item := itemOf(t, reviewed.Blocking, ruleInvalidTagName)
	if passed || item.Subject != directory || item.Command != "" || !strings.Contains(item.How, "rename its directory") {
		t.Errorf("passed %v, item %+v, want it to fail since no tag can carry the directory", passed, item)
	}
	if countRule(reviewed.Review, ruleGoModuleUnlisted) != 0 {
		t.Errorf("review = %+v, lists again what the renaming that blocks the runs resolves", reviewed.Review)
	}

	output, err = runCLIIn(repositoryPath, binaryPath, "modules", "-d")
	if err != nil || !strings.Contains(output, directory) {
		t.Errorf("modules returned %v\n%s", err, output)
	}
}

// TestRehearseWritesNothing runs a command as the command line does, proof
// included, and leaves the file as it was.
func TestRehearseWritesNothing(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")

	if refused := rehearse(t, newRehearsal(), []string{"modules", "add", "web/homepage", "-r", repositoryPath}); refused != "" {
		t.Fatalf("the rehearsal refused it: %s", refused)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("the rehearsal created %s: %v", configPath, err)
	}

	if refused := rehearse(t, newRehearsal(), []string{"modules", "add", "web/missing", "-r", repositoryPath}); !strings.Contains(refused, "web/missing matches no tracked file") {
		t.Errorf("the rehearsal of a command the proof refuses gave %q", refused)
	}
	if refused := rehearse(t, newRehearsal(), []string{"modules", "add", "web/homepage", "-B", "-r", repositoryPath}); !strings.Contains(refused, "unknown shorthand flag") {
		t.Errorf("the rehearsal of a command the flags refuse gave %q", refused)
	}

	writeFile(t, configPath, "output: xml\n")
	if refused := rehearse(t, newRehearsal(), []string{"config", "set", "output", "text", "-r", repositoryPath}); refused != "" {
		t.Errorf("the rehearsal of a key was refused: %s", refused)
	}
	expectFile(t, configPath, "output: xml\n")
}

func TestCLIEndToEndInitOutsideARepository(t *testing.T) {
	t.Parallel()

	expectFailure(t, buildCLI(t), []string{"init", "-r", t.TempDir()}, "no git repository")
}

// TestCLIEndToEndInitEdges proposes nothing a run owns already or that
// bumpit modules add would refuse, and carries on past a chart it cannot
// read.
func TestCLIEndToEndInitEdges(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitFile(t, repositoryPath, "bumpit.yaml", "tagMatch: release-*\n", "chore: configure bumpit")
	runGit(t, repositoryPath, "tag", "-a", "release-1.0.0", "-m", "release")
	runGit(t, repositoryPath, "tag", "-a", "1.0.0", "-m", "bare")
	if err := os.MkdirAll(filepath.Join(repositoryPath, "-old", "legacy"), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	writeFile(t, filepath.Join(repositoryPath, "-old", "legacy", "index.html"), "legacy")
	runGit(t, repositoryPath, "add", "--", "-old/legacy/index.html")
	runGit(t, repositoryPath, "commit", "-m", "feat: keep the legacy site")
	runGit(t, repositoryPath, "tag", "-a", "legacy/v1.0.0", "-m", "legacy")
	commitNewFile(t, repositoryPath, ".github/actions/setup/action.yml", "name: setup\n", "ci: add the setup action")
	runGit(t, repositoryPath, "tag", "-a", "setup/v1.0.0", "-m", "setup")
	commitNewFile(t, repositoryPath, "web/[legacy]/package.json", `{"name": "glob", "version": "1.0.0"}`, "feat: add a package")
	commitNewFile(t, repositoryPath, "web/string/package.json", `{"name": "string", "version": "1.0.0", "private": "true"}`, "feat: add a private package")
	commitNewFile(t, repositoryPath, "testdata/broken/Chart.yaml", "version: [\n", "test: add a broken chart")
	commitNewFile(t, repositoryPath, "deploy/bad/Chart.yaml", "version: [\n", "chore: add a chart")
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "init", "-r", repositoryPath)
	if err != nil {
		t.Fatalf("init returned error: %v\n%s", err, output)
	}
	for _, wanted := range []string{
		"(no prefix): 1 tag, the newest 1.0.0",
		"bumpit modules add ./-old/legacy -r " + repositoryPath + " -e legacy\n",
		"bumpit modules add .github/actions/setup -r " + repositoryPath + " -e setup\n",
		"web/[legacy]/package.json: version 1.0.0",
		"bumpit modules add would refuse web/[legacy]",
		"glob character",
		"could not read deploy/bad/Chart.yaml",
	} {
		if !strings.Contains(output, wanted) {
			t.Errorf("init output lacks %q:\n%s", wanted, output)
		}
	}
	for _, unwanted := range []string{"release-:", "web/string", "testdata"} {
		if strings.Contains(output, unwanted) {
			t.Errorf("init output holds %q:\n%s", unwanted, output)
		}
	}
	expectContains(t, binaryPath, []string{"modules", "add", "./-old/legacy", "-r", repositoryPath, "-e", "legacy"}, "added legacy")
}
