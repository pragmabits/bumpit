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

// Inputs lists what the packages in directory are built from outside it,
// among the tracked files of the repository, sorted:
//   - their local dependencies, transitively: those of every package.json in
//     directory, in its dependencies, devDependencies, optionalDependencies
//     and peerDependencies, and those the overrides of directory and of the
//     workspace roots above it point at;
//   - the files an install reads in each workspace root above directory:
//     every ancestor with a pnpm-workspace.yaml, or with a package.json that
//     has workspaces, not only the nearest, since yarn nests workspaces;
//   - every .yarnrc.yml above directory, which yarn reads in parent
//     directories.
//
// A dependency on directory or on an ancestor of it is left out: it would
// read the whole workspace, whose root files are read already. A directory
// that holds no package.json has no input. Inputs fails on a package.json or
// a pnpm-workspace.yaml it must read and cannot: one in directory, that of a
// root above it, or that of a package a path points at.
func (r Repository) Inputs(directory string) ([]string, error) {
	own, err := r.packagesWithin(directory)
	if err != nil || len(own) == 0 {
		return nil, err
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

	pending := overrides
	for _, found := range own {
		pending = append(pending, r.dependenciesOf(found)...)
	}
	if err := r.followDependencies(directory, pending, inputs); err != nil {
		return nil, err
	}
	return sortedKeys(inputs), nil
}

// packagesWithin lists the directories of the packages in directory, its own
// included, and fails on a package.json there that cannot be read.
func (r Repository) packagesWithin(directory string) ([]string, error) {
	for file, err := range r.unreadable {
		if within(path.Dir(file), directory) {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
	}
	var own []string
	for found := range r.packages {
		if within(found, directory) {
			own = append(own, found)
		}
	}
	slices.Sort(own)
	return own, nil
}

// roots lists the workspace roots above directory, nearest first, and the
// overrides they and directory itself set, when it is a root too, each with
// the directory its paths are read from.
func (r Repository) roots(directory string) ([]string, []sourcedDependency, error) {
	_, overrides, err := r.rootAt(directory)
	if err != nil {
		return nil, nil, err
	}
	var roots []string
	for _, ancestor := range ancestors(directory) {
		if err, unreadable := r.unreadable[manifestOf(ancestor)]; unreadable {
			return nil, nil, fmt.Errorf("read %s: %w", manifestOf(ancestor), err)
		}
		isRoot, rootOverrides, err := r.rootAt(ancestor)
		if err != nil {
			return nil, nil, err
		}
		if isRoot {
			roots = append(roots, ancestor)
			overrides = append(overrides, rootOverrides...)
		}
	}
	return roots, overrides, nil
}

// rootAt reports whether directory is a workspace root, and the overrides it
// sets: those of its package.json, and those of its pnpm-workspace.yaml,
// each named after the package it overrides.
func (r Repository) rootAt(directory string) (bool, []sourcedDependency, error) {
	workspaceFile := path.Join(directory, "pnpm-workspace.yaml")
	found, readable := r.packages[directory]
	hasWorkspaceFile := r.files[workspaceFile]
	if isRoot := hasWorkspaceFile || readable && found.Workspaces; !isRoot {
		return false, nil, nil
	}

	overrides := found.Overrides
	if hasWorkspaceFile {
		pnpmOverrides, err := r.readPnpmOverrides(workspaceFile)
		if err != nil {
			return false, nil, err
		}
		overrides = append(slices.Clone(overrides), pnpmOverrides...)
	}

	sourced := make([]sourcedDependency, 0, len(overrides))
	for _, override := range overrides {
		named := Dependency{Name: overrideName(override.Name), Specification: override.Specification}
		sourced = append(sourced, sourcedDependency{Dependency: named, from: directory})
	}
	return true, sourced, nil
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

// followDependencies adds to inputs where pending points, and where the
// packages it reaches depend in turn.
func (r Repository) followDependencies(directory string, pending []sourcedDependency, inputs map[string]bool) error {
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
			pending = append(pending, r.dependenciesOf(found)...)
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
