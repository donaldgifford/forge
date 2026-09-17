package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/config"
)

// TestValidateBlueprint_Valid covers the happy path: a blueprint with
// one well-formed scalar variable passes ValidateBlueprint.
//
// Type-shape validation moved to ParseVariableType (vartype.go) at
// LoadBlueprint time per IMPL-0009 B.5 — ValidateBlueprint is now
// limited to invariants that aren't expressible in the HCL schema
// (variable name non-empty, sync strategies in the allowed set,
// managed file paths non-empty).
func TestValidateBlueprint_Valid(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{
		Name: "test",
		Variables: []config.Variable{
			{Name: "name", Type: cty.String},
		},
	}
	require.NoError(t, config.ValidateBlueprint(bp))
}

func TestValidateBlueprint_EmptyName(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{Name: ""}
	err := config.ValidateBlueprint(bp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestValidateBlueprint_InvalidOverrideStrategy(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{
		Name: "test",
		Defaults: config.Defaults{
			OverrideStrategy: map[string]string{"file.txt": "invalid"},
		},
	}
	err := config.ValidateBlueprint(bp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid override_strategy")
}

func TestValidateBlueprint_InvalidManagedFileStrategy(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{
		Name: "test",
		Sync: config.SyncConfig{
			ManagedFiles: []config.ManagedFile{
				{Path: "Makefile", Strategy: "bad"},
			},
		},
	}
	err := config.ValidateBlueprint(bp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid strategy")
}

func TestValidateRegistry_Valid(t *testing.T) {
	t.Parallel()

	reg := &config.Registry{
		Name: "test-registry",
		Blueprints: []config.BlueprintEntry{
			{Name: "go/api", Path: "go/api"},
		},
	}
	require.NoError(t, config.ValidateRegistry(reg))
}

func TestValidateRegistry_EmptyName(t *testing.T) {
	t.Parallel()

	reg := &config.Registry{Name: ""}
	err := config.ValidateRegistry(reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestValidateRegistry_MissingBlueprintPath(t *testing.T) {
	t.Parallel()

	reg := &config.Registry{
		Name: "test",
		Blueprints: []config.BlueprintEntry{
			{Name: "go/api", Path: ""},
		},
	}
	err := config.ValidateRegistry(reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

// TestValidateBlueprint_VariableNameRequired covers the one
// variable-level validation that survives in ValidateBlueprint —
// per-name uniqueness check still belongs to a future RFC.
func TestValidateBlueprint_VariableNameRequired(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{
		Name: "test",
		Variables: []config.Variable{
			{Name: "", Type: cty.String},
		},
	}
	err := config.ValidateBlueprint(bp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}
