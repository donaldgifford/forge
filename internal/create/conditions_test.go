package create_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/create"
	"github.com/donaldgifford/forge/internal/defaults"
	tmpl "github.com/donaldgifford/forge/internal/template"
)

func buildFileSet(paths ...string) *defaults.FileSet {
	fs := defaults.NewFileSet()

	for _, p := range paths {
		fs.Add(&defaults.FileEntry{
			AbsPath:     "/test/" + p,
			RelPath:     p,
			SourceLayer: defaults.LayerBlueprint,
		})
	}

	return fs
}

func TestEvaluateConditions_ExcludeWhenTrue(t *testing.T) {
	t.Parallel()

	fs := buildFileSet(
		"cmd/main.go",
		"proto/service.proto",
		"internal/grpc/server.go",
		"README.md",
	)

	conditions := []config.Condition{
		{
			When:    `!use_grpc`,
			Exclude: []string{"proto/*", "internal/grpc/*"},
		},
	}

	vars := map[string]cty.Value{"use_grpc": cty.False}

	err := create.EvaluateConditions(conditions, vars, fs, tmpl.NewHCLRenderer())
	require.NoError(t, err)

	assert.Equal(t, 2, fs.Len())
	assert.NotNil(t, fs.Get("cmd/main.go"))
	assert.NotNil(t, fs.Get("README.md"))
	assert.Nil(t, fs.Get("proto/service.proto"))
	assert.Nil(t, fs.Get("internal/grpc/server.go"))
}

func TestEvaluateConditions_KeepWhenFalse(t *testing.T) {
	t.Parallel()

	fs := buildFileSet(
		"cmd/main.go",
		"proto/service.proto",
		"internal/grpc/server.go",
	)

	conditions := []config.Condition{
		{
			When:    `!use_grpc`,
			Exclude: []string{"proto/*", "internal/grpc/*"},
		},
	}

	vars := map[string]cty.Value{"use_grpc": cty.True}

	err := create.EvaluateConditions(conditions, vars, fs, tmpl.NewHCLRenderer())
	require.NoError(t, err)

	assert.Equal(t, 3, fs.Len())
}

func TestEvaluateConditions_DirectoryPrefix(t *testing.T) {
	t.Parallel()

	fs := buildFileSet(
		"docs/README.md",
		"docs/api/spec.yaml",
		"src/main.go",
	)

	conditions := []config.Condition{
		{
			When:    `!include_docs`,
			Exclude: []string{"docs/*"},
		},
	}

	vars := map[string]cty.Value{"include_docs": cty.False}

	err := create.EvaluateConditions(conditions, vars, fs, tmpl.NewHCLRenderer())
	require.NoError(t, err)

	assert.Equal(t, 1, fs.Len())
	assert.NotNil(t, fs.Get("src/main.go"))
}

func TestEvaluateConditions_NoConditions(t *testing.T) {
	t.Parallel()

	fs := buildFileSet("cmd/main.go", "README.md")

	err := create.EvaluateConditions(nil, map[string]cty.Value{}, fs, tmpl.NewHCLRenderer())
	require.NoError(t, err)

	assert.Equal(t, 2, fs.Len())
}

func TestEvaluateConditions_MultipleConditions(t *testing.T) {
	t.Parallel()

	fs := buildFileSet(
		"cmd/main.go",
		"proto/service.proto",
		"docker/Dockerfile",
		"README.md",
	)

	conditions := []config.Condition{
		{
			When:    `!use_grpc`,
			Exclude: []string{"proto/*"},
		},
		{
			When:    `!use_docker`,
			Exclude: []string{"docker/*"},
		},
	}

	vars := map[string]cty.Value{
		"use_grpc":   cty.False,
		"use_docker": cty.False,
	}

	err := create.EvaluateConditions(conditions, vars, fs, tmpl.NewHCLRenderer())
	require.NoError(t, err)

	assert.Equal(t, 2, fs.Len())
	assert.NotNil(t, fs.Get("cmd/main.go"))
	assert.NotNil(t, fs.Get("README.md"))
}

func TestEvaluateConditions_InvalidTemplate(t *testing.T) {
	t.Parallel()

	fs := buildFileSet("cmd/main.go")

	conditions := []config.Condition{
		{
			When:    `!!!nonsense`,
			Exclude: []string{"*"},
		},
	}

	err := create.EvaluateConditions(conditions, map[string]cty.Value{}, fs, tmpl.NewHCLRenderer())
	require.Error(t, err)
}
