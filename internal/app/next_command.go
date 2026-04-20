package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newNextCmd() *cobra.Command {
	flags := releasePlanFlags{}

	command := &cobra.Command{
		Use:   "next",
		Short: "Print the next version tag",
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

			if !releasePlan.HasRelease {
				_, err = fmt.Fprintln(outputWriter, "no release")
				return err
			}

			_, err = fmt.Fprintln(outputWriter, releasePlan.NextTag)
			return err
		},
	}

	flags.bind(command, true)
	return command
}
