package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// embeddingSource is a Go file that embeds pattern, of package name.
func embeddingSource(name, pattern string) string {
	return "package " + name + "\n\nimport _ \"embed\"\n\n//go:embed " + pattern + "\nvar content string\n"
}

// readCheck runs bumpit modules check as JSON from directory, and returns its
// report and whether it passed.
func readCheck(t *testing.T, binaryPath, directory string, arguments ...string) (ModulesCheck, bool) {
	t.Helper()

	command := append([]string{"modules", "check", "-o", "json"}, arguments...)
	output, err := runCLIIn(directory, binaryPath, command...)
	var report ModulesCheck
	if decodeErr := json.NewDecoder(strings.NewReader(output)).Decode(&report); decodeErr != nil {
		t.Fatalf("bumpit %v printed no report: %v\n%s", command, decodeErr, output)
	}
	return report, err == nil
}

// itemOf is the item of items with the rule, which must exist.
func itemOf(t *testing.T, items []CheckItem, rule string) CheckItem {
	t.Helper()

	index := slices.IndexFunc(items, func(item CheckItem) bool { return item.Rule == rule })
	if index < 0 {
		t.Fatalf("no item with the rule %s among %+v", rule, items)
	}
	return items[index]
}

func TestCLIEndToEndCheckWithNoFile(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "modules", "check", "-r", repositoryPath)
	if err != nil {
		t.Fatalf("check returned %v\n%s", err, output)
	}
	for _, wanted := range []string{"no bumpit.yaml", "bumpit init -r " + repositoryPath} {
		if !strings.Contains(output, wanted) {
			t.Errorf("check output lacks %q:\n%s", wanted, output)
		}
	}
	if _, err := os.Stat(filepath.Join(repositoryPath, "bumpit.yaml")); !os.IsNotExist(err) {
		t.Errorf("check created a file: %v", err)
	}
}

func TestCLIEndToEndCheckOutsideARepository(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	outside := t.TempDir()
	expectFailure(t, binaryPath, []string{"modules", "check", "-r", outside}, "no git repository")
	expectNoReport(t, binaryPath, "modules", "check", "-r", outside)
	expectNoReport(t, binaryPath, "modules", "check", "-r", outside, "-o", "json")
}

// expectNoReport runs a check that could not ask anything and fails: it prints
// the reason, and no report that could be read as saying all is well.
func expectNoReport(t *testing.T, binaryPath string, arguments ...string) {
	t.Helper()

	output, err := runCLI(binaryPath, arguments...)
	if err == nil || strings.Contains(output, "nothing to resolve") || strings.Contains(output, `"blocking"`) {
		t.Errorf("bumpit %v returned %v\n%s", arguments, err, output)
	}
}

// TestCLIEndToEndCheckOfAFileItCannotRead covers a file nothing else can be
// checked in: one problem, which says what the file takes.
func TestCLIEndToEndCheckOfAFileItCannotRead(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	for _, content := range []string{"modules: [\n", "colour: red\n", "modules:\n  - directory: web\n    shippedBy: []\n"} {
		repositoryPath := initGoRepository(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, content)

		report, passed := readCheck(t, binaryPath, repositoryPath)
		if passed || len(report.Blocking) != 1 || report.Blocking[0].Rule != ruleUnreadableFile || report.Blocking[0].Command != "" {
			t.Errorf("%q: passed %v, report %+v", content, passed, report)
		}
		expectKeysOf[Config](t, report.Blocking[0].How)
		expectKeysOf[ModuleDeclaration](t, report.Blocking[0].How)
		expectFile(t, configPath, content)
	}
}

// expectKeysOf checks that text says every key of the file that type T reads.
func expectKeysOf[T any](t *testing.T, text string) {
	t.Helper()

	fields := reflect.TypeFor[T]()
	for index := range fields.NumField() {
		if key := fields.Field(index).Tag.Get("mapstructure"); !strings.Contains(text, key) {
			t.Errorf("%q does not say the key %s", text, key)
		}
	}
}

// TestCLIEndToEndCheckStopsAtTheFile covers a file that does not hold: what it
// says of the repository is not checked until it does.
func TestCLIEndToEndCheckStopsAtTheFile(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	content := "output: xml\nmodules:\n  - directories: [web/missing]\n    shippedBy: []\n"
	writeFile(t, configPath, content)
	binaryPath := buildCLI(t)

	report, passed := readCheck(t, binaryPath, repositoryPath)
	if passed || !report.FileOnly || len(report.Blocking) != 1 || report.Blocking[0].Subject != "output" {
		t.Fatalf("passed %v, report %+v", passed, report)
	}
	if !strings.HasPrefix(report.Blocking[0].Command, "bumpit config unset output") {
		t.Errorf("command = %q", report.Blocking[0].Command)
	}
	if len(report.Review) != 0 {
		t.Errorf("review = %+v, want none until the file holds", report.Review)
	}
	output, _ := runCLI(binaryPath, "modules", "check", "-r", repositoryPath)
	if !strings.Contains(output, "only once the file holds") {
		t.Errorf("check does not say more may follow:\n%s", output)
	}
	expectFile(t, configPath, content)

	entries := "modules:\n  - directories: [web/missing, web/missing/sub]\n    shippedBy: []\n  - name: site\n    directories: [web, docs]\n"
	writeFile(t, configPath, entries)
	report, passed = readCheck(t, binaryPath, repositoryPath)
	if passed || !report.FileOnly || len(report.Blocking) != 2 {
		t.Errorf("two entries that do not hold: passed %v, report %+v", passed, report)
	}
}

// TestCLIEndToEndCheckListsWhatBlocksTheRuns covers the problems a run stops at
// one by one, all at once, and what each says.
func TestCLIEndToEndCheckListsWhatBlocksTheRuns(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/homepage/index.html", "homepage", "feat: add the homepage")
	commitNewFile(t, repositoryPath, "web/admin/index.html", "admin", "feat: add the admin")
	commitNewFile(t, repositoryPath, "docs/guide.md", "guide", "docs: write the guide")
	commitFile(t, repositoryPath, "site.go", embeddingSource("repository", "web/homepage/index.html"), "feat: serve the homepage")
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	content := "modules:\n" +
		"  - directories: [web/homepage]\n    shippedBy: []\n" +
		"  - directories: [web/admin]\n    tagPrefix: site/v\n    shippedBy: [ghost]\n" +
		"  - directories: [docs]\n    tagPrefix: site/v\n"
	writeFile(t, configPath, content)
	binaryPath := buildCLI(t)

	report, passed := readCheck(t, binaryPath, repositoryPath)
	if passed || report.FileOnly {
		t.Fatalf("passed %v, report %+v", passed, report)
	}
	for _, rule := range []string{ruleEmbedWithoutCarrier, ruleCarrierUnknown, ruleTagPrefixTaken, ruleShippedByMissing} {
		item := itemOf(t, report.Blocking, rule)
		if item.Subject == "" || item.What == "" || item.Why == "" || item.How == "" {
			t.Errorf("%s: item %+v says too little", rule, item)
		}
	}
	if command := itemOf(t, report.Blocking, ruleEmbedWithoutCarrier).Command; !strings.HasPrefix(command, "bumpit modules set web/homepage -b .") {
		t.Errorf("command of the embed = %q", command)
	}
	for _, rule := range []string{ruleCarrierUnknown, ruleTagPrefixTaken, ruleShippedByMissing} {
		if command := itemOf(t, report.Blocking, rule).Command; command != "" {
			t.Errorf("%s: command = %q, want the choice left to whoever reads it", rule, command)
		}
	}
	expectFile(t, configPath, content)

	output, err := runCLI(binaryPath, "modules", "check", "-r", repositoryPath)
	if err == nil || !strings.Contains(output, "block the runs") {
		t.Errorf("check returned %v\n%s", err, output)
	}
	for _, wanted := range []string{"why: ", "how: ", "run: bumpit modules set web/homepage -b ."} {
		if !strings.Contains(output, wanted) {
			t.Errorf("check output lacks %q:\n%s", wanted, output)
		}
	}
}

