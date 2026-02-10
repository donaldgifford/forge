package registry_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/registry"
)

func TestResolve_ShortName(t *testing.T) {
	t.Parallel()

	resolved, err := registry.Resolve("go/api", "github.com/acme/blueprints")
	require.NoError(t, err)

	assert.Equal(t, "github.com/acme/blueprints", resolved.RegistryURL)
	assert.Equal(t, "go/api", resolved.BlueprintPath)
	assert.Empty(t, resolved.Ref)
	assert.False(t, resolved.Standalone)
}

func TestResolve_ShortNameWithRef(t *testing.T) {
	t.Parallel()

	resolved, err := registry.Resolve("go/api@v2.1.0", "github.com/acme/blueprints")
	require.NoError(t, err)

	assert.Equal(t, "github.com/acme/blueprints", resolved.RegistryURL)
	assert.Equal(t, "go/api", resolved.BlueprintPath)
	assert.Equal(t, "v2.1.0", resolved.Ref)
	assert.False(t, resolved.Standalone)
}

func TestResolve_FullGoGetterURL(t *testing.T) {
	t.Parallel()

	resolved, err := registry.Resolve("github.com/acme/blueprints//go/api", "")
	require.NoError(t, err)

	assert.Equal(t, "github.com/acme/blueprints", resolved.RegistryURL)
	assert.Equal(t, "go/api", resolved.BlueprintPath)
	assert.Empty(t, resolved.Ref)
	assert.False(t, resolved.Standalone)
}

func TestResolve_FullGoGetterURLWithRef(t *testing.T) {
	t.Parallel()

	resolved, err := registry.Resolve("github.com/acme/blueprints//go/api?ref=v2.1.0", "")
	require.NoError(t, err)

	assert.Equal(t, "github.com/acme/blueprints", resolved.RegistryURL)
	assert.Equal(t, "go/api", resolved.BlueprintPath)
	assert.Equal(t, "v2.1.0", resolved.Ref)
}

func TestResolve_SSHStandalone(t *testing.T) {
	t.Parallel()

	resolved, err := registry.Resolve("git@github.com:someone/standalone-blueprint.git", "")
	require.NoError(t, err)

	assert.Equal(t, "git@github.com:someone/standalone-blueprint.git", resolved.RegistryURL)
	assert.True(t, resolved.Standalone)
	assert.Empty(t, resolved.BlueprintPath)
}

func TestResolve_HTTPSStandalone(t *testing.T) {
	t.Parallel()

	resolved, err := registry.Resolve("https://github.com/someone/standalone.git", "")
	require.NoError(t, err)

	assert.Equal(t, "https://github.com/someone/standalone.git", resolved.RegistryURL)
	assert.True(t, resolved.Standalone)
}

func TestResolve_EmptyInput(t *testing.T) {
	t.Parallel()

	_, err := registry.Resolve("", "github.com/acme/blueprints")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestResolve_ShortNameNoDefaultRegistry(t *testing.T) {
	t.Parallel()

	_, err := registry.Resolve("go/api", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no default registry configured")
}

func TestResolve_FullURLEmptySubpath(t *testing.T) {
	t.Parallel()

	_, err := registry.Resolve("github.com/acme/blueprints//", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty blueprint path")
}

func TestResolvedBlueprint_GetterURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resolved registry.ResolvedBlueprint
		expected string
	}{
		{
			name: "registry blueprint without ref",
			resolved: registry.ResolvedBlueprint{
				RegistryURL:   "github.com/acme/blueprints",
				BlueprintPath: "go/api",
			},
			expected: "github.com/acme/blueprints//go/api",
		},
		{
			name: "registry blueprint with ref",
			resolved: registry.ResolvedBlueprint{
				RegistryURL:   "github.com/acme/blueprints",
				BlueprintPath: "go/api",
				Ref:           "v2.1.0",
			},
			expected: "github.com/acme/blueprints//go/api?ref=v2.1.0",
		},
		{
			name: "standalone blueprint",
			resolved: registry.ResolvedBlueprint{
				RegistryURL: "git@github.com:someone/blueprint.git",
				Standalone:  true,
			},
			expected: "git@github.com:someone/blueprint.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.resolved.GetterURL())
		})
	}
}
