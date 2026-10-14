package app

import (
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"text/tabwriter"

	"github.com/pragmabits/bumpit/internal/gitx"
	"github.com/spf13/cobra"
)

func newTagCmd() *cobra.Command {
	flags := tagCommandFlags{}

	command := &cobra.Command{
		Use:   "tag",
		Short: "Create the computed annotated tag locally",
		Long:  "bumpit tag creates the annotated tag of the next version at HEAD, locally. With --all-modules it plans every Go module of the repository and creates no tag when a plan fails; it then tags each module that has a release, and holds the ones that require another module of the repository below its newest version, since tagged now they would require the older one.",
		RunE: func(command *cobra.Command, args []string) error {
			outputWriter := command.OutOrStdout()

			config, err := flags.applyConfig(command)
			if err != nil {
				return err
			}
			if flags.allModules {
				return tagAllModules(command, flags, config)
			}

			plan, err := BuildReleasePlan(flags.options())
			if err != nil {
				return err
			}
			if !plan.HasRelease {
				return fmt.Errorf("no releasable commits found; refusing to create a tag")
			}

			client := gitx.New(flags.repository)
			tagMessage := resolveTagMessage(flags.message, config, plan.NextTag)
			if err := client.CreateAnnotatedTag(plan.NextTag, tagMessage); err != nil {
				return err
			}

			_, err = fmt.Fprintf(outputWriter, "created tag %s\n", plan.NextTag)
			return err
		},
	}

	flags.bind(command)
	return command
}

func resolveTagMessage(commandLineMessage string, config *Config, nextTag string) string {
	if commandLineMessage != "" {
		return commandLineMessage
	}
	if config != nil && config.TagMessage != "" {
		return config.TagMessage
	}
	return "Release " + nextTag
}

// moduleOutcome is what tagging every module did to one of them.
type moduleOutcome struct {
	directory string
	text      string
}

// tagAllModules tags every Go module that has a release and is not held. It
// plans every module before it creates a tag, creates none when a plan fails,
// and fails when it creates none.
func tagAllModules(command *cobra.Command, flags tagCommandFlags, config *Config) error {
	if err := flags.checkAllModules(command.Flags()); err != nil {
		return err
	}

	releases, err := ListModules(Options{Repository: flags.repository, AllowDirty: flags.allowDirty, StartVersion: flags.startVersion})
	if err != nil {
		return err
	}
	if len(releases) == 0 {
		return fmt.Errorf("no Go module found; --all-modules tags the Go modules of the repository")
	}
	if err := planFailures(releases); err != nil {
		return err
	}

	client := gitx.New(flags.repository)
	outcomes, created, tagErr := createModuleTags(client, releases, flags.message, config)
	if err := writeOutcomes(command.OutOrStdout(), outcomes); err != nil {
		return err
	}
	if tagErr != nil {
		return tagErr
	}
	if created == 0 {
		return fmt.Errorf("no module can be tagged; refusing to create a tag")
	}
	return nil
}

// planFailures joins the error of every module whose plan failed.
func planFailures(releases []ModuleRelease) error {
	var failures []error
	for _, release := range releases {
		if release.Error != "" {
			failures = append(failures, fmt.Errorf("%s: %s", release.Directory, release.Error))
		}
	}
	if len(failures) == 0 {
		return nil
	}
	return fmt.Errorf("no tag created; planning failed for these modules:\n%w", errors.Join(failures...))
}

// createModuleTags tags, in order, each module that has a release and is not
// held, and stops at the first tag git fails to create. It returns what it did
// to each module it reached and how many tags it created.
func createModuleTags(client gitx.Client, releases []ModuleRelease, commandLineMessage string, config *Config) ([]moduleOutcome, int, error) {
	outcomes := make([]moduleOutcome, 0, len(releases))
	created := 0
	for _, release := range releases {
		outcome := moduleOutcome{directory: release.Directory}
		switch {
		case !release.HasRelease:
			outcome.text = "no release"
		case release.Hold != nil:
			outcome.text = "held: " + holdReason(release.Hold) + "; " + holdAdvice(release)
		default:
			tagMessage := resolveTagMessage(commandLineMessage, config, release.NextTag)
			if err := client.CreateAnnotatedTag(release.NextTag, tagMessage); err != nil {
				return outcomes, created, err
			}
			outcome.text = "created tag " + release.NextTag
			created++
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, created, nil
}

// holdAdvice says how to release a held module: update its go.mod once the
// modules it requires are tagged, or, in a cycle, release one module alone.
func holdAdvice(release ModuleRelease) string {
	if release.Hold.Cycle {
		return "a requirement cycle: release one of its modules with --module"
	}

	var waiting []string
	for _, requirement := range release.Hold.Requirements {
		if requirement.Held {
			waiting = append(waiting, requirement.Path)
		}
	}
	if len(waiting) > 0 {
		return "waits for " + strings.Join(waiting, ", ") + ", held too"
	}
	return "update " + path.Join(release.Directory, "go.mod") + ", commit and rerun"
}

// writeOutcomes writes one row per module: its directory and what tagging it
// did.
func writeOutcomes(writer io.Writer, outcomes []moduleOutcome) error {
	var text strings.Builder
	table := tabwriter.NewWriter(&text, 0, 0, 2, ' ', 0)
	for _, outcome := range outcomes {
		fmt.Fprintf(table, "%s\t%s\n", outcome.directory, outcome.text)
	}
	if err := table.Flush(); err != nil {
		return err
	}

	_, err := io.WriteString(writer, text.String())
	return err
}
