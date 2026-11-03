package app

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"

	"github.com/pragmabits/bumpit/internal/gitx"
	"github.com/spf13/cobra"
)

func newModulesAddCmd() *cobra.Command {
	flags := moduleEditFlags{}
	command := &cobra.Command{
		Use:   "add <directory>",
		Short: "Declare a module in bumpit.yaml",
		Long:  "bumpit modules add declares the module in the directory, relative to --repo, in the config file whose modules the runs read: the one --config names when it declares modules, else the bumpit.yaml at the top of the repository, created when the repository has none. --path and --shipped-by are relative to --repo too, and every path is written relative to the top of the repository. The rest of the file is left as it is, and the file is written only once the new config loads and the repository accepts its modules.",
		RunE: func(command *cobra.Command, arguments []string) error {
			if !flagGiven(command, "shipped-by") && !flagGiven(command, "shipped-by-none") {
				return fmt.Errorf("bumpit modules add needs --shipped-by or --shipped-by-none: name the modules whose release also carries %s, or say that none does", arguments[0])
			}
			return flags.edit(command, arguments[0], "added %s to %s", flags.add)
		},
	}
	command.Args = cobra.ExactArgs(1)
	flags.bind(command, false)
	return command
}

func newModulesSetCmd() *cobra.Command {
	flags := moduleEditFlags{}
	command := &cobra.Command{
		Use:   "set <directory>",
		Short: "Change the declaration of a module in bumpit.yaml",
		Long:  "bumpit modules set changes the declaration of the module in the directory, relative to --repo: the flags given replace what the declaration says, and the others leave it. The declaration is written anew, so one that holds a comment is refused. The file is chosen and proved as bumpit modules add does.",
		RunE: func(command *cobra.Command, arguments []string) error {
			changes := []string{"tag-prefix", "path", "no-paths", "shipped-by", "shipped-by-none"}
			if !slices.ContainsFunc(changes, func(name string) bool { return flagGiven(command, name) }) {
				return fmt.Errorf("nothing to change: give --tag-prefix, --path, --no-paths, --shipped-by or --shipped-by-none")
			}
			return flags.edit(command, arguments[0], "changed %s in %s", flags.set)
		},
	}
	command.Args = cobra.ExactArgs(1)
	flags.bind(command, true)
	return command
}

func newModulesRemoveCmd() *cobra.Command {
	flags := moduleEditFlags{}
	command := &cobra.Command{
		Use:   "remove <directory>",
		Short: "Remove the declaration of a module from bumpit.yaml",
		Long:  "bumpit modules remove removes the declaration of the module in the directory, relative to --repo, with the comments inside it. Removing the last one leaves modules: [], which still says the repository declares no module. The file is chosen and proved as bumpit modules add does.",
		RunE: func(command *cobra.Command, arguments []string) error {
			return flags.edit(command, arguments[0], "removed %s from %s", flags.remove)
		},
	}
	command.Args = cobra.ExactArgs(1)
	flags.configEditFlags.bind(command)
	return command
}

type moduleEditFlags struct {
	configEditFlags
	tagPrefix     string
	paths         []string
	noPaths       bool
	shippedBy     []string
	shippedByNone bool
	// top is the top of the repository, which the paths of a declaration
	// are relative to.
	top string
}

