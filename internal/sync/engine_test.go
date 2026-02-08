package sync_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/lockfile"
	forgesync "github.com/donaldgifford/forge/internal/sync"
)

func setupSyncTest(t *testing.T) (projectDir, registryDir string) {
	t.Helper()

	projectDir = t.TempDir()
	registryDir = t.TempDir()

	// Create registry _defaults/ files.
	require.NoError(t, os.MkdirAll(filepath.Join(registryDir, "_defaults"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "_defaults", ".editorconfig"),
		[]byte("root = true\nindent_style = space\n"),
		0o644,
	))

	// Create lockfile.
	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "test-bp",
			Path: "test/bp",
		},
		Defaults: []lockfile.DefaultEntry{
			{Path: ".editorconfig", Source: "registry-default", Strategy: "overwrite"},
		},
		Variables: map[string]any{},
	}

	require.NoError(t, lockfile.Write(filepath.Join(projectDir, lockfile.FileName), lock))

	// Create local files (initially matching).
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, ".editorconfig"),
		[]byte("root = true\nindent_style = space\n"),
		0o644,
	))

	return projectDir, registryDir
}

func TestSync_NoChanges(t *testing.T) {
	t.Parallel()

	projectDir, registryDir := setupSyncTest(t)

	opts := &forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
	}

	result, err := forgesync.Run(opts)
	require.NoError(t, err)

	assert.Empty(t, result.Updated)
	assert.NotEmpty(t, result.Skipped)
}

func TestSync_OverwriteUpdatedFile(t *testing.T) {
	t.Parallel()

	projectDir, registryDir := setupSyncTest(t)

	// Update registry content.
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "_defaults", ".editorconfig"),
		[]byte("root = true\nindent_style = tab\n"),
		0o644,
	))

	opts := &forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
	}

	result, err := forgesync.Run(opts)
	require.NoError(t, err)

	assert.Len(t, result.Updated, 1)

	// Verify file was updated.
	content, err := os.ReadFile(filepath.Join(projectDir, ".editorconfig"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "indent_style = tab")
}

func TestSync_DryRun(t *testing.T) {
	t.Parallel()

	projectDir, registryDir := setupSyncTest(t)

	// Update registry content.
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "_defaults", ".editorconfig"),
		[]byte("root = true\nindent_style = tab\n"),
		0o644,
	))

	opts := &forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
		DryRun:      true,
	}

	result, err := forgesync.Run(opts)
	require.NoError(t, err)

	assert.Len(t, result.Updated, 1)

	// Verify file was NOT updated.
	content, err := os.ReadFile(filepath.Join(projectDir, ".editorconfig"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "indent_style = space")
}

func TestSync_FileFilter(t *testing.T) {
	t.Parallel()

	projectDir, registryDir := setupSyncTest(t)

	opts := &forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
		FileFilter:  "nonexistent.txt",
	}

	result, err := forgesync.Run(opts)
	require.NoError(t, err)

	// Nothing should be synced because the filter doesn't match.
	assert.Empty(t, result.Updated)
	assert.Empty(t, result.Skipped)
}

func TestSync_MergeStrategy_CleanMerge(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	registryDir := t.TempDir()
	baseDir := t.TempDir()

	baseContent := "line1\nline2\nline3\n"
	localContent := "line1-local\nline2\nline3\n"
	remoteContent := "line1\nline2\nline3-remote\n"

	// Set up base registry (last synced version).
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "test", "bp"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(baseDir, "test", "bp", "Makefile"),
		[]byte(baseContent),
		0o644,
	))

	// Set up current registry (remote).
	require.NoError(t, os.MkdirAll(filepath.Join(registryDir, "test", "bp"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "test", "bp", "Makefile"),
		[]byte(remoteContent),
		0o644,
	))

	// Set up local project with local modifications.
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, "Makefile"),
		[]byte(localContent),
		0o644,
	))

	// Create lockfile with merge strategy.
	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "test-bp",
			Path: "test/bp",
		},
		ManagedFiles: []lockfile.ManagedFileEntry{
			{Path: "Makefile", Strategy: "merge"},
		},
		Variables: map[string]any{},
	}

	require.NoError(t, lockfile.Write(filepath.Join(projectDir, lockfile.FileName), lock))

	opts := &forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
		BaseDir:     baseDir,
	}

	result, err := forgesync.Run(opts)
	require.NoError(t, err)

	assert.Len(t, result.Updated, 1)
	assert.Empty(t, result.Conflicts)

	// Verify merged content has both changes.
	content, err := os.ReadFile(filepath.Join(projectDir, "Makefile"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "line1-local")
	assert.Contains(t, string(content), "line3-remote")
}

