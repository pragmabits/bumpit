package app

import (
	"slices"
	"strings"
)

// declaredModuleFindings lists what the repository contradicts of each declared
// module, in the order of the modules.
func declaredModuleFindings(snapshot *repositorySnapshot, tracked trackedTree, modules []repositoryModule) ([]Finding, error) {
	var findings []Finding
	for _, module := range modules {
		if !module.withoutGoMod() {
			continue
		}
		found, err := checkDeclaredModule(snapshot, tracked, module, modules)
		if err != nil {
			return nil, err
		}
		findings = append(findings, found...)
	}
	return findings, nil
}

// packageFindings lists the declared modules whose Node.js packages cannot be read,
// which a plan of the module stops at.
func packageFindings(snapshot *repositorySnapshot, top string, tracked trackedTree, modules []repositoryModule) []Finding {
	var findings []Finding
	for _, module := range modules {
		if !module.withoutGoMod() {
			continue
		}
		for _, directory := range module.directories {
			if err := snapshot.packageError(top, tracked, directory); err != nil {
				findings = append(findings, Finding{
					Rule:    ruleUnreadablePackage,
					Subject: module.name,
					What:    "a package.json it reads cannot be read",
					Why:     err.Error(),
					How:     "repair it: bumpit cannot know what the module reads without it",
				})
				break
			}
		}
	}
	return findings
}

// nameFindings lists a name that is the directory of another module as a command
// line reads it from the top of the repository, through the links the repository
// tracks: the commands could not tell the two apart, and shippedBy calls a module
// with no name of its own by its directory. A link only this working tree holds
// is no problem of the file, which says the same in every clone. An error is a
// failure to ask.
func nameFindings(snapshot *repositorySnapshot, modules []repositoryModule) ([]Finding, error) {
	var findings []Finding
	for index, module := range modules {
		if !module.named {
			continue
		}
		directory, err := snapshot.directoryOfName(module.name)
		if err != nil {
			return nil, err
		}
		for otherIndex, other := range modules {
			if otherIndex != index && other.owns(directory) {
				findings = append(findings, Finding{
					Rule:    ruleNameIsADirectory,
					Subject: module.name,
					Other:   other.name,
					What:    "the name is the directory of another module",
					Why:     whyNameIsADirectory(module.name, directory, other.name),
					How:     "give the module another name",
				})
			}
		}
	}
	return findings, nil
}

// whyNameIsADirectory says why name cannot be told from the module other, whose
// directory it is, through a link when that directory is not the name itself.
func whyNameIsADirectory(name, directory, other string) string {
	if directory == name {
		return "the commands could not tell it from " + other
	}
	return "a link makes it " + directory + ", so the commands could not tell it from " + other
}

// checkDeclaredModule lists what the repository contradicts of the module. An
// error is a failure to ask, no finding of the module.
func checkDeclaredModule(snapshot *repositorySnapshot, tracked trackedTree, module repositoryModule, modules []repositoryModule) ([]Finding, error) {
	var findings []Finding
	for _, entry := range append(slices.Clone(module.directories), module.paths...) {
		if !tracked.holds(entry) {
			findings = append(findings, Finding{
				Rule:    ruleUntracked,
				Subject: module.name,
				What:    entry + " matches no tracked file",
				Why:     "no commit of it would ever be read",
				How:     "commit it, or correct the path",
			})
		}
	}
	for _, entry := range module.paths {
		if module.holds(entry) && !insideNestedModule(entry, module, modules) {
			findings = append(findings, Finding{
				Rule:    rulePathInsideOwnDirectory,
				Subject: module.name,
				What:    "paths lists " + entry + ", inside its own directory",
				Why:     "the module reads it already",
				How:     "take it out of paths",
			})
		}
	}
	findings = append(findings, carrierFindings(module, modules)...)
	prefixFindings, err := tagPrefixFindings(snapshot, module, modules)
	return append(findings, prefixFindings...), err
}

