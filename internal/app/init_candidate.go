package app

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/pragmabits/bumpit/internal/helm"
)

// initialConfig is the text of the bumpit.yaml bumpit init creates, to which
// it adds the Go modules of the repository.
const initialConfig = "# bumpit reads this file at the top of the repository. Declare the modules\n# that have no go.mod with bumpit modules add, and set the other keys with\n# bumpit config set. bumpit init lists the Go modules here, which also work\n# when no entry lists them.\n"

// InitOptions are the options of bumpit init.
type InitOptions struct {
	// Repository is the repository init reads, the one the config in --repo
	// gives when the command line gives none.
	Repository string
	// RepositoryGiven is the --repo of the command line, empty when it gives
	// none, which the commands init proposes carry.
	RepositoryGiven string
	// Config is the config in --repo, at ConfigPath, nil when it has none: a run
	// from there reads the tags of its pattern as its own, and stops at modules
	// it declares below the top of the repository.
	Config     *Config
	ConfigPath string
	// rehearsal is the rehearsal init runs in, in which it writes nothing, nil
	// when it runs for real.
	rehearsal *rehearsal
}

// InitReport is what bumpit init did and what it proposes: the config file,
// whether it created it, and the parts of the repository that look released
// on their own while no module covers them.
type InitReport struct {
	Config  string `json:"config"`
	Created bool   `json:"created"`
	// Declared are the directories of the Go modules init listed in the
	// config, which no entry listed, as the config writes them: relative to
	// the top of the repository, wherever --repo points.
	Declared []string `json:"declared"`
	// Unlisted are those it could not list, and UnlistedReason says why.
	Unlisted       []string `json:"unlisted,omitempty"`
	UnlistedReason string   `json:"unlisted_reason,omitempty"`
	// Unnameable are the Go modules in a directory no entry can name, each with
	// why.
	Unnameable []LeftOutDirectory  `json:"unnameable,omitempty"`
	TagLines   []TagLine           `json:"tag_lines"`
	Manifests  []ManifestCandidate `json:"manifests"`
	// Unreadable are the manifests that could have been candidates but
	// could not be read.
	Unreadable []UnreadableManifest `json:"unreadable,omitempty"`
}

// ManifestCandidate is a manifest that gives its directory a version of its
// own, a Helm chart or a public Node.js package, in a directory that is no
// module.
type ManifestCandidate struct {
	// Manifest is relative to the repository init was given.
	Manifest  string    `json:"manifest"`
	Version   string    `json:"version"`
	Candidate Candidate `json:"candidate"`
}

// UnreadableManifest is a manifest init could not read, and why.
type UnreadableManifest struct {
	Manifest string `json:"manifest"`
	Error    string `json:"error"`
}

// Init creates bumpit.yaml at the top of the repository when it has neither
// bumpit.yaml nor .bumpit.yaml, lists in it the Go modules no entry lists, and
// lists the candidate modules: the lines of version tags that no module owns,
// with the directories they may be the releases of, and the manifests with a
// version of their own. It declares nothing else, and stops on a config that
// holds a problem, over which it could promise nothing. Each proposal is
// rehearsed over what those before it and the Go modules would write, and the
// file is written last, so that nothing is written when they cannot be made up.
func Init(opts InitOptions) (InitReport, error) {
	finder, topConfig, err := newCandidateFinder(opts)
	if err != nil {
		return InitReport{}, err
	}

	report := InitReport{Config: topConfig, Declared: []string{}}
	if topConfig == "" {
		report.Config, report.Created = filepath.Join(finder.top, "bumpit.yaml"), true
		target, err := pendingTarget(report.Config)
		if err != nil {
			return InitReport{}, err
		}
		finder.rehearsal.files[target] = []byte(initialConfig)
	}
	directories, unnameable := splitNameable(finder.unlistedGoModules())
	report.Unnameable = unnameable
	listing := report.listGoModules(finder, directories, finder.rehearsal)

	if report.TagLines, err = finder.tagLines(); err != nil {
		return InitReport{}, err
	}
	if report.Manifests, report.Unreadable, err = finder.manifests(); err != nil {
		return InitReport{}, err
	}
	if opts.rehearsal != nil {
		return report, nil
	}
	if err := report.writeConfig(finder, directories, listing); err != nil {
		return InitReport{}, err
	}
	return report, nil
}

