package app

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pragmabits/bumpit/internal/gitx"
	"github.com/pragmabits/bumpit/internal/gomod"
	"github.com/pragmabits/bumpit/internal/semver"
)

// Dependent is a module of the repository that requires the module being
// released at a version below the next one.
type Dependent struct {
	Directory string `json:"directory"`
	Path      string `json:"path"`
	Requires  string `json:"requires"`
}

// ModuleRelease is the next release of one module, as bumpit modules lists
// it. A module whose plan failed carries the error instead.
type ModuleRelease struct {
	Directory  string `json:"directory"`
	Path       string `json:"path"`
	CurrentTag string `json:"current_tag,omitempty"`
	NextTag    string `json:"next_tag,omitempty"`
	HasRelease bool   `json:"has_release"`
	Error      string `json:"error,omitempty"`
}

// moduleScope is the Go module a plan versions: its tags start with the
// module's tag prefix, and its commits are those touching its directory,
// less the modules nested in it.
type moduleScope struct {
	module  gomod.Module
	modules []gomod.Module
}

// ListModules plans the next release of every Go module in the repository.
// A module whose plan fails is listed with the error, so that one module does
// not hide the others.
func ListModules(opts Options) ([]ModuleRelease, error) {
	client, err := openRepository(opts)
	if err != nil {
		return nil, err
	}
	modules, top, err := discoverModules(client)
	if err != nil {
		return nil, err
	}

	releases := make([]ModuleRelease, 0, len(modules))
	for _, module := range modules {
		moduleOptions := Options{Repository: top, AllowDirty: true, Module: module.Directory}
		release := ModuleRelease{Directory: module.Directory, Path: module.Path}
		plan, err := BuildReleasePlan(moduleOptions)
		if err != nil {
			release.Error = err.Error()
		}
		release.CurrentTag, release.NextTag, release.HasRelease = plan.CurrentTag, plan.NextTag, plan.HasRelease
		releases = append(releases, release)
	}
	return releases, nil
}

// resolveModuleScope picks the module a plan versions: the directory given
// with --module, relative to the repository path, or, with neither --module
// nor --match, the root module when the repository has one. Outside a Go
// module it returns nil, and the plan reads tags by pattern alone.
func resolveModuleScope(client gitx.Client, repository, moduleDirectory, tagMatch string) (*moduleScope, error) {
	if moduleDirectory == "" && tagMatch != "" {
		return nil, nil
	}

	modules, top, err := discoverModules(client)
	if err != nil {
		return nil, err
	}

	directory := "."
	if moduleDirectory != "" {
		if directory, err = relativeToTop(top, repository, moduleDirectory); err != nil {
			return nil, err
		}
	}
	for _, module := range modules {
		if module.Directory == directory {
			return &moduleScope{module: module, modules: modules}, nil
		}
	}

	if moduleDirectory == "" {
		return nil, nil
	}
	return nil, fmt.Errorf("no Go module in %s: the directory holds no tracked go.mod", moduleDirectory)
}

// discoverModules loads every tracked go.mod of the repository, leaving out
// the directories the go command ignores.
func discoverModules(client gitx.Client) ([]gomod.Module, string, error) {
	top, err := client.Top()
	if err != nil {
		return nil, "", err
	}
	files, err := client.TrackedFiles(":(top,glob)**/go.mod")
	if err != nil {
		return nil, "", err
	}

	var modules []gomod.Module
	for _, file := range files {
		directory := path.Dir(file)
		if gomod.Ignored(directory) {
			continue
		}
		module, err := gomod.Load(top, directory)
		if err != nil {
			return nil, "", err
		}
		modules = append(modules, module)
	}
	slices.SortFunc(modules, compareModules)
	return modules, top, nil
}

// compareModules orders the root module first and the others by directory.
func compareModules(left, right gomod.Module) int {
	switch {
	case left.Directory == right.Directory:
		return 0
	case left.Directory == ".":
		return -1
	case right.Directory == ".":
		return 1
	default:
		return strings.Compare(left.Directory, right.Directory)
	}
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
		return scope.module.TagPrefix() + "*"
	case tagMatch != "":
		return tagMatch
	default:
		return "v*"
	}
}

// pathspecs selects the files of the module: its directory, less every module
// nested in it. Outside a module scope, every commit counts.
func (m *moduleScope) pathspecs() []string {
	if m == nil {
		return nil
	}

	pathspecs := []string{":/"}
	if m.module.Directory != "." {
		pathspecs = []string{":(top)" + m.module.Directory}
	}
	for _, other := range m.modules {
		if nestedIn(other.Directory, m.module.Directory) {
			pathspecs = append(pathspecs, ":(top,exclude)"+other.Directory)
		}
	}
	return pathspecs
}

// checkVersion refuses a version the module path cannot carry.
func (m *moduleScope) checkVersion(next semver.Version) error {
	if m == nil {
		return nil
	}
	return m.module.CheckVersion("v" + next.String())
}

// dependentsBelow lists the modules of the repository that require the module
// at a version below next.
func (m *moduleScope) dependentsBelow(next semver.Version) []Dependent {
	if m == nil {
		return nil
	}

	var dependents []Dependent
	for _, other := range m.modules {
		required, ok := other.Requires[m.module.Path]
		if !ok {
			continue
		}
		version, err := semver.Parse(strings.TrimPrefix(required, "v"))
		if err != nil || semver.Compare(version, next) < 0 {
			dependents = append(dependents, Dependent{Directory: other.Directory, Path: other.Path, Requires: required})
		}
	}
	return dependents
}

func (m *moduleScope) modulePath() string {
	if m == nil {
		return ""
	}
	return m.module.Path
}

// nestedIn reports whether the module in directory lives inside the module
// in parent, which then does not own its files.
func nestedIn(directory, parent string) bool {
	if directory == parent {
		return false
	}
	return parent == "." || strings.HasPrefix(directory, parent+"/")
}
