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
// go.mod, or a module bumpit.yaml declares, which has none.
type repositoryModule struct {
	directory string
	tagPrefix string
	// goModule is the go.mod of a Go module, nil for a declared module.
	goModule *gomod.Module
	// paths are the files and directories outside its directory that a
	// declared module also reads.
	paths []string
	// shippedBy lists the directories of the modules whose release also
	// carries a declared module.
	shippedBy []string
	// node derives the paths outside its directory that a declared module
	// reads because of its package.json, nil for a Go module.
	node *nodePackages
	// goCode is the Go code of the enclosing Go module that a declared
	// directory holds.
	goCode goCodeInDirectory
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
	return repositoryModule{directory: module.Directory, tagPrefix: module.TagPrefix(), goModule: &module}
}

func declaredRepositoryModule(declaration ModuleDeclaration) repositoryModule {
	return repositoryModule{
		directory: declaration.Directory,
		tagPrefix: declaration.prefix(),
		paths:     declaration.Paths,
		shippedBy: *declaration.ShippedBy,
	}
}

// declared reports a module bumpit.yaml declares, which has no go.mod.
func (r repositoryModule) declared() bool {
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
	return discoverRepositoryModulesIn(client, declarations, nil)
}

// discoverRepositoryModulesIn is discoverRepositoryModules with tracked, the
// tracked files of the repository, listed already, or nil to list them when
// a declaration needs them.
func discoverRepositoryModulesIn(client gitx.Client, declarations []ModuleDeclaration, tracked *trackedTree) ([]repositoryModule, string, error) {
	if err := validateModuleDeclarations(declarations); err != nil {
		return nil, "", err
	}
	goModules, top, err := discoverModules(client)
	if err != nil {
		return nil, "", err
	}

	modules := make([]repositoryModule, 0, len(goModules)+len(declarations))
	for _, module := range goModules {
		modules = append(modules, goRepositoryModule(module))
	}
	for _, declaration := range declarations {
		modules = append(modules, declaredRepositoryModule(declaration))
	}
	slices.SortStableFunc(modules, func(left, right repositoryModule) int {
		return compareDirectories(left.directory, right.directory)
	})
	if !slices.ContainsFunc(modules, repositoryModule.declared) {
		return modules, top, nil
	}
	if err := checkAndReadDeclared(client, top, modules, tracked); err != nil {
		return nil, "", err
	}
	return modules, top, nil
}

// checkAndReadDeclared refuses the declared modules the repository
// contradicts, and gives the others what they read: their Node.js packages
// and the Go code in their directories. tracked is the tracked files of the
// repository, or nil to list them.
func checkAndReadDeclared(client gitx.Client, top string, modules []repositoryModule, tracked *trackedTree) error {
	if tracked == nil {
		listed, err := listTracked(client)
		if err != nil {
			return err
		}
		tracked = &listed
	}
	if err := checkDeclaredModules(client, *tracked, top, modules); err != nil {
		return fmt.Errorf("modules: %w", err)
	}
	node := newNodePackages(top, *tracked)
	for index := range modules {
		if modules[index].declared() {
			modules[index].node = node
			modules[index].goCode = findGoCode(top, *tracked, modules[index], modules)
		}
	}
	return nil
}

