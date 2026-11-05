package app

import (
	"strings"
	"testing"
)

func TestAddModuleText(t *testing.T) {
	t.Parallel()

	homepage := declare("web/homepage", "homepage/v", []string{"web/brand"})
	testCases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "to an empty file",
			content: "",
			want:    "modules:\n  - directories: [web/homepage]\n    tagPrefix: homepage/v\n    paths: [web/brand]\n    shippedBy: []\n",
		},
		{
			name:    "after the last key, to a file without a final newline",
			content: "# bumpit\n\nrepository: . # here\ntagMatch: v*",
			want:    "# bumpit\n\nrepository: . # here\ntagMatch: v*\nmodules:\n  - directories: [web/homepage]\n    tagPrefix: homepage/v\n    paths: [web/brand]\n    shippedBy: []\n",
		},
		{
			name:    "below an empty flow list, keeping its comment",
			content: "modules: [] # none yet\noutput: text\n",
			want:    "modules: # none yet\n  - directories: [web/homepage]\n    tagPrefix: homepage/v\n    paths: [web/brand]\n    shippedBy: []\noutput: text\n",
		},
		{
			name:    "below a key with no value",
			content: "modules:\noutput: text\n",
			want:    "modules:\n  - directories: [web/homepage]\n    tagPrefix: homepage/v\n    paths: [web/brand]\n    shippedBy: []\noutput: text\n",
		},
		{
			name:    "after the last item, in its indentation and line endings",
			content: "modules:\r\n-   directories: [web/admin] # the admin\r\n    shippedBy: [\".\"]\r\n\r\n# after\r\noutput: text\r\n",
			want:    "modules:\r\n-   directories: [web/admin] # the admin\r\n    shippedBy: [\".\"]\r\n-   directories: [web/homepage]\r\n    tagPrefix: homepage/v\r\n    paths: [web/brand]\r\n    shippedBy: []\r\n\r\n# after\r\noutput: text\r\n",
		},
		{
			name:    "below an empty flow list after a byte order mark",
			content: "\ufeffmodules: [] # none yet\n",
			want:    "\ufeffmodules: # none yet\n  - directories: [web/homepage]\n    tagPrefix: homepage/v\n    paths: [web/brand]\n    shippedBy: []\n",
		},
		{
			name:    "under a document start",
			content: "---\nModules:\n  - directories: [web/admin]\n    shippedBy: []\n",
			want:    "---\nModules:\n  - directories: [web/admin]\n    shippedBy: []\n  - directories: [web/homepage]\n    tagPrefix: homepage/v\n    paths: [web/brand]\n    shippedBy: []\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := addModuleText([]byte(testCase.content), homepage)
			if err != nil {
				t.Fatalf("addModuleText returned error: %v", err)
			}
			if string(got) != testCase.want {
				t.Errorf("addModuleText wrote\n%q\nwant\n%q", got, testCase.want)
			}
		})
	}
}

func TestAddModuleTextWritesValuesAsYAML(t *testing.T) {
	t.Parallel()

	declaration := declare("web/home, page", "", []string{"#brand", "yes"}, "true")
	got, err := addModuleText(nil, declaration)
	if err != nil {
		t.Fatalf("addModuleText returned error: %v", err)
	}
	want := "modules:\n  - directories: ['web/home, page']\n    paths: ['#brand', yes]\n    shippedBy: [\"true\"]\n"
	if string(got) != want {
		t.Errorf("addModuleText wrote\n%q\nwant\n%q", got, want)
	}
}

func TestSetModuleText(t *testing.T) {
	t.Parallel()

	content := "modules:\n- directories: [web/admin]\n  shippedBy: []\n\n- {directories: [web/homepage], paths: [web/brand], shippedBy: []}\noutput: text # plain\n"
	got, err := setModuleText([]byte(content), 1, declare("web/homepage", "site/v", nil, "web/admin"))
	if err != nil {
		t.Fatalf("setModuleText returned error: %v", err)
	}
	want := "modules:\n- directories: [web/admin]\n  shippedBy: []\n\n- directories: [web/homepage]\n  tagPrefix: site/v\n  shippedBy: [web/admin]\noutput: text # plain\n"
	if string(got) != want {
		t.Errorf("setModuleText wrote\n%q\nwant\n%q", got, want)
	}
}

func TestSetModuleTextRefusesACommentInTheDeclaration(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		"modules:\n  - directories: [web/homepage]\n    # the brand\n    paths: [web/brand]\n    shippedBy: []\n",
		"modules:\n  - directories: [web/homepage]\n    tagPrefix: a, 'b # the prefix\n    shippedBy: []\n",
		"modules:\n  - {directories: [web/homepage], shippedBy: []} # the homepage\n",
	} {
		_, err := setModuleText([]byte(content), 0, declare("web/homepage", "", nil))
		if err == nil || !strings.Contains(err.Error(), "comment") {
			t.Errorf("setModuleText of\n%s\nreturned %v, want a refusal that names the comment", content, err)
		}
	}
}