// TestCLIEndToEndCheckFusesOnlyWhatIsOneProblem covers a conflict two modules
// share, reported once, beside problems that only read alike.
func TestCLIEndToEndCheckFusesOnlyWhatIsOneProblem(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/a/index.html", "a", "feat: add a")
	commitNewFile(t, repositoryPath, "web/b/index.html", "b", "feat: add b")
	commitNewFile(t, repositoryPath, "a/index.html", "a", "feat: add a")
	commitNewFile(t, repositoryPath, "b/index.html", "b", "feat: add b")
	commitFile(t, repositoryPath, "site.go", embeddingSource("repository", "web"), "feat: serve the site")
	binaryPath := buildCLI(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")

	writeFile(t, configPath, "modules:\n  - directories: [web/a]\n    tagPrefix: site/v\n    shippedBy: [.]\n  - directories: [web/b]\n    tagPrefix: site/v\n    shippedBy: [.]\n")
	report, _ := readCheck(t, binaryPath, repositoryPath)
	if conflicts := countRule(report.Blocking, ruleTagPrefixTaken); conflicts != 1 {
		t.Errorf("a conflict of two modules was reported %d times: %+v", conflicts, report.Blocking)
	}

	writeFile(t, configPath, "modules:\n  - directories: [web/a]\n    shippedBy: []\n  - directories: [web/b]\n    shippedBy: []\n")
	report, _ = readCheck(t, binaryPath, repositoryPath)
	if embeds := countRule(report.Blocking, ruleEmbedWithoutCarrier); embeds != 2 {
		t.Errorf("one embed that reaches two modules was reported %d times: %+v", embeds, report.Blocking)
	}

	writeFile(t, configPath, "modules:\n  - name: a\n    directories: [web/a]\n    shippedBy: [.]\n  - directories: [a]\n    shippedBy: []\n  - name: b\n    directories: [web/b]\n    shippedBy: [.]\n  - directories: [b]\n    shippedBy: []\n")
	report, _ = readCheck(t, binaryPath, repositoryPath)
	if names := countRule(report.Blocking, ruleNameIsADirectory); names != 2 {
		t.Errorf("two conflicts of names were reported %d times: %+v", names, report.Blocking)
	}

	writeFile(t, configPath, "modules:\n  - name: b\n    directories: [a]\n    tagPrefix: first/v\n    shippedBy: []\n  - name: a\n    directories: [b]\n    tagPrefix: second/v\n    shippedBy: []\n")
	report, _ = readCheck(t, binaryPath, repositoryPath)
	if names := countRule(report.Blocking, ruleNameIsADirectory); names != 2 {
		t.Errorf("two names that are each other's directories, which take a new name each, were reported %d times: %+v", names, report.Blocking)
	}
}

func countRule(items []CheckItem, rule string) int {
	count := 0
	for _, item := range items {
		if item.Rule == rule {
			count++
		}
	}
	return count
}

// TestCLIEndToEndCheckFindsAGoNameThatIsADirectory covers two Go modules, which
// no check of declared modules reaches.
func TestCLIEndToEndCheckFindsAGoNameThatIsADirectory(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - name: tool\n    directories: [.]\n")
	binaryPath := buildCLI(t)

	report, passed := readCheck(t, binaryPath, repositoryPath)
	if passed || countRule(report.Blocking, ruleNameIsADirectory) != 1 {
		t.Fatalf("a name that is the directory of another Go module: passed %v, report %+v", passed, report)
	}
}

// TestCLIEndToEndANameALinkMakesADirectory covers a name that a link at the top of
// the repository makes the directory of another module, as --module reads it from
// there: the command line could not tell the two apart, so the runs refuse it as
// they refuse a name that is that directory, the check lists it, and no edit writes
// it.
func TestCLIEndToEndANameALinkMakesADirectory(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitNewFile(t, repositoryPath, "web/front/index.html", "front", "feat: add the front")
	commitNewFile(t, repositoryPath, "web/back/index.html", "back", "feat: add the back")
	if err := os.Symlink("web/back", filepath.Join(repositoryPath, "front")); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}
	runGit(t, repositoryPath, "add", "--", "front")
	runGit(t, repositoryPath, "commit", "-m", "chore: link the back")
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	binaryPath := buildCLI(t)

	writeFile(t, configPath, "modules:\n  - name: front\n    directories: [web/front]\n    shippedBy: []\n  - name: back\n    directories: [web/back]\n    shippedBy: []\n")
	expectFailure(t, binaryPath, []string{"next", "-d", "-g", "web/front", "-r", repositoryPath}, "front: the name is the directory of another module")
	report, passed := readCheck(t, binaryPath, repositoryPath)
	item := itemOf(t, report.Blocking, ruleNameIsADirectory)
	if passed || item.Subject != "front" || item.Other != "back" || !strings.Contains(item.Why, "web/back") {
		t.Errorf("passed %v, report %+v", passed, report)
	}

	unnamed := "modules:\n  - directories: [web/front]\n    shippedBy: []\n  - name: back\n    directories: [web/back]\n    shippedBy: []\n"
	writeFile(t, configPath, unnamed)
	expectFailure(t, binaryPath, []string{"modules", "set", "web/front", "-e", "front", "-r", repositoryPath}, "the name is the directory of another module")
	expectFile(t, configPath, unnamed)
}

// TestCLIEndToEndANameALinkGitDoesNotTrackMakesADirectory covers a link only this
// working tree holds: the file says the same in every clone, so the runs and the
// check take the name as it is written, and only the command line here, which
// follows the link, refuses the name read from the top.
func TestCLIEndToEndANameALinkGitDoesNotTrackMakesADirectory(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitNewFile(t, repositoryPath, "web/front/index.html", "front", "feat: add the front")
	commitNewFile(t, repositoryPath, "web/back/index.html", "back", "feat: add the back")
	if err := os.Symlink("web/back", filepath.Join(repositoryPath, "front")); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - name: front\n    directories: [web/front]\n    shippedBy: []\n  - name: back\n    directories: [web/back]\n    shippedBy: []\n")
	binaryPath := buildCLI(t)

	expectContains(t, binaryPath, []string{"next", "-d", "-g", "web/front", "-r", repositoryPath}, "front/v0.1.0")
	if report, _ := readCheck(t, binaryPath, repositoryPath); countRule(report.Blocking, ruleNameIsADirectory) != 0 {
		t.Errorf("blocking = %+v, holds a link git does not track against the file", report.Blocking)
	}
	expectFailure(t, binaryPath, []string{"next", "-d", "-g", "front", "-r", repositoryPath}, "front is the name of one module and the directory web/back of another")
}

