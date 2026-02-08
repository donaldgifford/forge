// Package cmd defines the CLI commands for forge.
package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var (
	verbose bool
	noColor bool
	cfgFile string
)

// rootCmd is the base command for the forge CLI.
var rootCmd = &cobra.Command{
	Use:   "forge",
	Short: "Scaffold projects from blueprints",
	Long: `Forge scaffolds new projects from blueprints — project templates stored
in a Git-based registry. It supports layered defaults inheritance, managed
file sync, and remote tool resolution.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		initLogger()
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/forge/config.yaml)")
}

func initLogger() {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}
