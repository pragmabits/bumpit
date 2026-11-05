package app

import (
	"slices"
	"strings"
)

// The rules a config can break, stable names a report and a proof can tell
// apart whatever the text of a finding says.
const (
	ruleUnreadableFile         = "unreadable-file"
	ruleModulesBelowTop        = "modules-below-top"
	ruleInvalidKey             = "invalid-key"
	ruleNoDirectories          = "no-directories"
	ruleInvalidPath            = "invalid-path"
	ruleDirectoryListedTwice   = "directory-listed-twice"
	ruleDirectoryInside        = "directory-inside"
	ruleDirectoryDeclaredTwice = "directory-declared-twice"
	ruleNameNeeded             = "name-needed"
	ruleInvalidName            = "invalid-name"
	ruleNameGivenTwice         = "name-given-twice"
	rulePathIsOwnDirectory     = "path-is-own-directory"
	ruleInvalidTagPrefix       = "invalid-tag-prefix"
	ruleRootTagPrefix          = "root-tag-prefix"
	ruleShippedByItself        = "shipped-by-itself"
	ruleGoEntryDirectories     = "go-entry-directories"
	ruleGoEntryKeys            = "go-entry-keys"
	ruleShippedByMissing       = "shipped-by-missing"
	ruleNameIsADirectory       = "name-is-a-directory"
	ruleUntracked              = "untracked"
	rulePathInsideOwnDirectory = "path-inside-own-directory"
	ruleCarrierUnknown         = "carrier-unknown"
	ruleCycle                  = "cycle"
	ruleTagPrefixOption        = "tag-prefix-option"
	ruleTagPrefixTaken         = "tag-prefix-taken"
	ruleTagPrefixContinues     = "tag-prefix-continues"
	ruleInvalidTagName         = "invalid-tag-name"
	ruleEmbedWithoutCarrier    = "embed-without-carrier"
	ruleUnreadablePackage      = "unreadable-package"
	ruleNoFile                 = "no-file"
	ruleGoModuleUnlisted       = "go-module-unlisted"
	ruleTagsUnowned            = "tags-unowned"
	ruleManifest               = "manifest"
	ruleUnreadableManifest     = "unreadable-manifest"
)

// Finding is something a config says that bumpit refuses or points out: what
// is wrong, why it matters, and how to resolve it. The runs stop at the first
// one, as an error; bumpit modules check lists them all. What does not name the
// module it is about, so that it stays the same problem when that module changes
// its name. How may say which command resolves it, never the words to run, since a
// finding does not know whether the module it speaks of was written: those are for
// a CheckItem, which rehearses them.
type Finding struct {
	Rule string `json:"rule"`
	// Subject is what the finding is about: the name of a module or a key.
	Subject string `json:"subject,omitempty"`
	// Other is the other module or key of a conflict between two.
	Other string `json:"other,omitempty"`
	What  string `json:"what"`
	Why   string `json:"why,omitempty"`
	How   string `json:"how,omitempty"`
	// missingCarrier is the Go module an embed asks shippedBy to name.
	missingCarrier string
}

// Error is the finding on one line, as a run reports it.
func (f Finding) Error() string {
	var text strings.Builder
	if f.Subject != "" {
		text.WriteString(f.Subject + ": ")
	}
	text.WriteString(f.What)
	if f.Why != "" {
		text.WriteString(": " + f.Why)
	}
	if f.How != "" {
		text.WriteString("; " + f.How)
	}
	return text.String()
}

// firstFinding is the first of findings as an error, the one a run stops at,
// nil when there is none.
func firstFinding(findings []Finding) error {
	if len(findings) == 0 {
		return nil
	}
	return findings[0]
}

// addedFinding is a finding of after that the edit behind it added, nil when it
// added none: one about what the edit is about that no finding before says of
// it, or one more than before about something else. An edit may leave what it
// did not come to fix, so that a file can be repaired one problem at a time, and
// each finding before answers for one after, so that it cannot add more of a
// problem the file holds.
func addedFinding(before, after []Finding, subjects []editSubject) error {
	type place struct{ rule, subject string }
	counts := map[place]int{}
	for _, finding := range before {
		counts[place{rule: finding.Rule, subject: finding.Subject}]++
	}
	unanswered := slices.Clone(before)
	for _, finding := range after {
		index := slices.IndexFunc(subjects, func(subject editSubject) bool { return subject.current == finding.Subject })
		if index >= 0 {
			earlier := slices.IndexFunc(unanswered, func(earlier Finding) bool {
				return earlier.Rule == finding.Rule && earlier.What == finding.What && subjects[index].calls(earlier.Subject)
			})
			if earlier < 0 {
				return finding
			}
			unanswered = slices.Delete(unanswered, earlier, earlier+1)
			continue
		}
		key := place{rule: finding.Rule, subject: finding.Subject}
		if counts[key] == 0 {
			return finding
		}
		counts[key]--
	}
	return nil
}
