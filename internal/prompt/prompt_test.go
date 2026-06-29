package prompt_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/prompt"
)

// TestCollectVariables_Overrides covers the --set path. After IMPL-0009
// Phase B, types are cty.Type — bool/number coercion follows.
func TestCollectVariables_Overrides(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "project_name", Type: cty.String, Required: true},
		{Name: "use_grpc", Type: cty.Bool, DefaultSource: "false"},
	}

	overrides := map[string]string{
		"project_name": "my-api",
		"use_grpc":     "true",
	}

	result, err := prompt.CollectVariables(vars, overrides, nil, false, nil)
	require.NoError(t, err)

	assert.Equal(t, "my-api", result["project_name"])
	assert.Equal(t, true, result["use_grpc"])
}

func TestCollectVariables_Defaults(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "project_name", Type: cty.String, DefaultSource: "default-project"},
		{Name: "port", Type: cty.Number, DefaultSource: "8080"},
		{Name: "verbose", Type: cty.Bool, DefaultSource: "false"},
	}

	result, err := prompt.CollectVariables(vars, nil, nil, true, nil)
	require.NoError(t, err)

	assert.Equal(t, "default-project", result["project_name"])
	assert.Equal(t, 8080, result["port"])
	assert.Equal(t, false, result["verbose"])
}

func TestCollectVariables_TemplatedDefault(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "project_name", Type: cty.String, DefaultSource: "my-api"},
		{Name: "go_module", Type: cty.String, DefaultSource: "github.com/example/${project_name}"},
	}

	result, err := prompt.CollectVariables(vars, nil, nil, true, nil)
	require.NoError(t, err)

	assert.Equal(t, "github.com/example/my-api", result["go_module"])
}

func TestCollectVariables_RequiredNoDefault(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "project_name", Type: cty.String, Required: true},
	}

	_, err := prompt.CollectVariables(vars, nil, nil, true, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required but has no default")
}

func TestCollectVariables_InvalidBoolOverride(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "flag", Type: cty.Bool},
	}

	overrides := map[string]string{
		"flag": "not-a-bool",
	}

	_, err := prompt.CollectVariables(vars, overrides, nil, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid override")
}

func TestCollectVariables_InvalidIntOverride(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "port", Type: cty.Number},
	}

	overrides := map[string]string{
		"port": "not-a-number",
	}

	_, err := prompt.CollectVariables(vars, overrides, nil, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid override")
}

func TestCollectVariables_PromptFn(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "project_name", Type: cty.String, Required: true},
		{Name: "license", Type: cty.String, DefaultSource: "MIT"},
	}

	promptFn := func(v *config.Variable, _ map[string]any) (string, error) {
		switch v.Name {
		case "project_name":
			return "prompted-project", nil
		case "license":
			return "Apache-2.0", nil
		default:
			return "", nil
		}
	}

	result, err := prompt.CollectVariables(vars, nil, nil, false, promptFn)
	require.NoError(t, err)

	assert.Equal(t, "prompted-project", result["project_name"])
	assert.Equal(t, "Apache-2.0", result["license"])
}

func TestCollectVariables_PromptFnUsesDefault(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "name", Type: cty.String, DefaultSource: "default-name"},
	}

	// Prompt returns empty string — should fall back to default.
	promptFn := func(_ *config.Variable, _ map[string]any) (string, error) {
		return "", nil
	}

	result, err := prompt.CollectVariables(vars, nil, nil, false, promptFn)
	require.NoError(t, err)

	assert.Equal(t, "default-name", result["name"])
}

func TestCollectVariables_OverrideTakesPrecedence(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "name", Type: cty.String, DefaultSource: "default-name"},
	}

	overrides := map[string]string{"name": "override-name"}

	// Even with a promptFn, overrides should win.
	promptFn := func(_ *config.Variable, _ map[string]any) (string, error) {
		return "prompted-name", nil
	}

	result, err := prompt.CollectVariables(vars, overrides, nil, false, promptFn)
	require.NoError(t, err)

	assert.Equal(t, "override-name", result["name"])
}

func TestCollectVariables_ZeroValues(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "name", Type: cty.String},
		{Name: "flag", Type: cty.Bool},
		{Name: "count", Type: cty.Number},
	}

	result, err := prompt.CollectVariables(vars, nil, nil, true, nil)
	require.NoError(t, err)

	assert.Empty(t, result["name"])
	assert.Equal(t, false, result["flag"])
	assert.Equal(t, 0, result["count"])
}