// carrierFindings lists a carrier that is no module of the repository and a
// cycle of carriers: a module that, through the modules it is shipped by,
// carries itself, which no release can mean.
func carrierFindings(module repositoryModule, modules []repositoryModule) []Finding {
	var findings []Finding
	for _, carrier := range module.shippedBy {
		if moduleNamed(modules, carrier) < 0 {
			findings = append(findings, Finding{
				Rule:    ruleCarrierUnknown,
				Subject: module.name,
				What:    "shippedBy lists " + carrier + ", which is no module of the repository",
				Why:     "shippedBy calls a module by its name, or by its directory when it has none",
				How:     "correct it, or take it out of shippedBy",
			})
		}
	}
	if carriersOf(module, modules)[module.name] {
		findings = append(findings, Finding{
			Rule:    ruleCycle,
			Subject: module.name,
			What:    "shippedBy forms a cycle",
			Why:     "through " + strings.Join(module.shippedBy, ", ") + ", the module carries itself",
			How:     "take out the carrier that closes it",
		})
	}
	return findings
}

// tagPrefixFindings lists a tag prefix that cannot tell the tags of the module
// from those of another: the same prefix, or one that continues the other with
// what can start a version, as v1 continues v, which makes v10.0.0 a version of
// both. It also lists a prefix git takes in no tag name.
func tagPrefixFindings(snapshot *repositorySnapshot, module repositoryModule, modules []repositoryModule) ([]Finding, error) {
	prefix := module.tagPrefix
	if strings.HasPrefix(prefix, "-") {
		return []Finding{{
			Rule:    ruleTagPrefixOption,
			Subject: module.name,
			What:    "tag prefix " + prefix + " starts with -",
			Why:     "git takes it for an option",
			How:     "give the module another prefix",
		}}, nil
	}

	var findings []Finding
	for _, other := range modules {
		if finding, conflict := prefixConflict(module, other); conflict {
			findings = append(findings, finding)
		}
	}
	renamed, err := snapshot.takesNoTagName(module)
	if err != nil {
		return nil, err
	}
	if renamed {
		findings = append(findings, invalidTagNameFinding(module))
	}
	return findings, nil
}

// invalidTagNameFinding is the finding of a module whose tag prefix makes no tag
// name git takes. The prefix of a Go module comes from its directory, and that of a
// declared module can be given.
func invalidTagNameFinding(module repositoryModule) Finding {
	how := "give the module another prefix"
	if !module.withoutGoMod() {
		how = "rename its directory, which gives it the prefix"
	}
	return Finding{
		Rule:    ruleInvalidTagName,
		Subject: module.name,
		What:    "tag prefix " + module.tagPrefix + " does not make a valid tag name",
		Why:     "git would refuse every tag of the module",
		How:     how,
	}
}

// prefixConflict is the finding of the tag prefix of module against that of
// other, the same whichever of the two reports it. A module has no conflict with
// itself, and two with the same name are still two: their directories tell them
// apart.
func prefixConflict(module, other repositoryModule) (Finding, bool) {
	finding := Finding{Subject: module.name, Other: other.name, How: "give one of them another prefix, which leaves its tags behind"}
	switch prefix := module.tagPrefix; {
	case other.name == module.name && slices.Equal(other.directories, module.directories):
		return Finding{}, false
	case other.tagPrefix == prefix:
		finding.Rule = ruleTagPrefixTaken
		finding.What = "tag prefix " + prefix + " is taken by another module"
		finding.Why = "it is the one of " + other.name + " too, so their tags could not be told apart"
	case continuesWithVersion(prefix, other.tagPrefix):
		finding.Rule = ruleTagPrefixContinues
		finding.What = "tag prefix " + prefix + " continues " + other.tagPrefix
		finding.Why = other.tagPrefix + " is the one of " + other.name + ", and what follows it can start a version, so a tag could be a version of both"
	case continuesWithVersion(other.tagPrefix, prefix):
		finding.Rule = ruleTagPrefixContinues
		finding.What = "tag prefix " + other.tagPrefix + " continues " + prefix
		finding.Why = other.tagPrefix + " is the one of " + other.name + ", and what follows " + prefix + " can start a version, so a tag could be a version of both"
	default:
		return Finding{}, false
	}
	return finding, true
}

// continuesWithVersion reports whether longer is shorter followed by what can
// start a semantic version: a digit, then only characters a version holds.
func continuesWithVersion(longer, shorter string) bool {
	rest, found := strings.CutPrefix(longer, shorter)
	if !found || rest == "" || rest[0] < '0' || rest[0] > '9' {
		return false
	}
	return !strings.ContainsFunc(rest, func(character rune) bool { return !isVersionCharacter(character) })
}

func isVersionCharacter(character rune) bool {
	switch {
	case character >= '0' && character <= '9', character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z':
		return true
	default:
		return strings.ContainsRune(".+-", character)
	}
}
