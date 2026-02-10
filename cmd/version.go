package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	buildVersion = "dev"
	buildCommit  = "none"
)

// SetVersionInfo sets the build-time version information.
func SetVersionInfo(version, commit string) {
	buildVersion = version
	buildCommit = commit
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of forge",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("forge %s (commit: %s)\n", buildVersion, buildCommit)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
