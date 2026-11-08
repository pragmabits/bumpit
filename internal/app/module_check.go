package app

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// CheckOptions are the options of bumpit modules check.
type CheckOptions struct {
	Repository string
	// RepositoryGiven reports a --repo on the command line, which the config
	// then does not replace.
	RepositoryGiven bool
	ConfigPath      string
}

// CheckItem is a finding of bumpit modules check and the command that resolves
// it, empty when resolving it takes a choice or the command would be refused.
type CheckItem struct {
	Finding
	Command string `json:"command,omitempty"`
	// Embeds and LeftOut are what a proposal rests on: the //go:embed patterns
	// behind its carriers, and the directories it could not propose.
	Embeds  []EmbedPlace       `json:"embeds,omitempty"`
	LeftOut []LeftOutDirectory `json:"left_out,omitempty"`
}

// ModulesCheck is what needs resolving in the config a run reads.
type ModulesCheck struct {
	// Config is the file that was read, empty when there is none.
	Config string `json:"config"`
	// FileOnly reports a file that does not hold, whose modules were not
	// checked against the repository.
	FileOnly bool `json:"file_only,omitempty"`
	// Blocking are the problems a run stops at.
	Blocking []CheckItem `json:"blocking"`
	// Review are what stops no run and may need declaring.
	Review []CheckItem `json:"review"`
	// Unverified are the modules that say no module ships them, which bumpit
	// can only check against //go:embed.
	Unverified []string `json:"unverified"`
	// output is the output the config asks for.
	output string
}

// moduleChecker is one check of the config of a run: what it has read of that
// config, and the report it fills.
type moduleChecker struct {
	options CheckOptions
	report  ModulesCheck
	// config is the config of the run, at configPath, nil when it has none.
	config     *Config
	configPath string
	context    commandContext
	// repository is the repository a run reads, the one the config gives when
	// the command line gives none.
	repository string
	// source is where a run reads the modules from.
	source       moduleSource
	declarations []ModuleDeclaration
	// tagMatches are the tag patterns a run reads tags by.
	tagMatches []string
	// fileProblems are the problems of the files found while reading them.
	fileProblems []Finding
	// rehearsal runs the commands of the report, in the order they are printed.
	rehearsal *rehearsal
}

// CheckModules lists what needs resolving in the config a run reads, as a run
// reads it, and writes nothing. A file that does not hold stops the check before
// the repository, which it could not be compared with. An error is a failure to
// ask, such as a go.mod that cannot be read, beside what was found until then.
func CheckModules(opts CheckOptions) (ModulesCheck, error) {
	checker := newModuleChecker(opts)
	err := checker.check()
	return checker.report, err
}

func newModuleChecker(opts CheckOptions) *moduleChecker {
	checker := &moduleChecker{
		options:    opts,
		report:     ModulesCheck{Blocking: []CheckItem{}, Review: []CheckItem{}, Unverified: []string{}, output: "text"},
		context:    commandContext{configGiven: opts.ConfigPath},
		repository: opts.Repository,
		rehearsal:  newRehearsal(),
	}
	if opts.RepositoryGiven {
		checker.context.repositoryGiven = opts.Repository
	}
	return checker
}

// check runs the stages of a check in order, each of which fills the report, until
// one leaves nothing more the report can say.
func (c *moduleChecker) check() error {
	if !c.readRunConfig() {
		return nil
	}
	if err := c.findSource(); err != nil {
		return err
	}
	switch {
	case c.source.top == "":
		return c.reviewNoRepository()
	case c.report.Config == "":
		return c.reviewNoFile()
	}
	if !c.readDeclarations() {
		return nil
	}
	found, err := c.reportFileFindings()
	if err != nil || found {
		return err
	}
	return c.checkRepository()
}

// snapshot is what the check reads of the repository a run reads.
func (c *moduleChecker) snapshot() *repositorySnapshot {
	return c.rehearsal.snapshotAt(c.repository)
}

