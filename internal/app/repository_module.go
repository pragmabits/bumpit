package app

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pragmabits/bumpit/internal/gitx"
	"github.com/pragmabits/bumpit/internal/gomod"
)

// repositoryModule is a module bumpit versions: a Go module, found by its
// go.mod, which bumpit.yaml may also list to name it, or a module bumpit.yaml
// declares, which has none.
type repositoryModule struct {
	// name is what the commands and shippedBy call the module: the name
	// bumpit.yaml gives it, else its directory.
	name string
	// named reports a name bumpit.yaml gives, which a command line can write
	// apart from a path.
	named bool
	// directories are the directories the module owns. A Go module has one.
	directories []string
	tagPrefix   string
	// goModule is the go.mod of a Go module, nil for a declared module.
	goModule *gomod.Module
	// paths are the files and directories outside its directory that a
	// declared module also reads.
	paths []string
	// shippedBy calls the modules whose release also carries a declared module,
	// each by its name or, when it has none, its directory, and is nil when
	// bumpit.yaml says nothing of them.
	shippedBy []string
	// node derives the paths outside its directory that a declared module
	// reads because of its package.json, nil for a Go module.
	node *nodePackages
	// goCode is the Go code of the Go module around it that each directory of
	// a declared module holds, by directory.
	goCode map[string]goCodeInDirectory
}

// goCodeInDirectory is the Go code a declared directory holds for the Go
// module around it, which reads its commits as its own.
type goCodeInDirectory struct {
	// held reports a .go file some build of the Go module compiles.
	held bool
	// neverCompiled lists the tracked .go files of the directory that no
	// build compiles, which the Go module does not read.
	neverCompiled []string
}

func goRepositoryModule(module gomod.Module) repositoryModule {
	return repositoryModule{name: module.Directory, directories: []string{module.Directory}, tagPrefix: module.TagPrefix(), goModule: &module}
}

func declaredRepositoryModule(declaration ModuleDeclaration) repositoryModule {
	module := repositoryModule{
		name:        declaration.identity(),
		named:       declaration.Name != "",
		directories: declaration.Directories,
		tagPrefix:   declaration.prefix(),
		paths:       declaration.Paths,
	}
	if declaration.ShippedBy != nil {
		module.shippedBy = *declaration.ShippedBy
	}
	return module
}

// applyGoEntry gives the Go module what the entry that lists it says, which is
// only its name, and lists what else the entry says: the rest of a Go module
// comes from its go.mod.
func applyGoEntry(module *repositoryModule, declaration ModuleDeclaration) []Finding {
	if declaration.Name != "" {
		module.name, module.named = declaration.Name, true
	}
	name := declaration.identity()
	switch {
	case len(declaration.Directories) > 1:
		return []Finding{{
			Rule:    ruleGoEntryDirectories,
			Subject: name,
			What:    module.directories[0] + " holds a go.mod",
			Why:     "a Go module has one directory",
			How:     "give the other directories a module of their own",
		}}
	case declaration.TagPrefix != "" || len(declaration.Paths) > 0 || declaration.ShippedBy != nil:
		return []Finding{{
			Rule:    ruleGoEntryKeys,
			Subject: name,
			What:    "the directory holds a go.mod, so it is a Go module",
			Why:     "it takes only directories and name, and its go.mod says the rest",
			How:     "take out tagPrefix, paths and shippedBy",
		}}
	}
	return nil
}