// TestCLIEndToEndProposalsFollowTheLinksOfAName covers the commands the check
// proposes beside links at the top of the repository, which the command line
// follows: one calls by its path a module whose name a link makes the directory
// of another, and a proposal gives no name a link makes such a directory.
func TestCLIEndToEndProposalsFollowTheLinksOfAName(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/front/index.html", "front", "feat: add the front")
	commitNewFile(t, repositoryPath, "web/back/index.html", "back", "feat: add the back")
	commitNewFile(t, repositoryPath, "web/site.go", "package web\n\nimport _ \"embed\"\n\n//go:embed front/index.html\nvar page string\n", "feat: serve the front")
	commitNewFile(t, repositoryPath, "deploy/site/index.html", "site", "feat: deploy the site")
	runGit(t, repositoryPath, "tag", "-a", "site/v1.0.0", "-m", "Release site/v1.0.0")
	for _, link := range []string{"front", "site"} {
		if err := os.Symlink("web/back", filepath.Join(repositoryPath, link)); err != nil {
			t.Fatalf("Symlink returned error: %v", err)
		}
	}
	runGit(t, repositoryPath, "add", "--", "front", "site")
	runGit(t, repositoryPath, "commit", "-m", "chore: link the back")
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [.]\n  - directories: [tool]\n  - name: front\n    directories: [web/front]\n    shippedBy: []\n  - name: back\n    directories: [web/back]\n    shippedBy: []\n")
	binaryPath := buildCLI(t)

	report, _ := readCheck(t, binaryPath, repositoryPath)
	if item := itemOf(t, report.Blocking, ruleEmbedWithoutCarrier); !strings.HasPrefix(item.Command, "bumpit modules set web/front ") {
		t.Errorf("the embed: %+v, want the module called by its path", item)
	}
	if item := itemOf(t, report.Review, ruleTagsUnowned); !strings.Contains(item.Command, "-T site/v") || strings.Contains(item.Command, "-e site") {
		t.Errorf("the tag line: %+v, want no name a link makes the directory of another module", item)
	}
}

// TestCLIEndToEndCheckFindsThePrefixOfAModuleOfTheSameName covers a module called
// as the directory of another, which takes that one's tag prefix too: the check
// says both problems, and what it says to do resolves them one at a time.
func TestCLIEndToEndCheckFindsThePrefixOfAModuleOfTheSameName(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/a/index.html", "a", "feat: add a")
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [.]\n  - directories: [tool]\n  - name: tool\n    directories: [web/a]\n    shippedBy: []\n")
	binaryPath := buildCLI(t)

	report, passed := readCheck(t, binaryPath, repositoryPath)
	if passed || countRule(report.Blocking, ruleNameIsADirectory) != 1 || countRule(report.Blocking, ruleTagPrefixTaken) != 1 {
		t.Fatalf("passed %v, report %+v", passed, report)
	}

	expectContains(t, binaryPath, []string{"modules", "set", "web/a", "-e", "site", "-r", repositoryPath}, "changed site")
	report, passed = readCheck(t, binaryPath, repositoryPath)
	if passed || countRule(report.Blocking, ruleNameIsADirectory) != 0 || itemOf(t, report.Blocking, ruleTagPrefixTaken).Subject != "site" {
		t.Fatalf("after the name: passed %v, report %+v", passed, report)
	}

	expectContains(t, binaryPath, []string{"modules", "set", "site", "-T", "site/v", "-r", repositoryPath}, "changed site")
	report, passed = readCheck(t, binaryPath, repositoryPath)
	if !passed || len(report.Blocking) != 0 {
		t.Errorf("after the prefix: passed %v, report %+v", passed, report)
	}
}

// TestCLIEndToEndCheckListsWhatToReview covers what stops no run, which leaves
// the exit code alone.
func TestCLIEndToEndCheckListsWhatToReview(t *testing.T) {
	t.Parallel()

	repositoryPath := initCandidateRepository(t)
	runGit(t, repositoryPath, "tag", "-a", "1.0.0", "-m", "bare")
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	writeFile(t, configPath, "output: text\n")
	binaryPath := buildCLI(t)

	report, passed := readCheck(t, binaryPath, repositoryPath)
	if !passed || len(report.Blocking) != 0 {
		t.Fatalf("passed %v, report %+v", passed, report)
	}
	if item := itemOf(t, report.Review, ruleGoModuleUnlisted); item.Subject != "." || item.Command != "bumpit modules add ." {
		t.Errorf("the Go module with no entry: %+v", item)
	}
	if item := itemOf(t, report.Review, ruleTagsUnowned); item.Subject != "homepage/v" || !strings.Contains(item.Command, "deploy/homepage web/homepage") {
		t.Errorf("the tag line: %+v", item)
	}
	if manifests := countRule(report.Review, ruleManifest); manifests != 2 {
		t.Errorf("%d manifests, want 2: %+v", manifests, report.Review)
	}
	if item := itemOf(t, report.Review, ruleManifest); !strings.Contains(item.How, "if bumpit is to version it") {
		t.Errorf("the manifest says how = %q", item.How)
	}
	if slices.ContainsFunc(report.Review, func(item CheckItem) bool { return item.Subject == "" || item.Subject == "(no prefix)" }) {
		t.Errorf("the tags with no prefix, which nothing can resolve, are listed: %+v", report.Review)
	}
	expectFile(t, configPath, "output: text\n")
}

// TestCLIEndToEndCheckListsAManifestItCannotRead covers what init says too: it
// stops no run, and a report cannot say there is nothing to resolve beside it.
func TestCLIEndToEndCheckListsAManifestItCannotRead(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "deploy/bad/Chart.yaml", "version: [\n", "chore: chart it")
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [.]\n  - directories: [tool]\n")
	binaryPath := buildCLI(t)

	report, passed := readCheck(t, binaryPath, repositoryPath)
	if !passed {
		t.Errorf("a chart it cannot read failed the check: %+v", report)
	}
	item := itemOf(t, report.Review, ruleUnreadableManifest)
	if item.Subject != "deploy/bad/Chart.yaml" || item.What == "" || item.Why == "" || item.How == "" || item.Command != "" {
		t.Errorf("item = %+v", item)
	}
	output, err := runCLI(binaryPath, "modules", "check", "-r", repositoryPath)
	if err != nil || strings.Contains(output, "nothing to resolve") || !strings.Contains(output, "deploy/bad/Chart.yaml") {
		t.Errorf("check returned %v\n%s", err, output)
	}
}

// TestCLIEndToEndCheckShowsWhatAProposalRestsOn covers what init shows of a
// proposal: the directories it left out, and the patterns behind its -b.
func TestCLIEndToEndCheckShowsWhatAProposalRestsOn(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	proposed := func(t *testing.T, content string) string {
		t.Helper()
		repositoryPath := initGoRepository(t)
		commitNewFile(t, repositoryPath, "web/homepage/homepage/index.html", "page", "feat: add the page")
		commitFile(t, repositoryPath, "site.go", embeddingSource("repository", "web/homepage/homepage/index.html"), "feat: serve it")
		runGit(t, repositoryPath, "tag", "-a", "homepage/v1.0.0", "-m", "homepage")
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), content)
		return repositoryPath
	}
	leftOut := LeftOutDirectory{Directory: "web/homepage/homepage", Reason: "it lies in web/homepage"}
	embed := EmbedPlace{Place: "site.go:5:12", Pattern: "web/homepage/homepage/index.html"}

	t.Run("a proposal with a command", func(t *testing.T) {
		t.Parallel()
		repositoryPath := proposed(t, "modules:\n  - directories: [.]\n  - directories: [tool]\n")
		report, _ := readCheck(t, binaryPath, repositoryPath)
		item := itemOf(t, report.Review, ruleTagsUnowned)
		if !slices.Equal(item.LeftOut, []LeftOutDirectory{leftOut}) || !slices.Equal(item.Embeds, []EmbedPlace{embed}) {
			t.Errorf("item = %+v", item)
		}
		output, err := runCLI(binaryPath, "modules", "check", "-r", repositoryPath)
		if err != nil || !strings.Contains(output, "site.go:5:12 embeds web/homepage/homepage/index.html\n") || !strings.Contains(output, "left out web/homepage/homepage: it lies in web/homepage\n") {
			t.Errorf("check returned %v\n%s", err, output)
		}
	})

	t.Run("a proposal it would refuse", func(t *testing.T) {
		t.Parallel()
		repositoryPath := proposed(t, "modules: [{directories: [tool]}]\n")
		report, _ := readCheck(t, binaryPath, repositoryPath)
		item := itemOf(t, report.Review, ruleTagsUnowned)
		if item.Command != "" || !slices.Equal(item.LeftOut, []LeftOutDirectory{leftOut}) || !slices.Equal(item.Embeds, []EmbedPlace{embed}) {
			t.Errorf("item = %+v", item)
		}
		output, _ := runCLI(binaryPath, "modules", "check", "-r", repositoryPath)
		if strings.Contains(output, "embeds web/homepage") || !strings.Contains(output, "left out web/homepage/homepage") {
			t.Errorf("as init does, it shows the patterns with the command alone:\n%s", output)
		}
	})
}

