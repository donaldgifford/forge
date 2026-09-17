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

// writeVarsFile is a small helper used across the sync vars-file tests.
func writeVarsFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// setupVarsFileSyncTest builds a minimal project + registry pair: the
// registry has a `_defaults/greet.txt.tmpl` template that renders
// `Hello, ${name}!`. The project's initial lockfile records the
// pre-sync values (name = "world") and the matching default entry, so
// `sync` can re-render after a --var-file overlay.
func setupVarsFileSyncTest(t *testing.T) (projectDir, registryDir string) {
	t.Helper()

	projectDir = t.TempDir()
	registryDir = t.TempDir()

	// Registry: blueprint.hcl declares `name` as a string, and
	// _defaults/ ships a templated greeting.
	bpDir := filepath.Join(registryDir, "demo", "bp")
	require.NoError(t, os.MkdirAll(bpDir, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(bpDir, "blueprint.hcl"),
		[]byte(`name = "demo-bp"

variable "name" {
  type    = "string"
  default = "world"
}
`),
		0o600,
	))

	require.NoError(t, os.MkdirAll(filepath.Join(registryDir, "_defaults"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "_defaults", "greet.txt.tmpl"),
		[]byte("Hello, ${name}!\n"),
		0o600,
	))

	// Project: lockfile already records the pre-sync state.
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, "greet.txt"),
		[]byte("Hello, world!\n"),
		0o600,
	))

	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "demo-bp",
			Path: "demo/bp",
		},
		Variables: map[string]any{"name": "world"},
		Defaults: []lockfile.DefaultEntry{
			{Path: "greet.txt.tmpl", Source: "registry-default", Strategy: "overwrite"},
		},
	}
	require.NoError(t, lockfile.WriteHCL(
		filepath.Join(projectDir, lockfile.HCLFileName), lock,
	))

	return projectDir, registryDir
}

// TestSync_VarsFile_OverridesLockfileValue is the headline happy-path
// test: --var-file overlays a new value for `name`, sync re-renders
// the template, and the lockfile records the new value.
func TestSync_VarsFile_OverridesLockfileValue(t *testing.T) {
	t.Parallel()

	projectDir, registryDir := setupVarsFileSyncTest(t)
	tmp := t.TempDir()
	varsPath := writeVarsFile(t, tmp, "v.forge-vars.hcl", `name = "forge"`+"\n")

	opts := &forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
		VarsFiles:   []string{varsPath},
	}

	result, err := forgesync.Run(opts)
	require.NoError(t, err)
	assert.Empty(t, result.UnknownVarsFileKeys)

	// Rendered file updated.
	content, err := os.ReadFile(filepath.Join(projectDir, "greet.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Hello, forge!\n", string(content))

	// Lockfile records the new value.
	lock, err := lockfile.LoadLockfile(projectDir)
	require.NoError(t, err)
	assert.Equal(t, "forge", lock.Variables["name"])
}

// TestSync_VarsFile_DryRun_DoesNotWrite verifies that --dry-run
// suppresses both the rendered output and the lockfile rewrite even
// when a --var-file is supplied.
func TestSync_VarsFile_DryRun_DoesNotWrite(t *testing.T) {
	t.Parallel()

	projectDir, registryDir := setupVarsFileSyncTest(t)
	tmp := t.TempDir()
	varsPath := writeVarsFile(t, tmp, "v.forge-vars.hcl", `name = "forge"`+"\n")

	opts := &forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
		VarsFiles:   []string{varsPath},
		DryRun:      true,
	}

	_, err := forgesync.Run(opts)
	require.NoError(t, err)

	// File unchanged.
	content, err := os.ReadFile(filepath.Join(projectDir, "greet.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!\n", string(content))

	// Lockfile still records the original value.
	lock, err := lockfile.LoadLockfile(projectDir)
	require.NoError(t, err)
	assert.Equal(t, "world", lock.Variables["name"])
}

// TestSync_VarsFile_UnknownKey_Warned verifies that a --var-file with
// a key not declared in the blueprint surfaces a warning on
// Result.UnknownVarsFileKeys but does not error the sync.
func TestSync_VarsFile_UnknownKey_Warned(t *testing.T) {
	t.Parallel()

	projectDir, registryDir := setupVarsFileSyncTest(t)
	tmp := t.TempDir()
	varsPath := writeVarsFile(t, tmp, "extra.forge-vars.hcl",
		"name = \"forge\"\nstray_key = \"ignored\"\n")

	opts := &forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
		VarsFiles:   []string{varsPath},
	}

	result, err := forgesync.Run(opts)
	require.NoError(t, err)
	assert.Contains(t, result.UnknownVarsFileKeys, "stray_key")

	// Unknown key NOT persisted to the lockfile.
	lock, err := lockfile.LoadLockfile(projectDir)
	require.NoError(t, err)
	_, hasStray := lock.Variables["stray_key"]
	assert.False(t, hasStray)
}

// TestSync_VarsFile_TypeMismatch_Aborts verifies that a coercion
// failure short-circuits the sync before any files are written or
// the lockfile is rewritten.
func TestSync_VarsFile_TypeMismatch_Aborts(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	registryDir := t.TempDir()

	// Blueprint declares `count` as int.
	bpDir := filepath.Join(registryDir, "demo", "bp")
	require.NoError(t, os.MkdirAll(bpDir, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(bpDir, "blueprint.hcl"),
		[]byte(`name = "demo-bp"

variable "count" {
  type    = "int"
  default = "1"
}
`),
		0o600,
	))

	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			Name: "demo-bp",
			Path: "demo/bp",
		},
		Variables: map[string]any{"count": int64(1)},
	}
	lockPath := filepath.Join(projectDir, lockfile.HCLFileName)
	require.NoError(t, lockfile.WriteHCL(lockPath, lock))

	preSync, err := os.ReadFile(lockPath)
	require.NoError(t, err)

	tmp := t.TempDir()
	varsPath := writeVarsFile(t, tmp, "bad.forge-vars.hcl", `count = "not-a-number"`+"\n")

	_, err = forgesync.Run(&forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
		VarsFiles:   []string{varsPath},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "count")

	// Lockfile unchanged.
	postSync, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, preSync, postSync)
}

// TestSync_NoVarsFile_LockfileUnchangedWhenIdle is a regression guard:
// without --var-file and without registry drift, sync must NOT touch
// the lockfile (timestamps included). This keeps git diffs quiet for
// the common "ran sync, nothing changed" case.
func TestSync_NoVarsFile_LockfileUnchangedWhenIdle(t *testing.T) {
	t.Parallel()

	projectDir, registryDir := setupVarsFileSyncTest(t)
	lockPath := filepath.Join(projectDir, lockfile.HCLFileName)

	preSync, err := os.ReadFile(lockPath)
	require.NoError(t, err)

	_, err = forgesync.Run(&forgesync.Opts{
		ProjectDir:  projectDir,
		RegistryDir: registryDir,
	})
	require.NoError(t, err)

	postSync, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, preSync, postSync, "lockfile should be byte-identical when nothing changed")
}
