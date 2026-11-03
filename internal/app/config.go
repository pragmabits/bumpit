package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/pragmabits/bumpit/internal/gitx"
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
	// the root module when the repository has one.
	TagMatch string `mapstructure:"tagMatch"`
	// StartVersion is the first release when the repository has no tag.
	StartVersion string `mapstructure:"startVersion"`
	AllowDirty   *bool  `mapstructure:"allowDirty"`
	// Output is text or json.
	Output     string `mapstructure:"output"`
	TagMessage string `mapstructure:"tagMessage"`
	PreRelease string `mapstructure:"preRelease"`
	Promote    *bool  `mapstructure:"promote"`
	// Modules declares the modules of the repository that have no go.mod.
	Modules []ModuleDeclaration `mapstructure:"modules"`
}

// LoadConfig reads the config at path over defaults and validates it. It
// returns the config and its absolute path.
func LoadConfig(path string, defaults Config) (Config, string, error) {
	config, absolutePath, err := decodeConfig(path, defaults)
	if err != nil {
		return Config{}, "", err
	}
	if err := validateConfig(config); err != nil {
		return Config{}, "", err
	}
	return config, absolutePath, nil
}

// decodeConfig reads the config at path over defaults, its repository
// resolved against the directory of path, without validating it. It returns
// the config and its absolute path.
func decodeConfig(path string, defaults Config) (Config, string, error) {
	if strings.TrimSpace(path) == "" {
		return Config{}, "", fmt.Errorf("config path cannot be empty")
	}

	configReader := newConfigReader(defaults)
	configReader.SetConfigFile(path)
	if err := configReader.ReadInConfig(); err != nil {
		return Config{}, "", err
	}
	absolutePath, err := filepath.Abs(configReader.ConfigFileUsed())
	if err != nil {
		return Config{}, "", err
	}
	config, err := unmarshalConfig(configReader, path, absolutePath)
	return config, absolutePath, err
}

// decodeConfigContent reads content as the YAML config at path over
// defaults, without validating it, as decodeConfig reads the file.
func decodeConfigContent(content []byte, defaults Config, path string) (Config, error) {
	configReader := newConfigReader(defaults)
	configReader.SetConfigType("yaml")
	if err := configReader.ReadConfig(bytes.NewReader(content)); err != nil {
		return Config{}, err
	}
	return unmarshalConfig(configReader, path, path)
}

// unmarshalConfig is the config configReader read from path, its repository
// resolved against the directory of absolutePath.
func unmarshalConfig(configReader *viper.Viper, path, absolutePath string) (Config, error) {
	var config Config
	if err := configReader.UnmarshalExact(&config); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	config.Repository = resolveRepositoryPath(absolutePath, config.Repository)
	return config, nil
}

// DiscoverConfig loads bumpit.yaml, or else .bumpit.yaml, from the repository,
// and returns ErrConfigNotFound when neither exists.
func DiscoverConfig(repository string, defaults Config) (Config, string, error) {
	configPath, err := findConfig(repository)
	if err != nil {
		return Config{}, "", err
	}
	return LoadConfig(configPath, defaults)
}

// findConfig is the path of bumpit.yaml, or else .bumpit.yaml, in directory,
// and ErrConfigNotFound when neither exists.
func findConfig(directory string) (string, error) {
	for _, configPath := range configCandidatePaths(directory) {
		_, err := os.Stat(configPath)
		if err == nil {
			return configPath, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "", ErrConfigNotFound
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

// readDeclarations is the modules a run reads: those of the config given
// with --config when it declares modules, an empty list included, and else
// those of the config at the top of the repository, wherever --repo points.
// The declarations describe the repository, as its go.mod files do, so a run
// from a subdirectory reads them too, and a config found in a subdirectory
// cannot declare others. config is the config the run loaded, at configPath,
// nil when it loaded none. Outside a repository there is none, and the
// command reports that repository is none.
func readDeclarations(explicit bool, repository string, config *Config, configPath string) ([]ModuleDeclaration, error) {
	if explicit && config != nil && config.Modules != nil {
		return config.Modules, nil
	}

	top, topConfig, err := findTopConfig(repository)
	if err != nil || top == "" {
		return nil, err
	}
	if !explicit && config != nil && len(config.Modules) > 0 && !sameFile(configPath, topConfig) {
		return nil, fmt.Errorf("%s declares modules, which bumpit reads from the bumpit.yaml at the top of the repository: move them to %s", configPath, filepath.Join(top, "bumpit.yaml"))
	}
	return topDeclarations(topConfig, config, configPath)
}

// findTopConfig is the top of the repository at repository and the config
// there, empty when it has none. Outside a repository both are empty: the
// command reports that repository is none.
func findTopConfig(repository string) (string, string, error) {
	top, err := gitx.New(repository).Top()
	if err != nil {
		return "", "", nil
	}
	topConfig, err := findConfig(top)
	if errors.Is(err, ErrConfigNotFound) {
		return top, "", nil
	}
	return top, topConfig, err
}

// topDeclarations reads the modules the config at topConfig declares, from
// config when the run loaded that file already. Of a config the run did not
// load, it reads and checks the modules key alone: the other keys are not
// the run's, so a value one of them could not take does not stop it.
func topDeclarations(topConfig string, config *Config, configPath string) ([]ModuleDeclaration, error) {
	switch {
	case topConfig == "":
		return nil, nil
	case config != nil && sameFile(configPath, topConfig):
		return config.Modules, nil
	}

	configReader := viper.New()
	configReader.SetConfigFile(topConfig)
	var declarations []ModuleDeclaration
	err := configReader.ReadInConfig()
	if err == nil {
		err = configReader.UnmarshalKey("modules", &declarations, func(decoder *mapstructure.DecoderConfig) { decoder.ErrorUnused = true })
	}
	if err == nil {
		err = validateModuleDeclarations(declarations)
	}
	if err != nil {
		return nil, fmt.Errorf("read the modules of %s: %w", topConfig, err)
	}
	return declarations, nil
}

func sameFile(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	leftInfo, err := os.Stat(left)
	if err != nil {
		return false
	}
	rightInfo, err := os.Stat(right)
	return err == nil && os.SameFile(leftInfo, rightInfo)
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

	if err := validateReleaseSettings(config); err != nil {
		return err
	}
	return validateModuleDeclarations(config.Modules)
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