func (f *moduleEditFlags) bind(command *cobra.Command, withNoPaths bool) {
	f.configEditFlags.bind(command)
	command.Flags().StringVarP(&f.tagPrefix, "tag-prefix", "T", "", `what every version tag of the module starts with, its "v" included (default the directory followed by "/v")`)
	command.Flags().StringArrayVarP(&f.paths, "path", "f", nil, "a file or directory outside the module, relative to --repo, that the module is built from; repeat it for each")
	command.Flags().StringArrayVarP(&f.shippedBy, "shipped-by", "b", nil, "the directory, relative to --repo, of a module whose release also carries this one; repeat it for each")
	command.Flags().BoolVarP(&f.shippedByNone, "shipped-by-none", "B", false, "say that no other module's release carries this one")
	if withNoPaths {
		command.Flags().BoolVarP(&f.noPaths, "no-paths", "N", false, "leave out every path the declaration lists")
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
// module in directory, given relative to the top of the repository, to the
// config and returns the text edit that writes it.
type moduleChange func(command *cobra.Command, directory string, config *Config) (textEdit, error)

// edit applies change to the config file whose modules the runs read, and
// prints done with the directory and the file.
func (f *moduleEditFlags) edit(command *cobra.Command, directory, done string, change moduleChange) error {
	path, err := f.modulesFile(command)
	if err != nil {
		return err
	}
	relative, err := f.relative(directory)
	if err != nil {
		return err
	}

	edit := configEdit{
		path:   path,
		create: true,
		change: func(want *Config, _ Config) (textEdit, error) {
			return change(command, relative, want)
		},
		check: func(edited Config) error {
			_, _, err := discoverRepositoryModules(gitx.New(f.top), edited.Modules)
			return err
		},
	}
	if err := edit.apply(); err != nil {
		return err
	}
	return printEdit(command.OutOrStdout(), fmt.Sprintf(done, relative, path))
}

// modulesFile is the config file whose modules the runs read: the one
// --config names when it declares modules, else the bumpit.yaml or
// .bumpit.yaml at the top of the repository, else a new bumpit.yaml there.
// It applies the repository the config gives, as the runs do. It reads that
// config without validating it, since the edit may write another file.
func (f *moduleEditFlags) modulesFile(command *cobra.Command) (string, error) {
	config, configPath, err := f.peekConfig()
	if err != nil {
		return "", err
	}
	if config != nil {
		overrideString(command.Flags(), "repo", &f.repository, config.Repository)
	}
	top, topConfig, err := findTopConfig(f.repository)
	switch {
	case err != nil:
		return "", err
	case top == "":
		return "", fmt.Errorf("%s is in no git repository, whose modules bumpit.yaml would declare", f.repository)
	}
	f.top = top

	if f.configPath == "" {
		return topConfigOrNew(top, topConfig), nil
	}
	if config.Modules != nil || sameFile(configPath, topConfig) {
		return configPath, nil
	}
	return "", fmt.Errorf("%s declares no modules, so the runs read them from the bumpit.yaml at the top of the repository: leave out --config to edit that one, or write modules: [] in %s first", configPath, configPath)
}

// peekConfig is the config --config names, or else the one in --repo, nil
// when there is none, read but not validated.
func (f *moduleEditFlags) peekConfig() (*Config, string, error) {
	path := f.configPath
	if path == "" {
		found, err := findConfig(f.repository)
		if errors.Is(err, ErrConfigNotFound) {
			return nil, "", nil
		}
		if err != nil {
			return nil, "", err
		}
		path = found
	}
	config, absolutePath, err := decodeConfig(path, Config{Repository: f.repository, Output: "text"})
	if err != nil {
		return nil, "", err
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

// relative is the path given relative to --repo, relative to the top of
// the repository.
func (f *moduleEditFlags) relative(path string) (string, error) {
	return relativeToTop(f.top, f.repository, path)
}

func (f *moduleEditFlags) relativeAll(paths []string) ([]string, error) {
	relative := make([]string, 0, len(paths))
	for _, path := range paths {
		entry, err := f.relative(path)
		if err != nil {
			return nil, err
		}
		relative = append(relative, entry)
	}
	return relative, nil
}

func (f *moduleEditFlags) add(_ *cobra.Command, directory string, config *Config) (textEdit, error) {
	declaration := ModuleDeclaration{Directory: directory, TagPrefix: f.tagPrefix}
	if err := f.applyPaths(&declaration); err != nil {
		return nil, err
	}
	shippedBy, err := f.relativeAll(f.shippedBy)
	if err != nil {
		return nil, err
	}
	declaration.ShippedBy = &shippedBy
	config.Modules = append(config.Modules, declaration)
	return func(content []byte) ([]byte, error) { return addModuleText(content, declaration) }, nil
}

func (f *moduleEditFlags) set(command *cobra.Command, directory string, config *Config) (textEdit, error) {
	index, err := declaredIndex(config.Modules, directory)
	if err != nil {
		return nil, err
	}
	declaration := config.Modules[index]
	if flagGiven(command, "tag-prefix") {
		declaration.TagPrefix = f.tagPrefix
	}
	if flagGiven(command, "path") || flagGiven(command, "no-paths") {
		if err := f.applyPaths(&declaration); err != nil {
			return nil, err
		}
	}
	if flagGiven(command, "shipped-by") || flagGiven(command, "shipped-by-none") {
		shippedBy, err := f.relativeAll(f.shippedBy)
		if err != nil {
			return nil, err
		}
		declaration.ShippedBy = &shippedBy
	}
	config.Modules[index] = declaration
	return func(content []byte) ([]byte, error) { return setModuleText(content, declaration) }, nil
}

func (f *moduleEditFlags) remove(_ *cobra.Command, directory string, config *Config) (textEdit, error) {
	index, err := declaredIndex(config.Modules, directory)
	if err != nil {
		return nil, err
	}
	config.Modules = slices.Delete(config.Modules, index, index+1)
	return func(content []byte) ([]byte, error) { return removeModuleText(content, directory) }, nil
}

// applyPaths gives the declaration the paths of --path, none when it gives
// none, as a config that leaves out paths loads.
func (f *moduleEditFlags) applyPaths(declaration *ModuleDeclaration) error {
	declaration.Paths = nil
	if len(f.paths) == 0 {
		return nil
	}
	paths, err := f.relativeAll(f.paths)
	declaration.Paths = paths
	return err
}

func declaredIndex(declarations []ModuleDeclaration, directory string) (int, error) {
	index := slices.IndexFunc(declarations, func(declaration ModuleDeclaration) bool { return declaration.Directory == directory })
	if index < 0 {
		return 0, fmt.Errorf("%s is not declared: declare it with bumpit modules add", directory)
	}
	return index, nil
}

// printEdit prints what the edit did and that the file is to be committed:
// the runs read it from the working tree, but the other clones of the
// repository only once it is committed.
func printEdit(writer io.Writer, done string) error {
	_, err := fmt.Fprintf(writer, "%s\ncommit the file so that every clone of the repository reads the change\n", done)
	return err
}
