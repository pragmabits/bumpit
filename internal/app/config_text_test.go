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
			want:    "modules:\n  - directory: web/homepage\n    tagPrefix: homepage/v\n    paths: [web/brand]\n    shippedBy: []\n",
		},
		{
			name:    "after the last key, to a file without a final newline",
			content: "# bumpit\n\nrepository: . # here\ntagMatch: v*",
			want:    "# bumpit\n\nrepository: . # here\ntagMatch: v*\nmodules:\n  - directory: web/homepage\n    tagPrefix: homepage/v\n    paths: [web/brand]\n    shippedBy: []\n",
		},
		{
			name:    "below an empty flow list, keeping its comment",
			content: "modules: [] # none yet\noutput: text\n",
			want:    "modules: # none yet\n  - directory: web/homepage\n    tagPrefix: homepage/v\n    paths: [web/brand]\n    shippedBy: []\noutput: text\n",
		},
		{
			name:    "below a key with no value",
			content: "modules:\noutput: text\n",
			want:    "modules:\n  - directory: web/homepage\n    tagPrefix: homepage/v\n    paths: [web/brand]\n    shippedBy: []\noutput: text\n",
		},
		{
			name:    "after the last item, in its indentation and line endings",
			content: "modules:\r\n-   directory: web/admin # the admin\r\n    shippedBy: [\".\"]\r\n\r\n# after\r\noutput: text\r\n",
			want:    "modules:\r\n-   directory: web/admin # the admin\r\n    shippedBy: [\".\"]\r\n-   directory: web/homepage\r\n    tagPrefix: homepage/v\r\n    paths: [web/brand]\r\n    shippedBy: []\r\n\r\n# after\r\noutput: text\r\n",
		},
		{
			name:    "below an empty flow list after a byte order mark",
			content: "\ufeffmodules: [] # none yet\n",
			want:    "\ufeffmodules: # none yet\n  - directory: web/homepage\n    tagPrefix: homepage/v\n    paths: [web/brand]\n    shippedBy: []\n",
		},
		{
			name:    "under a document start",
			content: "---\nModules:\n  - directory: web/admin\n    shippedBy: []\n",
			want:    "---\nModules:\n  - directory: web/admin\n    shippedBy: []\n  - directory: web/homepage\n    tagPrefix: homepage/v\n    paths: [web/brand]\n    shippedBy: []\n",
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
	want := "modules:\n  - directory: web/home, page\n    paths: ['#brand', yes]\n    shippedBy: [\"true\"]\n"
	if string(got) != want {
		t.Errorf("addModuleText wrote\n%q\nwant\n%q", got, want)
	}
}

func TestAddModuleTextRefusesADeclaredDirectory(t *testing.T) {
	t.Parallel()

	_, err := addModuleText([]byte("modules:\n  - directory: web/homepage\n    shippedBy: []\n"), declare("web/homepage", "", nil))
	if err == nil || !strings.Contains(err.Error(), "web/homepage is declared already") {
		t.Fatalf("addModuleText returned %v, want web/homepage is declared already", err)
	}
}

func TestSetModuleText(t *testing.T) {
	t.Parallel()

	content := "modules:\n- directory: web/admin\n  shippedBy: []\n\n- {directory: web/homepage, paths: [web/brand], shippedBy: []}\noutput: text # plain\n"
	got, err := setModuleText([]byte(content), declare("web/homepage", "site/v", nil, "web/admin"))
	if err != nil {
		t.Fatalf("setModuleText returned error: %v", err)
	}
	want := "modules:\n- directory: web/admin\n  shippedBy: []\n\n- directory: web/homepage\n  tagPrefix: site/v\n  shippedBy: [web/admin]\noutput: text # plain\n"
	if string(got) != want {
		t.Errorf("setModuleText wrote\n%q\nwant\n%q", got, want)
	}
}

func TestSetModuleTextRefusesACommentInTheDeclaration(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		"modules:\n  - directory: web/homepage\n    # the brand\n    paths: [web/brand]\n    shippedBy: []\n",
		"modules:\n  - directory: web/homepage\n    tagPrefix: a, 'b # the prefix\n    shippedBy: []\n",
		"modules:\n  - {directory: web/homepage, shippedBy: []} # the homepage\n",
	} {
		_, err := setModuleText([]byte(content), declare("web/homepage", "", nil))
		if err == nil || !strings.Contains(err.Error(), "comment") {
			t.Errorf("setModuleText of\n%s\nreturned %v, want a refusal that names the comment", content, err)
		}
	}
}

func TestSetModuleTextRefusesAnUndeclaredDirectory(t *testing.T) {
	t.Parallel()

	_, err := setModuleText([]byte("modules: []\n"), declare("web/homepage", "", nil))
	if err == nil || !strings.Contains(err.Error(), "web/homepage is not declared") {
		t.Fatalf("setModuleText returned %v, want web/homepage is not declared", err)
	}
}

func TestRemoveModuleText(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		content   string
		directory string
		want      string
	}{
		{
			name:      "one of several, with the comments inside it",
			directory: "web/admin",
			content:   "modules:\n  - directory: web/admin\n    # shipped alone\n    shippedBy: []\n  # the homepage\n  - directory: web/homepage\n    shippedBy: []\n",
			want:      "modules:\n  # the homepage\n  - directory: web/homepage\n    shippedBy: []\n",
		},
		{
			name:      "the last one, back to an empty list with the comment of the key",
			directory: "web/homepage",
			content:   "modules: # declared by hand\n  - directory: web/homepage\n    shippedBy: []\noutput: text\n",
			want:      "modules: [] # declared by hand\noutput: text\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := removeModuleText([]byte(testCase.content), testCase.directory)
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
		{name: "a flow list with items", content: "modules: [{directory: web/admin, shippedBy: []}]\n", want: "modules is written as a list in [...]"},
		{name: "a block scalar", content: "modules:\n  - directory: >\n      web/admin\n    shippedBy: []\n", want: "block scalar"},
		{name: "an anchor", content: "modules: &declared\n  - directory: web/admin\n    shippedBy: []\n", want: "anchor"},
		{name: "an alias", content: "carriers: &carriers []\nmodules:\n  - directory: web/admin\n    shippedBy: *carriers\n", want: "alias"},
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
