package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
)

// TestLoadBlueprint_RejectsBareYAML covers the IMPL-0005 C.1 contract:
// a .yaml path with no .hcl sibling returns a migration-pointer error
// directing the user to `forge migrate config` and docs/MIGRATION.md.
// Doubles as the C.7 regression test.
func TestLoadBlueprint_RejectsBareYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: test\n"), 0o600))

	_, err := config.LoadBlueprint(path)
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "YAML config files are no longer supported")
	assert.Contains(t, msg, "forge migrate config")
	assert.Contains(t, msg, "docs/MIGRATION.md")
	assert.Contains(t, msg, "blueprint.hcl")
}

// TestLoadRegistry_RejectsBareYAML mirrors the blueprint case for
// registry.yaml.
func TestLoadRegistry_RejectsBareYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "registry.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: test\n"), 0o600))

	_, err := config.LoadRegistry(path)
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "YAML config files are no longer supported")
	assert.Contains(t, msg, "forge migrate config")
	assert.Contains(t, msg, "docs/MIGRATION.md")
	assert.Contains(t, msg, "registry.hcl")
}

// TestLoadBlueprint_RejectsV1Fixture: even a v1-shaped blueprint.yaml
// hits the YAML-rejection path now. Users coming from v0.2.x or
// earlier read MIGRATION.md to learn the two-step path
// (`forge migrate templates` then `forge migrate config`).
func TestLoadBlueprint_RejectsV1Fixture(t *testing.T) {
	t.Parallel()

	v1Path := filepath.Join("..", "..", "testdata", "v1-registry", "go", "api", "blueprint.yaml")

	_, err := config.LoadBlueprint(v1Path)
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "YAML config files are no longer supported")
	assert.Contains(t, msg, "forge migrate config")
	assert.Contains(t, msg, "docs/MIGRATION.md")
}

// TestLoadBlueprint_HCLFileNotFound: an .hcl path that doesn't exist
// surfaces a read error from the HCL loader.
func TestLoadBlueprint_HCLFileNotFound(t *testing.T) {
	t.Parallel()

	_, err := config.LoadBlueprint("/nonexistent/blueprint.hcl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading blueprint file")
}

// TestLoadRegistry_HCLFileNotFound mirrors the blueprint case.
func TestLoadRegistry_HCLFileNotFound(t *testing.T) {
	t.Parallel()

	_, err := config.LoadRegistry("/nonexistent/registry.hcl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading registry file")
}
