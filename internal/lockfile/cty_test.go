package lockfile_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/lockfile"
)

func TestToCtyValues_DeclaredTypesWin(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "use_grpc", Type: "bool"},
		{Name: "replicas", Type: "int"},
		{Name: "project_name", Type: "string"},
		{Name: "license", Type: "choice"},
	}

	raw := map[string]any{
		"use_grpc":     "true", // string in YAML, should coerce to bool
		"replicas":     "3",    // string in YAML, should coerce to int
		"project_name": "myapp",
		"license":      "MIT",
	}

	got, err := lockfile.ToCtyValues(raw, vars)
	require.NoError(t, err)

	assert.Equal(t, cty.True, got["use_grpc"])
	assert.Equal(t, cty.NumberIntVal(3), got["replicas"])
	assert.Equal(t, cty.StringVal("myapp"), got["project_name"])
	assert.Equal(t, cty.StringVal("MIT"), got["license"])
}

func TestToCtyValues_PreservesNativeTypes(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "use_grpc", Type: "bool"},
		{Name: "replicas", Type: "int"},
	}

	raw := map[string]any{
		"use_grpc": false,
		"replicas": 7,
	}

	got, err := lockfile.ToCtyValues(raw, vars)
	require.NoError(t, err)

	assert.Equal(t, cty.False, got["use_grpc"])
	assert.Equal(t, cty.NumberIntVal(7), got["replicas"])
}

func TestToCtyValues_BackfillsMissingDeclared(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "project_name", Type: "string"},
		{Name: "use_grpc", Type: "bool"},
	}

	raw := map[string]any{
		"project_name": "myapp",
	}

	got, err := lockfile.ToCtyValues(raw, vars)
	require.NoError(t, err)

	assert.Equal(t, cty.StringVal("myapp"), got["project_name"])

	v, ok := got["use_grpc"]
	require.True(t, ok, "missing declared variable should be backfilled")
	assert.True(t, v.IsNull())
	assert.Equal(t, cty.Bool, v.Type())
}

func TestToCtyValues_FallsBackWhenUndeclared(t *testing.T) {
	t.Parallel()

	raw := map[string]any{
		"adhoc_string": "hello",
		"adhoc_int":    42,
		"adhoc_bool":   true,
	}

	got, err := lockfile.ToCtyValues(raw, nil)
	require.NoError(t, err)

	assert.Equal(t, cty.StringVal("hello"), got["adhoc_string"])
	assert.Equal(t, cty.NumberIntVal(42), got["adhoc_int"])
	assert.Equal(t, cty.True, got["adhoc_bool"])
}

func TestFromCtyValues_RoundTrip(t *testing.T) {
	t.Parallel()

	vals := map[string]cty.Value{
		"name":     cty.StringVal("myapp"),
		"enabled":  cty.True,
		"replicas": cty.NumberIntVal(5),
	}

	raw := lockfile.FromCtyValues(vals)

	assert.Equal(t, "myapp", raw["name"])
	assert.Equal(t, true, raw["enabled"])
	assert.Equal(t, int64(5), raw["replicas"])
}

func TestToCtyValues_RejectsBadCoercion(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "use_grpc", Type: "bool"},
	}

	raw := map[string]any{
		"use_grpc": "not-a-bool",
	}

	_, err := lockfile.ToCtyValues(raw, vars)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "use_grpc")
}
