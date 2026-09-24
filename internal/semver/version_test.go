package semver

import "testing"

func TestParse(t *testing.T) {
	t.Parallel()

	version, err := Parse("1.2.3")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if version.Major != 1 || version.Minor != 2 || version.Patch != 3 {
		t.Fatalf("unexpected version: %#v", version)
	}
}

func TestParseFollowsTheGrammar(t *testing.T) {
	t.Parallel()

	valid := []string{
		"0.0.0",
		"1.2.3-0",
		"1.2.3-0a",
		"1.2.3--1",
		"1.2.3-rc.1+build.007",
		"1.2.3+0.build",
	}
	for _, input := range valid {
		if _, err := Parse(input); err != nil {
			t.Errorf("Parse(%q) returned error: %v", input, err)
		}
	}

	invalid := []string{
		"v1.2.3",
		"01.2.3",
		"1.02.3",
		"1.2.03",
		"1.2",
		"1.2.3.4",
		"1.2.3-",
		"1.2.3-01",
		"1.2.3-rc.01",
		"1.2.3-rc..1",
		"1.2.3+",
		"1.2.3+build..1",
	}
	for _, input := range invalid {
		if version, err := Parse(input); err == nil {
			t.Errorf("Parse(%q) = %s, want an error", input, version)
		}
	}
}

func TestParsePreRelease(t *testing.T) {
	t.Parallel()

	version, err := Parse("1.2.3-beta.1+build.7")
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

func TestNextInMajorVersionZero(t *testing.T) {
	t.Parallel()

	base := Version{Major: 0, Minor: 4, Patch: 2}

	testCases := []struct {
		level    BumpLevel
		expected string
	}{
		{BumpPatch, "0.4.3"},
		{BumpMinor, "0.5.0"},
		{BumpMajor, "0.5.0"},
	}

	for _, testCase := range testCases {
		if next := base.Next(testCase.level); next.String() != testCase.expected {
			t.Errorf("Next(%s) from %s = %s, want %s", testCase.level, base, next, testCase.expected)
		}
	}
}

func TestNextPreRelease(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		preRelease string
		expected   string
	}{
		{"beta", "1.0.0-beta.1"},
		{"beta.1", "1.0.0-beta.2"},
		{"rc.beta", "1.0.0-rc.beta.1"},
		{"rc.1.alpha", "1.0.0-rc.1.alpha.1"},
	}

	for _, testCase := range testCases {
		version := Version{Major: 1, PreRelease: testCase.preRelease}
		next, err := version.NextPreRelease()
		if err != nil {
			t.Fatalf("NextPreRelease(%s) returned error: %v", version, err)
		}
		if next.String() != testCase.expected {
			t.Errorf("NextPreRelease(%s) = %s, want %s", version, next, testCase.expected)
		}
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
		{"1.10.0", "1.9.0", 1},
		{"1.2.3", "1.2.3", 0},
		{"2.0.0", "10.0.0", -1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-rc.2", "1.0.0-rc.10", -1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-1", "1.0.0-alpha", -1},
		{"1.2.3+build.1", "1.2.3+build.9", 0},
		{"1.0.0-alpha.1", "1.0.0-alpha.-1", -1},
		{"1.0.0-alpha.99999999999999999999", "1.0.0-alpha.100000000000000000000", -1},
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
			t.Errorf("Compare(%q, %q) = %d, want %d", testCase.left, testCase.right, result, testCase.expected)
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

func TestWithPreReleaseFollowsTheGrammar(t *testing.T) {
	t.Parallel()

	version := Version{Major: 1, Minor: 2, Patch: 3}
	for _, preRelease := range []string{"rc.01", "01", "rc..1", "beta!"} {
		if withPreRelease, err := version.WithPreRelease(preRelease); err == nil {
			t.Errorf("WithPreRelease(%q) = %s, want an error", preRelease, withPreRelease)
		}
	}
}
