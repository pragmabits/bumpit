package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRehearsalReadsWhatTheCommandsBeforeItWrote runs commands in order: each
// reads the file as those before it would have left it, and nothing reaches the
// disk.
func TestRehearsalReadsWhatTheCommandsBeforeItWrote(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	series := newRehearsal()

	add := []string{"modules", "add", "web/homepage", "-r", repositoryPath}
	if err := series.run(add); err != nil {
		t.Fatalf("the first add returned %v", err)
	}
	if err := series.run(add); err == nil || !strings.Contains(err.Error(), "web/homepage is declared already") {
		t.Errorf("the second add returned %v, want it to see the first", err)
	}
	if err := series.run([]string{"modules", "set", "web/homepage", "-T", "site/v", "-r", repositoryPath}); err != nil {
		t.Errorf("a set of what the add declared returned %v", err)
	}

	if err := series.run([]string{"modules", "add", "web/missing", "-r", repositoryPath}); err == nil || !strings.Contains(err.Error(), "web/missing matches no tracked file") {
		t.Fatalf("an add the proof refuses returned %v", err)
	}
	if err := series.run([]string{"modules", "remove", "web/missing", "-r", repositoryPath}); err == nil || !strings.Contains(err.Error(), "web/missing is not declared") {
		t.Errorf("a remove of what was refused returned %v, want it not to be there", err)
	}

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("the rehearsal created %s: %v", configPath, err)
	}
	if err := newRehearsal().run(add); err != nil {
		t.Errorf("an add in another rehearsal returned %v, want it to see only the disk", err)
	}
}

// TestRehearsalOfTheKeys covers the commands of a key, which find the file on
// disk and read what those before them would have left of it.
func TestRehearsalOfTheKeys(t *testing.T) {
	t.Parallel()

	repositoryPath := initHomepageRepository(t)
	configPath := filepath.Join(repositoryPath, "bumpit.yaml")
	content := "output: xml\ntagMessage: Release\n"
	writeFile(t, configPath, content)
	series := newRehearsal()

	unset := []string{"config", "unset", "output", "-r", repositoryPath}
	if err := series.run(unset); err != nil {
		t.Fatalf("the first unset returned %v", err)
	}
	if err := series.run(unset); err == nil || !strings.Contains(err.Error(), "output is not set") {
		t.Errorf("the second unset returned %v, want it to see the first", err)
	}
	expectFile(t, configPath, content)
}

func TestRefusalReason(t *testing.T) {
	t.Parallel()

	proof := fmt.Errorf("the edited file is refused, so it is left as it was: %w", Finding{Subject: "site", What: "it is wrong"})
	if got := refusalReason(proof); got != "site: it is wrong" {
		t.Errorf("refusalReason of a finding = %q", got)
	}
	if got := refusalReason(errors.New("modules is written as a list")); got != "modules is written as a list" {
		t.Errorf("refusalReason of another error = %q", got)
	}
}
