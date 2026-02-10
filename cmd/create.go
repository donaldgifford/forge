package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/create"
	"github.com/donaldgifford/forge/internal/getter"
	"github.com/donaldgifford/forge/internal/registry"
	"github.com/donaldgifford/forge/internal/ui"
)

var (
	setVars     []string
	outputDir   string
	useDefault  bool
	noHooks     bool
	registryDir string
	forceCreate bool
)

var createCmd = &cobra.Command{
	Use:   "create <blueprint>",
	Short: "Create a new project from a blueprint",
	Long: `Create a new project by scaffolding from a blueprint. The blueprint can be
specified as a short name (e.g., "go/api"), a pinned reference (e.g.,
"go/api@v1.0.0"), or a full go-getter URL.

Use --registry-dir to specify a local directory or remote go-getter URL
as the blueprint registry source.`,
	Args: cobra.ExactArgs(1),
	RunE: runCreate,
}

func init() {
	createCmd.Flags().StringArrayVar(&setVars, "set", nil, "set a variable value (key=value, can be repeated)")
	createCmd.Flags().StringVarP(&outputDir, "output-dir", "o", "", "target output directory")
	createCmd.Flags().StringVar(&registryDir, "registry-dir", "", "path or URL to the blueprint registry")
	createCmd.Flags().BoolVar(&useDefault, "defaults", false, "use all default values without prompting")
	createCmd.Flags().BoolVar(&noHooks, "no-hooks", false, "skip post-create hooks")
	createCmd.Flags().BoolVar(&forceCreate, "force", false, "overwrite existing non-empty output directory")
	rootCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	overrides := parseOverrides(setVars)
	logger := slog.Default()
	blueprintRef := args[0]

	var (
		resolvedDir string
		regURL      string
		cleanup     func()
		defaultURL  string
		err         error
	)

	if registryDir != "" {
		// Explicit --registry-dir: resolve as local path or go-getter URL.
		resolvedDir, regURL, cleanup, err = resolveRegistrySource(
			cmd.Context(), logger, registryDir,
		)
		if err != nil {
			return err
		}
	} else {
		// No --registry-dir: resolve from global config or full URL in blueprint ref.
		resolvedDir, regURL, defaultURL, cleanup, err = resolveFromConfig(
			cmd.Context(), logger, blueprintRef,
		)
		if err != nil {
			return err
		}
	}

	if cleanup != nil {
		defer cleanup()
	}

	opts := &create.Opts{
		BlueprintRef:       blueprintRef,
		OutputDir:          outputDir,
		RegistryDir:        resolvedDir,
		RegistryURL:        regURL,
		DefaultRegistryURL: defaultURL,
		Overrides:          overrides,
		UseDefaults:        useDefault,
		NoHooks:            noHooks,
		ForceCreate:        forceCreate,
		ForgeVersion:       buildVersion,
		Logger:             logger,
	}

	result, err := create.Run(opts)
	if err != nil {
		return err
	}

	w := ui.NewWriter(noColor)
	w.Successf("Created project %q in %s (%d files)", result.Blueprint, result.OutputDir, result.FilesCreated)

	return nil
}

// resolveFromConfig resolves a registry from global config when --registry-dir
// is not provided. For full go-getter URLs (containing "//"), it fetches the
// registry directly. For short names, it looks up the default registry from
// config and fetches that.
func resolveFromConfig(
	ctx context.Context,
	logger *slog.Logger,
	blueprintRef string,
) (localDir, registryURL, defaultRegistryURL string, cleanup func(), err error) {
	// Check if the blueprint ref is a full go-getter URL.
	if strings.Contains(blueprintRef, "//") {
		resolved, resolveErr := registry.Resolve(blueprintRef, "")
		if resolveErr != nil {
			return "", "", "", nil, fmt.Errorf("resolving blueprint: %w", resolveErr)
		}

		dir, url, cleanFn, fetchErr := resolveRegistrySource(ctx, logger, resolved.RegistryURL)
		if fetchErr != nil {
			return "", "", "", nil, fetchErr
		}

		return dir, url, "", cleanFn, nil
	}

	// Short name — load global config and find default registry.
	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = filepath.Join(config.DefaultConfigDir(), "config.yaml")
	}

	globalCfg, cfgErr := config.LoadGlobalConfig(cfgPath)
	if cfgErr != nil {
		return "", "", "", nil, fmt.Errorf("loading config: %w", cfgErr)
	}

	reg, regErr := globalCfg.FindRegistry("")
	if regErr != nil {
		return "", "", "", nil, fmt.Errorf(
			"no registry directory provided — use --registry-dir or configure a default registry in %s", cfgPath,
		)
	}

	dir, url, cleanFn, fetchErr := resolveRegistrySource(ctx, logger, reg.URL)
	if fetchErr != nil {
		return "", "", "", nil, fetchErr
	}

	return dir, url, reg.URL, cleanFn, nil
}

// resolveRegistrySource resolves --registry-dir to a local directory path.
// If the value is a local directory, it returns the absolute path.
// If the value is a remote go-getter URL, it fetches into a temp directory
// and returns both the temp path and a cleanup function.
// The registryURL return value is the canonical URL to store in the lockfile.
func resolveRegistrySource(
	ctx context.Context,
	logger *slog.Logger,
	dir string,
) (localDir, registryURL string, cleanup func(), err error) {
	if dir == "" {
		return "", "", nil, nil
	}

	// Check if the path exists on the local filesystem.
	info, statErr := os.Stat(dir)
	if statErr == nil && info.IsDir() {
		abs, absErr := filepath.Abs(dir)
		if absErr != nil {
			return "", "", nil, fmt.Errorf("resolving registry-dir path: %w", absErr)
		}

		return abs, abs, nil, nil
	}

	// Not a local directory — treat as a go-getter URL and fetch.
	logger.Info("fetching registry", "source", dir)

	tmpDir, tmpErr := os.MkdirTemp("", "forge-registry-*")
	if tmpErr != nil {
		return "", "", nil, fmt.Errorf("creating temp directory: %w", tmpErr)
	}

	g := getter.New(logger)
	if fetchErr := g.Fetch(ctx, dir, tmpDir, getter.FetchOpts{}); fetchErr != nil {
		cleanupDir(logger, tmpDir)

		return "", "", nil, fmt.Errorf("fetching registry from %s: %w", dir, fetchErr)
	}

	cleanupFn := func() { cleanupDir(logger, tmpDir) }

	return tmpDir, dir, cleanupFn, nil
}

// parseOverrides converts --set key=value strings to a map.
func parseOverrides(setFlags []string) map[string]string {
	overrides := make(map[string]string, len(setFlags))

	for _, s := range setFlags {
		key, value, found := strings.Cut(s, "=")
		if found {
			overrides[key] = value
		}
	}

	return overrides
}
