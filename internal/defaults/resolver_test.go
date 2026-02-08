package defaults_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/defaults"
)

const testRegistryRoot = "../../testdata/registry"

func TestResolve_InheritsRootDefaults(t *testing.T) {
	t.Parallel()

	fs, err := defaults.Resolve(testRegistryRoot, "go/api", nil)
	require.NoError(t, err)

	// .editorconfig comes from root _defaults/
	entry := fs.Get(".editorconfig")
	require.NotNil(t, entry, "expected .editorconfig from root defaults")
	assert.Equal(t, defaults.LayerRegistryDefault, entry.SourceLayer)
	assert.False(t, entry.IsTemplate)
}

func TestResolve_InheritsRootTemplates(t *testing.T) {
	t.Parallel()

	fs, err := defaults.Resolve(testRegistryRoot, "go/api", nil)
	require.NoError(t, err)

	// LICENSE.tmpl comes from root _defaults/
	entry := fs.Get("LICENSE.tmpl")
	require.NotNil(t, entry, "expected LICENSE.tmpl from root defaults")
	assert.Equal(t, defaults.LayerRegistryDefault, entry.SourceLayer)
	assert.True(t, entry.IsTemplate)
}

func TestResolve_CategoryOverridesRoot(t *testing.T) {
	t.Parallel()

	fs, err := defaults.Resolve(testRegistryRoot, "go/api", nil)
	require.NoError(t, err)

	// scripts/lint.sh exists in both root _defaults/ and go/_defaults/
	// The category default should win.
	entry := fs.Get(filepath.Join("scripts", "lint.sh"))
	require.NotNil(t, entry, "expected scripts/lint.sh")
	assert.Equal(t, defaults.LayerCategoryDefault, entry.SourceLayer)
	assert.Contains(t, entry.AbsPath, filepath.Join("go", "_defaults"))
}

func TestResolve_CategoryAddsNewFiles(t *testing.T) {
	t.Parallel()

	fs, err := defaults.Resolve(testRegistryRoot, "go/api", nil)
	require.NoError(t, err)

	// .golangci.yml comes from go/_defaults/ (not in root)
	entry := fs.Get(".golangci.yml")
	require.NotNil(t, entry, "expected .golangci.yml from go/_defaults/")
	assert.Equal(t, defaults.LayerCategoryDefault, entry.SourceLayer)
}

func TestResolve_BlueprintFiles(t *testing.T) {
	t.Parallel()

	fs, err := defaults.Resolve(testRegistryRoot, "go/api", nil)
	require.NoError(t, err)

	// Blueprint-specific template files
	entry := fs.Get(filepath.Join("{{project_name}}", "cmd", "main.go.tmpl"))
	require.NotNil(t, entry, "expected blueprint template file")
	assert.Equal(t, defaults.LayerBlueprint, entry.SourceLayer)
	assert.True(t, entry.IsTemplate)
}

func TestResolve_ExcludesBluerintYAML(t *testing.T) {
	t.Parallel()

	fs, err := defaults.Resolve(testRegistryRoot, "go/api", nil)
	require.NoError(t, err)

	// blueprint.yaml should not be in the output file set
	entry := fs.Get("blueprint.yaml")
	assert.Nil(t, entry, "blueprint.yaml should be excluded from file set")
}

func TestResolve_AppliesExclusions(t *testing.T) {
	t.Parallel()

	exclusions := []string{".editorconfig"}

	fs, err := defaults.Resolve(testRegistryRoot, "go/api", exclusions)
	require.NoError(t, err)

	entry := fs.Get(".editorconfig")
	assert.Nil(t, entry, ".editorconfig should be excluded")

	// Other files should still be present
	assert.Positive(t, fs.Len())
}

func TestResolve_NonexistentRegistry(t *testing.T) {
	t.Parallel()

	fs, err := defaults.Resolve("/nonexistent/registry", "go/api", nil)
	require.NoError(t, err)

	// No files found, but no error (directories just don't exist)
	assert.Equal(t, 0, fs.Len())
}

func TestResolve_FileCount(t *testing.T) {
	t.Parallel()

	fs, err := defaults.Resolve(testRegistryRoot, "go/api", nil)
	require.NoError(t, err)

	// Expected files:
	// Root defaults: .editorconfig, .gitignore.tmpl, LICENSE.tmpl, scripts/lint.sh (overridden)
	// Category defaults: .golangci.yml, scripts/lint.sh (overrides root)
	// Blueprint: {{project_name}}/cmd/main.go.tmpl, {{project_name}}/go.mod.tmpl, {{project_name}}/README.md.tmpl
	// Total unique: .editorconfig, .gitignore.tmpl, LICENSE.tmpl, scripts/lint.sh, .golangci.yml,
	//              {{project_name}}/cmd/main.go.tmpl, {{project_name}}/go.mod.tmpl, {{project_name}}/README.md.tmpl
	assert.Equal(t, 8, fs.Len())
}

func TestFileSet_Operations(t *testing.T) {
	t.Parallel()

	fs := defaults.NewFileSet()
	assert.Equal(t, 0, fs.Len())

	fs.Add(&defaults.FileEntry{
		AbsPath:     "/tmp/test.txt",
		RelPath:     "test.txt",
		SourceLayer: defaults.LayerRegistryDefault,
	})
	assert.Equal(t, 1, fs.Len())

	entry := fs.Get("test.txt")
	require.NotNil(t, entry)
	assert.Equal(t, "/tmp/test.txt", entry.AbsPath)

	// Replace with higher-priority entry
	fs.Add(&defaults.FileEntry{
		AbsPath:     "/tmp/override/test.txt",
		RelPath:     "test.txt",
		SourceLayer: defaults.LayerCategoryDefault,
	})
	assert.Equal(t, 1, fs.Len())

	entry = fs.Get("test.txt")
	assert.Equal(t, "/tmp/override/test.txt", entry.AbsPath)

	fs.Remove("test.txt")
	assert.Equal(t, 0, fs.Len())
	assert.Nil(t, fs.Get("test.txt"))
}

func TestSourceLayer_String(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "registry-default", defaults.LayerRegistryDefault.String())
	assert.Equal(t, "category-default", defaults.LayerCategoryDefault.String())
	assert.Equal(t, "blueprint", defaults.LayerBlueprint.String())
}
