package lockfile_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/lockfile"
)

// TestHCLRoundTrip_StructToFileAndBack verifies that
// WriteLockfileHCL → LoadLockfileHCL preserves every field of an
// in-memory Lockfile. Pins format equivalence for the loader/emitter
// pair.
func TestHCLRoundTrip_StructToFileAndBack(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Nanosecond)

	original := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			RegistryURL: "github.com/acme/blueprints",
			Name:        "go-api",
			Path:        "go/api",
			Ref:         "v1.0.0",
			Commit:      "abc123",
		},
		CreatedAt:    now,
		LastSynced:   now,
		ForgeVersion: "0.5.0",
		Variables: map[string]any{
			"project_name": "my-api",
			"go_module":    "github.com/example/my-api",
			"use_grpc":     false,
			"port":         int64(8080),
		},
		Defaults: []lockfile.DefaultEntry{
			{Path: ".editorconfig", Source: "registry-default", Strategy: "overwrite", Hash: "sha256:abc"},
			{Path: ".golangci.yml", Source: "category-default", Strategy: "overwrite"},
		},
		ManagedFiles: []lockfile.ManagedFileEntry{
			{Path: "Makefile", Strategy: "merge", SyncedCommit: "abc123"},
			{Path: "Dockerfile", Strategy: "overwrite", Hash: "sha256:def"},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, lockfile.WriteLockfileHCL(&buf, original))

	dir := t.TempDir()
	path := filepath.Join(dir, lockfile.HCLFileName)
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))

	loaded, err := lockfile.LoadLockfileHCL(path)
	require.NoError(t, err)

	assert.Equal(t, original.Blueprint, loaded.Blueprint)
	assert.Equal(t, original.ForgeVersion, loaded.ForgeVersion)
	assert.True(t, original.CreatedAt.Equal(loaded.CreatedAt), "CreatedAt mismatch: %v vs %v", original.CreatedAt, loaded.CreatedAt)
	assert.True(t, original.LastSynced.Equal(loaded.LastSynced), "LastSynced mismatch: %v vs %v", original.LastSynced, loaded.LastSynced)
	assert.Equal(t, original.Variables, loaded.Variables)
	assert.Equal(t, original.Defaults, loaded.Defaults)
	assert.Equal(t, original.ManagedFiles, loaded.ManagedFiles)
}

// TestHCLRoundTrip_YAMLToHCL verifies that a Lockfile loaded from
// YAML can be emitted as HCL and re-loaded with equal struct content.
// Confirms YAML and HCL representations are semantically equivalent
// for every field shape currently in use.
func TestHCLRoundTrip_YAMLToHCL(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	yamlPath := filepath.Join(dir, lockfile.FileName)
	original := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			RegistryURL: "git@example.com:org/registry",
			Name:        "py-cli",
			Path:        "python/cli",
			Ref:         "main",
		},
		CreatedAt:    time.Date(2026, 5, 18, 10, 15, 30, 0, time.UTC),
		LastSynced:   time.Date(2026, 5, 19, 8, 0, 0, 0, time.UTC),
		ForgeVersion: "0.5.0",
		Variables: map[string]any{
			"pkg_name": "mycli",
			"verbose":  true,
		},
		Defaults: []lockfile.DefaultEntry{
			{Path: "pyproject.toml", Source: "registry-default", Strategy: "overwrite"},
		},
	}

	require.NoError(t, lockfile.Write(yamlPath, original))

	fromYAML, err := lockfile.Read(yamlPath)
	require.NoError(t, err)

	hclPath := filepath.Join(dir, lockfile.HCLFileName)
	var buf bytes.Buffer
	require.NoError(t, lockfile.WriteLockfileHCL(&buf, fromYAML))
	require.NoError(t, os.WriteFile(hclPath, buf.Bytes(), 0o644))

	fromHCL, err := lockfile.LoadLockfileHCL(hclPath)
	require.NoError(t, err)

	assert.Equal(t, fromYAML.Blueprint, fromHCL.Blueprint)
	assert.Equal(t, fromYAML.ForgeVersion, fromHCL.ForgeVersion)
	assert.True(t, fromYAML.CreatedAt.Equal(fromHCL.CreatedAt))
	assert.True(t, fromYAML.LastSynced.Equal(fromHCL.LastSynced))
	assert.Equal(t, fromYAML.Variables, fromHCL.Variables)
	assert.Equal(t, fromYAML.Defaults, fromHCL.Defaults)
}
