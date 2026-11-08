package app

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// unseenShipping is what a declaration no module carries rests on, which
// bumpit modules add and bumpit init both say.
const unseenShipping = "bumpit sees only //go:embed: check that no Dockerfile, CI job or other build copies it into the release of another module"

func newModulesAddCmd() *cobra.Command {
	flags := moduleEditFlags{}
	command := &cobra.Command{
		Use:   "add <directory>...",
		Short: "Declare a module in bumpit.yaml",
		Long:  "bumpit modules add declares a module that owns the directories, relative to --repo, in the config file whose modules the runs read: the one --config names when it declares modules, else the bumpit.yaml at the top of the repository, created when the repository has none. Several directories need --name. A directory that holds a go.mod takes an entry too, which only names the Go module. --path and --shipped-by are relative to --repo too, and every path is written relative to the top of the repository. The rest of the file is left as it is, and the file is written only once the new config loads and the repository accepts its modules. With no --shipped-by, the declaration of a module with no go.mod says that no module's release carries it, and the command says what that rests on.",
		RunE: func(command *cobra.Command, arguments []string) error {
			return flags.edit(command, "added %s to %s", func(config *Config) (textEdit, error) {
				return flags.add(command, arguments, config)
			})
		},
	}
	command.Args = cobra.MinimumNArgs(1)
	flags.bind(command, false)
	return command
}

func newModulesSetCmd() *cobra.Command {
	flags := moduleEditFlags{}
	command := &cobra.Command{
		Use:   "set <module>",
		Short: "Change the declaration of a module in bumpit.yaml",
		Long:  "bumpit modules set changes the declaration of the module, given by its name or a directory relative to --repo: the flags given replace what the declaration says, and the others leave it. A change of name or directories that would change the tag prefix writes the old one down, so that the module keeps its tags, and the modules that call it in shippedBy follow it, unless another module answers to that name too, which the command then says. A move out of . needs --tag-prefix, since v is the prefix of the module in . alone. The declaration is written anew, so one that holds a comment is refused. The file is chosen and proved as bumpit modules add does.",
		RunE: func(command *cobra.Command, arguments []string) error {
			changes := []string{"name", "directory", "tag-prefix", "path", "no-paths", "shipped-by", "shipped-by-none"}
			if !slices.ContainsFunc(changes, func(name string) bool { return flagGiven(command, name) }) {
				return fmt.Errorf("nothing to change: give --name, --directory, --tag-prefix, --path, --no-paths, --shipped-by or --shipped-by-none")
			}
			return flags.edit(command, "changed %s in %s", func(config *Config) (textEdit, error) {
				return flags.set(command, arguments[0], config)
			})
		},
	}
	command.Args = cobra.ExactArgs(1)
	flags.bind(command, true)
	return command
}

func newModulesRemoveCmd() *cobra.Command {
	flags := moduleEditFlags{}
	command := &cobra.Command{
		Use:   "remove <module>",
		Short: "Remove the declaration of a module from bumpit.yaml",
		Long:  "bumpit modules remove removes the declaration of the module, given by its name or a directory relative to --repo, with the comments inside it. Removing the last one leaves modules: [], which still says the repository declares no module. The entry of a Go module goes, and the module stays, called by its directory again. The file is chosen and proved as bumpit modules add does.",
		RunE: func(command *cobra.Command, arguments []string) error {
			return flags.edit(command, "removed %s from %s", func(config *Config) (textEdit, error) {
				return flags.remove(arguments[0], config)
			})
		},
	}
	command.Args = cobra.ExactArgs(1)
	flags.configEditFlags.bind(command)
	return command
}

type moduleEditFlags struct {
	configEditFlags
	name          string
	directories   []string
	tagPrefix     string
	paths         []string
	noPaths       bool
	shippedBy     []string
	shippedByNone bool
	// top is the top of the repository, which the paths of a declaration
	// are relative to.
	top string
	// snapshot is what the command reads of the repository.
	snapshot *repositorySnapshot
	// subject is what the commands call the module the edit is about, and
	// formerSubject what they called it before, empty for a new one.
	subject       string
	formerSubject string
	// unshipped reports an edit that says no module carries the module on
	// the strength of no flag, which the command then says it rests on.
	unshipped bool
	// keptCarrier is what the command says of the entries that keep a name in
	// shippedBy that the edit could not follow, empty when there are none.
	keptCarrier string
}

