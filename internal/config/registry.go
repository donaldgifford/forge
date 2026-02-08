package config

// Registry represents the index file at the root of a registry repo (registry.yaml).
type Registry struct {
	APIVersion  string           `yaml:"apiVersion"`
	Name        string           `yaml:"name"`
	Description string           `yaml:"description"`
	Maintainers []Maintainer     `yaml:"maintainers"`
	Defaults    RegistryDefaults `yaml:"defaults"`
	Tools       []Tool           `yaml:"tools"`
	Blueprints  []BlueprintEntry `yaml:"blueprints"`
}

// Maintainer identifies a registry maintainer.
type Maintainer struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

// RegistryDefaults holds registry-wide default configuration.
type RegistryDefaults struct {
	SyncStrategy string `yaml:"sync_strategy"`
	Managed      bool   `yaml:"managed"`
}

// BlueprintEntry is a catalog entry for a blueprint within a registry.
type BlueprintEntry struct {
	Name         string   `yaml:"name"`
	Path         string   `yaml:"path"`
	Description  string   `yaml:"description"`
	Version      string   `yaml:"version"`
	Tags         []string `yaml:"tags"`
	LatestCommit string   `yaml:"latest_commit"`
}
