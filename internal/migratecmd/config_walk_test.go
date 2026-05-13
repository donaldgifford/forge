package migratecmd_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/migratecmd"
)

// initGitRepo bootstraps a clean git repo at root so the dirty-worktree
// guard inside RunMigrateConfig accepts the test fixture path.
func initGitRepo(t *testing.T, root string) {
	t.Helper()

	ctx := context.Background()
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, out)
	}
}

// TestRunMigrateConfig_RewritesBlueprintAndRegistry exercises the full
// walker: builds a real-on-disk YAML registry, runs the migrator,
// and verifies the resulting .hcl files load cleanly.
func TestRunMigrateConfig_RewritesBlueprintAndRegistry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initGitRepo(t, root)

	registryYAML := filepath.Join(root, "registry.yaml")
	require.NoError(t, os.WriteFile(registryYAML, []byte(`
apiVersion: v2
name: "test"
description: "fixture"

blueprints:
  - name: go/api
    path: go/api
    description: "Go API"
    version: "2.0.0"
    tags: ["go", "api"]
`), 0o600))

	bpDir := filepath.Join(root, "go", "api")
	require.NoError(t, os.MkdirAll(bpDir, 0o755))

	blueprintYAML := filepath.Join(bpDir, "blueprint.yaml")
	require.NoError(t, os.WriteFile(blueprintYAML, []byte(`
apiVersion: v2
name: "go-api"
description: "fixture"
version: "2.0.0"

variables:
  - name: project_name
    type: string
    required: true

  - name: go_module
    type: string
    default: "github.com/example/${project_name}"
`), 0o600))

	result, err := migratecmd.RunMigrateConfig(&migratecmd.MigrateOpts{
		Path:  root,
		Force: true, // empty git repo is "clean enough"; Force keeps the test independent of the worktree state checker
	})
	require.NoError(t, err)
	require.Len(t, result.Files, 2, "should report on both blueprint.yaml and registry.yaml")

	// All reports should show success and the .yaml originals should be
	// gone.
	for _, r := range result.Files {
		assert.True(t, r.Migrated, "%s should be marked migrated", r.Path)
		assert.Empty(t, r.Errors, "%s reported errors: %v", r.Path, r.Errors)
		assert.NotEmpty(t, r.Output)

		_, statErr := os.Stat(r.Path)
		assert.True(t, os.IsNotExist(statErr), "source YAML %s should be removed", r.Path)

		_, statErr = os.Stat(r.Output)
		require.NoError(t, statErr, "output HCL %s should exist", r.Output)
	}

	// Sanity-load the rewritten files via the HCL loader.
	bp, err := config.LoadBlueprintHCL(filepath.Join(bpDir, "blueprint.hcl"))
	require.NoError(t, err)
	assert.Equal(t, "go-api", bp.Name)
	assert.Equal(t, "github.com/example/${project_name}", bp.Variables[1].Default)

	reg, err := config.LoadRegistryHCL(filepath.Join(root, "registry.hcl"))
	require.NoError(t, err)
	assert.Equal(t, "test", reg.Name)
	require.Len(t, reg.Blueprints, 1)
	assert.Equal(t, "go/api", reg.Blueprints[0].Name)
}

// TestRunMigrateConfig_DryRunWritesNothing verifies dry-run reports
// what would change without touching disk.
func TestRunMigrateConfig_DryRunWritesNothing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	yamlPath := filepath.Join(root, "registry.yaml")
	require.NoError(t, os.WriteFile(yamlPath, []byte(`
apiVersion: v2
name: "x"
`), 0o600))

	result, err := migratecmd.RunMigrateConfig(&migratecmd.MigrateOpts{
		Path:   root,
		DryRun: true,
		Force:  true,
	})
	require.NoError(t, err)
	require.Len(t, result.Files, 1)

	r := result.Files[0]
	assert.False(t, r.Migrated, "dry-run must not mark Migrated=true")
	assert.Empty(t, r.Errors)
	assert.Equal(t, filepath.Join(root, "registry.hcl"), r.Output)

	// Source still exists; output does not.
	_, err = os.Stat(yamlPath)
	require.NoError(t, err, "dry-run must leave source .yaml in place")

	_, err = os.Stat(filepath.Join(root, "registry.hcl"))
	require.True(t, os.IsNotExist(err), "dry-run must not write the .hcl output")
}

// TestRunMigrateConfig_RefusesOnSiblingCollision verifies OQ-5: the
// walker errors when both .yaml and .hcl already coexist.
func TestRunMigrateConfig_RefusesOnSiblingCollision(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "registry.yaml"), []byte(`
apiVersion: v2
name: "x"
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "registry.hcl"),
		[]byte(`name = "preexisting"`), 0o600))

	result, err := migratecmd.RunMigrateConfig(&migratecmd.MigrateOpts{
		Path:  root,
		Force: true,
	})
	require.NoError(t, err) // walker continues past per-file errors
	require.Len(t, result.Files, 1)

	r := result.Files[0]
	assert.True(t, r.Skipped, "collision should be reported as Skipped")
	assert.Equal(t, "sibling .hcl already exists", r.SkipReason)
	assert.NotEmpty(t, r.Errors, "collision should surface an error")
}
