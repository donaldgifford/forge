package cmd

import (
	"github.com/spf13/cobra"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Manage blueprint tools",
	Long:  `Commands for installing, listing, and checking project tools declared in blueprints.`,
}

func init() {
	rootCmd.AddCommand(toolsCmd)
}
