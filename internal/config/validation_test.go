package config_test

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/config"
)

// parseCondition is the test helper for building a single-line
// hcl.Expression that emulates what the loader hands to Validation.Condition.
func parseCondition(t *testing.T, src string) hcl.Expression {
	t.Helper()

	expr, diags := hclsyntax.ParseExpression([]byte(src), "validation.hcl", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "parse %q: %s", src, diags.Error())

	return expr
}

// TestBuildEvalContext_ExposesBothNamespaces verifies a bound value
// surfaces under both the bare reference and the `var.*` namespace —
// the bridge that keeps legacy bare-ref defaults compiling while new
// validation conditions use the `var.X` form.
func TestBuildEvalContext_ExposesBothNamespaces(t *testing.T) {
	t.Parallel()

	bound := map[string]cty.Value{"project_name": cty.StringVal("my-api")}

	ctx := config.BuildEvalContext(bound)

	require.NotNil(t, ctx)
	require.Contains(t, ctx.Variables, "project_name", "bare-name reference must resolve")
	assert.Equal(t, cty.StringVal("my-api"), ctx.Variables["project_name"])

	require.Contains(t, ctx.Variables, "var", "var.* namespace must be populated")

	varVal := ctx.Variables["var"]
	require.True(t, varVal.Type().IsObjectType())
	assert.Equal(t, cty.StringVal("my-api"), varVal.GetAttr("project_name"))
}

// TestBuildEvalContext_EmptyBoundSkipsVarNamespace keeps the eval
// context cheap when no variables have been resolved yet — useful
// for the early-prompt path where renderDefault for the first
// variable runs against an empty `current` map.
func TestBuildEvalContext_EmptyBoundSkipsVarNamespace(t *testing.T) {
	t.Parallel()

	ctx := config.BuildEvalContext(nil)

	require.NotNil(t, ctx)
	assert.NotContains(t, ctx.Variables, "var",
		"var.* namespace must only appear once at least one variable is bound")
}

// TestEvaluateValidations_HappyPath: a regex-based condition that
// passes returns no errors. Mirrors the canonical "kebab-case
// project_name" migration pattern from the v0.7 fixtures.
func TestEvaluateValidations_HappyPath(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{
			Name: "project_name",
			Type: cty.String,
			Validations: []config.Validation{
				{
					Condition:    parseCondition(t, `can(regex("^[a-z][a-z0-9-]*$", var.project_name))`),
					ErrorMessage: "project_name must be kebab-case",
				},
			},
		},
	}

	bound := map[string]cty.Value{"project_name": cty.StringVal("my-api")}

	errs := config.EvaluateValidations(vars, bound)
	assert.Empty(t, errs, "valid input must produce no errors: %v", errs)
}

// TestEvaluateValidations_FailureCarriesErrorMessage: a failing
// condition surfaces the author's verbatim error_message plus the
// variable name and the source range.
func TestEvaluateValidations_FailureCarriesErrorMessage(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{
			Name: "project_name",
			Type: cty.String,
			Validations: []config.Validation{
				{
					Condition:    parseCondition(t, `can(regex("^[a-z][a-z0-9-]*$", var.project_name))`),
					ErrorMessage: "project_name must be kebab-case",
					DefRange:     hcl.Range{Filename: "blueprint.hcl", Start: hcl.Pos{Line: 7, Column: 3}, End: hcl.Pos{Line: 9, Column: 4}},
				},
			},
		},
	}

	bound := map[string]cty.Value{"project_name": cty.StringVal("INVALID-Caps")}

	errs := config.EvaluateValidations(vars, bound)
	require.Len(t, errs, 1)

	got := errs[0].Error()
	assert.Contains(t, got, "project_name must be kebab-case", "verbatim error_message must appear")
	assert.Contains(t, got, `"project_name"`, "variable name must appear")
	assert.Contains(t, got, "blueprint.hcl", "source range must appear")
}

