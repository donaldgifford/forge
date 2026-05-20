package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/getter"
	"github.com/donaldgifford/forge/internal/lockfile"
	forgesync "github.com/donaldgifford/forge/internal/sync"
	"github.com/donaldgifford/forge/internal/ui"
)

var (
	syncDryRun      bool
	syncForce       bool
	syncFileFilter  string
	syncRegistryDir string
	syncRef         string
	syncVarFiles    []string
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync project files with the source blueprint",
	Long: `Synchronize local project files with the latest version from the source
blueprint and registry. Defaults use overwrite strategy; managed files use
overwrite or three-way merge depending on their configuration.

Use --registry-dir to override the registry source from the lockfile.
Use --ref to sync against a specific registry version.`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "print what would change without writing")
	syncCmd.Flags().BoolVarP(&syncForce, "force", "f", false, "skip confirmation prompts; required when --var-file rewrites the lockfile")
	syncCmd.Flags().StringVar(&syncFileFilter, "file", "", "sync only a specific file path")
	syncCmd.Flags().StringVar(&syncRegistryDir, "registry-dir", "", "override registry source (local path or go-getter URL)")
	syncCmd.Flags().StringVar(&syncRef, "ref", "", "sync against a specific registry version/ref")
	syncCmd.Flags().StringArrayVar(&syncVarFiles, "var-file", nil,
		"load variable values from an HCL document (.hcl extension required). "+
			"Repeatable; later files override earlier ones on key collision. "+
			"Requires --force because applying values to a sync rewrites the lockfile.")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, _ []string) error {
	if err := requireForceForVarFile(syncVarFiles, syncForce); err != nil {
		return err
	}

	logger := slog.Default()
	w := ui.NewWriter(noColor)
	projectDir := "."

	lock, err := lockfile.LoadLockfile(projectDir)
	if err != nil {
		return fmt.Errorf("reading lockfile: %w (is this a forge project?)", err)
	}

	ctx := cmd.Context()

	// Determine registry source and ref.
	regSource, ref := resolveSyncSource(lock)

	// Output which ref is being used.
	if ref != "" {
		w.Infof("syncing against ref %q", ref)
	} else {
		w.Info("syncing against latest")
	}

	// Resolve registry directory (local path or remote fetch).
	registryDir, regCleanup, err := resolveSyncRegistry(ctx, logger, regSource)
	if err != nil {
		return fmt.Errorf("resolving registry: %w", err)
	}

	if regCleanup != nil {
		defer regCleanup()
	}

	// Fetch base registry content for three-way merge support.
	var baseDir string

	if lock.Blueprint.Commit != "" {
		baseDir, err = fetchRegistry(ctx, logger, regSource, lock.Blueprint.Commit)
		if err != nil {
			logger.Warn("could not fetch base registry for merge, falling back to overwrite", "error", err)
		} else {
			defer cleanupDir(logger, baseDir)
		}
	}

	opts := &forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
		BaseDir:     baseDir,
		DryRun:      syncDryRun,
		Force:       syncForce,
		FileFilter:  syncFileFilter,
		VarsFiles:   syncVarFiles,
	}

	result, err := forgesync.Run(opts)
	if err != nil {
		return err
	}

	if len(result.UnknownVarsFileKeys) > 0 {
		w.Warningf(
			"--var-file declared values for variable(s) not in this blueprint: %s",
			strings.Join(result.UnknownVarsFileKeys, ", "),
		)
	}

	printSyncSummary(w, result)

	// Report conflicts to stderr and return error if any exist.
	if len(result.ConflictFiles) > 0 {
		return forgesync.ReportConflicts(os.Stderr, result.ConflictFiles)
	}

	return nil
}

// resolveSyncSource determines the registry source URL and ref for syncing.
// --registry-dir overrides the lockfile's registry_url.
// --ref overrides the lockfile's blueprint ref.
func resolveSyncSource(lock *lockfile.Lockfile) (source, ref string) {
	source = lock.Blueprint.RegistryURL
	if syncRegistryDir != "" {
		source = syncRegistryDir
	}

	ref = lock.Blueprint.Ref
	if syncRef != "" {
		ref = syncRef
	}

	return source, ref
}

// resolveSyncRegistry resolves a registry source to a local directory.
// Uses the same logic as create: local paths are used directly, remote
// go-getter URLs are fetched into a temp directory.
func resolveSyncRegistry(
	ctx context.Context,
	logger *slog.Logger,
	source string,
) (string, func(), error) {
	if source == "" {
		return "", nil, fmt.Errorf("no registry source — set --registry-dir or ensure lockfile has registry_url")
	}

	localDir, _, cleanup, err := resolveRegistrySource(ctx, logger, source)
	if err != nil {
		return "", nil, err
	}

	return localDir, cleanup, nil
}

func fetchRegistry(ctx context.Context, logger *slog.Logger, registryURL, ref string) (string, error) {
	dir, err := os.MkdirTemp("", "forge-sync-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	g := getter.New(logger)

	fetchOpts := getter.FetchOpts{
		Ref: ref,
	}

	if err := g.Fetch(ctx, registryURL, dir, fetchOpts); err != nil {
		cleanupDir(logger, dir)

		return "", err
	}

	return dir, nil
}

func cleanupDir(logger *slog.Logger, dir string) {
	if err := os.RemoveAll(dir); err != nil {
		logger.Warn("failed to clean up temp directory", "dir", dir, "error", err)
	}
}

// requireForceForVarFile enforces the IMPL-0008 OQ-4 contract on
// sync: --var-file rewrites the lockfile with the new resolved
// values, so the caller must pass --force to acknowledge that
// side effect. Returning early when no var-file paths were supplied
// keeps the no-op sync path unchanged.
func requireForceForVarFile(varFilePaths []string, force bool) error {
	if len(varFilePaths) == 0 || force {
		return nil
	}

	return errors.New(
		"--var-file on `forge sync` rewrites the lockfile with the new resolved values. " +
			"Pass --force to confirm:\n" +
			"  forge sync … --var-file FILE --force",
	)
}

func printSyncSummary(w *ui.Writer, result *forgesync.Result) {
	if len(result.Updated) == 0 && len(result.Conflicts) == 0 {
		w.Success("Everything up to date.")

		return
	}

	for _, f := range result.Updated {
		w.Successf("updated: %s", f)
	}

	for _, f := range result.Conflicts {
		w.Warningf("conflict: %s", f)
	}

	for _, f := range result.Skipped {
		w.Infof("skipped: %s", f)
	}

	w.Infof("%d updated, %d conflicts, %d skipped",
		len(result.Updated), len(result.Conflicts), len(result.Skipped))
}
