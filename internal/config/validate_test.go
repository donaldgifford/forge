package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
)

func TestValidateBlueprint_Valid(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{
		APIVersion: "v1",
		Name:       "test",
		Variables: []config.Variable{
			{Name: "name", Type: "string"},
		},
	}
	require.NoError(t, config.ValidateBlueprint(bp))
}

func TestValidateBlueprint_InvalidAPIVersion(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{
		APIVersion: "v2",
		Name:       "test",
	}
	err := config.ValidateBlueprint(bp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apiVersion")
}

func TestValidateBlueprint_EmptyName(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{
		APIVersion: "v1",
		Name:       "",
	}
	err := config.ValidateBlueprint(bp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestValidateBlueprint_InvalidVariableType(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{
		APIVersion: "v1",
		Name:       "test",
		Variables: []config.Variable{
			{Name: "foo", Type: "float"},
		},
	}
	err := config.ValidateBlueprint(bp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid type")
}

func TestValidateBlueprint_ChoiceWithoutChoices(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{
		APIVersion: "v1",
		Name:       "test",
		Variables: []config.Variable{
			{Name: "pick", Type: "choice"},
		},
	}
	err := config.ValidateBlueprint(bp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "choices are required")
}

func TestValidateBlueprint_InvalidRegex(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{
		APIVersion: "v1",
		Name:       "test",
		Variables: []config.Variable{
			{Name: "bad", Type: "string", Validate: "[invalid"},
		},
	}
	err := config.ValidateBlueprint(bp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid validate regex")
}

func TestValidateBlueprint_InvalidOverrideStrategy(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{
		APIVersion: "v1",
		Name:       "test",
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
		APIVersion: "v1",
		Name:       "test",
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
		APIVersion: "v1",
		Name:       "test-registry",
		Blueprints: []config.BlueprintEntry{
			{Name: "go/api", Path: "go/api"},
		},
	}
	require.NoError(t, config.ValidateRegistry(reg))
}

func TestValidateRegistry_InvalidAPIVersion(t *testing.T) {
	t.Parallel()

	reg := &config.Registry{
		APIVersion: "v0",
		Name:       "test",
	}
	err := config.ValidateRegistry(reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apiVersion")
}

func TestValidateRegistry_MissingBlueprintPath(t *testing.T) {
	t.Parallel()

	reg := &config.Registry{
		APIVersion: "v1",
		Name:       "test",
		Blueprints: []config.BlueprintEntry{
			{Name: "go/api", Path: ""},
		},
	}
	err := config.ValidateRegistry(reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

func TestValidateBlueprint_VariableNameRequired(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{
		APIVersion: "v1",
		Name:       "test",
		Variables: []config.Variable{
			{Name: "", Type: "string"},
		},
	}
	err := config.ValidateBlueprint(bp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestValidateBlueprint_VariableTypeRequired(t *testing.T) {
	t.Parallel()

	bp := &config.Blueprint{
		APIVersion: "v1",
		Name:       "test",
		Variables: []config.Variable{
			{Name: "foo", Type: ""},
		},
	}
	err := config.ValidateBlueprint(bp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type is required")
}
