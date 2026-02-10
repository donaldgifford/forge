package cmd

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/registry"
	"github.com/donaldgifford/forge/internal/ui"
)

var (
	cacheCleanRegistries bool
	cacheCleanAll        bool
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the forge cache",
}

var cacheCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clear cached registries",
	Long:  `Remove cached data to free disk space. By default, cleans all caches.`,
	RunE:  runCacheClean,
}

func init() {
	cacheCleanCmd.Flags().BoolVar(&cacheCleanRegistries, "registries", false, "clean only registry cache")
	cacheCleanCmd.Flags().BoolVar(&cacheCleanAll, "all", false, "clean all caches (default)")
	cacheCmd.AddCommand(cacheCleanCmd)
	rootCmd.AddCommand(cacheCmd)
}

func runCacheClean(_ *cobra.Command, _ []string) error {
	logger := slog.Default()
	w := ui.NewWriter(noColor)

	baseDir := registry.DefaultCacheDir()

	freed, err := cleanDir(filepath.Join(baseDir, "registries"), logger)
	if err != nil {
		return fmt.Errorf("cleaning registry cache: %w", err)
	}

	if freed > 0 {
		w.Successf("Cleaned registry cache (%s)", formatBytes(freed))
	} else {
		w.Info("Registry cache already clean")
	}

	return nil
}

func cleanDir(dir string, logger *slog.Logger) (int64, error) {
	size, err := dirSize(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}

		return 0, err
	}

	logger.Debug("removing cache directory", "dir", dir, "size", size)

	if err := os.RemoveAll(dir); err != nil {
		return 0, fmt.Errorf("removing %s: %w", dir, err)
	}

	return size, nil
}

func dirSize(path string) (int64, error) {
	var size int64

	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			info, infoErr := d.Info()
			if infoErr != nil {
				return infoErr
			}

			size += info.Size()
		}

		return nil
	})

	return size, err
}

func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)

	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
