package config

// HCL decoding strategy
//
// The HCL config loaders (LoadBlueprintHCL, LoadRegistryHCL) decode
// blueprint.hcl and registry.hcl into the same Blueprint / Registry
// structs the YAML loaders produce.
//
// Decoding goes through hashicorp/hcl/v2/hcldec rather than gohcl
// (struct-tag-based decoding) or hand-rolled hclsyntax walks.
//
// hcldec.ObjectSpec is a declarative spec sitting next to the loader
// (see hcldec_spec.go). It has three properties this codebase wants:
//
//  1. The Go structs in blueprint.go / registry.go stay encoding-agnostic.
//     No second set of `hcl:"..."` tags alongside the existing YAML tags
//     during the side-by-side window; no leftover tags after the YAML
//     loader is deleted in Phase C.
//
//  2. Diagnostics carry source ranges. A malformed `condition.when` or
//     a missing required attribute surfaces with file/line/col, the same
//     way the template renderer's hcl.Diagnostics do. gohcl loses some
//     of this fidelity by mapping errors back through reflection.
//
//  3. Block-typed schemas (variable "name" { ... }) decode without
//     intermediate wrapper types. gohcl requires a slice of a struct
//     with the right hcl tags; the spec form is a single ObjectSpec
//     entry.
//
// The decision is captured in DESIGN-0004 / IMPL-0005 OQ-1.
//
// hcldec doesn't fit two cases natively, both of which need to preserve
// expressions at load time:
//
//   - `condition.when` is an hcl.Expression that stays unevaluated until
//     create/sync time (per OQ-7). The loader pulls it from the block
//     body via hcl.Body.Content with a small inline schema.
//
//   - `variable.type` and `variable.default` are HCL expressions that
//     need lazy access: `type` parses through ParseVariableType (which
//     understands forge's type surface plus the v0.7 transition shims),
//     and `default` is kept as an hcl.Expression so it can evaluate
//     against the resolved-variable scope at prompt time. The raw
//     source bytes for both are also retained for diagnostics, lockfile
//     snapshots, and `forge info --output json` (IMPL-0009 OQ-6).
//
//   - Each `variable` block can also carry one or more nested
//     `validation { condition, error_message }` blocks; the loader
//     keeps `condition` as an unevaluated hcl.Expression for Phase C's
//     post-resolution evaluation flow.
//
// For all of these, the loader uses hcl.Body.PartialContent to peel
// the expression-bearing blocks off the top-level body, then runs
// hcldec on the remaining body for everything else.

import (
	"errors"
	"fmt"
	"maps"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// LoadBlueprintHCL reads and parses a blueprint.hcl file from the given
// path, returning the same Blueprint shape LoadBlueprint produces from
// YAML.
func LoadBlueprintHCL(path string) (*Blueprint, error) {
	src, err := os.ReadFile(path) //nolint:gosec // path is provided by the caller; this is a scaffolding tool that reads user-specified config files
	if err != nil {
		return nil, fmt.Errorf("reading blueprint file %s: %w", path, err)
	}

	parser := hclparse.NewParser()

	file, diags := parser.ParseHCL(src, path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing blueprint file %s: %s", path, diags.Error())
	}

	bp, err := decodeBlueprintBody(file.Body, src)
	if err != nil {
		return nil, fmt.Errorf("decoding blueprint file %s: %w", path, err)
	}

	if err := ValidateBlueprint(bp); err != nil {
		return nil, fmt.Errorf("validating blueprint %s: %w", path, err)
	}

	return bp, nil
}

// LoadRegistryHCL reads and parses a registry.hcl file from the given
// path, returning the same Registry shape LoadRegistry produces from
// YAML.
func LoadRegistryHCL(path string) (*Registry, error) {
	src, err := os.ReadFile(path) //nolint:gosec // path is provided by the caller; this is a scaffolding tool that reads user-specified config files
	if err != nil {
		return nil, fmt.Errorf("reading registry file %s: %w", path, err)
	}

	parser := hclparse.NewParser()

	file, diags := parser.ParseHCL(src, path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing registry file %s: %s", path, diags.Error())
	}

	val, diags := hcldec.Decode(file.Body, registrySpec, nil)
	if diags.HasErrors() {
		return nil, fmt.Errorf("decoding registry file %s: %s", path, diags.Error())
	}

	reg := &Registry{}
	assignRegistryFromCty(val, reg)

	if err := ValidateRegistry(reg); err != nil {
		return nil, fmt.Errorf("validating registry %s: %w", path, err)
	}

	return reg, nil
}

// decodeBlueprintBody splits the body into eager content (decoded via
// hcldec) and lazy blocks (variable, condition, rename — decoded by
// hand to preserve expressions, source text, and templated labels).
// Returns a Blueprint with any non-fatal Deprecation notices attached
// for the caller to surface via ui.Warningf.
func decodeBlueprintBody(body hcl.Body, src []byte) (*Blueprint, error) {
	lazyContent, eagerBody, diags := body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "variable", LabelNames: []string{"name"}},
			{Type: "condition"},
			{Type: "rename"},
		},
	})
	if diags.HasErrors() {
		return nil, fmt.Errorf("splitting blueprint body: %s", diags.Error())
	}

	eagerVal, diags := hcldec.Decode(eagerBody, blueprintEagerSpec, nil)
	if diags.HasErrors() {
		return nil, fmt.Errorf("decoding blueprint attributes: %s", diags.Error())
	}

	bp := &Blueprint{}
	assignEagerBlueprint(eagerVal, bp)

	for _, block := range lazyContent.Blocks.OfType("variable") {
		v, deps, err := decodeVariableBlock(block, src)
		if err != nil {
			return nil, err
		}

		bp.Variables = append(bp.Variables, v)
		bp.Deprecations = append(bp.Deprecations, deps...)
	}

	for _, block := range lazyContent.Blocks.OfType("condition") {
		c, err := decodeConditionBlock(block, src)
		if err != nil {
			return nil, err
		}

		bp.Conditions = append(bp.Conditions, c)
	}

	for _, block := range lazyContent.Blocks.OfType("rename") {
		entries, err := decodeRenameBlock(block, src)
		if err != nil {
			return nil, err
		}

		if bp.Rename == nil {
			bp.Rename = make(map[string]string, len(entries))
		}

		maps.Copy(bp.Rename, entries)
	}

	return bp, nil
}