// assembleModules joins the Go modules and what declarations says of them with
// the modules declarations declares, the root one first and the others by
// directory, and lists what the go.mod files contradict of declarations. A
// declaration that owns no directory is left out: it is nowhere in the repository.
func assembleModules(goModules []gomod.Module, declarations []ModuleDeclaration) ([]repositoryModule, []Finding) {
	modules := make([]repositoryModule, 0, len(goModules)+len(declarations))
	for _, module := range goModules {
		modules = append(modules, goRepositoryModule(module))
	}
	var findings []Finding
	for _, declaration := range declarations {
		if len(declaration.Directories) == 0 {
			continue
		}
		index := slices.IndexFunc(modules[:len(goModules)], func(module repositoryModule) bool {
			return slices.Contains(declaration.Directories, module.directories[0])
		})
		if index >= 0 {
			findings = append(findings, applyGoEntry(&modules[index], declaration)...)
			continue
		}
		if declaration.ShippedBy == nil {
			findings = append(findings, Finding{
				Rule:    ruleShippedByMissing,
				Subject: declaration.identity(),
				What:    "shippedBy is required",
				Why:     "bumpit cannot always tell whether the release of another module carries it",
				How:     "list the modules that carry it, or write shippedBy: []; if its go.mod is gone, bumpit modules remove takes the entry out",
			})
		}
		modules = append(modules, declaredRepositoryModule(declaration))
	}
	slices.SortStableFunc(modules, func(left, right repositoryModule) int {
		return compareDirectories(left.directories[0], right.directories[0])
	})
	return modules, findings
}

// owns reports whether directory is one of the directories of the module.
func (r repositoryModule) owns(directory string) bool {
	return slices.Contains(r.directories, directory)
}

// holds reports whether entry, a file or a directory, is a directory of the
// module or lies under one.
func (r repositoryModule) holds(entry string) bool {
	return slices.ContainsFunc(r.directories, func(directory string) bool { return within(entry, directory) })
}

// surrounds reports whether directory lies under a directory of the module.
func (r repositoryModule) surrounds(directory string) bool {
	return slices.ContainsFunc(r.directories, func(own string) bool { return nestedIn(directory, own) })
}

// moduleNamed is the index among modules of the first one called name, -1 when
// none is.
func moduleNamed(modules []repositoryModule, name string) int {
	return slices.IndexFunc(modules, func(module repositoryModule) bool { return module.name == name })
}

// identity is what a command line can call the module.
func (r repositoryModule) identity() moduleIdentity {
	identity := moduleIdentity{directories: r.directories}
	if r.named {
		identity.name = r.name
	}
	return identity
}

func identitiesOf(modules []repositoryModule) []moduleIdentity {
	identities := make([]moduleIdentity, 0, len(modules))
	for _, module := range modules {
		identities = append(identities, module.identity())
	}
	return identities
}

// withoutGoMod reports a module that exists because bumpit.yaml declares it,
// since no go.mod makes it one.
func (r repositoryModule) withoutGoMod() bool {
	return r.goModule == nil
}

// modulePath is the module path of a Go module, empty for a declared one.
func (r repositoryModule) modulePath() string {
	if r.goModule == nil {
		return ""
	}
	return r.goModule.Path
}

// discoverRepositoryModules lists every module of the repository, the root
// one first and the others by directory: its Go modules and the modules
// declarations declares. It refuses a declaration the repository contradicts.
func discoverRepositoryModules(client gitx.Client, declarations []ModuleDeclaration) ([]repositoryModule, string, error) {
	return discoverRepositoryModulesIn(newRepositorySnapshot(client), declarations)
}

// discoverRepositoryModulesIn is discoverRepositoryModules over what a run has
// read of the repository.
func discoverRepositoryModulesIn(snapshot *repositorySnapshot, declarations []ModuleDeclaration) ([]repositoryModule, string, error) {
	if err := validateModuleDeclarations(declarations); err != nil {
		return nil, "", err
	}
	top, err := snapshot.top()
	if err != nil {
		return nil, "", err
	}
	goModules, err := snapshot.goModules()
	if err != nil {
		return nil, "", err
	}

	modules, findings := assembleModules(goModules, declarations)
	names, err := nameFindings(snapshot, modules)
	if err != nil {
		return nil, "", err
	}
	if err := firstFinding(append(findings, names...)); err != nil {
		return nil, "", fmt.Errorf("modules: %w", err)
	}
	if !slices.ContainsFunc(modules, repositoryModule.withoutGoMod) {
		return modules, top, nil
	}
	if err := checkAndReadDeclared(snapshot, top, modules); err != nil {
		return nil, "", err
	}
	return modules, top, nil
}

