package registry_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/registry"
)

func TestLoadIndex(t *testing.T) {
	t.Parallel()

	reg, err := registry.LoadIndex("../../testdata/registry")
	require.NoError(t, err)

	assert.Equal(t, "test-blueprints", reg.Name)
	assert.Len(t, reg.Blueprints, 2)
}

func TestLoadIndex_NotFound(t *testing.T) {
	t.Parallel()

	_, err := registry.LoadIndex("/nonexistent/path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading registry index")
}

func TestFindBlueprint(t *testing.T) {
	t.Parallel()

	reg, err := registry.LoadIndex("../../testdata/registry")
	require.NoError(t, err)

	entry, err := registry.FindBlueprint(reg, "go/api")
	require.NoError(t, err)

	assert.Equal(t, "go/api", entry.Name)
	assert.Equal(t, "go/api", entry.Path)
	assert.Equal(t, "1.0.0", entry.Version)
}

func TestFindBlueprint_ByPath(t *testing.T) {
	t.Parallel()

	reg, err := registry.LoadIndex("../../testdata/registry")
	require.NoError(t, err)

	entry, err := registry.FindBlueprint(reg, "go/cli")
	require.NoError(t, err)

	assert.Equal(t, "go/cli", entry.Name)
}

func TestFindBlueprint_NotFound(t *testing.T) {
	t.Parallel()

	reg, err := registry.LoadIndex("../../testdata/registry")
	require.NoError(t, err)

	_, err = registry.FindBlueprint(reg, "python/flask")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Contains(t, err.Error(), "go/api")
	assert.Contains(t, err.Error(), "go/cli")
}

func TestFindBlueprint_TrailingSlash(t *testing.T) {
	t.Parallel()

	reg, err := registry.LoadIndex("../../testdata/registry")
	require.NoError(t, err)

	entry, err := registry.FindBlueprint(reg, "go/api/")
	require.NoError(t, err)

	assert.Equal(t, "go/api", entry.Name)
}
