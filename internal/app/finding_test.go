package app

import (
	"strings"
	"testing"

	"github.com/pragmabits/bumpit/internal/gitx"
)

func TestFindingError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		finding Finding
		want    string
	}{
		{finding: Finding{Subject: "web", What: "it is wrong", Why: "because", How: "fix it"}, want: "web: it is wrong: because; fix it"},
		{finding: Finding{What: "it is wrong", Why: "because", How: "fix it"}, want: "it is wrong: because; fix it"},
		{finding: Finding{Subject: "web", What: "it is wrong", How: "fix it"}, want: "web: it is wrong; fix it"},
		{finding: Finding{Subject: "web", What: "it is wrong", Why: "because"}, want: "web: it is wrong: because"},
		{finding: Finding{What: "it is wrong"}, want: "it is wrong"},
	}
	for _, testCase := range testCases {
		if got := testCase.finding.Error(); got != testCase.want {
			t.Errorf("Error() = %q, want %q", got, testCase.want)
		}
	}
}

// TestAddedFinding covers what a proof counts as new: of what the edit is about,
// what no finding before says of it, and each finding answers for one; of the
// rest, one more than there was.
func TestAddedFinding(t *testing.T) {
	t.Parallel()

	ghost := func(subject string) Finding {
		return Finding{Rule: ruleCarrierUnknown, Subject: subject, What: "shippedBy lists ghost"}
	}
	testCases := []struct {
		name     string
		before   []Finding
		after    []Finding
		subjects []editSubject
		added    bool
	}{
		{name: "a new module with the problem of another", before: []Finding{ghost("web/a")}, after: []Finding{ghost("web/a"), ghost("web/b")}, subjects: []editSubject{{current: "web/b"}}, added: true},
		{name: "a new module called as one that has the problem", before: []Finding{ghost("tool")}, after: []Finding{ghost("tool")}, subjects: []editSubject{{current: "tool"}}},
		{name: "a module that keeps its problem under another name", before: []Finding{ghost("page")}, after: []Finding{ghost("site")}, subjects: []editSubject{{current: "site", former: "page"}}},
		{name: "a new name with the problem of another module", before: []Finding{ghost("other")}, after: []Finding{ghost("other"), ghost("site")}, subjects: []editSubject{{current: "site", former: "page"}}, added: true},
		{name: "a problem the file holds, one more time", before: []Finding{ghost("site")}, after: []Finding{ghost("site"), ghost("site")}, subjects: []editSubject{{current: "site"}}, added: true},
		{name: "a problem of the module, said otherwise", before: []Finding{ghost("site")}, after: []Finding{{Rule: ruleCarrierUnknown, Subject: "site", What: "shippedBy lists ghost2"}}, subjects: []editSubject{{current: "site", former: "site"}}, added: true},
		{name: "a problem that stays on another module", before: []Finding{ghost("other")}, after: []Finding{ghost("other")}, subjects: []editSubject{{current: "site"}}},
		{name: "one more problem on another module", before: []Finding{ghost("other")}, after: []Finding{ghost("other"), ghost("other")}, subjects: []editSubject{{current: "site"}}, added: true},
		{name: "a problem gone", before: []Finding{ghost("site")}, subjects: []editSubject{{current: "site", former: "site"}}},
	}
	for _, testCase := range testCases {
		if err := addedFinding(testCase.before, testCase.after, testCase.subjects); (err != nil) != testCase.added {
			t.Errorf("%s: addedFinding returned %v", testCase.name, err)
		}
	}
}

