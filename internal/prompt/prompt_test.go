package prompt_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/prompt"
)

func TestCollectVariables_Overrides(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "project_name", Type: "string", Required: true},
		{Name: "use_grpc", Type: "bool", Default: "false"},
	}

	overrides := map[string]string{
		"project_name": "my-api",
		"use_grpc":     "true",
	}

	result, err := prompt.CollectVariables(vars, overrides, false, nil)
	require.NoError(t, err)

	assert.Equal(t, "my-api", result["project_name"])
	assert.Equal(t, true, result["use_grpc"])
}

func TestCollectVariables_Defaults(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "project_name", Type: "string", Default: "default-project"},
		{Name: "port", Type: "int", Default: "8080"},
		{Name: "verbose", Type: "bool", Default: "false"},
	}

	result, err := prompt.CollectVariables(vars, nil, true, nil)
	require.NoError(t, err)

	assert.Equal(t, "default-project", result["project_name"])
	assert.Equal(t, 8080, result["port"])
	assert.Equal(t, false, result["verbose"])
}

func TestCollectVariables_TemplatedDefault(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "project_name", Type: "string", Default: "my-api"},
		{Name: "go_module", Type: "string", Default: "github.com/example/{{ .project_name }}"},
	}

	result, err := prompt.CollectVariables(vars, nil, true, nil)
	require.NoError(t, err)

	assert.Equal(t, "github.com/example/my-api", result["go_module"])
}

func TestCollectVariables_RequiredNoDefault(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "project_name", Type: "string", Required: true},
	}

	_, err := prompt.CollectVariables(vars, nil, true, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required but has no default")
}

func TestCollectVariables_OverrideValidation(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "project_name", Type: "string", Validate: "^[a-z][a-z0-9-]*$"},
	}

	overrides := map[string]string{
		"project_name": "INVALID",
	}

	_, err := prompt.CollectVariables(vars, overrides, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed validation")
}

func TestCollectVariables_InvalidBoolOverride(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "flag", Type: "bool"},
	}

	overrides := map[string]string{
		"flag": "not-a-bool",
	}

	_, err := prompt.CollectVariables(vars, overrides, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid override")
}

func TestCollectVariables_InvalidIntOverride(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "port", Type: "int"},
	}

	overrides := map[string]string{
		"port": "not-a-number",
	}

	_, err := prompt.CollectVariables(vars, overrides, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid override")
}

func TestCollectVariables_PromptFn(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "project_name", Type: "string", Required: true},
		{Name: "license", Type: "choice", Choices: []string{"MIT", "Apache-2.0"}, Default: "MIT"},
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

	result, err := prompt.CollectVariables(vars, nil, false, promptFn)
	require.NoError(t, err)

	assert.Equal(t, "prompted-project", result["project_name"])
	assert.Equal(t, "Apache-2.0", result["license"])
}

func TestCollectVariables_PromptFnUsesDefault(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "name", Type: "string", Default: "default-name"},
	}

	// Prompt returns empty string — should fall back to default.
	promptFn := func(_ *config.Variable, _ map[string]any) (string, error) {
		return "", nil
	}

	result, err := prompt.CollectVariables(vars, nil, false, promptFn)
	require.NoError(t, err)

	assert.Equal(t, "default-name", result["name"])
}

func TestCollectVariables_OverrideTakesPrecedence(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "name", Type: "string", Default: "default-name"},
	}

	overrides := map[string]string{"name": "override-name"}

	// Even with a promptFn, overrides should win.
	promptFn := func(_ *config.Variable, _ map[string]any) (string, error) {
		return "prompted-name", nil
	}

	result, err := prompt.CollectVariables(vars, overrides, false, promptFn)
	require.NoError(t, err)

	assert.Equal(t, "override-name", result["name"])
}

func TestCollectVariables_ChoiceType(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "license", Type: "choice", Choices: []string{"MIT", "Apache-2.0", "none"}, Default: "Apache-2.0"},
	}

	result, err := prompt.CollectVariables(vars, nil, true, nil)
	require.NoError(t, err)

	assert.Equal(t, "Apache-2.0", result["license"])
}

func TestCollectVariables_ZeroValues(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "name", Type: "string"},
		{Name: "flag", Type: "bool"},
		{Name: "count", Type: "int"},
	}

	result, err := prompt.CollectVariables(vars, nil, true, nil)
	require.NoError(t, err)

	assert.Empty(t, result["name"])
	assert.Equal(t, false, result["flag"])
	assert.Equal(t, 0, result["count"])
}
