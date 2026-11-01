package app

import (
	"path"

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

	tree := trackedTree{files: files, present: map[string]bool{}, directories: map[string]bool{}}
	for _, file := range files {
		tree.present[file] = true
		for directory := path.Dir(file); directory != "."; directory = path.Dir(directory) {
			tree.directories[directory] = true
		}
	}
	return tree, nil
}

// holds reports whether the repository tracks entry: a file, a directory
// that holds tracked files, or the top itself when it tracks any file.
func (t trackedTree) holds(entry string) bool {
	if entry == "." {
		return len(t.files) > 0
	}
	return t.present[entry] || t.directories[entry]
}
