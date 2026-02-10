// Package config handles parsing and validation of blueprint.yaml and registry.yaml files.
package config

// Blueprint represents the configuration of a single blueprint (blueprint.yaml).
type Blueprint struct {
	APIVersion  string            `yaml:"apiVersion"`
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Version     string            `yaml:"version"`
	Tags        []string          `yaml:"tags"`
	Defaults    Defaults          `yaml:"defaults"`
	Variables   []Variable        `yaml:"variables"`
	Conditions  []Condition       `yaml:"conditions"`
	Tools       []Tool            `yaml:"tools"`
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

// Condition defines conditional file inclusion/exclusion based on template expressions.
type Condition struct {
	When    string   `yaml:"when"`
	Exclude []string `yaml:"exclude"`
}

// Tool declares a remote CLI tool with version pin and download source.
type Tool struct {
	Name        string     `yaml:"name"`
	Version     string     `yaml:"version"`
	Description string     `yaml:"description"`
	Source      ToolSource `yaml:"source"`
	InstallPath string     `yaml:"install_path"`
	Checksum    Checksum   `yaml:"checksum"`
	Condition   string     `yaml:"condition"`
}

// ToolSource describes how to obtain a tool binary.
type ToolSource struct {
	Type         string `yaml:"type"`
	Repo         string `yaml:"repo"`
	AssetPattern string `yaml:"asset_pattern"`
	URL          string `yaml:"url"`
	Module       string `yaml:"module"`
	Package      string `yaml:"package"`
	Crate        string `yaml:"crate"`
}

// Checksum holds platform-specific SHA256 checksums for tool verification.
type Checksum struct {
	SHA256 map[string]string `yaml:"sha256"`
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
