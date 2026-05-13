package config

// hcldec schemas for blueprint.hcl and registry.hcl.
//
// The specs in this file cover the *eagerly-evaluable* parts of the
// blueprint/registry schemas — attributes and blocks whose contents
// fully resolve against an empty hcl.EvalContext at load time.
//
// Two block kinds are intentionally *not* part of the eager specs and
// are decoded by hand in loader_hcl.go:
//
//   - `condition` blocks: `condition.when` is an hcl.Expression that
//     must stay unevaluated until create/sync time, when the user's
//     variables are bound (per OQ-7).
//
//   - `variable` blocks: `variable.default` and `variable.validate`
//     may carry template strings that reference later-bound variables.
//     The loader pulls these as raw source bytes so the prompt
//     renderer can re-parse them when other vars are known. Other
//     variable attributes (description, type, required, choices) are
//     decoded as plain cty values.
//
// Singleton blocks whose contents *do* fully resolve at load time
// (defaults, hooks, sync without its managed_files children) are
// composed into the top-level eager spec via hcldec.BlockSpec.
//
// Registry decoding (registry.hcl) doesn't have any
// expression-preservation requirements, so its spec is straightforward.

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/zclconf/go-cty/cty"
)

// blueprintEagerSpec decodes the parts of blueprint.hcl that resolve
// against an empty EvalContext. Variable, condition, and rename blocks
// are excluded; the loader handles them by hand because their content
// can carry templates that reference later-bound variables.
var blueprintEagerSpec = hcldec.ObjectSpec{
	"name":        &hcldec.AttrSpec{Name: "name", Type: cty.String, Required: true},
	"description": &hcldec.AttrSpec{Name: "description", Type: cty.String},
	"version":     &hcldec.AttrSpec{Name: "version", Type: cty.String},
	"tags":        &hcldec.AttrSpec{Name: "tags", Type: cty.List(cty.String)},
	"defaults":    &hcldec.BlockSpec{TypeName: "defaults", Nested: defaultsSpec},
	"hooks":       &hcldec.BlockSpec{TypeName: "hooks", Nested: hooksSpec},
	"sync":        &hcldec.BlockSpec{TypeName: "sync", Nested: syncSpec},
}

// renameOuterBodySchema is the body schema for the top-level
// `rename { ... }` block. Children are unlabeled `entry { from = ...,
// to = ... }` blocks. We don't put rename keys into HCL block labels
// or attribute names because both positions reject template sequences.
var renameOuterBodySchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "entry"},
	},
}

// renameEntryBodySchema is the body schema for a single `entry { ... }`
// inside a rename block. Both `from` and `to` are captured as raw
// source text by the loader so the keys/values can carry templates
// that reference later-bound variables.
var renameEntryBodySchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "from", Required: true},
		{Name: "to", Required: true},
	},
}

// defaultsSpec covers `defaults { ... }`.
var defaultsSpec = hcldec.ObjectSpec{
	"exclude":           &hcldec.AttrSpec{Name: "exclude", Type: cty.List(cty.String)},
	"override_strategy": &hcldec.AttrSpec{Name: "override_strategy", Type: cty.Map(cty.String)},
}

// hooksSpec covers `hooks { ... }`.
var hooksSpec = hcldec.ObjectSpec{
	"post_create": &hcldec.AttrSpec{Name: "post_create", Type: cty.List(cty.String)},
}

// syncSpec covers `sync { ignore = [...] managed_file "..." { ... } }`.
// ManagedFiles are labeled child blocks captured via hcldec.BlockListSpec.
var syncSpec = hcldec.ObjectSpec{
	"ignore":        &hcldec.AttrSpec{Name: "ignore", Type: cty.List(cty.String)},
	"managed_files": &hcldec.BlockListSpec{TypeName: "managed_file", Nested: managedFileSpec},
}

// managedFileSpec covers the body of a `managed_file "path" { ... }`
// block. The label becomes the ManagedFile.Path field.
var managedFileSpec = hcldec.ObjectSpec{
	"path":     &hcldec.BlockLabelSpec{Index: 0, Name: "path"},
	"strategy": &hcldec.AttrSpec{Name: "strategy", Type: cty.String, Required: true},
}

// variableBlockBodySchema is the body schema for a `variable "name" { ... }`
// block. The loader hand-decodes these because `default` and `validate`
// must be captured as raw source text (they may carry templates that
// reference later-bound variables).
var variableBlockBodySchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "description"},
		{Name: "type"},
		{Name: "required"},
		{Name: "choices"},
		{Name: "default"},
		{Name: "validate"},
	},
}

// conditionBlockBodySchema is the body schema for a `condition { ... }`
// block. The loader hand-decodes because `when` must be captured as an
// unevaluated hcl.Expression (per OQ-7).
var conditionBlockBodySchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "when", Required: true},
		{Name: "exclude"},
	},
}

// registrySpec decodes registry.hcl. Everything in a registry resolves
// at load time, so a single declarative spec is enough — no
// PartialContent dance needed.
var registrySpec = hcldec.ObjectSpec{
	"name":        &hcldec.AttrSpec{Name: "name", Type: cty.String, Required: true},
	"description": &hcldec.AttrSpec{Name: "description", Type: cty.String},
	"blueprints":  &hcldec.BlockListSpec{TypeName: "blueprint", Nested: blueprintEntrySpec},
}

// blueprintEntrySpec covers the body of each registry-level
// `blueprint "name" { ... }` block. The label becomes BlueprintEntry.Name.
var blueprintEntrySpec = hcldec.ObjectSpec{
	"name":        &hcldec.BlockLabelSpec{Index: 0, Name: "name"},
	"path":        &hcldec.AttrSpec{Name: "path", Type: cty.String, Required: true},
	"description": &hcldec.AttrSpec{Name: "description", Type: cty.String},
	"tags":        &hcldec.AttrSpec{Name: "tags", Type: cty.List(cty.String)},
}
