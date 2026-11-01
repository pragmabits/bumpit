package app

import (
	"github.com/pragmabits/bumpit/internal/gitx"
	"github.com/pragmabits/bumpit/internal/semver"
)

// ModuleRelease is the next release of one module, as bumpit modules lists
// it. A module whose plan failed carries the error instead.
type ModuleRelease struct {
	Directory  string `json:"directory"`
	Path       string `json:"path,omitempty"`
	CurrentTag string `json:"current_tag,omitempty"`
	NextTag    string `json:"next_tag,omitempty"`
	HasRelease bool   `json:"has_release"`
	// Hold is why the release waits, nil when it does not.
	Hold  *Hold  `json:"hold,omitempty"`
	Error string `json:"error,omitempty"`
	// Declared reports a module bumpit.yaml declares, which has no go.mod
	// and no module path.
	Declared bool `json:"declared,omitempty"`
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
		release := ModuleRelease{Directory: module.directory, Path: module.modulePath(), Declared: module.declared()}
		plan, err := planRelease(topClient, scope, otherModulePrefixes(scope, modules, ""), Options{StartVersion: opts.StartVersion})
		if err != nil {
			release.Error = err.Error()
		}
		release.CurrentTag, release.NextTag, release.HasRelease = plan.CurrentTag, plan.NextTag, plan.HasRelease
		releases = append(releases, release)
		if version, ok := plan.newestVersion(); ok && !module.declared() {
			newest[module.goModule.Path] = version
		}
	}
	holdReleases(releases, modules, newest)
	return releases, nil
}
