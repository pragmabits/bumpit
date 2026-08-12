package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newLatestCmd() *cobra.Command {
	flags := latestCommandFlags{}

	command := &cobra.Command{
		Use:   "latest",
		Short: "Print the latest semantic version tag",
		Long:  "bumpit latest prints the highest semantic version tag in the repository, ordered by SemVer precedence instead of the lexicographic order used by git tag -l.",
		RunE: func(command *cobra.Command, args []string) error {
			outputWriter := command.OutOrStdout()

			if _, err := flags.applyConfig(command); err != nil {
				return err
			}

			if err := validateOutputFormat(flags.output); err != nil {
				return err
			}

			latest, err := FindLatestTag(flags.options())
			if err != nil {
				return err
			}

			if flags.output == "json" {
				return writeJSON(outputWriter, latest)
			}

			if !latest.Found {
				_, err = fmt.Fprintln(outputWriter, "no tag")
				return err
			}

			if flags.noPrefix {
				_, err = fmt.Fprintln(outputWriter, latest.Version)
				return err
			}

			_, err = fmt.Fprintln(outputWriter, latest.Tag)
			return err
		},
	}

	flags.bind(command)
	return command
}