// TestSetModuleTextFindsTheItemByItsPlace covers an item no directory can
// find: one whose list is empty, or that repeats the directory of another.
func TestSetModuleTextFindsTheItemByItsPlace(t *testing.T) {
	t.Parallel()

	content := "modules:\n  - directories: [web]\n    shippedBy: []\n  - directories: [web]\n    shippedBy: []\n  - directories: []\n"
	got, err := setModuleText([]byte(content), 1, declare("docs", "", nil))
	if err != nil {
		t.Fatalf("setModuleText returned error: %v", err)
	}
	want := "modules:\n  - directories: [web]\n    shippedBy: []\n  - directories: [docs]\n    shippedBy: []\n  - directories: []\n"
	if string(got) != want {
		t.Errorf("setModuleText wrote\n%q\nwant\n%q", got, want)
	}

	if _, err := setModuleText([]byte("modules: []\n"), 0, declare("web", "", nil)); err == nil {
		t.Errorf("setModuleText of an item that does not exist returned no error")
	}
	if _, err := setModuleText([]byte("modules:\n  - web\n"), 0, declare("web", "", nil)); err == nil || !strings.Contains(err.Error(), "item 1 is no mapping") {
		t.Errorf("setModuleText of an item that is no mapping returned %v", err)
	}
}

// TestSetCarriersText covers the key an edit changes in an entry it was not
// asked to change: the rest of the entry stays, comments included.
func TestSetCarriersText(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		index   int
		want    string
	}{
		{
			name:    "a flow list, with the comment after it",
			content: "modules:\n  - directories: [web] # the site\n    shippedBy: [stack, 'a b'] # carried\n",
			want:    "modules:\n  - directories: [web] # the site\n    shippedBy: [root] # carried\n",
		},
		{
			name:    "the second item of a flow mapping",
			content: "modules:\n  - directories: [web]\n  - {directories: [docs], shippedBy: [stack], paths: [a]}\n",
			index:   1,
			want:    "modules:\n  - directories: [web]\n  - {directories: [docs], shippedBy: [root], paths: [a]}\n",
		},
		{
			name:    "a single value",
			content: "modules:\n  - directories: [web]\n    shippedBy: stack # carried\n",
			want:    "modules:\n  - directories: [web]\n    shippedBy: [root] # carried\n",
		},
		{
			name:    "a quoted value, in a file with other line endings",
			content: "modules:\r\n  - directories: [web]\r\n    shippedBy: 'stack'\r\n",
			want:    "modules:\r\n  - directories: [web]\r\n    shippedBy: [root]\r\n",
		},
	}
	for _, testCase := range testCases {
		got, err := setCarriersText([]byte(testCase.content), testCase.index, []string{"root"})
		if err != nil {
			t.Errorf("%s: setCarriersText returned error: %v", testCase.name, err)
			continue
		}
		if string(got) != testCase.want {
			t.Errorf("%s: setCarriersText wrote\n%q\nwant\n%q", testCase.name, got, testCase.want)
		}
	}

	for _, content := range []string{
		"modules:\n  - directories: [web]\n    shippedBy:\n      - stack\n",
		"modules:\n  - directories: [web]\n    shippedBy: [stack,\n      other]\n",
	} {
		_, err := setCarriersText([]byte(content), 0, []string{"root"})
		if err == nil || !strings.Contains(err.Error(), "shippedBy is written over several lines") {
			t.Errorf("setCarriersText of\n%s\nreturned %v", content, err)
		}
	}
}

func TestRemoveModuleText(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		index   int
		want    string
	}{
		{
			name:    "one of several, with the comments inside it",
			index:   0,
			content: "modules:\n  - directories: [web/admin]\n    # shipped alone\n    shippedBy: []\n  # the homepage\n  - directories: [web/homepage]\n    shippedBy: []\n",
			want:    "modules:\n  # the homepage\n  - directories: [web/homepage]\n    shippedBy: []\n",
		},
		{
			name:    "the last one, back to an empty list with the comment of the key",
			index:   0,
			content: "modules: # declared by hand\n  - directories: [web/homepage]\n    shippedBy: []\noutput: text\n",
			want:    "modules: [] # declared by hand\noutput: text\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := removeModuleText([]byte(testCase.content), testCase.index)
			if err != nil {
				t.Fatalf("removeModuleText returned error: %v", err)
			}
			if string(got) != testCase.want {
				t.Errorf("removeModuleText wrote\n%q\nwant\n%q", got, testCase.want)
			}
		})
	}
}

