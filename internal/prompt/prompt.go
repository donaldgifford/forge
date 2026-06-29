// Package prompt handles interactive variable collection for forge blueprints.
package prompt

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/config"
	tmpl "github.com/donaldgifford/forge/internal/template"
)

// defaultRenderer is the HCL2 renderer used to evaluate default-value
// templates. Constructed once because it carries no per-call state.
var defaultRenderer = tmpl.NewRenderer()

// PromptFn is a callback for interactive variable input.
type PromptFn func(v *config.Variable, current map[string]any) (string, error)

// CollectVariables resolves all blueprint variables using vars-file inputs,
// --set overrides, defaults, and optional interactive prompting. Variables
// are processed in declaration order so that later defaults can reference
// earlier variable values.
//
// Resolution order per declared variable:
//
//  1. varsFileValues (IMPL-0008): a value loaded from `--var-file` short-circuits
//     the override / default / prompt chain. These are already coerced to the
//     declared blueprint type by varsfile.Load.
//  2. overrides: a `--set key=value` value coerced and validated against the
//     declared blueprint type.
//  3. default: the blueprint-supplied default, rendered through the HCL2
//     renderer so it can reference earlier variable values.
//  4. prompt: the interactive callback (skipped when useDefaults is true or
//     promptFn is nil).
//
// varsFileValues and overrides are mutually exclusive at the CLI layer;
// CollectVariables accepts both for flexibility (tests may stub either
// path) but does not enforce that exclusion itself.
//
// IMPL-0009 Phase B: this function still operates on the legacy
// scalar-only resolution shape. Structured-type (object / list / map)
// support and validation-block evaluation land in Phases C and E.
func CollectVariables(
	vars []config.Variable,
	overrides map[string]string,
	varsFileValues map[string]cty.Value,
	useDefaults bool,
	promptFn PromptFn,
) (map[string]any, error) {
	result := make(map[string]any, len(vars))

	for i := range vars {
		v := &vars[i]

		val, err := resolveVariable(v, overrides, varsFileValues, result, useDefaults, promptFn)
		if err != nil {
			return nil, err
		}

		result[v.Name] = val
	}

	return result, nil
}

// resolveVariable resolves a single variable value through the
// vars-file → override → default → prompt chain.
func resolveVariable(
	v *config.Variable,
	overrides map[string]string,
	varsFileValues map[string]cty.Value,
	current map[string]any,
	useDefaults bool,
	promptFn PromptFn,
) (any, error) {
	// 1. Vars-file value short-circuits everything else (IMPL-0008).
	if val, ok := varsFileValues[v.Name]; ok {
		return resolveFromVarsFile(val, v)
	}

	// 2. --set override.
	if raw, ok := overrides[v.Name]; ok {
		return resolveFromOverride(raw, v)
	}

	// Render the default value as a template (it can reference earlier variables).
	defaultVal, err := renderDefault(v.DefaultSource, current)
	if err != nil {
		return nil, fmt.Errorf("rendering default for %q: %w", v.Name, err)
	}

	// If using defaults mode or no prompt function, use the default.
	if useDefaults || promptFn == nil {
		return resolveFromDefault(defaultVal, v)
	}

	// Interactive prompt.
	return resolveFromPrompt(v, current, defaultVal, promptFn)
}

// resolveFromVarsFile converts a cty.Value supplied via --var-file into
// the `any` form used throughout the variable resolution chain. The
// value is already coerced to the declared blueprint type by
// varsfile.Load, so this is a straightforward Go-type unwrap.
func resolveFromVarsFile(val cty.Value, v *config.Variable) (any, error) {
	if !val.IsKnown() || val.IsNull() {
		if v.Required {
			return nil, fmt.Errorf("vars-file value for %q is null but variable is required", v.Name)
		}

		return zeroValue(v.Type), nil
	}

	return ctyToGo(val), nil
}

