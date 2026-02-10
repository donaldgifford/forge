package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/search"
)

var (
	searchOutputFormat string
	searchRegistryDir  string
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for blueprints",
	Long: `Search across registries by name, description, or tags.
Case-insensitive substring matching is used.`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().StringVarP(&searchOutputFormat, "output", "o", "table", "output format (table, json)")
	searchCmd.Flags().StringVar(&searchRegistryDir, "registry", "", "path to registry directory")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(_ *cobra.Command, args []string) error {
	opts := &search.Opts{
		Query:        args[0],
		RegistryDir:  searchRegistryDir,
		OutputFormat: searchOutputFormat,
		Writer:       os.Stdout,
	}

	return search.Run(opts)
}