// readRunConfig reads the config of the run and the repository it gives. It
// reports false for a file it cannot read, which is then all the report can say.
func (c *moduleChecker) readRunConfig() bool {
	flags := configEditFlags{configPath: c.options.ConfigPath, repository: c.options.Repository}
	config, configPath, err := flags.peekConfig()
	if err != nil {
		c.reportUnreadable(configPath, err)
		return false
	}

	c.config, c.configPath = config, configPath
	if config != nil {
		c.report.output = config.Output
		if !c.options.RepositoryGiven && validateRepositoryPath(config.Repository) == nil {
			c.repository = config.Repository
		}
	}
	c.context.effective = c.repository
	return true
}

// reportUnreadable fills the report with a file nothing else can be read from.
func (c *moduleChecker) reportUnreadable(path string, err error) {
	c.report.Config = path
	c.report.Blocking = []CheckItem{{Finding: unreadableFinding(path, err)}}
}

// findSource finds where a run reads the modules from, and so the file the report
// is about, empty when the repository has no config.
func (c *moduleChecker) findSource() error {
	source, err := findModuleSource(c.options.ConfigPath != "", c.snapshot().top, c.config, c.configPath)
	if err != nil {
		return err
	}
	c.source = source
	c.report.Config = valueOr(c.configPath, source.topConfig)
	return nil
}

// reviewNoRepository says what the keys of the file hold wrong, the repository key
// perhaps being why there is no repository, and else that there is none.
func (c *moduleChecker) reviewNoRepository() error {
	found, err := c.reportFileFindings()
	if err != nil || found {
		return err
	}
	return errNoRepository(c.repository, c.source.noTop)
}

// reviewNoFile says that the repository has no config to check, and how to give
// it one. A go.mod that cannot be read stops every run, with or without a config,
// and the command that creates the file.
func (c *moduleChecker) reviewNoFile() error {
	if _, err := c.snapshot().goModules(); err != nil {
		return err
	}
	item, err := c.noFileItem()
	if err != nil {
		return err
	}
	c.report.Review = []CheckItem{item}
	return nil
}

// readDeclarations reads the modules a run reads, from where a run reads them,
// without stopping at what is wrong with them. It reports false for a file it
// cannot read, which is then all the report can say.
func (c *moduleChecker) readDeclarations() bool {
	c.context.configHoldsModules = c.context.configGiven != "" && c.source.fromConfig()

	if err := c.readTop(); err != nil && !c.source.given {
		c.reportUnreadable(c.source.topConfig, err)
		return false
	}

	switch {
	case c.source.fromConfig():
		c.declarations = c.config.Modules
	case c.source.belowTop:
		c.fileProblems = append(c.fileProblems, Finding{
			Rule:    ruleModulesBelowTop,
			Subject: c.configPath,
			What:    "it declares modules",
			Why:     "bumpit reads them from the bumpit.yaml at the top of the repository, as the go.mod files describe all of it",
			How:     "move them to the bumpit.yaml at " + c.source.top,
		})
	}
	return true
}

// reportFileFindings fills the report with what is wrong with the file alone, and
// reports whether there is any: such a file stops the check before the repository,
// which it could not be compared with. An error is a failure to ask, beside the
// items made up until then.
func (c *moduleChecker) reportFileFindings() (bool, error) {
	findings := c.fileFindings()
	if len(findings) == 0 {
		return false, nil
	}
	items, err := c.keyItems(findings)
	c.report.FileOnly = true
	c.report.Blocking = items
	return true, err
}

// readTop reads the tag pattern of the config of the run, whose tags that run
// reads as its own, and the modules and the tag pattern of the config at the top,
// from the config of the run when that is the same file. Of another file it reads
// those keys alone, as a run does.
func (c *moduleChecker) readTop() error {
	if c.config != nil {
		c.tagMatches = append(c.tagMatches, c.config.TagMatch)
	}
	if c.source.atTop || c.source.topConfig == "" {
		return nil
	}
	declarations, tagMatch, err := readTopModules(c.source.topConfig)
	c.declarations, c.tagMatches = declarations, append(c.tagMatches, tagMatch)
	return err
}