// TestCLIEndToEndTagsOfTheConfigOfTheRun covers tags a run reads as its own,
// because of the config it loads: no module is proposed for them.
func TestCLIEndToEndTagsOfTheConfigOfTheRun(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	tagged := func(t *testing.T) string {
		t.Helper()
		repositoryPath := initGoRepository(t)
		commitNewFile(t, repositoryPath, "app/main.txt", "main", "feat: add the app")
		commitNewFile(t, repositoryPath, "web/index.html", "site", "feat: add the site")
		runGit(t, repositoryPath, "tag", "-a", "app-v1.0.0", "-m", "app")
		return repositoryPath
	}
	expectNoProposal := func(t *testing.T, directory string, arguments ...string) {
		t.Helper()
		output, err := runCLIIn(directory, binaryPath, arguments...)
		if err != nil || strings.Contains(output, "app-v") {
			t.Errorf("bumpit %v returned %v\n%s", arguments, err, output)
		}
	}

	t.Run("given with --config", func(t *testing.T) {
		t.Parallel()
		repositoryPath := tagged(t)
		other := filepath.Join(repositoryPath, "other.yaml")
		writeFile(t, other, "tagMatch: app-v*\nmodules: []\n")
		expectNoProposal(t, repositoryPath, "modules", "check", "-c", other)
	})

	t.Run("found in --repo", func(t *testing.T) {
		t.Parallel()
		repositoryPath := tagged(t)
		web := filepath.Join(repositoryPath, "web")
		writeFile(t, filepath.Join(web, "bumpit.yaml"), "tagMatch: app-v*\n")
		expectNoProposal(t, repositoryPath, "modules", "check", "-r", web)
		expectNoProposal(t, repositoryPath, "init", "-r", web)
	})

	t.Run("at the top, from below", func(t *testing.T) {
		t.Parallel()
		repositoryPath := tagged(t)
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "tagMatch: app-v*\n")
		web := filepath.Join(repositoryPath, "web")
		expectNoProposal(t, repositoryPath, "modules", "check", "-r", web)
		expectNoProposal(t, repositoryPath, "init", "-r", web)
	})
}

// TestCLIEndToEndCheckNamesTheFileThatFailed covers a file it cannot read, or
// cannot get to: the report names that one.
func TestCLIEndToEndCheckNamesTheFileThatFailed(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)

	repositoryPath := initGoRepository(t)
	writeFile(t, filepath.Join(repositoryPath, ".bumpit.yaml"), "modules: [\n")
	report, passed := readCheck(t, binaryPath, repositoryPath)
	if passed || !strings.HasSuffix(report.Config, ".bumpit.yaml") || len(report.Blocking) != 1 || !strings.HasSuffix(report.Blocking[0].Subject, ".bumpit.yaml") {
		t.Errorf("an unreadable .bumpit.yaml: passed %v, report %+v", passed, report)
	}

	inALoop := initGoRepository(t)
	if err := os.Symlink("bumpit.yaml", filepath.Join(inALoop, "bumpit.yaml")); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}
	report, passed = readCheck(t, binaryPath, inALoop)
	if passed || report.Config != "bumpit.yaml" || len(report.Blocking) != 1 || report.Blocking[0].Subject != "bumpit.yaml" {
		t.Errorf("a config in a loop of links: passed %v, report %+v", passed, report)
	}
}

// TestCLIEndToEndARepositoryThatIsAFile covers a --repo that is no directory: it is
// no repository, and no config to edit by hand.
func TestCLIEndToEndARepositoryThatIsAFile(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	plain := filepath.Join(repositoryPath, "plain")
	writeFile(t, plain, "a file\n")
	binaryPath := buildCLI(t)

	for _, arguments := range [][]string{{"modules", "check"}, {"modules", "add", "web"}, {"init"}} {
		expectFailure(t, binaryPath, append(arguments, "-r", plain), "is in no git repository")
	}
	expectNoReport(t, binaryPath, "modules", "check", "-r", plain)
	output, err := runCLI(binaryPath, "next", "-r", plain)
	if err == nil || strings.Contains(output, "bumpit.yaml") {
		t.Errorf("next returned %v\n%s", err, output)
	}
}

// TestCLIEndToEndProposesAManifestARefusedTagLineLeaves covers a tag line whose
// proposal would be refused: it takes no directory from the manifest there, whose
// own proposal would be accepted.
func TestCLIEndToEndProposesAManifestARefusedTagLineLeaves(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/index.html", "site", "feat: add the site")
	commitNewFile(t, repositoryPath, "x/v1/Chart.yaml", "apiVersion: v2\nname: chart\nversion: 1.0.0\n", "chore: chart it")
	runGit(t, repositoryPath, "tag", "-a", "x/v1-2.0.0", "-m", "line")
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [.]\n  - directories: [tool]\n  - name: x\n    directories: [web]\n    shippedBy: []\n")
	binaryPath := buildCLI(t)

	report, _ := readCheck(t, binaryPath, repositoryPath)
	if line := itemOf(t, report.Review, ruleTagsUnowned); line.Command != "" || !strings.Contains(line.How, "would refuse") {
		t.Errorf("the tag line: %+v", line)
	}
	manifest := itemOf(t, report.Review, ruleManifest)
	if manifest.Subject != "x/v1/Chart.yaml" || manifest.Command != "bumpit modules add x/v1" {
		t.Errorf("the manifest: %+v", manifest)
	}
	expectContains(t, binaryPath, []string{"init", "-r", repositoryPath}, "x/v1/Chart.yaml: version 1.0.0", "bumpit modules add x/v1 -r "+repositoryPath+"\n")
	if output, err := runCLIIn(repositoryPath, binaryPath, strings.Fields(manifest.Command)[1:]...); err != nil {
		t.Errorf("%s returned %v\n%s", manifest.Command, err, output)
	}
}

// TestCLIEndToEndCheckSaysWhatItCannotVerify covers its blind spot: a clean
// report does not prove that no build carries a module.
func TestCLIEndToEndCheckSaysWhatItCannotVerify(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [.]\n  - name: site\n    directories: [web/homepage]\n    shippedBy: []\n  - directories: [web/brand]\n    shippedBy: [.]\n")
	binaryPath := buildCLI(t)

	report, passed := readCheck(t, binaryPath, repositoryPath)
	if !passed || !slices.Equal(report.Unverified, []string{"site"}) {
		t.Fatalf("passed %v, report %+v", passed, report)
	}
	output, err := runCLI(binaryPath, "modules", "check", "-r", repositoryPath)
	if err != nil || !strings.Contains(output, "not verified that no module ships site:") || !strings.Contains(output, "bumpit sees only //go:embed") {
		t.Errorf("check returned %v\n%s", err, output)
	}

	// An entry that says nothing of its carriers does not say there are none.
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [.]\n  - directories: [web/brand]\n")
	report, passed = readCheck(t, binaryPath, repositoryPath)
	if passed || len(report.Unverified) != 0 || countRule(report.Blocking, ruleShippedByMissing) != 1 {
		t.Errorf("an entry with no shippedBy: passed %v, report %+v", passed, report)
	}
}

