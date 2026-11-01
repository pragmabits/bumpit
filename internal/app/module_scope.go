package app

import (
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

// resolveModuleScope picks the module a plan versions: the directory given
// with --module, relative to the repository path, or, with neither --module
// nor --match, the root module when the repository has one. Outside a
// module it returns nil, and the plan reads tags by pattern alone. It also
// returns the modules it discovered, none with --match alone.
func resolveModuleScope(client gitx.Client, repository, moduleDirectory, tagMatch string, declarations []ModuleDeclaration) (*moduleScope, []repositoryModule, error) {
	if moduleDirectory == "" && tagMatch != "" {
		return nil, nil, nil
	}

	modules, top, err := discoverRepositoryModules(client, declarations)
	if err != nil {
		return nil, nil, err
	}

	directory := "."
	if moduleDirectory != "" {
		if directory, err = relativeToTop(top, repository, moduleDirectory); err != nil {
			return nil, nil, err
		}
	}
	for _, module := range modules {
		if module.directory == directory {
			return &moduleScope{module: module, modules: modules}, modules, nil
		}
	}

	if moduleDirectory == "" {
		return nil, modules, nil
	}
	return nil, nil, fmt.Errorf("no module in %s: the directory holds no tracked go.mod, and bumpit.yaml declares no module there", moduleDirectory)
}

// relativeToTop turns a directory given relative to the repository path into
// one relative to the top of the working tree, slash separated.
func relativeToTop(top, repository, directory string) (string, error) {
	absolute, err := filepath.Abs(filepath.Join(repository, directory))
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	}

	relative, err := filepath.Rel(top, absolute)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(relative), nil
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
// The parts are its directory, less the modules nested in it, each of its
// paths, and the same of every module it ships.
func (m *moduleScope) queries() []commitQuery {
	queries := []commitQuery{{pathspecs: ownPathspecs(m.module, m.modules)}}
	queries = append(queries, m.goCodeQueries()...)
	for _, entry := range m.module.readPaths() {
		queries = append(queries, commitQuery{pathspecs: []string{topPathspec(entry)}, via: entry})
	}
	for _, shipped := range m.shippedModules() {
		queries = append(queries, commitQuery{pathspecs: ownPathspecs(shipped, m.modules), via: shipped.directory, shipped: true})
		for _, entry := range shipped.readPaths() {
			queries = append(queries, commitQuery{pathspecs: []string{topPathspec(entry)}, via: shipped.directory, shipped: true})
		}
	}
	return queries
}

// goCodeQueries are the queries of the Go code a Go module has in the
// declared modules nested in it: the .go files of a declared directory are
// still packages of the Go module, compiled and released with it, so their
// commits are its own, with their own level, whether or not it ships the
// rest of the directory. One query reads each outermost such directory, less
// the Go modules nested in it, which own their files.
func (m *moduleScope) goCodeQueries() []commitQuery {
	if m.module.declared() {
		return nil
	}

	var queries []commitQuery
	for _, other := range m.modules {
		if !other.declared() || !m.holdsGoCodeOf(other) {
			continue
		}
		pathspecs := []string{":(top,glob)" + other.directory + "/**/*.go"}
		for _, nested := range m.modules {
			if !nested.declared() && nestedIn(nested.directory, other.directory) {
				pathspecs = append(pathspecs, ":(top,exclude)"+nested.directory)
			}
		}
		queries = append(queries, commitQuery{pathspecs: pathspecs})
	}
	return queries
}

// holdsGoCodeOf reports whether the Go code in the declared directory of
// other belongs to this Go module: the Go module is the nearest module
// around it, so no Go module between them owns the code, and no declared
// module between them reads the directory already.
func (m *moduleScope) holdsGoCodeOf(other repositoryModule) bool {
	return encloses(m.module, other, m.modules)
}

// ownPathspecs selects the files a module owns: its directory, less every
// module nested in it.
func ownPathspecs(module repositoryModule, modules []repositoryModule) []string {
	pathspecs := []string{topPathspec(module.directory)}
	for _, other := range modules {
		if nestedIn(other.directory, module.directory) {
			pathspecs = append(pathspecs, ":(top,exclude)"+other.directory)
		}
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
		if read.derivationError != nil {
			return read.derivationError
		}
	}
	return nil
}

// shippedModules are the declared modules whose release the module carries,
// by directory.
func (m *moduleScope) shippedModules() []repositoryModule {
	var shipped []repositoryModule
	for _, other := range m.modules {
		if other.declared() && other.directory != m.module.directory && carriersOf(other, m.modules)[m.module.directory] {
			shipped = append(shipped, other)
		}
	}
	return shipped
}

// describe writes into plan what the module is, and what it reads beyond its
// own directory or leaves out of it. A Go module has its module path; a
// declared module has its directory, its paths and its carriers.
func (m *moduleScope) describe(plan *ReleasePlan) {
	if m == nil {
		return
	}

	plan.Module = m.module.modulePath()
	if m.module.declared() {
		plan.Directory, plan.Paths, plan.ShippedBy = m.module.directory, m.module.paths, m.module.shippedBy
		plan.DerivedPaths = m.module.derivedPaths
	}
	shipped := m.shippedModules()
	for _, module := range shipped {
		plan.Ships = append(plan.Ships, module.directory)
	}
	plan.LeavesOut = m.leftOut(shipped)
}

// leftOut lists the declared modules whose commits the module does not read
// although they lie in a directory it reads: its own, or that of a module it
// ships, with no other module between. It reads those it ships, and those a
// path of its own or of a shipped module names.
func (m *moduleScope) leftOut(shipped []repositoryModule) []string {
	readers := append([]repositoryModule{m.module}, shipped...)
	var paths []string
	for _, reader := range readers {
		paths = append(paths, reader.readPaths()...)
	}

	var left []string
	for _, other := range m.modules {
		read := slices.ContainsFunc(readers, func(reader repositoryModule) bool { return reader.directory == other.directory }) ||
			slices.ContainsFunc(paths, func(entry string) bool { return within(other.directory, entry) })
		enclosed := slices.ContainsFunc(readers, func(reader repositoryModule) bool { return encloses(reader, other, m.modules) })
		if other.declared() && !read && enclosed {
			left = append(left, other.directory)
		}
	}
	return left
}

// encloses reports whether outer is the nearest module around inner.
func encloses(outer, inner repositoryModule, modules []repositoryModule) bool {
	if !nestedIn(inner.directory, outer.directory) {
		return false
	}
	return !slices.ContainsFunc(modules, func(between repositoryModule) bool {
		return nestedIn(between.directory, outer.directory) && nestedIn(inner.directory, between.directory)
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
