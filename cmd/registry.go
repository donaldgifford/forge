package cmd

import (
	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage blueprint registries",
	Long:  `Commands for initializing and managing blueprint registries.`,
}

func init() {
	rootCmd.AddCommand(registryCmd)
}