// checkAndReadDeclared refuses the declared modules the repository
// contradicts, and gives the others what they read: their Node.js packages
// and the Go code in their directories.
func checkAndReadDeclared(snapshot *repositorySnapshot, top string, modules []repositoryModule) error {
	tracked, err := snapshot.tracked()
	if err != nil {
		return err
	}
	findings, err := declaredModuleFindings(snapshot, tracked, modules)
	if err != nil {
		return err
	}
	if err := firstFinding(findings); err != nil {
		return fmt.Errorf("modules: %w", err)
	}
	if err := checkGoShipping(tracked, top, modules, snapshot.embeds); err != nil {
		return fmt.Errorf("modules: %w", err)
	}
	readDeclared(top, tracked, modules)
	return nil
}

// readDeclared gives the declared modules what they read: their Node.js
// packages and the Go code in their directories.
func readDeclared(top string, tracked trackedTree, modules []repositoryModule) {
	node := newNodePackages(top, tracked)
	for index := range modules {
		if modules[index].withoutGoMod() {
			modules[index].node = node
			modules[index].goCode = findGoCode(top, tracked, modules[index], modules)
		}
	}
}

// repositoryReport is the modules of the repository, as far as a config lets
// them be read, and what the repository contradicts of that config.
type repositoryReport struct {
	modules  []repositoryModule
	tracked  trackedTree
	findings []Finding
}

// repositoryFindings lists all that a run would refuse of declarations, where a
// run stops at the first of them. An error is a failure to ask, such as a go.mod
// that cannot be read. The modules it returns have yet to read what they read,
// which a proof has no use for.
func repositoryFindings(snapshot *repositorySnapshot, declarations []ModuleDeclaration) (repositoryReport, error) {
	top, err := snapshot.top()
	if err != nil {
		return repositoryReport{}, err
	}
	goModules, err := snapshot.goModules()
	if err != nil {
		return repositoryReport{}, err
	}
	modules, findings := assembleModules(goModules, declarations)
	names, err := nameFindings(snapshot, modules)
	if err != nil {
		return repositoryReport{}, err
	}
	findings = append(findings, names...)

	tracked, err := snapshot.tracked()
	if err != nil {
		return repositoryReport{}, err
	}
	declaredFindings, err := declaredModuleFindings(snapshot, tracked, modules)
	if err != nil {
		return repositoryReport{}, err
	}
	shipped, err := shippingFindings(tracked, top, modules, snapshot.embeds)
	if err != nil {
		return repositoryReport{}, err
	}
	findings = append(append(findings, declaredFindings...), shipped...)
	findings = append(findings, packageFindings(snapshot, top, tracked, modules)...)
	return repositoryReport{modules: modules, tracked: tracked, findings: findings}, nil
}

// keptDerivedPaths leaves out of inputs what the module reads already: a path
// its paths list, and one inside its own directory that no nested module
// owns, by the same rule as its paths.
func keptDerivedPaths(module repositoryModule, inputs []string, modules []repositoryModule) []string {
	var kept []string
	for _, entry := range inputs {
		inside := module.holds(entry) && !insideNestedModule(entry, module, modules)
		if !inside && !slices.Contains(module.paths, entry) {
			kept = append(kept, entry)
		}
	}
	return kept
}

// derivedPaths are the paths a declared module reads because of its
// package.json, and why they cannot be known when they cannot.
func (r repositoryModule) derivedPaths(modules []repositoryModule) ([]string, error) {
	return r.node.derive(r, modules)
}

// readPaths are the paths outside its directory that a module reads: those
// bumpit.yaml lists, then those its package.json derives. A plan checks the
// derivation first, so that one that fails stops it.
func (r repositoryModule) readPaths(modules []repositoryModule) []string {
	derived, _ := r.derivedPaths(modules)
	return append(slices.Clone(r.paths), derived...)
}

