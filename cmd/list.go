package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/list"
)

var (
	listTag          string
	listOutputFormat string
	listRegistryDir  string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available blueprints",
	Long: `List blueprints from a registry. By default, lists all blueprints
in table format. Use --tag to filter and --output to change the format.`,
	Aliases: []string{"ls"},
	RunE:    runList,
}

func init() {
	listCmd.Flags().StringVar(&listTag, "tag", "", "filter blueprints by tag")
	listCmd.Flags().StringVarP(&listOutputFormat, "output", "o", "table", "output format (table, json)")
	listCmd.Flags().StringVar(&listRegistryDir, "registry", "", "path to registry directory")
	rootCmd.AddCommand(listCmd)
}

func runList(_ *cobra.Command, _ []string) error {
	opts := &list.Opts{
		RegistryDir:  listRegistryDir,
		TagFilter:    listTag,
		OutputFormat: listOutputFormat,
		Writer:       os.Stdout,
	}

	return list.Run(opts)
}
