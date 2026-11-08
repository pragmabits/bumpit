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
	// names are the directories the module names asked so far lead to.
	names map[string]string
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
		names:         map[string]string{},
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

// directoryOfName is the directory, relative to the top, that name means read as a
// path from the top in every clone of the repository, the links the repository
// tracks followed. An error is a failure to ask.
func (s *repositorySnapshot) directoryOfName(name string) (string, error) {
	if directory, asked := s.names[name]; asked {
		return directory, nil
	}
	top, err := s.top()
	if err != nil {
		return "", err
	}
	tracked, err := s.tracked()
	if err != nil {
		return "", err
	}
	s.names[name] = tracked.followLinks(top, name)
	return s.names[name], nil
}

// takesNoTagName reports whether git takes no tag name from the prefix of module,
// as it takes none from a directory with a space or that starts with -.
func (s *repositorySnapshot) takesNoTagName(module repositoryModule) (bool, error) {
	valid, err := s.validTagName(firstTagName(module.tagPrefix))
	return !valid, err
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
