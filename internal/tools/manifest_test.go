package tools_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/tools"
)

func TestResolveTools_RegistryOnly(t *testing.T) {
	t.Parallel()

	regTools := []config.Tool{
		{Name: "actionlint", Version: "1.7.0"},
		{Name: "golangci-lint", Version: "1.60.0"},
	}

	result, err := tools.ResolveTools(regTools, "", nil, nil)
	require.NoError(t, err)

	assert.Len(t, result, 2)
	assert.Equal(t, "actionlint", result[0].Name)
	assert.Equal(t, "registry", result[0].SourceLayer)
}

func TestResolveTools_BlueprintOverrides(t *testing.T) {
	t.Parallel()

	regTools := []config.Tool{
		{Name: "golangci-lint", Version: "1.60.0"},
	}
	bpTools := []config.Tool{
		{Name: "golangci-lint", Version: "1.62.0"},
	}

	result, err := tools.ResolveTools(regTools, "", bpTools, nil)
	require.NoError(t, err)

	require.Len(t, result, 1)
	assert.Equal(t, "1.62.0", result[0].Version)
	assert.Equal(t, "blueprint", result[0].SourceLayer)
}

func TestResolveTools_CategoryOverrides(t *testing.T) {
	t.Parallel()

	catDir := t.TempDir()
	catPath := filepath.Join(catDir, "tools.yaml")

	content := `tools:
  - name: golangci-lint
    version: "1.61.0"
`
	require.NoError(t, os.WriteFile(catPath, []byte(content), 0o644))

	regTools := []config.Tool{
		{Name: "golangci-lint", Version: "1.60.0"},
	}

	result, err := tools.ResolveTools(regTools, catPath, nil, nil)
	require.NoError(t, err)

	require.Len(t, result, 1)
	assert.Equal(t, "1.61.0", result[0].Version)
	assert.Equal(t, "category", result[0].SourceLayer)
}

func TestResolveTools_ConditionalExclude(t *testing.T) {
	t.Parallel()

	bpTools := []config.Tool{
		{
			Name:      "protoc",
			Version:   "3.20.0",
			Condition: `{{ eq .use_grpc "true" }}`,
		},
		{
			Name:    "golangci-lint",
			Version: "1.60.0",
		},
	}

	vars := map[string]any{"use_grpc": "false"}

	result, err := tools.ResolveTools(nil, "", bpTools, vars)
	require.NoError(t, err)

	assert.Len(t, result, 1)
	assert.Equal(t, "golangci-lint", result[0].Name)
}

func TestResolveTools_ConditionalInclude(t *testing.T) {
	t.Parallel()

	bpTools := []config.Tool{
		{
			Name:      "protoc",
			Version:   "3.20.0",
			Condition: `{{ eq .use_grpc "true" }}`,
		},
	}

	vars := map[string]any{"use_grpc": "true"}

	result, err := tools.ResolveTools(nil, "", bpTools, vars)
	require.NoError(t, err)

	assert.Len(t, result, 1)
	assert.Equal(t, "protoc", result[0].Name)
}

func TestResolveTools_MissingCategoryFile(t *testing.T) {
	t.Parallel()

	result, err := tools.ResolveTools(nil, "/nonexistent/tools.yaml", nil, nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestResolveTools_EmptyCategoryPath(t *testing.T) {
	t.Parallel()

	result, err := tools.ResolveTools(nil, "", nil, nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestResolveTools_OrderPreserved(t *testing.T) {
	t.Parallel()

	regTools := []config.Tool{
		{Name: "tool-a", Version: "1.0"},
		{Name: "tool-b", Version: "1.0"},
		{Name: "tool-c", Version: "1.0"},
	}

	result, err := tools.ResolveTools(regTools, "", nil, nil)
	require.NoError(t, err)

	require.Len(t, result, 3)
	assert.Equal(t, "tool-a", result[0].Name)
	assert.Equal(t, "tool-b", result[1].Name)
	assert.Equal(t, "tool-c", result[2].Name)
}