// TestCLIEndToEndCheckReadsThePackages covers a package.json a plan stops at,
// which the discovery of the modules does not read.
func TestCLIEndToEndCheckReadsThePackages(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	commitNewFile(t, repositoryPath, "web/homepage/package.json", `{"name": `, "chore: break the package")
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [web/homepage]\n    shippedBy: []\n")
	binaryPath := buildCLI(t)

	report, passed := readCheck(t, binaryPath, repositoryPath)
	if passed {
		t.Fatalf("an unreadable package passed: %+v", report)
	}
	if item := itemOf(t, report.Blocking, ruleUnreadablePackage); item.Subject != "web/homepage" {
		t.Errorf("item = %+v", item)
	}
	expectContains(t, binaryPath, []string{"modules", "add", "web/brand", "-r", repositoryPath}, "added web/brand")
}

// TestCLIEndToEndCheckStopsAtWhatItCannotRead covers what is no problem of the
// file: bumpit prints what it found and fails with the reason.
func TestCLIEndToEndCheckStopsAtWhatItCannotRead(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitFile(t, repositoryPath, "tool/go.mod", "not a go.mod\n", "chore: break the module")
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "tagMessage: Release\n")
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "modules", "check", "-r", repositoryPath)
	if err == nil || !strings.Contains(output, "go.mod") {
		t.Errorf("check returned %v\n%s", err, output)
	}
	expectNoReport(t, binaryPath, "modules", "check", "-r", repositoryPath)
	expectNoReport(t, binaryPath, "modules", "check", "-r", repositoryPath, "-o", "json")
}

// TestCLIEndToEndCheckKeepsWhatItFoundBeforeAFailure covers a failure halfway: it
// prints all it found until then, fails with the reason, and says nothing of
// what it did not get to.
func TestCLIEndToEndCheckKeepsWhatItFoundBeforeAFailure(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)

	t.Run("in the tag lines", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initGoRepository(t)
		commitNewFile(t, repositoryPath, "tool/assets/logo.svg", "logo", "feat: add the logo")
		commitNewFile(t, repositoryPath, "pkg/site.go", "package pkg\n", "feat: add the site")
		commitNewFile(t, repositoryPath, "pkg/x/index.html", "x", "feat: add x")
		runGit(t, repositoryPath, "tag", "-a", "x/v1.0.0", "-m", "x")
		if err := os.RemoveAll(filepath.Join(repositoryPath, "pkg")); err != nil {
			t.Fatalf("RemoveAll returned error: %v", err)
		}
		writeFile(t, filepath.Join(repositoryPath, "pkg"), "now a file")
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [tool/assets]\n    shippedBy: [ghost]\n")

		report, passed := readCheck(t, binaryPath, repositoryPath)
		if passed || countRule(report.Blocking, ruleCarrierUnknown) != 1 || countRule(report.Review, ruleGoModuleUnlisted) != 2 {
			t.Errorf("passed %v, report %+v", passed, report)
		}
		output, err := runCLI(binaryPath, "modules", "check", "-r", repositoryPath)
		if err == nil || !strings.Contains(output, "not a directory") || strings.Contains(output, "nothing to resolve") {
			t.Errorf("check returned %v\n%s", err, output)
		}
	})

	t.Run("in the manifests", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initGoRepository(t)
		commitNewFile(t, repositoryPath, "tool/a/index.html", "a", "feat: add a")
		commitFile(t, repositoryPath, "site.go", "package repository\n", "feat: add the site")
		commitNewFile(t, repositoryPath, "deploy/chart/Chart.yaml", "apiVersion: v2\nname: chart\nversion: 1.0.0\n", "chore: chart it")
		runGit(t, repositoryPath, "tag", "-a", "a/v1.0.0", "-m", "a")
		sitePath := filepath.Join(repositoryPath, "site.go")
		if err := os.Remove(sitePath); err != nil {
			t.Fatalf("Remove returned error: %v", err)
		}
		if err := os.Symlink("site.go", sitePath); err != nil {
			t.Fatalf("Symlink returned error: %v", err)
		}
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "tagMessage: Release\n")

		report, passed := readCheck(t, binaryPath, repositoryPath)
		if passed || countRule(report.Review, ruleTagsUnowned) != 1 || countRule(report.Review, ruleManifest) != 0 {
			t.Errorf("passed %v, report %+v", passed, report)
		}
	})
}

// TestCLIEndToEndCheckWithNoFileThatInitWouldStopAt covers the command the check
// prints with no file: it is rehearsed as any other, and what init could not read
// fails the check, which then sends no one to init, nor init back to the check.
func TestCLIEndToEndCheckWithNoFileThatInitWouldStopAt(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)

	t.Run("a go.mod it cannot read", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initGoRepository(t)
		commitFile(t, repositoryPath, "tool/go.mod", "not a go.mod\n", "chore: break the module")

		output, err := runCLI(binaryPath, "modules", "check", "-r", repositoryPath)
		if err == nil || !strings.Contains(output, "declares no module path") || strings.Contains(output, "bumpit init") {
			t.Errorf("check returned %v\n%s", err, output)
		}
		output, err = runCLI(binaryPath, "init", "-r", repositoryPath)
		if err == nil || !strings.Contains(output, "declares no module path") || strings.Contains(output, "modules check") {
			t.Errorf("init returned %v\n%s", err, output)
		}
	})

	t.Run("a proposal it cannot make up", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initGoRepository(t)
		commitNewFile(t, repositoryPath, "pkg/site.go", "package pkg\n", "feat: add the site")
		commitNewFile(t, repositoryPath, "pkg/x/index.html", "x", "feat: add x")
		runGit(t, repositoryPath, "tag", "-a", "x/v1.0.0", "-m", "x")
		if err := os.RemoveAll(filepath.Join(repositoryPath, "pkg")); err != nil {
			t.Fatalf("RemoveAll returned error: %v", err)
		}
		writeFile(t, filepath.Join(repositoryPath, "pkg"), "now a file")

		output, err := runCLI(binaryPath, "modules", "check", "-r", repositoryPath)
		if err == nil || !strings.Contains(output, "not a directory") || strings.Contains(output, "refuse") || strings.Contains(output, "bumpit init") {
			t.Errorf("check returned %v\n%s", err, output)
		}
		if _, err := os.Stat(filepath.Join(repositoryPath, "bumpit.yaml")); !os.IsNotExist(err) {
			t.Errorf("the check created a file: %v", err)
		}
	})
}

func TestCLIEndToEndCheckOfAFileWithNothingToResolve(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	content := "# ours\nmodules:\n  - directories: [.]\n  - directories: [tool]\n"
	writeFile(t, configPath, content)
	binaryPath := buildCLI(t)

	output, err := runCLI(binaryPath, "modules", "check", "-r", repositoryPath)
	if err != nil || !strings.Contains(output, "nothing to resolve") {
		t.Errorf("check returned %v\n%s", err, output)
	}
	expectFile(t, configPath, content)
}

// resolvePrinted runs every command the check prints, in order, each of which
// must be accepted, until it prints no more.
func resolvePrinted(t *testing.T, binaryPath, directory string, arguments ...string) ModulesCheck {
	t.Helper()

	for range 5 {
		report, _ := readCheck(t, binaryPath, directory, arguments...)
		var commands []string
		for _, item := range append(slices.Clone(report.Blocking), report.Review...) {
			if item.Command != "" {
				commands = append(commands, item.Command)
			}
		}
		if len(commands) == 0 {
			return report
		}
		for _, command := range commands {
			words := strings.Fields(command)
			if output, err := runCLIIn(directory, binaryPath, words[1:]...); err != nil {
				t.Fatalf("%s returned %v\n%s", command, err, output)
			}
		}
	}
	t.Fatalf("the commands of the check do not converge")
	return ModulesCheck{}
}

