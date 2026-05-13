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

import (
	"errors"
)

// errHCLNotImplemented is returned by the HCL loaders until Phase A.3
// wires up hcldec decoding. The sentinel keeps the package compiling
// while A.2 (schema spec) and A.3 (decode) land in their own commits.
var errHCLNotImplemented = errors.New("config: HCL loader not yet implemented (IMPL-0005 Phase A.3)")

// LoadBlueprintHCL reads and parses a blueprint.hcl file from the given
// path, returning the same Blueprint shape LoadBlueprint produces from
// YAML. Implementation lands in IMPL-0005 Phase A.3.
func LoadBlueprintHCL(path string) (*Blueprint, error) {
	_ = path
	return nil, errHCLNotImplemented
}

// LoadRegistryHCL reads and parses a registry.hcl file from the given
// path, returning the same Registry shape LoadRegistry produces from
// YAML. Implementation lands in IMPL-0005 Phase A.3.
func LoadRegistryHCL(path string) (*Registry, error) {
	_ = path
	return nil, errHCLNotImplemented
}
