package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
)

func TestLoadBlueprint(t *testing.T) {
	t.Parallel()

	bp, err := config.LoadBlueprint(testdataPath(t, "go/api/blueprint.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "v1", bp.APIVersion)
	assert.Equal(t, "go-api", bp.Name)
	assert.Equal(t, "1.0.0", bp.Version)
	assert.Contains(t, bp.Tags, "go")
	assert.Contains(t, bp.Tags, "api")

	// Variables.
	require.Len(t, bp.Variables, 4)
	assert.Equal(t, "project_name", bp.Variables[0].Name)
	assert.Equal(t, "string", bp.Variables[0].Type)
	assert.True(t, bp.Variables[0].Required)
	assert.Equal(t, "^[a-z][a-z0-9-]*$", bp.Variables[0].Validate)

	assert.Equal(t, "license", bp.Variables[3].Name)
	assert.Equal(t, "choice", bp.Variables[3].Type)
	assert.Equal(t, []string{"MIT", "Apache-2.0", "BSD-3-Clause", "none"}, bp.Variables[3].Choices)

	// Defaults.
	assert.Contains(t, bp.Defaults.Exclude, ".pre-commit-config.yaml")
	assert.Equal(t, "merge", bp.Defaults.OverrideStrategy["renovate.json"])

	// Conditions.
	require.Len(t, bp.Conditions, 1)
	assert.Equal(t, "{{ not .use_grpc }}", bp.Conditions[0].When)
	assert.Contains(t, bp.Conditions[0].Exclude, "proto/")

	// Hooks.
	assert.Contains(t, bp.Hooks.PostCreate, "git init")

	// Sync.
	require.Len(t, bp.Sync.ManagedFiles, 1)
	assert.Equal(t, "Makefile", bp.Sync.ManagedFiles[0].Path)
	assert.Equal(t, "merge", bp.Sync.ManagedFiles[0].Strategy)

	// Rename.
	assert.Equal(t, ".", bp.Rename["{{project_name}}/"])
}

func TestLoadRegistry(t *testing.T) {
	t.Parallel()

	reg, err := config.LoadRegistry(testdataPath(t, "registry.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "v1", reg.APIVersion)
	assert.Equal(t, "test-blueprints", reg.Name)

	// Maintainers.
	require.Len(t, reg.Maintainers, 1)
	assert.Equal(t, "Test Team", reg.Maintainers[0].Name)

	// Defaults.
	assert.Equal(t, "overwrite", reg.Defaults.SyncStrategy)
	assert.True(t, reg.Defaults.Managed)

	// Blueprints.
	require.Len(t, reg.Blueprints, 2)
	assert.Equal(t, "go/api", reg.Blueprints[0].Name)
	assert.Equal(t, "go/api", reg.Blueprints[0].Path)
	assert.Contains(t, reg.Blueprints[0].Tags, "go")
}

func TestLoadBlueprint_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := config.LoadBlueprint("/nonexistent/blueprint.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading blueprint file")
}

func TestLoadRegistry_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := config.LoadRegistry("/nonexistent/registry.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading registry file")
}

func TestLoadBlueprint_InvalidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.yaml")
	require.NoError(t, os.WriteFile(path, []byte("{{invalid yaml"), 0o644))

	_, err := config.LoadBlueprint(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing blueprint file")
}

func TestLoadBlueprint_ValidationError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.yaml")

	// Missing apiVersion.
	content := []byte("name: test\n")
	require.NoError(t, os.WriteFile(path, content, 0o644))

	_, err := config.LoadBlueprint(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apiVersion")
}

func testdataPath(t *testing.T, relPath string) string {
	t.Helper()

	// Find testdata relative to the repo root.
	path := filepath.Join("..", "..", "testdata", "registry", relPath)
	absPath, err := filepath.Abs(path)
	require.NoError(t, err)

	return absPath
}
