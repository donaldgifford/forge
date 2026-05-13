// Package config handles parsing and validation of blueprint and
// registry config files. HCL is the only supported on-disk format
// (post-IMPL-0005 cutover); the loaders in loader.go reject .yaml
// inputs with a migration-pointer error.
package config

import (
	"github.com/hashicorp/hcl/v2"
)

// Blueprint represents the configuration of a single blueprint
// (blueprint.hcl).
type Blueprint struct {
	Name        string
	Description string
	Version     string
	Tags        []string
	Defaults    Defaults
	Variables   []Variable
	Conditions  []Condition
	Hooks       Hooks
	Sync        SyncConfig
	Rename      map[string]string
}

// Defaults controls which inherited default files are included or excluded.
type Defaults struct {
	Exclude          []string
	OverrideStrategy map[string]string
}

// Variable represents a user-prompted variable in a blueprint.
type Variable struct {
	Name        string
	Description string
	Type        string
	Default     string
	Required    bool
	Validate    string
	Choices     []string
}

// Condition defines conditional file inclusion/exclusion based on
// template expressions. `When` is a parsed-at-load-time HCL expression
// (per IMPL-0005 OQ-7) so syntax errors surface at LoadBlueprint time
// with file/line/column rather than on first evaluation. `WhenSource`
// retains the original expression text for diagnostics, lockfile
// snapshots, and round-trip output by `forge migrate config`.
type Condition struct {
	When       hcl.Expression
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