// findGoCode tells which Go code each directory of module holds for the Go
// module around it, since each may have another one around it.
func findGoCode(top string, tracked trackedTree, module repositoryModule, modules []repositoryModule) map[string]goCodeInDirectory {
	found := make(map[string]goCodeInDirectory, len(module.directories))
	for _, directory := range module.directories {
		found[directory] = findGoCodeIn(top, tracked, directory, modules)
	}
	return found
}

// findGoCodeIn tells which Go code a declared directory holds for the Go
// module around it: its .go files that lie in no nested Go module and in no
// directory the go command leaves out, and among them those no build compiles.
func findGoCodeIn(top string, tracked trackedTree, directory string, modules []repositoryModule) goCodeInDirectory {
	enclosing := deepestGoModule(directory, modules)
	var found goCodeInDirectory
	if enclosing == nil || directory == "." {
		return found
	}
	for _, file := range tracked.files {
		if !goFileOf(file, directory, *enclosing, modules) {
			continue
		}
		if gomod.NeverCompiled(filepath.Join(top, filepath.FromSlash(file))) {
			found.neverCompiled = append(found.neverCompiled, file)
			continue
		}
		found.held = true
	}
	return found
}

// goFileOf reports whether file is a .go file of the declared directory that
// belongs to the Go module enclosing, rather than to a Go module nested in the
// directory, and that the go command does not leave out by its name or the
// name of a directory above it.
func goFileOf(file, directory string, enclosing repositoryModule, modules []repositoryModule) bool {
	if !strings.HasSuffix(file, ".go") || !within(file, directory) {
		return false
	}
	owner := deepestGoModule(file, modules)
	if owner == nil || owner.name != enclosing.name {
		return false
	}
	return !gomod.Ignored(path.Dir(file)) && !gomod.Ignored(path.Base(file))
}

// insideNestedModule reports whether entry lies in a module nested in module,
// whose files module does not read unless its paths name them.
func insideNestedModule(entry string, module repositoryModule, modules []repositoryModule) bool {
	return slices.ContainsFunc(modules, func(other repositoryModule) bool {
		return slices.ContainsFunc(other.directories, func(directory string) bool {
			return module.surrounds(directory) && within(entry, directory)
		})
	})
}

// carriersOf lists the modules whose release carries module: those its
// shippedBy names and, through each declared one of them, those that carry
// that one in turn.
func carriersOf(module repositoryModule, modules []repositoryModule) map[string]bool {
	carriers := map[string]bool{}
	pending := slices.Clone(module.shippedBy)
	for len(pending) > 0 {
		carrier := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if carriers[carrier] {
			continue
		}
		carriers[carrier] = true
		index := slices.IndexFunc(modules, func(other repositoryModule) bool { return other.withoutGoMod() && other.name == carrier })
		if index >= 0 {
			pending = append(pending, modules[index].shippedBy...)
		}
	}
	return carriers
}

// deepestGoModule is the Go module whose directory holds file most closely:
// the one the go command says owns it. nil when none holds it.
func deepestGoModule(file string, modules []repositoryModule) *repositoryModule {
	var deepest *repositoryModule
	for index := range modules {
		module := &modules[index]
		if module.withoutGoMod() || !within(file, module.directories[0]) {
			continue
		}
		if deepest == nil || len(module.directories[0]) > len(deepest.directories[0]) || deepest.directories[0] == "." {
			deepest = module
		}
	}
	return deepest
}

// nestedDirectories lists the directories of the modules nested in directory.
func nestedDirectories(directory string, modules []repositoryModule) []string {
	var nested []string
	for _, other := range modules {
		for _, candidate := range other.directories {
			if nestedIn(candidate, directory) {
				nested = append(nested, candidate)
			}
		}
	}
	return nested
}
