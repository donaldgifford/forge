package migratecmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/create"
	"github.com/donaldgifford/forge/internal/migratecmd"
)

// TestMigrateConfig_FullV2Registry exercises the migrator end-to-end
// against the in-tree testdata/v2-registry/ corpus. Confirms the
// rewriter handles every field shape that fixture exercises (the
// canonical post-IMPL-0004 v2 layout) and that `forge create` works
// against the migrated tree.
func TestMigrateConfig_FullV2Registry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	copyTree(t, "../../testdata/v2-registry", root)

	result, err := migratecmd.RunMigrateConfig(&migratecmd.MigrateOpts{
		Path:  root,
		Force: true,
	})
	require.NoError(t, err)

	// Three .yaml files in v2-registry: registry + two blueprints.
	require.Len(t, result.Files, 3)

	for _, r := range result.Files {
		assert.True(t, r.Migrated, "%s should migrate", r.Path)
		assert.Empty(t, r.Errors, "%s reported errors: %v", r.Path, r.Errors)
	}

	// HCL load on each migrated config to confirm semantic round-trip.
	regHCL, err := config.LoadRegistryHCL(filepath.Join(root, "registry.hcl"))
	require.NoError(t, err)
	assert.Equal(t, "test-blueprints-v2", regHCL.Name)
	require.Len(t, regHCL.Blueprints, 2)

	goAPIHCL, err := config.LoadBlueprintHCL(filepath.Join(root, "go", "api", "blueprint.hcl"))
	require.NoError(t, err)
	assert.Equal(t, "go-api", goAPIHCL.Name)
	require.GreaterOrEqual(t, len(goAPIHCL.Variables), 3)

	helmHCL, err := config.LoadBlueprintHCL(filepath.Join(root, "helm", "chart", "blueprint.hcl"))
	require.NoError(t, err)
	assert.Equal(t, "helm-chart", helmHCL.Name)

	// Run forge create against the migrated tree. The output must match
	// the equivalent run against the pre-migration YAML registry — the
	// loader sees the .hcl sibling first.
	outputDir := filepath.Join(t.TempDir(), "my-api")
	createResult, err := create.Run(&create.Opts{
		BlueprintRef: "go/api",
		OutputDir:    outputDir,
		RegistryDir:  root,
		UseDefaults:  true,
		ForgeVersion: "0.0.0-migrate-test",
		Overrides: map[string]string{
			"project_name": "my-api",
			"go_module":    "github.com/example/my-api",
			"use_grpc":     "false",
		},
	})
	require.NoError(t, err)
	assert.Positive(t, createResult.FilesCreated)

	main, err := os.ReadFile(filepath.Join(outputDir, "cmd", "main.go"))
	require.NoError(t, err)
	assert.Contains(t, string(main), "Hello from my-api")
}