// decodeRenameBlock parses a `rename { entry { from = "...", to = "..." } ... }`
// block, returning a from→to map. Each entry's from/to comes from raw
// source text (not evaluated) so both can carry templates that
// reference later-bound variables.
func decodeRenameBlock(block *hcl.Block, src []byte) (map[string]string, error) {
	content, diags := block.Body.Content(renameOuterBodySchema)
	if diags.HasErrors() {
		return nil, fmt.Errorf("decoding rename block: %s", diags.Error())
	}

	entries := make(map[string]string, len(content.Blocks))

	for _, entry := range content.Blocks.OfType("entry") {
		from, to, err := decodeRenameEntry(entry, src)
		if err != nil {
			return nil, err
		}

		entries[from] = to
	}

	return entries, nil
}

// decodeRenameEntry pulls from + to from a single `entry` block as
// raw source text.
func decodeRenameEntry(block *hcl.Block, src []byte) (from, to string, err error) {
	content, diags := block.Body.Content(renameEntryBodySchema)
	if diags.HasErrors() {
		return "", "", fmt.Errorf("decoding rename entry: %s", diags.Error())
	}

	fromAttr, ok := content.Attributes["from"]
	if !ok {
		return "", "", errors.New("rename entry: missing required `from` attribute")
	}

	toAttr, ok := content.Attributes["to"]
	if !ok {
		return "", "", errors.New("rename entry: missing required `to` attribute")
	}

	return sourceText(fromAttr.Expr, src), sourceText(toAttr.Expr, src), nil
}

// assignEagerBlueprint copies values from a cty.Object produced by
// hcldec.Decode(blueprintEagerSpec, …) into the Blueprint struct.
func assignEagerBlueprint(val cty.Value, bp *Blueprint) {
	bp.Name = ctyToString(val.GetAttr("name"))
	bp.Description = ctyToString(val.GetAttr("description"))
	bp.Version = ctyToString(val.GetAttr("version"))
	bp.Tags = ctyToStringSlice(val.GetAttr("tags"))

	if defaultsVal := val.GetAttr("defaults"); !defaultsVal.IsNull() {
		bp.Defaults = Defaults{
			Exclude:          ctyToStringSlice(defaultsVal.GetAttr("exclude")),
			OverrideStrategy: ctyToStringMap(defaultsVal.GetAttr("override_strategy")),
		}
	}

	if hooksVal := val.GetAttr("hooks"); !hooksVal.IsNull() {
		bp.Hooks = Hooks{
			PostCreate: ctyToStringSlice(hooksVal.GetAttr("post_create")),
		}
	}

	if syncVal := val.GetAttr("sync"); !syncVal.IsNull() {
		bp.Sync = SyncConfig{
			Ignore:       ctyToStringSlice(syncVal.GetAttr("ignore")),
			ManagedFiles: ctyToManagedFiles(syncVal.GetAttr("managed_files")),
		}
	}
}

