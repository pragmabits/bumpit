package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

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
	// Modules declares the modules of the repository that have no go.mod, and
	// lists the Go modules it names.
	Modules []ModuleDeclaration `mapstructure:"modules"`
}

// LoadConfig reads the config at path over defaults and validates it. It
// returns the config and its absolute path.
func LoadConfig(path string, defaults Config) (Config, string, error) {
	configReader, absolutePath, err := readConfig(path, defaults)
	if err != nil {
		return Config{}, "", err
	}
	config, err := unmarshalConfig(configReader, path, absolutePath)
	if err != nil {
		return Config{}, "", err
	}
	// A repository the file does not hold is the one the command line gives, read
	// from where the command runs, whatever directory the file lies in.
	validated := config
	if !configReader.InConfig("repository") {
		validated.Repository = defaults.Repository
	}
	if err := validateConfig(validated, configReader.InConfig); err != nil {
		return Config{}, "", err
	}
	return config, absolutePath, nil
}

// decodeConfig reads the config at path over defaults, its repository
// resolved against the directory of path, without validating it. It returns
// the config and its absolute path.
func decodeConfig(path string, defaults Config) (Config, string, error) {
	configReader, absolutePath, err := readConfig(path, defaults)
	if err != nil {
		return Config{}, "", err
	}
	config, err := unmarshalConfig(configReader, path, absolutePath)
	return config, absolutePath, err
}

// readConfig reads the file at path over defaults, and returns the reader and the
// absolute path of the file.
func readConfig(path string, defaults Config) (*viper.Viper, string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, "", fmt.Errorf("config path cannot be empty")
	}

	configReader := newConfigReader(defaults)
	configReader.SetConfigFile(path)
	if err := configReader.ReadInConfig(); err != nil {
		return nil, "", err
	}
	absolutePath, err := filepath.Abs(configReader.ConfigFileUsed())
	if err != nil {
		return nil, "", err
	}
	return configReader, absolutePath, nil
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
// and ErrConfigNotFound when neither exists, as neither does in a directory that
// is a file. An error about one of them comes with its path.
func findConfig(directory string) (string, error) {
	for _, configPath := range configCandidatePaths(directory) {
		_, err := os.Stat(configPath)
		if err == nil {
			return configPath, nil
		}
		if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
			return configPath, err
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

func resolveRepositoryPath(configPath, repository string) string {
	if strings.TrimSpace(repository) == "" || filepath.IsAbs(repository) {
		return repository
	}

	return filepath.Join(filepath.Dir(configPath), repository)
}

// validateConfig refuses what config holds, stopping at the first problem. A key
// the file holds, as inFile tells, is a problem of the file, and the finding says
// how to resolve it there. A value the command line gives is none, nor is a conflict
// with one: the file does not say it, so the error says only what is wrong.
func validateConfig(config Config, inFile func(key string) bool) error {
	findings := configKeyFindings(config)
	if len(findings) == 0 {
		return validateModuleDeclarations(config.Modules)
	}
	first := findings[0]
	if !inFile(first.Subject) || first.Other != "" && !inFile(first.Other) {
		return errors.New(first.What)
	}
	return first
}

// checkFlagValues refuses what a run takes from the command line: loading a file
// checks the values the file holds, and a flag replaces one only after, while with
// no file nothing is loaded. values are those the run takes, the file's among them,
// which were checked already, so that what it finds came from a flag.
func checkFlagValues(values Config) error {
	return validateConfig(values, inNoFile)
}

// inNoFile is the inFile of values no file holds, which validateConfig then
// refuses with what is wrong alone.
func inNoFile(string) bool {
	return false
}

// repositoryKeyFinding is the finding of the repository key of config, nil when it
// holds: what stops a command that reads the repository the key gives.
func repositoryKeyFinding(config Config) error {
	findings := configKeyFindings(config)
	index := slices.IndexFunc(findings, func(finding Finding) bool { return finding.Subject == "repository" })
	if index < 0 {
		return nil
	}
	return findings[index]
}

// configFindings lists what can be told wrong from the config alone: its keys,
// and the text of its modules.
func configFindings(config Config) []Finding {
	return append(configKeyFindings(config), declarationFindings(config.Modules)...)
}

// configKeyFindings lists the keys of the config that hold what a run refuses,
// each whatever the others hold. Two keys that cannot go together name each
// other, since which of them goes is a choice.
func configKeyFindings(config Config) []Finding {
	const anotherValue = "give it another value with bumpit config set, or take it out with bumpit config unset"
	checks := []struct {
		key   string
		other string
		how   string
		err   error
	}{
		{key: "repository", how: anotherValue, err: validateRepositoryPath(config.Repository)},
		{key: "tagMatch", how: anotherValue, err: validateTagMatch(config.TagMatch)},
		{key: "output", how: anotherValue, err: validateOutputFormat(config.Output)},
		{key: "preRelease", how: anotherValue, err: validatePreRelease(config)},
		{key: "promote", other: "preRelease", how: "take out one of them with bumpit config unset", err: validatePromote(config)},
		{key: "startVersion", how: anotherValue, err: validateStartVersion(config)},
	}
	var findings []Finding
	for _, check := range checks {
		if check.err != nil {
			findings = append(findings, Finding{
				Rule:    ruleInvalidKey,
				Subject: check.key,
				Other:   check.other,
				What:    check.err.Error(),
				Why:     "a run that loads the file stops at it",
				How:     check.how,
			})
		}
	}
	return findings
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

func validatePreRelease(config Config) error {
	if strings.TrimSpace(config.PreRelease) == "" {
		return nil
	}
	if _, err := (semver.Version{}).WithPreRelease(config.PreRelease); err != nil {
		return fmt.Errorf("invalid preRelease: %w", err)
	}
	return nil
}

func validatePromote(config Config) error {
	if config.Promote != nil && *config.Promote && strings.TrimSpace(config.PreRelease) != "" {
		return fmt.Errorf("preRelease and promote cannot be used together")
	}
	return nil
}

// validateTagMatch refuses a tag pattern that starts with -: git would read it as
// an option, and the tags it could match are none git creates.
func validateTagMatch(tagMatch string) error {
	if strings.HasPrefix(tagMatch, "-") {
		return fmt.Errorf("tag pattern %q starts with -, which git reads as an option", tagMatch)
	}
	return nil
}

func validateStartVersion(config Config) error {
	if strings.TrimSpace(config.StartVersion) == "" {
		return nil
	}
	if _, err := versionAfterPrefix(config.StartVersion, tagPrefix(config.TagMatch)); err != nil {
		return fmt.Errorf("invalid startVersion: %w", err)
	}
	return nil
}
