package lockfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/lockfile"
)

const fixturePath = "../../testdata/lockfile-hcl/.forge-lock.hcl"

func TestLoadLockfileHCL_HappyPath(t *testing.T) {
	t.Parallel()

	lock, err := lockfile.LoadLockfileHCL(fixturePath)
	require.NoError(t, err)

	assert.Equal(t, "github.com/donaldgifford/forge-registry", lock.Blueprint.RegistryURL)
	assert.Equal(t, "go-api", lock.Blueprint.Name)
	assert.Equal(t, "go/api", lock.Blueprint.Path)
	assert.Equal(t, "main", lock.Blueprint.Ref)
	assert.Equal(t, "abc123def456", lock.Blueprint.Commit)

	assert.Equal(t, "0.5.0", lock.ForgeVersion)
	assert.False(t, lock.CreatedAt.IsZero())
	assert.False(t, lock.LastSynced.IsZero())

	assert.Equal(t, "mockta", lock.Variables["project_name"])
	assert.Equal(t, "A lightweight, embeddable Okta mock", lock.Variables["project_description"])
	assert.Equal(t, true, lock.Variables["use_docker"])
	assert.Equal(t, int64(3), lock.Variables["max_replicas"])

	require.Len(t, lock.Defaults, 2)
	assert.Equal(t, "Makefile", lock.Defaults[0].Path)
	assert.Equal(t, "overwrite", lock.Defaults[0].Strategy)
	assert.Equal(t, "sha256:cafe0001", lock.Defaults[0].Hash)
	assert.Equal(t, "abc123def456", lock.Defaults[0].SyncedCommit)
	assert.Equal(t, ".github/workflows/ci.yml", lock.Defaults[1].Path)
	assert.Empty(t, lock.Defaults[1].Hash)

	require.Len(t, lock.ManagedFiles, 2)
	assert.Equal(t, "README.md", lock.ManagedFiles[0].Path)
	assert.Equal(t, "sha256:cafe0002", lock.ManagedFiles[0].Hash)
	assert.Equal(t, "config.yaml", lock.ManagedFiles[1].Path)
}

func TestLoadLockfileHCL_NotFound(t *testing.T) {
	t.Parallel()

	_, err := lockfile.LoadLockfileHCL("/nonexistent/.forge-lock.hcl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading lockfile")
}

func TestLoadLockfileHCL_MalformedHCL(t *testing.T) {
	t.Parallel()

	path := writeTempHCL(t, "this is not { valid hcl =")

	_, err := lockfile.LoadLockfileHCL(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing lockfile")
}

func TestLoadLockfileHCL_MissingRequiredAttribute(t *testing.T) {
	t.Parallel()

	// Missing `forge_version` — required by lockfileEagerSpec.
	src := `
blueprint {
  registry_url = "git"
  name         = "n"
  path         = "p"
}
created_at  = "2026-05-18T10:15:30Z"
last_synced = "2026-05-18T10:15:30Z"
`
	path := writeTempHCL(t, src)

	_, err := lockfile.LoadLockfileHCL(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding lockfile attributes")
}

func TestLoadLockfileHCL_MissingBlueprintBlock(t *testing.T) {
	t.Parallel()

	src := `
created_at    = "2026-05-18T10:15:30Z"
last_synced   = "2026-05-18T10:15:30Z"
forge_version = "0.5.0"
`
	path := writeTempHCL(t, src)

	_, err := lockfile.LoadLockfileHCL(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding lockfile attributes")
}

func TestLoadLockfileHCL_InvalidTimestamp(t *testing.T) {
	t.Parallel()

	src := `
blueprint {
  registry_url = "git"
  name         = "n"
  path         = "p"
}
created_at    = "not-a-timestamp"
last_synced   = "2026-05-18T10:15:30Z"
forge_version = "0.5.0"
`
	path := writeTempHCL(t, src)

	_, err := lockfile.LoadLockfileHCL(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timestamp")
}

func TestLoadLockfileHCL_MultipleVariablesBlocks(t *testing.T) {
	t.Parallel()

	src := `
blueprint {
  registry_url = "git"
  name         = "n"
  path         = "p"
}
created_at    = "2026-05-18T10:15:30Z"
last_synced   = "2026-05-18T10:15:30Z"
forge_version = "0.5.0"

variables {
  a = "x"
}

variables {
  b = "y"
}
`
	path := writeTempHCL(t, src)

	_, err := lockfile.LoadLockfileHCL(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "variables")
	assert.Contains(t, err.Error(), "expected at most one")
}

func TestLoadLockfileHCL_NoVariablesBlock(t *testing.T) {
	t.Parallel()

	src := `
blueprint {
  registry_url = "git"
  name         = "n"
  path         = "p"
}
created_at    = "2026-05-18T10:15:30Z"
last_synced   = "2026-05-18T10:15:30Z"
forge_version = "0.5.0"
`
	path := writeTempHCL(t, src)

	lock, err := lockfile.LoadLockfileHCL(path)
	require.NoError(t, err)
	assert.NotNil(t, lock.Variables)
	assert.Empty(t, lock.Variables)
}

func TestLoadLockfile_PrefersHCLOverYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// YAML lockfile to fall back to.
	yamlData := `forge_version: "0.4.0"
blueprint:
  registry_url: r
  name: y
  path: p
`
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, lockfile.FileName), []byte(yamlData), 0o644,
	))

	hclSrc := `
blueprint {
  registry_url = "r"
  name         = "h"
  path         = "p"
}
created_at    = "2026-05-18T10:15:30Z"
last_synced   = "2026-05-18T10:15:30Z"
forge_version = "0.5.0"
`
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, lockfile.HCLFileName), []byte(hclSrc), 0o644,
	))

	lock, err := lockfile.LoadLockfile(dir)
	require.NoError(t, err)
	assert.Equal(t, "h", lock.Blueprint.Name)
	assert.Equal(t, "0.5.0", lock.ForgeVersion)
}

func TestLoadLockfile_RejectsYAMLOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	yamlData := `forge_version: "0.4.0"
blueprint:
  registry_url: r
  name: y
  path: p
`
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, lockfile.FileName), []byte(yamlData), 0o644,
	))

	_, err := lockfile.LoadLockfile(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "YAML lockfiles are no longer supported")
	assert.Contains(t, err.Error(), "rescaffold")
	assert.Contains(t, err.Error(), "pin forge to v0.4.x")
	assert.Contains(t, err.Error(), "docs/MIGRATION.md")
}

func TestLoadLockfile_NeitherFormatPresent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_, err := lockfile.LoadLockfile(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no lockfile found")
}

func writeTempHCL(t *testing.T, src string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, lockfile.HCLFileName)

	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	return path
}
