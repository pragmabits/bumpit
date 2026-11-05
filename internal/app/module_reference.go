package app

import (
	"fmt"
	"slices"
)

// moduleIdentity is what a command line can call a module: the name bumpit.yaml
// gives it, empty when it gives none, and its directories.
type moduleIdentity struct {
	name        string
	directories []string
}

// shippedByName is what shippedBy calls the module: its name, else its
// directory.
func (i moduleIdentity) shippedByName() string {
	if i.name != "" || len(i.directories) == 0 {
		return i.name
	}
	return i.directories[0]
}

// moduleReference is a module as a command line names it: as written, which
// may be its name, and as the directory that writing means, relative to the top
// of the repository.
type moduleReference struct {
	written   string
	directory string
}

// find is the index among identities of the module the reference names, -1
// when it names none. A module with no name of its own is found by its
// directory alone, so that a path keeps meaning what it means from --repo. A
// reference that is the name of one module and the directory of another is
// refused: a name is never written with ./, which then says the directory.
func (r moduleReference) find(identities []moduleIdentity) (int, error) {
	byName := slices.IndexFunc(identities, func(identity moduleIdentity) bool {
		return identity.name != "" && identity.name == r.written
	})
	byDirectory := slices.IndexFunc(identities, func(identity moduleIdentity) bool {
		return slices.Contains(identity.directories, r.directory)
	})
	switch {
	case byName >= 0 && byDirectory >= 0 && byName != byDirectory:
		return -1, fmt.Errorf("%s is the name of one module and the directory %s of another: write ./%s for the directory", r.written, r.directory, r.written)
	case byName >= 0:
		return byName, nil
	default:
		return byDirectory, nil
	}
}
