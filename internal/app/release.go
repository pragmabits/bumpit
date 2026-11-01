package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pragmabits/bumpit/internal/gitx"
	"github.com/pragmabits/bumpit/internal/rules"
	"github.com/pragmabits/bumpit/internal/semver"
)

// Options is what a release plan is built from: one field per flag of next,
// explain and tag, and the modules bumpit.yaml declares.
type Options struct {
	Repository string
	// TagMatch is the tag pattern; empty means "v*", or the root module's
	// tag prefix. It cannot be combined with Module.
	TagMatch     string
	AllowDirty   bool
	StartVersion string
	PreRelease   string
	Promote      bool
	// ReleaseAs is a version to release as is, validated like any other.
	ReleaseAs string
	// Module is the directory of the module to version, relative to
	// Repository.
	Module string
	// Modules are the modules bumpit.yaml declares.
	Modules []ModuleDeclaration
}

// IgnoredTag is a tag that matches the pattern without carrying a semantic
// version after its prefix.
type IgnoredTag struct {
	Tag    string `json:"tag"`
	Reason string `json:"reason"`
}

// CommitDecision is what one commit asks for, as explain lists it.
type CommitDecision struct {
	Hash     string `json:"hash"`
	Subject  string `json:"subject"`
	Type     string `json:"type,omitempty"`
	Bump     string `json:"bump"`
	Reason   string `json:"reason"`
	Breaking bool   `json:"breaking"`
	// Via is the path or the module the commit came in through, empty when
	// it touches the module's own directory.
	Via string `json:"via,omitempty"`
}

// ReleasePlan is the next release and how it was reached: next and tag use
// NextTag, and explain prints the rest.
type ReleasePlan struct {
	CurrentTag      string           `json:"current_tag,omitempty"`
	CurrentVersion  string           `json:"current_version,omitempty"`
	BaseTag         string           `json:"base_tag,omitempty"`
	NextTag         string           `json:"next_tag,omitempty"`
	NextVersion     string           `json:"next_version,omitempty"`
	HasRelease      bool             `json:"has_release"`
	HighestBump     string           `json:"highest_bump"`
	CommitCount     int              `json:"commit_count"`
	RelevantCommits int              `json:"relevant_commit_count"`
	Prefix          string           `json:"-"`
	Analyses        []CommitDecision `json:"commits"`
	IgnoredTags     []IgnoredTag     `json:"ignored_tags,omitempty"`
	Module          string           `json:"module,omitempty"`
	Dependents      []Dependent      `json:"dependents,omitempty"`
	// Directory, Paths and ShippedBy describe a module bumpit.yaml declares,
	// which has no module path.
	Directory string   `json:"directory,omitempty"`
	Paths     []string `json:"paths,omitempty"`
	ShippedBy []string `json:"shipped_by,omitempty"`
	// Ships lists the declared modules whose commits the module reads, since
	// its release carries them.
	Ships []string `json:"ships,omitempty"`
	// DerivedPaths are the paths a declared module reads because of its
	// package.json: the packages it depends on and the files of its
	// workspace roots.
	DerivedPaths []string `json:"derived_paths,omitempty"`
	// LeavesOut lists the declared modules nested in the module, whose
	// commits it does not read.
	LeavesOut        []string `json:"leaves_out,omitempty"`
	highestBumpLevel semver.BumpLevel
}

// taggedVersion is a tag and the semantic version it carries.
type taggedVersion struct {
	tag     string
	version semver.Version
}

// versionTags is what the tags matching a pattern say about the release line:
// the highest version, the highest final release, and the tags that match
// without carrying a semantic version.
type versionTags struct {
	current taggedVersion
	base    taggedVersion
	ignored []IgnoredTag
}

// releaseState is what the next version is computed from.
type releaseState struct {
	versionTags
	// level is the bump the commits since the base release ask for.
	level semver.BumpLevel
	// pending reports a releasable commit after the current tag.
	pending bool
	prefix  string
	// scope is the module being versioned, nil outside one.
	scope *moduleScope
}

