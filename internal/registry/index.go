package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/donaldgifford/forge/internal/config"
)

// LoadIndex reads registry.hcl from the given registry root directory
// and returns the parsed registry config. If only registry.yaml exists
// (legacy v2 YAML registry), the dispatcher surfaces the
// rescaffold-or-pin error written by config.LoadRegistry — the
// in-tool migrator was removed in IMPL-0007 per ADR-0002.
func LoadIndex(registryRoot string) (*config.Registry, error) {
	hclPath := filepath.Join(registryRoot, "registry.hcl")
	if _, err := os.Stat(hclPath); err == nil {
		reg, loadErr := config.LoadRegistry(hclPath)
		if loadErr != nil {
			return nil, fmt.Errorf("loading registry index from %s: %w", registryRoot, loadErr)
		}

		return reg, nil
	}

	// No HCL — fall through to the YAML path so LoadRegistry can
	// produce the migration-pointer error (or a clean "not found" if
	// neither file exists).
	yamlPath := filepath.Join(registryRoot, "registry.yaml")

	reg, err := config.LoadRegistry(yamlPath)
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
