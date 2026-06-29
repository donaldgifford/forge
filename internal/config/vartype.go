package config

// vartype.go parses a `variable.type` HCL expression from blueprint.hcl
// into a cty.Type using the cty type-expression grammar via
// hashicorp/hcl/v2/ext/typeexpr. It layers forge-specific concerns on
// top of typeexpr: rejection of unsupported forms (tuple, set), the
// `int`-as-alias-for-`number` deprecation warning, and continued
// acceptance of the legacy quoted-string forms ("string", "bool",
// "int") during the v0.7 transition window. Implements DESIGN-0006
// Phase A.
//
// Phase B note: the returned hcl.Diagnostics may contain DiagWarning
// entries (today: the `int` deprecation notice). The caller in
// internal/config/loader_hcl.go should split warnings out and surface
// them via ui.Warningf before converting any DiagError entries to a
// Go error at the LoadBlueprint boundary.

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// migrationAnchor is the docs/MIGRATION.md section that explains the
// v0.7 variable-type changes. Surfaced in every error/warning so
// users have a single canonical pointer for the upgrade path.
const migrationAnchor = "docs/MIGRATION.md#variable-type-system-upgrade-v07"

// ParseVariableType resolves a `variable.type` HCL expression to a
// cty.Type.
//
// Accepted forms:
//   - Bareword scalars: string, bool, number, int (int is
//     deprecated; emits a DiagWarning).
//   - Bareword constructors: list(T), map(T), object({k = T, ...}),
//     including nested object types.
//   - Legacy quoted-string scalars during the v0.7 transition:
//     "string", "bool", "int", "number".
//
// Rejected forms:
//   - tuple([...]) and set(T) — not supported in v0.7.
//   - Types containing optional() attributes — rejected by
//     typeexpr.Type itself (which forwards the diagnostic).
//   - any / cty.DynamicPseudoType — rejected by typeexpr.Type
//     itself.
//   - "choice" (legacy) — rejected with a pointer at the
//     validation-block migration pattern.
//
// The returned hcl.Diagnostics may contain DiagWarning entries
// (today: only the `int` deprecation notice). Callers should split
// warnings from errors via diags.HasErrors() before treating the
// result as an error.
func ParseVariableType(varName string, expr hcl.Expression) (cty.Type, hcl.Diagnostics) {
	// 1. Legacy quoted-string form. If the expression is a literal
	//    string, we handle it ourselves — typeexpr.Type expects
	//    type *expressions* (traversals, function calls), not
	//    string scalars, and would otherwise produce a confusing
	//    error.
	if ty, diags, handled := legacyQuotedTypeFromExpr(varName, expr); handled {
		return ty, diags
	}

	// 2. Detect and resolve the bare `int` keyword directly:
	//    typeexpr.Type only recognises `bool`, `number`, `string`,
	//    so `int` would otherwise produce a confusing error.
	//    We resolve it to cty.Number ourselves and attach the
	//    deprecation warning.
	if isBarewordInt(expr) {
		return cty.Number, hcl.Diagnostics{intDeprecationDiag(varName, expr.Range())}
	}

	// 3. Delegate to typeexpr for the actual parse. Use Type (not
	//    TypeConstraint) so `any` is rejected — forge variables
	//    must have concrete declared types.
	ty, diags := typeexpr.Type(expr)
	if diags.HasErrors() {
		return cty.NilType, diags
	}

	// 4. Reject unsupported shapes (tuple, set) that typeexpr.Type
	//    happily produces but forge does not yet support. The check
	//    walks the type tree so a nested `object({k = set(string)})`
	//    is also caught.
	if rejectDiags := rejectUnsupportedTypes(varName, ty, expr.Range()); rejectDiags.HasErrors() {
		return cty.NilType, rejectDiags
	}

	return ty, nil
}

// legacyQuotedTypeFromExpr handles the v0.7 transition shim for the
// existing `type = "string"` / `"bool"` / `"int"` / `"number"`
// declarations and rejects the now-removed `"choice"` form.
//
// Returns (type, diags, true) when the expression is a literal
// string and we recognise its value; (cty.NilType, nil, false)
// otherwise so the caller continues to the bareword parser.
func legacyQuotedTypeFromExpr(varName string, expr hcl.Expression) (cty.Type, hcl.Diagnostics, bool) {
	s, ok := literalStringFromExpr(expr)
	if !ok {
		return cty.NilType, nil, false
	}

	switch s {
	case "string":
		return cty.String, nil, true
	case "bool":
		return cty.Bool, nil, true
	case "number":
		return cty.Number, nil, true
	case "int":
		return cty.Number, hcl.Diagnostics{intDeprecationDiag(varName, expr.Range())}, true
	case "choice":
		return cty.NilType, hcl.Diagnostics{choiceRemovedDiag(varName, expr.Range())}, true
	}

	return cty.NilType, nil, false
}

// literalStringFromExpr reports whether expr is a string literal
// and, if so, returns its value. Handles both raw
// *hclsyntax.LiteralValueExpr and the wrapping *hclsyntax.TemplateExpr
// that HCL produces for any double-quoted source string (since those
// may carry ${...} interpolations).
func literalStringFromExpr(expr hcl.Expression) (string, bool) {
	if lit, ok := expr.(*hclsyntax.LiteralValueExpr); ok && lit.Val.Type() == cty.String {
		return lit.Val.AsString(), true
	}

	tmpl, ok := expr.(*hclsyntax.TemplateExpr)
	if !ok || !tmpl.IsStringLiteral() {
		return "", false
	}

	val, diags := tmpl.Value(nil)
	if diags.HasErrors() || val.Type() != cty.String {
		return "", false
	}

	return val.AsString(), true
}

