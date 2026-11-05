package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func newModulesCheckCmd() *cobra.Command {
	var configPath, repository, output string
	command := &cobra.Command{
		Use:   "check",
		Short: "List what needs resolving in bumpit.yaml, why and how",
		Long:  "bumpit modules check reads the config a run reads, as a run reads it, and lists all that a run would stop at, where a run stops at the first: its keys and the text of its modules, and, once those hold, what the repository contradicts of the modules. Each comes with what is wrong, why and how to resolve it, and with the command that resolves it when that takes no choice. It also lists what stops no run: the Go modules no entry lists, the tags no module owns, the manifests with a version of their own and the charts it cannot read. A command runs without writing before it is printed, over what the commands before it would have written, so they are safe to follow in that order. It fails when something would stop a run or when it cannot read the repository, and writes nothing. It does not read tags or commits, so a tag with no version can still stop a plan, and it sees only //go:embed, so that a module that says no module ships it is listed as not verified.",
		RunE: func(command *cobra.Command, _ []string) error {
			if err := validateOutputFormat(output); err != nil {
				return err
			}
			report, err := CheckModules(CheckOptions{Repository: repository, RepositoryGiven: command.Flags().Changed("repo"), ConfigPath: configPath})
			if err != nil && len(report.Blocking) == 0 && len(report.Review) == 0 {
				return err
			}
			if writeErr := writeModulesCheckAs(command, output, report); writeErr != nil {
				return writeErr
			}
			if err != nil {
				return err
			}
			if len(report.Blocking) > 0 {
				return errModulesBlocked(len(report.Blocking))
			}
			return nil
		},
	}
	command.Args = cobra.NoArgs
	bindConfigFlag(command, &configPath)
	bindRepositoryFlag(command, &repository)
	bindOutputFlag(command, &output)
	return command
}

// writeModulesCheckAs writes the report in the output the command line gives,
// else in the one the config asks for, when it can be taken.
func writeModulesCheckAs(command *cobra.Command, output string, report ModulesCheck) error {
	if !command.Flags().Changed("output") && validateOutputFormat(report.output) == nil {
		output = report.output
	}
	if output == "json" {
		return writeJSON(command.OutOrStdout(), report)
	}
	return writeModulesCheck(command.OutOrStdout(), report)
}

// writeModulesCheck writes the report as text: what blocks the runs, what to
// review, and what bumpit could not verify.
func writeModulesCheck(writer io.Writer, report ModulesCheck) error {
	var text strings.Builder
	writeCheckItems(&text, "blocks the runs:", report.Blocking)
	if report.FileOnly {
		text.WriteString("the modules are checked against the repository only once the file holds\n")
	}
	writeCheckItems(&text, "to review:", report.Review)
	if len(report.Unverified) > 0 {
		fmt.Fprintf(&text, "not verified that no module ships %s: %s\n", strings.Join(report.Unverified, ", "), unseenShipping)
	}
	if len(report.Blocking) == 0 && len(report.Review) == 0 {
		fmt.Fprintf(&text, "nothing to resolve in %s\n", report.Config)
	}
	_, err := io.WriteString(writer, text.String())
	return err
}

func writeCheckItems(text *strings.Builder, title string, items []CheckItem) {
	if len(items) == 0 {
		return
	}
	text.WriteString(title + "\n")
	for _, item := range items {
		fmt.Fprintf(text, "  %s: %s\n", item.Subject, item.What)
		fmt.Fprintf(text, "    why: %s\n    how: %s\n", item.Why, item.How)
		if item.Command != "" {
			fmt.Fprintf(text, "    run: %s\n", item.Command)
			writeEmbeds(text, item.Embeds)
		}
		writeLeftOut(text, item.LeftOut)
	}
}
