package app

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	flags := configEditFlags{}
	var output string
	command := &cobra.Command{
		Use:   "init",
		Short: "Create bumpit.yaml and propose the modules to declare",
		Long:  "bumpit init creates bumpit.yaml at the top of the repository, where the runs read the modules from, when it has neither bumpit.yaml nor .bumpit.yaml, and lists in it the Go modules no entry lists, which bumpit can tell for certain, but those in a directory no entry can name or git takes in no tag name, which says to rename it. It stops on a file that holds a problem. What it cannot tell it proposes, and does not declare, each with the bumpit modules add that declares it: the directories a line of version tags that no module owns may be the releases of, as one module per line, and the directories whose Helm chart or public Node.js package gives a version of its own. A module is proposed as shipped by the Go modules whose //go:embed patterns reach it, and by none when no pattern does, which is all bumpit sees. It runs each command without writing before it prints it, over what the Go modules and the commands before it would write, and says why when it would be refused, so they are safe to follow in that order. It writes the file last, so that it writes nothing when it cannot make up the proposals. The paths of the proposals are relative to --repo, and the Go modules it lists are named as the file writes them, relative to the top of the repository.",
		RunE: func(command *cobra.Command, _ []string) error {
			if err := validateOutputFormat(output); err != nil {
				return err
			}
			options := InitOptions{rehearsal: rehearsalOf(command)}
			if command.Flags().Changed("repo") {
				options.RepositoryGiven = flags.repository
			}
			config, configPath, err := flags.applyConfigRepository(command)
			if err != nil {
				return pointAtCheck(err, commandContext{repositoryGiven: options.RepositoryGiven})
			}
			options.Repository, options.Config, options.ConfigPath = flags.repository, config, configPath
			report, err := Init(options)
			if err != nil {
				return err
			}
			if output == "json" {
				return writeJSON(command.OutOrStdout(), report)
			}
			return writeInit(command.OutOrStdout(), report)
		},
	}
	command.Args = cobra.NoArgs
	bindRepositoryFlag(command, &flags.repository)
	bindOutputFlag(command, &output)
	return command
}

// writeInit writes the report as text: the config, the tag lines and the
// manifests, each candidate with its command and the patterns that ship it.
func writeInit(writer io.Writer, report InitReport) error {
	var text strings.Builder
	writeInitConfig(&text, report)

	var candidates []Candidate
	if len(report.TagLines) > 0 {
		text.WriteString("tags no module owns:\n")
	}
	for _, line := range report.TagLines {
		writeTagLine(&text, line)
		if line.Candidate != nil {
			candidates = append(candidates, *line.Candidate)
		}
	}
	if len(report.Manifests) > 0 {
		text.WriteString("manifests with a version of their own:\n")
	}
	for _, manifest := range report.Manifests {
		fmt.Fprintf(&text, "  %s: version %s\n", manifest.Manifest, manifest.Version)
		writeCandidate(&text, manifest.Candidate)
		candidates = append(candidates, manifest.Candidate)
	}

	for _, manifest := range report.Unreadable {
		fmt.Fprintf(&text, "could not read %s: %s\n", manifest.Manifest, manifest.Error)
	}
	writeInitNotes(&text, report, candidates)
	if report.Created || len(report.Declared) > 0 {
		text.WriteString("commit the file so that every clone of the repository reads the change\n")
	}
	_, err := io.WriteString(writer, text.String())
	return err
}

