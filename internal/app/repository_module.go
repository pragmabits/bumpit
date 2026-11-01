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

	if err := checkDeclaredModules(client, top, modules); err != nil {
		return nil, "", fmt.Errorf("modules: %w", err)
	}
	return modules, top, nil
}

// checkDeclaredModules refuses a declared module the repository contradicts,
// and one that Go code ships while bumpit.yaml does not say so.
func checkDeclaredModules(client gitx.Client, top string, modules []repositoryModule) error {
	for _, module := range modules {
		if !module.declared() {
			continue
		}
		if err := checkDeclaredModule(client, module, modules); err != nil {
			return fmt.Errorf("%s: %w", module.directory, err)
		}
	}
	return checkGoShipping(client, top, modules)
}

func checkDeclaredModule(client gitx.Client, module repositoryModule, modules []repositoryModule) error {
	if slices.ContainsFunc(modules, func(other repositoryModule) bool { return !other.declared() && other.directory == module.directory }) {
		return fmt.Errorf("the directory holds a go.mod: it is a Go module, versioned by its go.mod")
	}
	for _, entry := range append([]string{module.directory}, module.paths...) {
		if err := checkTracked(client, entry); err != nil {
			return err
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

// checkTracked refuses a path of a declaration that names no tracked file: a
// misspelt path would otherwise read no commit, and the module would never
// be released for it.
func checkTracked(client gitx.Client, entry string) error {
	files, err := client.TrackedFiles(topPathspec(entry))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("%s matches no tracked file", entry)
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
func checkGoShipping(client gitx.Client, top string, modules []repositoryModule) error {
	declared := slices.DeleteFunc(slices.Clone(modules), func(module repositoryModule) bool {
		return !module.declared() || module.directory == "."
	})
	if len(declared) == 0 {
		return nil
	}

	files, err := client.TrackedFiles(":(top,glob)**/*.go")
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := checkGoFile(top, file, declared, modules); err != nil {
			return err
		}
	}
	return nil
}

// checkGoFile checks the declared modules that the //go:embed patterns of one
// Go file can reach: those its package lies in, and those under the
// directory of its package. A test file ships nothing.
func checkGoFile(top, file string, declared, modules []repositoryModule) error {
	packageDirectory := path.Dir(file)
	goModule := deepestModule(file, modules, func(module repositoryModule) bool { return !module.declared() })
	if strings.HasSuffix(file, "_test.go") || gomod.Ignored(packageDirectory) || goModule == nil {
		return nil
	}

	reachable := slices.DeleteFunc(slices.Clone(declared), func(module repositoryModule) bool {
		return !within(packageDirectory, module.directory) && !within(module.directory, packageDirectory)
	})
	if len(reachable) == 0 {
		return nil
	}
	embeds, err := gomod.FileEmbeds(filepath.Join(top, filepath.FromSlash(file)))
	if err != nil {
		return err
	}
	for _, module := range reachable {
		if err := checkShipping(file, embeds, *goModule, module, modules); err != nil {
			return fmt.Errorf("%s: %w", module.directory, err)
		}
	}
	return nil
}

// checkShipping refuses the Go file of goModule when one of its //go:embed
// patterns reaches the declared module and goModule does not carry it.
func checkShipping(file string, embeds []gomod.Embed, goModule, module repositoryModule, modules []repositoryModule) error {
	if carriersOf(module, modules)[goModule.directory] {
		return nil
	}

	nested := nestedDirectories(module, modules)
	for _, embed := range embeds {
		if gomod.PatternReaches(path.Dir(file), embed.Pattern, module.directory, nested) {
			return fmt.Errorf("%s:%s embeds %s into the module %s, which then ships %s; add %s to its shippedBy", file, embed.Place(), embed.Pattern, goModule.directory, module.directory, goModule.directory)
		}
	}
	return nil
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
