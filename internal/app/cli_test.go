package app

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCommandFlagShorthands(t *testing.T) {
	t.Parallel()

	releaseShorthands := map[string]string{
		"config":        "c",
		"repo":          "r",
		"match":         "t",
		"start-version": "s",
		"allow-dirty":   "d",
		"pre":           "p",
		"promote":       "P",
		"release-as":    "V",
		"module":        "g",
	}
	editShorthands := map[string]string{"config": "c", "repo": "r"}
	declarationShorthands := withShorthands(editShorthands, map[string]string{"name": "e", "tag-prefix": "T", "path": "f", "shipped-by": "b"})

	expected := map[string]map[string]string{
		"bumpit latest":         {"config": "c", "repo": "r", "match": "t", "output": "o", "all": "a", "no-prefix": "n", "module": "g"},
		"bumpit next":           withShorthands(releaseShorthands, map[string]string{"output": "o"}),
		"bumpit explain":        withShorthands(releaseShorthands, map[string]string{"output": "o"}),
		"bumpit tag":            withShorthands(releaseShorthands, map[string]string{"message": "m", "all-modules": "M"}),
		"bumpit modules":        {"config": "c", "repo": "r", "output": "o", "allow-dirty": "d"},
		"bumpit modules add":    declarationShorthands,
		"bumpit modules set":    withShorthands(declarationShorthands, map[string]string{"directory": "D", "no-paths": "N", "shipped-by-none": "B"}),
		"bumpit modules remove": editShorthands,
		"bumpit modules check":  {"config": "c", "repo": "r", "output": "o"},
		"bumpit config":         {},
		"bumpit config get":     editShorthands,
		"bumpit config set":     editShorthands,
		"bumpit config unset":   editShorthands,
		"bumpit init":           {"repo": "r", "output": "o"},
		"bumpit version":        {},
	}

	commands := map[string]*cobra.Command{}
	for _, command := range allCommands(newRootCmd()) {
		commands[command.CommandPath()] = command
	}
	for path, expectedShorthands := range expected {
		command, found := commands[path]
		if !found {
			t.Errorf("%s: the command does not exist", path)
			continue
		}
		checkShorthands(t, command, expectedShorthands)
	}
	for path := range commands {
		if _, documented := expected[path]; !documented && path != "bumpit" {
			t.Errorf("%s: the command is not covered by the expected shorthand table", path)
		}
	}
}

// checkShorthands checks that every flag of command but help has the
// shorthand expected gives it, and that every flag expected names exists.
func checkShorthands(t *testing.T, command *cobra.Command, expected map[string]string) {
	t.Helper()

	path := command.CommandPath()
	shorthandOwners := map[string]string{}
	command.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Name == "help" {
			return
		}
		if flag.Shorthand == "" {
			t.Errorf("%s: --%s has no shorthand", path, flag.Name)
			return
		}
		if owner, taken := shorthandOwners[flag.Shorthand]; taken {
			t.Errorf("%s: -%s is shared by --%s and --%s", path, flag.Shorthand, owner, flag.Name)
		}
		shorthandOwners[flag.Shorthand] = flag.Name

		wanted, listed := expected[flag.Name]
		if !listed {
			t.Errorf("%s: --%s is not covered by the expected shorthand table", path, flag.Name)
			return
		}
		if flag.Shorthand != wanted {
			t.Errorf("%s: --%s shorthand = %q, want %q", path, flag.Name, flag.Shorthand, wanted)
		}
	})

	for flagName := range expected {
		if command.Flags().Lookup(flagName) == nil {
			t.Errorf("%s: expected flag --%s to exist", path, flagName)
		}
	}
}

// allCommands is command and every command below it, help left out.
func allCommands(command *cobra.Command) []*cobra.Command {
	commands := []*cobra.Command{command}
	for _, child := range command.Commands() {
		if child.Name() != "help" {
			commands = append(commands, allCommands(child)...)
		}
	}
	return commands
}