// initShippedRepository is a repository whose root module embeds web/homepage,
// with the Go module tool and the directories web/brand and tool/assets.
func initShippedRepository(t *testing.T) string {
	t.Helper()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/homepage/index.html", "homepage", "feat: add the homepage")
	commitNewFile(t, repositoryPath, "web/brand/logo.svg", "logo", "feat: draw the logo")
	commitNewFile(t, repositoryPath, "tool/assets/logo.svg", "logo", "feat: add the logo")
	commitFile(t, repositoryPath, "site.go", embeddingSource("repository", "web/homepage/index.html"), "feat: serve the homepage")
	commitFile(t, repositoryPath, "tool/logo.go", embeddingSource("tool", "assets/logo.svg"), "feat: embed the logo")
	return repositoryPath
}

// TestCLIEndToEndCheckPrintsOnlyWhatIsAccepted follows every command the check
// prints, from each place a run can start. None may be refused, and following
// them must leave nothing they can resolve.
func TestCLIEndToEndCheckPrintsOnlyWhatIsAccepted(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	onlyEmbed := "modules:\n  - directories: [web/homepage]\n    shippedBy: []\n"

	t.Run("from the top", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, "output: xml\n"+onlyEmbed)
		report := resolvePrinted(t, binaryPath, repositoryPath)
		if len(report.Blocking) != 0 || len(report.Review) != 0 {
			t.Errorf("report = %+v, want nothing left", report)
		}
		expectFile(t, configPath, "modules:\n  - directories: [web/homepage]\n    shippedBy: [.]\n  - directories: [.]\n  - directories: [tool]\n")
	})

	t.Run("keeping the carriers the file gives", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, "modules:\n  - directories: [web/brand]\n    shippedBy: []\n  - directories: [web/homepage]\n    shippedBy: [web/brand]\n")
		report := resolvePrinted(t, binaryPath, repositoryPath)
		if len(report.Blocking) != 0 {
			t.Errorf("report = %+v", report)
		}
		content, err := os.ReadFile(configPath)
		if err != nil || !strings.Contains(string(content), "shippedBy: [web/brand, .]") {
			t.Errorf("the file is now %s, %v", content, err)
		}
	})

	t.Run("with a module two Go modules ship", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, "modules:\n  - name: site\n    directories: [web/homepage, tool/assets]\n    shippedBy: []\n")
		report := resolvePrinted(t, binaryPath, repositoryPath)
		if len(report.Blocking) != 0 {
			t.Errorf("report = %+v", report)
		}
		content, err := os.ReadFile(configPath)
		if err != nil || !strings.Contains(string(content), "shippedBy: [., tool]") {
			t.Errorf("the file is now %s, %v", content, err)
		}
	})

	t.Run("from a subdirectory", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), onlyEmbed)
		report := resolvePrinted(t, binaryPath, repositoryPath, "-r", filepath.Join(repositoryPath, "web"))
		if len(report.Blocking) != 0 || len(report.Review) != 0 {
			t.Errorf("report = %+v", report)
		}
	})

	t.Run("with a config that declares modules", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		alternative := filepath.Join(repositoryPath, "alternative.yaml")
		writeFile(t, alternative, onlyEmbed)
		report := resolvePrinted(t, binaryPath, repositoryPath, "-c", alternative)
		if len(report.Blocking) != 0 || len(report.Review) != 0 {
			t.Errorf("report = %+v", report)
		}
		if _, err := os.Stat(filepath.Join(repositoryPath, "bumpit.yaml")); !os.IsNotExist(err) {
			t.Errorf("the commands wrote the file at the top: %v", err)
		}
	})

	t.Run("with a config that declares modules, from a subdirectory", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		alternative := filepath.Join(repositoryPath, "alternative.yaml")
		writeFile(t, alternative, onlyEmbed)
		report := resolvePrinted(t, binaryPath, repositoryPath, "-c", alternative, "-r", filepath.Join(repositoryPath, "web"))
		if len(report.Blocking) != 0 || len(report.Review) != 0 {
			t.Errorf("report = %+v", report)
		}
	})

	t.Run("with a config silent on the modules", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		topPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, topPath, onlyEmbed)
		silent := filepath.Join(repositoryPath, "silent.yaml")
		writeFile(t, silent, "tagMessage: Release\n")
		report := resolvePrinted(t, binaryPath, repositoryPath, "-c", silent)
		if len(report.Blocking) != 0 || len(report.Review) != 0 {
			t.Errorf("report = %+v", report)
		}
		content, err := os.ReadFile(topPath)
		if err != nil || !strings.Contains(string(content), "shippedBy: [.]") {
			t.Errorf("the top file is now %s, %v", content, err)
		}
		expectFile(t, silent, "tagMessage: Release\n")
	})

	t.Run("with a repository key", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		commitNewFile(t, repositoryPath, "sub/notes.txt", "notes", "docs: write the notes")
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "repository: sub\n"+onlyEmbed)
		report := resolvePrinted(t, binaryPath, repositoryPath)
		if len(report.Blocking) != 0 || len(report.Review) != 0 {
			t.Errorf("report = %+v", report)
		}
	})

	t.Run("with two keys to take out", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, "output: xml\nrepository: ./missing\n"+onlyEmbed)
		report, _ := readCheck(t, binaryPath, repositoryPath)
		if !report.FileOnly || len(report.Blocking) != 2 || report.Blocking[0].Command == "" || report.Blocking[1].Command == "" {
			t.Fatalf("report = %+v, want a command for each key", report)
		}
		report = resolvePrinted(t, binaryPath, repositoryPath)
		if len(report.Blocking) != 0 || len(report.Review) != 0 {
			t.Errorf("report = %+v, want nothing left", report)
		}
	})

	t.Run("with a repository key that leads nowhere", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, "repository: ./missing\n"+onlyEmbed)
		report, _ := readCheck(t, binaryPath, repositoryPath)
		if !report.FileOnly || len(report.Blocking) != 1 || !strings.Contains(report.Blocking[0].Command, "unset repository") {
			t.Fatalf("report = %+v", report)
		}
		resolvePrinted(t, binaryPath, repositoryPath)
	})
}

// TestCLIEndToEndCheckProposesBesideAConflictOfNames covers a conflict the file
// holds already: it is no reason to refuse a proposal it has nothing to do with,
// which the command would accept.
func TestCLIEndToEndCheckProposesBesideAConflictOfNames(t *testing.T) {
	t.Parallel()

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/a/index.html", "a", "feat: add a")
	commitNewFile(t, repositoryPath, "docs/guide.md", "guide", "docs: write the guide")
	runGit(t, repositoryPath, "tag", "-a", "docs/v1.0.0", "-m", "docs")
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - name: tool\n    directories: [web/a]\n    shippedBy: []\n")
	binaryPath := buildCLI(t)

	report, _ := readCheck(t, binaryPath, repositoryPath)
	item := itemOf(t, report.Review, ruleTagsUnowned)
	if item.Subject != "docs/v" || item.Command != "bumpit modules add docs" {
		t.Fatalf("item = %+v, want the command that declares docs", item)
	}
	if output, err := runCLIIn(repositoryPath, binaryPath, strings.Fields(item.Command)[1:]...); err != nil {
		t.Errorf("%s returned %v\n%s", item.Command, err, output)
	}
}

