package cmd

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/create"
)

var (
	setVars    []string
	outputDir  string
	useDefault bool
	noTools    bool
)

var createCmd = &cobra.Command{
	Use:   "create <blueprint>",
	Short: "Create a new project from a blueprint",
	Long: `Create a new project by scaffolding from a blueprint. The blueprint can be
specified as a short name (e.g., "go/api"), a pinned reference (e.g.,
"go/api@v1.0.0"), or a full go-getter URL.`,
	Args: cobra.ExactArgs(1),
	RunE: runCreate,
}

func init() {
	createCmd.Flags().StringArrayVar(&setVars, "set", nil, "set a variable value (key=value, can be repeated)")
	createCmd.Flags().StringVarP(&outputDir, "output-dir", "o", "", "target output directory")
	createCmd.Flags().BoolVar(&useDefault, "defaults", false, "use all default values without prompting")
	createCmd.Flags().BoolVar(&noTools, "no-tools", false, "skip tool installation")
	rootCmd.AddCommand(createCmd)
}

func runCreate(_ *cobra.Command, args []string) error {
	overrides := parseOverrides(setVars)

	opts := &create.Opts{
		BlueprintRef: args[0],
		OutputDir:    outputDir,
		Overrides:    overrides,
		UseDefaults:  useDefault,
		NoTools:      noTools,
		ForgeVersion: buildVersion,
		Logger:       slog.Default(),
	}

	result, err := create.Run(opts)
	if err != nil {
		return err
	}

	fmt.Printf("Created project %q in %s (%d files)\n", result.Blueprint, result.OutputDir, result.FilesCreated)

	return nil
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
