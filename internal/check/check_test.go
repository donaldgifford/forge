package check_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/check"
	"github.com/donaldgifford/forge/internal/lockfile"
)

func setupProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Write lockfile.
	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "test-bp",
			Path: "test/bp",
		},
		Defaults: []lockfile.DefaultEntry{
			{Path: ".editorconfig", Source: "registry-default", Strategy: "overwrite"},
			{Path: ".golangci.yml", Source: "category-default", Strategy: "overwrite"},
		},
		ManagedFiles: []lockfile.ManagedFileEntry{
			{Path: "Makefile", Strategy: "overwrite"},
		},
	}

	require.NoError(t, lockfile.Write(filepath.Join(dir, lockfile.FileName), lock))

	// Create project files.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte("root = true"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".golangci.yml"), []byte("run:\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Makefile"), []byte("all:\n"), 0o644))

	return dir
}

func TestRun_AllUpToDate(t *testing.T) {
	t.Parallel()

	dir := setupProject(t)
	var buf bytes.Buffer

	opts := &check.Opts{
		ProjectDir:   dir,
		OutputFormat: "text",
		Writer:       &buf,
	}

	result, err := check.Run(opts)
	require.NoError(t, err)

	assert.Len(t, result.DefaultsUpdates, 2)
	assert.Len(t, result.ManagedUpdates, 1)

	for _, u := range result.DefaultsUpdates {
		assert.Equal(t, check.StatusUpToDate, u.Status)
	}

	for _, u := range result.ManagedUpdates {
		assert.Equal(t, check.StatusUpToDate, u.Status)
	}
}

func TestRun_MissingFile(t *testing.T) {
	t.Parallel()

	dir := setupProject(t)

	// Remove a tracked file.
	require.NoError(t, os.Remove(filepath.Join(dir, ".golangci.yml")))

	var buf bytes.Buffer

	opts := &check.Opts{
		ProjectDir:   dir,
		OutputFormat: "text",
		Writer:       &buf,
	}

	result, err := check.Run(opts)
	require.NoError(t, err)

	// Find the missing file.
	var found bool

	for _, u := range result.DefaultsUpdates {
		if u.Path == ".golangci.yml" {
			assert.Equal(t, check.StatusMissing, u.Status)

			found = true
		}
	}

	assert.True(t, found, "should detect missing .golangci.yml")
}

func TestRun_JSONOutput(t *testing.T) {
	t.Parallel()

	dir := setupProject(t)
	var buf bytes.Buffer

	opts := &check.Opts{
		ProjectDir:   dir,
		OutputFormat: "json",
		Writer:       &buf,
	}

	_, err := check.Run(opts)
	require.NoError(t, err)

	var result check.Result
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Len(t, result.DefaultsUpdates, 2)
}

func TestRun_NoLockfile(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &check.Opts{
		ProjectDir:   t.TempDir(),
		OutputFormat: "text",
		Writer:       &buf,
	}

	_, err := check.Run(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading lockfile")
}

func TestRun_TextOutput(t *testing.T) {
	t.Parallel()

	dir := setupProject(t)
	var buf bytes.Buffer

	opts := &check.Opts{
		ProjectDir:   dir,
		OutputFormat: "text",
		Writer:       &buf,
	}

	_, err := check.Run(opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "FILE")
	assert.Contains(t, output, ".editorconfig")
	assert.Contains(t, output, "ok")
}