// decodeVariableBlock parses a `variable "name" { ... }` block.
//
// The type expression flows through ParseVariableType (vartype.go) so
// the loader gets a resolved cty.Type plus any DiagWarning entries
// (today: the `int`-as-alias-for-`number` notice). The `default`
// attribute is captured as the parsed hcl.Expression itself so the
// resolution flow can evaluate it lazily against the resolved-variable
// scope at create/sync time (DESIGN-0006 Phase C).
//
// Legacy `choices = [...]` and `validate = "regex"` attributes are
// rejected with an actionable error pointing at MIGRATION.md per
// IMPL-0009 OQ-4; both fields were removed in favour of the
// `validation { condition, error_message }` nested block.
//
// Returns the populated Variable, any non-fatal Deprecation notices,
// and a fatal error if the block fails to decode.
func decodeVariableBlock(block *hcl.Block, src []byte) (Variable, []Deprecation, error) {
	v := Variable{Name: block.Labels[0]}

	if err := rejectLegacyVariableAttrs(block.Body, v.Name); err != nil {
		return v, nil, err
	}

	content, diags := block.Body.Content(variableBlockBodySchema)
	if diags.HasErrors() {
		return v, nil, fmt.Errorf("decoding variable %q: %s", v.Name, diags.Error())
	}

	if attr, ok := content.Attributes["description"]; ok {
		s, err := evalAttrAsString(attr)
		if err != nil {
			return v, nil, fmt.Errorf("variable %q description: %w", v.Name, err)
		}

		v.Description = s
	}

	deps, err := decodeVariableType(content.Attributes["type"], &v, src)
	if err != nil {
		return v, nil, err
	}

	if attr, ok := content.Attributes["required"]; ok {
		b, evalErr := evalAttrAsBool(attr)
		if evalErr != nil {
			return v, nil, fmt.Errorf("variable %q required: %w", v.Name, evalErr)
		}

		v.Required = b
	}

	if attr, ok := content.Attributes["default"]; ok {
		v.DefaultExpr = attr.Expr
		v.DefaultSource = sourceText(attr.Expr, src)
	}

	for _, vblock := range content.Blocks.OfType("validation") {
		val, vErr := decodeValidationBlock(vblock, v.Name, src)
		if vErr != nil {
			return v, nil, vErr
		}

		v.Validations = append(v.Validations, val)
	}

	return v, deps, nil
}

// decodeVariableType parses the `type` attribute through
// ParseVariableType, separating DiagWarning entries (returned as
// Deprecation notices) from DiagError entries (returned as a Go error).
// A missing `type` attribute is treated as a fatal error to match the
// pre-IMPL-0009 contract.
func decodeVariableType(attr *hcl.Attribute, v *Variable, src []byte) ([]Deprecation, error) {
	if attr == nil {
		return nil, fmt.Errorf("variable %q: type is required", v.Name)
	}

	ty, diags := ParseVariableType(v.Name, attr.Expr)
	if diags.HasErrors() {
		return nil, fmt.Errorf("variable %q type: %s", v.Name, diags.Error())
	}

	v.Type = ty
	v.TypeSource = sourceText(attr.Expr, src)
	v.TypeFieldOrder = ObjectFieldOrder(attr.Expr)

	return diagsToDeprecations(v.Name, diags), nil
}

// decodeValidationBlock parses a single `validation { condition,
// error_message }` block. The condition stays as an unevaluated
// hcl.Expression so it can run against the resolved-variable scope at
// create/sync time; error_message is captured as a static string
// (DESIGN-0006 OQ-3 — template interpolation is a v0.8+ concern).
func decodeValidationBlock(block *hcl.Block, varName string, _ []byte) (Validation, error) {
	val := Validation{DefRange: block.DefRange}

	content, diags := block.Body.Content(validationBlockBodySchema)
	if diags.HasErrors() {
		return val, fmt.Errorf("variable %q validation: %s", varName, diags.Error())
	}

	val.Condition = content.Attributes["condition"].Expr

	msg, err := evalAttrAsString(content.Attributes["error_message"])
	if err != nil {
		return val, fmt.Errorf("variable %q validation error_message: %w", varName, err)
	}

	val.ErrorMessage = msg

	return val, nil
}

