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

var ErrConfigNotFound = errors.New("bumpit config not found")

type Config struct {
	Repository   string `mapstructure:"repository"`
	TagMatch     string `mapstructure:"tagMatch"`
	StartVersion string `mapstructure:"startVersion"`
	AllowDirty   *bool  `mapstructure:"allowDirty"`
	Output       string `mapstructure:"output"`
	TagMessage   string `mapstructure:"tagMessage"`
	PreRelease   string `mapstructure:"preRelease"`
	Promote      *bool  `mapstructure:"promote"`
}

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
	if strings.TrimSpace(config.Repository) == "" {
		return fmt.Errorf("repository cannot be empty")
	}

	repositoryInfo, err := os.Stat(config.Repository)
	if err != nil {
		return fmt.Errorf("repository path %q: %w", config.Repository, err)
	}
	if !repositoryInfo.IsDir() {
		return fmt.Errorf("repository path %q is not a directory", config.Repository)
	}

	if strings.TrimSpace(config.TagMatch) == "" {
		return fmt.Errorf("tagMatch cannot be empty")
	}

	if err := validateOutputFormat(config.Output); err != nil {
		return err
	}

	if strings.TrimSpace(config.PreRelease) != "" {
		if _, err := (semver.Version{}).WithPreRelease(config.PreRelease); err != nil {
			return fmt.Errorf("invalid preRelease: %w", err)
		}
	}

	if config.Promote != nil && *config.Promote && strings.TrimSpace(config.PreRelease) != "" {
		return fmt.Errorf("preRelease and promote cannot be used together")
	}

	if strings.TrimSpace(config.StartVersion) != "" {
		if _, err := semver.Parse(config.StartVersion); err != nil {
			return fmt.Errorf("invalid startVersion: %w", err)
		}
	}

	return nil
}
