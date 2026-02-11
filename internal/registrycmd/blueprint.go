package registrycmd

import "fmt"

// BlueprintOpts configures the blueprint scaffolding operation.
type BlueprintOpts struct {
	// RegistryDir is the registry root directory (must contain registry.yaml).
	RegistryDir string
	// Category is the blueprint category directory (e.g., "go").
	Category string
	// Name is the blueprint name within the category (e.g., "grpc-service").
	Name string
	// Description is the blueprint description. Defaults to a TODO placeholder if empty.
	Description string
	// Tags are the tags for the registry index. Defaults to [category] if empty.
	Tags []string
}

// BlueprintResult holds the outcome of a blueprint scaffolding operation.
type BlueprintResult struct {
	// BlueprintDir is the absolute path to the created blueprint directory.
	BlueprintDir string
	// BlueprintYAML is the absolute path to the created blueprint.yaml.
	BlueprintYAML string
	// RegistryYAML is the absolute path to the updated registry.yaml.
	RegistryYAML string
}

// RunBlueprint scaffolds a new blueprint directory inside a registry.
func RunBlueprint(opts *BlueprintOpts) (*BlueprintResult, error) {
	if opts.RegistryDir == "" {
		return nil, fmt.Errorf("registry directory is required")
	}

	return nil, fmt.Errorf("not implemented")
}
