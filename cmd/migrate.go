package cmd

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/migratecmd"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate blueprints between forge contract versions",
	Long: `Tools for migrating blueprints between major forge contract versions.

Subcommands:
  templates  Convert v1 (Go text/template) blueprints to v2 (HCL2).`,
}

var (
	migrateTemplatesPath   string
	migrateTemplatesDryRun bool
	migrateTemplatesStrict bool
	migrateTemplatesForce  bool
)

var migrateTemplatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "Rewrite v1 blueprints to v2 (HCL2)",
	Long: `Walks every blueprint under --path and rewrites .tmpl files,
blueprint.yaml expression fields, and the registry.yaml apiVersion to
the v2 (HCL2) shape. The migration runs once per registry and is the
required upgrade path before forge stops accepting v1 blueprints.

By default refuses to write inside a dirty git worktree; --force
overrides. --dry-run prints what would change without writing.`,
	RunE: runMigrateTemplates,
}

func init() {
	migrateTemplatesCmd.Flags().StringVar(
		&migrateTemplatesPath, "path", ".",
		"path to a blueprint or registry root",
	)
	migrateTemplatesCmd.Flags().BoolVar(
		&migrateTemplatesDryRun, "dry-run", false,
		"print would-be changes without writing",
	)
	migrateTemplatesCmd.Flags().BoolVar(
		&migrateTemplatesStrict, "strict", false,
		"exit non-zero if any blueprint surfaces an UntranslatedHit",
	)
	migrateTemplatesCmd.Flags().BoolVar(
		&migrateTemplatesForce, "force", false,
		"skip the dirty-worktree guard",
	)

	migrateCmd.AddCommand(migrateTemplatesCmd)
	rootCmd.AddCommand(migrateCmd)
}

func runMigrateTemplates(cmd *cobra.Command, _ []string) error {
	opts := &migratecmd.MigrateOpts{
		Path:   migrateTemplatesPath,
		DryRun: migrateTemplatesDryRun,
		Strict: migrateTemplatesStrict,
		Force:  migrateTemplatesForce,
	}

	result, err := migratecmd.RunMigrate(opts)
	if err != nil {
		return err
	}

	if err := printMigrateSummary(cmd.OutOrStdout(), result, migrateTemplatesDryRun); err != nil {
		return fmt.Errorf("printing summary: %w", err)
	}

	if migrateTemplatesStrict && hasUntranslated(result) {
		return fmt.Errorf("strict mode: untranslated constructs surfaced")
	}

	return nil
}

func printMigrateSummary(out io.Writer, r *migratecmd.MigrateResult, dryRun bool) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(w, "BLUEPRINT\tSTATUS\tFILES\tUNTRANSLATED"); err != nil {
		return err
	}

	for _, bp := range r.Blueprints {
		status := blueprintStatus(&bp, dryRun)

		filesCol := "—"
		if len(bp.FilesRewritten) > 0 {
			filesCol = fmt.Sprintf("%d", len(bp.FilesRewritten))
		}

		hitsCol := "—"
		if len(bp.UntranslatedHits) > 0 {
			hitsCol = fmt.Sprintf("%d", len(bp.UntranslatedHits))
		}

		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", bp.Path, status, filesCol, hitsCol); err != nil {
			return err
		}
	}

	return w.Flush()
}

func blueprintStatus(bp *migratecmd.BlueprintReport, dryRun bool) string {
	switch {
	case bp.AlreadyV2:
		return "skipped (v2)"
	case dryRun:
		return "would migrate"
	case len(bp.UntranslatedHits) > 0 && bp.Migrated:
		return "partial"
	case bp.Migrated:
		return "migrated"
	default:
		return "unchanged"
	}
}

func hasUntranslated(r *migratecmd.MigrateResult) bool {
	for _, bp := range r.Blueprints {
		if len(bp.UntranslatedHits) > 0 {
			return true
		}
	}

	return false
}
