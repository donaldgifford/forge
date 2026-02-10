package cmd

import (
	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/registrycmd"
	"github.com/donaldgifford/forge/internal/ui"
)

var (
	regInitName        string
	regInitDescription string
	regInitGit         bool
	regInitCategories  []string
)

var registryInitCmd = &cobra.Command{
	Use:   "init <path>",
	Short: "Initialize a new blueprint registry",
	Long: `Initialize a new blueprint registry by creating the directory structure,
registry.yaml, default files, and optionally initializing a git repository.`,
	Args: cobra.ExactArgs(1),
	RunE: runRegistryInit,
}

func init() {
	registryInitCmd.Flags().StringVar(&regInitName, "name", "", "registry display name (defaults to directory name)")
	registryInitCmd.Flags().StringVar(&regInitDescription, "description", "", "registry description")
	registryInitCmd.Flags().BoolVar(&regInitGit, "git-init", true, "initialize a git repository")
	registryInitCmd.Flags().StringArrayVar(&regInitCategories, "category", nil, "category directories to pre-create (repeatable)")
	registryCmd.AddCommand(registryInitCmd)
}

func runRegistryInit(_ *cobra.Command, args []string) error {
	w := ui.NewWriter(noColor)

	opts := &registrycmd.Opts{
		Path:        args[0],
		Name:        regInitName,
		Description: regInitDescription,
		GitInit:     regInitGit,
		Categories:  regInitCategories,
	}

	result, err := registrycmd.Run(opts)
	if err != nil {
		return err
	}

	w.Successf("Registry initialized at %s", result.Dir)

	if result.GitInitialized {
		w.Info("Initialized git repository")
	}

	w.Infof("Add blueprints with: forge init <category>/<name> --registry %s", result.Dir)

	return nil
}
