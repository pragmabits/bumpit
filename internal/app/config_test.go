package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "bumpit.yaml")
	content := []byte("repository: .\ntagMatch: release-*\nstartVersion: 1.2.3\nallowDirty: true\noutput: json\ntagMessage: custom\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	defaults := Config{
		Repository: ".",
		TagMatch:   "v*",
		Output:     "text",
	}

	config, resolvedConfigPath, err := LoadConfig(path, defaults)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if resolvedConfigPath != path {
		t.Fatalf("unexpected resolved path: %s", resolvedConfigPath)
	}
	if config.TagMatch != "release-*" {
		t.Fatalf("unexpected tagMatch: %s", config.TagMatch)
	}
	if config.Repository != directory {
		t.Fatalf("unexpected repository: %s", config.Repository)
	}
	if config.StartVersion != "1.2.3" {
		t.Fatalf("unexpected startVersion: %s", config.StartVersion)
	}
	if config.AllowDirty == nil || !*config.AllowDirty {
		t.Fatalf("unexpected allowDirty: %#v", config.AllowDirty)
	}
	if config.Output != "json" {
		t.Fatalf("unexpected output: %s", config.Output)
	}
	if config.TagMessage != "custom" {
		t.Fatalf("unexpected tagMessage: %s", config.TagMessage)
	}
}

func TestLoadConfigSupportsPreReleaseSettings(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "bumpit.yaml")
	content := []byte("repository: .\ntagMatch: v*\nstartVersion: 1.2.3\nallowDirty: true\noutput: json\npreRelease: beta\npromote: false\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	config, _, err := LoadConfig(path, Config{Repository: ".", TagMatch: "v*", Output: "text"})
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if config.PreRelease != "beta" {
		t.Fatalf("unexpected preRelease: %s", config.PreRelease)
	}
	if config.Promote == nil || *config.Promote {
		t.Fatalf("unexpected promote: %#v", config.Promote)
	}
}

func TestLoadConfigAcceptsStartVersionWithPrefix(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "bumpit.yaml")
	content := []byte("repository: .\ntagMatch: v*\nstartVersion: v1.2.3\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	config, _, err := LoadConfig(path, Config{Repository: ".", TagMatch: "v*", Output: "text"})
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if config.StartVersion != "v1.2.3" {
		t.Fatalf("unexpected startVersion: %s", config.StartVersion)
	}
}

func TestDiscoverConfig(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "bumpit.yaml")
	if err := os.WriteFile(path, []byte("tagMatch: v*\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, resolvedConfigPath, err := DiscoverConfig(directory, Config{Repository: ".", TagMatch: "v*", Output: "text"})
	if err != nil {
		t.Fatalf("DiscoverConfig returned error: %v", err)
	}
	if resolvedConfigPath != path {
		t.Fatalf("unexpected resolved path: %s", resolvedConfigPath)
	}
}

func TestDiscoverConfigNotFound(t *testing.T) {
	t.Parallel()

	_, _, err := DiscoverConfig(t.TempDir(), Config{Repository: ".", TagMatch: "v*", Output: "text"})
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfigRejectsUnknownField(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "bumpit.yaml")
	content := []byte("repository: .\nunknownKey: true\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, _, err := LoadConfig(path, Config{Repository: ".", TagMatch: "v*", Output: "text"})
	if err == nil {
		t.Fatalf("expected unknown field error")
	}
}

func TestLoadConfigRejectsPreReleaseAndPromoteTogether(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "bumpit.yaml")
	content := []byte("repository: .\npreRelease: beta\npromote: true\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, _, err := LoadConfig(path, Config{Repository: ".", TagMatch: "v*", Output: "text"})
	if err == nil {
		t.Fatalf("expected preRelease/promote conflict error")
	}
}
