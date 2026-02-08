package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/check"
)

var checkOutputFormat string

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check project for drift against blueprint",
	Long: `Compare the local project state against the source blueprint to detect
changes to defaults, managed files, and tools.`,
	RunE: runCheck,
}

func init() {
	checkCmd.Flags().StringVarP(&checkOutputFormat, "output", "o", "text", "output format (text, json)")
	rootCmd.AddCommand(checkCmd)
}

func runCheck(_ *cobra.Command, _ []string) error {
	opts := &check.Opts{
		ProjectDir:   ".",
		OutputFormat: checkOutputFormat,
		Writer:       os.Stdout,
	}

	_, err := check.Run(opts)

	return err
}
