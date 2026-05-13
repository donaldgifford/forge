package config

// Registry represents the index file at the root of a registry repo (registry.hcl).
type Registry struct {
	Name        string
	Description string
	Maintainers []Maintainer
	Defaults    RegistryDefaults
	Blueprints  []BlueprintEntry
}

// Maintainer identifies a registry maintainer.
type Maintainer struct {
	Name  string
	Email string
}

// RegistryDefaults holds registry-wide default configuration.
type RegistryDefaults struct {
	SyncStrategy string
	Managed      bool
}

// BlueprintEntry is a catalog entry for a blueprint within a registry.
type BlueprintEntry struct {
	Name         string
	Path         string
	Description  string
	Version      string
	Tags         []string
	LatestCommit string
}
