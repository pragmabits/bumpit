package app

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	moduleVersion := func(version string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/pragmabits/bumpit", Version: version}}
	}

	tests := []struct {
		name   string
		linked string
		info   *debug.BuildInfo
		want   string
	}{
		{name: "set at link time", linked: "v1.2.3-4-gabcdef0", info: moduleVersion("v0.3.0"), want: "v1.2.3-4-gabcdef0"},
		{name: "recorded by go install", linked: "dev", info: moduleVersion("v0.3.0"), want: "v0.3.0"},
		{name: "recorded from git for a local build", linked: "dev", info: moduleVersion("v0.3.1-0.20260924150000-abcdef012345+dirty"), want: "v0.3.1-0.20260924150000-abcdef012345+dirty"},
		{name: "development build", linked: "dev", info: moduleVersion("(devel)"), want: "dev"},
		{name: "no version recorded", linked: "dev", info: moduleVersion(""), want: "dev"},
		{name: "no build information", linked: "dev", info: nil, want: "dev"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveVersion(test.linked, test.info); got != test.want {
				t.Fatalf("resolveVersion(%q) = %q, want %q", test.linked, got, test.want)
			}
		})
	}
}
