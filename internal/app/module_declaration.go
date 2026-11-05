package app

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// ModuleDeclaration is a module of the repository as bumpit.yaml lists it under
// modules: directories released on their own, with tags and commits of their
// own, or a Go module, which the entry only names.
type ModuleDeclaration struct {
	// Name is what --module, shippedBy and the output call the module. Empty
	// means its only directory, and several directories need one.
	Name string `mapstructure:"name"`
	// Directories are the directories the module owns, slash separated and
	// relative to the top of the repository: "." for the root.
	Directories []string `mapstructure:"directories"`
	// TagPrefix is what every version tag of the module starts with, its "v"
	// included. Empty means the name or the directory followed by "/v", or "v"
	// for the root.
	TagPrefix string `mapstructure:"tagPrefix"`
	// Paths are the files and directories outside Directories that the module
	// is built from: a commit touching one of them changes the module too.
	Paths []string `mapstructure:"paths"`
	// ShippedBy lists the modules whose release also carries this one, which
	// then read its commits, by their names, or by their directories when they
	// have none. A module with no go.mod requires it, since bumpit cannot always
	// tell, and a Go module takes none: nil means bumpit.yaml leaves it out.
	ShippedBy *[]string `mapstructure:"shippedBy"`
}

// prefix is the tag prefix of the module, the default one when bumpit.yaml
// gives none.
func (d ModuleDeclaration) prefix() string {
	switch name := d.identity(); {
	case d.TagPrefix != "":
		return d.TagPrefix
	case name == ".":
		return "v"
	default:
		return name + "/v"
	}
}

// identity is what the commands and shippedBy call the module: its name, else
// its only directory.
func (d ModuleDeclaration) identity() string {
	switch {
	case d.Name != "":
		return d.Name
	case len(d.Directories) == 0:
		return ""
	default:
		return strings.Join(d.Directories, ", ")
	}
}

// listsDirectory reports whether a declaration of declarations owns directory.
func listsDirectory(declarations []ModuleDeclaration, directory string) bool {
	return slices.ContainsFunc(declarations, func(declaration ModuleDeclaration) bool {
		return slices.Contains(declaration.Directories, directory)
	})
}

// validateModuleDeclarations refuses what can be told wrong from bumpit.yaml
// alone. What the repository must confirm, such as a go.mod in the directory
// or the files a path names, is checked when the modules are discovered.
func validateModuleDeclarations(declarations []ModuleDeclaration) error {
	if err := firstFinding(declarationFindings(declarations)); err != nil {
		return fmt.Errorf("modules: %w", err)
	}
	return nil
}

// declarationFindings lists what can be told wrong from the declarations alone:
// what each of them says, and what two of them say together.
func declarationFindings(declarations []ModuleDeclaration) []Finding {
	var findings []Finding
	directories := map[string]bool{}
	names := map[string]bool{}
	for _, declaration := range declarations {
		findings = append(findings, declarationTextFindings(declaration)...)
		if declaration.Name != "" && names[declaration.Name] {
			findings = append(findings, Finding{
				Rule:    ruleNameGivenTwice,
				Subject: declaration.Name,
				What:    "the name is given twice",
				Why:     "the commands could not tell the two modules apart",
				How:     "give one of them another name",
			})
		}
		names[declaration.Name] = true
		for _, directory := range declaration.Directories {
			if directories[directory] {
				findings = append(findings, Finding{
					Rule:    ruleDirectoryDeclaredTwice,
					Subject: declaration.identity(),
					What:    directory + " is declared twice",
					Why:     "a directory belongs to one module, which owns its commits",
					How:     "leave it to one of them",
				})
			}
			directories[directory] = true
		}
	}
	return findings
}

// declarationTextFindings lists what can be told wrong from one declaration
// alone, in the order a run reports them.
func declarationTextFindings(declaration ModuleDeclaration) []Finding {
	findings := directoriesFindings(declaration)
	findings = append(findings, nameFindingsOf(declaration)...)
	findings = append(findings, pathsFindings(declaration)...)
	findings = append(findings, tagPrefixTextFindings(declaration)...)
	return append(findings, shippedByTextFindings(declaration)...)
}

func directoriesFindings(declaration ModuleDeclaration) []Finding {
	if len(declaration.Directories) == 0 {
		return []Finding{{
			Rule:    ruleNoDirectories,
			Subject: declaration.identity(),
			What:    "directories cannot be empty",
			Why:     "a module owns at least one directory",
			How:     "list what it owns under directories",
		}}
	}
	var findings []Finding
	for _, directory := range declaration.Directories {
		findings = append(findings, modulePathFindings(declaration.identity(), "directory", directory)...)
	}
	return append(findings, directoriesApartFindings(declaration)...)
}

// directoriesApartFindings lists a directory listed twice in a declaration, and
// one that lies in another of its directories, which owns it already.
func directoriesApartFindings(declaration ModuleDeclaration) []Finding {
	var findings []Finding
	for index, directory := range declaration.Directories {
		if slices.Contains(declaration.Directories[:index], directory) {
			findings = append(findings, Finding{
				Rule:    ruleDirectoryListedTwice,
				Subject: declaration.identity(),
				What:    directory + " is listed twice",
				Why:     "a module owns a directory once",
				How:     "list it once",
			})
		}
		for _, other := range declaration.Directories {
			if nestedIn(directory, other) {
				findings = append(findings, Finding{
					Rule:    ruleDirectoryInside,
					Subject: declaration.identity(),
					What:    directory + " lies in " + other,
					Why:     "the module owns it already",
					How:     "leave out " + directory,
				})
			}
		}
	}
	return findings
}

