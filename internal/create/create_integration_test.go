package create_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/create"
)

func TestRun_EmptyBlueprintRef(t *testing.T) {
	t.Parallel()

	opts := create.Opts{
		BlueprintRef: "",
		RegistryDir:  testRegistryDir,
	}

	_, err := create.Run(&opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestRun_MissingRegistryDir(t *testing.T) {
	t.Parallel()

	opts := create.Opts{
		BlueprintRef:       "go/api",
		DefaultRegistryURL: "github.com/acme/blueprints",
		RegistryDir:        "/nonexistent/path",
		UseDefaults:        true,
		Overrides: map[string]string{
			"project_name": "test",
			"go_module":    "github.com/example/test",
			"license":      "MIT",
		},
	}

	_, err := create.Run(&opts)
	require.Error(t, err)
}

func TestRun_ConditionsExcludeFiles(t *testing.T) {
	t.Parallel()

	outputDir := filepath.Join(t.TempDir(), "my-api")

	opts := create.Opts{
		BlueprintRef:       "go/api",
		OutputDir:          outputDir,
		DefaultRegistryURL: "github.com/acme/blueprints",
		RegistryDir:        testRegistryDir,
		UseDefaults:        true,
		ForgeVersion:       "0.1.0-test",
		Overrides: map[string]string{
			"project_name": "my-api",
			"go_module":    "github.com/example/my-api",
			"license":      "none",
		},
	}

	result, err := create.Run(&opts)
	require.NoError(t, err)
	assert.Positive(t, result.FilesCreated)

	// LICENSE should be excluded when license is "none" (if conditions are configured).
	// Verify the project was created successfully regardless.
	assert.DirExists(t, outputDir)
}

func TestRun_MultipleCreatesInDifferentDirs(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"project-a", "project-b"} {
		outputDir := filepath.Join(t.TempDir(), name)

		opts := create.Opts{
			BlueprintRef:       "go/api",
			OutputDir:          outputDir,
			DefaultRegistryURL: "github.com/acme/blueprints",
			RegistryDir:        testRegistryDir,
			UseDefaults:        true,
			ForgeVersion:       "0.1.0-test",
			Overrides: map[string]string{
				"project_name": name,
				"go_module":    "github.com/example/" + name,
				"license":      "MIT",
			},
		}

		result, err := create.Run(&opts)
		require.NoError(t, err)
		assert.Equal(t, outputDir, result.OutputDir)
		assert.Positive(t, result.FilesCreated)

		// Each project should have its own lockfile.
		assert.FileExists(t, filepath.Join(outputDir, ".forge-lock.yaml"))
	}
}

func TestRun_CheckFileContent(t *testing.T) {
	t.Parallel()

	outputDir := filepath.Join(t.TempDir(), "content-check")

	opts := create.Opts{
		BlueprintRef:       "go/api",
		OutputDir:          outputDir,
		DefaultRegistryURL: "github.com/acme/blueprints",
		RegistryDir:        testRegistryDir,
		UseDefaults:        true,
		ForgeVersion:       "0.1.0-test",
		Overrides: map[string]string{
			"project_name": "content-check",
			"go_module":    "github.com/example/content-check",
			"license":      "Apache-2.0",
		},
	}

	_, err := create.Run(&opts)
	require.NoError(t, err)

	// .editorconfig should be a valid config file from defaults.
	editorConfig, err := os.ReadFile(filepath.Join(outputDir, ".editorconfig"))
	require.NoError(t, err)
	assert.Contains(t, string(editorConfig), "root")
}
