package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-viper/mapstructure/v2"
	"github.com/pragmabits/bumpit/internal/gitx"
	"github.com/spf13/viper"
)

// moduleSource says which config gives the modules of a run: the one given with
// --config when it declares modules, an empty list included, and else the one at
// the top of the repository, wherever --repo points. The declarations describe the
// repository, as its go.mod files do, so a run from a subdirectory reads them too,
// and a config found in a subdirectory cannot declare others.
type moduleSource struct {
	// top is the top of the repository, empty outside one, and topConfig the
	// config there, empty when it has none.
	top       string
	topConfig string
	// given reports a --config that declares modules.
	given bool
	// atTop reports that the config of the run is the one at the top.
	atTop bool
	// belowTop reports a config of the run that declares modules bumpit does not
	// read, below the top of the repository.
	belowTop bool
}

// findModuleSource finds where the modules of a run come from. config is the
// config the run loaded, at configPath, nil when it loaded none, and explicit
// reports that --config named it. It fills what it can when the config at the
// top cannot be found out, which only a --config that declares modules can do
// without.
func findModuleSource(explicit bool, findTop func() (string, error), config *Config, configPath string) (moduleSource, error) {
	source := moduleSource{given: explicit && config != nil && config.Modules != nil}
	var err error
	if source.top, source.topConfig, err = findTopConfig(findTop); err != nil {
		return source, err
	}
	source.atTop = config != nil && sameFile(configPath, source.topConfig)
	source.belowTop = source.top != "" && config != nil && len(config.Modules) > 0 && !source.fromConfig()
	return source, nil
}

// errModulesBelowTop is what stops a run that loads a config below the top of the
// repository that declares modules.
func errModulesBelowTop(configPath, top string) error {
	return fmt.Errorf("%s declares modules, which bumpit reads from the bumpit.yaml at the top of the repository: move them to %s", configPath, filepath.Join(top, "bumpit.yaml"))
}

// errNoRepository is what a command that needs the top of the repository says
// outside one.
func errNoRepository(path string) error {
	return fmt.Errorf("%s is in no git repository, whose modules bumpit.yaml would declare", path)
}

// fromConfig reports that the modules come from the config of the run, which it
// holds already.
func (s moduleSource) fromConfig() bool {
	return s.given || s.atTop
}

// readDeclarations is the modules a run reads, and refuses a config below the top
// that declares others. Outside a repository there is none, and the command
// reports that repository is none.
func readDeclarations(explicit bool, repository string, config *Config, configPath string) ([]ModuleDeclaration, error) {
	source, err := findModuleSource(explicit, gitx.New(repository).Top, config, configPath)
	switch {
	case source.given:
		return config.Modules, nil
	case err != nil || source.top == "":
		return nil, err
	case source.belowTop:
		return nil, errModulesBelowTop(configPath, source.top)
	case source.atTop:
		return config.Modules, nil
	}
	return topDeclarations(source.topConfig)
}

// findTopConfig is the top of the repository findTop finds and the config there,
// empty when it has none. Outside a repository both are empty: the command
// reports that repository is none.
func findTopConfig(findTop func() (string, error)) (string, string, error) {
	top, err := findTop()
	if err != nil {
		return "", "", nil
	}
	topConfig, err := findConfig(top)
	if errors.Is(err, ErrConfigNotFound) {
		return top, "", nil
	}
	return top, topConfig, err
}

// topDeclarations reads the modules the config at topConfig declares, a config
// the run did not load. It reads and checks the modules key alone: the other keys
// are not the run's, so a value one of them could not take does not stop it.
func topDeclarations(topConfig string) ([]ModuleDeclaration, error) {
	if topConfig == "" {
		return nil, nil
	}
	declarations, _, err := readTopModules(topConfig)
	if err == nil {
		err = validateModuleDeclarations(declarations)
	}
	if err != nil {
		return nil, fmt.Errorf("read the modules of %s: %w", topConfig, err)
	}
	return declarations, nil
}

// readTopModules reads the modules key and the tag pattern of the config at
// topConfig alone, and does not validate them.
func readTopModules(topConfig string) ([]ModuleDeclaration, string, error) {
	configReader := viper.New()
	configReader.SetConfigFile(topConfig)
	if err := configReader.ReadInConfig(); err != nil {
		return nil, "", err
	}
	var declarations []ModuleDeclaration
	err := configReader.UnmarshalKey("modules", &declarations, func(decoder *mapstructure.DecoderConfig) { decoder.ErrorUnused = true })
	return declarations, configReader.GetString("tagMatch"), err
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
