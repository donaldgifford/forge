package lockfile

import (
	"fmt"
	"strconv"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/donaldgifford/forge/internal/config"
)

// ToCtyValues converts a raw lockfile variable map (loaded from disk) into a
// cty.Value map, normalising values against the declared variable types from
// the blueprint. The declared type wins over the on-disk shape — for example,
// a value of `false` on disk becomes cty.False whether it was stored as a
// bool or the string "false".
//
// Variables not declared in vars fall back to runtime-type inference. Missing
// raw values produce cty.NullVal of the declared type.
func ToCtyValues(raw map[string]any, vars []config.Variable) (map[string]cty.Value, error) {
	out := make(map[string]cty.Value, len(vars))
	declared := make(map[string]cty.Type, len(vars))

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

// FromCtyValues converts a cty.Value map back into a Go map suitable for
// on-disk marshaling. Used by create when writing the lockfile.
func FromCtyValues(vals map[string]cty.Value) map[string]any {
	out := make(map[string]any, len(vals))

	for k, v := range vals {
		out[k] = fromCty(v)
	}

	return out
}

// convertValue coerces a raw Go value to a cty.Value of declaredType.
// Falls back to runtime-type inference when declaredType is cty.NilType
// (variable not declared in the blueprint, e.g. legacy lockfile entry).
//
// IMPL-0009 Phase D: a cty.Value input (carried in-memory through the
// resolution chain for structured-typed overrides and vars-file
// values) flows through with a final cty.Convert pass to enforce the
// declared shape.
func convertValue(v any, declaredType cty.Type) (cty.Value, error) {
	if v == nil {
		return nullValueForType(declaredType), nil
	}

	if ctyVal, ok := v.(cty.Value); ok {
		if declaredType == cty.NilType {
			return ctyVal, nil
		}

		return convert.Convert(ctyVal, declaredType)
	}

	if declaredType == cty.NilType {
		return inferValue(v)
	}

	switch {
	case declaredType.Equals(cty.String):
		return cty.StringVal(toString(v)), nil
	case declaredType.Equals(cty.Bool):
		b, err := toBool(v)
		if err != nil {
			return cty.NilVal, err
		}

		return cty.BoolVal(b), nil
	case declaredType.Equals(cty.Number):
		i, err := toInt(v)
		if err != nil {
			return cty.NilVal, err
		}

		return cty.NumberIntVal(i), nil
	default:
		// Structured types (list/map/object) — defer to inferValue
		// and let cty.Convert do the structural coercion against the
		// declared shape. Lockfiles produced by IMPL-0009-aware
		// writers already round-trip cleanly; the convert pass is for
		// pre-v0.7 entries that may shape-mismatch.
		inferred, err := inferValue(v)
		if err != nil {
			return cty.NilVal, err
		}

		return convert.Convert(inferred, declaredType)
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

func nullValueForType(declaredType cty.Type) cty.Value {
	if declaredType == cty.NilType {
		return cty.NullVal(cty.String)
	}

	return cty.NullVal(declaredType)
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
		// Structured types (object/list/map) stay as a cty.Value so
		// the emit path (ctyForVariableValue) can hand them straight
		// to hclwrite without round-tripping through Go shapes. The
		// load side hand-decodes the lockfile's variables block back
		// to cty.Value already, so symmetry holds.
		return v
	}
}
