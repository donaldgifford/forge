// Package prompt handles interactive variable collection for forge blueprints.
package prompt

import (
	"fmt"
	"regexp"
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

// CollectVariables resolves all blueprint variables using overrides, defaults, and
// optional interactive prompting. Variables are processed in declaration order so
// that later defaults can reference earlier variable values.
//
// If useDefaults is true, all variables use their default values without prompting.
// The promptFn callback is called for variables that need interactive input; pass nil
// to skip interactive prompting (useful in tests or CI with --defaults).
func CollectVariables(
	vars []config.Variable,
	overrides map[string]string,
	useDefaults bool,
	promptFn PromptFn,
) (map[string]any, error) {
	result := make(map[string]any, len(vars))

	for i := range vars {
		v := &vars[i]

		val, err := resolveVariable(v, overrides, result, useDefaults, promptFn)
		if err != nil {
			return nil, err
		}

		result[v.Name] = val
	}

	return result, nil
}

// resolveVariable resolves a single variable value through the override → default → prompt chain.
func resolveVariable(
	v *config.Variable,
	overrides map[string]string,
	current map[string]any,
	useDefaults bool,
	promptFn PromptFn,
) (any, error) {
	// Check for CLI override first.
	if raw, ok := overrides[v.Name]; ok {
		return resolveFromOverride(raw, v)
	}

	// Render the default value as a template (it can reference earlier variables).
	defaultVal, err := renderDefault(v.Default, current)
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

// resolveFromOverride validates and coerces an override value.
func resolveFromOverride(raw string, v *config.Variable) (any, error) {
	if err := validateValue(raw, v); err != nil {
		return nil, fmt.Errorf("override for %q failed validation: %w", v.Name, err)
	}

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

// resolveFromPrompt calls the prompt function and validates the result.
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

	if err := validateValue(raw, v); err != nil {
		return nil, fmt.Errorf("variable %q failed validation: %w", v.Name, err)
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

// coerceValue converts a string value to the appropriate Go type based on the variable type.
func coerceValue(raw, varType string) (any, error) {
	switch varType {
	case "bool":
		return strconv.ParseBool(raw)
	case "int":
		return strconv.Atoi(raw)
	case "string", "choice", "":
		return raw, nil
	default:
		return raw, nil
	}
}

// zeroValue returns the zero value for a variable type.
func zeroValue(varType string) any {
	switch varType {
	case "bool":
		return false
	case "int":
		return 0
	default:
		return ""
	}
}

// validateValue checks a string value against the variable's validation regex.
func validateValue(raw string, v *config.Variable) error {
	if v.Validate == "" {
		return nil
	}

	re, err := regexp.Compile(v.Validate)
	if err != nil {
		return fmt.Errorf("invalid validation regex %q: %w", v.Validate, err)
	}

	if !re.MatchString(raw) {
		return fmt.Errorf("value %q does not match pattern %q", raw, v.Validate)
	}

	return nil
}