// unreadableFinding is the finding of a file nothing else can be read from.
func unreadableFinding(path string, err error) Finding {
	return Finding{
		Rule:    ruleUnreadableFile,
		Subject: path,
		What:    err.Error(),
		Why:     "every run that reads it stops, and nothing else in it can be checked",
		How:     "edit it by hand: the file takes " + strings.Join(keysOf[Config](), ", ") + ", and each module takes " + strings.Join(keysOf[ModuleDeclaration](), ", "),
	}
}

// keysOf lists the keys of the config file that type T reads, in its order.
func keysOf[T any]() []string {
	fields := reflect.TypeFor[T]()
	keys := make([]string, 0, fields.NumField())
	for index := range fields.NumField() {
		keys = append(keys, fields.Field(index).Tag.Get("mapstructure"))
	}
	return keys
}

func (c *moduleChecker) noFileItem() (CheckItem, error) {
	words := append([]string{"init"}, c.context.repositoryFlags()...)
	return c.rehearsal.item(Finding{
		Rule:    ruleNoFile,
		Subject: c.source.top,
		What:    "no bumpit.yaml",
		Why:     "nothing to check: the Go modules work without one, and a part of the repository with no go.mod needs it to be versioned",
		How:     "create it, with the Go modules, if the repository has anything else to declare",
	}, words)
}

// fileFindings lists what is wrong with the file alone: its keys and the text of
// its modules.
func (c *moduleChecker) fileFindings() []Finding {
	findings := slices.Clone(c.fileProblems)
	if c.config != nil {
		findings = append(findings, configKeyFindings(*c.config)...)
	}
	return append(findings, declarationFindings(c.declarations)...)
}

// keyItems are the items of the findings of a file, the only ones with a command
// being the keys, whose value nothing depends on keeping, unless two of them
// conflict, which of them goes being a choice. An error is a failure to ask, which
// comes with the items made up until then.
func (c *moduleChecker) keyItems(findings []Finding) ([]CheckItem, error) {
	items := make([]CheckItem, 0, len(findings))
	for _, finding := range findings {
		var words []string
		if finding.Rule == ruleInvalidKey && finding.Other == "" {
			words = append([]string{"config", "unset", finding.Subject}, c.context.keyFlags()...)
		}
		item, err := c.rehearsal.item(finding, words)
		if err != nil {
			return items, err
		}
		items = append(items, item)
	}
	return items, nil
}

// checkRepository compares the modules with the repository, and fills the report
// with what the file says wrong of it and what it leaves unsaid.
func (c *moduleChecker) checkRepository() error {
	snapshot := c.snapshot()
	found, err := repositoryFindings(snapshot, c.declarations)
	if err != nil {
		return err
	}
	tagNames, err := goTagNameFindings(snapshot, found.modules)
	if err != nil {
		return err
	}
	finder := &candidateFinder{
		snapshot:     snapshot,
		repository:   c.repository,
		context:      c.context,
		top:          c.source.top,
		declarations: c.declarations,
		tracked:      found.tracked,
		modules:      found.modules,
		tagMatches:   c.tagMatches,
		rehearsal:    c.rehearsal,
	}
	if err := finder.readBase(); err != nil {
		return err
	}

	if c.report.Blocking, err = finder.blockingItems(append(mergeConflicts(found.findings), tagNames...)); err != nil {
		return err
	}
	review, err := finder.reviewItems()
	c.report.Review = review
	if err != nil {
		return err
	}
	for _, module := range found.modules {
		if module.withoutGoMod() && module.shippedBy != nil && len(module.shippedBy) == 0 {
			c.report.Unverified = append(c.report.Unverified, module.name)
		}
	}
	return nil
}

