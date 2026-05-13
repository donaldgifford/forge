// Package config handles parsing and validation of blueprint and
// registry config files.
//
// The HCL loaders (LoadBlueprintHCL, LoadRegistryHCL in loader_hcl.go)
// are the new primary path; the YAML loaders (LoadBlueprint,
// LoadRegistry in loader.go) stay around during the IMPL-0005 Phase A
// side-by-side window and are removed in Phase C.
package config

import (
	"github.com/hashicorp/hcl/v2"
)

// Blueprint represents the configuration of a single blueprint
// (blueprint.hcl, or legacy blueprint.yaml).
type Blueprint struct {
	APIVersion  string            `yaml:"apiVersion"`
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Version     string            `yaml:"version"`
	Tags        []string          `yaml:"tags"`
	Defaults    Defaults          `yaml:"defaults"`
	Variables   []Variable        `yaml:"variables"`
	Conditions  []Condition       `yaml:"conditions"`
	Hooks       Hooks             `yaml:"hooks"`
	Sync        SyncConfig        `yaml:"sync"`
	Rename      map[string]string `yaml:"rename"`
}

// Defaults controls which inherited default files are included or excluded.
type Defaults struct {
	Exclude          []string          `yaml:"exclude"`
	OverrideStrategy map[string]string `yaml:"override_strategy"`
}

// Variable represents a user-prompted variable in a blueprint.
type Variable struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Type        string   `yaml:"type"`
	Default     string   `yaml:"default"`
	Required    bool     `yaml:"required"`
	Validate    string   `yaml:"validate"`
	Choices     []string `yaml:"choices"`
}

// Condition defines conditional file inclusion/exclusion based on
// template expressions. `When` is a parsed-at-load-time HCL expression
// (per IMPL-0005 OQ-7) so syntax errors surface at LoadBlueprint time
// with file/line/column rather than on first evaluation. `WhenSource`
// retains the original expression text for diagnostics, lockfile
// snapshots, and round-trip output by `forge migrate config`.
type Condition struct {
	When       hcl.Expression `yaml:"-"`
	WhenSource string         `yaml:"when"`
	Exclude    []string       `yaml:"exclude"`
}

// Hooks defines lifecycle hooks for blueprint operations.
type Hooks struct {
	PostCreate []string `yaml:"post_create"`
}

// SyncConfig defines which files are managed for ongoing sync.
type SyncConfig struct {
	ManagedFiles []ManagedFile `yaml:"managed_files"`
	Ignore       []string      `yaml:"ignore"`
}

// ManagedFile represents a file tracked for sync with a specific strategy.
type ManagedFile struct {
	Path     string `yaml:"path"`
	Strategy string `yaml:"strategy"`
}
