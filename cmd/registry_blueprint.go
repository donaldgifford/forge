package cmd

import (
	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/registrycmd"
	"github.com/donaldgifford/forge/internal/ui"
)

var (
	regBlueprintCategory    string
	regBlueprintName        string
	regBlueprintDescription string
	regBlueprintTags        []string
	regBlueprintRegistryDir string
)

var registryBlueprintCmd = &cobra.Command{
	Use:   "blueprint [category/name]",
	Short: "Scaffold a new blueprint in a registry",
	Long: `Scaffold a new blueprint directory with a rich starter blueprint.yaml,
template files, and automatic registry.yaml update.

Provide the blueprint path as a positional argument (category/name) or
use --category and --name flags.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRegistryBlueprint,
}

func init() {
	registryBlueprintCmd.Flags().StringVar(&regBlueprintCategory, "category", "", "blueprint category directory")
	registryBlueprintCmd.Flags().StringVar(&regBlueprintName, "name", "", "blueprint name within category")
	registryBlueprintCmd.Flags().StringVar(&regBlueprintDescription, "description", "", "blueprint description")
	registryBlueprintCmd.Flags().StringSliceVar(&regBlueprintTags, "tags", nil, "tags for registry index (comma-separated)")
	registryBlueprintCmd.Flags().StringVar(&regBlueprintRegistryDir, "registry-dir", ".", "registry root directory")
	registryCmd.AddCommand(registryBlueprintCmd)
}

func runRegistryBlueprint(_ *cobra.Command, args []string) error {
	w := ui.NewWriter(noColor)

	category := regBlueprintCategory
	name := regBlueprintName

	// Positional arg overrides flags.
	if len(args) > 0 {
		var err error

		category, name, err = registrycmd.ParseBlueprintPath(args[0])
		if err != nil {
			return err
		}
	}

	opts := &registrycmd.BlueprintOpts{
		RegistryDir: regBlueprintRegistryDir,
		Category:    category,
		Name:        name,
		Description: regBlueprintDescription,
		Tags:        regBlueprintTags,
	}

	result, err := registrycmd.RunBlueprint(opts)
	if err != nil {
		return err
	}

	w.Successf("Blueprint scaffolded at %s", result.BlueprintDir)
	w.Infof("Edit %s to customize your blueprint", result.BlueprintYAML)
	w.Infof("Run: forge registry update --registry-dir %s", regBlueprintRegistryDir)

	return nil
}
