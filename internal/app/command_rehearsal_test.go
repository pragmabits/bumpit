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
	if refused := rehearse(t, series, add); refused != "" {
		t.Fatalf("the first add was refused: %s", refused)
	}
	if refused := rehearse(t, series, add); !strings.Contains(refused, "web/homepage is declared already") {
		t.Errorf("the second add was refused with %q, want it to see the first", refused)
	}
	if refused := rehearse(t, series, []string{"modules", "set", "web/homepage", "-T", "site/v", "-r", repositoryPath}); refused != "" {
		t.Errorf("a set of what the add declared was refused: %s", refused)
	}

	if refused := rehearse(t, series, []string{"modules", "add", "web/missing", "-r", repositoryPath}); !strings.Contains(refused, "web/missing matches no tracked file") {
		t.Fatalf("an add the proof refuses was refused with %q", refused)
	}
	if refused := rehearse(t, series, []string{"modules", "remove", "web/missing", "-r", repositoryPath}); !strings.Contains(refused, "web/missing is not declared") {
		t.Errorf("a remove of what was refused was refused with %q, want it not to be there", refused)
	}

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("the rehearsal created %s: %v", configPath, err)
	}
	if refused := rehearse(t, newRehearsal(), add); refused != "" {
		t.Errorf("an add in another rehearsal was refused: %s, want it to see only the disk", refused)
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
	if refused := rehearse(t, series, unset); refused != "" {
		t.Fatalf("the first unset was refused: %s", refused)
	}
	if refused := rehearse(t, series, unset); !strings.Contains(refused, "output is not set") {
		t.Errorf("the second unset was refused with %q, want it to see the first", refused)
	}
	expectFile(t, configPath, content)
}

// rehearse runs words in series and returns why they would be refused. Git fails
// to answer in no fixture here, so that fails the test.
func rehearse(t *testing.T, series *rehearsal, words []string) string {
	t.Helper()

	refused, err := series.run(words)
	if err != nil {
		t.Fatalf("the rehearsal of %v failed: %v", words, err)
	}
	return refused
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