// BuildReleasePlan computes the next version. The commits since the last
// final release decide the level, and a prerelease belongs to its normal
// version: later commits continue it while they ask for no more than that
// version. The result must outrank the current version and must not be
// tagged yet, since a released version never changes.
func BuildReleasePlan(opts Options) (ReleasePlan, error) {
	if err := validateOptions(opts); err != nil {
		return ReleasePlan{}, err
	}

	client, err := openRepository(opts)
	if err != nil {
		return ReleasePlan{}, err
	}

	scope, modules, err := resolveModuleScope(client, opts.Repository, opts.Module, opts.TagMatch, opts.Modules)
	if err != nil {
		return ReleasePlan{}, err
	}
	return planRelease(client, scope, otherModulePrefixes(scope, modules, opts.TagMatch), opts)
}

// planRelease computes the next version of the module scope versions, or of
// the tags opts matches outside one, leaving out the versions of the modules
// whose prefixes otherPrefixes lists.
func planRelease(client gitx.Client, scope *moduleScope, otherPrefixes []string, opts Options) (ReleasePlan, error) {
	if err := scope.derivationError(); err != nil {
		return ReleasePlan{}, err
	}
	tagMatch := tagPattern(scope, opts.TagMatch)
	tags, err := collectVersionTags(client, tagMatch, false, otherPrefixes)
	if err != nil {
		return ReleasePlan{}, err
	}

	state := releaseState{versionTags: tags, prefix: prefixOfTag(tags.current.tag, tagPrefix(tagMatch)), scope: scope}
	plan, err := analyzeCommits(client, &state)
	if err != nil {
		return ReleasePlan{}, err
	}

	next, found, err := decideNext(state, opts)
	if err != nil {
		return ReleasePlan{}, err
	}
	if !found {
		return plan, nil
	}

	nextTag := next.Tag(state.prefix)
	if err := verifyNext(client, state, next, nextTag); err != nil {
		return ReleasePlan{}, err
	}

	plan.HasRelease = true
	plan.NextVersion = next.String()
	plan.NextTag = nextTag
	plan.Dependents = scope.dependentsBelow(next)
	return plan, nil
}

func validateOptions(opts Options) error {
	preRelease := strings.TrimSpace(opts.PreRelease) != ""
	releaseAs := strings.TrimSpace(opts.ReleaseAs) != ""

	switch {
	case opts.Promote && preRelease:
		return fmt.Errorf("--promote and --pre cannot be used together")
	case releaseAs && (opts.Promote || preRelease):
		return fmt.Errorf("--release-as cannot be used with --pre or --promote")
	case opts.Module != "" && opts.TagMatch != "":
		return fmt.Errorf("--module sets the tag pattern from the module; drop --match")
	default:
		return nil
	}
}

func openRepository(opts Options) (gitx.Client, error) {
	client := gitx.New(opts.Repository)
	if err := client.EnsureRepository(); err != nil {
		return gitx.Client{}, err
	}
	if opts.AllowDirty {
		return client, nil
	}

	dirty, err := client.IsDirty()
	if err != nil {
		return gitx.Client{}, err
	}
	if dirty {
		return gitx.Client{}, fmt.Errorf("working tree is dirty; rerun with --allow-dirty to bypass")
	}
	return client, nil
}

// analyzeCommits reads the commits since the base release, whose changes the
// next version carries, and whether a releasable one came after the current
// tag.
func analyzeCommits(client gitx.Client, state *releaseState) (ReleasePlan, error) {
	commits, err := state.scope.commitsSince(client, state.base.tag)
	if err != nil {
		return ReleasePlan{}, err
	}

	plan := ReleasePlan{
		CurrentTag:  state.current.tag,
		BaseTag:     state.base.tag,
		CommitCount: len(commits),
		Prefix:      state.prefix,
		IgnoredTags: state.ignored,
	}
	state.scope.describe(&plan)
	if state.current.tag != "" {
		plan.CurrentVersion = state.current.version.String()
	}
	for _, commit := range commits {
		plan.add(commit)
	}
	plan.HighestBump = plan.highestBumpLevel.String()

	state.level = plan.highestBumpLevel
	state.pending, err = hasPendingChanges(client, *state, commits)
	return plan, err
}

// hasPendingChanges reports a releasable commit after the current tag, among
// read, the commits read since the base release. When the current tag is the
// base release, the commits already read are those.
func hasPendingChanges(client gitx.Client, state releaseState, read []readCommit) (bool, error) {
	if state.current.tag == state.base.tag {
		return state.level > semver.BumpNone, nil
	}

	commits, err := commitsAfterCurrent(client, state, read)
	if err != nil {
		return false, err
	}
	for _, commit := range commits {
		if analyzeCommit(commit).Level > semver.BumpNone {
			return true, nil
		}
	}
	return false, nil
}

