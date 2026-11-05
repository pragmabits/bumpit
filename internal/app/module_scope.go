package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pragmabits/bumpit/internal/gitx"
)

// moduleScope is the module a plan versions: its tags start with the
// module's tag prefix, and its commits are those touching what it reads.
type moduleScope struct {
	module  repositoryModule
	modules []repositoryModule
	// built holds the queries once they are built, since a plan reads its
	// commits more than once.
	built []commitQuery
}

// readCommit is a commit a plan reads, and how it came in.
type readCommit struct {
	gitx.Commit
	// via is the path or the module the commit came in through, empty when
	// it touches the module's own directory.
	via string
	// shipped reports that the commit came in only through a module that
	// this one ships.
	shipped bool
}

// commitQuery is one git log of the commits a plan reads: the pathspecs of
// one part of what the module reads, and how a commit it returns came in.
type commitQuery struct {
	pathspecs []string
	via       string
	shipped   bool
}

// resolveModuleScope picks the module a plan versions: the one --module names,
// by its name or by a directory relative to the repository path, or, with
// neither --module nor --match, the root module when the repository has one.
// Outside a module it returns nil, and the plan reads tags by pattern alone.
// It also returns the modules it discovered, none with --match alone.
func resolveModuleScope(client gitx.Client, repository, moduleName, tagMatch string, declarations []ModuleDeclaration) (*moduleScope, []repositoryModule, error) {
	if moduleName == "" && tagMatch != "" {
		return nil, nil, nil
	}

	modules, top, err := discoverRepositoryModules(client, declarations)
	if err != nil {
		return nil, nil, err
	}

	if moduleName == "" {
		index := slices.IndexFunc(modules, func(module repositoryModule) bool { return module.owns(".") })
		if index < 0 {
			return nil, modules, nil
		}
		return &moduleScope{module: modules[index], modules: modules}, modules, nil
	}

	directory, err := relativeToTop(top, repository, moduleName)
	if err != nil {
		return nil, nil, err
	}
	index, err := moduleReference{written: moduleName, directory: directory}.find(identitiesOf(modules))
	if err != nil {
		return nil, nil, err
	}
	if index < 0 {
		return nil, nil, fmt.Errorf("no module %s: no module has that name, the directory holds no tracked go.mod, and bumpit.yaml declares no module there", moduleName)
	}
	return &moduleScope{module: modules[index], modules: modules}, modules, nil
}

// relativeToTop turns a directory given relative to the repository path, or
// absolute, into one relative to the top of the working tree, slash separated. An
// empty one is refused: it names nothing, though joined to the repository path it
// would name that, as a shell gives an unset variable.
func relativeToTop(top, repository, directory string) (string, error) {
	if directory == "" {
		return "", errors.New("a path cannot be empty: write . for the directory --repo gives")
	}
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(repository, directory)
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}

	relative, err := filepath.Rel(top, resolveLinks(absolute))
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(relative), nil
}

// resolveLinks resolves the links of path as far as it exists on disk: what git
// tracks may be gone from it, while the directory above it is reached by a link.
func resolveLinks(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	return filepath.Join(resolveLinks(parent), filepath.Base(path))
}

// tagPattern is the pattern of the tags a plan reads: the module's tag prefix
// in a module scope, else the --match pattern, else "v*".
func tagPattern(scope *moduleScope, tagMatch string) string {
	switch {
	case scope != nil:
		return scope.module.tagPrefix + "*"
	case tagMatch != "":
		return tagMatch
	default:
		return "v*"
	}
}

// commitsSince reads the commits after tag that the module reads, or every
// commit outside a module scope. A commit that several queries return comes
// in through the first of them: the module's own directory, then its paths,
// then the modules it ships.
func (m *moduleScope) commitsSince(client gitx.Client, tag string) ([]readCommit, error) {
	queries := []commitQuery{{}}
	if m != nil {
		queries = m.queries()
	}

	if len(queries) == 1 {
		commits, err := client.CommitsSince(tag, queries[0].pathspecs...)
		return readCommits(commits, queries[0]), err
	}

	read := map[string]readCommit{}
	for _, query := range queries {
		commits, err := client.CommitsSince(tag, query.pathspecs...)
		if err != nil {
			return nil, err
		}
		for _, commit := range readCommits(commits, query) {
			if _, seen := read[commit.Hash]; !seen {
				read[commit.Hash] = commit
			}
		}
	}
	order, err := readOrder(client, tag, read)
	if err != nil {
		return nil, err
	}

	commits := make([]readCommit, 0, len(order))
	for _, hash := range order {
		commits = append(commits, read[hash])
	}
	return commits, nil
}

// readCommits records how the commits query returned came in.
func readCommits(commits []gitx.Commit, query commitQuery) []readCommit {
	read := make([]readCommit, 0, len(commits))
	for _, commit := range commits {
		read = append(read, readCommit{Commit: commit, via: query.via, shipped: query.shipped})
	}
	return read
}