// mergeConflicts leaves one finding of each conflict of tag prefixes between two
// modules, which each reports. Following both would change both, and only they
// are one problem. Findings alike about other modules are problems of their own,
// and so are two names that are each other's directories, which take a new name
// each.
func mergeConflicts(findings []Finding) []Finding {
	merged := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		isPrefixConflict := finding.Rule == ruleTagPrefixTaken || finding.Rule == ruleTagPrefixContinues
		reported := isPrefixConflict && slices.ContainsFunc(merged, func(earlier Finding) bool {
			return earlier.Rule == finding.Rule && earlier.Subject == finding.Other && earlier.Other == finding.Subject
		})
		if !reported {
			merged = append(merged, finding)
		}
	}
	return merged
}

// blockingItems are the items of the problems the repository shows. Only an
// embed has a command, once for each module: the others take a choice. An error
// is a failure to ask, which comes with the items made up until then.
func (f *candidateFinder) blockingItems(findings []Finding) ([]CheckItem, error) {
	items := make([]CheckItem, 0, len(findings))
	commanded := map[string]bool{}
	for _, finding := range findings {
		var words []string
		if finding.Rule == ruleEmbedWithoutCarrier && !commanded[finding.Subject] {
			commanded[finding.Subject] = true
			words = f.carriersCommand(finding.Subject, findings)
		}
		item, err := f.rehearsal.item(finding, words)
		if err != nil {
			return items, err
		}
		items = append(items, item)
	}
	return items, nil
}

// carriersCommand is the words of the bumpit modules set that gives the module
// the carriers it has and all those its embeds ask for, since --shipped-by
// replaces the list. A carrier that is no module takes a decision, and no
// command.
func (f *candidateFinder) carriersCommand(name string, findings []Finding) []string {
	index := moduleNamed(f.modules, name)
	unknown := slices.ContainsFunc(findings, func(finding Finding) bool {
		return finding.Rule == ruleCarrierUnknown && finding.Subject == name
	})
	if index < 0 || unknown {
		return nil
	}

	carriers := slices.Clone(f.modules[index].shippedBy)
	for _, finding := range findings {
		if finding.Rule == ruleEmbedWithoutCarrier && finding.Subject == name && !slices.Contains(carriers, finding.missingCarrier) {
			carriers = append(carriers, finding.missingCarrier)
		}
	}
	words := []string{"modules", "set", f.moduleArgument(f.modules[index])}
	for _, carrier := range carriers {
		words = append(words, "-b", f.moduleArgument(f.modules[moduleNamed(f.modules, carrier)]))
	}
	return append(words, f.context.moduleFlags()...)
}

// reviewItems are what stops no run and may need declaring: the Go modules no
// entry lists, the tag lines no module owns and the manifests with a version of
// their own. The tags with no prefix, which no module can own, are left out. An
// error comes with the items made up until then.
func (f *candidateFinder) reviewItems() ([]CheckItem, error) {
	items, err := f.unlistedGoModuleItems()
	if err != nil {
		return items, err
	}

	lines, err := f.tagLines()
	if err != nil {
		return items, err
	}
	for _, line := range lines {
		if line.Prefix != "" {
			items = append(items, tagLineItem(line))
		}
	}
	manifests, unreadable, err := f.manifests()
	if err != nil {
		return items, err
	}
	for _, manifest := range manifests {
		items = append(items, manifestItem(manifest))
	}
	for _, manifest := range unreadable {
		items = append(items, unreadableManifestItem(manifest))
	}
	return items, nil
}

// unlistedGoModuleItems are the items of the Go modules no entry lists, but those in
// a directory git takes in no tag name, which block the runs: the new name for the
// directory that problem asks for comes before an entry can list it. An error comes
// with the items made up until then.
func (f *candidateFinder) unlistedGoModuleItems() ([]CheckItem, error) {
	items := []CheckItem{}
	for _, module := range f.unlistedGoModules() {
		renamed, err := f.snapshot.takesNoTagName(module)
		if err != nil {
			return items, err
		}
		if renamed {
			continue
		}
		item, err := f.unlistedGoModuleItem(module.directories[0])
		if err != nil {
			return items, err
		}
		items = append(items, item)
	}
	return items, nil
}