// TestEveryRefusalSaysWhatWhyAndHow covers each rule a file can break: the
// report has what to say of each, and what it calls wrong names no module,
// since names change while the problem stays.
func TestEveryRefusalSaysWhatWhyAndHow(t *testing.T) {
	t.Parallel()

	declarations := []ModuleDeclaration{
		{ShippedBy: &[]string{}},
		{Name: "zzname", Directories: []string{"/absolute"}, ShippedBy: &[]string{}},
		{Name: "zzclean", Directories: []string{"web/"}, ShippedBy: &[]string{}},
		{Name: "zzleaves", Directories: []string{"../web"}, ShippedBy: &[]string{}},
		{Name: "zzmagic", Directories: []string{":(exclude)web"}, ShippedBy: &[]string{}},
		{Name: "zzglob", Directories: []string{"web/*"}, ShippedBy: &[]string{}},
		{Name: "zztwice", Directories: []string{"a", "a"}, ShippedBy: &[]string{}},
		{Name: "zzinside", Directories: []string{"a", "a/b"}, ShippedBy: &[]string{}},
		{Directories: []string{"c", "d"}, ShippedBy: &[]string{}},
		{Name: ".", Directories: []string{"e"}, ShippedBy: &[]string{}},
		{Name: "zzown", Directories: []string{"f"}, Paths: []string{"f"}, ShippedBy: &[]string{}},
		{Name: "zzprefix", Directories: []string{"g"}, TagPrefix: "g*", ShippedBy: &[]string{}},
		{Name: "zzroot", Directories: []string{"h"}, TagPrefix: "v", ShippedBy: &[]string{}},
		{Name: "zzself", Directories: []string{"i"}, ShippedBy: &[]string{"zzself"}},
		{Name: "zzcarrier", Directories: []string{"j"}, ShippedBy: &[]string{"./k"}},
		{Name: "zzcarriedtwice", Directories: []string{"m"}, ShippedBy: &[]string{"n", "n"}},
		{Name: "zzsame", Directories: []string{"j"}, ShippedBy: &[]string{}},
		{Name: "zzsame", Directories: []string{"l"}, ShippedBy: &[]string{}},
	}
	findings := declarationFindings(declarations)
	rules := []string{ruleNoDirectories, ruleInvalidPath, ruleDirectoryListedTwice, ruleDirectoryInside, ruleNameNeeded, ruleInvalidName, rulePathIsOwnDirectory, ruleInvalidTagPrefix, ruleRootTagPrefix, ruleShippedByItself, ruleCarrierListedTwice, ruleDirectoryDeclaredTwice, ruleNameGivenTwice}
	checkFindings(t, findings, rules)

	repositoryPath := initGoRepository(t)
	commitNewFile(t, repositoryPath, "web/homepage/index.html", "homepage", "feat: add the homepage")
	commitNewFile(t, repositoryPath, "web/admin/index.html", "admin", "feat: add the admin")
	commitNewFile(t, repositoryPath, "web/brand/logo.svg", "logo", "feat: draw the logo")
	commitNewFile(t, repositoryPath, "web/deck/slide.txt", "slide", "feat: add the deck")
	commitNewFile(t, repositoryPath, "web/form/form.txt", "form", "feat: add the form")
	commitNewFile(t, repositoryPath, "web/gate/gate.txt", "gate", "feat: add the gate")
	commitNewFile(t, repositoryPath, "web/hall/hall.txt", "hall", "feat: add the hall")
	commitFile(t, repositoryPath, "site.go", embeddingSource("repository", "web/homepage/index.html"), "feat: serve the homepage")

	entries := []ModuleDeclaration{
		{Name: "zzkeys", Directories: []string{"tool"}, TagPrefix: "tools/v"},
		{Name: "zzmissing", Directories: []string{"web/deck"}},
		{Name: "zzembed", Directories: []string{"web/homepage"}, ShippedBy: &[]string{}},
		{Name: "tool", Directories: []string{"web/admin"}, ShippedBy: &[]string{}},
		{Name: "zzuntracked", Directories: []string{"web/gone"}, ShippedBy: &[]string{}},
		{Name: "zzpath", Directories: []string{"web/brand"}, Paths: []string{"web/brand/logo.svg"}, ShippedBy: &[]string{}},
		{Name: "zzghost", Directories: []string{"web/form"}, ShippedBy: &[]string{"ghost"}},
		{Name: "zzcyclea", Directories: []string{"web/gate"}, TagPrefix: "gate/v", ShippedBy: &[]string{"zzcycleb"}},
		{Name: "zzcycleb", Directories: []string{"web/hall"}, TagPrefix: "gate/v1", ShippedBy: &[]string{"zzcyclea"}},
	}
	report, err := repositoryFindings(newRepositorySnapshot(gitx.New(repositoryPath)), entries)
	if err != nil {
		t.Fatalf("repositoryFindings returned error: %v", err)
	}
	checkFindings(t, report.findings, []string{ruleGoEntryKeys, ruleShippedByMissing, ruleEmbedWithoutCarrier, ruleNameIsADirectory, ruleUntracked, rulePathInsideOwnDirectory, ruleCarrierUnknown, ruleCycle, ruleTagPrefixContinues})

	several := []ModuleDeclaration{{Name: "zzgo", Directories: []string{"tool", "web/brand"}}}
	report, err = repositoryFindings(newRepositorySnapshot(gitx.New(repositoryPath)), several)
	if err != nil {
		t.Fatalf("repositoryFindings returned error: %v", err)
	}
	checkFindings(t, report.findings, []string{ruleGoEntryDirectories})

	prefixes := []ModuleDeclaration{
		{Name: "zzoption", Directories: []string{"web/brand"}, TagPrefix: "-x/v", ShippedBy: &[]string{}},
		{Name: "zzinvalid", Directories: []string{"web/deck"}, TagPrefix: "a..b/v", ShippedBy: &[]string{}},
		{Name: "zztaken", Directories: []string{"web/form"}, TagPrefix: "tool/v", ShippedBy: &[]string{}},
	}
	report, err = repositoryFindings(newRepositorySnapshot(gitx.New(repositoryPath)), prefixes)
	if err != nil {
		t.Fatalf("repositoryFindings returned error: %v", err)
	}
	checkFindings(t, report.findings, []string{ruleTagPrefixOption, ruleInvalidTagName, ruleTagPrefixTaken})
}