// writeConfig writes what the rehearsal proved: the new file, and the Go modules
// unless listing says why they cannot be listed. A file the edit cannot change
// stops nothing else init does, and the report says why.
func (r *InitReport) writeConfig(finder *candidateFinder, directories []string, listing error) error {
	if r.Created {
		if err := createConfig(r.Config); err != nil {
			return err
		}
	}
	if listing == nil {
		listing = r.listGoModules(finder, directories, nil)
	}
	switch {
	case listing != nil:
		r.Unlisted, r.UnlistedReason = directories, listing.Error()
	case len(directories) > 0:
		r.Declared = directories
	}
	return nil
}

// splitNameable keeps the directories of Go modules an entry can name, and says why
// of the others.
func splitNameable(directories []string) ([]string, []LeftOutDirectory) {
	var nameable []string
	var unnameable []LeftOutDirectory
	for _, directory := range directories {
		if reason := whyNoEntryNames(directory); reason != "" {
			unnameable = append(unnameable, LeftOutDirectory{Directory: directory, Reason: reason})
			continue
		}
		nameable = append(nameable, directory)
	}
	return nameable, unnameable
}

// whyNoEntryNames says what keeps an entry of the file from naming the directory of
// a Go module, empty when nothing does. It leaves out how the finding would resolve
// it, which is to write another path: the directory is where its go.mod is.
func whyNoEntryNames(directory string) string {
	findings := modulePathFindings("", "directory", directory)
	if len(findings) == 0 {
		return ""
	}
	return findings[0].What + ": " + findings[0].Why
}

// listGoModules adds to the config, in one edit, an entry for each of directories,
// the Go modules no entry lists. It writes nothing when given a rehearsal, which
// keeps the edit.
func (r *InitReport) listGoModules(finder *candidateFinder, directories []string, series *rehearsal) error {
	if len(directories) == 0 {
		return nil
	}
	edit := configEdit{
		path:      r.Config,
		create:    true,
		rehearsal: series,
		change: func(want *Config, _ Config) (textEdit, error) {
			edits := make([]textEdit, 0, len(directories))
			for _, directory := range directories {
				declaration := ModuleDeclaration{Directories: []string{directory}}
				want.Modules = append(want.Modules, declaration)
				edits = append(edits, func(content []byte) ([]byte, error) { return addModuleText(content, declaration) })
			}
			return chainEdits(edits), nil
		},
		subjects: func() []editSubject {
			subjects := make([]editSubject, 0, len(directories))
			for _, directory := range directories {
				subjects = append(subjects, editSubject{current: directory})
			}
			return subjects
		},
		repository: func(modules []ModuleDeclaration) ([]Finding, error) {
			report, err := repositoryFindings(finder.snapshot, modules)
			return report.findings, err
		},
	}
	return edit.apply()
}

// createConfig writes initialConfig to configPath, which must not exist, or to the
// file that does not exist yet that the links from configPath lead to.
func createConfig(configPath string) error {
	target, err := pendingTarget(configPath)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(initialConfig)
	return errors.Join(writeErr, file.Close())
}

// candidateFinder finds the candidate modules of the repository at top,
// which init was given as repository.
type candidateFinder struct {
	// snapshot is what it reads of the repository.
	snapshot *repositorySnapshot
	// repository is the repository init reads.
	repository string
	// context is what the commands it proposes carry.
	context commandContext
	// base is the absolute path of repository, links resolved, which the
	// paths init prints are relative to.
	base string
	top  string
	// declarations are the modules the config at the top lists.
	declarations []ModuleDeclaration
	tracked      trackedTree
	// modules are the modules of the repository, and the candidates accepted
	// so far: each command runs after those printed before it.
	modules []repositoryModule
	// tagMatches are the tag patterns of the config at the top and of the
	// config of the run, whose tags a run with no module reads.
	tagMatches []string
	// rehearsal runs the commands it proposes, in the order they are printed.
	rehearsal *rehearsal
}

