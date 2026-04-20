package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestApplyConfigUsesConfigValuesWhenFlagsAreUnset(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	repositoryPath := filepath.Join(directory, "repository")
	if err := os.Mkdir(repositoryPath, 0o755); err != nil {
		t.Fatalf("Mkdir returned error: %v", err)
	}
	configPath := filepath.Join(directory, "bumpit.yaml")
	content := []byte("repository: " + repositoryPath + "\ntagMatch: release-*\nstartVersion: 2.0.0\nallowDirty: true\noutput: json\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	flags := releasePlanFlags{}
	command := &cobra.Command{Use: "next"}
	flags.bind(command, true)
	if err := command.Flags().Set("config", configPath); err != nil {
		t.Fatalf("Set(config) returned error: %v", err)
	}

	config, err := flags.applyConfig(command)
	if err != nil {
		t.Fatalf("applyConfig returned error: %v", err)
	}
	if config == nil {
		t.Fatalf("expected config to be loaded")
	}

	if flags.repository != repositoryPath {
		t.Fatalf("unexpected repository: %s", flags.repository)
	}
	if flags.tagMatch != "release-*" {
		t.Fatalf("unexpected match: %s", flags.tagMatch)
	}
	if flags.startVersion != "2.0.0" {
		t.Fatalf("unexpected start version: %s", flags.startVersion)
	}
	if !flags.allowDirty {
		t.Fatalf("expected allowDirty to be true")
	}
	if flags.output != "json" {
		t.Fatalf("unexpected output: %s", flags.output)
	}
}

func TestApplyConfigPreservesExplicitFlags(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	configPath := filepath.Join(directory, "bumpit.yaml")
	content := []byte("tagMatch: release-*\nallowDirty: true\noutput: json\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	flags := releasePlanFlags{}
	command := &cobra.Command{Use: "next"}
	flags.bind(command, true)
	if err := command.Flags().Set("config", configPath); err != nil {
		t.Fatalf("Set(config) returned error: %v", err)
	}

	if err := command.Flags().Set("match", "custom-*"); err != nil {
		t.Fatalf("Set(match) returned error: %v", err)
	}
	if err := command.Flags().Set("allow-dirty", "false"); err != nil {
		t.Fatalf("Set(allow-dirty) returned error: %v", err)
	}
	if err := command.Flags().Set("output", "text"); err != nil {
		t.Fatalf("Set(output) returned error: %v", err)
	}

	config, err := flags.applyConfig(command)
	if err != nil {
		t.Fatalf("applyConfig returned error: %v", err)
	}
	if config == nil {
		t.Fatalf("expected config to be loaded")
	}

	if flags.tagMatch != "custom-*" {
		t.Fatalf("match was overwritten: %s", flags.tagMatch)
	}
	if flags.allowDirty {
		t.Fatalf("allowDirty was overwritten")
	}
	if flags.output != "text" {
		t.Fatalf("output was overwritten: %s", flags.output)
	}
}

func TestApplyConfigReturnsTagSection(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	configPath := filepath.Join(directory, "bumpit.yaml")
	content := []byte("tagMessage: Release from config\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	flags := tagCommandFlags{}
	command := &cobra.Command{Use: "tag"}
	flags.bind(command)
	if err := command.Flags().Set("config", configPath); err != nil {
		t.Fatalf("Set(config) returned error: %v", err)
	}

	config, err := flags.applyConfig(command)
	if err != nil {
		t.Fatalf("applyConfig returned error: %v", err)
	}
	if config == nil {
		t.Fatalf("expected config to be loaded")
	}
	if config.TagMessage != "Release from config" {
		t.Fatalf("unexpected tag message: %s", config.TagMessage)
	}
}
