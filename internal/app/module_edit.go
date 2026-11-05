package app

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// relative is the path given relative to --repo, relative to the top of
// the repository.
func (f *moduleEditFlags) relative(path string) (string, error) {
	return relativeToTop(f.top, f.repository, path)
}

func (f *moduleEditFlags) relativeAll(paths []string) ([]string, error) {
	relative := make([]string, 0, len(paths))
	for _, path := range paths {
		entry, err := f.relative(path)
		if err != nil {
			return nil, err
		}
		relative = append(relative, entry)
	}
	return relative, nil
}

// goDirectories lists the directories of the Go modules of the repository.
func (f *moduleEditFlags) goDirectories() ([]string, error) {
	goModules, err := f.snapshot.goModules()
	if err != nil {
		return nil, err
	}
	directories := make([]string, 0, len(goModules))
	for _, module := range goModules {
		directories = append(directories, module.Directory)
	}
	return directories, nil
}

// identities lists what a command line can call each module: the ones config
// declares first, in their order, then the Go modules no entry lists. It reads
// the go.mod files rather than the modules a run discovers, since an edit may
// be what repairs the config.
func (f *moduleEditFlags) identities(config *Config) ([]moduleIdentity, error) {
	goDirectories, err := f.goDirectories()
	if err != nil {
		return nil, err
	}
	identities := make([]moduleIdentity, 0, len(config.Modules)+len(goDirectories))
	for _, declaration := range config.Modules {
		identities = append(identities, moduleIdentity{name: declaration.Name, directories: declaration.Directories})
	}
	for _, directory := range goDirectories {
		if !listsDirectory(config.Modules, directory) {
			identities = append(identities, moduleIdentity{directories: []string{directory}})
		}
	}
	return identities, nil
}

// find is the index among identities of the module a command line calls
// written, -1 when it calls none.
func (f *moduleEditFlags) find(written string, identities []moduleIdentity) (int, error) {
	directory, err := f.relative(written)
	if err != nil {
		return -1, err
	}
	return moduleReference{written: written, directory: directory}.find(identities)
}

// declaredIndex is the index in config.Modules of the module a command line
// calls written.
func (f *moduleEditFlags) declaredIndex(written string, config *Config) (int, error) {
	identities, err := f.identities(config)
	if err != nil {
		return 0, err
	}
	index, err := f.find(written, identities)
	if err != nil {
		return 0, err
	}
	if index < 0 || index >= len(config.Modules) {
		return 0, fmt.Errorf("%s is not declared: declare it with bumpit modules add", written)
	}
	return index, nil
}

// carriers is what shippedBy calls the modules --shipped-by gives. One that is
// no module stays the path it says, which the proof of the edit then refuses.
func (f *moduleEditFlags) carriers(config *Config) ([]string, error) {
	identities, err := f.identities(config)
	if err != nil {
		return nil, err
	}
	carriers := make([]string, 0, len(f.shippedBy))
	for _, written := range f.shippedBy {
		index, err := f.find(written, identities)
		if err != nil {
			return nil, err
		}
		if index >= 0 {
			carriers = append(carriers, identities[index].shippedByName())
			continue
		}
		relative, err := f.relative(written)
		if err != nil {
			return nil, err
		}
		carriers = append(carriers, relative)
	}
	return carriers, nil
}

func (f *moduleEditFlags) add(command *cobra.Command, arguments []string, config *Config) (textEdit, error) {
	directories, err := f.relativeAll(arguments)
	if err != nil {
		return nil, err
	}
	if err := checkUndeclared(directories, config.Modules); err != nil {
		return nil, err
	}
	onGoModule, err := f.holdsGoModule(directories)
	if err != nil {
		return nil, err
	}
	declaration, err := f.newDeclaration(command, directories, onGoModule, config)
	if err != nil {
		return nil, err
	}

	f.subject = declaration.identity()
	f.unshipped = !onGoModule && !flagGiven(command, "shipped-by")
	var edits []textEdit
	if onGoModule && f.name != "" {
		if edits, err = f.followIdentity(config, directories[0], f.name); err != nil {
			return nil, err
		}
	}
	config.Modules = append(config.Modules, declaration)
	edits = append(edits, func(content []byte) ([]byte, error) { return addModuleText(content, declaration) })
	return chainEdits(edits), nil
}

