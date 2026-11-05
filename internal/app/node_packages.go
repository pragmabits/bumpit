package app

import (
	"fmt"
	"slices"

	"github.com/pragmabits/bumpit/internal/nodejs"
)

// nodePackages derives the paths declared modules read because of their
// package.json. It reads the Node.js packages of the repository the first
// time a plan needs a derivation, and keeps each derivation, so that a run
// that plans no declared module reads none, and one that plans every module
// reads them once.
type nodePackages struct {
	top      string
	tracked  []string
	packages *nodejs.Repository
	derived  map[string]derivation
}

// derivation is the paths a declared module derives, or why it cannot.
type derivation struct {
	paths []string
	err   error
}

func newNodePackages(top string, tracked trackedTree) *nodePackages {
	return &nodePackages{top: top, tracked: tracked.files, derived: map[string]derivation{}}
}

// derive is the paths module derives from its package.json, less what it
// reads already. A nil nodePackages, that of a Go module, derives none.
func (n *nodePackages) derive(module repositoryModule, modules []repositoryModule) ([]string, error) {
	if n == nil {
		return nil, nil
	}
	if found, known := n.derived[module.name]; known {
		return found.paths, found.err
	}
	found := n.deriveDirectories(module, modules)
	n.derived[module.name] = found
	return found.paths, found.err
}

// deriveDirectories joins what the package.json files of each directory of
// module derive, sorted.
func (n *nodePackages) deriveDirectories(module repositoryModule, modules []repositoryModule) derivation {
	var found derivation
	for _, directory := range module.directories {
		inputs, err := n.repository().Inputs(directory)
		if err != nil {
			return derivation{err: fmt.Errorf("%s: %w", module.name, err)}
		}
		found.paths = append(found.paths, keptDerivedPaths(module, inputs, modules)...)
	}
	slices.Sort(found.paths)
	found.paths = slices.Compact(found.paths)
	return found
}

// repository is the Node.js packages of the repository, read the first time
// they are needed.
func (n *nodePackages) repository() *nodejs.Repository {
	if n.packages == nil {
		read := nodejs.Read(n.top, n.tracked)
		n.packages = &read
	}
	return n.packages
}
