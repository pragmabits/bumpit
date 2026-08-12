package semver

import "testing"

func TestParse(t *testing.T) {
	t.Parallel()

	version, err := Parse("v1.2.3")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if version.Major != 1 || version.Minor != 2 || version.Patch != 3 {
		t.Fatalf("unexpected version: %#v", version)
	}
}

func TestParsePreRelease(t *testing.T) {
	t.Parallel()

	version, err := Parse("v1.2.3-beta.1+build.7")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if version.PreRelease != "beta.1" {
		t.Fatalf("unexpected prerelease: %q", version.PreRelease)
	}
	if version.BuildMetadata != "build.7" {
		t.Fatalf("unexpected build metadata: %q", version.BuildMetadata)
	}
}

func TestNext(t *testing.T) {
	t.Parallel()

	base := Version{Major: 1, Minor: 2, Patch: 3}

	if next := base.Next(BumpPatch); next.String() != "1.2.4" {
		t.Fatalf("patch bump mismatch: %s", next.String())
	}

	if next := base.Next(BumpMinor); next.String() != "1.3.0" {
		t.Fatalf("minor bump mismatch: %s", next.String())
	}

	if next := base.Next(BumpMajor); next.String() != "2.0.0" {
		t.Fatalf("major bump mismatch: %s", next.String())
	}
}

func TestPromote(t *testing.T) {
	t.Parallel()

	version := Version{Major: 1, Minor: 2, Patch: 3, PreRelease: "beta.1", BuildMetadata: "build.7"}
	if promoted := version.Promote(); promoted.String() != "1.2.3" {
		t.Fatalf("promote mismatch: %s", promoted.String())
	}
}

func TestCompare(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		left     string
		right    string
		expected int
	}{
		{"1.9.0", "1.10.0", -1},
		{"v1.10.0", "v1.9.0", 1},
		{"1.2.3", "1.2.3", 0},
		{"2.0.0", "10.0.0", -1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-rc.2", "1.0.0-rc.10", -1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-1", "1.0.0-alpha", -1},
		{"1.2.3+build.1", "1.2.3+build.9", 0},
	}

	for _, testCase := range testCases {
		left, err := Parse(testCase.left)
		if err != nil {
			t.Fatalf("Parse(%q) returned error: %v", testCase.left, err)
		}
		right, err := Parse(testCase.right)
		if err != nil {
			t.Fatalf("Parse(%q) returned error: %v", testCase.right, err)
		}

		if result := Compare(left, right); result != testCase.expected {
			t.Fatalf("Compare(%q, %q) = %d, want %d", testCase.left, testCase.right, result, testCase.expected)
		}
	}
}

func TestWithPreRelease(t *testing.T) {
	t.Parallel()

	version := Version{Major: 1, Minor: 2, Patch: 3}
	withPreRelease, err := version.WithPreRelease("rc.1")
	if err != nil {
		t.Fatalf("WithPreRelease returned error: %v", err)
	}

	if withPreRelease.String() != "1.2.3-rc.1" {
		t.Fatalf("prerelease mismatch: %s", withPreRelease.String())
	}
}