// isBarewordInt reports whether expr is the single-identifier
// traversal `int` (as opposed to `number`, `string`, etc.). Used to
// emit the int-deprecation warning before delegating to typeexpr.
func isBarewordInt(expr hcl.Expression) bool {
	trav, ok := expr.(*hclsyntax.ScopeTraversalExpr)
	if !ok {
		return false
	}

	if len(trav.Traversal) != 1 {
		return false
	}

	root, ok := trav.Traversal[0].(hcl.TraverseRoot)
	if !ok {
		return false
	}

	return root.Name == "int"
}

// intDeprecationDiag builds the DiagWarning emitted when an author
// uses `int` (bareword or quoted) instead of the canonical `number`.
// Phase A.4 / DESIGN-0006 OQ-6.
func intDeprecationDiag(varName string, rng hcl.Range) *hcl.Diagnostic {
	return &hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "variable type `int` is deprecated",
		Detail: "variable " + quoteName(varName) + ": use `type = number` instead of `type = int`. " +
			"The `int` form continues to work in v0.7 but may be removed in a future release. " +
			"See " + migrationAnchor + ".",
		Subject: rng.Ptr(),
	}
}

// choiceRemovedDiag builds the DiagError emitted when an author uses
// the removed `type = "choice"` form. Points at the validation-block
// migration pattern.
func choiceRemovedDiag(varName string, rng hcl.Range) *hcl.Diagnostic {
	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "variable type `choice` is no longer supported",
		Detail: "variable " + quoteName(varName) + ": the `choice` type was removed in v0.7. " +
			"Re-declare as `type = string` plus a `validation { condition = contains([...], var." +
			varName + ") error_message = \"...\" }` block. " +
			"See " + migrationAnchor + ".",
		Subject: rng.Ptr(),
	}
}

// rejectUnsupportedTypes walks ty and appends a DiagError for each
// tuple or set encountered — these are valid cty types that
// typeexpr.Type happily produces but forge does not yet support.
// Walking handles nested cases like `object({k = set(string)})`.
func rejectUnsupportedTypes(varName string, ty cty.Type, rng hcl.Range) hcl.Diagnostics {
	var diags hcl.Diagnostics

	walkTypeForRejection(varName, ty, rng, &diags)

	return diags
}

func walkTypeForRejection(varName string, ty cty.Type, rng hcl.Range, diags *hcl.Diagnostics) {
	switch {
	case ty.IsTupleType():
		*diags = append(*diags, unsupportedTypeDiag(varName, "tuple", rng))
	case ty.IsSetType():
		*diags = append(*diags, unsupportedTypeDiag(varName, "set", rng))
		walkTypeForRejection(varName, ty.ElementType(), rng, diags)
	case ty.IsListType(), ty.IsMapType():
		walkTypeForRejection(varName, ty.ElementType(), rng, diags)
	case ty.IsObjectType():
		for _, atty := range ty.AttributeTypes() {
			walkTypeForRejection(varName, atty, rng, diags)
		}
	}
}

// unsupportedTypeDiag builds the DiagError emitted for a tuple or
// set type. Points at the supported type table in REFERENCE.md.
func unsupportedTypeDiag(varName, typeName string, rng hcl.Range) *hcl.Diagnostic {
	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "variable type `" + typeName + "` is not supported",
		Detail: "variable " + quoteName(varName) + ": the `" + typeName + "` type is not in the v0.7 " +
			"supported set (string, bool, number, list(T), map(T), object({...})). " +
			"See docs/REFERENCE.md for the canonical type table.",
		Subject: rng.Ptr(),
	}
}

// quoteName wraps a variable name in double quotes for diagnostic
// messages. Kept as a helper so the quoting style stays consistent
// across this file's diagnostic builders.
func quoteName(s string) string {
	return `"` + s + `"`
}

// ObjectFieldOrder walks expr looking for the top-level object
// constructor and returns its attribute names in author-declared
// source order. Returns nil when expr isn't an object constructor
// (scalar, list/map/tuple, or a `object("…")` form that didn't parse
// cleanly).
//
// cty.Object's AttributeTypes() iteration order is hash-based, so
// the loader captures the source order from the underlying
// hclsyntax.ObjectConsExpr — necessary for IMPL-0009 Phase E's
// object-unfold prompt UX (fields prompt in declaration order).
//
// Nested object types only contribute their own level; the caller
// can recurse into the nested cty.Object via Variable.Type and apply
// the same helper to nested defaults if needed.
func ObjectFieldOrder(expr hcl.Expression) []string {
	call, ok := expr.(*hclsyntax.FunctionCallExpr)
	if !ok || call.Name != "object" || len(call.Args) != 1 {
		return nil
	}

	objCons, ok := call.Args[0].(*hclsyntax.ObjectConsExpr)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(objCons.Items))

	for _, item := range objCons.Items {
		name, ok := objectConsKeyName(item.KeyExpr)
		if !ok {
			continue
		}

		out = append(out, name)
	}

	return out
}

// objectConsKeyName extracts the attribute name from an object
// constructor key expression. HCL wraps bareword keys as
// ObjectConsKeyExpr → ScopeTraversalExpr; quoted keys arrive as
// LiteralValueExpr / TemplateExpr. Returns "" + false for shapes
// the prompt-unfold path can't render.
func objectConsKeyName(key hcl.Expression) (string, bool) {
	if wrapper, ok := key.(*hclsyntax.ObjectConsKeyExpr); ok {
		key = wrapper.Wrapped
	}

	if trav, ok := key.(*hclsyntax.ScopeTraversalExpr); ok && len(trav.Traversal) == 1 {
		if root, ok := trav.Traversal[0].(hcl.TraverseRoot); ok {
			return root.Name, true
		}
	}

	if s, ok := literalStringFromExpr(key); ok {
		return s, true
	}

	return "", false
}
