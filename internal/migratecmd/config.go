package migratecmd

// Types and entry point for `forge migrate config` — the YAML→HCL
// config-file rewriter introduced in IMPL-0005 Phase B.
//
// The opts shape (Path / DryRun / Strict / Force) is identical to the
// templates migrator, so we reuse MigrateOpts directly per OQ-2 (the
// IMPL-0005 decision to keep templates/config as separate subcommands
// with a shared opts type).
//
// The result type is per-file rather than per-blueprint because the
// rewriter touches both blueprint.yaml inside per-blueprint dirs and
// registry.yaml at the registry root — a per-blueprint shape doesn't
// fit cleanly.

import (
	"errors"
	"fmt"
)

// MigrateConfigResult is the outcome of a `forge migrate config` run.
type MigrateConfigResult struct {
	// Files is one report per .yaml config file the walker visited.
	Files []ConfigFileReport
}

// ConfigFileReport describes the per-file outcome of a YAML→HCL
// rewrite. Authored to be friendly for both the CLI summary table and
// strict-mode error tallying.
type ConfigFileReport struct {
	// Path is the source .yaml file the walker visited.
	Path string

	// Output is the destination .hcl file path written by the rewriter.
	// Empty when no output was produced (Skipped or errored).
	Output string

	// Migrated is true when the rewriter actually wrote a new .hcl file
	// and removed the source .yaml. False in dry-run, when the file was
	// already HCL, or when an error blocked the write.
	Migrated bool

	// Skipped is true when the walker chose not to touch the file —
	// either an .hcl sibling already exists or the file is otherwise
	// outside the migration scope.
	Skipped bool

	// SkipReason explains why Skipped is true (rendered in the summary).
	SkipReason string

	// Errors carries any rewrite/IO errors. The walker continues on
	// per-file errors so a single bad blueprint doesn't abort the run.
	Errors []error
}

// errMigrateConfigNotImplemented is returned by RunMigrateConfig until
// Phase B.3 wires up the file walker. Sentinel keeps the package
// compiling while B.2 (rewriter) and B.3 (walker) land in their own
// commits.
var errMigrateConfigNotImplemented = errors.New("migratecmd: config rewriter not yet implemented (IMPL-0005 Phase B.3)")

// RunMigrateConfig executes the YAML→HCL config rewrite described by
// opts. Mirrors RunMigrate's dirty-worktree guard and dry-run/strict
// semantics. Implementation lands in IMPL-0005 Phase B.3.
func RunMigrateConfig(opts *MigrateOpts) (*MigrateConfigResult, error) {
	if opts == nil {
		return nil, fmt.Errorf("migrate config: opts is nil")
	}

	return nil, errMigrateConfigNotImplemented
}
