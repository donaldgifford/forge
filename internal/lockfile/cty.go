package lockfile

import (
	"fmt"
	"strconv"

	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/config"
)

// ToCtyValues converts a raw lockfile variable map (loaded from YAML) into a
// cty.Value map, normalising values against the declared variable types from
// the blueprint. The declared type wins over YAML's loose typing — for
// example, a value of `false` in YAML becomes cty.False whether the on-disk
// value parsed as bool or string "false".
//
// Variables not declared in vars fall back to runtime-type inference. Missing
// raw values produce cty.NullVal of the declared type.
func ToCtyValues(raw map[string]any, vars []config.Variable) (map[string]cty.Value, error) {
	out := make(map[string]cty.Value, len(vars))
	declared := make(map[string]string, len(vars))

	for i := range vars {
		declared[vars[i].Name] = vars[i].Type
	}

	for k, v := range raw {
		val, err := convertValue(v, declared[k])
		if err != nil {
			return nil, fmt.Errorf("variable %q: %w", k, err)
		}

		out[k] = val
	}

	// Backfill declared variables that weren't present in raw with null
	// values typed to their declaration. This keeps the EvalContext
	// schema-stable.
	for i := range vars {
		if _, ok := out[vars[i].Name]; ok {
			continue
		}

		out[vars[i].Name] = nullValueForType(vars[i].Type)
	}

	return out, nil
}

// FromCtyValues converts a cty.Value map back into a Go map suitable for YAML
// marshaling. Used by create when writing the lockfile.
func FromCtyValues(vals map[string]cty.Value) map[string]any {
	out := make(map[string]any, len(vals))

	for k, v := range vals {
		out[k] = fromCty(v)
	}

	return out
}

func convertValue(v any, declaredType string) (cty.Value, error) {
	if v == nil {
		return nullValueForType(declaredType), nil
	}

	switch declaredType {
	case "string", "choice":
		return cty.StringVal(toString(v)), nil
	case "bool":
		b, err := toBool(v)
		if err != nil {
			return cty.NilVal, err
		}

		return cty.BoolVal(b), nil
	case "int":
		i, err := toInt(v)
		if err != nil {
			return cty.NilVal, err
		}

		return cty.NumberIntVal(i), nil
	case "":
		return inferValue(v)
	default:
		return inferValue(v)
	}
}

func inferValue(v any) (cty.Value, error) {
	switch x := v.(type) {
	case string:
		return cty.StringVal(x), nil
	case bool:
		return cty.BoolVal(x), nil
	case int:
		return cty.NumberIntVal(int64(x)), nil
	case int32:
		return cty.NumberIntVal(int64(x)), nil
	case int64:
		return cty.NumberIntVal(x), nil
	case float32:
		return cty.NumberFloatVal(float64(x)), nil
	case float64:
		return cty.NumberFloatVal(x), nil
	default:
		return cty.NilVal, fmt.Errorf("unsupported type %T", v)
	}
}

func nullValueForType(declaredType string) cty.Value {
	switch declaredType {
	case "bool":
		return cty.NullVal(cty.Bool)
	case "int":
		return cty.NullVal(cty.Number)
	default:
		return cty.NullVal(cty.String)
	}
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}

	return fmt.Sprintf("%v", v)
}

func toBool(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		return strconv.ParseBool(x)
	default:
		return false, fmt.Errorf("cannot coerce %T to bool", v)
	}
}

func toInt(v any) (int64, error) {
	switch x := v.(type) {
	case int:
		return int64(x), nil
	case int32:
		return int64(x), nil
	case int64:
		return x, nil
	case float64:
		return int64(x), nil
	case string:
		return strconv.ParseInt(x, 10, 64)
	default:
		return 0, fmt.Errorf("cannot coerce %T to int", v)
	}
}

func fromCty(v cty.Value) any {
	if !v.IsKnown() || v.IsNull() {
		return nil
	}

	switch v.Type() {
	case cty.String:
		return v.AsString()
	case cty.Bool:
		return v.True()
	case cty.Number:
		if i, acc := v.AsBigFloat().Int64(); acc == 0 {
			return i
		}

		f, _ := v.AsBigFloat().Float64()

		return f
	default:
		return nil
	}
}
