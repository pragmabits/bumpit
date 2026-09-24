package rules

import "testing"

func TestAnalyze(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		subject string
		body    string
		want    string
	}{
		{name: "feature", subject: "feat(api): add export", want: "minor"},
		{name: "fix", subject: "fix(cli): handle nil pointer", want: "patch"},
		{name: "breaking footer", subject: "chore: cleanup", body: "BREAKING CHANGE: new API", want: "major"},
		{name: "breaking marker", subject: "feat!: remove endpoint", want: "major"},
		{name: "ignored", subject: "docs: update readme", want: "none"},
		{name: "plain message", subject: "misc commit", want: "none"},
		{name: "breaking footer after a body", subject: "refactor: reshape", body: "Reshape the API.\n\nBREAKING CHANGE: the old call is gone", want: "major"},
		{name: "breaking footer with hyphen", subject: "fix: rename flag", body: "BREAKING-CHANGE: --old is gone", want: "major"},
		{name: "breaking phrase inside body text", subject: "fix: typo", body: "This is not a BREAKING CHANGE: only a typo.", want: "patch"},
		{name: "breaking footer in lowercase", subject: "fix: typo", body: "breaking change: not the token", want: "patch"},
		{name: "breaking footer on non-conventional commit", subject: "update readme", body: "BREAKING CHANGE: nothing really", want: "none"},
		{name: "no space after colon", subject: "feat:no space", want: "none"},
		{name: "no description", subject: "feat:", want: "none"},
		{name: "deprecation footer", subject: "docs: mark Load as deprecated", body: "Deprecated: use LoadFile instead", want: "minor"},
		{name: "deprecation footer in any case", subject: "fix: handle nil config", body: "DEPRECATED: the nil config", want: "minor"},
		{name: "deprecation inside body text", subject: "docs: history", body: "The old API was Deprecated: long ago.", want: "none"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := Analyze(test.subject, test.body)
			if got.Bump != test.want {
				t.Fatalf("Analyze(%q) = %q, want %q", test.subject, got.Bump, test.want)
			}
		})
	}
}
