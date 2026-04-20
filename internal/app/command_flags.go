package app

import (
	"errors"

	"github.com/spf13/cobra"
)

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
	command.Flags().StringVar(&f.configPath, "config", "", "path to bumpit.yaml")
	command.Flags().StringVar(&f.repository, "repo", ".", "path to the git repository")
	command.Flags().StringVar(&f.tagMatch, "match", "v*", "tag pattern used with git describe")
	command.Flags().StringVar(&f.startVersion, "start-version", "", "version to use for the first release when no tags exist")
	command.Flags().BoolVar(&f.allowDirty, "allow-dirty", false, "allow a dirty working tree")
	command.Flags().StringVar(&f.preRelease, "pre", "", "prerelease label to apply, for example beta or rc.1")
	command.Flags().BoolVar(&f.promote, "promote", false, "promote the current prerelease to a final release")
	if includeOutput {
		command.Flags().StringVar(&f.output, "output", "text", "output format: text or json")
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
	command.Flags().StringVar(&f.configPath, "config", "", "path to bumpit.yaml")
	command.Flags().StringVar(&f.repository, "repo", ".", "path to the git repository")
	command.Flags().StringVar(&f.tagMatch, "match", "v*", "tag pattern used with git describe")
	command.Flags().StringVar(&f.startVersion, "start-version", "", "version to use for the first release when no tags exist")
	command.Flags().BoolVar(&f.allowDirty, "allow-dirty", false, "allow a dirty working tree")
	command.Flags().StringVar(&f.preRelease, "pre", "", "prerelease label to apply, for example beta or rc.1")
	command.Flags().BoolVar(&f.promote, "promote", false, "promote the current prerelease to a final release")
	command.Flags().StringVar(&f.message, "message", "", "annotated tag message")
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
