package template_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tmpl "github.com/donaldgifford/forge/internal/template"
)

func TestRenderString(t *testing.T) {
	t.Parallel()

	r := tmpl.NewRenderer()

	result, err := r.RenderString("Hello {{ .name }}", map[string]any{"name": "world"})
	require.NoError(t, err)
	assert.Equal(t, "Hello world", result)
}

func TestRenderString_MissingKey(t *testing.T) {
	t.Parallel()

	r := tmpl.NewRenderer()

	_, err := r.RenderString("Hello {{ .missing }}", map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "executing template")
}

func TestRenderString_InvalidTemplate(t *testing.T) {
	t.Parallel()

	r := tmpl.NewRenderer()

	_, err := r.RenderString("{{ invalid", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing template")
}

func TestRenderFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "test.tmpl")
	require.NoError(t, os.WriteFile(tmplPath, []byte("Project: {{ .project_name }}"), 0o644))

	r := tmpl.NewRenderer()

	result, err := r.RenderFile(tmplPath, map[string]any{"project_name": "my-api"})
	require.NoError(t, err)
	assert.Equal(t, "Project: my-api", string(result))
}

func TestRenderFile_NotFound(t *testing.T) {
	t.Parallel()

	r := tmpl.NewRenderer()

	_, err := r.RenderFile("/nonexistent/file.tmpl", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading template")
}

func TestRenderPath(t *testing.T) {
	t.Parallel()

	r := tmpl.NewRenderer()

	tests := []struct {
		name     string
		path     string
		vars     map[string]any
		expected string
	}{
		{
			name:     "no template expressions",
			path:     "cmd/main.go",
			vars:     nil,
			expected: "cmd/main.go",
		},
		{
			name:     "project name in path",
			path:     "{{.project_name}}/cmd/main.go",
			vars:     map[string]any{"project_name": "my-api"},
			expected: "my-api/cmd/main.go",
		},
		{
			name:     "multiple expressions",
			path:     "{{.project_name}}/{{.module}}/main.go",
			vars:     map[string]any{"project_name": "my-api", "module": "cmd"},
			expected: "my-api/cmd/main.go",
		},
		{
			name:     "shorthand without dot",
			path:     "{{project_name}}/cmd/main.go",
			vars:     map[string]any{"project_name": "my-api"},
			expected: "my-api/cmd/main.go",
		},
		{
			name:     "mixed shorthand and dot notation",
			path:     "{{project_name}}/{{.module}}/main.go",
			vars:     map[string]any{"project_name": "my-api", "module": "cmd"},
			expected: "my-api/cmd/main.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := r.RenderPath(tt.path, tt.vars)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStripTemplateExtension(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "main.go", tmpl.StripTemplateExtension("main.go.tmpl"))
	assert.Equal(t, "main.go", tmpl.StripTemplateExtension("main.go"))
	assert.Equal(t, ".gitignore", tmpl.StripTemplateExtension(".gitignore.tmpl"))
}

func TestIsTemplate(t *testing.T) {
	t.Parallel()

	assert.True(t, tmpl.IsTemplate("main.go.tmpl"))
	assert.True(t, tmpl.IsTemplate(".gitignore.tmpl"))
	assert.False(t, tmpl.IsTemplate("main.go"))
	assert.False(t, tmpl.IsTemplate(".editorconfig"))
}
