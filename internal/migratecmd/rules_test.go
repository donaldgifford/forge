package migratecmd_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/migratecmd"
)

func TestRewriteTemplate_BareField(t *testing.T) {
	t.Parallel()

	out, hits, err := migratecmd.RewriteTemplate("test", `Hello {{ .name }}`)
	require.NoError(t, err)
	assert.Empty(t, hits)
	assert.Equal(t, `Hello ${name}`, out)
}

func TestRewriteTemplate_NestedField(t *testing.T) {
	t.Parallel()

	out, hits, err := migratecmd.RewriteTemplate("test", `{{ .a.b }}`)
	require.NoError(t, err)
	assert.Empty(t, hits)
	assert.Equal(t, `${a.b}`, out)
}

func TestRewriteTemplate_FuncCall(t *testing.T) {
	t.Parallel()

	out, hits, err := migratecmd.RewriteTemplate("test", `{{ snakeCase .name }}`)
	require.NoError(t, err)
	assert.Empty(t, hits)
	assert.Equal(t, `${snakeCase(name)}`, out)
}

func TestRewriteTemplate_PipeFunc(t *testing.T) {
	t.Parallel()

	out, hits, err := migratecmd.RewriteTemplate("test", `{{ .name | snakeCase }}`)
	require.NoError(t, err)
	assert.Empty(t, hits)
	assert.Equal(t, `${snakeCase(name)}`, out)
}

func TestRewriteTemplate_PipeFuncWithArg(t *testing.T) {
	t.Parallel()

	out, hits, err := migratecmd.RewriteTemplate("test", `{{ .name | replace "-" "_" }}`)
	require.NoError(t, err)
	assert.Empty(t, hits)
	assert.Equal(t, `${replace(name, "-", "_")}`, out)
}

func TestRewriteTemplate_Default(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "pipe form",
			input:    `{{ .x | default "fb" }}`,
			expected: `${coalesce(x, "fb")}`,
		},
		{
			name:     "positional form swaps args",
			input:    `{{ default .x "fb" }}`,
			expected: `${coalesce("fb", x)}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, hits, err := migratecmd.RewriteTemplate("test", tt.input)
			require.NoError(t, err)
			assert.Empty(t, hits)
			assert.Equal(t, tt.expected, out)
		})
	}
}

func TestRewriteTemplate_If(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple if/end",
			input:    `{{ if .x }}on{{ end }}`,
			expected: `%{ if x ~}on%{ endif ~}`,
		},
		{
			name:     "if/else/end",
			input:    `{{ if .x }}on{{ else }}off{{ end }}`,
			expected: `%{ if x ~}on%{ else ~}off%{ endif ~}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, hits, err := migratecmd.RewriteTemplate("test", tt.input)
			require.NoError(t, err)
			assert.Empty(t, hits)
			assert.Equal(t, tt.expected, out)
		})
	}
}

func TestRewriteTemplate_Comparators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "eq",
			input:    `{{ if eq .x "y" }}on{{ end }}`,
			expected: `%{ if x == "y" ~}on%{ endif ~}`,
		},
		{
			name:     "ne",
			input:    `{{ if ne .x "y" }}on{{ end }}`,
			expected: `%{ if x != "y" ~}on%{ endif ~}`,
		},
		{
			name:     "not",
			input:    `{{ if not .x }}on{{ end }}`,
			expected: `%{ if !x ~}on%{ endif ~}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, hits, err := migratecmd.RewriteTemplate("test", tt.input)
			require.NoError(t, err)
			assert.Empty(t, hits)
			assert.Equal(t, tt.expected, out)
		})
	}
}

func TestRewriteTemplate_TextPreserved(t *testing.T) {
	t.Parallel()

	// Text outside actions — including HCL/Helm-looking content — must
	// pass through verbatim. text/template never inspects {{ }} that
	// appear inside text contexts after a v1 action ends; only the v1
	// {{ … }} delimiters get reparsed. Helm-style {{ .Values.x }}
	// content in a v1 source is itself a v1 action — but the rewriter
	// turns it into ${.Values.x} which is wrong for Helm. The migration
	// philosophy is: authors stripped those by hand BEFORE running the
	// migrator (the v1 file would have crashed on render anyway). The
	// migrator handles only well-formed v1 input.
	out, hits, err := migratecmd.RewriteTemplate(
		"test",
		"plain text\n  with: leading-spaces\n",
	)
	require.NoError(t, err)
	assert.Empty(t, hits)
	assert.Equal(t, "plain text\n  with: leading-spaces\n", out)
}

func TestRewriteTemplate_RangeReportsUntranslated(t *testing.T) {
	t.Parallel()

	out, hits, err := migratecmd.RewriteTemplate("test", `{{ range .items }}x{{ end }}`)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "range block", hits[0].Reason)
	assert.Equal(t, "test", hits[0].File)
	assert.Equal(t, 1, hits[0].Line)
	assert.NotEmpty(t, out, "verbatim fallback should still emit something")
}

func TestRewriteTemplate_WithReportsUntranslated(t *testing.T) {
	t.Parallel()

	_, hits, err := migratecmd.RewriteTemplate("test", `{{ with .x }}body{{ end }}`)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "with block", hits[0].Reason)
}

func TestRewriteTemplate_Idempotent(t *testing.T) {
	t.Parallel()

	// HCL syntax has no v1 actions, so a v2 file that accidentally
	// runs through the migrator should pass through untouched.
	src := `name: ${project_name}
%{ if use_grpc ~}
grpc: yes
%{ endif ~}
`
	out, hits, err := migratecmd.RewriteTemplate("test", src)
	require.NoError(t, err)
	assert.Empty(t, hits)
	assert.Equal(t, src, out)
}