func TestSync_MergeStrategy_Conflict(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	registryDir := t.TempDir()
	baseDir := t.TempDir()

	baseContent := "line1\nline2\nline3\n"
	localContent := "line1\nline2-local\nline3\n"
	remoteContent := "line1\nline2-remote\nline3\n"

	// Set up base registry.
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "test", "bp"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(baseDir, "test", "bp", "Makefile"),
		[]byte(baseContent),
		0o644,
	))

	// Set up current registry.
	require.NoError(t, os.MkdirAll(filepath.Join(registryDir, "test", "bp"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "test", "bp", "Makefile"),
		[]byte(remoteContent),
		0o644,
	))

	// Set up local project.
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, "Makefile"),
		[]byte(localContent),
		0o644,
	))

	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "test-bp",
			Path: "test/bp",
		},
		ManagedFiles: []lockfile.ManagedFileEntry{
			{Path: "Makefile", Strategy: "merge"},
		},
		Variables: map[string]any{},
	}

	require.NoError(t, lockfile.Write(filepath.Join(projectDir, lockfile.FileName), lock))

	opts := &forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
		BaseDir:     baseDir,
	}

	result, err := forgesync.Run(opts)
	require.NoError(t, err)

	assert.Len(t, result.Updated, 1)
	assert.Len(t, result.Conflicts, 1)
	assert.Contains(t, result.Conflicts, "Makefile")

	// Verify conflict markers are present.
	content, err := os.ReadFile(filepath.Join(projectDir, "Makefile"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "<<<<<<< local")
	assert.Contains(t, string(content), ">>>>>>> remote")
}

func TestSync_MergeStrategy_NoBaseDir_FallsBackToOverwrite(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	registryDir := t.TempDir()

	localContent := "line1\nline2-local\nline3\n"
	remoteContent := "line1\nline2-remote\nline3\n"

	// Set up registry.
	require.NoError(t, os.MkdirAll(filepath.Join(registryDir, "test", "bp"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "test", "bp", "Makefile"),
		[]byte(remoteContent),
		0o644,
	))

	// Set up local project.
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, "Makefile"),
		[]byte(localContent),
		0o644,
	))

	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "test-bp",
			Path: "test/bp",
		},
		ManagedFiles: []lockfile.ManagedFileEntry{
			{Path: "Makefile", Strategy: "merge"},
		},
		Variables: map[string]any{},
	}

	require.NoError(t, lockfile.Write(filepath.Join(projectDir, lockfile.FileName), lock))

	// No BaseDir — should fall back to overwrite.
	opts := &forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
	}

	result, err := forgesync.Run(opts)
	require.NoError(t, err)

	assert.Len(t, result.Updated, 1)

	// File should be overwritten with remote content.
	content, err := os.ReadFile(filepath.Join(projectDir, "Makefile"))
	require.NoError(t, err)
	assert.Equal(t, remoteContent, string(content))
}

func TestSync_MissingSource(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	registryDir := t.TempDir()

	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "test-bp",
			Path: "test/bp",
		},
		Defaults: []lockfile.DefaultEntry{
			{Path: "nonexistent.file", Source: "registry-default", Strategy: "overwrite"},
		},
		Variables: map[string]any{},
	}

	require.NoError(t, lockfile.Write(filepath.Join(projectDir, lockfile.FileName), lock))

	opts := &forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
	}

	result, err := forgesync.Run(opts)
	require.NoError(t, err)
	assert.Contains(t, result.Skipped, "nonexistent.file")
}
