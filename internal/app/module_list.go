package app

import (
	"github.com/pragmabits/bumpit/internal/gitx"
	"github.com/pragmabits/bumpit/internal/semver"
)

// ModuleRelease is the next release of one module, as bumpit modules lists
// it. A module whose plan failed carries the error instead.
type ModuleRelease struct {
	// Name is what the commands call the module, left out for a Go module
	// bumpit.yaml gives no name, which its directory names.
	Name string `json:"name,omitempty"`
	// Directory is the directory of a Go module.
	Directory string `json:"directory,omitempty"`
	// Directories are the directories of a module with no go.mod, which also
	// has no module path.
	Directories []string `json:"directories,omitempty"`
	Path        string   `json:"path,omitempty"`
	CurrentTag  string   `json:"current_tag,omitempty"`
	NextTag     string   `json:"next_tag,omitempty"`
	HasRelease  bool     `json:"has_release"`
	// Hold is why the release waits, nil when it does not.
	Hold  *Hold  `json:"hold,omitempty"`
	Error string `json:"error,omitempty"`
}

func newModuleRelease(module repositoryModule) ModuleRelease {
	if module.withoutGoMod() {
		return ModuleRelease{Name: module.name, Directories: module.directories}
	}
	release := ModuleRelease{Directory: module.goModule.Directory, Path: module.modulePath()}
	if module.named {
		release.Name = module.name
	}
	return release
}

// label is what the commands call the module.
func (r ModuleRelease) label() string {
	if r.Name != "" {
		return r.Name
	}
	return r.Directory
}

// ListModules plans the next release of every module of the repository, its
// Go modules and the modules opts declares, each with the start version of
// opts, and holds the releases of Go modules that require another module
// below its newest version. A module whose plan fails is listed with the
// error, so that one module does not hide the others.
func ListModules(opts Options) ([]ModuleRelease, error) {
	client, err := openRepository(opts)
	if err != nil {
		return nil, err
	}
	modules, top, err := discoverRepositoryModules(client, opts.Modules)
	if err != nil {
		return nil, err
	}

	topClient := gitx.New(top)
	releases := make([]ModuleRelease, 0, len(modules))
	newest := map[string]semver.Version{}
	for _, module := range modules {
		scope := &moduleScope{module: module, modules: modules}
		release := newModuleRelease(module)
		plan, err := planRelease(topClient, scope, otherModulePrefixes(scope, modules, ""), Options{StartVersion: opts.StartVersion})
		if err != nil {
			release.Error = err.Error()
		}
		release.CurrentTag, release.NextTag, release.HasRelease = plan.CurrentTag, plan.NextTag, plan.HasRelease
		releases = append(releases, release)
		if version, ok := plan.newestVersion(); ok && !module.withoutGoMod() {
			newest[module.goModule.Path] = version
		}
	}
	holdReleases(releases, modules, newest)
	return releases, nil
}