// TestCLIEndToEndCheckLeavesOutWhatItWouldRefuse covers commands that would be
// refused or would need a choice: there are none in the report, and it says why.
func TestCLIEndToEndCheckLeavesOutWhatItWouldRefuse(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)

	t.Run("an entry with a comment", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [web/homepage] # the site\n    shippedBy: []\n")
		report, _ := readCheck(t, binaryPath, repositoryPath)
		item := itemOf(t, report.Blocking, ruleEmbedWithoutCarrier)
		if item.Command != "" || !strings.Contains(item.How, "edit it by hand") || !strings.Contains(item.How, "comment") {
			t.Errorf("item = %+v", item)
		}
	})

	t.Run("a carrier that is no module", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [web/homepage]\n    shippedBy: [ghost]\n")
		report, _ := readCheck(t, binaryPath, repositoryPath)
		if item := itemOf(t, report.Blocking, ruleEmbedWithoutCarrier); item.Command != "" {
			t.Errorf("item = %+v", item)
		}
	})

	t.Run("a key the file at the top holds and a run from below does not read", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initShippedRepository(t)
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "colour: red\n"+"modules:\n  - directories: [web/homepage]\n    shippedBy: []\n")
		report, _ := readCheck(t, binaryPath, repositoryPath, "-r", filepath.Join(repositoryPath, "web"))
		item := itemOf(t, report.Blocking, ruleEmbedWithoutCarrier)
		if item.Command != "" || !strings.Contains(item.How, "invalid keys: colour") {
			t.Errorf("item = %+v", item)
		}
	})

	t.Run("a Go module whose directory another module is named after", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initGoRepository(t)
		commitNewFile(t, repositoryPath, "web/index.html", "site", "feat: add the site")
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [.]\n  - name: tool\n    directories: [web]\n    shippedBy: []\n")
		report, _ := readCheck(t, binaryPath, repositoryPath)
		item := itemOf(t, report.Review, ruleGoModuleUnlisted)
		if item.Command == "" {
			t.Fatalf("item = %+v", item)
		}
		if output, err := runCLIIn(repositoryPath, binaryPath, strings.Fields(item.Command)[1:]...); err != nil {
			t.Errorf("%s returned %v\n%s", item.Command, err, output)
		}
	})
}

// TestCLIEndToEndInitPointsAtTheCheck covers what init says when it stops: the
// command that lists every problem, from where init ran.
func TestCLIEndToEndInitPointsAtTheCheck(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	for _, content := range []string{"output: xml\n", "colour: red\n", "modules: [\n"} {
		repositoryPath := initGoRepository(t)
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), content)

		expectFailure(t, binaryPath, []string{"init", "-r", repositoryPath}, "bumpit modules check -r "+repositoryPath+" lists all there is to resolve")
		output, err := runCLIIn(repositoryPath, binaryPath, "init")
		if err == nil || !strings.Contains(output, "bumpit modules check lists all there is to resolve") {
			t.Errorf("%q: init returned %v\n%s", content, err, output)
		}
	}

	// A key of the file at the top, which a check from below does not read: the
	// command points at the check that lists it.
	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/index.html", "site", "feat: add the site")
	writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "preRelease: 'bad version!'\n")
	expectFailure(t, binaryPath, []string{"init", "-r", filepath.Join(repositoryPath, "web")}, "bumpit modules check -r "+repositoryPath+" lists all there is to resolve")
	report, passed := readCheck(t, binaryPath, repositoryPath, "-r", repositoryPath)
	if item := itemOf(t, report.Blocking, ruleInvalidKey); passed || item.Subject != "preRelease" {
		t.Errorf("the check it points at: passed %v, item %+v", passed, item)
	}
}

// TestCLIEndToEndCheckLeavesAConflictOfKeysToAChoice covers two keys that cannot be
// used together: which one goes is not for a command to say, and following the
// commands printed cannot take out a key that holds.
func TestCLIEndToEndCheckLeavesAConflictOfKeysToAChoice(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)

	t.Run("beside a value that is wrong", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initGoRepository(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		writeFile(t, configPath, "preRelease: 'bad!'\npromote: true\n")
		report, _ := readCheck(t, binaryPath, repositoryPath)
		if len(report.Blocking) != 2 || report.Blocking[0].Command != "bumpit config unset preRelease" {
			t.Fatalf("report = %+v", report)
		}
		if conflict := report.Blocking[1]; conflict.Subject != "promote" || conflict.Other != "preRelease" || conflict.Command != "" || !strings.Contains(conflict.How, "one of them") {
			t.Errorf("the conflict: %+v", conflict)
		}
		resolvePrinted(t, binaryPath, repositoryPath)
		content, err := os.ReadFile(configPath)
		if err != nil || !strings.Contains(string(content), "promote: true") {
			t.Errorf("the file is now %s, %v", content, err)
		}
	})

	t.Run("with two values that hold", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initGoRepository(t)
		configPath := filepath.Join(repositoryPath, "bumpit.yaml")
		content := "preRelease: rc\npromote: true\n"
		writeFile(t, configPath, content)
		report, passed := readCheck(t, binaryPath, repositoryPath)
		if passed || len(report.Blocking) != 1 || report.Blocking[0].Command != "" {
			t.Errorf("passed %v, report %+v", passed, report)
		}
		expectFile(t, configPath, content)
	})
}

// TestCLIEndToEndProofReadsThePackages covers a package.json a plan of the module
// stops at: a module that would read it is refused, which init says in place of
// the command, and a module beside it is not.
func TestCLIEndToEndProofReadsThePackages(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	commitNewFile(t, repositoryPath, "web/homepage/package.json", `{"name": `, "chore: break the package")
	runGit(t, repositoryPath, "tag", "-a", "web/homepage/v1.0.0", "-m", "homepage")
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	binaryPath := buildCLI(t)

	expectFailure(t, binaryPath, []string{"modules", "add", "web/homepage", "-r", repositoryPath}, "a package.json it reads cannot be read")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("the refused add left %s: %v", configPath, err)
	}
	expectContains(t, binaryPath, []string{"init", "-r", repositoryPath}, "bumpit modules add would refuse web/homepage:", "a package.json it reads cannot be read")
	expectContains(t, binaryPath, []string{"modules", "add", "web/brand", "-r", repositoryPath}, "added web/brand")

	commitFile(t, repositoryPath, "web/homepage/package.json", `{"name": "homepage"}`, "fix: repair the package")
	expectContains(t, binaryPath, []string{"modules", "add", "web/homepage", "-r", repositoryPath}, "added web/homepage")
}