// keptDerivedPaths leaves out of inputs what the module reads already: a path
// its paths list, and one inside its own directory that no nested module
// owns, by the same rule as its paths.
func keptDerivedPaths(module repositoryModule, inputs []string, modules []repositoryModule) []string {
	var kept []string
	for _, entry := range inputs {
		inside := within(entry, module.directory) && !insideNestedModule(entry, module, modules)
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

// findGoCode tells which Go code the declared directory of module holds for
// the Go module around it: its .go files that lie in no nested Go module and
// in no directory the go command leaves out, and among them those no build
// compiles.
func findGoCode(top string, tracked trackedTree, module repositoryModule, modules []repositoryModule) goCodeInDirectory {
	enclosing := deepestModule(module.directory, modules, isGoModule)
	var found goCodeInDirectory
	if enclosing == nil || module.directory == "." {
		return found
	}
	for _, file := range tracked.files {
		if !goFileOf(file, module, *enclosing, modules) {
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

// goFileOf reports whether file is a .go file of the declared directory of
// module that belongs to the Go module enclosing, rather than to a Go module
// nested in the directory, and that the go command does not leave out by its
// name or the name of a directory above it.
func goFileOf(file string, module, enclosing repositoryModule, modules []repositoryModule) bool {
	if !strings.HasSuffix(file, ".go") || !within(file, module.directory) {
		return false
	}
	owner := deepestModule(file, modules, isGoModule)
	if owner == nil || owner.directory != enclosing.directory {
		return false
	}
	return !gomod.Ignored(path.Dir(file)) && !gomod.Ignored(path.Base(file))
}

func isGoModule(module repositoryModule) bool {
	return !module.declared()
}

// checkDeclaredModules refuses a declared module the repository contradicts,
// and one that Go code ships while bumpit.yaml does not say so.
func checkDeclaredModules(client gitx.Client, tracked trackedTree, top string, modules []repositoryModule) error {
	for _, module := range modules {
		if !module.declared() {
			continue
		}
		if err := checkDeclaredModule(client, tracked, module, modules); err != nil {
			return fmt.Errorf("%s: %w", module.directory, err)
		}
	}
	return checkGoShipping(tracked, top, modules)
}

func checkDeclaredModule(client gitx.Client, tracked trackedTree, module repositoryModule, modules []repositoryModule) error {
	if slices.ContainsFunc(modules, func(other repositoryModule) bool { return !other.declared() && other.directory == module.directory }) {
		return fmt.Errorf("the directory holds a go.mod: it is a Go module, versioned by its go.mod")
	}
	for _, entry := range append([]string{module.directory}, module.paths...) {
		if !tracked.holds(entry) {
			return fmt.Errorf("%s matches no tracked file", entry)
		}
	}
	for _, entry := range module.paths {
		if within(entry, module.directory) && !insideNestedModule(entry, module, modules) {
			return fmt.Errorf("paths lists %s, inside its own directory, which the module reads already", entry)
		}
	}
	if err := checkCarriers(module, modules); err != nil {
		return err
	}
	return checkTagPrefix(client, module, modules)
}

// checkCarriers refuses a carrier that is no module of the repository, and a
// cycle of carriers: a module that, through the modules it is shipped by,
// carries itself, which no release can mean.
func checkCarriers(module repositoryModule, modules []repositoryModule) error {
	for _, carrier := range module.shippedBy {
		if !slices.ContainsFunc(modules, func(other repositoryModule) bool { return other.directory == carrier }) {
			return fmt.Errorf("shippedBy lists %s, which is no module of the repository", carrier)
		}
	}
	if carriersOf(module, modules)[module.directory] {
		return fmt.Errorf("shippedBy forms a cycle: through %s, the module carries itself", strings.Join(module.shippedBy, ", "))
	}
	return nil
}

// insideNestedModule reports whether entry lies in a module nested in module,
// whose files module does not read unless its paths name them.
func insideNestedModule(entry string, module repositoryModule, modules []repositoryModule) bool {
	return slices.ContainsFunc(modules, func(other repositoryModule) bool {
		return nestedIn(other.directory, module.directory) && within(entry, other.directory)
	})
}

// checkTagPrefix refuses a tag prefix that cannot tell the tags of the module
// from those of another: the same prefix, or one that continues the other
// with what can start a version, as v1 continues v, which makes v10.0.0 a
// version of both. It also refuses a prefix git takes in no tag name.
func checkTagPrefix(client gitx.Client, module repositoryModule, modules []repositoryModule) error {
	prefix := module.tagPrefix
	if strings.HasPrefix(prefix, "-") {
		return fmt.Errorf("its tag prefix %s starts with -, which git takes for an option", prefix)
	}
	for _, other := range modules {
		switch {
		case other.directory == module.directory:
		case other.tagPrefix == prefix:
			return fmt.Errorf("its tag prefix %s is the one of %s too, so their tags could not be told apart", prefix, other.directory)
		case continuesWithVersion(prefix, other.tagPrefix):
			return fmt.Errorf("its tag prefix %s continues %s, the one of %s, with what can start a version, so a tag could be a version of both", prefix, other.tagPrefix, other.directory)
		case continuesWithVersion(other.tagPrefix, prefix):
			return fmt.Errorf("the tag prefix %s of %s continues its own, %s, with what can start a version, so a tag could be a version of both", other.tagPrefix, other.directory, prefix)
		}
	}
	valid, err := client.ValidTagName(prefix + "0.0.0")
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("its tag prefix %s does not make a valid tag name", prefix)
	}
	return nil
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

// checkGoShipping refuses a declared module that a Go module ships through a
// //go:embed pattern that reaches its files, when that Go module is not among
// the modules shippedBy says carry it. The .go files in the declared
// directory are not shipping: they are the Go module's own code, which it
// reads as its own. No pattern of a nested Go module leaves it, so none ships
// a declared root module.
func checkGoShipping(tracked trackedTree, top string, modules []repositoryModule) error {
	declared := slices.DeleteFunc(slices.Clone(modules), func(module repositoryModule) bool {
		return !module.declared() || module.directory == "."
	})
	return visitShipping(tracked, top, declared, modules, embedCache{}, func(found shipping) error {
		if carriersOf(found.module, modules)[found.goModule.directory] {
			return nil
		}
		return fmt.Errorf("%s: %s:%s embeds %s into the module %s, which then ships %s; add %s to its shippedBy", found.module.directory, found.file, found.embed.Place(), found.embed.Pattern, found.goModule.directory, found.module.directory, found.goModule.directory)
	})
}

// shipping is a //go:embed pattern of a Go file by which its Go module
// ships a declared module.
type shipping struct {
	module   repositoryModule
	goModule repositoryModule
	file     string
	embed    gomod.Embed
}

// embedCache keeps the //go:embed patterns of each Go file read, so that a
// file is read once however many modules ask about it.
type embedCache map[string][]gomod.Embed

func (c embedCache) read(top, file string) ([]gomod.Embed, error) {
	if embeds, read := c[file]; read {
		return embeds, nil
	}
	embeds, err := gomod.FileEmbeds(filepath.Join(top, filepath.FromSlash(file)))
	if err == nil {
		c[file] = embeds
	}
	return embeds, err
}

// findShipping lists every //go:embed pattern by which a Go module ships one
// of declared, in the order of the files and of the patterns in each.
func findShipping(tracked trackedTree, top string, declared, modules []repositoryModule, cache embedCache) ([]shipping, error) {
	var shippings []shipping
	err := visitShipping(tracked, top, declared, modules, cache, func(found shipping) error {
		shippings = append(shippings, found)
		return nil
	})
	return shippings, err
}

// visitShipping calls visit with every //go:embed pattern by which a Go
// module ships one of declared, in the order of the files and of the
// patterns in each, and stops at the first error, before it reads a later
// file. A Go file ships the declared modules it can reach: those its
// package lies in, and those under the directory of its package, when no
// nested Go module stands between them, since the go command embeds no file
// of another module. A test file ships nothing.
func visitShipping(tracked trackedTree, top string, declared, modules []repositoryModule, cache embedCache, visit func(shipping) error) error {
	if len(declared) == 0 {
		return nil
	}
	for _, file := range tracked.files {
		packageDirectory := path.Dir(file)
		goModule := deepestModule(file, modules, isGoModule)
		if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") || gomod.Ignored(packageDirectory) || goModule == nil {
			continue
		}
		reachable := reachableModules(packageDirectory, *goModule, declared, modules)
		if len(reachable) == 0 {
			continue
		}
		embeds, err := cache.read(top, file)
		if err != nil {
			return err
		}
		if err := visitFile(shipping{goModule: *goModule, file: file}, embeds, reachable, modules, visit); err != nil {
			return err
		}
	}
	return nil
}

// visitFile calls visit with each pattern of the Go file of found that
// reaches one of reachable.
func visitFile(found shipping, embeds []gomod.Embed, reachable, modules []repositoryModule, visit func(shipping) error) error {
	for _, module := range reachable {
		nested := nestedDirectories(module, modules)
		for _, embed := range embeds {
			if !gomod.PatternReaches(path.Dir(found.file), embed.Pattern, module.directory, nested) {
				continue
			}
			found.module, found.embed = module, embed
			if err := visit(found); err != nil {
				return err
			}
		}
	}
	return nil
}

// reachableModules lists, among declared, the modules the //go:embed
// patterns of a package of goModule in packageDirectory can reach: those the
// package lies in or that lie under it, with goModule the nearest Go module
// around them.
func reachableModules(packageDirectory string, goModule repositoryModule, declared, modules []repositoryModule) []repositoryModule {
	return slices.DeleteFunc(slices.Clone(declared), func(module repositoryModule) bool {
		nearest := deepestModule(module.directory, modules, isGoModule)
		sameGoModule := nearest != nil && nearest.directory == goModule.directory
		return !sameGoModule || !within(packageDirectory, module.directory) && !within(module.directory, packageDirectory)
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
		index := slices.IndexFunc(modules, func(other repositoryModule) bool { return other.declared() && other.directory == carrier })
		if index >= 0 {
			pending = append(pending, modules[index].shippedBy...)
		}
	}
	return carriers
}

// deepestModule is the module, among those keep accepts, whose directory
// holds file most closely: the one that owns it. nil when none holds it.
func deepestModule(file string, modules []repositoryModule, keep func(repositoryModule) bool) *repositoryModule {
	var deepest *repositoryModule
	for index := range modules {
		module := &modules[index]
		if !keep(*module) || !within(file, module.directory) {
			continue
		}
		if deepest == nil || len(module.directory) > len(deepest.directory) || deepest.directory == "." {
			deepest = module
		}
	}
	return deepest
}

// nestedDirectories lists the directories of the modules nested in module.
func nestedDirectories(module repositoryModule, modules []repositoryModule) []string {
	var nested []string
	for _, other := range modules {
		if nestedIn(other.directory, module.directory) {
			nested = append(nested, other.directory)
		}
	}
	return nested
}
