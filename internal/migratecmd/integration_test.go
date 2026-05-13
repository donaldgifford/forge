package migratecmd_test

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"

	"github.com/donaldgifford/forge/internal/migratecmd"
	tmpl "github.com/donaldgifford/forge/internal/template"
)

const v1RegistryFixture = "../../testdata/v1-registry"

// copyTree shallow-copies the src directory tree to dst, preserving
// file modes. Used to materialise testdata/v1-registry/ into a
// per-test tempdir so the migration tool can write into a fresh tree.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()

	require.NoError(t, filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		return os.WriteFile(target, data, 0o644)
	}))
}

// TestRunMigrate_AgainstV1RegistryFixture migrates the frozen
// testdata/v1-registry corpus and verifies:
//  1. The rewritten templates pass the v2 renderer end-to-end.
//  2. The migrated blueprint.yaml is well-formed YAML (post-OQ-4 the
//     migrator no longer touches the apiVersion field — the user runs
//     `forge migrate config` as a second pass to reach the HCL form).
func TestRunMigrate_AgainstV1RegistryFixture(t *testing.T) {
	t.Parallel()

	dst := t.TempDir()
	copyTree(t, v1RegistryFixture, dst)

	ctx := context.Background()

	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "fixture"},
	} {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dst}, args...)...)
		require.NoError(t, cmd.Run(), "git %v", args)
	}

	result, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: dst})
	require.NoError(t, err)
	require.NotEmpty(t, result.Blueprints)

	for _, bp := range result.Blueprints {
		assert.True(t, bp.Migrated, "blueprint %s should be migrated", bp.Path)
		assert.Empty(t, bp.UntranslatedHits, "no untranslated hits expected for v1-registry corpus")
	}

	// Render a sample of the migrated tmpl files via the v2 renderer
	// and check the substitutions resolve cleanly.
	r := tmpl.NewRenderer()

	mainPath := filepath.Join(dst, "go", "api", "${project_name}", "cmd", "main.go.tmpl")

	mainOut, err := r.RenderFile(mainPath, map[string]cty.Value{
		"project_name": cty.StringVal("my-svc"),
	})
	require.NoError(t, err)
	assert.Contains(t, string(mainOut), `Hello from my-svc`)
}

// TestRunMigrate_AgainstV1RegistryFixture_Strict verifies the rewriter
// produces zero untranslated hits across the entire v1 corpus.
func TestRunMigrate_AgainstV1RegistryFixture_Strict(t *testing.T) {
	t.Parallel()

	dst := t.TempDir()
	copyTree(t, v1RegistryFixture, dst)

	result, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{
		Path:   dst,
		DryRun: true,
		Force:  true,
	})
	require.NoError(t, err)

	for _, bp := range result.Blueprints {
		assert.Empty(t, bp.UntranslatedHits, "blueprint %s", bp.Path)
	}
}

// TestRunMigrate_ParsesAfterMigration verifies the migrated
// blueprint.yaml is still well-formed YAML and carries the expected
// blueprint name. Post-OQ-4 the migrator output is YAML-shaped and
// requires a second `forge migrate config` pass to reach the HCL
// form the loader accepts — full two-step end-to-end coverage lives
// in C.8.
func TestRunMigrate_ParsesAfterMigration(t *testing.T) {
	t.Parallel()

	dst := t.TempDir()
	copyTree(t, v1RegistryFixture, dst)

	ctx := context.Background()
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "fixture"},
	} {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dst}, args...)...)
		require.NoError(t, cmd.Run())
	}

	_, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: dst})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dst, "go", "api", "blueprint.yaml"))
	require.NoError(t, err)

	// Local probe — config.Blueprint dropped its yaml tags in C.6 and
	// the Condition struct can no longer be yaml-unmarshalled directly
	// (its `When hcl.Expression` field has no string-decoder).
	var bp struct {
		Name string `yaml:"name"`
	}
	require.NoError(t, yaml.Unmarshal(data, &bp))
	assert.Equal(t, "go-api", bp.Name)
}
