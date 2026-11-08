package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFollowLinks covers the links a name leads through: those the repository
// tracks are followed, one after another and below the name too, and a link only
// the working tree holds, one out of the repository, one to an absolute path and
// a loop leave the name as it is written.
func TestFollowLinks(t *testing.T) {
	t.Parallel()

	top := t.TempDir()
	if err := os.MkdirAll(filepath.Join(top, "web", "back", "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	links := map[string]string{
		"front":    "web/back",
		"chain":    "front",
		"loose":    "web/back",
		"out":      "../elsewhere",
		"absolute": top,
		"loop":     "loop",
	}
	for link, target := range links {
		if err := os.Symlink(target, filepath.Join(top, link)); err != nil {
			t.Fatalf("Symlink returned error: %v", err)
		}
	}
	tracked := newTrackedTree([]string{"web/back/sub/index.html", "front", "chain", "out", "absolute", "loop"})

	for name, want := range map[string]string{
		"front":     "web/back",
		"chain":     "web/back",
		"front/sub": "web/back/sub",
		"web/back":  "web/back",
		"loose":     "loose",
		"out":       "out",
		"absolute":  "absolute",
		"loop":      "loop",
		"missing":   "missing",
	} {
		if got := tracked.followLinks(top, name); got != want {
			t.Errorf("followLinks(%q) = %q, want %q", name, got, want)
		}
	}
}