// newCandidateFinder reads the modules of the repository, those the config
// at its top declares included, and returns that config, empty when the
// repository has none.
func newCandidateFinder(opts InitOptions) (*candidateFinder, string, error) {
	series := opts.rehearsal
	if series == nil {
		series = newRehearsal()
	}
	snapshot := series.snapshotAt(opts.Repository)
	top, err := snapshot.top()
	if err != nil {
		return nil, "", fmt.Errorf("%s is in no git repository: %w", opts.Repository, err)
	}
	finder := &candidateFinder{snapshot: snapshot, repository: opts.Repository, top: top, rehearsal: series}
	finder.context = commandContext{repositoryGiven: opts.RepositoryGiven, effective: opts.Repository}
	if opts.Config != nil {
		finder.tagMatches = []string{opts.Config.TagMatch}
	}
	topConfig, err := finder.readTopConfig(opts)
	if err != nil {
		return nil, "", err
	}
	if err := finder.readBase(); err != nil {
		return nil, "", err
	}
	return finder, topConfig, nil
}

// readTopConfig reads the config at the top of the repository, which must
// hold no problem, its tag pattern and its modules, and the modules of the
// repository, and returns the path of that config, empty when there is none. A
// config in --repo that declares modules stops it, as it stops a run from there.
func (f *candidateFinder) readTopConfig(opts InitOptions) (string, error) {
	source, err := findModuleSource(false, f.snapshot.top, opts.Config, opts.ConfigPath)
	if err != nil {
		return "", err
	}
	if source.belowTop {
		return "", pointAtCheck(errModulesBelowTop(opts.ConfigPath, source.top), f.context)
	}
	if source.topConfig != "" {
		config, _, err := LoadConfig(source.topConfig, configEditDefaults)
		if err != nil {
			return "", pointAtCheck(err, f.checkOfTopConfig(source))
		}
		f.tagMatches, f.declarations = append(f.tagMatches, config.TagMatch), config.Modules
	}

	if f.tracked, err = f.snapshot.tracked(); err != nil {
		return "", err
	}
	if f.modules, _, err = discoverRepositoryModulesIn(f.snapshot, f.declarations); err != nil {
		if _, isFinding := errors.AsType[Finding](err); isFinding {
			err = pointAtCheck(err, f.context)
		}
		return "", err
	}
	return source.topConfig, nil
}

// checkOfTopConfig is where a check lists what is wrong with the config at the top:
// from below the top it reads the modules of that file and no other key.
func (f *candidateFinder) checkOfTopConfig(source moduleSource) commandContext {
	if source.atTop {
		return f.context
	}
	return commandContext{repositoryGiven: source.top}
}

// pointAtCheck adds to a problem of the file that stops init the command that
// lists them all. A failure to read the repository is none: the command would
// stop at it too.
func pointAtCheck(err error, context commandContext) error {
	words := append([]string{"modules", "check"}, context.repositoryFlags()...)
	return fmt.Errorf("%w; %s lists all there is to resolve", err, commandText(words))
}

func (f *candidateFinder) readBase() error {
	base, err := filepath.Abs(f.repository)
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}
	f.base = base
	return nil
}

// unlistedGoModules lists the directories of the Go modules no entry of the
// config lists, in the order of the modules.
func (f *candidateFinder) unlistedGoModules() []string {
	var directories []string
	for _, module := range f.modules {
		if !module.withoutGoMod() && !listsDirectory(f.declarations, module.directories[0]) {
			directories = append(directories, module.directories[0])
		}
	}
	return directories
}