// commitsAfterCurrent is what the module reads after the current tag. A
// module read with several queries does not run them again when the base
// release precedes the current tag, as a final release precedes the
// prereleases after it: the commits after the current tag are then those of
// read that the history after it holds. A module read with one query, and
// any other case, reads them again.
func commitsAfterCurrent(client gitx.Client, state releaseState, read []readCommit) ([]readCommit, error) {
	if state.scope == nil || len(state.scope.queries()) == 1 {
		return state.scope.commitsSince(client, state.current.tag)
	}

	precedes := state.base.tag == ""
	if !precedes {
		var err error
		if precedes, err = client.IsAncestor(state.base.tag, state.current.tag); err != nil {
			return nil, err
		}
	}
	if !precedes {
		return state.scope.commitsSince(client, state.current.tag)
	}

	hashes, err := client.CommitOrder(state.current.tag)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(slices.Clone(read), func(commit readCommit) bool {
		return !slices.Contains(hashes, commit.Hash)
	}), nil
}

// decideNext returns the next version, and whether there is one to release.
func decideNext(state releaseState, opts Options) (semver.Version, bool, error) {
	preRelease := strings.TrimSpace(opts.PreRelease)

	var next semver.Version
	var err error
	switch {
	case strings.TrimSpace(opts.ReleaseAs) != "":
		next, err = versionAfterPrefix(opts.ReleaseAs, state.prefix)
	case opts.Promote:
		next, err = promotion(state.current.version)
	case preRelease != "":
		next, err = explicitPreRelease(state, preRelease, opts.StartVersion)
	case !state.pending:
		return semver.Version{}, false, nil
	case state.current.version.HasPreRelease():
		next, err = continuedPreRelease(state, opts.StartVersion)
	default:
		next, err = targetVersion(state, opts.StartVersion)
	}

	if err != nil {
		return semver.Version{}, false, err
	}
	return next, true, nil
}

func promotion(current semver.Version) (semver.Version, error) {
	if !current.HasPreRelease() {
		return semver.Version{}, fmt.Errorf("cannot promote a non-prerelease version")
	}
	return current.Promote(), nil
}

// targetVersion is the normal version the commits since the base release ask
// for. With prerelease tags and no final release, the prerelease already names
// the first release; with no tag at all, initial development starts at 0.1.0,
// as the SemVer FAQ suggests, unless a start version is given.
func targetVersion(state releaseState, startVersion string) (semver.Version, error) {
	switch {
	case state.base.tag != "":
		return state.base.version.Next(state.level), nil
	case state.current.tag != "":
		return state.current.version.Promote(), nil
	case strings.TrimSpace(startVersion) != "":
		return versionAfterPrefix(startVersion, state.prefix)
	default:
		return semver.Version{Minor: 1}, nil
	}
}

// continuedPreRelease continues the current prerelease while the commits ask
// for no more than its normal version, and otherwise opens a prerelease of the
// version they ask for, keeping the label.
func continuedPreRelease(state releaseState, startVersion string) (semver.Version, error) {
	target, err := targetVersion(state, startVersion)
	if err != nil {
		return semver.Version{}, err
	}

	current := state.current.version
	if semver.Compare(target, current.Promote()) <= 0 {
		return current.NextPreRelease()
	}
	return target.WithPreRelease(current.PreReleaseLabel())
}

// explicitPreRelease applies the label given with --pre. On the normal version
// of the current prerelease, the same label is incremented and another label
// replaces it; from a final release, or once the commits ask for more than
// that normal version, the label goes on the version they ask for.
func explicitPreRelease(state releaseState, label, startVersion string) (semver.Version, error) {
	current := state.current.version
	if !current.HasPreRelease() && !state.pending {
		return semver.Version{}, fmt.Errorf("cannot create a prerelease without releasable commits or an existing prerelease")
	}

	target, err := targetVersion(state, startVersion)
	if err != nil {
		return semver.Version{}, err
	}
	if !current.HasPreRelease() || semver.Compare(target, current.Promote()) > 0 {
		return target.WithPreRelease(label)
	}
	if label == current.PreRelease || label == current.PreReleaseLabel() {
		return current.NextPreRelease()
	}
	return current.Promote().WithPreRelease(label)
}

