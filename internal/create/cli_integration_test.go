package create_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/create"
	"github.com/donaldgifford/forge/internal/lockfile"
)

// absTestRegistryDir returns the absolute path to the test registry, matching
// how the CLI resolves --registry-dir.
func absTestRegistryDir(t *testing.T) string {
	t.Helper()

	abs, err := filepath.Abs(testRegistryDir)
	require.NoError(t, err)

	return abs
}

// newCLIOpts builds create.Opts that mirror what cmd/create.go would produce
// from CLI flags.
func newCLIOpts(t *testing.T, outputDir string) *create.Opts {
	t.Helper()

	return &create.Opts{
		BlueprintRef: "go/api",
		OutputDir:    outputDir,
		RegistryDir:  absTestRegistryDir(t),
		UseDefaults:  true,
		NoHooks:      true,
		ForceCreate:  false,
		ForgeVersion: "test-cli",
		Overrides: map[string]string{
			"project_name": "my-test-api",
			"go_module":    "github.com/example/my-test-api",
			"use_grpc":     "false",
			"license":      "MIT",
		},
	}
}

func TestCLI_CreateEndToEnd(t *testing.T) {
	t.Parallel()

	outputDir := filepath.Join(t.TempDir(), "my-test-api")
	opts := newCLIOpts(t, outputDir)

	result, err := create.Run(opts)
	require.NoError(t, err)
	assert.Equal(t, outputDir, result.OutputDir)
	assert.Equal(t, "go-api", result.Blueprint)
	assert.Positive(t, result.FilesCreated)

	// Verify rendered files exist with correct content.
	mainGo, err := os.ReadFile(filepath.Join(outputDir, "cmd", "main.go"))
	require.NoError(t, err)
	assert.Contains(t, string(mainGo), "package main")
	assert.Contains(t, string(mainGo), "my-test-api")

	goMod, err := os.ReadFile(filepath.Join(outputDir, "go.mod"))
	require.NoError(t, err)
	assert.Contains(t, string(goMod), "github.com/example/my-test-api")

	readme, err := os.ReadFile(filepath.Join(outputDir, "README.md"))
	require.NoError(t, err)
	assert.Contains(t, string(readme), "my-test-api")

	// Verify defaults inheritance.
	assert.FileExists(t, filepath.Join(outputDir, ".editorconfig"))
	assert.FileExists(t, filepath.Join(outputDir, ".golangci.yml"))
	assert.FileExists(t, filepath.Join(outputDir, "scripts", "lint.sh"))
	assert.FileExists(t, filepath.Join(outputDir, ".gitignore"))
	assert.FileExists(t, filepath.Join(outputDir, "LICENSE"))

	// Verify scripts/lint.sh is from go/_defaults/ (overrides root).
	lintSh, err := os.ReadFile(filepath.Join(outputDir, "scripts", "lint.sh"))
	require.NoError(t, err)
	assert.Contains(t, string(lintSh), "golangci-lint")

	// Verify excluded file is absent.
	assert.NoFileExists(t, filepath.Join(outputDir, ".pre-commit-config.yaml"))

	// Verify no .tmpl extensions in output.
	err = filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			assert.False(t, strings.HasSuffix(path, ".tmpl"),
				"output file should not have .tmpl extension: %s", path)
		}

		return nil
	})
	require.NoError(t, err)

	// Verify lockfile is valid and parseable.
	lockPath := filepath.Join(outputDir, lockfile.FileName)
	assert.FileExists(t, lockPath)

	lock, err := lockfile.Read(lockPath)
	require.NoError(t, err)
	assert.Equal(t, "go-api", lock.Blueprint.Name)
	assert.Equal(t, "go/api", lock.Blueprint.Path)
	assert.Equal(t, "my-test-api", lock.Variables["project_name"])
	assert.NotEmpty(t, lock.Defaults)
	assert.NotEmpty(t, lock.ManagedFiles)
}

func TestCLI_ForceGuard_RejectsNonEmptyDir(t *testing.T) {
	t.Parallel()

	outputDir := t.TempDir()

	// Create a file to make the directory non-empty.
	require.NoError(t, os.WriteFile(filepath.Join(outputDir, "existing.txt"), []byte("data"), 0o644))

	opts := newCLIOpts(t, outputDir)
	opts.ForceCreate = false

	_, err := create.Run(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not empty")
	assert.Contains(t, err.Error(), "--force")
}

func TestCLI_ForceGuard_AllowsWithForce(t *testing.T) {
	t.Parallel()

	outputDir := t.TempDir()

	// Create a file to make the directory non-empty.
	require.NoError(t, os.WriteFile(filepath.Join(outputDir, "existing.txt"), []byte("data"), 0o644))

	opts := newCLIOpts(t, outputDir)
	opts.ForceCreate = true

	result, err := create.Run(opts)
	require.NoError(t, err)
	assert.Positive(t, result.FilesCreated)

	// New files should be created alongside existing ones.
	assert.FileExists(t, filepath.Join(outputDir, "cmd", "main.go"))
	assert.FileExists(t, filepath.Join(outputDir, "existing.txt"))
}

func TestCLI_ForceGuard_AllowsEmptyDir(t *testing.T) {
	t.Parallel()

	outputDir := filepath.Join(t.TempDir(), "empty-new")
	opts := newCLIOpts(t, outputDir)
	opts.ForceCreate = false

	// Should succeed because directory doesn't exist yet.
	result, err := create.Run(opts)
	require.NoError(t, err)
	assert.Positive(t, result.FilesCreated)
}