// tagLines lists the tag lines no module owns, each with the one candidate it
// may be the releases of. The tags of a run with no module are owned whatever
// the modules: those of the tag patterns of the configs, and those of v.
func (f *candidateFinder) tagLines() ([]TagLine, error) {
	tags, err := f.snapshot.client.Tags("")
	if err != nil {
		return nil, err
	}
	prefixes := []string{"v"}
	for _, tagMatch := range f.tagMatches {
		if tagMatch != "" {
			prefixes = append(prefixes, tagPrefix(tagMatch))
		}
	}
	for _, module := range f.modules {
		prefixes = append(prefixes, module.tagPrefix)
	}

	lines := unownedTagLines(tags, prefixes)
	for index := range lines {
		guesses := guessDirectories(lines[index].Prefix, f.tracked, f.modules)
		if len(guesses) == 0 {
			lines[index].Command, lines[index].Refused = f.giveToModule(lines[index].Prefix, tags)
			continue
		}
		candidate, err := f.lineCandidate(lines[index].Prefix, guesses)
		if err != nil {
			return nil, err
		}
		lines[index].Candidate = &candidate
	}
	return lines, nil
}

// giveToModule is the command that gives a tag line to the module that left it
// behind, as one does when its go.mod goes and its name gives it another prefix,
// which it rehearses, or why that command would be refused. Both are empty when no
// module left it behind.
func (f *candidateFinder) giveToModule(prefix string, tags []string) (command, refused string) {
	index := f.moduleLeftBehind(prefix, tags)
	if index < 0 {
		return "", ""
	}
	words := append([]string{"modules", "set", f.moduleArgument(f.modules[index]), "-T", prefix}, f.context.moduleFlags()...)
	if err := f.rehearsal.run(words); err != nil {
		return "", refusalReason(err)
	}
	f.modules[index].tagPrefix = prefix
	return commandText(words), ""
}

// moduleLeftBehind is the index among the modules of the one declared module that
// owns a directory the prefix names and has no tag of its own, -1 when there is
// not exactly one. A module with tags of its own left the others on purpose.
func (f *candidateFinder) moduleLeftBehind(prefix string, tags []string) int {
	name := prefixName(prefix)
	found := -1
	for index, module := range f.modules {
		named := slices.ContainsFunc(module.directories, func(directory string) bool { return directoryIsNamed(directory, name) })
		hasTags := slices.ContainsFunc(tags, func(tag string) bool { return versionOfAnotherModule(tag, []string{module.tagPrefix}) })
		if !module.withoutGoMod() || !named || hasTags {
			continue
		}
		if found >= 0 {
			return -1
		}
		found = index
	}
	return found
}

// lineCandidate is the candidate of a tag line: the module that owns the
// directories it may be the releases of, those it can own at least.
func (f *candidateFinder) lineCandidate(prefix string, guesses []string) (Candidate, error) {
	directories, leftOut := f.splitGuesses(guesses)
	if len(directories) == 0 {
		return Candidate{Directories: f.displayAll(guesses), Refused: leftOut[0].Reason, ShippedBy: []string{}, LeftOut: leftOut[1:]}, nil
	}

	declaration := ModuleDeclaration{Directories: directories}
	name := prefixName(prefix)
	oneDirectoryNames := len(directories) == 1 && (directories[0] == name || f.nameTaken(name))
	if !oneDirectoryNames {
		declaration.Name = name
	}
	if prefix != declaration.prefix() {
		declaration.TagPrefix = prefix
	}
	candidate, err := f.candidate(declaration)
	candidate.LeftOut = leftOut
	return candidate, err
}

