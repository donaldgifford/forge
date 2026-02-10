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

	editorContent := []byte("root = true")
	golangciContent := []byte("run:\n")
	makefileContent := []byte("all:\n")

	// Write lockfile with content hashes.
	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "test-bp",
			Path: "test/bp",
		},
		Defaults: []lockfile.DefaultEntry{
			{Path: ".editorconfig", Source: "registry-default", Strategy: "overwrite", Hash: lockfile.ContentHash(editorContent)},
			{Path: ".golangci.yml", Source: "category-default", Strategy: "overwrite", Hash: lockfile.ContentHash(golangciContent)},
		},
		ManagedFiles: []lockfile.ManagedFileEntry{
			{Path: "Makefile", Strategy: "overwrite", Hash: lockfile.ContentHash(makefileContent)},
		},
	}

	require.NoError(t, lockfile.Write(filepath.Join(dir, lockfile.FileName), lock))

	// Create project files.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".editorconfig"), editorContent, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".golangci.yml"), golangciContent, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Makefile"), makefileContent, 0o644))

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

func TestRun_ModifiedFile(t *testing.T) {
	t.Parallel()

	dir := setupProject(t)

	// Modify a tracked file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte("root = false\nmodified"), 0o644))

	var buf bytes.Buffer

	opts := &check.Opts{
		ProjectDir:   dir,
		OutputFormat: "text",
		Writer:       &buf,
	}

	result, err := check.Run(opts)
	require.NoError(t, err)

	var found bool

	for _, u := range result.DefaultsUpdates {
		if u.Path == ".editorconfig" {
			assert.Equal(t, check.StatusModified, u.Status)

			found = true
		}
	}

	assert.True(t, found, "should detect modified .editorconfig")

	// Unmodified file should still be up-to-date.
	for _, u := range result.DefaultsUpdates {
		if u.Path == ".golangci.yml" {
			assert.Equal(t, check.StatusUpToDate, u.Status)
		}
	}
}

func TestRun_NoHashInLockfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Write lockfile WITHOUT hashes (backwards compatibility).
	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "test-bp",
			Path: "test/bp",
		},
		Defaults: []lockfile.DefaultEntry{
			{Path: ".editorconfig", Source: "registry-default", Strategy: "overwrite"},
		},
	}

	require.NoError(t, lockfile.Write(filepath.Join(dir, lockfile.FileName), lock))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte("anything"), 0o644))

	var buf bytes.Buffer

	opts := &check.Opts{
		ProjectDir:   dir,
		OutputFormat: "text",
		Writer:       &buf,
	}

	result, err := check.Run(opts)
	require.NoError(t, err)

	// Without hash, file existence alone means up-to-date.
	assert.Equal(t, check.StatusUpToDate, result.DefaultsUpdates[0].Status)
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

