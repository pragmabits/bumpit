package nodejs

import (
	"path"
	"strings"
)

// target is where a specification of a dependency points: the directories
// of the packages it names, or the path it gives, inside the repository.
type target struct {
	names []string
	// byPath reports a specification that gives a path: its package.json
	// must be read to follow its own dependencies.
	byPath bool
}

// resolve tells where a dependency that the package in from asks for points.
// A path, written bare or after workspace:, file:, link: or portal:, is read
// from the directory of the package. workspace: followed by an alias, as
// workspace:@x/a@*, and npm: followed by one point at the package of that
// name; any other specification, a version range or workspace:*, at the
// package of the name the dependency is given. A remote one, from git or a
// URL, points at nothing in the repository.
func (r Repository) resolve(dependency Dependency, from string) target {
	specification := strings.TrimSpace(dependency.Specification)
	if rest, found := cutLocalProtocol(specification); found {
		return r.byPath(from, rest)
	}
	if rest, found := strings.CutPrefix(specification, "workspace:"); found {
		return r.resolveWorkspace(rest, dependency.Name, from)
	}

	switch {
	case strings.HasPrefix(specification, "npm:"):
		return r.byName(aliasName(strings.TrimPrefix(specification, "npm:")))
	case isPath(specification):
		return r.byPath(from, specification)
	case isRemote(specification):
		return target{}
	default:
		return r.byName(dependency.Name)
	}
}

// cutLocalProtocol is the path after file:, link: or portal:.
func cutLocalProtocol(specification string) (string, bool) {
	for _, protocol := range []string{"file:", "link:", "portal:"} {
		if rest, found := strings.CutPrefix(specification, protocol); found {
			return rest, true
		}
	}
	return "", false
}

// resolveWorkspace tells where workspace:rest points, for a dependency given
// name: a path, the package an alias names, or the package of that name.
func (r Repository) resolveWorkspace(rest, name, from string) target {
	switch {
	case isPath(rest):
		return r.byPath(from, rest)
	case aliasName(rest) != "":
		return r.byName(aliasName(rest))
	default:
		return r.byName(name)
	}
}

func (r Repository) byName(name string) target {
	return target{names: r.names[name]}
}

// byPath is the tracked file or directory relative leads to from the
// directory from. A path that leaves the repository, or names nothing it
// tracks, points at nothing.
func (r Repository) byPath(from, relative string) target {
	if strings.HasPrefix(relative, "/") || strings.HasPrefix(relative, "~") {
		return target{}
	}
	joined := path.Clean(path.Join(from, strings.TrimPrefix(relative, "//")))
	if joined == ".." || strings.HasPrefix(joined, "../") || !r.tracks(joined) {
		return target{}
	}
	return target{names: []string{joined}, byPath: true}
}

// tracks reports whether the repository tracks entry, a file or a directory
// that holds tracked files.
func (r Repository) tracks(entry string) bool {
	return entry == "." || r.files[entry] || r.directories[entry]
}

// aliasName is the package name an alias gives, as @x/a in @x/a@* or a in
// a@^1.0.0, and empty for a bare range.
func aliasName(specification string) string {
	start := 0
	if strings.HasPrefix(specification, "@") {
		start = 1
	}
	at := strings.Index(specification[start:], "@")
	switch {
	case at >= 0:
		return specification[:start+at]
	case start == 1:
		return specification
	default:
		return ""
	}
}

func isPath(specification string) bool {
	for _, start := range []string{"./", "../", "/", "~/"} {
		if strings.HasPrefix(specification, start) {
			return true
		}
	}
	return specification == "." || specification == ".."
}

func isRemote(specification string) bool {
	if strings.Contains(specification, "://") {
		return true
	}
	for _, protocol := range []string{"git+", "git:", "github:", "gitlab:", "bitbucket:", "gist:"} {
		if strings.HasPrefix(specification, protocol) {
			return true
		}
	}
	return false
}
