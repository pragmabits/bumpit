package app

import (
	"fmt"
	"strings"

	"github.com/pragmabits/bumpit/internal/gitx"
	"github.com/pragmabits/bumpit/internal/rules"
	"github.com/pragmabits/bumpit/internal/semver"
)

type Options struct {
	Repository   string
	TagMatch     string
	AllowDirty   bool
	StartVersion string
	PreRelease   string
	Promote      bool
}

type CommitDecision struct {
	Hash     string `json:"hash"`
	Subject  string `json:"subject"`
	Type     string `json:"type,omitempty"`
	Bump     string `json:"bump"`
	Reason   string `json:"reason"`
	Breaking bool   `json:"breaking"`
}

type ReleasePlan struct {
	CurrentTag       string           `json:"current_tag,omitempty"`
	CurrentVersion   string           `json:"current_version,omitempty"`
	NextTag          string           `json:"next_tag,omitempty"`
	NextVersion      string           `json:"next_version,omitempty"`
	HasRelease       bool             `json:"has_release"`
	HighestBump      string           `json:"highest_bump"`
	CommitCount      int              `json:"commit_count"`
	RelevantCommits  int              `json:"relevant_commit_count"`
	Prefix           string           `json:"-"`
	Analyses         []CommitDecision `json:"commits"`
	highestBumpLevel semver.BumpLevel
}

func BuildReleasePlan(opts Options) (ReleasePlan, error) {
	if opts.Promote && strings.TrimSpace(opts.PreRelease) != "" {
		return ReleasePlan{}, fmt.Errorf("--promote and --pre cannot be used together")
	}

	client := gitx.New(opts.Repository)
	if err := client.EnsureRepository(); err != nil {
		return ReleasePlan{}, err
	}

	if !opts.AllowDirty {
		dirty, err := client.IsDirty()
		if err != nil {
			return ReleasePlan{}, err
		}
		if dirty {
			return ReleasePlan{}, fmt.Errorf("working tree is dirty; rerun with --allow-dirty to bypass")
		}
	}

	tagMatch := opts.TagMatch
	if tagMatch == "" {
		tagMatch = "v*"
	}

	lastTag, currentVersion, err := latestSemverTag(client, tagMatch, false)
	if err != nil {
		return ReleasePlan{}, err
	}

	commits, err := client.CommitsSince(lastTag)
	if err != nil {
		return ReleasePlan{}, err
	}

	plan := ReleasePlan{
		CurrentTag:  lastTag,
		CommitCount: len(commits),
		Prefix:      inferPrefix(tagMatch, lastTag),
	}

	if lastTag != "" {
		plan.CurrentVersion = currentVersion.String()
	}

	for _, commit := range commits {
		analysis := rules.Analyze(commit.Subject, commit.Body)
		if analysis.Level > semver.BumpNone {
			plan.RelevantCommits++
		}
		plan.highestBumpLevel = semver.MaxBump(plan.highestBumpLevel, analysis.Level)
		plan.Analyses = append(plan.Analyses, CommitDecision{
			Hash:     shortHash(commit.Hash),
			Subject:  commit.Subject,
			Type:     analysis.Type,
			Bump:     analysis.Bump,
			Reason:   analysis.Reason,
			Breaking: analysis.Breaking,
		})
	}

	plan.HighestBump = plan.highestBumpLevel.String()
	if plan.highestBumpLevel == semver.BumpNone && !opts.Promote && strings.TrimSpace(opts.PreRelease) == "" {
		return plan, nil
	}

	nextVersion, err := nextVersion(currentVersion, plan.highestBumpLevel, lastTag == "", opts)
	if err != nil {
		return ReleasePlan{}, err
	}

	plan.HasRelease = true
	plan.NextVersion = nextVersion.String()
	plan.NextTag = nextVersion.Tag(plan.Prefix)

	return plan, nil
}

func nextVersion(current semver.Version, level semver.BumpLevel, isInitial bool, opts Options) (semver.Version, error) {
	if opts.Promote {
		if !current.HasPreRelease() {
			return semver.Version{}, fmt.Errorf("cannot promote a non-prerelease version")
		}
		return current.Promote(), nil
	}

	preRelease := strings.TrimSpace(opts.PreRelease)
	if preRelease != "" {
		if level == semver.BumpNone {
			if current.HasPreRelease() {
				if preRelease == current.PreRelease || preRelease == current.PreReleaseLabel() {
					return current.NextPreRelease()
				}
				return current.WithPreRelease(preRelease)
			}
			return semver.Version{}, fmt.Errorf("cannot create a prerelease without releasable commits or an existing prerelease")
		}

		baseVersion, err := nextBaseVersion(current, level, isInitial, opts.StartVersion)
		if err != nil {
			return semver.Version{}, err
		}
		return baseVersion.WithPreRelease(preRelease)
	}

	if current.HasPreRelease() {
		if level == semver.BumpNone {
			return semver.Version{}, fmt.Errorf("no releasable commits found for prerelease continuation")
		}

		baseVersion, err := nextBaseVersion(current, level, isInitial, opts.StartVersion)
		if err != nil {
			return semver.Version{}, err
		}

		label := current.PreReleaseLabel()
		if label == "" {
			label = current.PreRelease
		}
		return baseVersion.WithPreRelease(label)
	}

	return nextBaseVersion(current, level, isInitial, opts.StartVersion)
}

func nextBaseVersion(current semver.Version, level semver.BumpLevel, isInitial bool, startVersion string) (semver.Version, error) {
	if !isInitial {
		return current.Next(level), nil
	}

	if strings.TrimSpace(startVersion) != "" {
		return semver.Parse(startVersion)
	}

	switch level {
	case semver.BumpMajor:
		return semver.Version{Major: 1}, nil
	case semver.BumpMinor:
		return semver.Version{Major: 0, Minor: 1}, nil
	case semver.BumpPatch:
		return semver.Version{Major: 0, Minor: 0, Patch: 1}, nil
	default:
		return semver.Version{}, nil
	}
}

// latestSemverTag returns the highest semantic version tag by SemVer
// precedence. Git orders tags lexicographically or by creation date, so
// neither `git tag -l` nor `git describe` can be trusted to answer this.
func latestSemverTag(client gitx.Client, tagMatch string, includeUnreachable bool) (string, semver.Version, error) {
	var tags []string
	var err error
	if includeUnreachable {
		tags, err = client.Tags(tagMatch)
	} else {
		tags, err = client.TagsMergedIntoHEAD(tagMatch)
	}
	if err != nil {
		return "", semver.Version{}, err
	}

	latestTag := ""
	latestVersion := semver.Version{}
	invalidTagCount := 0
	for _, tag := range tags {
		version, err := semver.Parse(tag)
		if err != nil {
			invalidTagCount++
			continue
		}
		if latestTag == "" || semver.Compare(version, latestVersion) > 0 {
			latestTag = tag
			latestVersion = version
		}
	}

	if latestTag != "" {
		return latestTag, latestVersion, nil
	}

	if invalidTagCount > 0 {
		return "", semver.Version{}, fmt.Errorf("no semantic version tags found among tags matching %q", tagMatch)
	}

	return "", semver.Version{}, nil
}

func inferPrefix(match, lastTag string) string {
	if strings.HasPrefix(lastTag, "v") {
		return "v"
	}
	if strings.HasPrefix(match, "v") {
		return "v"
	}
	return ""
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}
