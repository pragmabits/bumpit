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

	"github.com/pragmabits/bumpit/internal/gitx"
	"github.com/pragmabits/bumpit/internal/helm"
)

// initialConfig is the bumpit.yaml bumpit init creates, which declares
// nothing.
const initialConfig = "# bumpit reads this file at the top of the repository. Declare the modules\n# that have no go.mod with bumpit modules add, and set the other keys with\n# bumpit config set.\n"

// InitOptions are the options of bumpit init.
type InitOptions struct {
	Repository string
}

// InitReport is what bumpit init did and what it proposes: the config file,
// whether it created it, and the parts of the repository that look released
// on their own while no module covers them.
type InitReport struct {
	Config    string              `json:"config"`
	Created   bool                `json:"created"`
	TagLines  []TagLine           `json:"tag_lines"`
	Manifests []ManifestCandidate `json:"manifests"`
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
// bumpit.yaml nor .bumpit.yaml, and lists the candidate modules: the lines
// of version tags that no module owns, with the directories they may be the
// releases of, and the manifests with a version of their own. It declares
// nothing.
func Init(opts InitOptions) (InitReport, error) {
	finder, topConfig, err := newCandidateFinder(opts.Repository)
	if err != nil {
		return InitReport{}, err
	}

	report := InitReport{Config: topConfig}
	if report.TagLines, err = finder.tagLines(); err != nil {
		return InitReport{}, err
	}
	if report.Manifests, report.Unreadable, err = finder.manifests(report.TagLines); err != nil {
		return InitReport{}, err
	}

	if topConfig == "" {
		report.Config = filepath.Join(finder.top, "bumpit.yaml")
		if err := createConfig(report.Config); err != nil {
			return InitReport{}, err
		}
		report.Created = true
	}
	return report, nil
}

// createConfig writes initialConfig to configPath, which must not exist.
func createConfig(configPath string) error {
	file, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(initialConfig)
	return errors.Join(writeErr, file.Close())
}

// candidateFinder finds the candidate modules of the repository at top,
// which init was given as repository.
type candidateFinder struct {
	client     gitx.Client
	repository string
	// base is the absolute path of repository, links resolved, which the
	// paths init prints are relative to.
	base    string
	top     string
	tracked trackedTree
	modules []repositoryModule
	// tagMatch is the tag pattern of the config at the top, whose tags a
	// run with no module reads.
	tagMatch string
	node     *nodePackages
	embeds   embedCache
}

// newCandidateFinder reads the modules of the repository, those the config
// at its top declares included, and returns that config, empty when the
// repository has none.
func newCandidateFinder(repository string) (candidateFinder, string, error) {
	client := gitx.New(repository)
	top, err := client.Top()
	if err != nil {
		return candidateFinder{}, "", fmt.Errorf("%s is in no git repository: %w", repository, err)
	}
	finder := candidateFinder{client: client, repository: repository, top: top, embeds: embedCache{}}
	topConfig, err := finder.readTopConfig()
	if err != nil {
		return candidateFinder{}, "", err
	}
	if err := finder.readBase(); err != nil {
		return candidateFinder{}, "", err
	}
	return finder, topConfig, nil
}

// readTopConfig reads the config at the top of the repository, its tag
// pattern and its modules, and the modules of the repository, and returns
// the path of that config, empty when there is none.
func (f *candidateFinder) readTopConfig() (string, error) {
	topConfig, err := findConfig(f.top)
	if errors.Is(err, ErrConfigNotFound) {
		topConfig, err = "", nil
	}
	if err != nil {
		return "", err
	}
	if topConfig != "" {
		config, _, err := decodeConfig(topConfig, Config{})
		if err != nil {
			return "", err
		}
		f.tagMatch = config.TagMatch
	}
	declarations, err := topDeclarations(topConfig, nil, "")
	if err != nil {
		return "", err
	}

	if f.tracked, err = listTracked(f.client); err != nil {
		return "", err
	}
	if f.modules, _, err = discoverRepositoryModulesIn(f.client, declarations, &f.tracked); err != nil {
		return "", err
	}
	f.node = newNodePackages(f.top, f.tracked)
	if index := slices.IndexFunc(f.modules, repositoryModule.declared); index >= 0 {
		f.node = f.modules[index].node
	}
	return topConfig, nil
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

// tagLines lists the tag lines no module owns, with a candidate for each
// directory they may be the releases of. The tags of a run with no module
// are owned whatever the modules: those of the tag pattern of the config,
// and those of v.
func (f candidateFinder) tagLines() ([]TagLine, error) {
	tags, err := f.client.Tags("")
	if err != nil {
		return nil, err
	}
	prefixes := []string{"v"}
	if f.tagMatch != "" {
		prefixes = append(prefixes, tagPrefix(f.tagMatch))
	}
	for _, module := range f.modules {
		prefixes = append(prefixes, module.tagPrefix)
	}

	lines := unownedTagLines(tags, prefixes)
	for index := range lines {
		lines[index].Candidates = []Candidate{}
		for _, directory := range guessDirectories(lines[index].Prefix, f.tracked, f.modules) {
			candidate, err := f.candidate(directory, lines[index].Prefix)
			if err != nil {
				return nil, err
			}
			lines[index].Candidates = append(lines[index].Candidates, candidate)
		}
	}
	return lines, nil
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
func (f candidateFinder) manifests(lines []TagLine) ([]ManifestCandidate, []UnreadableManifest, error) {
	charts, unreadableCharts := helm.Read(f.top, f.tracked.files)
	var found []foundManifest
	for _, chart := range charts {
		found = append(found, foundManifest{directory: chart.Directory, name: "Chart.yaml", version: chart.Version})
	}
	for _, nodePackage := range f.node.repository().Packages() {
		if !nodePackage.Private && nodePackage.Version != "" {
			found = append(found, foundManifest{directory: nodePackage.Directory, name: "package.json", version: nodePackage.Version})
		}
	}
	slices.SortFunc(found, func(left, right foundManifest) int {
		return strings.Compare(path.Join(left.directory, left.name), path.Join(right.directory, right.name))
	})

	candidates := []ManifestCandidate{}
	for _, manifest := range found {
		if !f.isFree(manifest.directory, lines) {
			continue
		}
		candidate, err := f.candidate(manifest.directory, "")
		if err != nil {
			return nil, nil, err
		}
		candidates = append(candidates, ManifestCandidate{Manifest: path.Join(candidate.Directory, manifest.name), Version: manifest.version, Candidate: candidate})
	}

	var unreadable []UnreadableManifest
	for _, chart := range unreadableCharts {
		if f.isFree(chart.Directory, lines) {
			unreadable = append(unreadable, UnreadableManifest{Manifest: path.Join(f.display(chart.Directory), "Chart.yaml"), Error: chart.Err.Error()})
		}
	}
	return candidates, unreadable, nil
}

// isFree reports a directory that no module and no tag line candidate
// takes, and that holds no fixtures or dependencies.
func (f candidateFinder) isFree(directory string, lines []TagLine) bool {
	if isFixtureDirectory(directory) || isModuleDirectory(directory, f.modules) {
		return false
	}
	return !slices.ContainsFunc(lines, func(line TagLine) bool {
		return slices.ContainsFunc(line.Candidates, func(candidate Candidate) bool { return candidate.topDirectory == directory })
	})
}

// candidate is the declaration of directory, relative to the top of the
// repository, with tagPrefix, empty for the default one: shipped by the Go
// modules whose //go:embed patterns reach it, and by none when no pattern
// does. When bumpit modules add would refuse that declaration, the
// candidate carries why in place of the command.
func (f candidateFinder) candidate(directory, tagPrefix string) (Candidate, error) {
	declaration := ModuleDeclaration{Directory: directory, ShippedBy: &[]string{}}
	if tagPrefix != declaration.prefix() {
		declaration.TagPrefix = tagPrefix
	}
	module := declaredRepositoryModule(declaration)
	modules := append(slices.Clone(f.modules), module)
	shippings, err := findShipping(f.tracked, f.top, []repositoryModule{module}, modules, f.embeds)
	if err != nil {
		return Candidate{}, err
	}

	candidate := Candidate{Directory: f.display(directory), TagPrefix: declaration.TagPrefix, ShippedBy: []string{}, topDirectory: directory}
	var carriers []string
	for _, found := range shippings {
		candidate.Embeds = append(candidate.Embeds, EmbedPlace{Place: f.display(found.file) + ":" + found.embed.Place(), Pattern: found.embed.Pattern})
		if !slices.Contains(carriers, found.goModule.directory) {
			carriers = append(carriers, found.goModule.directory)
			candidate.ShippedBy = append(candidate.ShippedBy, f.display(found.goModule.directory))
		}
	}

	declaration.ShippedBy = &carriers
	if err := f.check(declaration); err != nil {
		candidate.Refused = err.Error()
		return candidate, nil
	}
	candidate.Command = f.command(candidate)
	return candidate, nil
}

// check refuses the declaration as bumpit modules add would: what its text
// cannot take, and what the repository contradicts, the shipping by
// //go:embed aside, which the declaration follows.
func (f candidateFinder) check(declaration ModuleDeclaration) error {
	if err := validateModuleDeclaration(declaration); err != nil {
		return err
	}
	module := declaredRepositoryModule(declaration)
	return checkDeclaredModule(f.client, f.tracked, module, append(slices.Clone(f.modules), module))
}

// display is a path relative to the top of the repository, relative to the
// repository init was given, as the commands it proposes take it.
func (f candidateFinder) display(entry string) string {
	relative, err := filepath.Rel(f.base, filepath.Join(f.top, filepath.FromSlash(entry)))
	if err != nil {
		return entry
	}
	return filepath.ToSlash(relative)
}

// command is the bumpit modules add that declares candidate, to run from
// where init ran.
func (f candidateFinder) command(candidate Candidate) string {
	words := []string{"bumpit", "modules", "add", pathWord(candidate.Directory)}
	if f.repository != "." {
		words = append(words, "-r", pathWord(f.repository))
	}
	if candidate.TagPrefix != "" {
		words = append(words, "-T", shellWord(candidate.TagPrefix))
	}
	for _, carrier := range candidate.ShippedBy {
		words = append(words, "-b", pathWord(carrier))
	}
	if len(candidate.ShippedBy) == 0 {
		words = append(words, "-B")
	}
	return strings.Join(words, " ")
}

// pathWord is shellWord for a path, written after ./ when it starts with -,
// which the command would read as a flag.
func pathWord(entry string) string {
	if strings.HasPrefix(entry, "-") {
		entry = "./" + entry
	}
	return shellWord(entry)
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