// rejectLegacyVariableAttrs scans the raw body for the removed
// `choices` and `validate` attributes before hcldec runs, so the
// error surfaces with the original migration-pointer wording rather
// than the generic "unsupported argument" diagnostic that hcldec
// would produce. Pattern matches the v0.5/v0.6 rejection style
// referenced in IMPL-0009 OQ-4.
//
// JustAttributes' diagnostics are intentionally ignored — a body with
// nested blocks (the `validation { ... }` block) produces an
// "unexpected block" diagnostic that the subsequent Content() call
// surfaces with a proper diagnostic; this helper only cares about
// presence of the two removed attribute names.
func rejectLegacyVariableAttrs(body hcl.Body, varName string) error {
	attrs, _ := body.JustAttributes() //nolint:errcheck // see godoc above — only attribute presence matters here

	if attr, ok := attrs["choices"]; ok {
		return fmt.Errorf(
			"variable %q: the `choices` field was removed in v0.7; re-declare as "+
				"`type = string` plus a `validation { condition = contains([...], var.%s) "+
				"error_message = \"...\" }` block (see %s at %s)",
			varName, varName, migrationAnchor, attr.Range.String(),
		)
	}

	if attr, ok := attrs["validate"]; ok {
		return fmt.Errorf(
			"variable %q: the `validate` regex field was removed in v0.7; re-declare "+
				"as a `validation { condition = can(regex(\"...\", var.%s)) "+
				"error_message = \"...\" }` block (see %s at %s)",
			varName, varName, migrationAnchor, attr.Range.String(),
		)
	}

	return nil
}

// diagsToDeprecations converts the DiagWarning entries emitted by
// ParseVariableType into the forge-owned Deprecation shape carried on
// Blueprint.Deprecations. DiagError entries are dropped — the caller
// has already short-circuited on diags.HasErrors() before this runs.
func diagsToDeprecations(varName string, diags hcl.Diagnostics) []Deprecation {
	if len(diags) == 0 {
		return nil
	}

	out := make([]Deprecation, 0, len(diags))

	for _, d := range diags {
		if d.Severity != hcl.DiagWarning {
			continue
		}

		dep := Deprecation{
			Variable: varName,
			Summary:  d.Summary,
			Detail:   d.Detail,
		}
		if d.Subject != nil {
			dep.Subject = *d.Subject
		}

		out = append(out, dep)
	}

	return out
}

// decodeConditionBlock parses a `condition { ... }` block. The `when`
// attribute is captured as the parsed hcl.Expression so it can be
// evaluated lazily at create/sync time without re-parsing.
func decodeConditionBlock(block *hcl.Block, src []byte) (Condition, error) {
	c := Condition{}

	content, diags := block.Body.Content(conditionBlockBodySchema)
	if diags.HasErrors() {
		return c, fmt.Errorf("decoding condition: %s", diags.Error())
	}

	whenAttr, ok := content.Attributes["when"]
	if !ok {
		return c, errors.New("condition: missing required `when` attribute")
	}

	c.When = whenAttr.Expr
	c.WhenSource = exprSourceText(whenAttr.Expr, src)

	if excludeAttr, ok := content.Attributes["exclude"]; ok {
		ss, err := evalAttrAsStringSlice(excludeAttr)
		if err != nil {
			return c, fmt.Errorf("condition exclude: %w", err)
		}

		c.Exclude = ss
	}

	return c, nil
}

// assignRegistryFromCty maps the cty.Object produced by registrySpec
// into a Registry struct.
func assignRegistryFromCty(val cty.Value, reg *Registry) {
	reg.Name = ctyToString(val.GetAttr("name"))
	reg.Description = ctyToString(val.GetAttr("description"))

	if maint := val.GetAttr("maintainers"); !maint.IsNull() {
		for it := maint.ElementIterator(); it.Next(); {
			_, item := it.Element()
			reg.Maintainers = append(reg.Maintainers, Maintainer{
				Name:  ctyToString(item.GetAttr("name")),
				Email: ctyToString(item.GetAttr("email")),
			})
		}
	}

	if def := val.GetAttr("defaults"); !def.IsNull() {
		reg.Defaults = RegistryDefaults{
			SyncStrategy: ctyToString(def.GetAttr("sync_strategy")),
			Managed:      ctyToBool(def.GetAttr("managed")),
		}
	}

	bps := val.GetAttr("blueprints")
	if bps.IsNull() {
		return
	}

	for it := bps.ElementIterator(); it.Next(); {
		_, item := it.Element()
		reg.Blueprints = append(reg.Blueprints, BlueprintEntry{
			Name:         ctyToString(item.GetAttr("name")),
			Path:         ctyToString(item.GetAttr("path")),
			Description:  ctyToString(item.GetAttr("description")),
			Version:      ctyToString(item.GetAttr("version")),
			Tags:         ctyToStringSlice(item.GetAttr("tags")),
			LatestCommit: ctyToString(item.GetAttr("latest_commit")),
		})
	}
}
