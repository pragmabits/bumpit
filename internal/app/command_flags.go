package app

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Shorthands are declared once and shared by every command so a letter always
// means the same thing. Cobra reserves -h for help.
func bindConfigFlag(command *cobra.Command, target *string) {
	command.Flags().StringVarP(target, "config", "c", "", "path to bumpit.yaml")
}

func bindRepositoryFlag(command *cobra.Command, target *string) {
	command.Flags().StringVarP(target, "repo", "r", ".", "path to the git repository")
}

func bindTagMatchFlag(command *cobra.Command, target *string) {
	command.Flags().StringVarP(target, "match", "t", "", `tag pattern used to filter candidate tags (default "v*", or the Go module's tag prefix)`)
}

func bindModuleFlag(command *cobra.Command, target *string) {
	command.Flags().StringVarP(target, "module", "g", "", "Go module directory to version, relative to --repo; sets the tag pattern and the commits read")
}

func bindStartVersionFlag(command *cobra.Command, target *string) {
	command.Flags().StringVarP(target, "start-version", "s", "", "version to use for the first release when no tags exist")
}

func bindAllowDirtyFlag(command *cobra.Command, target *bool) {
	command.Flags().BoolVarP(target, "allow-dirty", "d", false, "allow a dirty working tree")
}

func bindPreReleaseFlag(command *cobra.Command, target *string) {
	command.Flags().StringVarP(target, "pre", "p", "", "prerelease label to apply, for example beta or rc.1")
}

func bindPromoteFlag(command *cobra.Command, target *bool) {
	command.Flags().BoolVarP(target, "promote", "P", false, "promote the current prerelease to a final release")
}

func bindReleaseAsFlag(command *cobra.Command, target *string) {
	command.Flags().StringVarP(target, "release-as", "V", "", "release this exact version, for example 1.0.0 to declare the public API stable")
}

func bindOutputFlag(command *cobra.Command, target *string) {
	command.Flags().StringVarP(target, "output", "o", "text", "output format: text or json")
}

type releasePlanFlags struct {
	configPath   string
	repository   string
	tagMatch     string
	startVersion string
	allowDirty   bool
	output       string
	preRelease   string
	promote      bool
	releaseAs    string
	module       string
}

func (f releasePlanFlags) defaults() Config {
	allowDirty := f.allowDirty
	promote := f.promote
	return Config{
		Repository:   f.repository,
		TagMatch:     f.tagMatch,
		StartVersion: f.startVersion,
		AllowDirty:   &allowDirty,
		Output:       f.output,
		PreRelease:   f.preRelease,
		Promote:      &promote,
	}
}

func (f releasePlanFlags) options() Options {
	return Options{
		Repository:   f.repository,
		TagMatch:     f.tagMatch,
		StartVersion: f.startVersion,
		AllowDirty:   f.allowDirty,
		PreRelease:   f.preRelease,
		Promote:      f.promote,
		ReleaseAs:    f.releaseAs,
		Module:       f.module,
	}
}

func (f *releasePlanFlags) bind(command *cobra.Command, includeOutput bool) {
	bindConfigFlag(command, &f.configPath)
	bindRepositoryFlag(command, &f.repository)
	bindTagMatchFlag(command, &f.tagMatch)
	bindStartVersionFlag(command, &f.startVersion)
	bindAllowDirtyFlag(command, &f.allowDirty)
	bindPreReleaseFlag(command, &f.preRelease)
	bindPromoteFlag(command, &f.promote)
	bindReleaseAsFlag(command, &f.releaseAs)
	bindModuleFlag(command, &f.module)
	if includeOutput {
		bindOutputFlag(command, &f.output)
	}
}

func (f *releasePlanFlags) applyConfig(command *cobra.Command) (*Config, error) {
	config, _, err := loadConfig(f.configPath, f.repository, f.defaults())
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, nil
	}

	commandFlags := command.Flags()
	overrideString(commandFlags, "repo", &f.repository, config.Repository)
	overrideString(commandFlags, "match", &f.tagMatch, config.TagMatch)
	overrideString(commandFlags, "start-version", &f.startVersion, config.StartVersion)
	overrideBool(commandFlags, "allow-dirty", &f.allowDirty, config.AllowDirty)
	overrideString(commandFlags, "output", &f.output, config.Output)
	overrideString(commandFlags, "pre", &f.preRelease, config.PreRelease)
	overrideBool(commandFlags, "promote", &f.promote, config.Promote)

	return config, nil
}

type latestCommandFlags struct {
	configPath string
	repository string
	tagMatch   string
	output     string
	all        bool
	noPrefix   bool
	module     string
}

func (f latestCommandFlags) defaults() Config {
	return Config{
		Repository: f.repository,
		TagMatch:   f.tagMatch,
		Output:     f.output,
	}
}

func (f latestCommandFlags) options() LatestOptions {
	return LatestOptions{
		Repository: f.repository,
		TagMatch:   f.tagMatch,
		All:        f.all,
		Module:     f.module,
	}
}

func (f *latestCommandFlags) bind(command *cobra.Command) {
	bindConfigFlag(command, &f.configPath)
	bindRepositoryFlag(command, &f.repository)
	bindTagMatchFlag(command, &f.tagMatch)
	bindOutputFlag(command, &f.output)
	bindModuleFlag(command, &f.module)
	command.Flags().BoolVarP(&f.all, "all", "a", false, "consider every tag in the repository, not only the ones reachable from HEAD")
	command.Flags().BoolVarP(&f.noPrefix, "no-prefix", "n", false, "print the bare version instead of the tag, dropping its prefix")
}

