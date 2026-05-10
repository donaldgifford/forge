package migratecmd

// MigrateOpts configures a `forge migrate templates` run.
type MigrateOpts struct {
	// Path is the blueprint or registry root to migrate. Defaults to ".".
	Path string

	// DryRun collects would-be changes but writes nothing.
	DryRun bool

	// Strict makes the run exit non-zero if any blueprint surfaces an
	// UntranslatedHit.
	Strict bool

	// Force skips the dirty-worktree guard.
	Force bool
}

// MigrateResult is the outcome of a `forge migrate templates` run.
type MigrateResult struct {
	// Blueprints is one report per blueprint discovered under Path.
	Blueprints []BlueprintReport
}

// BlueprintReport summarizes the migration outcome for one blueprint.
type BlueprintReport struct {
	// Path is the directory containing blueprint.yaml.
	Path string

	// Migrated is true when the tool actually wrote changes (false in
	// dry-run or when the blueprint was already v2).
	Migrated bool

	// AlreadyV2 is true when the blueprint's apiVersion was v2 before
	// the run; the tool skips it.
	AlreadyV2 bool

	// FilesRewritten lists the file paths the tool rewrote.
	FilesRewritten []string

	// UntranslatedHits records nodes the AST walker refused to
	// translate (range, with, etc.) so the author can fix them by
	// hand.
	UntranslatedHits []UntranslatedHit
}

// UntranslatedHit is one occurrence of a v1 construct the migrator does
// not translate. Surfaced in the summary table and used by --strict.
type UntranslatedHit struct {
	// File is the source file path containing the hit.
	File string

	// Line is the 1-based source line of the hit.
	Line int

	// Snippet is the offending v1 source text (typically one line).
	Snippet string

	// Reason explains why the migrator refused — e.g. "range block",
	// "with block", "template invocation".
	Reason string
}

// RunMigrate executes the migration described by opts. The implementation
// lands in B.4 (file walker) once the AST rewriter (B.3) is in place. For
// now the function is a stub that returns the zero value so callers can
// be wired up.
func RunMigrate(opts *MigrateOpts) (*MigrateResult, error) {
	_ = opts

	return &MigrateResult{}, nil
}
