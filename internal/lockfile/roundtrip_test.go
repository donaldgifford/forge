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