// TestCLIEndToEndCheckGivesATagLineToTheModuleOfItsDirectory covers a module that
// left its tags behind, as a Go module with a name does when its go.mod goes: the
// tags its directory names can only be given to it with a new prefix.
func TestCLIEndToEndCheckGivesATagLineToTheModuleOfItsDirectory(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	leftBehind := func(t *testing.T) (string, string) {
		t.Helper()
		repositoryPath := initGoRepository(t)
		commitGoMod(t, repositoryPath, "sub", "example.com/repository/sub", "feat: add sub")
		commitNewFile(t, repositoryPath, "sub/notes.txt", "notes", "feat: add the notes")
		runGit(t, repositoryPath, "tag", "-a", "sub/v1.2.3", "-m", "sub")
		runGit(t, repositoryPath, "rm", "-q", "sub/go.mod")
		runGit(t, repositoryPath, "commit", "-q", "-m", "chore: drop the go.mod")
		return repositoryPath, filepath.Join(repositoryPath, "bumpit.yaml")
	}

	t.Run("a module with no tags of its own", func(t *testing.T) {
		t.Parallel()
		repositoryPath, configPath := leftBehind(t)
		writeFile(t, configPath, "modules:\n  - directories: [.]\n  - directories: [tool]\n  - name: core\n    directories: [sub]\n    shippedBy: []\n")

		report, _ := readCheck(t, binaryPath, repositoryPath)
		item := itemOf(t, report.Review, ruleTagsUnowned)
		if item.Subject != "sub/v" || item.Command != "bumpit modules set core -T sub/v" {
			t.Fatalf("item = %+v", item)
		}
		output, err := runCLIIn(repositoryPath, binaryPath, "init")
		if err != nil || !strings.Contains(output, "bumpit modules set core -T sub/v\n") {
			t.Errorf("init returned %v\n%s", err, output)
		}
		if output, err := runCLIIn(repositoryPath, binaryPath, strings.Fields(item.Command)[1:]...); err != nil {
			t.Fatalf("%s returned %v\n%s", item.Command, err, output)
		}
		expectOutput(t, binaryPath, []string{"latest", "-g", "core", "-r", repositoryPath}, "sub/v1.2.3")
		report, _ = readCheck(t, binaryPath, repositoryPath)
		if countRule(report.Review, ruleTagsUnowned) != 0 {
			t.Errorf("the tags are still unowned: %+v", report.Review)
		}
	})

	t.Run("a module with tags of its own", func(t *testing.T) {
		t.Parallel()
		repositoryPath, configPath := leftBehind(t)
		runGit(t, repositoryPath, "tag", "-a", "new/v1.0.0", "-m", "new")
		writeFile(t, configPath, "modules:\n  - directories: [.]\n  - directories: [tool]\n  - directories: [sub]\n    tagPrefix: new/v\n    shippedBy: []\n")

		report, _ := readCheck(t, binaryPath, repositoryPath)
		if item := itemOf(t, report.Review, ruleTagsUnowned); item.Subject != "sub/v" || item.Command != "" {
			t.Errorf("item = %+v, want the choice left to whoever reads it", item)
		}
	})

	t.Run("a module the command cannot give them to", func(t *testing.T) {
		t.Parallel()
		repositoryPath, configPath := leftBehind(t)
		content := "modules:\n  - directories: [.]\n  - directories: [tool]\n  - name: core\n    # the shared code\n    directories: [sub]\n    shippedBy: []\n"
		writeFile(t, configPath, content)

		report, _ := readCheck(t, binaryPath, repositoryPath)
		item := itemOf(t, report.Review, ruleTagsUnowned)
		if item.Command != "" || !strings.Contains(item.How, "bumpit modules set would refuse") || !strings.Contains(item.How, "holds a comment") {
			t.Errorf("item = %+v, want why the command would be refused", item)
		}
		if strings.Contains(item.How, "bumpit modules add") {
			t.Errorf("item = %+v, sends to a command for a directory that has a module", item)
		}

		output, err := runCLIIn(repositoryPath, binaryPath, "init")
		if err != nil || !strings.Contains(output, "bumpit modules set would refuse") || !strings.Contains(output, "holds a comment") {
			t.Errorf("init returned %v\n%s", err, output)
		}
		if strings.Contains(output, "no directory free of a module") || strings.Contains(output, "bumpit modules set core") {
			t.Errorf("init says what the refusal contradicts:\n%s", output)
		}

		output, err = runCLIIn(repositoryPath, binaryPath, "init", "-o", "json")
		var initReport InitReport
		if decodeErr := json.Unmarshal([]byte(output), &initReport); err != nil || decodeErr != nil {
			t.Fatalf("init returned %v, %v\n%s", err, decodeErr, output)
		}
		if line := initReport.TagLines[0]; line.Command != "" || !strings.Contains(line.Refused, "holds a comment") {
			t.Errorf("tag line = %+v", line)
		}
	})
}

// TestCLIEndToEndAnEmbedIsOneProblemOfEachModule covers a pattern that reaches
// several directories of one module: one thing to resolve, said once. Another
// pattern and another module are problems of their own.
func TestCLIEndToEndAnEmbedIsOneProblemOfEachModule(t *testing.T) {
	t.Parallel()

	binaryPath := buildCLI(t)
	withApps := func(t *testing.T, source string) string {
		t.Helper()
		repositoryPath := initGoRepository(t)
		commitNewFile(t, repositoryPath, "apps/a/index.html", "a", "feat: add a")
		commitNewFile(t, repositoryPath, "apps/b/index.html", "b", "feat: add b")
		commitNewFile(t, repositoryPath, "site.go", source, "feat: embed the apps")
		return repositoryPath
	}
	const oneModule = "modules:\n  - name: web\n    directories: [apps/a, apps/b]\n    shippedBy: []\n"

	t.Run("one pattern and one module", func(t *testing.T) {
		t.Parallel()
		repositoryPath := withApps(t, embeddingSource("repository", "apps"))
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), oneModule)

		report, _ := readCheck(t, binaryPath, repositoryPath)
		if count := countRule(report.Blocking, ruleEmbedWithoutCarrier); count != 1 {
			t.Fatalf("%d items of the embed: %+v", count, report.Blocking)
		}
		if item := itemOf(t, report.Blocking, ruleEmbedWithoutCarrier); item.Command != "bumpit modules set web -b ." {
			t.Errorf("item = %+v", item)
		}
		output, _ := runCLIIn(repositoryPath, binaryPath, "modules", "check")
		if !strings.Contains(output, "1 problem blocks the runs") {
			t.Errorf("check output:\n%s", output)
		}
	})

	t.Run("two patterns and one module", func(t *testing.T) {
		t.Parallel()
		source := "package repository\n\nimport \"embed\"\n\n//go:embed apps/a\nvar first embed.FS\n\n//go:embed apps/b\nvar second embed.FS\n"
		repositoryPath := withApps(t, source)
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), oneModule)

		report, _ := readCheck(t, binaryPath, repositoryPath)
		if count := countRule(report.Blocking, ruleEmbedWithoutCarrier); count != 2 {
			t.Errorf("%d items of the embeds: %+v", count, report.Blocking)
		}
	})

	t.Run("one pattern and two modules", func(t *testing.T) {
		t.Parallel()
		repositoryPath := withApps(t, embeddingSource("repository", "apps"))
		writeFile(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [apps/a]\n    shippedBy: []\n  - directories: [apps/b]\n    shippedBy: []\n")

		report, _ := readCheck(t, binaryPath, repositoryPath)
		if count := countRule(report.Blocking, ruleEmbedWithoutCarrier); count != 2 {
			t.Errorf("%d items of the embed: %+v", count, report.Blocking)
		}
	})

	t.Run("a proposal", func(t *testing.T) {
		t.Parallel()
		repositoryPath := initGoRepository(t)
		commitNewFile(t, repositoryPath, "services/a/web/index.html", "a", "feat: add a")
		commitNewFile(t, repositoryPath, "services/b/web/index.html", "b", "feat: add b")
		commitNewFile(t, repositoryPath, "site.go", embeddingSource("repository", "services"), "feat: embed the services")
		runGit(t, repositoryPath, "tag", "-a", "web/v1.0.0", "-m", "web")

		output, err := runCLIIn(repositoryPath, binaryPath, "init", "-o", "json")
		var report InitReport
		if decodeErr := json.Unmarshal([]byte(output), &report); err != nil || decodeErr != nil {
			t.Fatalf("init returned %v, %v\n%s", err, decodeErr, output)
		}
		candidate := report.TagLines[0].Candidate
		if candidate == nil || len(candidate.Embeds) != 1 || candidate.Embeds[0].Pattern != "services" {
			t.Errorf("candidate = %+v, want the pattern once", candidate)
		}
	})
}
