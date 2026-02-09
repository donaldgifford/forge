package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/info"
)

var infoOutputFormat string

var infoCmd = &cobra.Command{
	Use:   "info <blueprint.yaml>",
	Short: "Show detailed blueprint information",
	Long: `Display detailed information about a blueprint including its description,
variables, tools, managed files, and inherited defaults.

Provide a path to a blueprint.yaml file to inspect.`,
	Args: cobra.ExactArgs(1),
	RunE: runInfo,
}

func init() {
	infoCmd.Flags().StringVarP(&infoOutputFormat, "output", "o", "text", "output format (text, json)")
	rootCmd.AddCommand(infoCmd)
}

func runInfo(_ *cobra.Command, args []string) error {
	bpPath := args[0]

	bp, err := config.LoadBlueprint(bpPath)
	if err != nil {
		return fmt.Errorf("loading blueprint: %w", err)
	}

	opts := &info.Opts{
		Blueprint:    bp,
		Writer:       os.Stdout,
		OutputFormat: infoOutputFormat,
	}

	return info.Run(opts)
}
