package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/initcmd"
)

var initRegistryDir string

var initCmd = &cobra.Command{
	Use:   "init [blueprint-path]",
	Short: "Initialize a new blueprint",
	Long: `Initialize a new blueprint by creating a starter blueprint.yaml.

Without --registry, creates a standalone blueprint in the given path (or current directory).
With --registry, creates a blueprint within a registry repo and updates registry.yaml.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&initRegistryDir, "registry", "", "registry directory (enables registry mode)")
	rootCmd.AddCommand(initCmd)
}

func runInit(_ *cobra.Command, args []string) error {
	path := ""
	if len(args) > 0 {
		path = args[0]
	}

	opts := &initcmd.Opts{
		Path:        path,
		RegistryDir: initRegistryDir,
	}

	bpPath, err := initcmd.Run(opts)
	if err != nil {
		return err
	}

	fmt.Printf("Created blueprint at %s\n", bpPath)

	return nil
}