func (f *latestCommandFlags) applyConfig(command *cobra.Command) (*Config, error) {
	config, _, err := loadConfig(f.configPath, f.repository, f.defaults())
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, nil
	}

	flags := command.Flags()
	overrideString(flags, "repo", &f.repository, config.Repository)
	overrideString(flags, "match", &f.tagMatch, config.TagMatch)
	overrideString(flags, "output", &f.output, config.Output)

	return config, nil
}

type tagCommandFlags struct {
	configPath   string
	repository   string
	tagMatch     string
	startVersion string
	allowDirty   bool
	message      string
	preRelease   string
	promote      bool
	releaseAs    string
	module       string
}

func (f tagCommandFlags) defaults() Config {
	allowDirty := f.allowDirty
	promote := f.promote
	return Config{
		Repository:   f.repository,
		TagMatch:     f.tagMatch,
		StartVersion: f.startVersion,
		AllowDirty:   &allowDirty,
		Output:       "text",
		TagMessage:   f.message,
		PreRelease:   f.preRelease,
		Promote:      &promote,
	}
}

func (f tagCommandFlags) options() Options {
	return Options{
		Repository:   f.repository,
		TagMatch:     f.tagMatch,
		StartVersion: f.startVersion,
		AllowDirty:   f.allowDirty,
		PreRelease:   f.preRelease,
		Promote:      f.promote,
		ReleaseAs:    f.releaseAs,
		Module:       f.module,
	}
}

func (f *tagCommandFlags) bind(command *cobra.Command) {
	bindConfigFlag(command, &f.configPath)
	bindRepositoryFlag(command, &f.repository)
	bindTagMatchFlag(command, &f.tagMatch)
	bindStartVersionFlag(command, &f.startVersion)
	bindAllowDirtyFlag(command, &f.allowDirty)
	bindPreReleaseFlag(command, &f.preRelease)
	bindPromoteFlag(command, &f.promote)
	bindReleaseAsFlag(command, &f.releaseAs)
	bindModuleFlag(command, &f.module)
	command.Flags().StringVarP(&f.message, "message", "m", "", "annotated tag message")
}

func (f *tagCommandFlags) applyConfig(command *cobra.Command) (*Config, error) {
	config, _, err := loadConfig(f.configPath, f.repository, f.defaults())
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, nil
	}

	flags := command.Flags()
	overrideString(flags, "repo", &f.repository, config.Repository)
	overrideString(flags, "match", &f.tagMatch, config.TagMatch)
	overrideString(flags, "start-version", &f.startVersion, config.StartVersion)
	overrideBool(flags, "allow-dirty", &f.allowDirty, config.AllowDirty)
	overrideString(flags, "pre", &f.preRelease, config.PreRelease)
	overrideBool(flags, "promote", &f.promote, config.Promote)

	return config, nil
}

type modulesCommandFlags struct {
	configPath string
	repository string
	output     string
	allowDirty bool
}

func (f modulesCommandFlags) defaults() Config {
	allowDirty := f.allowDirty
	return Config{
		Repository: f.repository,
		Output:     f.output,
		AllowDirty: &allowDirty,
	}
}

func (f modulesCommandFlags) options() Options {
	return Options{
		Repository: f.repository,
		AllowDirty: f.allowDirty,
	}
}

func (f *modulesCommandFlags) bind(command *cobra.Command) {
	bindConfigFlag(command, &f.configPath)
	bindRepositoryFlag(command, &f.repository)
	bindOutputFlag(command, &f.output)
	bindAllowDirtyFlag(command, &f.allowDirty)
}

func (f *modulesCommandFlags) applyConfig(command *cobra.Command) error {
	config, _, err := loadConfig(f.configPath, f.repository, f.defaults())
	if err != nil || config == nil {
		return err
	}

	flags := command.Flags()
	overrideString(flags, "repo", &f.repository, config.Repository)
	overrideString(flags, "output", &f.output, config.Output)
	overrideBool(flags, "allow-dirty", &f.allowDirty, config.AllowDirty)
	return nil
}

// overrideString sets target to the config value when the command defines the
// flag, the flag was not given on the command line and the config carries a
// value: an explicit flag always wins over the file.
func overrideString(flags *pflag.FlagSet, name string, target *string, value string) {
	if flags.Lookup(name) == nil || flags.Changed(name) || value == "" {
		return
	}
	*target = value
}

// overrideBool is overrideString for a boolean the config may leave unset.
func overrideBool(flags *pflag.FlagSet, name string, target *bool, value *bool) {
	if flags.Lookup(name) == nil || flags.Changed(name) || value == nil {
		return
	}
	*target = *value
}

func loadConfig(path, repository string, defaults Config) (*Config, string, error) {
	if path != "" {
		config, resolvedConfigPath, err := LoadConfig(path, defaults)
		if err != nil {
			return nil, "", err
		}
		return &config, resolvedConfigPath, nil
	}

	config, resolvedConfigPath, err := DiscoverConfig(repository, defaults)
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return nil, "", nil
		}
		return nil, "", err
	}

	return &config, resolvedConfigPath, nil
}
