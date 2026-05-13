package config

// Helpers used by loader_hcl.go to convert hcldec-produced cty.Values into
// the plain Go types used by Blueprint and Registry, and to extract source
// text from HCL expressions where the loader needs the raw template
// source rather than the evaluated value.

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// ctyToString returns the underlying string of a cty.String value, or the
// empty string for a null/unknown input.
func ctyToString(v cty.Value) string {
	if v.IsNull() || !v.IsKnown() {
		return ""
	}

	return v.AsString()
}

// ctyToBool returns the underlying bool of a cty.Bool value, or false for
// a null/unknown input.
func ctyToBool(v cty.Value) bool {
	if v.IsNull() || !v.IsKnown() {
		return false
	}

	return v.True()
}

// ctyToStringSlice returns the elements of a cty.List(cty.String) value as
// a []string. Returns nil for null/unknown inputs.
func ctyToStringSlice(v cty.Value) []string {
	if v.IsNull() || !v.IsKnown() {
		return nil
	}

	out := make([]string, 0, v.LengthInt())
	for it := v.ElementIterator(); it.Next(); {
		_, item := it.Element()
		out = append(out, ctyToString(item))
	}

	return out
}

// ctyToStringMap returns the entries of a cty.Map(cty.String) value as a
// map[string]string. Returns nil for null/unknown inputs.
func ctyToStringMap(v cty.Value) map[string]string {
	if v.IsNull() || !v.IsKnown() {
		return nil
	}

	out := make(map[string]string, v.LengthInt())
	for it := v.ElementIterator(); it.Next(); {
		k, val := it.Element()
		out[ctyToString(k)] = ctyToString(val)
	}

	return out
}

// ctyToManagedFiles converts a cty tuple of objects (produced by
// BlockListSpec for managed_file blocks) into the []ManagedFile slice
// shape used by SyncConfig.
func ctyToManagedFiles(v cty.Value) []ManagedFile {
	if v.IsNull() || !v.IsKnown() {
		return nil
	}

	out := make([]ManagedFile, 0, v.LengthInt())
	for it := v.ElementIterator(); it.Next(); {
		_, item := it.Element()
		out = append(out, ManagedFile{
			Path:     ctyToString(item.GetAttr("path")),
			Strategy: ctyToString(item.GetAttr("strategy")),
		})
	}

	return out
}

// evalAttrAsString evaluates an attribute's expression against an empty
// hcl.EvalContext and returns the result as a string. Used for
// non-templated attributes (variable.description, variable.type, etc.)
// that don't reference user-bound variables.
func evalAttrAsString(attr *hcl.Attribute) (string, error) {
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return "", fmt.Errorf("%s", diags.Error())
	}

	conv, err := convert.Convert(val, cty.String)
	if err != nil {
		return "", err
	}

	return conv.AsString(), nil
}

// evalAttrAsBool evaluates an attribute as a bool against an empty
// EvalContext.
func evalAttrAsBool(attr *hcl.Attribute) (bool, error) {
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return false, fmt.Errorf("%s", diags.Error())
	}

	conv, err := convert.Convert(val, cty.Bool)
	if err != nil {
		return false, err
	}

	return conv.True(), nil
}

// evalAttrAsStringSlice evaluates an attribute as list(string).
func evalAttrAsStringSlice(attr *hcl.Attribute) ([]string, error) {
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s", diags.Error())
	}

	conv, err := convert.Convert(val, cty.List(cty.String))
	if err != nil {
		return nil, err
	}

	return ctyToStringSlice(conv), nil
}

// sourceText returns the source text of an HCL expression, with the
// outer quotes stripped if the expression is a quoted string template.
// Used to capture variable.default and variable.validate as the raw
// template source for the prompt renderer.
func sourceText(expr hcl.Expression, src []byte) string {
	rng := expr.Range()
	if rng.Start.Byte < 0 || rng.End.Byte > len(src) || rng.Start.Byte >= rng.End.Byte {
		return ""
	}

	raw := string(src[rng.Start.Byte:rng.End.Byte])
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		return raw[1 : len(raw)-1]
	}

	return raw
}

// exprSourceText returns the source text of an HCL expression by slicing
// the file source bytes against the expression's range. Used for
// condition.when where we keep the original source alongside the parsed
// expression for diagnostics and lockfile snapshots.
func exprSourceText(expr hcl.Expression, src []byte) string {
	rng := expr.Range()
	if rng.Start.Byte < 0 || rng.End.Byte > len(src) || rng.Start.Byte >= rng.End.Byte {
		return ""
	}

	return string(src[rng.Start.Byte:rng.End.Byte])
}
