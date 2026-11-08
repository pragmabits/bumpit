package app

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pragmabits/bumpit/internal/gitx"
)

// trackedTree is the files a repository tracks, listed once by a discovery
// for every check that needs them: the paths of the declarations, the Go
// code that may ship a declared module, and the Node.js packages.
type trackedTree struct {
	// files are slash separated and relative to the top of the repository.
	files       []string
	present     map[string]bool
	directories map[string]bool
}

func listTracked(client gitx.Client) (trackedTree, error) {
	files, err := client.TrackedFiles(":/")
	if err != nil {
		return trackedTree{}, err
	}
	return newTrackedTree(files), nil
}

// newTrackedTree is the tree of files, slash separated and relative to the
// top of the repository.
func newTrackedTree(files []string) trackedTree {
	tree := trackedTree{files: files, present: map[string]bool{}, directories: map[string]bool{}}
	for _, file := range files {
		tree.present[file] = true
		for directory := path.Dir(file); directory != "."; directory = path.Dir(directory) {
			tree.directories[directory] = true
		}
	}
	return tree
}

// maxLinks is how many links a name may lead through, as many as Linux follows,
// before it is taken for a loop of links.
const maxLinks = 40

// followLinks is the directory, relative to the top at top, that name means read
// as a path from there in every clone of the repository: the links the repository
// tracks are followed, as the command line follows them, and those only this
// working tree holds are not. A name that leads out of the repository, through a
// link to an absolute path or a loop of links means itself, since no directory of
// a module lies there.
func (t trackedTree) followLinks(top, name string) string {
	resolved := "."
	pending := strings.Split(name, "/")
	for links := 0; len(pending) > 0; {
		next := path.Join(resolved, pending[0])
		pending = pending[1:]
		target, err := os.Readlink(filepath.Join(top, filepath.FromSlash(next)))
		if err != nil || !t.present[next] {
			resolved = next
			continue
		}
		links++
		if links > maxLinks || filepath.IsAbs(target) {
			return name
		}
		joined := path.Join(path.Dir(next), filepath.ToSlash(target))
		if joined == ".." || strings.HasPrefix(joined, "../") {
			return name
		}
		resolved, pending = ".", append(strings.Split(joined, "/"), pending...)
	}
	return resolved
}

// holds reports whether the repository tracks entry: a file, a directory
// that holds tracked files, or the top itself when it tracks any file.
func (t trackedTree) holds(entry string) bool {
	if entry == "." {
		return len(t.files) > 0
	}
	return t.present[entry] || t.directories[entry]
}
