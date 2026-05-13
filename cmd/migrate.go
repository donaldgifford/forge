package cmd

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/migratecmd"
)

// emptyCol is the placeholder rendered into empty cells of the
// migrate-summary tabwriter tables.
const emptyCol = "—"

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate blueprints between forge contract versions",
	Long: `Tools for migrating blueprints between major forge contract versions.

Subcommands:
  templates  Convert v1 (Go text/template) blueprints to v2 (HCL2).
  config     Convert v2 YAML configs (blueprint.yaml, registry.yaml) to HCL.`,
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
	Long: `Walks every blueprint under --path and rewrites .tmpl files
and blueprint.yaml expression fields to the v2 (HCL2) shape. This is
step 1 of the v0.2.x → v0.4.x upgrade path: run ` + "`forge migrate config`" + `
afterwards to convert the YAML config files to HCL.

By default refuses to write inside a dirty git worktree; --force
overrides. --dry-run prints what would change without writing.`,
	RunE: runMigrateTemplates,
}

var (
	migrateConfigPath   string
	migrateConfigDryRun bool
	migrateConfigStrict bool
	migrateConfigForce  bool
)

var migrateConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Rewrite v2 YAML configs (blueprint.yaml, registry.yaml) to HCL",
	Long: `Walks every blueprint.yaml and registry.yaml under --path and
rewrites them as the equivalent .hcl files. Drops the apiVersion field
on emit (the file extension is the version signal). Templated fields
(variable.default, condition.when, rename entries) round-trip with
their HCL syntax intact.

Refuses to touch a directory where both .yaml and .hcl already exist
side by side — clean up the partial migration first. By default also
refuses to write inside a dirty git worktree; --force overrides.
--dry-run prints what would change without writing.

Comments in the source YAML are not preserved (per IMPL-0005 OQ-3).
Re-add any author comments by hand after migration.`,
	RunE: runMigrateConfig,
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

	migrateConfigCmd.Flags().StringVar(
		&migrateConfigPath, "path", ".",
		"path to a blueprint or registry root",
	)
	migrateConfigCmd.Flags().BoolVar(
		&migrateConfigDryRun, "dry-run", false,
		"print would-be changes without writing",
	)
	migrateConfigCmd.Flags().BoolVar(
		&migrateConfigStrict, "strict", false,
		"exit non-zero if any file fails to migrate",
	)
	migrateConfigCmd.Flags().BoolVar(
		&migrateConfigForce, "force", false,
		"skip the dirty-worktree guard",
	)

	migrateCmd.AddCommand(migrateTemplatesCmd)
	migrateCmd.AddCommand(migrateConfigCmd)
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

		filesCol := emptyCol
		if len(bp.FilesRewritten) > 0 {
			filesCol = fmt.Sprintf("%d", len(bp.FilesRewritten))
		}

		hitsCol := emptyCol
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

func runMigrateConfig(cmd *cobra.Command, _ []string) error {
	opts := &migratecmd.MigrateOpts{
		Path:   migrateConfigPath,
		DryRun: migrateConfigDryRun,
		Strict: migrateConfigStrict,
		Force:  migrateConfigForce,
	}

	result, err := migratecmd.RunMigrateConfig(opts)
	if err != nil {
		return err
	}

	if err := printMigrateConfigSummary(cmd.OutOrStdout(), result, migrateConfigDryRun); err != nil {
		return fmt.Errorf("printing summary: %w", err)
	}

	if migrateConfigStrict && hasConfigErrors(result) {
		return fmt.Errorf("strict mode: one or more config files failed to migrate")
	}

	return nil
}

func printMigrateConfigSummary(out io.Writer, r *migratecmd.MigrateConfigResult, dryRun bool) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(w, "FILE\tSTATUS\tOUTPUT"); err != nil {
		return err
	}

	for i := range r.Files {
		f := &r.Files[i]
		status := configFileStatus(f, dryRun)

		output := emptyCol
		if f.Output != "" {
			output = f.Output
		}

		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", f.Path, status, output); err != nil {
			return err
		}
	}

	return w.Flush()
}

func configFileStatus(f *migratecmd.ConfigFileReport, dryRun bool) string {
	switch {
	case f.Skipped:
		if f.SkipReason != "" {
			return "skipped (" + f.SkipReason + ")"
		}

		return "skipped"
	case len(f.Errors) > 0:
		return "error"
	case dryRun:
		return "would migrate"
	case f.Migrated:
		return "migrated"
	default:
		return "unchanged"
	}
}

func hasConfigErrors(r *migratecmd.MigrateConfigResult) bool {
	for _, f := range r.Files {
		if len(f.Errors) > 0 {
			return true
		}
	}

	return false
}
