package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/create"
	"github.com/donaldgifford/forge/internal/ui"
)

var (
	setVars     []string
	outputDir   string
	useDefault  bool
	noTools     bool
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
	createCmd.Flags().BoolVar(&noTools, "no-tools", false, "skip tool installation")
	createCmd.Flags().BoolVar(&noHooks, "no-hooks", false, "skip post-create hooks")
	createCmd.Flags().BoolVar(&forceCreate, "force", false, "overwrite existing non-empty output directory")
	rootCmd.AddCommand(createCmd)
}

func runCreate(_ *cobra.Command, args []string) error {
	overrides := parseOverrides(setVars)
	logger := slog.Default()

	// Resolve registry-dir to an absolute path if it's a local directory.
	resolvedRegistryDir, err := resolveRegistryDir(registryDir)
	if err != nil {
		return err
	}

	opts := &create.Opts{
		BlueprintRef: args[0],
		OutputDir:    outputDir,
		RegistryDir:  resolvedRegistryDir,
		Overrides:    overrides,
		UseDefaults:  useDefault,
		NoTools:      noTools,
		NoHooks:      noHooks,
		ForceCreate:  forceCreate,
		ForgeVersion: buildVersion,
		Logger:       logger,
	}

	result, err := create.Run(opts)
	if err != nil {
		return err
	}

	w := ui.NewWriter(noColor)
	w.Successf("Created project %q in %s (%d files)", result.Blueprint, result.OutputDir, result.FilesCreated)

	return nil
}

// resolveRegistryDir resolves --registry-dir to an absolute path if it points
// to a local directory. Returns the value unchanged if empty.
func resolveRegistryDir(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}

	// Check if the path exists on the local filesystem.
	info, err := os.Stat(dir)
	if err == nil && info.IsDir() {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", fmt.Errorf("resolving registry-dir path: %w", err)
		}

		return abs, nil
	}

	// Not a local directory — return as-is (will be treated as go-getter URL in Gap 2).
	return dir, nil
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
