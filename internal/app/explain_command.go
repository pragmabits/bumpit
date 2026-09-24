package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func newExplainCmd() *cobra.Command {
	flags := releasePlanFlags{}

	command := &cobra.Command{
		Use:   "explain",
		Short: "Explain how the next version was calculated",
		RunE: func(command *cobra.Command, args []string) error {
			outputWriter := command.OutOrStdout()

			if _, err := flags.applyConfig(command); err != nil {
				return err
			}

			releasePlan, err := BuildReleasePlan(flags.options())
			if err != nil {
				return err
			}

			if err := validateOutputFormat(flags.output); err != nil {
				return err
			}
			if flags.output == "json" {
				return writeJSON(outputWriter, releasePlan)
			}
			return writeExplanation(outputWriter, releasePlan)
		},
	}

	flags.bind(command, true)
	return command
}

// writeExplanation writes the plan as the text explain prints. The text is
// built first and written once, so a failing writer is reported by one error.
func writeExplanation(writer io.Writer, plan ReleasePlan) error {
	var text strings.Builder
	if plan.Module != "" {
		fmt.Fprintf(&text, "Module: %s\n", plan.Module)
	}
	fmt.Fprintf(&text, "Current tag: %s\n", valueOr(plan.CurrentTag, "none"))
	fmt.Fprintf(&text, "Base release: %s\n", valueOr(plan.BaseTag, "none"))
	fmt.Fprintf(&text, "Next version: %s\n", valueOr(plan.NextTag, "no release"))
	fmt.Fprintf(&text, "Highest bump: %s\n", plan.HighestBump)
	fmt.Fprintf(&text, "Commits since base release: %d\n", plan.CommitCount)
	fmt.Fprintf(&text, "Releasable commits: %d\n", plan.RelevantCommits)

	if len(plan.Analyses) == 0 {
		text.WriteString("No commits found in range.\n")
	} else {
		text.WriteString("\nCommit analysis:\n")
	}
	for _, commitAnalysis := range plan.Analyses {
		fmt.Fprintf(&text, "- %s %s [%s: %s]\n", commitAnalysis.Hash, commitAnalysis.Subject, commitAnalysis.Bump, commitAnalysis.Reason)
	}

	if len(plan.IgnoredTags) > 0 {
		text.WriteString("\nIgnored tags:\n")
	}
	for _, ignored := range plan.IgnoredTags {
		fmt.Fprintf(&text, "- %s: %s\n", ignored.Tag, ignored.Reason)
	}

	if len(plan.Dependents) > 0 {
		text.WriteString("\nDependents requiring an older version:\n")
	}
	for _, dependent := range plan.Dependents {
		fmt.Fprintf(&text, "- %s (%s) requires %s\n", dependent.Directory, dependent.Path, dependent.Requires)
	}

	_, err := io.WriteString(writer, text.String())
	return err
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