// ctyToGo converts a cty.Value to the Go `any` shape this package uses
// for the resolution chain. Mirrors lockfile.FromCtyValues for the
// primitive cty types; vars files are scalar-only in IMPL-0008 so
// nested/structural types don't appear here.
func ctyToGo(val cty.Value) any {
	switch val.Type() {
	case cty.String:
		return val.AsString()
	case cty.Bool:
		return val.True()
	case cty.Number:
		if i, acc := val.AsBigFloat().Int64(); acc == 0 {
			return i
		}

		f, _ := val.AsBigFloat().Float64()

		return f
	default:
		return val.GoString()
	}
}

// resolveFromOverride coerces an override value against the declared
// scalar type.
func resolveFromOverride(raw string, v *config.Variable) (any, error) {
	val, err := coerceValue(raw, v.Type)
	if err != nil {
		return nil, fmt.Errorf("invalid override for %q: %w", v.Name, err)
	}

	return val, nil
}

// resolveFromDefault uses the rendered default value, checking required constraints.
func resolveFromDefault(defaultVal string, v *config.Variable) (any, error) {
	if defaultVal == "" && v.Required {
		return nil, fmt.Errorf("variable %q is required but has no default value", v.Name)
	}

	if defaultVal == "" {
		return zeroValue(v.Type), nil
	}

	val, err := coerceValue(defaultVal, v.Type)
	if err != nil {
		return nil, fmt.Errorf("invalid default for %q: %w", v.Name, err)
	}

	return val, nil
}

// resolveFromPrompt calls the prompt function and coerces the result.
func resolveFromPrompt(
	v *config.Variable,
	current map[string]any,
	defaultVal string,
	promptFn PromptFn,
) (any, error) {
	raw, err := promptFn(v, current)
	if err != nil {
		return nil, fmt.Errorf("prompting for %q: %w", v.Name, err)
	}

	if raw == "" {
		raw = defaultVal
	}

	if raw == "" && v.Required {
		return nil, fmt.Errorf("variable %q is required", v.Name)
	}

	val, err := coerceValue(raw, v.Type)
	if err != nil {
		return nil, fmt.Errorf("invalid value for %q: %w", v.Name, err)
	}

	return val, nil
}

// renderDefault renders a default value through the HCL2 renderer with the
// current variable values. Defaults without `${` or `%{` markers short-circuit
// and pass through unchanged.
func renderDefault(defaultTmpl string, current map[string]any) (string, error) {
	if defaultTmpl == "" || (!strings.Contains(defaultTmpl, "${") && !strings.Contains(defaultTmpl, "%{")) {
		return defaultTmpl, nil
	}

	ctyVars := make(map[string]cty.Value, len(current))

	for k, v := range current {
		switch x := v.(type) {
		case string:
			ctyVars[k] = cty.StringVal(x)
		case bool:
			ctyVars[k] = cty.BoolVal(x)
		case int:
			ctyVars[k] = cty.NumberIntVal(int64(x))
		case int64:
			ctyVars[k] = cty.NumberIntVal(x)
		default:
			ctyVars[k] = cty.StringVal(fmt.Sprintf("%v", v))
		}
	}

	out, err := defaultRenderer.RenderString(defaultTmpl, ctyVars)
	if err != nil {
		return "", fmt.Errorf("rendering default %q: %w", defaultTmpl, err)
	}

	return out, nil
}

// coerceValue converts a string value to the appropriate Go type based on
// the declared cty.Type. Scalar-only — list / map / object inputs come
// through the vars-file path (IMPL-0009 Phase D) and never hit this
// helper.
func coerceValue(raw string, varType cty.Type) (any, error) {
	switch {
	case varType.Equals(cty.Bool):
		return strconv.ParseBool(raw)
	case varType.Equals(cty.Number):
		return strconv.Atoi(raw)
	default:
		return raw, nil
	}
}

// zeroValue returns the zero value for a scalar cty.Type. Structured
// types resolve through the vars-file path and never need this helper
// in Phase B.
func zeroValue(varType cty.Type) any {
	switch {
	case varType.Equals(cty.Bool):
		return false
	case varType.Equals(cty.Number):
		return 0
	default:
		return ""
	}
}
