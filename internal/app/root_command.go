package app

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func Execute(args []string) int {
	rootCommand := newRootCmd()
	rootCommand.SetArgs(args)

	if err := rootCommand.Execute(); err != nil {
		fmt.Fprintln(rootCommand.ErrOrStderr(), err)
		return 1
	}
	return 0
}

func newRootCmd() *cobra.Command {
	rootCommand := &cobra.Command{
		Use:           "bumpit",
		Short:         "Compute the next semantic version from local git history",
		Long:          "bumpit reads the commits after the latest semantic version tag and computes the next release using Conventional Commits and SemVer rules.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	rootCommand.SetOut(os.Stdout)
	rootCommand.SetErr(os.Stderr)
	rootCommand.AddCommand(newNextCmd())
	rootCommand.AddCommand(newExplainCmd())
	rootCommand.AddCommand(newTagCmd())
	rootCommand.AddCommand(newVersionCmd())
	return rootCommand
}
