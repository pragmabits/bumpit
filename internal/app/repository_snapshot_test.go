package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/pragmabits/bumpit/internal/gitx"
)

// gitCommandOfTrace is the git command in a line of GIT_TRACE.
var gitCommandOfTrace = regexp.MustCompile(`trace: built-in: git ([a-z-]+)`)

// TestRepositoryFindingsReadsNoCodeOfTheModules covers what a proof asks, twice for
// each edit: what is wrong of the modules. What they read, which only a plan and
// the check of their packages need, is not read for it.
func TestRepositoryFindingsReadsNoCodeOfTheModules(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	commitNewFile(t, repositoryPath, "web/homepage/page.go", "package page\n", "feat: add the page")
	declarations := []ModuleDeclaration{{Directories: []string{"web/homepage"}, ShippedBy: &[]string{}}}

	report, err := repositoryFindings(newRepositorySnapshot(gitx.New(repositoryPath)), declarations)
	if err != nil {
		t.Fatalf("repositoryFindings returned error: %v", err)
	}
	for _, module := range report.modules {
		if module.goCode != nil || module.node != nil {
			t.Errorf("module %s has what it reads: %+v", module.name, module)
		}
	}
}

// TestCLIEndToEndReadsTheRepositoryOnce covers what a command costs: it reads the
// repository once, however many edits it proves and proposals it rehearses, and
// asks git only of a prefix it has not asked about. It sets GIT_TRACE, so it does
// not run in parallel.
func TestCLIEndToEndReadsTheRepositoryOnce(t *testing.T) {
	const proposals = 6
	// goModules are those of initGoRepository, whose prefixes only init, of those no
	// entry lists, and the check ask of.
	const goModules = 2

	binaryPath := buildCLI(t)
	repositoryPath := initGoRepository(t)
	for index := range proposals {
		chart := fmt.Sprintf("apiVersion: v2\nname: chart%d\nversion: 1.0.0\n", index)
		commitNewFile(t, repositoryPath, fmt.Sprintf("charts/chart%d/Chart.yaml", index), chart, "chore: add a chart")
	}
	tracePath := filepath.Join(t.TempDir(), "trace")
	t.Setenv("GIT_TRACE", tracePath)

	testCases := []struct {
		arguments []string
		prefixes  int
	}{
		{arguments: []string{"init"}, prefixes: proposals + goModules},
		{arguments: []string{"modules", "check"}, prefixes: proposals + goModules},
		{arguments: []string{"modules", "add", "charts/chart0"}, prefixes: 1},
		{arguments: []string{"modules", "set", "charts/chart0", "-T", "first/v"}, prefixes: 2},
	}
	for _, testCase := range testCases {
		if err := os.WriteFile(tracePath, nil, 0o644); err != nil {
			t.Fatalf("WriteFile returned error: %v", err)
		}
		if output, err := runCLIIn(repositoryPath, binaryPath, testCase.arguments...); err != nil {
			t.Fatalf("bumpit %v returned %v\n%s", testCase.arguments, err, output)
		}
		trace, err := os.ReadFile(tracePath)
		if err != nil {
			t.Fatalf("ReadFile returned error: %v", err)
		}
		counts := map[string]int{}
		for _, match := range gitCommandOfTrace.FindAllStringSubmatch(string(trace), -1) {
			counts[match[1]]++
		}
		if counts["ls-files"] > 2 || counts["rev-parse"] > 2 || counts["check-ref-format"] > testCase.prefixes {
			t.Errorf("bumpit %v ran git %v times, want at most 2 ls-files, 2 rev-parse and %d check-ref-format", testCase.arguments, counts, testCase.prefixes)
		}
	}
}