// splitGuesses keeps the directories of guesses, which are sorted, that one
// module can own together, and says why it leaves out the others: a directory
// the file cannot name, and one inside another.
func (f *candidateFinder) splitGuesses(guesses []string) ([]string, []LeftOutDirectory) {
	var kept []string
	var leftOut []LeftOutDirectory
	for _, directory := range guesses {
		if err := firstFinding(modulePathFindings("", "directory", directory)); err != nil {
			leftOut = append(leftOut, LeftOutDirectory{Directory: f.display(directory), Reason: err.Error()})
			continue
		}
		index := slices.IndexFunc(kept, func(outer string) bool { return nestedIn(directory, outer) })
		if index >= 0 {
			leftOut = append(leftOut, LeftOutDirectory{Directory: f.display(directory), Reason: "it lies in " + f.display(kept[index])})
			continue
		}
		kept = append(kept, directory)
	}
	return kept, leftOut
}

// nameTaken reports a name some module has, or that is the directory of one.
func (f *candidateFinder) nameTaken(name string) bool {
	return slices.ContainsFunc(f.modules, func(module repositoryModule) bool {
		return module.named && module.name == name || module.owns(name)
	})
}

// foundManifest is a manifest that gives its directory a version.
type foundManifest struct {
	directory string
	name      string
	version   string
}

// manifests lists the Helm charts and the public Node.js packages that give
// a version in a directory that is free, and the charts there that could not
// be read.
func (f *candidateFinder) manifests() ([]ManifestCandidate, []UnreadableManifest, error) {
	charts, unreadableCharts := helm.Read(f.top, f.tracked.files)
	var found []foundManifest
	for _, chart := range charts {
		found = append(found, foundManifest{directory: chart.Directory, name: "Chart.yaml", version: chart.Version})
	}
	for _, nodePackage := range f.snapshot.packagesOf(f.top, f.tracked).Packages() {
		if !nodePackage.Private && nodePackage.Version != "" {
			found = append(found, foundManifest{directory: nodePackage.Directory, name: "package.json", version: nodePackage.Version})
		}
	}
	slices.SortFunc(found, func(left, right foundManifest) int {
		return strings.Compare(path.Join(left.directory, left.name), path.Join(right.directory, right.name))
	})

	candidates := []ManifestCandidate{}
	for _, manifest := range found {
		if !f.isFree(manifest.directory) {
			continue
		}
		candidate, err := f.candidate(ModuleDeclaration{Directories: []string{manifest.directory}})
		if err != nil {
			return nil, nil, err
		}
		candidates = append(candidates, ManifestCandidate{Manifest: path.Join(f.display(manifest.directory), manifest.name), Version: manifest.version, Candidate: candidate})
	}

	var unreadable []UnreadableManifest
	for _, chart := range unreadableCharts {
		if f.isFree(chart.Directory) {
			unreadable = append(unreadable, UnreadableManifest{Manifest: path.Join(f.display(chart.Directory), "Chart.yaml"), Error: chart.Err.Error()})
		}
	}
	return candidates, unreadable, nil
}

// isFree reports a directory that no module takes, the candidates accepted so far
// among them, and that holds no fixtures or dependencies.
func (f *candidateFinder) isFree(directory string) bool {
	return !isFixtureDirectory(directory) && !isModuleDirectory(directory, f.modules)
}

// candidate is the declaration, whose directories are relative to the top of
// the repository: shipped by the Go modules whose //go:embed patterns reach it,
// and by none when no pattern does. Its command is rehearsed after the commands
// printed before it: when that refuses it, the candidate carries why in place of
// the command, and takes no directory from those printed after it.
func (f *candidateFinder) candidate(declaration ModuleDeclaration) (Candidate, error) {
	declaration.ShippedBy = &[]string{}
	module := declaredRepositoryModule(declaration)
	modules := append(slices.Clone(f.modules), module)
	shippings, err := findShipping(f.tracked, f.top, []repositoryModule{module}, modules, f.snapshot.embeds)
	if err != nil {
		return Candidate{}, err
	}

	candidate := Candidate{Name: declaration.Name, Directories: f.displayAll(declaration.Directories), TagPrefix: declaration.TagPrefix, ShippedBy: []string{}}
	var carriers []string
	for _, found := range shippings {
		candidate.Embeds = append(candidate.Embeds, EmbedPlace{Place: f.display(found.file) + ":" + found.embed.Place(), Pattern: found.embed.Pattern})
		if !slices.Contains(carriers, found.goModule.name) {
			carriers = append(carriers, found.goModule.name)
			candidate.ShippedBy = append(candidate.ShippedBy, f.moduleArgument(found.goModule))
		}
	}

	declaration.ShippedBy = &carriers
	words := f.commandWords(candidate)
	if err := f.rehearsal.run(words); err != nil {
		candidate.Refused = refusalReason(err)
		return candidate, nil
	}
	candidate.Command = commandText(words)
	f.modules = append(f.modules, declaredRepositoryModule(declaration))
	return candidate, nil
}

