package template_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tmpl "github.com/donaldgifford/forge/internal/template"
)

func TestFuncMap_SnakeCase(t *testing.T) {
	t.Parallel()

	r := tmpl.NewRenderer()

	tests := []struct {
		input    string
		expected string
	}{
		{"MyProject", "my_project"},
		{"myProject", "my_project"},
		{"my-project", "my_project"},
		{"my project", "my_project"},
		{"already_snake", "already_snake"},
		{"HTTPServer", "h_t_t_p_server"},
	}

	for _, tt := range tests {
		result, err := r.RenderString(`{{ snakeCase "`+tt.input+`" }}`, nil)
		require.NoError(t, err)
		assert.Equal(t, tt.expected, result, "snakeCase(%q)", tt.input)
	}
}

func TestFuncMap_CamelCase(t *testing.T) {
	t.Parallel()

	r := tmpl.NewRenderer()

	tests := []struct {
		input    string
		expected string
	}{
		{"my-project", "myProject"},
		{"my_project", "myProject"},
		{"my project", "myProject"},
		{"MyProject", "myProject"},
	}

	for _, tt := range tests {
		result, err := r.RenderString(`{{ camelCase "`+tt.input+`" }}`, nil)
		require.NoError(t, err)
		assert.Equal(t, tt.expected, result, "camelCase(%q)", tt.input)
	}
}

func TestFuncMap_PascalCase(t *testing.T) {
	t.Parallel()

	r := tmpl.NewRenderer()

	tests := []struct {
		input    string
		expected string
	}{
		{"my-project", "MyProject"},
		{"my_project", "MyProject"},
		{"myProject", "MyProject"},
	}

	for _, tt := range tests {
		result, err := r.RenderString(`{{ pascalCase "`+tt.input+`" }}`, nil)
		require.NoError(t, err)
		assert.Equal(t, tt.expected, result, "pascalCase(%q)", tt.input)
	}
}

func TestFuncMap_KebabCase(t *testing.T) {
	t.Parallel()

	r := tmpl.NewRenderer()

	tests := []struct {
		input    string
		expected string
	}{
		{"MyProject", "my-project"},
		{"my_project", "my-project"},
		{"myProject", "my-project"},
	}

	for _, tt := range tests {
		result, err := r.RenderString(`{{ kebabCase "`+tt.input+`" }}`, nil)
		require.NoError(t, err)
		assert.Equal(t, tt.expected, result, "kebabCase(%q)", tt.input)
	}
}

func TestFuncMap_StringFunctions(t *testing.T) {
	t.Parallel()

	r := tmpl.NewRenderer()

	tests := []struct {
		tmplStr  string
		expected string
	}{
		{`{{ upper "hello" }}`, "HELLO"},
		{`{{ lower "HELLO" }}`, "hello"},
		{`{{ "foo-bar" | replace "-" "_" }}`, "foo_bar"},
		{`{{ "v1.2.3" | trimPrefix "v" }}`, "1.2.3"},
		{`{{ "file.tmpl" | trimSuffix ".tmpl" }}`, "file"},
	}

	for _, tt := range tests {
		result, err := r.RenderString(tt.tmplStr, nil)
		require.NoError(t, err)
		assert.Equal(t, tt.expected, result, "template: %s", tt.tmplStr)
	}
}

func TestFuncMap_Default(t *testing.T) {
	t.Parallel()

	r := tmpl.NewRenderer()

	result, err := r.RenderString(`{{ .desc | default "fallback" }}`, map[string]any{"desc": ""})
	require.NoError(t, err)
	assert.Equal(t, "fallback", result)

	result, err = r.RenderString(`{{ .desc | default "fallback" }}`, map[string]any{"desc": "actual"})
	require.NoError(t, err)
	assert.Equal(t, "actual", result)
}

func TestFuncMap_Env(t *testing.T) {
	r := tmpl.NewRenderer()

	t.Setenv("FORGE_TEST_VAR", "test_value")

	result, err := r.RenderString(`{{ env "FORGE_TEST_VAR" }}`, nil)
	require.NoError(t, err)
	assert.Equal(t, "test_value", result)
}

func TestFuncMap_Now(t *testing.T) {
	t.Parallel()

	r := tmpl.NewRenderer()

	result, err := r.RenderString(`{{ now "2006" }}`, nil)
	require.NoError(t, err)
	assert.Len(t, result, 4) // Year is 4 digits.
}
