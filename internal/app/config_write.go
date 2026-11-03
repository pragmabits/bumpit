package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
)

// textEdit is the new text of a config file, made from its current text.
type textEdit func(content []byte) ([]byte, error)

// configEdit is one edit of a config file, which bumpit writes only once it
// has proved it: the new text loads, valid, as the config the file loaded as
// with the edit applied, and nothing else changed.
type configEdit struct {
	// path is the config file as the command names it, maybe a symbolic
	// link, whose target the edit writes.
	path string
	// create allows a file that does not exist yet, which the edit creates.
	create bool
	// change applies the edit to want, the config the file loads as, given
	// empty, the config a file with no key loads as, and returns the text
	// edit that writes it.
	change func(want *Config, empty Config) (textEdit, error)
	// check, when set, proves the new config further.
	check func(edited Config) error
}

// configEditDefaults are the defaults the proof loads a config with, those
// that a valid config may leave to the command.
var configEditDefaults = Config{Repository: ".", Output: "text"}

// apply proves the edit and writes it over the file at once, by renaming the
// proved text onto it, keeping its mode and, for a link, the link. The file
// is left as it was when the proof fails. The current text is read without
// validation, so that an edit can repair a value the config cannot take.
func (e configEdit) apply() error {
	target, mode, content, err := readConfigFile(e.path, e.create)
	if err != nil {
		return err
	}
	// The runs resolve the repository from the path they name, a link
	// included, so the proof does too.
	absolutePath, err := filepath.Abs(e.path)
	if err != nil {
		return err
	}

	want, err := decodeConfigContent(content, configEditDefaults, absolutePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", e.path, err)
	}
	empty := configEditDefaults
	empty.Repository = resolveRepositoryPath(absolutePath, empty.Repository)
	editText, err := e.change(&want, empty)
	if err != nil {
		return err
	}
	edited, err := editText(content)
	if err != nil {
		return fmt.Errorf("edit %s: %w", e.path, err)
	}
	if err := e.prove(edited, absolutePath, want); err != nil {
		return err
	}
	return replaceFile(target, edited, mode)
}

// prove loads edited as the config at absolutePath and compares it with
// want, an empty list of paths the same as none, since a declaration
// written anew leaves it out.
func (e configEdit) prove(edited []byte, absolutePath string, want Config) error {
	got, err := decodeConfigContent(edited, configEditDefaults, absolutePath)
	if err == nil {
		err = validateConfig(got)
	}
	if err != nil {
		return fmt.Errorf("the edited %s would not load, so it is left as it was: %w", e.path, err)
	}
	if !reflect.DeepEqual(withoutEmptyPaths(got), withoutEmptyPaths(want)) {
		return fmt.Errorf("the edited %s would not load as the edit means, so it is left as it was: edit it by hand", e.path)
	}
	if e.check == nil {
		return nil
	}
	if err := e.check(got); err != nil {
		return fmt.Errorf("the edited %s is refused, so it is left as it was: %w", e.path, err)
	}
	return nil
}

// withoutEmptyPaths is config with every empty list of paths made nil, which
// it means as well. An empty shippedBy is kept: it says no module carries
// the declared one, while nil says nothing.
func withoutEmptyPaths(config Config) Config {
	if config.Modules == nil {
		return config
	}
	config.Modules = slices.Clone(config.Modules)
	for index := range config.Modules {
		if len(config.Modules[index].Paths) == 0 {
			config.Modules[index].Paths = nil
		}
	}
	return config
}

// readConfigFile is the file a link at path points to, or path itself, its
// mode and its content: no content and the mode of a new file when it does
// not exist and create allows that.
func readConfigFile(path string, create bool) (string, fs.FileMode, []byte, error) {
	target, err := filepath.EvalSymlinks(path)
	if errors.Is(err, os.ErrNotExist) && create {
		target, err = pendingTarget(path)
		return target, 0o644, nil, err
	}
	if err != nil {
		return "", 0, nil, err
	}

	info, err := os.Stat(target)
	if err != nil {
		return "", 0, nil, err
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return "", 0, nil, err
	}
	return target, info.Mode().Perm(), content, nil
}

// pendingTarget is the file that does not exist yet at path, or that the
// links from path lead to, so that creating it keeps the links.
func pendingTarget(path string) (string, error) {
	for range 40 {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) || err == nil && info.Mode()&os.ModeSymlink == 0 {
			return path, nil
		}
		if err != nil {
			return "", err
		}
		link, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(path), link)
		}
		path = link
	}
	return "", fmt.Errorf("%s: too many links", path)
}

// replaceFile writes content over target at once: to a new hidden file
// next to it, with mode, renamed onto it.
func replaceFile(target string, content []byte, mode fs.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(target), ".bumpit-edit-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())

	_, writeErr := file.Write(content)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr, os.Chmod(file.Name(), mode)); err != nil {
		return err
	}
	return os.Rename(file.Name(), target)
}
