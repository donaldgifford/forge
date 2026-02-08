package registry

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/donaldgifford/forge/internal/config"
)

// LoadIndex reads registry.yaml from the given registry root directory and returns
// the parsed registry config. It validates the registry contents.
func LoadIndex(registryRoot string) (*config.Registry, error) {
	indexPath := filepath.Join(registryRoot, "registry.yaml")

	reg, err := config.LoadRegistry(indexPath)
	if err != nil {
		return nil, fmt.Errorf("loading registry index from %s: %w", registryRoot, err)
	}

	return reg, nil
}

// FindBlueprint looks up a blueprint entry in the registry by path or name.
// It returns the matching entry or an error listing available blueprints.
func FindBlueprint(reg *config.Registry, blueprintPath string) (*config.BlueprintEntry, error) {
	// Normalize the path by trimming trailing slashes.
	blueprintPath = strings.TrimRight(blueprintPath, "/")

	for i := range reg.Blueprints {
		entry := &reg.Blueprints[i]
		if entry.Path == blueprintPath || entry.Name == blueprintPath {
			return entry, nil
		}
	}

	available := make([]string, 0, len(reg.Blueprints))
	for i := range reg.Blueprints {
		available = append(available, reg.Blueprints[i].Name)
	}

	return nil, fmt.Errorf(
		"blueprint %q not found in registry %q; available blueprints: %s",
		blueprintPath,
		reg.Name,
		strings.Join(available, ", "),
	)
}
