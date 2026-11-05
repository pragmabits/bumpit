package app

import (
	"slices"
	"testing"

	"github.com/pragmabits/bumpit/internal/gomod"
)

// TestPathspecsOfGoModules pins the pathspecs git receives for each Go module
// of a repository with no module declared in bumpit.yaml.
func TestPathspecsOfGoModules(t *testing.T) {
	t.Parallel()

	modules := []gomod.Module{
		{Directory: ".", Path: "example.com/repository"},
		{Directory: "tool", Path: "example.com/repository/tool"},
		{Directory: "tool/plugin", Path: "example.com/repository/tool/plugin"},
		{Directory: "toolbox", Path: "example.com/repository/toolbox"},
	}
	cases := []struct {
		directory string
		want      []string
	}{
		{directory: ".", want: []string{":/", ":(top,exclude)tool", ":(top,exclude)tool/plugin", ":(top,exclude)toolbox"}},
		{directory: "tool", want: []string{":(top)tool", ":(top,exclude)tool/plugin"}},
		{directory: "tool/plugin", want: []string{":(top)tool/plugin"}},
		{directory: "toolbox", want: []string{":(top)toolbox"}},
	}
	repositoryModules := make([]repositoryModule, 0, len(modules))
	for _, module := range modules {
		repositoryModules = append(repositoryModules, goRepositoryModule(module))
	}
	for _, testCase := range cases {
		index := slices.IndexFunc(repositoryModules, func(module repositoryModule) bool { return module.name == testCase.directory })
		scope := &moduleScope{module: repositoryModules[index], modules: repositoryModules}
		queries := scope.queries()
		if len(queries) != 1 || !slices.Equal(queries[0].pathspecs, testCase.want) {
			t.Errorf("queries of %s = %+v, want one with %q", testCase.directory, queries, testCase.want)
		}
	}
}
