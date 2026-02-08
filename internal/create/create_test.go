package create_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/create"
	"github.com/donaldgifford/forge/internal/lockfile"
)

const testRegistryDir = "../../testdata/registry"

func TestRun_EndToEnd(t *testing.T) {
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
			"license":      "MIT",
		},
	}

	result, err := create.Run(&opts)
	require.NoError(t, err)

	assert.Equal(t, outputDir, result.OutputDir)
	assert.Positive(t, result.FilesCreated)
	assert.Equal(t, "go-api", result.Blueprint)
}

func TestRun_TemplatesRendered(t *testing.T) {
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
			"license":      "MIT",
		},
	}

	_, err := create.Run(&opts)
	require.NoError(t, err)

	// Check that template variables are substituted.
	goModPath := filepath.Join(outputDir, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		content, err := os.ReadFile(goModPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "github.com/example/my-api")
	}

	// Check LICENSE was rendered.
	licensePath := filepath.Join(outputDir, "LICENSE")
	if _, err := os.Stat(licensePath); err == nil {
		content, err := os.ReadFile(licensePath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "MIT")
	}
}

func TestRun_DefaultsInherited(t *testing.T) {
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
			"license":      "MIT",
		},
	}

	_, err := create.Run(&opts)
	require.NoError(t, err)

	// Root default: .editorconfig should exist.
	assert.FileExists(t, filepath.Join(outputDir, ".editorconfig"))

	// Category default: .golangci.yml should exist.
	assert.FileExists(t, filepath.Join(outputDir, ".golangci.yml"))

	// Category override: scripts/lint.sh should have Go-specific content.
	lintPath := filepath.Join(outputDir, "scripts", "lint.sh")
	if assert.FileExists(t, lintPath) {
		content, err := os.ReadFile(lintPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "golangci-lint")
	}
}

func TestRun_LockfileGenerated(t *testing.T) {
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
			"license":      "MIT",
		},
	}

	_, err := create.Run(&opts)
	require.NoError(t, err)

	lockPath := filepath.Join(outputDir, lockfile.FileName)
	assert.FileExists(t, lockPath)

	lock, err := lockfile.Read(lockPath)
	require.NoError(t, err)

	assert.Equal(t, "go-api", lock.Blueprint.Name)
	assert.Equal(t, "go/api", lock.Blueprint.Path)
	assert.Equal(t, "0.1.0-test", lock.ForgeVersion)
	assert.Equal(t, "my-api", lock.Variables["project_name"])
	assert.NotEmpty(t, lock.Defaults)
	assert.Len(t, lock.ManagedFiles, 1)
	assert.Equal(t, "Makefile", lock.ManagedFiles[0].Path)
}

func TestRun_TmplExtensionsStripped(t *testing.T) {
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
			"license":      "MIT",
		},
	}

	_, err := create.Run(&opts)
	require.NoError(t, err)

	// .tmpl files should not appear in output.
	var tmplFiles []string

	_ = filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".tmpl" {
			tmplFiles = append(tmplFiles, path)
		}

		return nil
	})

	assert.Empty(t, tmplFiles, "no .tmpl files should exist in output")

	// But the rendered files should exist (without .tmpl).
	assert.FileExists(t, filepath.Join(outputDir, "LICENSE"))
}

func TestRun_OutputDirDerivedFromProjectName(t *testing.T) {
	absRegistryDir, err := filepath.Abs(testRegistryDir)
	require.NoError(t, err)

	parentDir := t.TempDir()
	t.Chdir(parentDir)

	opts := create.Opts{
		BlueprintRef:       "go/api",
		DefaultRegistryURL: "github.com/acme/blueprints",
		RegistryDir:        absRegistryDir,
		UseDefaults:        true,
		ForgeVersion:       "0.1.0-test",
		Overrides: map[string]string{
			"project_name": "derived-project",
			"go_module":    "github.com/example/derived-project",
			"license":      "MIT",
		},
	}

	result, err := create.Run(&opts)
	require.NoError(t, err)

	assert.Equal(t, "derived-project", result.OutputDir)
	assert.DirExists(t, filepath.Join(parentDir, "derived-project"))
}

func TestRun_InvalidBlueprint(t *testing.T) {
	t.Parallel()

	opts := create.Opts{
		BlueprintRef:       "nonexistent/blueprint",
		DefaultRegistryURL: "github.com/acme/blueprints",
		RegistryDir:        testRegistryDir,
	}

	_, err := create.Run(&opts)
	require.Error(t, err)
}
