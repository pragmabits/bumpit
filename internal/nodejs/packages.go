// Package nodejs reads the Node.js packages of a repository: its package.json
// files, the workspace roots pnpm, npm, yarn and bun declare, and what a
// package is built from among the files of the repository.
package nodejs

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Dependency is a dependency a package.json asks for: the name it gives the
// dependency and the specification it asks for, a version range, an alias or
// a path.
type Dependency struct {
	Name          string
	Specification string
}

// Package is a package.json of the repository that could be read.
type Package struct {
	// Directory holds the package.json, slash separated and relative to the
	// top of the repository: "." for the top.
	Directory string
	Name      string
	// Version is the version the package gives itself, empty when it gives
	// none.
	Version string
	// Private reports a package its manager never publishes.
	Private bool
	// Dependencies are those of its dependencies, devDependencies,
	// optionalDependencies and peerDependencies, in that order.
	Dependencies []Dependency
	// Overrides are the specifications its overrides, resolutions and
	// pnpm.overrides set, each under the name of the package it overrides.
	Overrides []Dependency
	// Workspaces reports a workspaces key: the package is the root of a
	// workspace.
	Workspaces bool
}

// Repository is the Node.js packages of a repository and the files it tracks.
type Repository struct {
	top         string
	files       map[string]bool
	directories map[string]bool
	packages    map[string]Package
	unreadable  map[string]error
	names       map[string][]string
}

// manifest is the part of a package.json bumpit reads.
type manifest struct {
	Name                 string            `json:"name"`
	Version              json.RawMessage   `json:"version"`
	Private              json.RawMessage   `json:"private"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	Workspaces           json.RawMessage   `json:"workspaces"`
	Overrides            json.RawMessage   `json:"overrides"`
	Resolutions          map[string]string `json:"resolutions"`
	Pnpm                 struct {
		Overrides map[string]string `json:"overrides"`
	} `json:"pnpm"`
}

// Read reads the package.json files among tracked, the tracked files of the
// repository at top, slash separated and relative to it. The files under a
// node_modules directory are left out, as every manager leaves them out. A
// package.json that cannot be read is no package; the derivations that must
// read it report why. A tracked package.json deleted from the working tree is
// no package either, as a deleted Go file embeds nothing.
func Read(top string, tracked []string) Repository {
	repository := Repository{
		top:         top,
		files:       map[string]bool{},
		directories: map[string]bool{},
		packages:    map[string]Package{},
		unreadable:  map[string]error{},
		names:       map[string][]string{},
	}
	for _, file := range tracked {
		repository.files[file] = true
		for directory := path.Dir(file); directory != "."; directory = path.Dir(directory) {
			repository.directories[directory] = true
		}
		if path.Base(file) == "package.json" && !slices.Contains(strings.Split(file, "/"), "node_modules") {
			repository.readPackage(file)
		}
	}
	return repository
}

func (r *Repository) readPackage(file string) {
	read, err := readManifest(filepath.Join(r.top, filepath.FromSlash(file)))
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		r.unreadable[file] = err
		return
	}

	directory := path.Dir(file)
	found := Package{
		Directory:  directory,
		Name:       read.Name,
		Version:    stringOf(read.Version),
		Private:    isTruthy(read.Private),
		Workspaces: len(read.Workspaces) > 0 && string(read.Workspaces) != "null",
		Overrides:  overridesOf(read),
	}
	for _, list := range []map[string]string{read.Dependencies, read.DevDependencies, read.OptionalDependencies, read.PeerDependencies} {
		found.Dependencies = append(found.Dependencies, sortedDependencies(list)...)
	}
	r.packages[directory] = found
	if found.Name != "" {
		r.names[found.Name] = append(r.names[found.Name], directory)
	}
}

// stringOf is the string a JSON value holds, empty for any other value, so
// that a version bumpit cannot read does not make the package unreadable for
// the derivations, which do not read it.
func stringOf(value json.RawMessage) string {
	var text string
	if json.Unmarshal(value, &text) != nil {
		return ""
	}
	return text
}

// isTruthy reports a JSON value JavaScript takes as true, as npm reads
// private: anything but false, null, 0, an empty string, or no value.
func isTruthy(value json.RawMessage) bool {
	var read any
	if json.Unmarshal(value, &read) != nil {
		return false
	}
	switch typed := read.(type) {
	case nil:
		return false
	case bool:
		return typed
	case float64:
		return typed != 0
	case string:
		return typed != ""
	default:
		return true
	}
}

// Packages are the packages that could be read, by directory.
func (r Repository) Packages() []Package {
	packages := make([]Package, 0, len(r.packages))
	for _, found := range r.packages {
		packages = append(packages, found)
	}
	slices.SortFunc(packages, func(left, right Package) int { return strings.Compare(left.Directory, right.Directory) })
	return packages
}

// readManifest reads the package.json at file. A byte order mark, which npm
// and pnpm accept, is dropped first.
func readManifest(file string) (manifest, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return manifest{}, err
	}
	var read manifest
	err = json.Unmarshal(bytes.TrimPrefix(data, []byte("\uFEFF")), &read)
	return read, err
}

// overridesOf lists what the overrides of npm, the resolutions of yarn and
// the pnpm.overrides of a package.json set. An override of npm nests, and its
// "." key stands for the package of the object it is in.
func overridesOf(read manifest) []Dependency {
	overrides := sortedDependencies(read.Resolutions)
	overrides = append(overrides, sortedDependencies(read.Pnpm.Overrides)...)
	if len(read.Overrides) > 0 {
		var nested map[string]json.RawMessage
		if json.Unmarshal(read.Overrides, &nested) == nil {
			overrides = append(overrides, nestedOverrides("", nested)...)
		}
	}
	return overrides
}

func nestedOverrides(parent string, nested map[string]json.RawMessage) []Dependency {
	var overrides []Dependency
	for _, name := range sortedKeys(nested) {
		var specification string
		if json.Unmarshal(nested[name], &specification) == nil {
			if name == "." {
				name = parent
			}
			overrides = append(overrides, Dependency{Name: name, Specification: specification})
			continue
		}
		var deeper map[string]json.RawMessage
		if json.Unmarshal(nested[name], &deeper) == nil {
			overrides = append(overrides, nestedOverrides(name, deeper)...)
		}
	}
	return overrides
}

func sortedDependencies(list map[string]string) []Dependency {
	dependencies := make([]Dependency, 0, len(list))
	for _, name := range sortedKeys(list) {
		dependencies = append(dependencies, Dependency{Name: name, Specification: list[name]})
	}
	return dependencies
}

func sortedKeys[Value any](values map[string]Value) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
