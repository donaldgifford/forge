package cmd

import (
	"context"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/check"
)

var (
	checkOutputFormat string
	checkRegistryDir  string
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check project for drift against blueprint",
	Long: `Compare the local project state against the source blueprint to detect
changes to defaults, managed files, and tools.

Without --registry-dir, compares local file hashes against lockfile hashes
to detect local modifications.

With --registry-dir, also compares against the registry source to detect
upstream changes. Statuses: modified-locally, upstream-changed, both-changed.`,
	RunE: runCheck,
}

func init() {
	checkCmd.Flags().StringVarP(&checkOutputFormat, "output", "o", "text", "output format (text, json)")
	checkCmd.Flags().StringVar(&checkRegistryDir, "registry-dir", "", "registry source for upstream comparison")
	rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, _ []string) error {
	logger := slog.Default()

	resolvedRegistryDir, cleanup, err := resolveCheckRegistry(cmd.Context(), logger)
	if err != nil {
		return err
	}

	if cleanup != nil {
		defer cleanup()
	}

	opts := &check.Opts{
		ProjectDir:   ".",
		RegistryDir:  resolvedRegistryDir,
		OutputFormat: checkOutputFormat,
		Writer:       os.Stdout,
	}

	_, err = check.Run(opts)

	return err
}

// resolveCheckRegistry resolves the --registry-dir flag for check.
// Reuses the same local/remote detection logic as create and sync.
func resolveCheckRegistry(
	ctx context.Context,
	logger *slog.Logger,
) (string, func(), error) {
	if checkRegistryDir == "" {
		return "", nil, nil
	}

	localDir, _, cleanup, err := resolveRegistrySource(ctx, logger, checkRegistryDir)
	if err != nil {
		return "", nil, err
	}

	return localDir, cleanup, nil
}
