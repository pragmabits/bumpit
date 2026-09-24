package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pragmabits/bumpit/internal/semver"
	"github.com/spf13/viper"
)

// ErrConfigNotFound is returned by DiscoverConfig when the repository holds
// neither bumpit.yaml nor .bumpit.yaml.
var ErrConfigNotFound = errors.New("bumpit config not found")

// Config is bumpit.yaml. A flag given on the command line overrides the
// value here, and a value here overrides the flag's default.
type Config struct {
	// Repository is the git repository, relative to the config file.
	Repository string `mapstructure:"repository"`
	// TagMatch is the tag pattern. Empty means "v*", or the tag prefix of
	// the root Go module when the repository has one.
	TagMatch string `mapstructure:"tagMatch"`
	// StartVersion is the first release when the repository has no tag.
	StartVersion string `mapstructure:"startVersion"`
	AllowDirty   *bool  `mapstructure:"allowDirty"`
	// Output is text or json.
	Output     string `mapstructure:"output"`
	TagMessage string `mapstructure:"tagMessage"`
	PreRelease string `mapstructure:"preRelease"`
	Promote    *bool  `mapstructure:"promote"`
}

// LoadConfig reads the config at path over defaults and validates it. It
// returns the config and its absolute path.
func LoadConfig(path string, defaults Config) (Config, string, error) {
	if strings.TrimSpace(path) == "" {
		return Config{}, "", fmt.Errorf("config path cannot be empty")
	}

	configReader := newConfigReader(defaults)
	configReader.SetConfigFile(path)
	if err := configReader.ReadInConfig(); err != nil {
		return Config{}, "", err
	}

	var config Config
	if err := configReader.UnmarshalExact(&config); err != nil {
		return Config{}, "", fmt.Errorf("parse config %q: %w", path, err)
	}

	absolutePath, err := filepath.Abs(configReader.ConfigFileUsed())
	if err != nil {
		return Config{}, "", err
	}
	config.Repository = resolveRepositoryPath(absolutePath, config.Repository)
	if err := validateConfig(config); err != nil {
		return Config{}, "", err
	}

	return config, absolutePath, nil
}

// DiscoverConfig loads bumpit.yaml, or else .bumpit.yaml, from the repository,
// and returns ErrConfigNotFound when neither exists.
func DiscoverConfig(repository string, defaults Config) (Config, string, error) {
	for _, configPath := range configCandidatePaths(repository) {
		if _, err := os.Stat(configPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return Config{}, "", err
		}

		config, resolvedConfigPath, err := LoadConfig(configPath, defaults)
		if err == nil {
			return config, resolvedConfigPath, nil
		}
		return Config{}, "", err
	}

	return Config{}, "", ErrConfigNotFound
}

func newConfigReader(defaults Config) *viper.Viper {
	configReader := viper.New()
	configReader.SetDefault("repository", defaults.Repository)
	configReader.SetDefault("tagMatch", defaults.TagMatch)
	configReader.SetDefault("startVersion", defaults.StartVersion)
	if defaults.AllowDirty != nil {
		configReader.SetDefault("allowDirty", *defaults.AllowDirty)
	}
	configReader.SetDefault("output", defaults.Output)
	configReader.SetDefault("tagMessage", defaults.TagMessage)
	configReader.SetDefault("preRelease", defaults.PreRelease)
	if defaults.Promote != nil {
		configReader.SetDefault("promote", *defaults.Promote)
	}
	return configReader
}

func configCandidatePaths(repository string) []string {
	baseDirectory := "."
	if strings.TrimSpace(repository) != "" {
		baseDirectory = repository
	}

	return []string{
		filepath.Join(baseDirectory, "bumpit.yaml"),
		filepath.Join(baseDirectory, ".bumpit.yaml"),
	}
}

func resolveRepositoryPath(configPath, repository string) string {
	if strings.TrimSpace(repository) == "" || filepath.IsAbs(repository) {
		return repository
	}

	return filepath.Join(filepath.Dir(configPath), repository)
}

func validateConfig(config Config) error {
	if err := validateRepositoryPath(config.Repository); err != nil {
		return err
	}

	if err := validateOutputFormat(config.Output); err != nil {
		return err
	}

	return validateReleaseSettings(config)
}

func validateRepositoryPath(repository string) error {
	if strings.TrimSpace(repository) == "" {
		return fmt.Errorf("repository cannot be empty")
	}

	repositoryInfo, err := os.Stat(repository)
	if err != nil {
		return fmt.Errorf("repository path %q: %w", repository, err)
	}
	if !repositoryInfo.IsDir() {
		return fmt.Errorf("repository path %q is not a directory", repository)
	}
	return nil
}

func validateReleaseSettings(config Config) error {
	if strings.TrimSpace(config.PreRelease) != "" {
		if _, err := (semver.Version{}).WithPreRelease(config.PreRelease); err != nil {
			return fmt.Errorf("invalid preRelease: %w", err)
		}
	}

	if config.Promote != nil && *config.Promote && strings.TrimSpace(config.PreRelease) != "" {
		return fmt.Errorf("preRelease and promote cannot be used together")
	}

	if strings.TrimSpace(config.StartVersion) != "" {
		if _, err := versionAfterPrefix(config.StartVersion, tagPrefix(config.TagMatch)); err != nil {
			return fmt.Errorf("invalid startVersion: %w", err)
		}
	}

	return nil
}
