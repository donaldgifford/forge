package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/getter"
	"github.com/donaldgifford/forge/internal/lockfile"
	forgesync "github.com/donaldgifford/forge/internal/sync"
)

var (
	syncDryRun       bool
	syncForce        bool
	syncFileFilter   string
	syncIncludeTools bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync project files with the source blueprint",
	Long: `Synchronize local project files with the latest version from the source
blueprint and registry. Defaults use overwrite strategy; managed files use
overwrite or three-way merge depending on their configuration.`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "print what would change without writing")
	syncCmd.Flags().BoolVarP(&syncForce, "force", "f", false, "skip confirmation prompts")
	syncCmd.Flags().StringVar(&syncFileFilter, "file", "", "sync only a specific file path")
	syncCmd.Flags().BoolVar(&syncIncludeTools, "include-tools", false, "also update installed tools")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, _ []string) error {
	logger := slog.Default()
	projectDir := "."

	lockPath := filepath.Join(projectDir, lockfile.FileName)

	lock, err := lockfile.Read(lockPath)
	if err != nil {
		return fmt.Errorf("reading lockfile: %w", err)
	}

	if lock.Blueprint.RegistryURL == "" {
		return fmt.Errorf("lockfile has no registry_url — cannot determine sync source")
	}

	ctx := cmd.Context()

	// Fetch latest registry content.
	registryDir, err := fetchRegistry(ctx, logger, lock.Blueprint.RegistryURL, "")
	if err != nil {
		return fmt.Errorf("fetching latest registry: %w", err)
	}
	defer cleanupDir(logger, registryDir)

	// Fetch base registry content for three-way merge support.
	var baseDir string

	if lock.Blueprint.Commit != "" {
		baseDir, err = fetchRegistry(ctx, logger, lock.Blueprint.RegistryURL, lock.Blueprint.Commit)
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
	}

	result, err := forgesync.Run(opts)
	if err != nil {
		return err
	}

	printSyncSummary(result)

	if syncIncludeTools {
		logger.Info("tool sync not yet implemented")
	}

	return nil
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

func printSyncSummary(result *forgesync.Result) {
	if len(result.Updated) == 0 && len(result.Conflicts) == 0 {
		fmt.Println("Everything up to date.")

		return
	}

	for _, f := range result.Updated {
		fmt.Printf("  updated: %s\n", f)
	}

	for _, f := range result.Conflicts {
		fmt.Printf("  conflict: %s\n", f)
	}

	for _, f := range result.Skipped {
		fmt.Printf("  skipped: %s\n", f)
	}

	fmt.Printf("\n%d updated, %d conflicts, %d skipped\n",
		len(result.Updated), len(result.Conflicts), len(result.Skipped))
}
