package app

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// buildVersion is the version set at link time, with -ldflags
// "-X github.com/pragmabits/bumpit/internal/app.buildVersion=...", as make build
// and make install do. It stays "dev" otherwise.
var buildVersion = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		RunE: func(command *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(command.OutOrStdout(), version())
			return err
		},
	}
}

// version is the version bumpit reports, from bumpit version and --version.
func version() string {
	info, _ := debug.ReadBuildInfo()
	return resolveVersion(buildVersion, info)
}

// resolveVersion prefers the version set at link time. Without one it takes
// the main module version the go command records in every binary: the version
// go install ...@v0.3.0 fetched, or for a local build the one derived from
// git, such as v0.3.0 at a clean tagged commit. A development build without
// either is "dev".
func resolveVersion(linked string, info *debug.BuildInfo) string {
	switch {
	case linked != "" && linked != "dev":
		return linked
	case info == nil || info.Main.Version == "" || info.Main.Version == "(devel)":
		return "dev"
	default:
		return info.Main.Version
	}
}
