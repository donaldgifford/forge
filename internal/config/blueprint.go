// Package config handles parsing and validation of blueprint and
// registry config files. HCL is the only supported on-disk format
// (post-IMPL-0005 cutover); the loaders in loader.go reject .yaml
// inputs with a migration-pointer error.
package config

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

// Blueprint represents the configuration of a single blueprint
// (blueprint.hcl).
//
// Deprecations are surfaced by the loader for non-fatal v0.7 transition
// notices (today: `int`-as-alias-for-`number`). Callers should walk this
// slice and surface each entry via ui.Warningf before proceeding. The
// field carries json:"-" so it never leaks into `forge info --output json`.
type Blueprint struct {
	Name         string
	Description  string
	Version      string
	Tags         []string
	Defaults     Defaults
	Variables    []Variable
	Conditions   []Condition
	Hooks        Hooks
	Sync         SyncConfig
	Rename       map[string]string
	Deprecations []Deprecation `json:"-"`
}

// Defaults controls which inherited default files are included or excluded.
type Defaults struct {
	Exclude          []string
	OverrideStrategy map[string]string
}

// Variable represents a user-prompted variable in a blueprint.
//
// `Type` is the resolved cty type parsed from the `type` attribute by
// ParseVariableType (vartype.go). `DefaultExpr` and the validation
// conditions stay as parsed-at-load-time hcl.Expression values so they
// can evaluate lazily against the resolved-variable scope at
// create/sync time — mirroring how `Condition.When` is captured.
// `TypeSource` / `DefaultSource` keep the original source bytes for
// diagnostics and lockfile snapshots.
type Variable struct {
	Name        string
	Description string
	// Type is the resolved cty.Type for this variable (string, bool,
	// number, list(T), map(T), object({...})).
	Type cty.Type `json:"-"`
	// TypeSource is the raw source text of the `type` expression
	// (e.g. `string`, `"int"`, `list(string)`). Kept for diagnostics
	// and round-trippable JSON output per IMPL-0009 OQ-6.
	TypeSource string
	// DefaultExpr is the parsed `default` expression, evaluated
	// lazily against the resolved-variable scope. Nil when the
	// variable declares no default.
	DefaultExpr hcl.Expression `json:"-"`
	// DefaultSource is the raw source text of the `default`
	// expression, kept for diagnostics and lockfile snapshots.
	DefaultSource string
	Required      bool
	// Validations are repeatable `validation { ... }` blocks attached
	// to the variable. Each block's `condition` is an hcl.Expression
	// evaluated post-resolution against the full resolved-variable
	// scope (DESIGN-0006 / IMPL-0009 Phase C).
	Validations []Validation `json:"-"`
	// TypeFieldOrder is the author-declared attribute order for
	// object-typed variables (IMPL-0009 E.3). cty.Object's attribute
	// map is unordered, so the loader captures the source order from
	// the type expression so the prompt UX can unfold object fields
	// in the order the author wrote them. Empty/nil for non-object
	// types and for nested object levels (each level only carries
	// its own top-level field order; nested orders are derived by
	// walking the type tree at prompt time).
	TypeFieldOrder []string `json:",omitempty"`
}

// Validation is a single `validation { condition = ..., error_message = ... }`
// block attached to a Variable. Authors stack multiple validation blocks
// per variable to express disjoint constraints.
type Validation struct {
	// Condition is the parsed condition expression, evaluated against
	// the full resolved-variable scope. Must evaluate to a cty.Bool.
	Condition hcl.Expression
	// ErrorMessage is the static error message surfaced when Condition
	// evaluates to false. Static-string only in v0.7 (DESIGN-0006 OQ-3
	// — no template interpolation in error_message yet).
	ErrorMessage string
	// DefRange points at the validation block in blueprint.hcl, used
	// to anchor error output with file/line/col.
	DefRange hcl.Range
}

// Deprecation is a non-fatal v0.7 transition notice surfaced by the
// loader and passed up to the CLI for ui.Warningf rendering. Today the
// only producer is ParseVariableType for the `int`-as-alias-for-`number`
// warning (DESIGN-0006 OQ-6).
type Deprecation struct {
	// Variable names the variable the deprecation applies to.
	Variable string
	// Summary is the short headline (e.g. "variable type `int` is deprecated").
	Summary string
	// Detail is the full message including the recommended fix and a
	// MIGRATION.md pointer.
	Detail string
	// Subject is the source range of the offending expression for
	// file:line:col-anchored output.
	Subject hcl.Range
}

// Condition defines conditional file inclusion/exclusion based on
// template expressions. `When` is a parsed-at-load-time HCL expression
// (per IMPL-0005 OQ-7) so syntax errors surface at LoadBlueprint time
// with file/line/column rather than on first evaluation. `WhenSource`
// retains the original expression text for diagnostics and lockfile
// snapshots.
type Condition struct {
	When       hcl.Expression `json:"-"`
	WhenSource string
	Exclude    []string
}

// Hooks defines lifecycle hooks for blueprint operations.
type Hooks struct {
	PostCreate []string
}

// SyncConfig defines which files are managed for ongoing sync.
type SyncConfig struct {
	ManagedFiles []ManagedFile
	Ignore       []string
}

// ManagedFile represents a file tracked for sync with a specific strategy.
type ManagedFile struct {
	Path     string
	Strategy string
}
