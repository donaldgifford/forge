package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/registrycmd"
	"github.com/donaldgifford/forge/internal/ui"
)

var (
	regUpdateRegistryDir string
	regUpdateCheck       bool
)

var registryUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update blueprint metadata in registry.yaml",
	Long: `Walk all blueprints in a registry, compare blueprint.yaml versions and
git commit hashes against registry.yaml entries, and update stale metadata.

Use --check for CI mode: reports stale entries and exits non-zero without
modifying any files.`,
	Args: cobra.NoArgs,
	RunE: runRegistryUpdate,
}

func init() {
	registryUpdateCmd.Flags().StringVar(
		&regUpdateRegistryDir, "registry-dir", ".", "registry root directory",
	)
	registryUpdateCmd.Flags().BoolVar(
		&regUpdateCheck, "check", false, "check-only mode: report stale entries without updating",
	)
	registryCmd.AddCommand(registryUpdateCmd)
}

func runRegistryUpdate(_ *cobra.Command, _ []string) error {
	w := ui.NewWriter(noColor)

	result, err := registrycmd.RunUpdate(&registrycmd.UpdateOpts{
		RegistryDir: regUpdateRegistryDir,
		Check:       regUpdateCheck,
	})
	if err != nil {
		return err
	}

	for _, r := range result.Reports {
		switch r.Status {
		case registrycmd.StatusUpToDate:
			w.Infof("  %-30s %s", r.Path, r.Status)
		case registrycmd.StatusMissing:
			w.Warningf("  %-30s %s", r.Path, r.Status)
		default:
			w.Warningf("  %-30s %s", r.Path, r.Status)
		}
	}

	if regUpdateCheck {
		if result.Stale > 0 {
			return fmt.Errorf(
				"registry metadata is stale (%d blueprint(s) need update)",
				result.Stale,
			)
		}

		w.Success("All blueprints up to date")

		return nil
	}

	if result.Updated > 0 {
		w.Successf("Updated registry.yaml (%d blueprint(s) updated)", result.Updated)
	} else {
		w.Info("All blueprints up to date")
	}

	return nil
}
