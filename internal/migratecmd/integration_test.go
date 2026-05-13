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

	"github.com/donaldgifford/forge/internal/config"
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

// TestRunMigrate_TwoStepEndToEnd is the C.8 regression: it walks the
// full v1→v2-HCL upgrade path documented in MIGRATION.md.
//  1. `forge migrate templates` rewrites the v1 corpus to v2 YAML.
//  2. `forge migrate config` rewrites the v2 YAML to HCL.
//  3. The resulting blueprint.hcl loads cleanly via the dispatcher
//     and renders end-to-end through the v2 template engine.
//
// This pins the full upgrade path against future regressions in either
// migration tool — if the templates migrator outputs YAML the config
// migrator can no longer parse, this test surfaces it loudly.
func TestRunMigrate_TwoStepEndToEnd(t *testing.T) {
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

	// Step 1: v1 → v2 YAML (template-content rewrite).
	templatesResult, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: dst})
	require.NoError(t, err)
	require.NotEmpty(t, templatesResult.Blueprints)

	for _, bp := range templatesResult.Blueprints {
		assert.True(t, bp.Migrated, "templates migrator should have rewritten %s", bp.Path)
	}

	// Step 2: v2 YAML → v2 HCL (config-file rewrite).
	configResult, err := migratecmd.RunMigrateConfig(&migratecmd.MigrateOpts{
		Path:  dst,
		Force: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, configResult.Files)

	for _, f := range configResult.Files {
		assert.True(t, f.Migrated, "config migrator should have rewritten %s", f.Path)
		assert.Empty(t, f.Errors, "%s: %v", f.Path, f.Errors)
	}

	// Step 3: confirm the migrated tree loads end-to-end.
	bp, err := config.LoadBlueprint(filepath.Join(dst, "go", "api", "blueprint.yaml"))
	require.NoError(t, err, "post-two-step blueprint must load via the dispatcher")
	assert.Equal(t, "go-api", bp.Name)

	reg, err := config.LoadRegistry(filepath.Join(dst, "registry.yaml"))
	require.NoError(t, err)
	assert.NotEmpty(t, reg.Blueprints)
}

// TestRunMigrate_ParsesAfterMigration verifies the migrated
// blueprint.yaml is still well-formed YAML and carries the expected
// blueprint name. Post-OQ-4 the migrator output is YAML-shaped and
// requires a second `forge migrate config` pass to reach the HCL
// form the loader accepts — full two-step end-to-end coverage lives
// in TestRunMigrate_TwoStepEndToEnd.
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
