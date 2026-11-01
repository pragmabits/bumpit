package app

import (
	"fmt"

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
	if found, known := n.derived[module.directory]; known {
		return found.paths, found.err
	}
	if n.packages == nil {
		read := nodejs.Read(n.top, n.tracked)
		n.packages = &read
	}

	var found derivation
	inputs, err := n.packages.Inputs(module.directory)
	if err != nil {
		found.err = fmt.Errorf("%s: %w", module.directory, err)
	} else {
		found.paths = keptDerivedPaths(module, inputs, modules)
	}
	n.derived[module.directory] = found
	return found.paths, found.err
}
