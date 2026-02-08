package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/lockfile"
	"github.com/donaldgifford/forge/internal/registry"
	"github.com/donaldgifford/forge/internal/tools"
)

var toolsInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install project tools",
	Long:  `Download and install tools declared in the project's blueprint.`,
	RunE:  runToolsInstall,
}

func init() {
	toolsCmd.AddCommand(toolsInstallCmd)
}

func runToolsInstall(cmd *cobra.Command, _ []string) error {
	lock, err := lockfile.Read(lockfile.FileName)
	if err != nil {
		return fmt.Errorf("reading lockfile: %w (is this a forge project?)", err)
	}

	if len(lock.Tools) == 0 {
		fmt.Println("No tools declared in this project.")

		return nil
	}

	cacheDir := registry.DefaultCacheDir() + "/tools"
	platform := tools.DetectPlatform()
	logger := slog.Default()

	for i := range lock.Tools {
		t := &lock.Tools[i]

		fmt.Printf("Installing %s@%s...\n", t.Name, t.Version)

		opts := &tools.DownloadOpts{
			Tool: tools.ResolvedTool{
				Name:    t.Name,
				Version: t.Version,
				Source:  config.ToolSource{Type: t.Source},
			},
			Platform: platform,
			DestDir:  ".forge/tools",
			CacheDir: cacheDir,
			Logger:   logger,
		}

		if err := tools.Download(cmd.Context(), opts); err != nil {
			logger.Warn("failed to install tool", "tool", t.Name, "err", err)

			continue
		}
	}

	fmt.Println("Done.")

	return nil
}

// InstallTools installs tools for a freshly created project.
// This is called from the create flow, not directly by users.
func InstallTools(ctx context.Context, resolved []tools.ResolvedTool, destDir, cacheDir string, logger *slog.Logger) error {
	platform := tools.DetectPlatform()

	for i := range resolved {
		t := &resolved[i]

		opts := &tools.DownloadOpts{
			Tool:     *t,
			Platform: platform,
			DestDir:  destDir,
			CacheDir: cacheDir,
			Logger:   logger,
		}

		if err := tools.Download(ctx, opts); err != nil {
			logger.Warn("failed to install tool", "tool", t.Name, "err", err)
		}
	}

	return nil
}
