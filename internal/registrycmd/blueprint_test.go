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

// scaffoldRegistry creates a minimal registry in a temp directory for testing.
func scaffoldRegistry(t *testing.T) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "test-registry")

	opts := &registrycmd.Opts{
		Path:    dir,
		Name:    "Test Registry",
		GitInit: false,
	}

	_, err := registrycmd.Run(opts)
	require.NoError(t, err)

	return dir
}

func TestRunBlueprint_BasicScaffold(t *testing.T) {
	t.Parallel()

	regDir := scaffoldRegistry(t)

	opts := &registrycmd.BlueprintOpts{
		RegistryDir: regDir,
		Category:    "go",
		Name:        "grpc-service",
	}

	result, err := registrycmd.RunBlueprint(opts)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify blueprint.yaml exists and is valid.
	bp, err := config.LoadBlueprint(result.BlueprintYAML)
	require.NoError(t, err)
	assert.Equal(t, "go-grpc-service", bp.Name)
	assert.Equal(t, "0.1.0", bp.Version)
	assert.Contains(t, bp.Tags, "go")
	assert.Len(t, bp.Variables, 2)
	assert.Equal(t, "project_name", bp.Variables[0].Name)

	// Verify starter template exists.
	tmplPath := filepath.Join(result.BlueprintDir, "{{project_name}}", "README.md.tmpl")
	assert.FileExists(t, tmplPath)

	tmplContent, err := os.ReadFile(tmplPath)
	require.NoError(t, err)
	assert.Contains(t, string(tmplContent), "{{ .project_name }}")

	// Verify category _defaults/.gitkeep exists.
	assert.FileExists(t, filepath.Join(regDir, "go", "_defaults", ".gitkeep"))

	// Verify registry.yaml updated with new entry.
	reg, err := config.LoadRegistry(filepath.Join(regDir, "registry.yaml"))
	require.NoError(t, err)

	var found bool
	for _, entry := range reg.Blueprints {
		if entry.Path == "go/grpc-service" {
			found = true
			assert.Equal(t, "go/grpc-service", entry.Name)
			assert.Equal(t, "0.1.0", entry.Version)
			assert.Contains(t, entry.Tags, "go")
		}
	}

	assert.True(t, found, "blueprint entry should be in registry.yaml")
}

func TestRunBlueprint_CustomTagsAndDescription(t *testing.T) {
	t.Parallel()

	regDir := scaffoldRegistry(t)

	opts := &registrycmd.BlueprintOpts{
		RegistryDir: regDir,
		Category:    "go",
		Name:        "api",
		Description: "A custom Go API blueprint",
		Tags:        []string{"go", "api", "http"},
	}

	result, err := registrycmd.RunBlueprint(opts)
	require.NoError(t, err)

	bp, err := config.LoadBlueprint(result.BlueprintYAML)
	require.NoError(t, err)
	assert.Equal(t, "A custom Go API blueprint", bp.Description)
	assert.Equal(t, []string{"go", "api", "http"}, bp.Tags)

	reg, err := config.LoadRegistry(filepath.Join(regDir, "registry.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, reg.Blueprints)

	entry := reg.Blueprints[0]
	assert.Equal(t, "A custom Go API blueprint", entry.Description)
	assert.Equal(t, []string{"go", "api", "http"}, entry.Tags)
}

func TestRunBlueprint_DuplicateGuard(t *testing.T) {
	t.Parallel()

	regDir := scaffoldRegistry(t)

	opts := &registrycmd.BlueprintOpts{
		RegistryDir: regDir,
		Category:    "go",
		Name:        "api",
	}

	_, err := registrycmd.RunBlueprint(opts)
	require.NoError(t, err)

	// Second call should fail.
	_, err = registrycmd.RunBlueprint(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestRunBlueprint_MissingRegistry(t *testing.T) {
	t.Parallel()

	opts := &registrycmd.BlueprintOpts{
		RegistryDir: filepath.Join(t.TempDir(), "nonexistent"),
		Category:    "go",
		Name:        "api",
	}

	_, err := registrycmd.RunBlueprint(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry.yaml not found")
}

func TestRunBlueprint_MissingCategoryOrName(t *testing.T) {
	t.Parallel()

	regDir := scaffoldRegistry(t)

	// Missing name.
	_, err := registrycmd.RunBlueprint(&registrycmd.BlueprintOpts{
		RegistryDir: regDir,
		Category:    "go",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both category and name are required")

	// Missing category.
	_, err = registrycmd.RunBlueprint(&registrycmd.BlueprintOpts{
		RegistryDir: regDir,
		Name:        "api",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both category and name are required")
}

func TestRunBlueprint_CategoryDefaultsAlreadyExist(t *testing.T) {
	t.Parallel()

	regDir := scaffoldRegistry(t)

	// Pre-create go/_defaults/ with a custom file.
	defaultsDir := filepath.Join(regDir, "go", "_defaults")
	require.NoError(t, os.MkdirAll(defaultsDir, 0o750))

	customFile := filepath.Join(defaultsDir, ".golangci.yml")
	require.NoError(t, os.WriteFile(customFile, []byte("linters: {}"), 0o644))

	opts := &registrycmd.BlueprintOpts{
		RegistryDir: regDir,
		Category:    "go",
		Name:        "api",
	}

	_, err := registrycmd.RunBlueprint(opts)
	require.NoError(t, err)

	// Custom file should still be there (idempotent).
	assert.FileExists(t, customFile)

	content, err := os.ReadFile(customFile)
	require.NoError(t, err)
	assert.Equal(t, "linters: {}", string(content))
}

func TestParseBlueprintPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantCat     string
		wantName    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid path",
			input:    "go/api",
			wantCat:  "go",
			wantName: "api",
		},
		{
			name:     "valid path with hyphens",
			input:    "go/grpc-service",
			wantCat:  "go",
			wantName: "grpc-service",
		},
		{
			name:        "empty string",
			input:       "",
			wantErr:     true,
			errContains: "blueprint path is required",
		},
		{
			name:        "no separator",
			input:       "go",
			wantErr:     true,
			errContains: "expected format category/name",
		},
		{
			name:        "too many segments",
			input:       "a/b/c",
			wantErr:     true,
			errContains: "expected format category/name",
		},
		{
			name:        "empty category",
			input:       "/api",
			wantErr:     true,
			errContains: "expected format category/name",
		},
		{
			name:        "empty name",
			input:       "go/",
			wantErr:     true,
			errContains: "expected format category/name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cat, name, err := registrycmd.ParseBlueprintPath(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantCat, cat)
			assert.Equal(t, tt.wantName, name)
		})
	}
}
