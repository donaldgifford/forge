package registrycmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/registrycmd"
)

func TestRun_BasicScaffold(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "my-registry")

	opts := &registrycmd.Opts{
		Path:        dir,
		Name:        "Test Registry",
		Description: "A test registry",
		GitInit:     false,
	}

	result, err := registrycmd.Run(opts)
	require.NoError(t, err)
	assert.Equal(t, dir, result.Dir)
	assert.False(t, result.GitInitialized)

	// Verify registry.yaml exists and is valid.
	regPath := filepath.Join(dir, "registry.yaml")
	assert.FileExists(t, regPath)

	reg, err := config.LoadRegistry(regPath)
	require.NoError(t, err)
	assert.Equal(t, "v1", reg.APIVersion)
	assert.Equal(t, "Test Registry", reg.Name)
	assert.Equal(t, "A test registry", reg.Description)
	assert.Empty(t, reg.Blueprints)

	// Verify _defaults directory and contents.
	assert.FileExists(t, filepath.Join(dir, "_defaults", ".editorconfig"))
	assert.FileExists(t, filepath.Join(dir, "_defaults", ".gitignore"))

	// Verify README.md.
	assert.FileExists(t, filepath.Join(dir, "README.md"))

	readmeContent, err := os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	assert.Contains(t, string(readmeContent), "Test Registry")
}

func TestRun_DerivedName(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "company-blueprints")

	opts := &registrycmd.Opts{
		Path:    dir,
		GitInit: false,
	}

	result, err := registrycmd.Run(opts)
	require.NoError(t, err)
	assert.Equal(t, dir, result.Dir)

	reg, err := config.LoadRegistry(filepath.Join(dir, "registry.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "company-blueprints", reg.Name)
}

func TestRun_WithCategories(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "my-registry")

	opts := &registrycmd.Opts{
		Path:       dir,
		Name:       "Test Registry",
		GitInit:    false,
		Categories: []string{"go", "python"},
	}

	_, err := registrycmd.Run(opts)
	require.NoError(t, err)

	// Verify category directories with _defaults/.gitkeep.
	assert.FileExists(t, filepath.Join(dir, "go", "_defaults", ".gitkeep"))
	assert.FileExists(t, filepath.Join(dir, "python", "_defaults", ".gitkeep"))
}

func TestRun_GuardExistingRegistry(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "existing-registry")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "registry.yaml"),
		[]byte("apiVersion: v1\nname: existing\n"),
		0o644,
	))

	opts := &registrycmd.Opts{
		Path:    dir,
		Name:    "New Registry",
		GitInit: false,
	}

	_, err := registrycmd.Run(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestRun_GitInit(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "git-registry")

	opts := &registrycmd.Opts{
		Path:    dir,
		Name:    "Git Registry",
		GitInit: true,
	}

	result, err := registrycmd.Run(opts)
	require.NoError(t, err)
	assert.True(t, result.GitInitialized)
	assert.DirExists(t, filepath.Join(dir, ".git"))
}

func TestRun_EmptyPath(t *testing.T) {
	t.Parallel()

	opts := &registrycmd.Opts{
		Path: "",
	}

	_, err := registrycmd.Run(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry path is required")
}
