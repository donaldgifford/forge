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
//   - `variable.default` and `variable.validate` are template strings
//     that may reference later-bound variables. We capture them as raw
//     source bytes (with outer quotes stripped) so the prompt renderer
//     can re-parse them with the right eval context later.
//
// For both, the loader uses hcl.Body.PartialContent to peel the
// expression-bearing blocks off the top-level body, then runs hcldec
// on the remaining body for everything else.

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

	if err := validateBlueprintFields(bp); err != nil {
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

	if err := validateRegistryFields(reg); err != nil {
		return nil, fmt.Errorf("validating registry %s: %w", path, err)
	}

	return reg, nil
}

// decodeBlueprintBody splits the body into eager content (decoded via
// hcldec) and lazy blocks (variable, condition, rename — decoded by
// hand to preserve expressions, source text, and templated labels).
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
		v, err := decodeVariableBlock(block, src)
		if err != nil {
			return nil, err
		}

		bp.Variables = append(bp.Variables, v)
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

// decodeVariableBlock parses a `variable "name" { ... }` block. Eager
// attributes (description/type/required/choices) evaluate against an
// empty context; default and validate are stored as raw source text so
// the prompt renderer can re-render them with the user's variables.
func decodeVariableBlock(block *hcl.Block, src []byte) (Variable, error) {
	v := Variable{Name: block.Labels[0]}

	content, diags := block.Body.Content(variableBlockBodySchema)
	if diags.HasErrors() {
		return v, fmt.Errorf("decoding variable %q: %s", v.Name, diags.Error())
	}

	if attr, ok := content.Attributes["description"]; ok {
		s, err := evalAttrAsString(attr)
		if err != nil {
			return v, fmt.Errorf("variable %q description: %w", v.Name, err)
		}

		v.Description = s
	}

	if attr, ok := content.Attributes["type"]; ok {
		s, err := evalAttrAsString(attr)
		if err != nil {
			return v, fmt.Errorf("variable %q type: %w", v.Name, err)
		}

		v.Type = s
	}

	if attr, ok := content.Attributes["required"]; ok {
		b, err := evalAttrAsBool(attr)
		if err != nil {
			return v, fmt.Errorf("variable %q required: %w", v.Name, err)
		}

		v.Required = b
	}

	if attr, ok := content.Attributes["choices"]; ok {
		ss, err := evalAttrAsStringSlice(attr)
		if err != nil {
			return v, fmt.Errorf("variable %q choices: %w", v.Name, err)
		}

		v.Choices = ss
	}

	if attr, ok := content.Attributes["default"]; ok {
		v.Default = sourceText(attr.Expr, src)
	}

	if attr, ok := content.Attributes["validate"]; ok {
		v.Validate = sourceText(attr.Expr, src)
	}

	return v, nil
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

	bps := val.GetAttr("blueprints")
	if bps.IsNull() {
		return
	}

	it := bps.ElementIterator()

	for it.Next() {
		_, item := it.Element()

		entry := BlueprintEntry{
			Name:        ctyToString(item.GetAttr("name")),
			Path:        ctyToString(item.GetAttr("path")),
			Description: ctyToString(item.GetAttr("description")),
			Tags:        ctyToStringSlice(item.GetAttr("tags")),
		}
		reg.Blueprints = append(reg.Blueprints, entry)
	}
}
