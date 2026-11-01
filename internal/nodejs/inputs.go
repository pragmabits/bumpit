package nodejs

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// rootFiles are the files of a workspace root that an install reads: the
// root package.json, the lockfiles and the configuration of each manager.
var rootFiles = []string{
	"package.json", "pnpm-workspace.yaml",
	"pnpm-lock.yaml", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "bun.lock", "bun.lockb",
	".npmrc", ".yarnrc", ".yarnrc.yml", ".pnpmfile.cjs", ".pnpmfile.mjs", "bunfig.toml",
}

// rootDirectories are the directories of a workspace root that an install
// reads: the releases, plugins and patches of yarn, and the patches of pnpm.
var rootDirectories = []string{".yarn", "patches"}

// pnpmWorkspace is the part of a pnpm-workspace.yaml bumpit reads.
type pnpmWorkspace struct {
	Overrides map[string]string `yaml:"overrides"`
}

// Inputs lists what the package in directory is built from outside its own
// directory, among the tracked files of the repository, sorted:
//   - its local dependencies, transitively, among its dependencies,
//     devDependencies, optionalDependencies and peerDependencies, and among
//     the overrides of the workspace roots above it;
//   - the files an install reads in each workspace root above it: every
//     ancestor with a pnpm-workspace.yaml, or with a package.json that has
//     workspaces, not only the nearest, since yarn nests workspaces;
//   - every .yarnrc.yml above it, which yarn reads in parent directories.
//
// A dependency on the package itself or on an ancestor of it is left out: it
// would read the whole workspace, whose root files are read already. A
// directory with no package.json has no input. Inputs fails on a package.json
// or a pnpm-workspace.yaml it must read and cannot: its own, that of a root
// above it, or that of a package a path points at.
func (r Repository) Inputs(directory string) ([]string, error) {
	if _, readable := r.packages[directory]; !readable {
		if err, unreadable := r.unreadable[manifestOf(directory)]; unreadable {
			return nil, fmt.Errorf("read %s: %w", manifestOf(directory), err)
		}
		return nil, nil
	}

	roots, overrides, err := r.roots(directory)
	if err != nil {
		return nil, err
	}
	inputs := map[string]bool{}
	for _, root := range roots {
		for _, file := range r.filesOfRoot(root) {
			inputs[file] = true
		}
	}
	for _, ancestor := range ancestors(directory) {
		if yarnrc := path.Join(ancestor, ".yarnrc.yml"); r.files[yarnrc] {
			inputs[yarnrc] = true
		}
	}
	if err := r.followDependencies(directory, overrides, inputs); err != nil {
		return nil, err
	}
	return sortedKeys(inputs), nil
}

// roots lists the workspace roots above directory, nearest first, and the
// overrides they set, each with the root it is read from.
func (r Repository) roots(directory string) ([]string, []sourcedDependency, error) {
	var roots []string
	var overrides []sourcedDependency
	for _, ancestor := range ancestors(directory) {
		if err, unreadable := r.unreadable[manifestOf(ancestor)]; unreadable {
			return nil, nil, fmt.Errorf("read %s: %w", manifestOf(ancestor), err)
		}
		workspaceFile := path.Join(ancestor, "pnpm-workspace.yaml")
		found, readable := r.packages[ancestor]
		hasWorkspaceFile := r.files[workspaceFile]
		if isRoot := hasWorkspaceFile || readable && found.Workspaces; !isRoot {
			continue
		}

		roots = append(roots, ancestor)
		overrides = append(overrides, sourcedOverrides(found.Overrides, ancestor)...)
		if !hasWorkspaceFile {
			continue
		}
		pnpmOverrides, err := r.readPnpmOverrides(workspaceFile)
		if err != nil {
			return nil, nil, err
		}
		overrides = append(overrides, sourcedOverrides(pnpmOverrides, ancestor)...)
	}
	return roots, overrides, nil
}

// sourcedDependency is a dependency and the directory its path is read from.
type sourcedDependency struct {
	Dependency
	from string
}

func sourcedOverrides(overrides []Dependency, from string) []sourcedDependency {
	sourced := make([]sourcedDependency, 0, len(overrides))
	for _, override := range overrides {
		sourced = append(sourced, sourcedDependency{Dependency: override, from: from})
	}
	return sourced
}

func (r Repository) readPnpmOverrides(workspaceFile string) ([]Dependency, error) {
	data, err := os.ReadFile(filepath.Join(r.top, filepath.FromSlash(workspaceFile)))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", workspaceFile, err)
	}
	var read pnpmWorkspace
	if err := yaml.Unmarshal(data, &read); err != nil {
		return nil, fmt.Errorf("read %s: %w", workspaceFile, err)
	}
	return sortedDependencies(read.Overrides), nil
}

// filesOfRoot lists the root files and root directories the root tracks.
func (r Repository) filesOfRoot(root string) []string {
	var found []string
	for _, name := range rootFiles {
		if file := path.Join(root, name); r.files[file] {
			found = append(found, file)
		}
	}
	for _, name := range rootDirectories {
		if directory := path.Join(root, name); r.directories[directory] {
			found = append(found, directory)
		}
	}
	return found
}

// followDependencies adds to inputs the packages the package in directory
// depends on, and those they depend on in turn, with what the overrides of
// its roots point at, which applies to every package they install.
func (r Repository) followDependencies(directory string, overrides []sourcedDependency, inputs map[string]bool) error {
	pending := slices.Clone(overrides)
	for index := range pending {
		pending[index].Name = overrideName(pending[index].Name)
	}
	pending = append(pending, r.dependenciesOf(directory)...)

	visited := map[string]bool{directory: true}
	for len(pending) > 0 {
		dependency := pending[0]
		pending = pending[1:]
		pointed := r.resolve(dependency.Dependency, dependency.from)
		for _, found := range pointed.names {
			if within(directory, found) || inputs[found] {
				continue
			}
			inputs[found] = true
			if err := r.checkReadable(found, pointed.byPath); err != nil {
				return err
			}
			if !visited[found] {
				visited[found] = true
				pending = append(pending, r.dependenciesOf(found)...)
			}
		}
	}
	return nil
}

// checkReadable refuses a package a path points at whose package.json cannot
// be read: its own dependencies would go unread.
func (r Repository) checkReadable(directory string, byPath bool) error {
	if err, unreadable := r.unreadable[manifestOf(directory)]; byPath && unreadable {
		return fmt.Errorf("read %s: %w", manifestOf(directory), err)
	}
	return nil
}

func (r Repository) dependenciesOf(directory string) []sourcedDependency {
	return sourcedOverrides(r.packages[directory].Dependencies, directory)
}

// overrideName is the package an override key names: foo in foo, **/foo,
// parent>foo or foo@1.
func overrideName(key string) string {
	name := key[strings.LastIndex(key, ">")+1:]
	name = strings.TrimPrefix(name, "**/")
	if alias := aliasName(name); alias != "" {
		return alias
	}
	return name
}

// ancestors lists the directories above directory, up to the top of the
// repository, nearest first. The top has none.
func ancestors(directory string) []string {
	var found []string
	for directory != "." {
		directory = path.Dir(directory)
		found = append(found, directory)
	}
	return found
}

func manifestOf(directory string) string {
	return path.Join(directory, "package.json")
}

// within reports whether directory is parent or lies under it.
func within(directory, parent string) bool {
	return parent == "." || directory == parent || strings.HasPrefix(directory, parent+"/")
}
