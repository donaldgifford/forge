package template_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	tmpl "github.com/donaldgifford/forge/internal/template"
)

func TestHCLRenderer_RenderString(t *testing.T) {
	t.Parallel()

	r := tmpl.NewHCLRenderer()

	tests := []struct {
		name     string
		template string
		vars     map[string]cty.Value
		expected string
	}{
		{
			name:     "scalar interpolation",
			template: `Hello ${name}`,
			vars:     map[string]cty.Value{"name": cty.StringVal("world")},
			expected: "Hello world",
		},
		{
			name:     "if directive true",
			template: `%{ if enabled ~}on%{ else ~}off%{ endif ~}`,
			vars:     map[string]cty.Value{"enabled": cty.True},
			expected: "on",
		},
		{
			name:     "if directive false",
			template: `%{ if enabled ~}on%{ else ~}off%{ endif ~}`,
			vars:     map[string]cty.Value{"enabled": cty.False},
			expected: "off",
		},
		{
			name:     "no expressions",
			template: `plain text`,
			vars:     nil,
			expected: "plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, err := r.RenderString(tt.template, tt.vars)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, out)
		})
	}
}

// TestHCLRenderer_HelmStyleSurvival is the critical fixture per IMPL-0004
// A.4: confirms that {{ .Values.x }} content in a template body is
// preserved verbatim while ${forge_var} interpolations resolve. This is
// the entire reason for the engine swap (INV-0001).
func TestHCLRenderer_HelmStyleSurvival(t *testing.T) {
	t.Parallel()

	r := tmpl.NewHCLRenderer()

	template := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${project_name}
spec:
  replicas: {{ .Values.replicas | default 1 }}
  template:
    spec:
      containers:
        - name: {{ .Values.containerName }}
          image: {{ .Values.image }}:{{ .Values.tag }}
`

	out, err := r.RenderString(template, map[string]cty.Value{
		"project_name": cty.StringVal("my-api"),
	})
	require.NoError(t, err)

	assert.Contains(t, out, "name: my-api")
	assert.Contains(t, out, "{{ .Values.replicas | default 1 }}")
	assert.Contains(t, out, "{{ .Values.containerName }}")
	assert.Contains(t, out, "{{ .Values.image }}:{{ .Values.tag }}")
}

func TestHCLRenderer_RenderFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "deployment.yaml.tmpl")
	require.NoError(t, os.WriteFile(path, []byte(`name: ${project_name}`), 0o644))

	r := tmpl.NewHCLRenderer()

	out, err := r.RenderFile(path, map[string]cty.Value{
		"project_name": cty.StringVal("svc"),
	})
	require.NoError(t, err)
	assert.Equal(t, "name: svc", string(out))
}

func TestHCLRenderer_RenderFile_NotFound(t *testing.T) {
	t.Parallel()

	r := tmpl.NewHCLRenderer()

	_, err := r.RenderFile("/nonexistent/file.tmpl", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading template")
}

func TestHCLRenderer_RenderPath(t *testing.T) {
	t.Parallel()

	r := tmpl.NewHCLRenderer()

	tests := []struct {
		name     string
		path     string
		vars     map[string]cty.Value
		expected string
	}{
		{
			name:     "no expression returns input unchanged",
			path:     "cmd/main.go",
			vars:     nil,
			expected: "cmd/main.go",
		},
		{
			name:     "single interpolation",
			path:     "${project_name}/cmd/main.go",
			vars:     map[string]cty.Value{"project_name": cty.StringVal("svc")},
			expected: "svc/cmd/main.go",
		},
		{
			name: "multiple interpolations",
			path: "${project_name}/${module}/main.go",
			vars: map[string]cty.Value{
				"project_name": cty.StringVal("svc"),
				"module":       cty.StringVal("cmd"),
			},
			expected: "svc/cmd/main.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, err := r.RenderPath(tt.path, tt.vars)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, out)
		})
	}
}

func TestHCLRenderer_EvaluateBool(t *testing.T) {
	t.Parallel()

	r := tmpl.NewHCLRenderer()

	tests := []struct {
		name     string
		expr     string
		vars     map[string]cty.Value
		expected bool
	}{
		{
			name:     "literal true",
			expr:     "true",
			expected: true,
		},
		{
			name:     "literal false",
			expr:     "false",
			expected: false,
		},
		{
			name:     "variable equality",
			expr:     `use_grpc == true`,
			vars:     map[string]cty.Value{"use_grpc": cty.True},
			expected: true,
		},
		{
			name:     "variable inequality",
			expr:     `use_grpc == true`,
			vars:     map[string]cty.Value{"use_grpc": cty.False},
			expected: false,
		},
		{
			name:     "boolean negation",
			expr:     `!enabled`,
			vars:     map[string]cty.Value{"enabled": cty.False},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := r.EvaluateBool(tt.expr, tt.vars)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestHCLRenderer_DiagnosticErrorIncludesSource verifies that parse errors
// surface with file/line/column information from hcl.Diagnostics, satisfying
// IMPL-0004 A.4's diagnostics requirement.
func TestHCLRenderer_DiagnosticErrorIncludesSource(t *testing.T) {
	t.Parallel()

	r := tmpl.NewHCLRenderer()

	// Unterminated interpolation triggers an HCL parse diagnostic.
	_, err := r.RenderString(`name: ${unterminated`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inline")
}

func TestHCLRenderer_CustomFuncs(t *testing.T) {
	t.Parallel()

	r := tmpl.NewHCLRenderer()

	tests := []struct {
		name     string
		template string
		vars     map[string]cty.Value
		expected string
	}{
		{"snakeCase", `${snakeCase("MyProject")}`, nil, "my_project"},
		{"camelCase", `${camelCase("my-project")}`, nil, "myProject"},
		{"pascalCase", `${pascalCase("my_project")}`, nil, "MyProject"},
		{"kebabCase", `${kebabCase("MyProject")}`, nil, "my-project"},
		{"upper from stdlib", `${upper("abc")}`, nil, "ABC"},
		{"lower from stdlib", `${lower("ABC")}`, nil, "abc"},
		{"title from stdlib", `${title("hello world")}`, nil, "Hello World"},
		{"replace from stdlib", `${replace("foo-bar", "-", "_")}`, nil, "foo_bar"},
		{"trimPrefix from stdlib", `${trimPrefix("v1.2.3", "v")}`, nil, "1.2.3"},
		{"trimSuffix from stdlib", `${trimSuffix("file.tmpl", ".tmpl")}`, nil, "file"},
		{
			"coalesce empty falls through",
			`${coalesce(empty, "fallback")}`,
			map[string]cty.Value{"empty": cty.NullVal(cty.String)},
			"fallback",
		},
		{
			"coalesce keeps non-empty",
			`${coalesce(present, "fallback")}`,
			map[string]cty.Value{"present": cty.StringVal("kept")},
			"kept",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, err := r.RenderString(tt.template, tt.vars)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, out)
		})
	}
}

func TestHCLRenderer_NowAndEnv(t *testing.T) {
	r := tmpl.NewHCLRenderer()

	t.Run("now formats year", func(t *testing.T) {
		out, err := r.RenderString(`${now("2006")}`, nil)
		require.NoError(t, err)
		assert.Len(t, out, 4)
	})

	t.Run("env reads variable", func(t *testing.T) {
		t.Setenv("FORGE_HCL_TEST_VAR", "ok")

		out, err := r.RenderString(`${env("FORGE_HCL_TEST_VAR")}`, nil)
		require.NoError(t, err)
		assert.Equal(t, "ok", out)
	})
}