func TestShorthandsAreConsistentAcrossCommands(t *testing.T) {
	t.Parallel()

	owners := map[string]string{}
	for _, command := range allCommands(newRootCmd()) {
		command.Flags().VisitAll(func(flag *pflag.Flag) {
			if flag.Name == "help" || flag.Shorthand == "" {
				return
			}
			if owner, taken := owners[flag.Shorthand]; taken && owner != flag.Name {
				t.Errorf("-%s means --%s in one command and --%s in %s", flag.Shorthand, owner, flag.Name, command.CommandPath())
				return
			}
			owners[flag.Shorthand] = flag.Name
		})
	}
}

func withShorthands(base map[string]string, extra map[string]string) map[string]string {
	merged := map[string]string{}
	for name, shorthand := range base {
		merged[name] = shorthand
	}
	for name, shorthand := range extra {
		merged[name] = shorthand
	}
	return merged
}

func TestApplyConfigUsesConfigValuesWhenFlagsAreUnset(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	repositoryPath := filepath.Join(directory, "repository")
	if err := os.Mkdir(repositoryPath, 0o755); err != nil {
		t.Fatalf("Mkdir returned error: %v", err)
	}
	configPath := filepath.Join(directory, "bumpit.yaml")
	content := []byte("repository: " + repositoryPath + "\ntagMatch: release-*\nstartVersion: 2.0.0\nallowDirty: true\noutput: json\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	flags := releasePlanFlags{}
	command := &cobra.Command{Use: "next"}
	flags.bind(command, true)
	if err := command.Flags().Set("config", configPath); err != nil {
		t.Fatalf("Set(config) returned error: %v", err)
	}

	config, err := flags.applyConfig(command)
	if err != nil {
		t.Fatalf("applyConfig returned error: %v", err)
	}
	if config == nil {
		t.Fatalf("expected config to be loaded")
	}

	if flags.repository != repositoryPath {
		t.Fatalf("unexpected repository: %s", flags.repository)
	}
	if flags.tagMatch != "release-*" {
		t.Fatalf("unexpected match: %s", flags.tagMatch)
	}
	if flags.startVersion != "2.0.0" {
		t.Fatalf("unexpected start version: %s", flags.startVersion)
	}
	if !flags.allowDirty {
		t.Fatalf("expected allowDirty to be true")
	}
	if flags.output != "json" {
		t.Fatalf("unexpected output: %s", flags.output)
	}
}

func TestApplyConfigPreservesExplicitFlags(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	configPath := filepath.Join(directory, "bumpit.yaml")
	content := []byte("tagMatch: release-*\nallowDirty: true\noutput: json\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	flags := releasePlanFlags{}
	command := &cobra.Command{Use: "next"}
	flags.bind(command, true)
	if err := command.Flags().Set("config", configPath); err != nil {
		t.Fatalf("Set(config) returned error: %v", err)
	}

	if err := command.Flags().Set("match", "custom-*"); err != nil {
		t.Fatalf("Set(match) returned error: %v", err)
	}
	if err := command.Flags().Set("allow-dirty", "false"); err != nil {
		t.Fatalf("Set(allow-dirty) returned error: %v", err)
	}
	if err := command.Flags().Set("output", "text"); err != nil {
		t.Fatalf("Set(output) returned error: %v", err)
	}

	config, err := flags.applyConfig(command)
	if err != nil {
		t.Fatalf("applyConfig returned error: %v", err)
	}
	if config == nil {
		t.Fatalf("expected config to be loaded")
	}

	if flags.tagMatch != "custom-*" {
		t.Fatalf("match was overwritten: %s", flags.tagMatch)
	}
	if flags.allowDirty {
		t.Fatalf("allowDirty was overwritten")
	}
	if flags.output != "text" {
		t.Fatalf("output was overwritten: %s", flags.output)
	}
}

func TestApplyConfigReturnsTagSection(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	configPath := filepath.Join(directory, "bumpit.yaml")
	content := []byte("tagMessage: Release from config\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	flags := tagCommandFlags{}
	command := &cobra.Command{Use: "tag"}
	flags.bind(command)
	if err := command.Flags().Set("config", configPath); err != nil {
		t.Fatalf("Set(config) returned error: %v", err)
	}

	config, err := flags.applyConfig(command)
	if err != nil {
		t.Fatalf("applyConfig returned error: %v", err)
	}
	if config == nil {
		t.Fatalf("expected config to be loaded")
	}
	if config.TagMessage != "Release from config" {
		t.Fatalf("unexpected tag message: %s", config.TagMessage)
	}
}

// TestApplyConfigPassesModuleDeclarations covers every command that plans a
// release or reads tags: the modules bumpit.yaml declares reach its options.
func TestApplyConfigPassesModuleDeclarations(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "bumpit.yaml")
	if err := os.WriteFile(configPath, []byte("modules:\n  - directories: [web]\n    shippedBy: []\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	want := []ModuleDeclaration{declare("web", "", nil)}

	releaseFlags, tagFlags, modulesFlags, latestFlags := releasePlanFlags{}, tagCommandFlags{}, modulesCommandFlags{}, latestCommandFlags{}
	commands := []struct {
		name     string
		bind     func(*cobra.Command)
		apply    func(*cobra.Command) error
		declared func() []ModuleDeclaration
	}{
		{name: "next", bind: func(command *cobra.Command) { releaseFlags.bind(command, true) }, apply: func(command *cobra.Command) error {
			_, err := releaseFlags.applyConfig(command)
			return err
		}, declared: func() []ModuleDeclaration { return releaseFlags.options().Modules }},
		{name: "tag", bind: tagFlags.bind, apply: func(command *cobra.Command) error {
			_, err := tagFlags.applyConfig(command)
			return err
		}, declared: func() []ModuleDeclaration { return tagFlags.options().Modules }},
		{name: "modules", bind: modulesFlags.bind, apply: modulesFlags.applyConfig, declared: func() []ModuleDeclaration { return modulesFlags.options().Modules }},
		{name: "latest", bind: latestFlags.bind, apply: func(command *cobra.Command) error {
			_, err := latestFlags.applyConfig(command)
			return err
		}, declared: func() []ModuleDeclaration { return latestFlags.options().Modules }},
	}
	for _, testCase := range commands {
		command := &cobra.Command{Use: testCase.name}
		testCase.bind(command)
		if err := command.Flags().Set("config", configPath); err != nil {
			t.Fatalf("%s: Set(config) returned error: %v", testCase.name, err)
		}
		if err := testCase.apply(command); err != nil {
			t.Fatalf("%s: applyConfig returned error: %v", testCase.name, err)
		}
		if got := testCase.declared(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: options carry %#v, want %#v", testCase.name, got, want)
		}
	}
}

// TestApplyConfigReadsDeclarationsAtTheTop covers where a run takes the
// modules from: the bumpit.yaml at the top of the repository, wherever --repo
// points, unless the config given with --config declares them.
func TestApplyConfigReadsDeclarationsAtTheTop(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitNewFile(t, repositoryPath, "web/homepage/index.html", "homepage", "feat: add the homepage")
	commitNewFile(t, repositoryPath, "docs/guide.md", "guide", "docs: write the guide")
	writeConfig(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: [web/homepage]\n    shippedBy: []\n")
	elsewhere := t.TempDir()
	writeConfig(t, filepath.Join(elsewhere, "docs.yaml"), "repository: "+repositoryPath+"\nmodules:\n  - directories: [docs]\n    shippedBy: []\n")
	writeConfig(t, filepath.Join(elsewhere, "none.yaml"), "repository: "+repositoryPath+"\nmodules: []\n")
	writeConfig(t, filepath.Join(elsewhere, "silent.yaml"), "repository: "+repositoryPath+"\ntagMessage: Release\n")
	web := filepath.Join(repositoryPath, "web")

	runs := []struct {
		name      string
		arguments map[string]string
		want      []string
	}{
		{name: "from the top", arguments: map[string]string{"repo": repositoryPath}, want: []string{"web/homepage"}},
		{name: "from a subdirectory", arguments: map[string]string{"repo": web}, want: []string{"web/homepage"}},
		{name: "a config that declares modules", arguments: map[string]string{"config": filepath.Join(elsewhere, "docs.yaml")}, want: []string{"docs"}},
		{name: "a config that declares none", arguments: map[string]string{"config": filepath.Join(elsewhere, "none.yaml")}},
		{name: "a config silent on modules", arguments: map[string]string{"config": filepath.Join(elsewhere, "silent.yaml")}, want: []string{"web/homepage"}},
	}
	for _, run := range runs {
		flags, err := applyReleaseConfig(run.arguments)
		if err != nil {
			t.Fatalf("%s: applyConfig returned error: %v", run.name, err)
		}
		var got []string
		for _, declaration := range flags.declarations {
			got = append(got, declaration.Directories...)
		}
		if !slices.Equal(got, run.want) {
			t.Errorf("%s: declarations %q, want %q", run.name, got, run.want)
		}
	}

	writeConfig(t, filepath.Join(web, "bumpit.yaml"), "modules: []\n")
	if flags, err := applyReleaseConfig(map[string]string{"repo": web}); err != nil || len(flags.declarations) != 1 {
		t.Errorf("a config of a subdirectory with an empty modules: error = %v, declarations %v", err, flags.declarations)
	}

	writeConfig(t, filepath.Join(web, "bumpit.yaml"), "modules:\n  - directories: [web/homepage]\n    shippedBy: []\n")
	_, err := applyReleaseConfig(map[string]string{"repo": web})
	want := filepath.Join(web, "bumpit.yaml") + " declares modules, which bumpit reads from the bumpit.yaml at the top of the repository: move them to " + filepath.Join(repositoryPath, "bumpit.yaml")
	if err == nil || err.Error() != want {
		t.Errorf("a config of a subdirectory that declares modules: error = %v, want %q", err, want)
	}

	if err := os.Remove(filepath.Join(web, "bumpit.yaml")); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	writeConfig(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules: [\n")
	if _, err := applyReleaseConfig(map[string]string{"repo": web}); err == nil || !strings.Contains(err.Error(), "read the modules of "+filepath.Join(repositoryPath, "bumpit.yaml")) {
		t.Errorf("a top config that cannot be read: error = %v", err)
	}
}

func applyReleaseConfig(arguments map[string]string) (releasePlanFlags, error) {
	flags := releasePlanFlags{}
	command := &cobra.Command{Use: "next"}
	flags.bind(command, true)
	for name, value := range arguments {
		if err := command.Flags().Set(name, value); err != nil {
			return flags, err
		}
	}
	_, err := flags.applyConfig(command)
	return flags, err
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}

// TestApplyConfigReadsOnlyTheModulesOfTheTop covers the config at the top
// that a run from a subdirectory reads only for its modules: its other keys,
// valid or not, are not the run's, and a directory that is no repository
// reports that it is none rather than where modules belong.
func TestApplyConfigReadsOnlyTheModulesOfTheTop(t *testing.T) {
	t.Parallel()

	repositoryPath := initRepository(t)
	commitNewFile(t, repositoryPath, "sub/notes.txt", "notes", "docs: write notes")
	sub := filepath.Join(repositoryPath, "sub")
	writeConfig(t, filepath.Join(sub, "bumpit.yaml"), "tagMessage: Release\n")

	writeConfig(t, filepath.Join(repositoryPath, "bumpit.yaml"), "repository: ./missing\noutput: xml\n")
	if flags, err := applyReleaseConfig(map[string]string{"repo": sub}); err != nil || flags.declarations != nil {
		t.Errorf("a top config with invalid keys and no modules: error = %v, declarations %v", err, flags.declarations)
	}

	writeConfig(t, filepath.Join(repositoryPath, "bumpit.yaml"), "output: xml\nmodules:\n  - directories: [sub]\n    shippedBy: []\n")
	if flags, err := applyReleaseConfig(map[string]string{"repo": sub}); err != nil || len(flags.declarations) != 1 {
		t.Errorf("a top config with an invalid key and modules: error = %v, declarations %v", err, flags.declarations)
	}

	writeConfig(t, filepath.Join(repositoryPath, "bumpit.yaml"), "modules:\n  - directories: []\n")
	if _, err := applyReleaseConfig(map[string]string{"repo": sub}); err == nil || !strings.Contains(err.Error(), "directories cannot be empty") {
		t.Errorf("a top config with an invalid declaration: error = %v", err)
	}

	outside := t.TempDir()
	writeConfig(t, filepath.Join(outside, "bumpit.yaml"), "modules:\n  - directories: [web]\n    shippedBy: []\n")
	if _, err := applyReleaseConfig(map[string]string{"repo": outside}); err != nil {
		t.Errorf("a directory that is no repository: applyConfig error = %v, want none, so that the command reports it", err)
	}
}
