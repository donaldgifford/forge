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

// TestSync_FullCycle exercises the complete sync lifecycle:
// 1. Project created with initial content.
// 2. Registry updated upstream.
// 3. forge sync applies changes.
// 4. Lockfile updated.
func TestSync_FullCycle(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	registryDir := t.TempDir()

	// Set up initial state (simulating post-create).
	require.NoError(t, os.MkdirAll(filepath.Join(registryDir, "_defaults"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(registryDir, "test", "bp"), 0o750))

	initialContent := "root = true\nindent_style = space\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "_defaults", ".editorconfig"),
		[]byte(initialContent), 0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "test", "bp", "Makefile"),
		[]byte("build:\n\tgo build ./...\n"), 0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, ".editorconfig"),
		[]byte(initialContent), 0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, "Makefile"),
		[]byte("build:\n\tgo build ./...\n"), 0o644,
	))

	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "test-bp",
			Path: "test/bp",
		},
		Defaults: []lockfile.DefaultEntry{
			{Path: ".editorconfig", Source: "registry-default", Strategy: "overwrite"},
		},
		ManagedFiles: []lockfile.ManagedFileEntry{
			{Path: "Makefile", Strategy: "overwrite"},
		},
		Variables: map[string]any{},
	}
	require.NoError(t, lockfile.Write(filepath.Join(projectDir, lockfile.FileName), lock))

	// Simulate upstream registry update.
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "_defaults", ".editorconfig"),
		[]byte("root = true\nindent_style = tab\nindent_size = 4\n"), 0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "test", "bp", "Makefile"),
		[]byte("build:\n\tgo build ./...\ntest:\n\tgo test ./...\n"), 0o644,
	))

	// Run sync.
	opts := &forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
	}
	result, err := forgesync.Run(opts)
	require.NoError(t, err)

	assert.Len(t, result.Updated, 2)
	assert.Empty(t, result.Conflicts)

	// Verify files updated.
	editorContent, err := os.ReadFile(filepath.Join(projectDir, ".editorconfig"))
	require.NoError(t, err)
	assert.Contains(t, string(editorContent), "indent_style = tab")
	assert.Contains(t, string(editorContent), "indent_size = 4")

	makeContent, err := os.ReadFile(filepath.Join(projectDir, "Makefile"))
	require.NoError(t, err)
	assert.Contains(t, string(makeContent), "go test")

	// Verify lockfile was updated.
	updatedLock, err := lockfile.Read(filepath.Join(projectDir, lockfile.FileName))
	require.NoError(t, err)
	assert.False(t, updatedLock.LastSynced.IsZero())
}

// TestSync_MergeWithConflict_FullCycle tests a three-way merge scenario
// where local and remote make conflicting changes.
func TestSync_MergeWithConflict_FullCycle(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	registryDir := t.TempDir()
	baseDir := t.TempDir()

	baseContent := "line1\nline2\nline3\nline4\n"

	// Set up base (last synced version).
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "test", "bp"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(baseDir, "test", "bp", "config.yaml"),
		[]byte(baseContent), 0o644,
	))

	// Local made changes on line 2.
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, "config.yaml"),
		[]byte("line1\nline2-local\nline3\nline4\n"), 0o644,
	))

	// Remote made changes on line 2 and line 4.
	require.NoError(t, os.MkdirAll(filepath.Join(registryDir, "test", "bp"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "test", "bp", "config.yaml"),
		[]byte("line1\nline2-remote\nline3\nline4-remote\n"), 0o644,
	))

	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "test-bp",
			Path: "test/bp",
		},
		ManagedFiles: []lockfile.ManagedFileEntry{
			{Path: "config.yaml", Strategy: "merge"},
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

	// Line 2 conflicts, line 4 should merge cleanly.
	assert.Len(t, result.Conflicts, 1)
	assert.Contains(t, result.Conflicts, "config.yaml")
	assert.Len(t, result.ConflictFiles, 1)

	// Verify merged content has conflict markers for line 2 and clean merge for line 4.
	content, err := os.ReadFile(filepath.Join(projectDir, "config.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "<<<<<<< local")
	assert.Contains(t, string(content), "line4-remote") // clean merge from remote
}

// TestSync_NewFileCreated verifies that sync creates files that exist
// in the registry but not locally.
func TestSync_NewFileCreated(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	registryDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(registryDir, "_defaults"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "_defaults", ".gitignore"),
		[]byte("*.o\n*.exe\n"), 0o644,
	))

	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "test-bp",
			Path: "test/bp",
		},
		Defaults: []lockfile.DefaultEntry{
			{Path: ".gitignore", Source: "registry-default", Strategy: "overwrite"},
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

	assert.Len(t, result.Updated, 1)
	assert.FileExists(t, filepath.Join(projectDir, ".gitignore"))
}
