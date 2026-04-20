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
