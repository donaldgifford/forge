package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
)

// TestLoadBlueprint_RejectsBareYAML covers the IMPL-0007 rejection
// contract: a .yaml path with no .hcl sibling returns the
// rescaffold-or-pin error (the in-tool `forge migrate` command was
// removed in IMPL-0007 per ADR-0002).
func TestLoadBlueprint_RejectsBareYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: test\n"), 0o600))

	_, err := config.LoadBlueprint(path)
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "YAML config files are no longer supported")
	assert.Contains(t, msg, "Rescaffold from the current blueprint")
	assert.Contains(t, msg, "go install github.com/donaldgifford/forge@v0.4.1")
	assert.Contains(t, msg, "docs/MIGRATION.md")
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
	assert.Contains(t, msg, "Rescaffold from the current blueprint")
	assert.Contains(t, msg, "go install github.com/donaldgifford/forge@v0.4.1")
	assert.Contains(t, msg, "docs/MIGRATION.md")
}

// TestLoadBlueprint_RejectsV1Fixture: even a v1-shaped blueprint.yaml
// hits the rescaffold-or-pin path now. Users coming from v0.2.x or
// earlier need to pin to v0.4.1, run `forge migrate templates` then
// `forge migrate config` against that binary, and then upgrade.
func TestLoadBlueprint_RejectsV1Fixture(t *testing.T) {
	t.Parallel()

	v1Path := filepath.Join("..", "..", "testdata", "v1-registry", "go", "api", "blueprint.yaml")

	_, err := config.LoadBlueprint(v1Path)
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "YAML config files are no longer supported")
	assert.Contains(t, msg, "go install github.com/donaldgifford/forge@v0.4.1")
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
