package app

import (
	"fmt"

	"github.com/pragmabits/bumpit/internal/gitx"
)

// LatestOptions selects the tags bumpit latest reads. Module and TagMatch
// work as in Options; All adds the tags HEAD cannot reach.
type LatestOptions struct {
	Repository string
	TagMatch   string
	All        bool
	Module     string
}

// LatestResult is the highest version tag, and the version it carries.
type LatestResult struct {
	Tag     string `json:"tag,omitempty"`
	Version string `json:"version,omitempty"`
	Found   bool   `json:"found"`
}

// FindLatestTag returns the tag with the highest SemVer precedence, or a
// result with Found false when no tag matches.
func FindLatestTag(opts LatestOptions) (LatestResult, error) {
	client := gitx.New(opts.Repository)
	if err := client.EnsureRepository(); err != nil {
		return LatestResult{}, err
	}

	if opts.Module != "" && opts.TagMatch != "" {
		return LatestResult{}, fmt.Errorf("--module sets the tag pattern from the module; drop --match")
	}
	scope, err := resolveModuleScope(client, opts.Repository, opts.Module, opts.TagMatch)
	if err != nil {
		return LatestResult{}, err
	}

	tag, version, err := latestSemverTag(client, tagPattern(scope, opts.TagMatch), opts.All)
	if err != nil {
		return LatestResult{}, err
	}
	if tag == "" {
		return LatestResult{}, nil
	}

	return LatestResult{Tag: tag, Version: version.String(), Found: true}, nil
}
