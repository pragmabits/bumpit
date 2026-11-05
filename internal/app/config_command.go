package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: "Read and change the keys of bumpit.yaml",
		Long:  "bumpit config reads and changes the keys of the config file: the one --config names, else the bumpit.yaml or .bumpit.yaml in --repo. Keys are compared without case. The modules are changed with bumpit modules add, set and remove.",
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.Args = cobra.NoArgs
	command.AddCommand(newConfigGetCmd(), newConfigSetCmd(), newConfigUnsetCmd())
	return command
}

func newConfigGetCmd() *cobra.Command {
	flags := configEditFlags{}
	command := &cobra.Command{
		Use:   "get <key>",
		Short: "Print the value the config file gives a key",
		Long:  "bumpit config get prints the value the config file gives the key, as the file writes it, an empty line for a key written with no value, and fails when the file does not hold the key.",
		RunE: func(command *cobra.Command, arguments []string) error {
			key, err := findConfigKey(arguments[0])
			if err != nil {
				return err
			}
			path, err := flags.file(false)
			if err != nil {
				return err
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value, found, err := configKeyValue(content, key.name)
			switch {
			case err != nil:
				return fmt.Errorf("read %s: %w", path, err)
			case !found:
				return fmt.Errorf("%s is not set in %s", key.name, path)
			}
			_, err = fmt.Fprintln(command.OutOrStdout(), value)
			return err
		},
	}
	command.Args = cobra.ExactArgs(1)
	flags.bind(command)
	return command
}

func newConfigSetCmd() *cobra.Command {
	flags := configEditFlags{}
	command := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Give a key of the config file a value",
		Long:  "bumpit config set gives the key the value, in place of the one the file gives it, keeping the comment after it, or as a new key at the end of the file. It creates bumpit.yaml in --repo when neither --config nor --repo names a config file. The rest of the file is left as it is, and the file is written only once the new config loads.",
		RunE: func(command *cobra.Command, arguments []string) error {
			key, err := findConfigKey(arguments[0])
			if err != nil {
				return err
			}
			text, err := key.valueText(arguments[1])
			if err != nil {
				return err
			}
			path, err := flags.file(true)
			if err != nil {
				return err
			}
			edit := configEdit{
				path:      path,
				create:    true,
				rehearsal: rehearsalOf(command),
				subjects:  key.subjects,
				change: func(want *Config, empty Config) (textEdit, error) {
					key.set(want, arguments[1], empty.Repository)
					return func(content []byte) ([]byte, error) { return setConfigKeyText(content, key.name, text) }, nil
				},
			}
			if err := edit.apply(); err != nil {
				return err
			}
			return printEdit(command.OutOrStdout(), fmt.Sprintf("set %s in %s", key.name, path))
		},
	}
	command.Args = cobra.ExactArgs(2)
	flags.bind(command)
	return command
}

func newConfigUnsetCmd() *cobra.Command {
	flags := configEditFlags{}
	command := &cobra.Command{
		Use:   "unset <key>",
		Short: "Remove a key from the config file",
		Long:  "bumpit config unset removes the key and its value from the config file, with the comment on its line, and fails when the file does not hold the key.",
		RunE: func(command *cobra.Command, arguments []string) error {
			key, err := findConfigKey(arguments[0])
			if err != nil {
				return err
			}
			path, err := flags.file(false)
			if err != nil {
				return err
			}
			edit := configEdit{
				path:      path,
				rehearsal: rehearsalOf(command),
				subjects:  key.subjects,
				change: func(want *Config, empty Config) (textEdit, error) {
					key.reset(want, empty)
					return func(content []byte) ([]byte, error) { return unsetConfigKeyText(content, key.name) }, nil
				},
			}
			err = edit.apply()
			if errors.Is(err, errConfigKeyNotSet) {
				return fmt.Errorf("%s is not set in %s", key.name, path)
			}
			if err != nil {
				return err
			}
			return printEdit(command.OutOrStdout(), fmt.Sprintf("unset %s in %s", key.name, path))
		},
	}
	command.Args = cobra.ExactArgs(1)
	flags.bind(command)
	return command
}