func TestModuleTextRefusals(t *testing.T) {
	t.Parallel()

	homepage := declare("web/homepage", "", nil)
	testCases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "several documents", content: "output: text\n---\noutput: json\n", want: "several YAML documents"},
		{name: "a root that is no mapping", content: "- output\n", want: "is no mapping"},
		{name: "a flow root", content: "{output: text}\n", want: "flow mapping"},
		{name: "a flow list with items", content: "modules: [{directories: [web/admin], shippedBy: []}]\n", want: "modules is written as a list in [...]"},
		{name: "a block scalar", content: "modules:\n  - directories: >\n      web/admin\n    shippedBy: []\n", want: "block scalar"},
		{name: "an anchor", content: "modules: &declared\n  - directories: [web/admin]\n    shippedBy: []\n", want: "anchor"},
		{name: "an alias", content: "carriers: &carriers []\nmodules:\n  - directories: [web/admin]\n    shippedBy: *carriers\n", want: "alias"},
		{name: "an empty list on a line of its own", content: "modules:\n            []\n", want: "on a line of its own"},
		{name: "a null on a line of its own", content: "modules:\n  ~\n", want: "on a line of its own"},
		{name: "a key written twice", content: "modules: []\nMODULES: []\n", want: "modules is written twice"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := addModuleText([]byte(testCase.content), homepage)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("addModuleText returned %v, want an error with %q", err, testCase.want)
			}
		})
	}
}

func TestSetConfigKeyText(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		key     string
		value   string
		want    string
	}{
		{name: "a plain value, keeping its comment", content: "tagMatch: v* # ours\noutput: text\n", key: "tagMatch", value: "release/v*", want: "tagMatch: release/v* # ours\noutput: text\n"},
		{name: "a quoted value", content: "tagMessage: \"Release # one\"\n", key: "tagMessage", value: "'Release two'", want: "tagMessage: 'Release two'\n"},
		{name: "a key compared without case", content: "TagMatch: v*\n", key: "tagMatch", value: "w*", want: "TagMatch: w*\n"},
		{name: "a key with no value", content: "tagMatch:\noutput: text\n", key: "tagMatch", value: "w*", want: "tagMatch: w*\noutput: text\n"},
		{name: "a plain value holding a quote, keeping its comment", content: "tagMatch: a-'b # keep\n", key: "tagMatch", value: "x", want: "tagMatch: x # keep\n"},
		{name: "after a byte order mark", content: "\ufefftagMatch: v* # ü\n", key: "tagMatch", value: "w*", want: "\ufefftagMatch: w* # ü\n"},
		{name: "after text of several bytes", content: "tagMessage: \"é\" # ü\n", key: "tagMessage", value: "x", want: "tagMessage: x # ü\n"},
		{name: "an absent key", content: "output: text\n# end\n", key: "promote", value: "true", want: "output: text\n# end\npromote: true\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := setConfigKeyText([]byte(testCase.content), testCase.key, testCase.value)
			if err != nil {
				t.Fatalf("setConfigKeyText returned error: %v", err)
			}
			if string(got) != testCase.want {
				t.Errorf("setConfigKeyText wrote\n%q\nwant\n%q", got, testCase.want)
			}
		})
	}
}

func TestSetConfigKeyTextRefusesAValueOverSeveralLines(t *testing.T) {
	t.Parallel()

	_, err := setConfigKeyText([]byte("tagMessage:\n  Release\n  notes\n"), "tagMessage", "x")
	if err == nil || !strings.Contains(err.Error(), "several lines") {
		t.Fatalf("setConfigKeyText returned %v, want a refusal of several lines", err)
	}
}

func TestUnsetConfigKeyText(t *testing.T) {
	t.Parallel()

	got, err := unsetConfigKeyText([]byte("tagMatch: v* # ours\noutput: text\n"), "tagMatch")
	if err != nil {
		t.Fatalf("unsetConfigKeyText returned error: %v", err)
	}
	if string(got) != "output: text\n" {
		t.Errorf("unsetConfigKeyText wrote %q", got)
	}

	if _, err := unsetConfigKeyText([]byte("output: text\n"), "tagMatch"); err != errConfigKeyNotSet {
		t.Errorf("unsetConfigKeyText of an absent key returned %v, want errConfigKeyNotSet", err)
	}
}

func TestConfigKeyValue(t *testing.T) {
	t.Parallel()

	content := []byte("tagMessage: \"Release # one\" # ours\ntagMatch:\n")
	value, found, err := configKeyValue(content, "tagmessage")
	if err != nil || !found || value != "Release # one" {
		t.Errorf("configKeyValue of tagMessage = %q, %v, %v", value, found, err)
	}
	if value, found, err := configKeyValue(content, "tagMatch"); err != nil || !found || value != "" {
		t.Errorf("configKeyValue of a key with no value = %q, %v, %v, want found with no value, as unset finds it", value, found, err)
	}
	if _, found, err := configKeyValue(content, "output"); err != nil || found {
		t.Errorf("configKeyValue of an absent key = %v, %v, want not found", found, err)
	}
}
