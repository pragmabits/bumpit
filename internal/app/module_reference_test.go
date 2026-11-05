package app

import (
	"strings"
	"testing"
)

func TestModuleReferenceFind(t *testing.T) {
	t.Parallel()

	identities := []moduleIdentity{
		{directories: []string{"."}},
		{name: "stack", directories: []string{"cmd/stack"}},
		{name: "homepage", directories: []string{"web/homepage", "deploy/homepage"}},
		{directories: []string{"docs"}},
		{directories: []string{"web/docs"}},
		{directories: []string{"web/stack"}},
	}
	testCases := []struct {
		name      string
		reference moduleReference
		want      int
	}{
		{name: "an explicit name", reference: moduleReference{written: "homepage", directory: "homepage"}, want: 2},
		{name: "the first directory", reference: moduleReference{written: "web/homepage", directory: "web/homepage"}, want: 2},
		{name: "another directory", reference: moduleReference{written: "deploy/homepage", directory: "deploy/homepage"}, want: 2},
		{name: "a directory of a named module", reference: moduleReference{written: "cmd/stack", directory: "cmd/stack"}, want: 1},
		{name: "a directory is relative to the repository", reference: moduleReference{written: "docs", directory: "web/docs"}, want: 4},
		{name: "a directory that is no name of another", reference: moduleReference{written: "..", directory: "."}, want: 0},
		{name: "the path form of a directory a name also fits", reference: moduleReference{written: "./stack", directory: "web/stack"}, want: 5},
		{name: "no module", reference: moduleReference{written: "missing", directory: "missing"}, want: -1},
		{name: "the root is no name", reference: moduleReference{written: ".", directory: "web"}, want: -1},
	}
	for _, testCase := range testCases {
		got, err := testCase.reference.find(identities)
		if err != nil || got != testCase.want {
			t.Errorf("%s: find = %d, %v, want %d", testCase.name, got, err, testCase.want)
		}
	}
}

// TestModuleReferenceFindRefusesTwoModules covers a reference that is the
// name of one module and, relative to the repository, the directory of
// another: reading either would pick a module nobody may have meant.
func TestModuleReferenceFindRefusesTwoModules(t *testing.T) {
	t.Parallel()

	identities := []moduleIdentity{
		{name: "stack", directories: []string{"cmd/stack"}},
		{directories: []string{"web/stack"}},
	}
	_, err := moduleReference{written: "stack", directory: "web/stack"}.find(identities)
	if err == nil || !strings.Contains(err.Error(), "write ./stack") {
		t.Errorf("find returned %v, want a refusal that says to write ./stack", err)
	}

	if index, err := (moduleReference{written: "stack", directory: "cmd/stack"}).find(identities); err != nil || index != 0 {
		t.Errorf("a name and a directory of the same module: find = %d, %v", index, err)
	}
}
