package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadBlueprint reads and parses a blueprint.yaml file from the given path.
func LoadBlueprint(path string) (*Blueprint, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is provided by the caller; this is a scaffolding tool that reads user-specified config files
	if err != nil {
		return nil, fmt.Errorf("reading blueprint file %s: %w", path, err)
	}

	var bp Blueprint
	if err := yaml.Unmarshal(data, &bp); err != nil {
		return nil, fmt.Errorf("parsing blueprint file %s: %w", path, err)
	}

	if err := ValidateBlueprint(&bp); err != nil {
		return nil, fmt.Errorf("validating blueprint %s: %w", path, err)
	}

	return &bp, nil
}

// LoadRegistry reads and parses a registry.yaml file from the given path.
func LoadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is provided by the caller; this is a scaffolding tool that reads user-specified config files
	if err != nil {
		return nil, fmt.Errorf("reading registry file %s: %w", path, err)
	}

	var reg Registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parsing registry file %s: %w", path, err)
	}

	if err := ValidateRegistry(&reg); err != nil {
		return nil, fmt.Errorf("validating registry %s: %w", path, err)
	}

	return &reg, nil
}
