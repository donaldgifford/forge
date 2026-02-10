package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/donaldgifford/forge/internal/getter"
)

// DownloadOpts configures a tool download.
type DownloadOpts struct {
	// Tool is the resolved tool to download.
	Tool ResolvedTool
	// Platform is the target platform.
	Platform Platform
	// DestDir is where to place the downloaded tool.
	DestDir string
	// CacheDir is the tool cache root (e.g., ~/.cache/forge/tools/).
	CacheDir string
	// Logger for debug output.
	Logger *slog.Logger
}

// Download fetches a tool based on its source type.
// It checks the cache first and copies from there if available.
func Download(ctx context.Context, opts *DownloadOpts) error {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Check cache.
	if opts.CacheDir != "" {
		cachePath := toolCachePath(opts.CacheDir, opts.Tool.Name, opts.Tool.Version)
		if info, err := os.Stat(cachePath); err == nil && info.IsDir() {
			logger.Debug("tool cache hit", "tool", opts.Tool.Name, "version", opts.Tool.Version)

			return copyDir(cachePath, opts.DestDir)
		}
	}

	// Download based on source type.
	switch opts.Tool.Source.Type {
	case "github-release":
		return downloadGitHubRelease(ctx, opts, logger)
	case "url":
		return downloadURL(ctx, opts, logger)
	case "go-install":
		return goInstall(ctx, opts, logger)
	default:
		return fmt.Errorf("unsupported tool source type: %s", opts.Tool.Source.Type)
	}
}

func downloadGitHubRelease(ctx context.Context, opts *DownloadOpts, logger *slog.Logger) error {
	asset := ResolveAssetURL(opts.Tool.Source.AssetPattern, opts.Tool.Version, opts.Platform)
	url := fmt.Sprintf(
		"https://github.com/%s/releases/download/v%s/%s",
		opts.Tool.Source.Repo,
		opts.Tool.Version,
		asset,
	)

	return fetchToDir(ctx, url, opts, logger)
}

func downloadURL(ctx context.Context, opts *DownloadOpts, logger *slog.Logger) error {
	url := ResolveAssetURL(opts.Tool.Source.URL, opts.Tool.Version, opts.Platform)

	return fetchToDir(ctx, url, opts, logger)
}

func fetchToDir(ctx context.Context, url string, opts *DownloadOpts, logger *slog.Logger) error {
	logger.Debug("downloading tool", "tool", opts.Tool.Name, "url", url)

	destDir := opts.DestDir
	if opts.CacheDir != "" {
		destDir = toolCachePath(opts.CacheDir, opts.Tool.Name, opts.Tool.Version)
	}

	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("creating dest dir: %w", err)
	}

	g := getter.New(logger)

	fetchOpts := getter.FetchOpts{}
	if err := g.Fetch(ctx, url, destDir, fetchOpts); err != nil {
		return fmt.Errorf("fetching %s: %w", opts.Tool.Name, err)
	}

	// Copy from cache to dest if using cache.
	if opts.CacheDir != "" && destDir != opts.DestDir {
		return copyDir(destDir, opts.DestDir)
	}

	return nil
}

func goInstall(ctx context.Context, opts *DownloadOpts, logger *slog.Logger) error {
	module := opts.Tool.Source.Module
	if module == "" {
		return fmt.Errorf("go-install source requires module field for tool %s", opts.Tool.Name)
	}

	target := module + "@" + opts.Tool.Version

	logger.Debug("go install", "tool", opts.Tool.Name, "target", target)

	cmd := exec.CommandContext(ctx, "go", "install", target) //nolint:gosec // target is from blueprint tool config, not user input
	cmd.Env = append(os.Environ(), "GOBIN="+opts.DestDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go install %s: %w", target, err)
	}

	return nil
}

func toolCachePath(cacheDir, name, version string) string {
	return filepath.Join(cacheDir, name, version)
}

// copyDir copies all files from src to dst directory.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o750); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}

			continue
		}

		if err := copyFile(srcPath, dstPath, entry); err != nil {
			return err
		}
	}

	return nil
}

func copyFile(srcPath, dstPath string, entry os.DirEntry) error {
	data, err := os.ReadFile(filepath.Clean(srcPath))
	if err != nil {
		return err
	}

	info, err := entry.Info()
	if err != nil {
		return err
	}

	return os.WriteFile(dstPath, data, info.Mode())
}
