package app

import (
	"errors"

	"github.com/spf13/cobra"
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
	command.Flags().StringVarP(target, "match", "t", "v*", "tag pattern used to filter candidate tags")
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
	if !commandFlags.Changed("repo") && config.Repository != "" {
		f.repository = config.Repository
	}
	if !commandFlags.Changed("match") && config.TagMatch != "" {
		f.tagMatch = config.TagMatch
	}
	if !commandFlags.Changed("start-version") && config.StartVersion != "" {
		f.startVersion = config.StartVersion
	}
	if !commandFlags.Changed("allow-dirty") && config.AllowDirty != nil {
		f.allowDirty = *config.AllowDirty
	}
	if commandFlags.Lookup("output") != nil && !commandFlags.Changed("output") && config.Output != "" {
		f.output = config.Output
	}
	if !commandFlags.Changed("pre") && config.PreRelease != "" {
		f.preRelease = config.PreRelease
	}
	if !commandFlags.Changed("promote") && config.Promote != nil {
		f.promote = *config.Promote
	}

	return config, nil
}

type latestCommandFlags struct {
	configPath string
	repository string
	tagMatch   string
	output     string
	all        bool
	noPrefix   bool
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
	}
}

func (f *latestCommandFlags) bind(command *cobra.Command) {
	bindConfigFlag(command, &f.configPath)
	bindRepositoryFlag(command, &f.repository)
	bindTagMatchFlag(command, &f.tagMatch)
	bindOutputFlag(command, &f.output)
	command.Flags().BoolVarP(&f.all, "all", "a", false, "consider every tag in the repository, not only the ones reachable from HEAD")
	command.Flags().BoolVarP(&f.noPrefix, "no-prefix", "n", false, "print the bare version instead of the tag, dropping the v prefix")
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
	if !flags.Changed("repo") && config.Repository != "" {
		f.repository = config.Repository
	}
	if !flags.Changed("match") && config.TagMatch != "" {
		f.tagMatch = config.TagMatch
	}
	if !flags.Changed("output") && config.Output != "" {
		f.output = config.Output
	}

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
	if !flags.Changed("repo") && config.Repository != "" {
		f.repository = config.Repository
	}
	if !flags.Changed("match") && config.TagMatch != "" {
		f.tagMatch = config.TagMatch
	}
	if !flags.Changed("start-version") && config.StartVersion != "" {
		f.startVersion = config.StartVersion
	}
	if !flags.Changed("allow-dirty") && config.AllowDirty != nil {
		f.allowDirty = *config.AllowDirty
	}
	if !flags.Changed("pre") && config.PreRelease != "" {
		f.preRelease = config.PreRelease
	}
	if !flags.Changed("promote") && config.Promote != nil {
		f.promote = *config.Promote
	}

	return config, nil
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
