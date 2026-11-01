package app

import (
	"fmt"
	"path"
	"strings"
)

// ModuleDeclaration is a module of the repository that has no go.mod, as
// bumpit.yaml declares it under modules: a directory released on its own,
// with tags and commits of its own.
type ModuleDeclaration struct {
	// Directory is the directory of the module, slash separated and relative
	// to the top of the repository: "." for the root.
	Directory string `mapstructure:"directory"`
	// TagPrefix is what every version tag of the module starts with, its "v"
	// included. Empty means the directory followed by "/v", or "v" for the
	// root.
	TagPrefix string `mapstructure:"tagPrefix"`
	// Paths are the files and directories outside Directory that the module
	// is built from: a commit touching one of them changes the module too.
	Paths []string `mapstructure:"paths"`
	// ShippedBy lists the directories of the modules whose release also
	// carries this one, which then read its commits. It is required, since
	// bumpit cannot always tell: nil means bumpit.yaml leaves it out.
	ShippedBy *[]string `mapstructure:"shippedBy"`
}

// prefix is the tag prefix of the module, the default one when bumpit.yaml
// gives none.
func (d ModuleDeclaration) prefix() string {
	switch {
	case d.TagPrefix != "":
		return d.TagPrefix
	case d.Directory == ".":
		return "v"
	default:
		return d.Directory + "/v"
	}
}

// validateModuleDeclarations refuses what can be told wrong from bumpit.yaml
// alone. What the repository must confirm, such as a go.mod in the directory
// or the files a path names, is checked when the modules are discovered.
func validateModuleDeclarations(declarations []ModuleDeclaration) error {
	declared := map[string]bool{}
	for _, declaration := range declarations {
		if err := validateModuleDeclaration(declaration); err != nil {
			return fmt.Errorf("modules: %w", err)
		}
		if declared[declaration.Directory] {
			return fmt.Errorf("modules: %s is declared twice", declaration.Directory)
		}
		declared[declaration.Directory] = true
	}
	return nil
}

func validateModuleDeclaration(declaration ModuleDeclaration) error {
	if err := checkModulePath("directory", declaration.Directory); err != nil {
		return err
	}

	name := declaration.Directory
	for _, entry := range declaration.Paths {
		if err := checkModulePath("paths", entry); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if entry == name {
			return fmt.Errorf("%s: paths lists %s, its own directory, which the module reads already", name, entry)
		}
	}
	if err := checkTagPrefixText(declaration); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return checkShippedByText(declaration)
}

// checkModulePath refuses a path of a declaration that git cannot read as a
// plain path relative to the top of the repository.
func checkModulePath(key, value string) error {
	switch {
	case value == "":
		return fmt.Errorf("%s cannot be empty", key)
	case strings.HasPrefix(value, "/"):
		return fmt.Errorf("%s %q is absolute; write it relative to the top of the repository", key, value)
	case path.Clean(value) != value:
		return fmt.Errorf("%s %q is not clean; write %q", key, value, path.Clean(value))
	case value == ".." || strings.HasPrefix(value, "../"):
		return fmt.Errorf("%s %q leaves the repository", key, value)
	case strings.HasPrefix(value, ":"):
		return fmt.Errorf("%s %q starts with :, which git reads as pathspec magic", key, value)
	case strings.ContainsAny(value, globMetacharacters):
		return fmt.Errorf("%s %q holds a glob character (%s), which a module path cannot take", key, value, globMetacharacters)
	default:
		return nil
	}
}

func checkTagPrefixText(declaration ModuleDeclaration) error {
	switch prefix := declaration.TagPrefix; {
	case strings.ContainsAny(prefix, globMetacharacters):
		return fmt.Errorf("tagPrefix %q holds a glob character (%s), which would read the tags of other modules", prefix, globMetacharacters)
	case prefix == "v" && declaration.Directory != ".":
		return fmt.Errorf("tagPrefix v is the root module's; only the module in . takes it")
	default:
		return nil
	}
}

func checkShippedByText(declaration ModuleDeclaration) error {
	name := declaration.Directory
	if declaration.ShippedBy == nil {
		return fmt.Errorf("%s: shippedBy is required: list the modules whose release also carries %s, or write shippedBy: []", name, name)
	}
	for _, carrier := range *declaration.ShippedBy {
		if carrier == name {
			return fmt.Errorf("%s: shippedBy lists the module itself", name)
		}
		if err := checkModulePath("shippedBy", carrier); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}
