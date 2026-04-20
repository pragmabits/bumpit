package app

import (
	"fmt"

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

			currentTag := releasePlan.CurrentTag
			if currentTag == "" {
				currentTag = "none"
			}

			nextTag := releasePlan.NextTag
			if nextTag == "" {
				nextTag = "no release"
			}

			if _, err := fmt.Fprintf(outputWriter, "Current tag: %s\n", currentTag); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(outputWriter, "Next version: %s\n", nextTag); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(outputWriter, "Highest bump: %s\n", releasePlan.HighestBump); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(outputWriter, "Commits since tag: %d\n", releasePlan.CommitCount); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(outputWriter, "Releasable commits: %d\n", releasePlan.RelevantCommits); err != nil {
				return err
			}

			if len(releasePlan.Analyses) == 0 {
				_, err = fmt.Fprintln(outputWriter, "No commits found in range.")
				return err
			}

			if _, err := fmt.Fprintln(outputWriter, "\nCommit analysis:"); err != nil {
				return err
			}
			for _, commitAnalysis := range releasePlan.Analyses {
				if _, err := fmt.Fprintf(outputWriter, "- %s %s [%s: %s]\n", commitAnalysis.Hash, commitAnalysis.Subject, commitAnalysis.Bump, commitAnalysis.Reason); err != nil {
					return err
				}
			}

			return nil
		},
	}

	flags.bind(command, true)
	return command
}