// setupProjectWithRegistry creates a project and a matching registry directory
// for three-way comparison tests.
func setupProjectWithRegistry(t *testing.T) (projectDir, registryDir string) {
	t.Helper()

	projectDir = t.TempDir()
	registryDir = t.TempDir()

	editorContent := []byte("root = true")
	makefileContent := []byte("all:\n")

	// Write lockfile with content hashes.
	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "test-bp",
			Path: "test/bp",
		},
		Variables: map[string]any{"project_name": "test"},
		Defaults: []lockfile.DefaultEntry{
			{Path: ".editorconfig", Source: "registry-default", Strategy: "overwrite", Hash: lockfile.ContentHash(editorContent)},
		},
		ManagedFiles: []lockfile.ManagedFileEntry{
			{Path: "Makefile", Strategy: "overwrite", Hash: lockfile.ContentHash(makefileContent)},
		},
	}

	require.NoError(t, lockfile.Write(filepath.Join(projectDir, lockfile.FileName), lock))

	// Create project files matching lockfile hashes.
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, ".editorconfig"), editorContent, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "Makefile"), makefileContent, 0o644))

	// Create registry source files matching lockfile hashes.
	require.NoError(t, os.MkdirAll(filepath.Join(registryDir, "_defaults"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(registryDir, "test", "bp"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(registryDir, "_defaults", ".editorconfig"), editorContent, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(registryDir, "test", "bp", "Makefile"), makefileContent, 0o644))

	return projectDir, registryDir
}

func TestRun_RegistryComparison_AllUpToDate(t *testing.T) {
	t.Parallel()

	projectDir, registryDir := setupProjectWithRegistry(t)
	var buf bytes.Buffer

	opts := &check.Opts{
		ProjectDir:   projectDir,
		RegistryDir:  registryDir,
		OutputFormat: "text",
		Writer:       &buf,
	}

	result, err := check.Run(opts)
	require.NoError(t, err)

	for _, u := range result.DefaultsUpdates {
		assert.Equal(t, check.StatusUpToDate, u.Status, "file %s", u.Path)
	}

	for _, u := range result.ManagedUpdates {
		assert.Equal(t, check.StatusUpToDate, u.Status, "file %s", u.Path)
	}
}

func TestRun_RegistryComparison_UpstreamChanged(t *testing.T) {
	t.Parallel()

	projectDir, registryDir := setupProjectWithRegistry(t)

	// Update the registry source (upstream change).
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "_defaults", ".editorconfig"),
		[]byte("root = true\n# updated upstream"),
		0o644,
	))

	var buf bytes.Buffer

	opts := &check.Opts{
		ProjectDir:   projectDir,
		RegistryDir:  registryDir,
		OutputFormat: "text",
		Writer:       &buf,
	}

	result, err := check.Run(opts)
	require.NoError(t, err)

	assert.Equal(t, check.StatusUpstreamChanged, result.DefaultsUpdates[0].Status)
}

func TestRun_RegistryComparison_ModifiedLocally(t *testing.T) {
	t.Parallel()

	projectDir, registryDir := setupProjectWithRegistry(t)

	// Modify local file (local change, registry unchanged).
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, ".editorconfig"),
		[]byte("root = false\nlocal edit"),
		0o644,
	))

	var buf bytes.Buffer

	opts := &check.Opts{
		ProjectDir:   projectDir,
		RegistryDir:  registryDir,
		OutputFormat: "text",
		Writer:       &buf,
	}

	result, err := check.Run(opts)
	require.NoError(t, err)

	assert.Equal(t, check.StatusModifiedLocally, result.DefaultsUpdates[0].Status)
}

func TestRun_RegistryComparison_BothChanged(t *testing.T) {
	t.Parallel()

	projectDir, registryDir := setupProjectWithRegistry(t)

	// Modify local file.
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, ".editorconfig"),
		[]byte("local changes"),
		0o644,
	))

	// Update registry source.
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "_defaults", ".editorconfig"),
		[]byte("upstream changes"),
		0o644,
	))

	var buf bytes.Buffer

	opts := &check.Opts{
		ProjectDir:   projectDir,
		RegistryDir:  registryDir,
		OutputFormat: "json",
		Writer:       &buf,
	}

	result, err := check.Run(opts)
	require.NoError(t, err)

	assert.Equal(t, check.StatusBothChanged, result.DefaultsUpdates[0].Status)

	// Verify JSON output includes the status.
	var jsonResult check.Result
	require.NoError(t, json.Unmarshal(buf.Bytes(), &jsonResult))
	assert.Equal(t, check.StatusBothChanged, jsonResult.DefaultsUpdates[0].Status)
}

func TestRun_RegistryComparison_ManagedFile_UpstreamChanged(t *testing.T) {
	t.Parallel()

	projectDir, registryDir := setupProjectWithRegistry(t)

	// Update managed file in registry (blueprint directory).
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "test", "bp", "Makefile"),
		[]byte("all:\n\t@echo updated\n"),
		0o644,
	))

	var buf bytes.Buffer

	opts := &check.Opts{
		ProjectDir:   projectDir,
		RegistryDir:  registryDir,
		OutputFormat: "text",
		Writer:       &buf,
	}

	result, err := check.Run(opts)
	require.NoError(t, err)

	assert.Equal(t, check.StatusUpstreamChanged, result.ManagedUpdates[0].Status)
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