// readOrder puts the commits several queries read in the order of the
// history, as one query returns them.
func readOrder(client gitx.Client, tag string, read map[string]readCommit) ([]string, error) {
	hashes, err := client.CommitOrder(tag)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(hashes, func(hash string) bool {
		_, seen := read[hash]
		return !seen
	}), nil
}

// queries are the git logs of what the module reads. Each part is a query of
// its own, so that git simplifies its history on its own, as it would if the
// part were a module: one query joining them would follow a side branch into
// a change a merge discarded whenever another part differs after the merge.
// The parts are each of its directories, less the modules nested in it, the
// Go code it has in declared directories, each of its paths, and the same of
// every module it ships. A path is read as the name it is, glob characters
// included.
func (m *moduleScope) queries() []commitQuery {
	if m.built != nil {
		return m.built
	}

	queries := make([]commitQuery, 0, len(m.module.directories))
	for _, directory := range m.module.directories {
		queries = append(queries, commitQuery{pathspecs: ownPathspecs(directory, m.modules)})
	}
	queries = append(queries, m.goCodeQueries()...)
	for _, entry := range m.module.readPaths(m.modules) {
		queries = append(queries, commitQuery{pathspecs: []string{literalPathspec(entry)}, via: entry})
	}
	for _, shipped := range m.shippedModules() {
		for _, directory := range shipped.directories {
			queries = append(queries, commitQuery{pathspecs: ownPathspecs(directory, m.modules), via: shipped.name, shipped: true})
		}
		for _, entry := range shipped.readPaths(m.modules) {
			queries = append(queries, commitQuery{pathspecs: []string{literalPathspec(entry)}, via: shipped.name, shipped: true})
		}
	}
	m.built = queries
	return queries
}

// goCodeLeftOut are the paths, below a directory, of the Go files the go
// command never compiles: those under testdata, vendor, or a file or
// directory whose name starts with _ or a dot.
var goCodeLeftOut = []string{"**/testdata/**", "**/vendor/**", "**/_*", "**/_*/**", "**/.*", "**/.*/**"}

// goCodeQueries are the queries of the Go code a Go module has in the
// declared modules nested in it: the .go files of a declared directory are
// still packages of the Go module, compiled and released with it, so their
// commits are its own, with their own level, whether or not it ships the
// rest of the directory. One query reads each outermost such directory, less
// the Go modules nested in it, which own their files, and less the files no
// build compiles.
func (m *moduleScope) goCodeQueries() []commitQuery {
	var queries []commitQuery
	for _, held := range m.goCodeDirectories() {
		pathspecs := []string{":(top,glob)" + held.directory + "/**/*.go"}
		for _, nested := range m.modules {
			if !nested.withoutGoMod() && nestedIn(nested.directories[0], held.directory) {
				pathspecs = append(pathspecs, ":(top,exclude)"+nested.directories[0])
			}
		}
		for _, leftOut := range goCodeLeftOut {
			pathspecs = append(pathspecs, ":(top,glob,exclude)"+held.directory+"/"+leftOut)
		}
		for _, file := range held.neverCompiled {
			pathspecs = append(pathspecs, ":(top,literal,exclude)"+file)
		}
		queries = append(queries, commitQuery{pathspecs: pathspecs})
	}
	return queries
}

// heldGoCode is a declared directory whose Go code a Go module compiles.
type heldGoCode struct {
	directory     string
	neverCompiled []string
}

// goCodeDirectories are the declared directories whose Go code a Go module
// compiles: those it is the nearest module around, so that no Go module
// between them owns the code and no declared module between them reads it
// already, and that hold such code.
func (m *moduleScope) goCodeDirectories() []heldGoCode {
	if m.module.withoutGoMod() {
		return nil
	}
	var found []heldGoCode
	for _, other := range m.modules {
		if other.withoutGoMod() {
			found = append(found, m.goCodeOf(other)...)
		}
	}
	return found
}

// goCodeOf lists the directories of the declared module other whose Go code
// the Go module of the scope compiles.
func (m *moduleScope) goCodeOf(other repositoryModule) []heldGoCode {
	var found []heldGoCode
	for _, directory := range other.directories {
		code := other.goCode[directory]
		if code.held && encloses(m.module, directory, m.modules) {
			found = append(found, heldGoCode{directory: directory, neverCompiled: code.neverCompiled})
		}
	}
	return found
}

// ownPathspecs selects the files a module owns in one of its directories: the
// directory, less every module nested in it.
func ownPathspecs(directory string, modules []repositoryModule) []string {
	pathspecs := []string{topPathspec(directory)}
	for _, nested := range nestedDirectories(directory, modules) {
		pathspecs = append(pathspecs, ":(top,exclude)"+nested)
	}
	return pathspecs
}

// derivationError is why the paths of the module, or of a module it ships,
// could not be derived from their package.json: the plan cannot know what
// it reads.
func (m *moduleScope) derivationError() error {
	if m == nil {
		return nil
	}
	for _, read := range append([]repositoryModule{m.module}, m.shippedModules()...) {
		if _, err := read.derivedPaths(m.modules); err != nil {
			return err
		}
	}
	return nil
}