// checkFindings checks that findings holds rules, and that each finding says
// what, why and how, and names no module in what it calls wrong.
func checkFindings(t *testing.T, findings []Finding, rules []string) {
	t.Helper()

	seen := map[string]bool{}
	for _, finding := range findings {
		seen[finding.Rule] = true
		if finding.Rule == "" || finding.What == "" || finding.Why == "" || finding.How == "" {
			t.Errorf("finding %+v says too little", finding)
		}
		if strings.Contains(finding.What, "zz") {
			t.Errorf("finding %+v names a module in what it calls wrong", finding)
		}
	}
	for _, rule := range rules {
		if !seen[rule] {
			t.Errorf("no finding with the rule %s among %+v", rule, findings)
		}
	}
}

// TestConfigKeyFindings covers the keys of the file, each of which is checked
// whatever the others hold.
func TestConfigKeyFindings(t *testing.T) {
	t.Parallel()

	promote := true
	findings := configKeyFindings(Config{Repository: "/does/not/exist", TagMatch: "--no-merged=HEAD", Output: "xml", PreRelease: "not a label!", Promote: &promote, StartVersion: "nope"})
	subjects := map[string]bool{}
	for _, finding := range findings {
		subjects[finding.Subject] = true
		if finding.Rule != ruleInvalidKey || finding.What == "" || finding.Why == "" || finding.How == "" {
			t.Errorf("finding %+v says too little", finding)
		}
	}
	for _, key := range []string{"repository", "tagMatch", "output", "preRelease", "promote", "startVersion"} {
		if !subjects[key] {
			t.Errorf("no finding of %s among %+v", key, findings)
		}
	}
	if findings := configKeyFindings(Config{Repository: ".", Output: "text"}); len(findings) != 0 {
		t.Errorf("findings of a valid config: %+v", findings)
	}
}
