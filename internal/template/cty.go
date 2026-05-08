package template

import (
	"fmt"

	"github.com/zclconf/go-cty/cty"
)

// ToCtyValues converts a Go variable map (as produced by the prompt package)
// into a cty.Value map suitable for the HCL2 EvalContext or the Renderer
// interface.
//
// Supported scalar types: string, bool, int, int32, int64, float32, float64.
// Nil entries map to cty.NilVal. Other types yield an error.
func ToCtyValues(raw map[string]any) (map[string]cty.Value, error) {
	if raw == nil {
		return map[string]cty.Value{}, nil
	}

	out := make(map[string]cty.Value, len(raw))

	for k, v := range raw {
		val, err := toCty(v)
		if err != nil {
			return nil, fmt.Errorf("variable %q: %w", k, err)
		}

		out[k] = val
	}

	return out, nil
}

// FromCtyValues converts a cty.Value map back into a Go variable map suitable
// for text/template execution. Used by the TextRenderer adapter during the
// HCL2 transition; deleted once the cutover (Phase C) lands.
func FromCtyValues(vals map[string]cty.Value) map[string]any {
	out := make(map[string]any, len(vals))

	for k, v := range vals {
		out[k] = fromCty(v)
	}

	return out
}

func toCty(v any) (cty.Value, error) {
	switch x := v.(type) {
	case nil:
		return cty.NilVal, nil
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
		f, _ := v.AsBigFloat().Float64()

		if i, acc := v.AsBigFloat().Int64(); acc == 0 {
			return i
		}

		return f
	default:
		return nil
	}
}
