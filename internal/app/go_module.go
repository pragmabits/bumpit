package app

import (
	"path"
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

// Hold is why the release of a module waits: it requires modules of the
// repository below their newest version, and released now it would require
// the older ones.
type Hold struct {
	Requirements []OutdatedRequirement `json:"requirements"`
	// Cycle reports that the module waits, through held modules, on itself,
	// so no rerun releases it.
	Cycle bool `json:"cycle,omitempty"`
}

// OutdatedRequirement is a module of the repository that a module requires
// below its newest version: the next one when it has a release, else its
// current one.
type OutdatedRequirement struct {
	Directory string `json:"directory"`
	Path      string `json:"path"`
	Requires  string `json:"requires"`
	Newest    string `json:"newest"`
	// Held reports that the required module is held too, so its newest
	// version is not tagged yet.
	Held bool `json:"held,omitempty"`
}

// holdReleases holds the release of every module that requires a module of
// the repository below its newest version. Tagged at the same commit, the
// release would require the older version, while the code it was built
// against may need the newer one. releases and modules share their order.
func holdReleases(releases []ModuleRelease, modules []repositoryModule, newest map[string]semver.Version) {
	goModules := goModulesOf(modules)
	holds := map[string]*Hold{}
	for index, module := range modules {
		if module.declared() || !releases[index].HasRelease {
			continue
		}
		if outdated := outdatedRequirements(*module.goModule, goModules, newest); len(outdated) > 0 {
			releases[index].Hold = &Hold{Requirements: outdated}
			holds[module.directory] = releases[index].Hold
		}
	}

	for _, hold := range holds {
		for index := range hold.Requirements {
			_, hold.Requirements[index].Held = holds[hold.Requirements[index].Directory]
		}
	}
	for directory, hold := range holds {
		hold.Cycle = waitsOnItself(directory, holds)
	}
}

// outdatedRequirements lists the modules of the repository that module
// requires below their newest version.
func outdatedRequirements(module gomod.Module, modules []gomod.Module, newest map[string]semver.Version) []OutdatedRequirement {
	var outdated []OutdatedRequirement
	for _, other := range modules {
		required, requires := module.Requires[other.Path]
		version, known := newest[other.Path]
		if !requires || !known || !requiresBelow(required, version) {
			continue
		}
		outdated = append(outdated, OutdatedRequirement{
			Directory: other.Directory,
			Path:      other.Path,
			Requires:  required,
			Newest:    "v" + version.String(),
		})
	}
	return outdated
}

// waitsOnItself reports whether the module in start waits, through held
// modules, on itself.
func waitsOnItself(start string, holds map[string]*Hold) bool {
	visited := map[string]bool{}
	pending := []string{start}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		for _, requirement := range holds[current].Requirements {
			if requirement.Directory == start && requirement.Held {
				return true
			}
			if requirement.Held && !visited[requirement.Directory] {
				visited[requirement.Directory] = true
				pending = append(pending, requirement.Directory)
			}
		}
	}
	return false
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
	return compareDirectories(left.Directory, right.Directory)
}

// compareDirectories orders the root directory first and the others by name.
func compareDirectories(left, right string) int {
	switch {
	case left == right:
		return 0
	case left == ".":
		return -1
	case right == ".":
		return 1
	default:
		return strings.Compare(left, right)
	}
}

// goModulesOf keeps the go.mod of each Go module among modules.
func goModulesOf(modules []repositoryModule) []gomod.Module {
	var goModules []gomod.Module
	for _, module := range modules {
		if !module.declared() {
			goModules = append(goModules, *module.goModule)
		}
	}
	return goModules
}

// checkVersion refuses a version the module path of a Go module cannot
// carry. A declared module has no module path and takes any version.
func (m *moduleScope) checkVersion(next semver.Version) error {
	if m == nil || m.module.declared() {
		return nil
	}
	return m.module.goModule.CheckVersion("v" + next.String())
}

// dependentsBelow lists the Go modules of the repository that require the
// Go module at a version below next.
func (m *moduleScope) dependentsBelow(next semver.Version) []Dependent {
	if m == nil || m.module.declared() {
		return nil
	}

	var dependents []Dependent
	for _, other := range goModulesOf(m.modules) {
		required, ok := other.Requires[m.module.goModule.Path]
		if ok && requiresBelow(required, next) {
			dependents = append(dependents, Dependent{Directory: other.Directory, Path: other.Path, Requires: required})
		}
	}
	return dependents
}

// requiresBelow reports whether a version a go.mod requires is below version.
// A requirement that is not a semantic version, such as the placeholder
// v0.0.0-00010101000000-000000000000 beside a replace directive, counts as
// below: the go command cannot find it once the replace is ignored.
func requiresBelow(required string, version semver.Version) bool {
	parsed, err := semver.Parse(strings.TrimPrefix(required, "v"))
	return err != nil || semver.Compare(parsed, version) < 0
}

// newestVersion is the version of the module once the plan is released: the
// next one when it has a release, else the current one. A module with neither
// has none.
func (r ReleasePlan) newestVersion() (semver.Version, bool) {
	newest := r.CurrentVersion
	if r.HasRelease {
		newest = r.NextVersion
	}
	version, err := semver.Parse(newest)
	return version, err == nil
}