// shippedModules are the declared modules whose release the module carries,
// by directory.
func (m *moduleScope) shippedModules() []repositoryModule {
	var shipped []repositoryModule
	for _, other := range m.modules {
		if other.withoutGoMod() && other.name != m.module.name && carriersOf(other, m.modules)[m.module.name] {
			shipped = append(shipped, other)
		}
	}
	return shipped
}

// describe writes into plan what the module is, and what it reads beyond its
// own directory or leaves out of it. A Go module has its module path; a
// declared module has its directories, its paths and its carriers.
func (m *moduleScope) describe(plan *ReleasePlan) {
	if m == nil {
		return
	}

	plan.Module = m.module.modulePath()
	if m.module.named || m.module.withoutGoMod() {
		plan.Name = m.module.name
	}
	if m.module.withoutGoMod() {
		plan.Directories, plan.Paths, plan.ShippedBy = m.module.directories, m.module.paths, m.module.shippedBy
		plan.DerivedPaths, _ = m.module.derivedPaths(m.modules)
	}
	for _, held := range m.goCodeDirectories() {
		plan.GoFilesIn = append(plan.GoFilesIn, held.directory)
	}
	shipped := m.shippedModules()
	for _, module := range shipped {
		plan.Ships = append(plan.Ships, module.name)
	}
	plan.LeavesOut = m.leftOut(shipped)
}

// leftOut lists the directories of declared modules whose commits the module
// does not read although they lie in a directory it reads: its own, or that of
// a module it ships, with no other module between. It reads those of the
// modules it ships, and those a path of its own or of a shipped module names.
func (m *moduleScope) leftOut(shipped []repositoryModule) []string {
	readers := append([]repositoryModule{m.module}, shipped...)
	var paths []string
	for _, reader := range readers {
		paths = append(paths, reader.readPaths(m.modules)...)
	}

	var left []string
	for _, other := range m.modules {
		isReader := slices.ContainsFunc(readers, func(reader repositoryModule) bool { return reader.name == other.name })
		if other.withoutGoMod() && !isReader {
			left = append(left, m.directoriesLeftOut(other, readers, paths)...)
		}
	}
	return left
}

// directoriesLeftOut lists the directories of other that readers surround with
// no module between and that no path of paths names.
func (m *moduleScope) directoriesLeftOut(other repositoryModule, readers []repositoryModule, paths []string) []string {
	var left []string
	for _, directory := range other.directories {
		named := slices.ContainsFunc(paths, func(entry string) bool { return within(directory, entry) })
		enclosed := slices.ContainsFunc(readers, func(reader repositoryModule) bool { return encloses(reader, directory, m.modules) })
		if !named && enclosed {
			left = append(left, directory)
		}
	}
	return left
}

// encloses reports whether outer is the nearest module around directory.
func encloses(outer repositoryModule, directory string, modules []repositoryModule) bool {
	return slices.ContainsFunc(outer.directories, func(own string) bool {
		return nestedIn(directory, own) && !slices.ContainsFunc(nestedDirectories(own, modules), func(between string) bool {
			return nestedIn(directory, between)
		})
	})
}

// otherModulePrefixes lists the tag prefixes of the modules whose versions
// the pattern of a plan also matches, since their prefix starts with its own:
// the root module's v* also matches vite/v0.1.0, the version of the module in
// vite. In a module scope the pattern is the module's prefix; outside one,
// the default v*. A pattern given with --match is read as given.
func otherModulePrefixes(scope *moduleScope, modules []repositoryModule, tagMatch string) []string {
	prefix := "v"
	switch {
	case scope != nil:
		prefix = scope.module.tagPrefix
	case tagMatch != "":
		return nil
	}

	var prefixes []string
	for _, other := range modules {
		if other.tagPrefix != prefix && strings.HasPrefix(other.tagPrefix, prefix) {
			prefixes = append(prefixes, other.tagPrefix)
		}
	}
	return prefixes
}

// topPathspec is the pathspec of a path relative to the top of the
// repository, whichever directory git runs in. The top itself is ":/": git
// reads ":(top)." as a path named ".", which no file has.
func topPathspec(entry string) string {
	if entry == "." {
		return ":/"
	}
	return ":(top)" + entry
}

// literalPathspec is the pathspec of a path relative to the top of the
// repository that git reads as the name it is, glob characters included.
func literalPathspec(entry string) string {
	if entry == "." {
		return ":/"
	}
	return ":(top,literal)" + entry
}

// nestedIn reports whether the module in directory lives inside the module
// in parent, which then does not own its files.
func nestedIn(directory, parent string) bool {
	if directory == parent {
		return false
	}
	return parent == "." || strings.HasPrefix(directory, parent+"/")
}

// within reports whether entry, a file or a directory, is directory or lies
// under it.
func within(entry, directory string) bool {
	return entry == directory || nestedIn(entry, directory)
}
