package app

import (
	"reflect"
	"strings"
	"testing"
)

func TestVersionPrefix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		tag    string
		prefix string
		found  bool
	}{
		{tag: "v1.2.3", prefix: "v", found: true},
		{tag: "homepage/v0.4.0", prefix: "homepage/v", found: true},
		{tag: "release-10.0.0", prefix: "release-", found: true},
		{tag: "app-v2.0.0-rc.1", prefix: "app-v", found: true},
		{tag: "1.2.3", prefix: "", found: true},
		{tag: "x10.0.0", prefix: "x", found: true},
		{tag: "v1.2"},
		{tag: "build-2024.01.01"},
		{tag: "notes"},
	}

	for _, testCase := range testCases {
		prefix, found := versionPrefix(testCase.tag)
		if prefix != testCase.prefix || found != testCase.found {
			t.Errorf("versionPrefix(%q) = %q, %v, want %q, %v", testCase.tag, prefix, found, testCase.prefix, testCase.found)
		}
	}
}

func TestUnownedTagLines(t *testing.T) {
	t.Parallel()

	tags := []string{"v1.0.0", "homepage/v0.1.0", "homepage/v0.4.0", "homepage/v0.2.0-rc.1", "cmd/tool/v1.0.0", "release-10.0.0", "notes", "build-2024.01.01", "vite/v0.1.0"}
	got := unownedTagLines(tags, []string{"v", "cmd/tool/v"})
	want := []TagLine{
		{Prefix: "homepage/v", Count: 3, Newest: "homepage/v0.4.0"},
		{Prefix: "release-", Count: 1, Newest: "release-10.0.0"},
		{Prefix: "vite/v", Count: 1, Newest: "vite/v0.1.0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unownedTagLines = %+v\nwant %+v", got, want)
	}
}

func TestGuessDirectories(t *testing.T) {
	t.Parallel()

	tracked := newTrackedTree([]string{
		"go.mod",
		"web/homepage/index.html",
		"deploy/homepage/values.yaml",
		"homepage/README.md",
		"tools/homepage/go.mod",
		"web/testdata/homepage/page.html",
		"web/_drafts/homepage/page.html",
		"web/myhomepage/index.html",
		"site/web/homepage/index.html",
		"kv/main.yaml",
		"k/main.yaml",
		"v/main.yaml",
		".github/actions/setup/action.yml",
		"web/vendor/homepage/page.html",
		"web/node_modules/homepage/package.json",
	})
	modules := []repositoryModule{{directories: []string{"."}}, {directories: []string{"tools/homepage"}}}

	testCases := []struct {
		prefix string
		want   []string
	}{
		{prefix: "homepage/v", want: []string{"deploy/homepage", "homepage", "site/web/homepage", "web/_drafts/homepage", "web/homepage"}},
		{prefix: "kv", want: []string{"kv"}},
		{prefix: "v-", want: nil},
		{prefix: "setup/v", want: []string{".github/actions/setup"}},
		{prefix: "web/homepage-", want: []string{"site/web/homepage", "web/homepage"}},
		{prefix: "v", want: nil},
		{prefix: "", want: nil},
	}
	for _, testCase := range testCases {
		got := guessDirectories(testCase.prefix, tracked, modules)
		if !reflect.DeepEqual(got, testCase.want) {
			t.Errorf("guessDirectories(%q) = %v, want %v", testCase.prefix, got, testCase.want)
		}
	}
}

func TestWriteInitNamesAnEmptyPrefix(t *testing.T) {
	t.Parallel()

	var text strings.Builder
	report := InitReport{Config: "bumpit.yaml", TagLines: []TagLine{{Prefix: "", Count: 2, Newest: "1.1.0"}}}
	if err := writeInit(&text, report); err != nil {
		t.Fatalf("writeInit returned error: %v", err)
	}
	if !strings.Contains(text.String(), "(no prefix): 2 tags, the newest 1.1.0") || strings.Contains(text.String(), "--tag-prefix") {
		t.Errorf("writeInit wrote\n%s\nwant the line named (no prefix), without advice it cannot follow", text.String())
	}
}
