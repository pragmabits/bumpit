package app

import (
	"sync"

	"github.com/pragmabits/bumpit/internal/gitx"
	"github.com/pragmabits/bumpit/internal/gomod"
	"github.com/pragmabits/bumpit/internal/nodejs"
)

// repositorySnapshot is what a run reads of a repository, each thing once and
// only when it needs it. The repository does not change while bumpit runs: the
// only file a run writes is its config, which is neither a go.mod nor a change to
// what git tracks. So however many edits it proves and proposals it rehearses, it
// asks git and reads the files once.
type repositorySnapshot struct {
	client gitx.Client
	// top is the top directory of the working tree.
	top func() (string, error)
	// goModules are the Go modules the tracked go.mod files describe.
	goModules func() ([]gomod.Module, error)
	// tracked are the tracked files.
	tracked func() (trackedTree, error)
	// embeds are the //go:embed patterns of the Go files read so far.
	embeds embedCache
	// tagNames are the answers of git about the tag names asked so far.
	tagNames map[string]bool
	// packages are the Node.js packages of the repository, once they are read,
	// and packageErrors why those a directory reads cannot be read, by directory.
	packages      *nodejs.Repository
	packageErrors map[string]error
}

func newRepositorySnapshot(client gitx.Client) *repositorySnapshot {
	top := sync.OnceValues(client.Top)
	return &repositorySnapshot{
		client: client,
		top:    top,
		goModules: sync.OnceValues(func() ([]gomod.Module, error) {
			directory, err := top()
			if err != nil {
				return nil, err
			}
			return discoverModules(client, directory)
		}),
		tracked:       sync.OnceValues(func() (trackedTree, error) { return listTracked(client) }),
		embeds:        embedCache{},
		tagNames:      map[string]bool{},
		packageErrors: map[string]error{},
	}
}

// packagesOf is the Node.js packages of the repository at top, of which tracked is
// the tracked files.
func (s *repositorySnapshot) packagesOf(top string, tracked trackedTree) *nodejs.Repository {
	if s.packages == nil {
		read := nodejs.Read(top, tracked.files)
		s.packages = &read
	}
	return s.packages
}

// packageError is why the Node.js packages that directory reads cannot be read,
// nil when they can.
func (s *repositorySnapshot) packageError(top string, tracked trackedTree, directory string) error {
	found, asked := s.packageErrors[directory]
	if !asked {
		_, found = s.packagesOf(top, tracked).Inputs(directory)
		s.packageErrors[directory] = found
	}
	return found
}

// validTagName reports whether git accepts name as the name of a tag.
func (s *repositorySnapshot) validTagName(name string) (bool, error) {
	if valid, asked := s.tagNames[name]; asked {
		return valid, nil
	}
	valid, err := s.client.ValidTagName(name)
	if err == nil {
		s.tagNames[name] = valid
	}
	return valid, err
}
