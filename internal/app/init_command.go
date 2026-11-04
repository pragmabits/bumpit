package app

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var repository, output string
	command := &cobra.Command{
		Use:   "init",
		Short: "Create bumpit.yaml and propose the modules to declare",
		Long:  "bumpit init creates bumpit.yaml at the top of the repository, where the runs read the modules from, when it has neither bumpit.yaml nor .bumpit.yaml, and declares nothing in it. It proposes the modules to declare, each with the bumpit modules add that declares it: the directories a line of version tags that no module owns may be the releases of, and the directories whose Helm chart or public Node.js package gives a version of its own. A module is proposed as shipped by the Go modules whose //go:embed patterns reach it, and by none when no pattern does, which is all bumpit sees. Paths are relative to --repo.",
		RunE: func(command *cobra.Command, _ []string) error {
			if err := validateOutputFormat(output); err != nil {
				return err
			}
			report, err := Init(InitOptions{Repository: repository})
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
	bindRepositoryFlag(command, &repository)
	bindOutputFlag(command, &output)
	return command
}

// writeInit writes the report as text: the config, the tag lines and the
// manifests, each candidate with its command and the patterns that ship it.
func writeInit(writer io.Writer, report InitReport) error {
	var text strings.Builder
	if report.Created {
		fmt.Fprintf(&text, "created %s\n", report.Config)
	} else {
		fmt.Fprintf(&text, "%s exists already: left as it is\n", report.Config)
	}

	var candidates []Candidate
	if len(report.TagLines) > 0 {
		text.WriteString("tags no module owns:\n")
	}
	for _, line := range report.TagLines {
		writeTagLine(&text, line)
		candidates = append(candidates, line.Candidates...)
	}
	if len(report.Manifests) > 0 {
		text.WriteString("manifests with a version of their own:\n")
	}
	for _, manifest := range report.Manifests {
		fmt.Fprintf(&text, "  %s: version %s\n", manifest.Manifest, manifest.Version)
		writeCandidates(&text, []Candidate{manifest.Candidate})
		candidates = append(candidates, manifest.Candidate)
	}

	for _, manifest := range report.Unreadable {
		fmt.Fprintf(&text, "could not read %s: %s\n", manifest.Manifest, manifest.Error)
	}
	writeInitNotes(&text, report, candidates)
	if report.Created {
		text.WriteString("commit the file so that every clone of the repository reads the change\n")
	}
	_, err := io.WriteString(writer, text.String())
	return err
}

// writeInitNotes says that init found nothing, or what -B rests on when a
// candidate takes it.
func writeInitNotes(text *strings.Builder, report InitReport, candidates []Candidate) {
	switch {
	case len(report.TagLines) == 0 && len(report.Manifests) == 0 && len(report.Unreadable) == 0:
		text.WriteString("found no tags that no module owns, and no manifest with a version of its own\n")
	case slices.ContainsFunc(candidates, func(candidate Candidate) bool { return candidate.Refused == "" && len(candidate.ShippedBy) == 0 }):
		text.WriteString("-B says no module ships the directory, but bumpit sees only //go:embed: check that no Dockerfile, CI job or other build copies it into the release of another module, and give that module with -b if one does\n")
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
	case len(line.Candidates) == 0:
		text.WriteString("    no tracked directory takes its name: declare its module with bumpit modules add and --tag-prefix\n")
	}
	writeCandidates(text, line.Candidates)
}

func writeCandidates(text *strings.Builder, candidates []Candidate) {
	for _, candidate := range candidates {
		if candidate.Refused != "" {
			fmt.Fprintf(text, "    bumpit modules add would refuse %s: %s\n", candidate.Directory, candidate.Refused)
			continue
		}
		fmt.Fprintf(text, "    %s\n", candidate.Command)
		for _, embed := range candidate.Embeds {
			fmt.Fprintf(text, "      %s embeds %s\n", embed.Place, embed.Pattern)
		}
	}
}

func countTags(count int) string {
	if count == 1 {
		return "1 tag"
	}
	return fmt.Sprintf("%d tags", count)
}