// verifyNext refuses a version SemVer does not allow as a new release: one
// that does not outrank the current version, or one already tagged anywhere
// in the repository. In a Go module it also refuses a major version the
// module path cannot carry.
func verifyNext(client gitx.Client, state releaseState, next semver.Version, nextTag string) error {
	current := state.current
	if current.tag != "" && semver.Compare(next, current.version) <= 0 {
		return fmt.Errorf("next version %s does not have higher precedence than the current version %s", next, current.version)
	}
	if err := state.scope.checkVersion(next); err != nil {
		return err
	}

	exists, err := client.TagExists(nextTag)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("tag %s already exists", nextTag)
	}
	return nil
}

// latestSemverTag returns the highest semantic version tag by SemVer
// precedence. Git orders tags lexicographically or by creation date, so
// neither `git tag -l` nor `git describe` can be trusted to answer this.
func latestSemverTag(client gitx.Client, tagMatch string, includeUnreachable bool, otherPrefixes []string) (string, semver.Version, error) {
	tags, err := collectVersionTags(client, tagMatch, includeUnreachable, otherPrefixes)
	if err != nil {
		return "", semver.Version{}, err
	}
	return tags.current.tag, tags.current.version, nil
}

// collectVersionTags reads the tags matching tagMatch, less the versions of
// the modules whose prefixes otherPrefixes lists. Matching tags of which none
// carries a semantic version are an error: bumpit would otherwise plan a
// first release over tags it cannot read.
func collectVersionTags(client gitx.Client, tagMatch string, includeUnreachable bool, otherPrefixes []string) (versionTags, error) {
	tags, err := listTags(client, tagMatch, includeUnreachable)
	if err != nil {
		return versionTags{}, err
	}

	prefix := tagPrefix(tagMatch)
	var collected versionTags
	for _, tag := range tags {
		if versionOfAnotherModule(tag, otherPrefixes) {
			continue
		}
		version, err := versionAfterPrefix(tag, prefix)
		if err != nil {
			collected.ignored = append(collected.ignored, IgnoredTag{Tag: tag, Reason: err.Error()})
			continue
		}
		collected.add(taggedVersion{tag: tag, version: version})
	}

	if collected.current.tag == "" && len(collected.ignored) > 0 {
		return versionTags{}, fmt.Errorf("no semantic version tags found among tags matching %q", tagMatch)
	}
	return collected, nil
}

func listTags(client gitx.Client, tagMatch string, includeUnreachable bool) ([]string, error) {
	if includeUnreachable {
		return client.Tags(tagMatch)
	}
	return client.TagsMergedIntoHEAD(tagMatch)
}

// add keeps candidate when it outranks the highest version, or the highest
// final release, seen so far.
func (v *versionTags) add(candidate taggedVersion) {
	if v.current.tag == "" || semver.Compare(candidate.version, v.current.version) > 0 {
		v.current = candidate
	}
	if candidate.version.HasPreRelease() {
		return
	}
	if v.base.tag == "" || semver.Compare(candidate.version, v.base.version) > 0 {
		v.base = candidate
	}
}

// analyzeCommit reads what a commit asks of the module. A breaking change
// that comes in only through a module this one ships is minor here: the
// contract it breaks is the shipped module's, and a breaking change of this
// module is marked in a commit of its own.
func analyzeCommit(commit readCommit) rules.Analysis {
	analysis := rules.Analyze(commit.Subject, commit.Body)
	if commit.shipped && analysis.Level == semver.BumpMajor {
		analysis.Level = semver.BumpMinor
		analysis.Bump = semver.BumpMinor.String()
		analysis.Reason = analysis.Reason + ", of a module this one ships: minor here"
		analysis.Breaking = false
	}
	return analysis
}

// add records the analysis of one commit.
func (r *ReleasePlan) add(commit readCommit) {
	analysis := analyzeCommit(commit)
	if analysis.Level > semver.BumpNone {
		r.RelevantCommits++
	}
	r.highestBumpLevel = semver.MaxBump(r.highestBumpLevel, analysis.Level)
	r.Analyses = append(r.Analyses, CommitDecision{
		Hash:     shortHash(commit.Hash),
		Subject:  commit.Subject,
		Type:     analysis.Type,
		Bump:     analysis.Bump,
		Reason:   analysis.Reason,
		Breaking: analysis.Breaking,
		Via:      commit.via,
	})
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}
