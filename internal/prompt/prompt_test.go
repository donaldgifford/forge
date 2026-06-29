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

// --- IMPL-0009 Phase E: prompt UX for structured types ---

// TestCollectVariables_ObjectUnfoldDeclarationOrder covers E.1 + E.3:
// the prompt callback fires once per object field in the order
// TypeFieldOrder dictates, regardless of cty.Object's hash-based
// attribute iteration.
func TestCollectVariables_ObjectUnfoldDeclarationOrder(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{
			Name: "git_provider",
			Type: cty.Object(map[string]cty.Type{
				"repo_type":   cty.String,
				"repo_url":    cty.String,
				"project_org": cty.String,
			}),
			TypeFieldOrder: []string{"repo_type", "repo_url", "project_org"},
		},
	}

	var seen []string

	promptFn := func(v *config.Variable, _ map[string]any) (string, error) {
		seen = append(seen, v.Name)

		switch v.Name {
		case "git_provider.repo_type":
			return "github", nil
		case "git_provider.repo_url":
			return "github.com", nil
		case "git_provider.project_org":
			return "donaldgifford", nil
		default:
			return "", nil
		}
	}

	result, err := prompt.CollectVariables(vars, nil, nil, false, promptFn)
	require.NoError(t, err)

	assert.Equal(t,
		[]string{"git_provider.repo_type", "git_provider.repo_url", "git_provider.project_org"},
		seen,
		"prompt callbacks must fire in TypeFieldOrder declaration order")

	gp, ok := result["git_provider"].(cty.Value)
	require.True(t, ok, "object variable should reconstruct as cty.Value")
	assert.Equal(t, cty.StringVal("github"), gp.GetAttr("repo_type"))
	assert.Equal(t, cty.StringVal("github.com"), gp.GetAttr("repo_url"))
	assert.Equal(t, cty.StringVal("donaldgifford"), gp.GetAttr("project_org"))
}

// TestCollectVariables_ObjectUnfoldNested covers nested object
// recursion. The inner object inherits its field order from cty
// attribute iteration (deterministic enough for the test scope:
// asserts shape, not order, for nested levels).
func TestCollectVariables_ObjectUnfoldNested(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{
			Name: "service",
			Type: cty.Object(map[string]cty.Type{
				"name": cty.String,
				"addr": cty.Object(map[string]cty.Type{
					"host": cty.String,
					"port": cty.Number,
				}),
			}),
			TypeFieldOrder: []string{"name", "addr"},
		},
	}

	promptFn := func(v *config.Variable, _ map[string]any) (string, error) {
		switch v.Name {
		case "service.name":
			return "api", nil
		case "service.addr.host":
			return "0.0.0.0", nil
		case "service.addr.port":
			return "8080", nil
		default:
			return "", nil
		}
	}

	result, err := prompt.CollectVariables(vars, nil, nil, false, promptFn)
	require.NoError(t, err)

	svc, ok := result["service"].(cty.Value)
	require.True(t, ok)
	assert.Equal(t, cty.StringVal("api"), svc.GetAttr("name"))

	addr := svc.GetAttr("addr")
	assert.Equal(t, cty.StringVal("0.0.0.0"), addr.GetAttr("host"))

	port, _ := addr.GetAttr("port").AsBigFloat().Int64()
	assert.Equal(t, int64(8080), port)
}

// TestCollectVariables_ListRequiredNonInteractiveError covers E.2:
// a required list variable without a value/default surfaces the
// copy-pasteable vars-file snippet rather than hitting the prompt
// callback.
func TestCollectVariables_ListRequiredNonInteractiveError(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{
			Name:     "exposed_ports",
			Type:     cty.List(cty.Number),
			Required: true,
		},
	}

	promptFn := func(_ *config.Variable, _ map[string]any) (string, error) {
		t.Fatal("prompt callback must NOT fire for list-typed variables")
		return "", nil
	}

	_, err := prompt.CollectVariables(vars, nil, nil, false, promptFn)
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "exposed_ports")
	assert.Contains(t, msg, "list of number")
	assert.Contains(t, msg, "--var-file",
		"error must point users at the --var-file escape hatch")
	assert.Contains(t, msg, "project.forge-vars.hcl",
		"error must include the copy-pasteable vars-file path")
	assert.Contains(t, msg, "exposed_ports = [...]",
		"error must include a one-line example snippet for the variable")
}

// TestCollectVariables_MapRequiredNonInteractiveError mirrors the
// list case for map(T) types.
func TestCollectVariables_MapRequiredNonInteractiveError(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{
			Name:     "build_targets",
			Type:     cty.Map(cty.String),
			Required: true,
		},
	}

	promptFn := func(_ *config.Variable, _ map[string]any) (string, error) {
		t.Fatal("prompt callback must NOT fire for map-typed variables")
		return "", nil
	}

	_, err := prompt.CollectVariables(vars, nil, nil, false, promptFn)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build_targets")
	assert.Contains(t, err.Error(), `build_targets = { key = "value" }`,
		"the snippet must show map shape, not list shape")
}

// TestCollectVariables_ListOptionalWithDefault covers the silent
// happy path: a list variable with a default (or just not required)
// flows through without prompting or erroring.
func TestCollectVariables_ListOptionalWithDefault(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{
			Name:     "exposed_ports",
			Type:     cty.List(cty.Number),
			Required: false,
		},
	}

	promptFn := func(_ *config.Variable, _ map[string]any) (string, error) {
		t.Fatal("prompt callback must NOT fire for non-required list variables")
		return "", nil
	}

	result, err := prompt.CollectVariables(vars, nil, nil, false, promptFn)
	require.NoError(t, err)

	got, ok := result["exposed_ports"].(cty.Value)
	require.True(t, ok)
	assert.True(t, got.IsNull(), "optional list with no default must yield null")
	assert.True(t, cty.List(cty.Number).Equals(got.Type()))
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