// TestEvaluateValidations_StacksMultipleFailures: every failing
// validation surfaces, not just the first — authors should see the
// full constraint set in one shot.
func TestEvaluateValidations_StacksMultipleFailures(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{
			Name: "license",
			Type: cty.String,
			Validations: []config.Validation{
				{
					Condition:    parseCondition(t, `contains(["MIT", "Apache-2.0"], var.license)`),
					ErrorMessage: "license must be MIT or Apache-2.0",
				},
				{
					Condition:    parseCondition(t, `var.license != ""`),
					ErrorMessage: "license must not be empty",
				},
			},
		},
	}

	// "" fails both conditions (not in the contains() list AND empty).
	bound := map[string]cty.Value{"license": cty.StringVal("")}

	errs := config.EvaluateValidations(vars, bound)
	require.Len(t, errs, 2, "every failing condition must surface independently")

	combined := errs[0].Error() + "\n" + errs[1].Error()
	assert.Contains(t, combined, "license must be MIT or Apache-2.0")
	assert.Contains(t, combined, "license must not be empty")
}

// TestEvaluateValidations_CrossVariableReferences covers DESIGN-0006
// OQ-4: a validation on variable B can reference variable A through
// the `var.*` scope.
func TestEvaluateValidations_CrossVariableReferences(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{Name: "a", Type: cty.String},
		{Name: "b", Type: cty.String},
		{
			Name: "c",
			Type: cty.String,
			Validations: []config.Validation{
				{
					Condition:    parseCondition(t, `var.a != var.b`),
					ErrorMessage: "a and b must differ",
				},
			},
		},
	}

	// a != b: passes.
	good := map[string]cty.Value{
		"a": cty.StringVal("x"),
		"b": cty.StringVal("y"),
		"c": cty.StringVal("z"),
	}
	assert.Empty(t, config.EvaluateValidations(vars, good),
		"distinct a/b must satisfy the cross-variable condition")

	// a == b: fails.
	bad := map[string]cty.Value{
		"a": cty.StringVal("x"),
		"b": cty.StringVal("x"),
		"c": cty.StringVal("z"),
	}

	errs := config.EvaluateValidations(vars, bad)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "a and b must differ")
}

// TestEvaluateValidations_NonBoolConditionErrors: a validation that
// produces a non-bool value (e.g. forgot the comparison) surfaces a
// targeted diagnostic rather than panicking on cty.Bool conversion.
func TestEvaluateValidations_NonBoolConditionErrors(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{
			Name: "count",
			Type: cty.Number,
			Validations: []config.Validation{
				{
					// Numeric expression instead of a bool comparison.
					Condition:    parseCondition(t, `var.count + 1`),
					ErrorMessage: "(unreachable — fails the type check first)",
				},
			},
		},
	}

	bound := map[string]cty.Value{"count": cty.NumberIntVal(5)}

	errs := config.EvaluateValidations(vars, bound)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "must yield a bool",
		"non-bool conditions must surface a typed-result diagnostic")
}

// TestEvaluateValidations_BadExpressionErrors: a condition that
// references an undeclared variable produces a diagnostic with the
// variable name and source range, not a panic.
func TestEvaluateValidations_BadExpressionErrors(t *testing.T) {
	t.Parallel()

	vars := []config.Variable{
		{
			Name: "project_name",
			Type: cty.String,
			Validations: []config.Validation{
				{
					Condition:    parseCondition(t, `var.does_not_exist == "x"`),
					ErrorMessage: "(unreachable — fails to evaluate first)",
				},
			},
		},
	}

	bound := map[string]cty.Value{"project_name": cty.StringVal("my-api")}

	errs := config.EvaluateValidations(vars, bound)
	require.Len(t, errs, 1)
	msg := errs[0].Error()
	assert.True(t,
		strings.Contains(msg, "does_not_exist") || strings.Contains(msg, "Unsupported attribute"),
		"undeclared reference must surface in the diagnostic: %s", msg)
}

// TestJoinErrors composes multi-failure output into a single Go
// error that callers can wrap once and surface verbatim.
func TestJoinErrors(t *testing.T) {
	t.Parallel()

	require.NoError(t, config.JoinErrors(nil), "nil input must produce nil error")

	joined := config.JoinErrors([]error{
		stringError("first"),
		stringError("second"),
	})
	require.Error(t, joined)
	msg := joined.Error()
	assert.Contains(t, msg, "first")
	assert.Contains(t, msg, "second")
}

type stringError string

func (e stringError) Error() string { return string(e) }