// checkUndeclared refuses a directory that declarations lists already.
func checkUndeclared(directories []string, declarations []ModuleDeclaration) error {
	for _, directory := range directories {
		if listsDirectory(declarations, directory) {
			return fmt.Errorf("%s is declared already: change it with bumpit modules set", directory)
		}
	}
	return nil
}

// holdsGoModule reports whether one of directories is the directory of a Go
// module.
func (f *moduleEditFlags) holdsGoModule(directories []string) (bool, error) {
	goDirectories, err := f.goDirectories()
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(directories, func(directory string) bool { return slices.Contains(goDirectories, directory) }), nil
}

// newDeclaration is the declaration the flags give the directories. One on a
// Go module says nothing of its carriers unless the command line does, which
// the proof of the edit then refuses.
func (f *moduleEditFlags) newDeclaration(command *cobra.Command, directories []string, onGoModule bool, config *Config) (ModuleDeclaration, error) {
	declaration := ModuleDeclaration{Name: f.name, Directories: directories, TagPrefix: f.tagPrefix}
	if err := f.applyPaths(&declaration); err != nil {
		return ModuleDeclaration{}, err
	}
	if onGoModule && !flagGiven(command, "shipped-by") {
		return declaration, nil
	}
	carriers, err := f.carriers(config)
	if err != nil {
		return ModuleDeclaration{}, err
	}
	declaration.ShippedBy = &carriers
	return declaration, nil
}

func (f *moduleEditFlags) set(command *cobra.Command, written string, config *Config) (textEdit, error) {
	index, err := f.declaredIndex(written, config)
	if err != nil {
		return nil, err
	}
	old := config.Modules[index]
	onGoModule, err := f.holdsGoModule(old.Directories)
	if err != nil {
		return nil, err
	}
	if onGoModule && flagGiven(command, "directory") {
		return nil, fmt.Errorf("%s: the directory of a Go module is where its go.mod is", written)
	}

	declaration := old
	if err := f.applyChanges(command, &declaration, config); err != nil {
		return nil, err
	}
	if !onGoModule && !flagGiven(command, "tag-prefix") {
		if err := keepPrefix(old, &declaration); err != nil {
			return nil, fmt.Errorf("%s: %w", written, err)
		}
	}

	f.subject, f.formerSubject = declaration.identity(), old.identity()
	edits, err := f.followIdentity(config, old.identity(), declaration.identity())
	if err != nil {
		return nil, err
	}
	config.Modules[index] = declaration
	edits = append(edits, func(content []byte) ([]byte, error) { return setModuleText(content, index, declaration) })
	return chainEdits(edits), nil
}

// applyChanges gives the declaration what the flags on the command line say,
// and leaves the rest.
func (f *moduleEditFlags) applyChanges(command *cobra.Command, declaration *ModuleDeclaration, config *Config) error {
	if flagGiven(command, "name") {
		declaration.Name = f.name
	}
	if flagGiven(command, "tag-prefix") {
		declaration.TagPrefix = f.tagPrefix
	}
	return f.applyListChanges(command, declaration, config)
}

// applyListChanges gives the declaration the lists the flags on the command
// line give.
func (f *moduleEditFlags) applyListChanges(command *cobra.Command, declaration *ModuleDeclaration, config *Config) error {
	if flagGiven(command, "directory") {
		directories, err := f.relativeAll(f.directories)
		if err != nil {
			return err
		}
		declaration.Directories = directories
	}
	if flagGiven(command, "path") || flagGiven(command, "no-paths") {
		if err := f.applyPaths(declaration); err != nil {
			return err
		}
	}
	if flagGiven(command, "shipped-by") || flagGiven(command, "shipped-by-none") {
		carriers, err := f.carriers(config)
		if err != nil {
			return err
		}
		declaration.ShippedBy = &carriers
	}
	return nil
}