// display is a path relative to the top of the repository, relative to the
// repository init reads, as the commands it proposes take it.
func (f *candidateFinder) display(entry string) string {
	relative, err := filepath.Rel(f.base, filepath.Join(f.top, filepath.FromSlash(entry)))
	if err != nil {
		return entry
	}
	return filepath.ToSlash(relative)
}

func (f *candidateFinder) displayAll(entries []string) []string {
	displayed := make([]string, 0, len(entries))
	for _, entry := range entries {
		displayed = append(displayed, f.display(entry))
	}
	return displayed
}

// moduleArgument is how a command that runs from where init ran calls the
// module: by its name, unless the name from there is the directory of another
// module, and else by a path.
func (f *candidateFinder) moduleArgument(module repositoryModule) string {
	if module.named && !f.nameIsAPathToAnother(module) {
		return module.name
	}
	return f.pathArgument(f.display(module.directories[0]))
}

// nameIsAPathToAnother reports whether the name of module, read as a path from
// where init ran, is the directory of another module.
func (f *candidateFinder) nameIsAPathToAnother(module repositoryModule) bool {
	relative, err := filepath.Rel(f.top, filepath.Join(f.base, filepath.FromSlash(module.name)))
	if err != nil {
		return false
	}
	return slices.ContainsFunc(f.modules, func(other repositoryModule) bool {
		return other.name != module.name && other.owns(filepath.ToSlash(relative))
	})
}

// pathArgument is a path as a command takes it: after ./ when it is the name of
// a module, which the command would read as that module, or when it starts
// with -, which the command would read as a flag.
func (f *candidateFinder) pathArgument(entry string) string {
	isName := slices.ContainsFunc(f.modules, func(module repositoryModule) bool { return module.named && module.name == entry })
	if isName {
		return "./" + entry
	}
	return notAFlag(entry)
}

// commandWords are the words after bumpit of the bumpit modules add that
// declares candidate, to run from where init ran.
func (f *candidateFinder) commandWords(candidate Candidate) []string {
	words := []string{"modules", "add"}
	for _, directory := range candidate.Directories {
		words = append(words, f.pathArgument(directory))
	}
	words = append(words, f.context.moduleFlags()...)
	if candidate.Name != "" {
		words = append(words, "-e", candidate.Name)
	}
	if candidate.TagPrefix != "" {
		words = append(words, "-T", candidate.TagPrefix)
	}
	for _, carrier := range candidate.ShippedBy {
		words = append(words, "-b", carrier)
	}
	return words
}

// commandText is the bumpit command with the words as a shell reads it back.
func commandText(words []string) string {
	quoted := make([]string, 0, len(words)+1)
	quoted = append(quoted, "bumpit")
	for _, word := range words {
		quoted = append(quoted, shellWord(word))
	}
	return strings.Join(quoted, " ")
}

var plainWord = regexp.MustCompile(`^[A-Za-z0-9_./@:+=,-]+$`)

// shellWord is word as a shell reads it back: as it is when it holds no
// character a shell gives a meaning, single quoted otherwise.
func shellWord(word string) string {
	if plainWord.MatchString(word) {
		return word
	}
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}