// writeInitConfig says what init did to the config file.
func writeInitConfig(text *strings.Builder, report InitReport) {
	switch {
	case report.Created:
		fmt.Fprintf(text, "created %s\n", report.Config)
	case len(report.Declared) == 0 && len(report.Unlisted) == 0:
		fmt.Fprintf(text, "%s exists already: left as it is\n", report.Config)
	}
	if len(report.Declared) > 0 {
		fmt.Fprintf(text, "listed the Go modules %s in %s\n", strings.Join(report.Declared, ", "), report.Config)
	}
	if len(report.Unlisted) > 0 {
		fmt.Fprintf(text, "could not list the Go modules %s in %s, so edit it by hand: %s\n", strings.Join(report.Unlisted, ", "), report.Config, report.UnlistedReason)
	}
	for _, module := range report.Unnameable {
		fmt.Fprintf(text, "no entry can name %s, so rename the directory to list its Go module: %s\n", module.Directory, module.Reason)
	}
	for _, module := range report.Untaggable {
		fmt.Fprintf(text, "no tag can carry %s, so rename the directory to version and list its Go module: %s\n", module.Directory, module.Reason)
	}
}

// writeInitNotes says that init found nothing, or what a candidate with no -b
// rests on.
func writeInitNotes(text *strings.Builder, report InitReport, candidates []Candidate) {
	switch {
	case len(report.TagLines) == 0 && len(report.Manifests) == 0 && len(report.Unreadable) == 0:
		text.WriteString("found no tags that no module owns, and no manifest with a version of its own\n")
	case slices.ContainsFunc(candidates, func(candidate Candidate) bool { return candidate.Refused == "" && len(candidate.ShippedBy) == 0 }):
		text.WriteString("a command with no -b says no module ships the directory, but " + unseenShipping + ", and give that module with -b if one does\n")
	}
}

// writeTagLine writes a tag line, named (no prefix) when its tags have
// none, and its candidates.
func writeTagLine(text *strings.Builder, line TagLine) {
	name := line.Prefix
	if name == "" {
		name = "(no prefix)"
	}
	fmt.Fprintf(text, "  %s: %s, the newest %s\n", name, countTags(line.Count), line.Newest)
	switch {
	case line.Prefix == "":
		text.WriteString("    a declared module's tags start with a prefix, so these can be no declared module's\n")
	case line.Command != "":
		fmt.Fprintf(text, "    a module with no tags of its own owns the directory they name: give them to it, if these are its releases\n    %s\n", line.Command)
	case line.Refused != "":
		fmt.Fprintf(text, "    a module with no tags of its own owns the directory they name, but bumpit modules set would refuse to give them to it: %s\n", line.Refused)
	case line.Candidate == nil:
		text.WriteString("    no directory free of a module takes its name: declare its module with bumpit modules add and --tag-prefix\n")
	}
	if line.Candidate != nil {
		writeCandidate(text, *line.Candidate)
	}
}

func writeCandidate(text *strings.Builder, candidate Candidate) {
	if candidate.Refused != "" {
		fmt.Fprintf(text, "    bumpit modules add would refuse %s: %s\n", strings.Join(candidate.Directories, ", "), candidate.Refused)
	} else {
		fmt.Fprintf(text, "    %s\n", candidate.Command)
		writeEmbeds(text, candidate.Embeds)
	}
	writeLeftOut(text, candidate.LeftOut)
	if candidate.Refused == "" && len(candidate.Directories) > 1 {
		text.WriteString("    leave out a directory that is no part of the module, with the -b only its patterns ask for: bumpit cannot tell a module from directories that share a name\n")
	}
}

// writeEmbeds writes the //go:embed patterns behind the carriers of a command.
func writeEmbeds(text *strings.Builder, embeds []EmbedPlace) {
	for _, embed := range embeds {
		fmt.Fprintf(text, "      %s embeds %s\n", embed.Place, embed.Pattern)
	}
}

// writeLeftOut writes the directories a proposal could not take, and why.
func writeLeftOut(text *strings.Builder, directories []LeftOutDirectory) {
	for _, leftOut := range directories {
		fmt.Fprintf(text, "    left out %s: %s\n", leftOut.Directory, leftOut.Reason)
	}
}

func countTags(count int) string {
	if count == 1 {
		return "1 tag"
	}
	return fmt.Sprintf("%d tags", count)
}
