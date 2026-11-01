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
	writeModuleLines(&text, plan)
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
		fmt.Fprintf(&text, "- %s %s [%s: %s]", commitAnalysis.Hash, commitAnalysis.Subject, commitAnalysis.Bump, commitAnalysis.Reason)
		if commitAnalysis.Via != "" {
			fmt.Fprintf(&text, " via %s", commitAnalysis.Via)
		}
		text.WriteString("\n")
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

// writeModuleLines writes which module the plan versions and what it reads
// beyond its own directory or leaves out of it.
func writeModuleLines(text *strings.Builder, plan ReleasePlan) {
	switch {
	case plan.Directory != "":
		fmt.Fprintf(text, "Module: %s (declared in bumpit.yaml)\n", plan.Directory)
	case plan.Module != "":
		fmt.Fprintf(text, "Module: %s\n", plan.Module)
	}

	lists := []struct {
		label  string
		values []string
	}{
		{label: "Paths", values: plan.Paths},
		{label: "Paths from the Node packages", values: plan.DerivedPaths},
		{label: "Shipped by", values: plan.ShippedBy},
		{label: "Ships", values: plan.Ships},
		{label: "Leaves out", values: plan.LeavesOut},
	}
	for _, list := range lists {
		if len(list.values) > 0 {
			fmt.Fprintf(text, "%s: %s\n", list.label, strings.Join(list.values, ", "))
		}
	}
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