// unlistedGoModuleItem is the item of a Go module no entry lists, with the command
// that lists it. unlistedGoModuleItems gives it only directories git takes a tag
// name from, and an entry can name each of them: the characters that keep an entry
// from naming a directory, those of a glob and a : at the start, are ones git
// refuses in a tag name. An error is a failure to ask.
func (f *candidateFinder) unlistedGoModuleItem(directory string) (CheckItem, error) {
	finding := Finding{
		Rule:    ruleGoModuleUnlisted,
		Subject: directory,
		What:    "go.mod with no entry in bumpit.yaml",
		Why:     "it is versioned all the same, but the file does not list it",
		How:     "list it, as bumpit init also does",
	}
	words := append([]string{"modules", "add", f.pathArgument(f.display(directory))}, f.context.moduleFlags()...)
	return f.rehearsal.item(finding, words)
}

// unreadableManifestItem is the item of a manifest that could give its directory a
// version, which no command can say until it can be read.
func unreadableManifestItem(manifest UnreadableManifest) CheckItem {
	return CheckItem{Finding: Finding{
		Rule:    ruleUnreadableManifest,
		Subject: manifest.Manifest,
		What:    "it cannot be read",
		Why:     manifest.Error,
		How:     "repair it, if bumpit is to version it",
	}}
}

func tagLineItem(line TagLine) CheckItem {
	finding := Finding{
		Rule:    ruleTagsUnowned,
		Subject: line.Prefix,
		What:    countTags(line.Count) + " no module owns, the newest " + line.Newest,
		Why:     "they may be the releases of a module not declared yet",
		How:     "declare its module with bumpit modules add and --tag-prefix",
	}
	switch {
	case line.Candidate != nil:
		return candidateItem(finding, *line.Candidate, "declare it, if these are its releases")
	case line.Command != "":
		finding.How = "give them to the module that owns the directory they name, which has no tags of its own, if these are its releases"
		return CheckItem{Finding: finding, Command: line.Command}
	case line.Refused != "":
		finding.How = "bumpit modules set would refuse to give them to the module that owns the directory they name, which has no tags of its own: " + line.Refused
	}
	return CheckItem{Finding: finding}
}

func manifestItem(manifest ManifestCandidate) CheckItem {
	return candidateItem(Finding{
		Rule:    ruleManifest,
		Subject: manifest.Manifest,
		What:    "version " + manifest.Version + " of its own",
		Why:     "it may be released on its own",
	}, manifest.Candidate, "declare it, if bumpit is to version it")
}

// candidateItem is the item of a proposal: its command, which has been rehearsed,
// with how, the way to resolve the finding, or why it would be refused, and what
// it rests on.
func candidateItem(finding Finding, candidate Candidate, how string) CheckItem {
	item := CheckItem{Command: candidate.Command, Embeds: candidate.Embeds, LeftOut: candidate.LeftOut}
	switch {
	case candidate.Refused != "":
		finding.How = "bumpit modules add would refuse " + strings.Join(candidate.Directories, ", ") + ": " + candidate.Refused
	case len(candidate.Directories) > 1:
		finding.How = how + ", without a directory that is no part of the module"
	default:
		finding.How = how
	}
	item.Finding = finding
	return item
}

// item is the item of the finding with the command of words, which runs in the
// rehearsal first, so that bumpit prints none it would refuse. One that would be
// refused gives its reason in place of the command. An error is a failure to ask.
func (r *rehearsal) item(finding Finding, words []string) (CheckItem, error) {
	if words == nil {
		return CheckItem{Finding: finding}, nil
	}
	refused, err := r.run(words)
	switch {
	case err != nil:
		return CheckItem{}, err
	case refused != "":
		finding.How += "; edit it by hand, since the command would be refused: " + refused
		return CheckItem{Finding: finding}, nil
	}
	return CheckItem{Finding: finding, Command: commandText(words)}, nil
}

// errModulesBlocked is what bumpit modules check fails with when a run would stop.
func errModulesBlocked(count int) error {
	if count == 1 {
		return errors.New("1 problem blocks the runs")
	}
	return fmt.Errorf("%d problems block the runs", count)
}
