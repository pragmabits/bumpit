// Package helm reads the Helm charts of a repository: where each one lies and
// the version its Chart.yaml gives it.
package helm

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Chart is a chart of the repository that is released on its own.
type Chart struct {
	// Directory holds the Chart.yaml, slash separated and relative to the
	// top of the repository: "." for the top.
	Directory string
	// Version is the version the Chart.yaml gives the chart.
	Version string
}

// Unreadable is a Chart.yaml that could not be read, and why.
type Unreadable struct {
	Directory string
	Err       error
}

// Read reads the Chart.yaml files among tracked, the tracked files of the
// repository at top, slash separated and relative to it, and returns the
// charts that give a version and are no subchart: a subchart, in the charts
// directory of another chart, is released with that one. A tracked
// Chart.yaml deleted from the working tree is no chart. One that cannot be
// read is set aside with why, so that it stops nothing that does not need
// it.
func Read(top string, tracked []string) ([]Chart, []Unreadable) {
	present := map[string]bool{}
	for _, file := range tracked {
		present[file] = true
	}

	var charts []Chart
	var unreadable []Unreadable
	for _, file := range tracked {
		if path.Base(file) != "Chart.yaml" || isSubchart(path.Dir(file), present) {
			continue
		}
		version, err := readVersion(filepath.Join(top, filepath.FromSlash(file)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			unreadable = append(unreadable, Unreadable{Directory: path.Dir(file), Err: err})
			continue
		}
		if version != "" {
			charts = append(charts, Chart{Directory: path.Dir(file), Version: version})
		}
	}
	slices.SortFunc(charts, func(left, right Chart) int { return strings.Compare(left.Directory, right.Directory) })
	return charts, unreadable
}

// isSubchart reports a chart in the charts directory of another chart.
func isSubchart(directory string, present map[string]bool) bool {
	parent := path.Dir(directory)
	return path.Base(parent) == "charts" && present[path.Join(path.Dir(parent), "Chart.yaml")]
}

func readVersion(file string) (string, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	var chart struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal(content, &chart); err != nil {
		return "", err
	}
	return chart.Version, nil
}