// nameFindingsOf lists a name the commands could not tell from a path, and
// several directories with no name, since none of them says what the module
// is.
func nameFindingsOf(declaration ModuleDeclaration) []Finding {
	switch {
	case declaration.Name == "" && len(declaration.Directories) > 1:
		return []Finding{{
			Rule:    ruleNameNeeded,
			Subject: declaration.identity(),
			What:    "several directories need a name",
			Why:     "none of them says what the module is",
			How:     "give the module a name",
		}}
	case declaration.Name == "":
		return nil
	case declaration.Name == ".":
		return []Finding{{
			Rule:    ruleInvalidName,
			Subject: declaration.Name,
			What:    fmt.Sprintf("name %q is a directory", declaration.Name),
			Why:     "the commands could not tell it from the root",
			How:     "give the module another name",
		}}
	case strings.HasPrefix(declaration.Name, "-"):
		return []Finding{{
			Rule:    ruleInvalidName,
			Subject: declaration.Name,
			What:    fmt.Sprintf("name %q starts with -", declaration.Name),
			Why:     "the commands would read it as a flag",
			How:     "give the module another name",
		}}
	default:
		return modulePathFindings(declaration.identity(), "name", declaration.Name)
	}
}

func pathsFindings(declaration ModuleDeclaration) []Finding {
	var findings []Finding
	for _, entry := range declaration.Paths {
		findings = append(findings, modulePathFindings(declaration.identity(), "paths", entry)...)
		if slices.Contains(declaration.Directories, entry) {
			findings = append(findings, Finding{
				Rule:    rulePathIsOwnDirectory,
				Subject: declaration.identity(),
				What:    "paths lists " + entry + ", its own directory",
				Why:     "the module reads it already",
				How:     "take it out of paths",
			})
		}
	}
	return findings
}

// modulePathFindings lists what makes value, a path of a declaration or a name
// the commands read beside paths, one git cannot read as a plain path relative
// to the top of the repository.
func modulePathFindings(subject, key, value string) []Finding {
	finding := Finding{Rule: ruleInvalidPath, Subject: subject}
	switch {
	case value == "":
		finding.What = key + " cannot be empty"
		finding.Why, finding.How = "it would name nothing", "write what it is, or leave it out"
	case strings.HasPrefix(value, "/"):
		finding.What = fmt.Sprintf("%s %q is absolute", key, value)
		finding.Why, finding.How = "bumpit.yaml travels with the repository", "write it relative to the top of the repository"
	case path.Clean(value) != value:
		finding.What = fmt.Sprintf("%s %q is not clean", key, value)
		finding.Why, finding.How = "bumpit compares paths as they are written", fmt.Sprintf("write %q", path.Clean(value))
	case value == ".." || strings.HasPrefix(value, "../"):
		finding.What = fmt.Sprintf("%s %q leaves the repository", key, value)
		finding.Why, finding.How = "git reads the files of the repository", "write a path inside it"
	case strings.HasPrefix(value, ":"):
		finding.What = fmt.Sprintf("%s %q starts with :", key, value)
		finding.Why, finding.How = "git reads that as pathspec magic", "write it without the :"
	case strings.ContainsAny(value, globMetacharacters):
		finding.What = fmt.Sprintf("%s %q holds a glob character (%s)", key, value, globMetacharacters)
		finding.Why, finding.How = "a module path cannot take one", "write it without the character"
	default:
		return nil
	}
	return []Finding{finding}
}

func tagPrefixTextFindings(declaration ModuleDeclaration) []Finding {
	switch prefix := declaration.TagPrefix; {
	case strings.ContainsAny(prefix, globMetacharacters):
		return []Finding{{
			Rule:    ruleInvalidTagPrefix,
			Subject: declaration.identity(),
			What:    fmt.Sprintf("tagPrefix %q holds a glob character (%s)", prefix, globMetacharacters),
			Why:     "it would read the tags of other modules",
			How:     "write it without the character",
		}}
	case prefix == "v" && !slices.Contains(declaration.Directories, "."):
		return []Finding{{
			Rule:    ruleRootTagPrefix,
			Subject: declaration.identity(),
			What:    "tagPrefix v is the root module's",
			Why:     "only the module in . takes it",
			How:     "give the module another prefix",
		}}
	default:
		return nil
	}
}

// shippedByTextFindings lists carriers that are wrong whatever the repository
// holds. Whether the module needs shippedBy at all only the repository can
// tell: a Go module takes none.
func shippedByTextFindings(declaration ModuleDeclaration) []Finding {
	if declaration.ShippedBy == nil {
		return nil
	}
	name := declaration.identity()
	var findings []Finding
	for _, carrier := range *declaration.ShippedBy {
		if carrier == name || slices.Contains(declaration.Directories, carrier) {
			findings = append(findings, Finding{
				Rule:    ruleShippedByItself,
				Subject: name,
				What:    "shippedBy lists the module itself",
				Why:     "no release can carry the module it is the release of",
				How:     "take it out of shippedBy",
			})
			continue
		}
		findings = append(findings, modulePathFindings(name, "shippedBy", carrier)...)
	}
	return findings
}