// keepPrefix writes down the tag prefix old had when declaration would take
// another from its name or its directories: the tags of a module are its
// history, and neither a new name nor a move cuts it from them. An old
// declaration that could not load has no prefix to keep. The prefix v is the
// root's alone, so that a module cannot keep it out of the root.
func keepPrefix(old ModuleDeclaration, declaration *ModuleDeclaration) error {
	if len(nameFindingsOf(old)) > 0 || len(old.Directories) == 0 || declaration.prefix() == old.prefix() {
		return nil
	}
	if old.prefix() == "v" && !slices.Contains(declaration.Directories, ".") {
		return errors.New("the tag prefix v belongs to the module in ., which this one would leave: give it another with --tag-prefix, which leaves its tags behind")
	}
	declaration.TagPrefix = old.prefix()
	return nil
}

func (f *moduleEditFlags) remove(written string, config *Config) (textEdit, error) {
	index, err := f.declaredIndex(written, config)
	if err != nil {
		return nil, err
	}
	old := config.Modules[index]
	onGoModule, err := f.holdsGoModule(old.Directories)
	if err != nil {
		return nil, err
	}

	f.subject, f.formerSubject = old.identity(), old.identity()
	var edits []textEdit
	if onGoModule && old.Name != "" && len(old.Directories) == 1 {
		if edits, err = f.followIdentity(config, old.Name, old.Directories[0]); err != nil {
			return nil, err
		}
	}
	config.Modules = slices.Delete(config.Modules, index, index+1)
	edits = append(edits, func(content []byte) ([]byte, error) { return removeModuleText(content, index) })
	return chainEdits(edits), nil
}

// applyPaths gives the declaration the paths of --path, none when it gives
// none, as a config that leaves out paths loads.
func (f *moduleEditFlags) applyPaths(declaration *ModuleDeclaration) error {
	declaration.Paths = nil
	if len(f.paths) == 0 {
		return nil
	}
	paths, err := f.relativeAll(f.paths)
	declaration.Paths = paths
	return err
}

// followIdentity returns the text edits that make the shippedBy lists follow a
// module from old to what it is called now, current. A name another module also
// answers to stays: the file cannot say which of them it meant, and it still calls
// a module, so that the command says which entries keep it rather than guess.
func (f *moduleEditFlags) followIdentity(config *Config, old, current string) ([]textEdit, error) {
	if old == current {
		return nil, nil
	}
	identities, err := f.identities(config)
	if err != nil {
		return nil, err
	}
	answering := 0
	for _, identity := range identities {
		if identity.shippedByName() == old {
			answering++
		}
	}
	if answering < 2 {
		return rewriteCarriers(config.Modules, old, current), nil
	}
	f.noteKeptCarrier(config.Modules, old, current)
	return nil, nil
}

// noteKeptCarrier writes down what the command says of the entries that keep old
// in shippedBy, when there are any.
func (f *moduleEditFlags) noteKeptCarrier(declarations []ModuleDeclaration, old, current string) {
	var entries []string
	for _, declaration := range declarations {
		if declaration.ShippedBy != nil && slices.Contains(*declaration.ShippedBy, old) {
			entries = append(entries, declaration.identity())
		}
	}
	if len(entries) > 0 {
		f.keptCarrier = fmt.Sprintf("the shippedBy of %s keeps %s, since another module is called %s too: if it meant this one, give %s there with bumpit modules set -b", strings.Join(entries, ", "), old, old, current)
	}
}

// rewriteCarriers makes every shippedBy that calls a module old call it
// current, and returns the text edits that write it: each changes that key
// alone, so that the rest of an entry nobody asked to change stays.
func rewriteCarriers(declarations []ModuleDeclaration, old, current string) []textEdit {
	var edits []textEdit
	for index, declaration := range declarations {
		if declaration.ShippedBy == nil || !slices.Contains(*declaration.ShippedBy, old) {
			continue
		}
		carriers := slices.Clone(*declaration.ShippedBy)
		for position, carrier := range carriers {
			if carrier == old {
				carriers[position] = current
			}
		}
		declarations[index].ShippedBy = &carriers
		edits = append(edits, func(content []byte) ([]byte, error) {
			edited, err := setCarriersText(content, index, carriers)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", declaration.identity(), err)
			}
			return edited, nil
		})
	}
	return edits
}

// chainEdits is the text edit that applies edits in order.
func chainEdits(edits []textEdit) textEdit {
	return func(content []byte) ([]byte, error) {
		for _, edit := range edits {
			edited, err := edit(content)
			if err != nil {
				return nil, err
			}
			content = edited
		}
		return content, nil
	}
}
