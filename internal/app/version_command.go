package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

var buildVersion = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		RunE: func(command *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(command.OutOrStdout(), buildVersion)
			return err
		},
	}
}
