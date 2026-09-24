package app

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newModulesCmd() *cobra.Command {
	flags := modulesCommandFlags{}

	command := &cobra.Command{
		Use:   "modules",
		Short: "List the Go modules of the repository and their next versions",
		Long:  "bumpit modules finds every tracked go.mod, leaving out vendor, testdata and directories starting with . or _, and plans the next release of each module from its own tags and the commits under its directory.",
		RunE: func(command *cobra.Command, args []string) error {
			outputWriter := command.OutOrStdout()

			if err := flags.applyConfig(command); err != nil {
				return err
			}
			if err := validateOutputFormat(flags.output); err != nil {
				return err
			}

			releases, err := ListModules(flags.options())
			if err != nil {
				return err
			}

			if flags.output == "json" {
				return writeJSON(outputWriter, releases)
			}
			return writeModules(outputWriter, releases)
		},
	}

	flags.bind(command)
	return command
}

// writeModules writes one row per module: its directory, module path, current
// tag and next tag, or why its plan failed.
func writeModules(writer io.Writer, releases []ModuleRelease) error {
	if len(releases) == 0 {
		_, err := fmt.Fprintln(writer, "no Go module found")
		return err
	}

	var text strings.Builder
	table := tabwriter.NewWriter(&text, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "DIRECTORY\tMODULE\tCURRENT\tNEXT")
	for _, release := range releases {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", release.Directory, release.Path, valueOr(release.CurrentTag, "none"), nextColumn(release))
	}
	if err := table.Flush(); err != nil {
		return err
	}

	_, err := io.WriteString(writer, text.String())
	return err
}

func nextColumn(release ModuleRelease) string {
	switch {
	case release.Error != "":
		return "error: " + release.Error
	case release.HasRelease:
		return release.NextTag
	default:
		return "no release"
	}
}
