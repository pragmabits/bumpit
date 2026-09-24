package app

import (
	"fmt"

	"github.com/pragmabits/bumpit/internal/gitx"
	"github.com/spf13/cobra"
)

func newTagCmd() *cobra.Command {
	flags := tagCommandFlags{}

	command := &cobra.Command{
		Use:   "tag",
		Short: "Create the computed annotated tag locally",
		RunE: func(command *cobra.Command, args []string) error {
			outputWriter := command.OutOrStdout()

			config, err := flags.applyConfig(command)
			if err != nil {
				return err
			}

			plan, err := BuildReleasePlan(flags.options())
			if err != nil {
				return err
			}
			if !plan.HasRelease {
				return fmt.Errorf("no releasable commits found; refusing to create a tag")
			}

			client := gitx.New(flags.repository)
			tagMessage := resolveTagMessage(flags.message, config, plan.NextTag)
			if err := client.CreateAnnotatedTag(plan.NextTag, tagMessage); err != nil {
				return err
			}

			_, err = fmt.Fprintf(outputWriter, "created tag %s\n", plan.NextTag)
			return err
		},
	}

	flags.bind(command)
	return command
}

func resolveTagMessage(commandLineMessage string, config *Config, nextTag string) string {
	if commandLineMessage != "" {
		return commandLineMessage
	}
	if config != nil && config.TagMessage != "" {
		return config.TagMessage
	}
	return "Release " + nextTag
}
