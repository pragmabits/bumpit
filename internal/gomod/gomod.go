// Package gomod reads the Go modules of a repository: where each one lives,
// the module path it declares and what it requires, and what that path means
// for its version tags.
package gomod

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// Module is a Go module of a repository.
type Module struct {
	// Directory is where the go.mod is, slash separated and relative to the
	// repository top: "." for the root module.
	Directory string `json:"directory"`
	// Path is the module path the go.mod declares.
	Path string `json:"path"`
	// Requires maps each module the go.mod requires to its version.
	Requires map[string]string `json:"-"`
}

// Load reads the go.mod in directory, relative to the repository top.
func Load(top, directory string) (Module, error) {
	file := filepath.Join(top, filepath.FromSlash(directory), "go.mod")
	data, err := os.ReadFile(file)
	if err != nil {
		return Module{}, err
	}

	parsed, err := modfile.ParseLax(file, data, nil)
	if err != nil {
		return Module{}, err
	}
	if parsed.Module == nil || parsed.Module.Mod.Path == "" {
		return Module{}, fmt.Errorf("%s declares no module path", file)
	}

	requires := make(map[string]string, len(parsed.Require))
	for _, requirement := range parsed.Require {
		requires[requirement.Mod.Path] = requirement.Mod.Version
	}
	return Module{Directory: path.Clean(directory), Path: parsed.Module.Mod.Path, Requires: requires}, nil
}

// TagPrefix is what every version tag of the module starts with: its module
// subdirectory, which the Go modules reference defines as the directory part
// of the module path without the major version suffix, then "v". A module in
// sub/v2 with the path example.com/repository/sub/v2 is tagged sub/v2.0.0,
// and a root module, v2.0.0.
func (m Module) TagPrefix() string {
	subdirectory := m.Directory
	_, pathMajor, _ := module.SplitPathVersion(m.Path)
	if strings.HasPrefix(pathMajor, "/") && path.Base(subdirectory) == pathMajor[1:] {
		subdirectory = path.Dir(subdirectory)
	}
	if subdirectory == "." {
		return "v"
	}
	return subdirectory + "/v"
}

// CheckVersion reports whether version, written with its "v", can be a
// version of the module. A module path without a major version suffix takes
// v0 and v1 only, and one ending in /vN takes vN only: the go command refuses
// any other tag of a module that has a go.mod, so tagging it would publish a
// version nobody can require.
func (m Module) CheckVersion(version string) error {
	_, pathMajor, ok := module.SplitPathVersion(m.Path)
	if !ok {
		return fmt.Errorf("invalid module path %q", m.Path)
	}
	if err := module.CheckPathMajor(version, pathMajor); err != nil {
		return fmt.Errorf("module %s cannot release %s, which needs %s in its module path: %w", m.Path, version, suffixFor(version), err)
	}
	return nil
}

// Ignored reports a directory the go command leaves out: vendor, testdata,
// and any directory whose name starts with "." or "_". A go.mod in one of them
// is not a module of the repository.
func Ignored(directory string) bool {
	for _, element := range strings.Split(directory, "/") {
		if element == "vendor" || element == "testdata" || hiddenElement(element) {
			return true
		}
	}
	return false
}

func hiddenElement(element string) bool {
	return element != "." && (strings.HasPrefix(element, ".") || strings.HasPrefix(element, "_"))
}

func suffixFor(version string) string {
	major := semver.Major(version)
	if major == "v0" || major == "v1" {
		return "no major version suffix"
	}
	return "the major version suffix /" + major
}
