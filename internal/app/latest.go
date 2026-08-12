package app

import (
	"github.com/pragmabits/bumpit/internal/gitx"
)

type LatestOptions struct {
	Repository string
	TagMatch   string
	All        bool
}

type LatestResult struct {
	Tag     string `json:"tag,omitempty"`
	Version string `json:"version,omitempty"`
	Found   bool   `json:"found"`
}

func FindLatestTag(opts LatestOptions) (LatestResult, error) {
	client := gitx.New(opts.Repository)
	if err := client.EnsureRepository(); err != nil {
		return LatestResult{}, err
	}

	tagMatch := opts.TagMatch
	if tagMatch == "" {
		tagMatch = "v*"
	}

	tag, version, err := latestSemverTag(client, tagMatch, opts.All)
	if err != nil {
		return LatestResult{}, err
	}
	if tag == "" {
		return LatestResult{}, nil
	}

	return LatestResult{Tag: tag, Version: version.String(), Found: true}, nil
}
