package create_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/create"
	"github.com/donaldgifford/forge/internal/defaults"
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
			When:    `{{ eq .use_grpc "false" }}`,
			Exclude: []string{"proto/*", "internal/grpc/*"},
		},
	}

	vars := map[string]any{"use_grpc": "false"}

	err := create.EvaluateConditions(conditions, vars, fs)
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
			When:    `{{ eq .use_grpc "false" }}`,
			Exclude: []string{"proto/*", "internal/grpc/*"},
		},
	}

	vars := map[string]any{"use_grpc": "true"}

	err := create.EvaluateConditions(conditions, vars, fs)
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
			When:    `{{ eq .include_docs "false" }}`,
			Exclude: []string{"docs/*"},
		},
	}

	vars := map[string]any{"include_docs": "false"}

	err := create.EvaluateConditions(conditions, vars, fs)
	require.NoError(t, err)

	assert.Equal(t, 1, fs.Len())
	assert.NotNil(t, fs.Get("src/main.go"))
}

func TestEvaluateConditions_NoConditions(t *testing.T) {
	t.Parallel()

	fs := buildFileSet("cmd/main.go", "README.md")

	err := create.EvaluateConditions(nil, map[string]any{}, fs)
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
			When:    `{{ eq .use_grpc "false" }}`,
			Exclude: []string{"proto/*"},
		},
		{
			When:    `{{ eq .use_docker "false" }}`,
			Exclude: []string{"docker/*"},
		},
	}

	vars := map[string]any{
		"use_grpc":   "false",
		"use_docker": "false",
	}

	err := create.EvaluateConditions(conditions, vars, fs)
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
			When:    `{{ invalid }}`,
			Exclude: []string{"*"},
		},
	}

	err := create.EvaluateConditions(conditions, map[string]any{}, fs)
	require.Error(t, err)
}