type configEditFlags struct {
	configPath string
	repository string
}

func (f *configEditFlags) bind(command *cobra.Command) {
	bindConfigFlag(command, &f.configPath)
	bindRepositoryFlag(command, &f.repository)
}

// file is the absolute path of the config file the command edits: the one
// --config names, else the bumpit.yaml or .bumpit.yaml in --repo, else, when
// create allows it, a new bumpit.yaml there.
func (f *configEditFlags) file(create bool) (string, error) {
	path, err := f.find(create)
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

func (f *configEditFlags) find(create bool) (string, error) {
	if f.configPath != "" {
		_, err := os.Stat(f.configPath)
		return f.configPath, err
	}
	path, err := findConfig(f.repository)
	switch {
	case errors.Is(err, ErrConfigNotFound) && create:
		return configCandidatePaths(f.repository)[0], nil
	case errors.Is(err, ErrConfigNotFound):
		return "", fmt.Errorf("%s holds no bumpit.yaml or .bumpit.yaml", f.repository)
	}
	return path, err
}

// configKey is a key of bumpit.yaml that holds a single value: its name and
// the field of Config that holds it.
type configKey struct {
	name  string
	field int
	// isSwitch reports a key that is true or false, which the config may
	// also leave unset.
	isSwitch bool
}

// configKeys are the keys of bumpit.yaml that hold a single value, as Config
// names them. The modules, a list, are edited by bumpit modules.
var configKeys = func() []configKey {
	configType := reflect.TypeFor[Config]()
	var keys []configKey
	for index := range configType.NumField() {
		field := configType.Field(index)
		if field.Type.Kind() != reflect.Slice {
			keys = append(keys, configKey{name: field.Tag.Get("mapstructure"), field: index, isSwitch: field.Type.Kind() == reflect.Pointer})
		}
	}
	return keys
}()

// findConfigKey is the key of bumpit.yaml that name names, compared without
// case, as the config is read.
func findConfigKey(name string) (configKey, error) {
	var names []string
	for _, key := range configKeys {
		if strings.EqualFold(key.name, name) {
			return key, nil
		}
		names = append(names, key.name)
	}
	if strings.EqualFold(name, "modules") {
		return configKey{}, fmt.Errorf("modules are changed with bumpit modules add, set and remove")
	}
	return configKey{}, fmt.Errorf("unknown key %s: bumpit.yaml takes %s", name, strings.Join(names, ", "))
}

// subjects are what an edit of the key is about: the key, called the same before
// and after it.
func (k configKey) subjects() []editSubject {
	return []editSubject{{current: k.name, former: k.name}}
}

// valueText is value as the YAML text the key takes: true or false for a
// switch, a string for the others, quoted when YAML would read it as
// something else.
func (k configKey) valueText(value string) (string, error) {
	if k.isSwitch {
		if value != "true" && value != "false" {
			return "", fmt.Errorf("%s takes true or false", k.name)
		}
		return value, nil
	}
	lines, err := encodeYAML(yamlString(value))
	if err != nil {
		return "", err
	}
	return strings.Join(lines, " "), nil
}

// set gives the field of config that holds the key the value, as the config
// loads it: a repository resolved against directory, the one of the config
// file, which a config with no key gives as its repository.
func (k configKey) set(config *Config, value, directory string) {
	field := reflect.ValueOf(config).Elem().Field(k.field)
	switch {
	case k.isSwitch:
		switched := value == "true"
		field.Set(reflect.ValueOf(&switched))
	case k.name == "repository":
		field.SetString(resolveRepositoryPath(filepath.Join(directory, "bumpit.yaml"), value))
	default:
		field.SetString(value)
	}
}

// reset gives the field of config that holds the key the value of empty,
// as a config that leaves the key out loads.
func (k configKey) reset(config *Config, empty Config) {
	reflect.ValueOf(config).Elem().Field(k.field).Set(reflect.ValueOf(empty).Field(k.field))
}