// bind binds the flags of a declaration. changing adds the flags that only an
// existing declaration has a use for: a new one gives its directories as
// arguments, and leaving out --path or --shipped-by already declares none.
func (f *moduleEditFlags) bind(command *cobra.Command, changing bool) {
	f.configEditFlags.bind(command)
	command.Flags().StringVarP(&f.name, "name", "e", "", "what --module, shippedBy and the output call the module; several directories need one (default its directory)")
	command.Flags().StringVarP(&f.tagPrefix, "tag-prefix", "T", "", `what every version tag of the module starts with, its "v" included (default the name or the directory followed by "/v")`)
	command.Flags().StringArrayVarP(&f.paths, "path", "f", nil, "a file or directory outside the module, relative to --repo, that the module is built from; repeat it for each")
	command.Flags().StringArrayVarP(&f.shippedBy, "shipped-by", "b", nil, "the name, or the directory relative to --repo, of a module whose release also carries this one; repeat it for each")
	if changing {
		command.Flags().StringArrayVarP(&f.directories, "directory", "D", nil, "a directory the module owns, relative to --repo, in place of those it has; repeat it for each")
		command.Flags().BoolVarP(&f.noPaths, "no-paths", "N", false, "leave out every path the declaration lists")
		command.Flags().BoolVarP(&f.shippedByNone, "shipped-by-none", "B", false, "say that no other module's release carries this one")
	}
	command.PreRunE = func(command *cobra.Command, _ []string) error {
		return checkExclusiveFlags(command, [2]string{"shipped-by", "shipped-by-none"}, [2]string{"path", "no-paths"})
	}
}

// flagGiven reports whether the command line gives the flag: a switch only
// when it turns it on, since --no-paths=false asks for nothing.
func flagGiven(command *cobra.Command, name string) bool {
	flag := command.Flags().Lookup(name)
	if flag == nil || !flag.Changed {
		return false
	}
	return flag.Value.Type() != "bool" || flag.Value.String() == "true"
}

// checkExclusiveFlags refuses two flags of a pair given together.
func checkExclusiveFlags(command *cobra.Command, pairs ...[2]string) error {
	for _, pair := range pairs {
		if flagGiven(command, pair[0]) && flagGiven(command, pair[1]) {
			return fmt.Errorf("--%s cannot be used with --%s", pair[0], pair[1])
		}
	}
	return nil
}

// moduleChange is one of add, set and remove: it applies the change to the
// config and returns the text edit that writes it.
type moduleChange func(config *Config) (textEdit, error)

// edit applies change to the config file whose modules the runs read, and
// prints done with what the commands call the module and the file.
func (f *moduleEditFlags) edit(command *cobra.Command, done string, change moduleChange) error {
	path, err := f.modulesFile(command)
	if err != nil {
		return err
	}

	var discovered []repositoryModule
	edit := configEdit{
		path:      path,
		create:    true,
		rehearsal: rehearsalOf(command),
		subjects:  func() []editSubject { return []editSubject{{current: f.subject, former: f.formerSubject}} },
		change: func(want *Config, _ Config) (textEdit, error) {
			return change(want)
		},
		repository: func(modules []ModuleDeclaration) ([]Finding, error) {
			report, err := repositoryFindings(f.snapshot, modules)
			discovered = report.modules
			return report.findings, err
		},
	}
	if err := edit.apply(); err != nil {
		return err
	}

	message := fmt.Sprintf(done, f.subject, path)
	if f.unshipped {
		message += "\n" + unshippedNotice(f.subject, discovered)
	}
	if f.keptCarrier != "" {
		message += "\n" + f.keptCarrier
	}
	return printEdit(command.OutOrStdout(), message)
}

