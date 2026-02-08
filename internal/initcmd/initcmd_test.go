package initcmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/initcmd"
)

func TestRun_Standalone(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "my-blueprint")

	opts := &initcmd.Opts{
		Path: dir,
	}

	bpPath, err := initcmd.Run(opts)
	require.NoError(t, err)

	assert.FileExists(t, bpPath)

	// Verify the generated blueprint is valid YAML.
	bp, err := config.LoadBlueprint(bpPath)
	require.NoError(t, err)
	assert.Equal(t, "v1", bp.APIVersion)
	assert.NotEmpty(t, bp.Name)
	assert.Len(t, bp.Variables, 1)
	assert.Equal(t, "project_name", bp.Variables[0].Name)
}

func TestRun_StandaloneDuplicate(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "my-blueprint")

	opts := &initcmd.Opts{
		Path: dir,
	}

	_, err := initcmd.Run(opts)
	require.NoError(t, err)

	// Second call should fail.
	_, err = initcmd.Run(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestRun_RegistryMode(t *testing.T) {
	t.Parallel()

	registryDir := t.TempDir()

	opts := &initcmd.Opts{
		Path:        "go/grpc-gateway",
		RegistryDir: registryDir,
	}

	bpPath, err := initcmd.Run(opts)
	require.NoError(t, err)
	assert.FileExists(t, bpPath)

	// Verify registry.yaml was created/updated.
	indexPath := filepath.Join(registryDir, "registry.yaml")
	assert.FileExists(t, indexPath)

	reg, err := config.LoadRegistry(indexPath)
	require.NoError(t, err)
	require.Len(t, reg.Blueprints, 1)
	assert.Equal(t, "go/grpc-gateway", reg.Blueprints[0].Path)
	assert.Equal(t, "go-grpc-gateway", reg.Blueprints[0].Name)
}

func TestRun_RegistryModeExistingIndex(t *testing.T) {
	t.Parallel()

	registryDir := t.TempDir()

	// Create an existing registry.yaml.
	indexContent := `apiVersion: v1
name: "test-registry"
blueprints:
  - name: existing
    path: existing/bp
    version: "1.0.0"
`
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "registry.yaml"),
		[]byte(indexContent),
		0o644,
	))

	opts := &initcmd.Opts{
		Path:        "go/new-bp",
		RegistryDir: registryDir,
	}

	_, err := initcmd.Run(opts)
	require.NoError(t, err)

	reg, err := config.LoadRegistry(filepath.Join(registryDir, "registry.yaml"))
	require.NoError(t, err)
	assert.Len(t, reg.Blueprints, 2)
}

func TestRun_RegistryModeNoBlueprintPath(t *testing.T) {
	t.Parallel()

	opts := &initcmd.Opts{
		RegistryDir: t.TempDir(),
	}

	_, err := initcmd.Run(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blueprint path is required")
}
