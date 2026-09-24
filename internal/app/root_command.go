// Package app is the bumpit command: its subcommands, flags and config, and
// the release plan they share, which reads the version tags and the commits
// after them and computes the next version.
package app

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Execute runs bumpit with the given arguments and returns the exit code.
func Execute(arguments []string) int {
	rootCommand := newRootCmd()
	rootCommand.SetArgs(arguments)

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
		Long:          "bumpit reads the commits after the latest semantic version tag and computes the next release using Conventional Commits and SemVer rules. In a Go repository it versions one module at a time: the root module by default, or the one --module names, from its own tags and the commits under its directory.",
		Version:       version(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	rootCommand.SetOut(os.Stdout)
	rootCommand.SetErr(os.Stderr)
	rootCommand.AddCommand(newLatestCmd())
	rootCommand.AddCommand(newNextCmd())
	rootCommand.AddCommand(newExplainCmd())
	rootCommand.AddCommand(newTagCmd())
	rootCommand.AddCommand(newModulesCmd())
	rootCommand.AddCommand(newVersionCmd())
	return rootCommand
}