// unshippedNotice says what a declaration no flag gave a carrier rests on,
// and which modules stop reading the module it declares.
func unshippedNotice(subject string, modules []repositoryModule) string {
	notice := "no -b, so no module ships " + subject
	if around := modulesAround(subject, modules); len(around) > 0 {
		notice += ", and the modules around it no longer read it (" + strings.Join(around, ", ") + ")"
	}
	return notice + ", but " + unseenShipping + ", and give that module with bumpit modules set -b if one does"
}

// modulesAround lists the modules nearest around the directories of the module
// called name.
func modulesAround(name string, modules []repositoryModule) []string {
	index := moduleNamed(modules, name)
	if index < 0 {
		return nil
	}
	var around []string
	for _, other := range modules {
		if slices.ContainsFunc(modules[index].directories, func(directory string) bool { return encloses(other, directory, modules) }) {
			around = append(around, other.name)
		}
	}
	return around
}

// modulesFile is the config file whose modules the runs read: the one
// --config names when it declares modules, else the bumpit.yaml or
// .bumpit.yaml at the top of the repository, else a new bumpit.yaml there.
// It applies the repository the config gives, as the runs do. Of that config it
// validates only the repository it applies, since the edit may write another file.
func (f *moduleEditFlags) modulesFile(command *cobra.Command) (string, error) {
	config, configPath, err := f.applyConfigRepository(command)
	if err != nil {
		return "", err
	}
	f.snapshot = rehearsalOf(command).snapshotAt(f.repository)
	source, err := findModuleSource(f.configPath != "", f.snapshot.top, config, configPath)
	switch {
	case err != nil:
		return "", err
	case source.top == "":
		return "", errNoRepository(f.repository, source.noTop)
	}
	f.top = source.top

	if f.configPath == "" {
		return topConfigOrNew(source.top, source.topConfig), nil
	}
	if source.fromConfig() {
		return configPath, nil
	}
	return "", fmt.Errorf("%s declares no modules, so the runs read them from the bumpit.yaml at the top of the repository: leave out --config to edit that one, or write modules: [] in %s first", configPath, configPath)
}

// applyConfigRepository reads the config --config names, or else the one in
// --repo, and applies the repository it gives unless the command line gives
// one, as the runs do. It stops at a repository key that leads nowhere, whose
// finding says more than that the path is no repository. It returns that config,
// nil when there is none, and its path.
func (f *configEditFlags) applyConfigRepository(command *cobra.Command) (*Config, string, error) {
	config, configPath, err := f.peekConfig()
	if err != nil || config == nil {
		return nil, "", err
	}
	overrideString(command.Flags(), "repo", &f.repository, config.Repository)
	if f.repository == config.Repository {
		if err := repositoryKeyFinding(*config); err != nil {
			return nil, "", err
		}
	}
	return config, configPath, nil
}

// peekConfig is the config --config names, or else the one in --repo, nil
// when there is none, read but not validated: what the file says, with nothing of
// the command line. An error about the file comes with its path.
func (f *configEditFlags) peekConfig() (*Config, string, error) {
	path := f.configPath
	if path == "" {
		found, err := findConfig(f.repository)
		if errors.Is(err, ErrConfigNotFound) {
			return nil, "", nil
		}
		if err != nil {
			return nil, found, err
		}
		path = found
	}
	config, absolutePath, err := decodeConfig(path, configEditDefaults)
	if err != nil {
		return nil, path, err
	}
	return &config, absolutePath, nil
}

// topConfigOrNew is the config at the top of the repository, or a new
// bumpit.yaml there when it has none.
func topConfigOrNew(top, topConfig string) string {
	if topConfig != "" {
		return topConfig
	}
	return filepath.Join(top, "bumpit.yaml")
}

// printEdit prints what the edit did and that the file is to be committed:
// the runs read it from the working tree, but the other clones of the
// repository only once it is committed.
func printEdit(writer io.Writer, done string) error {
	_, err := fmt.Fprintf(writer, "%s\ncommit the file so that every clone of the repository reads the change\n", done)
	return err
}
