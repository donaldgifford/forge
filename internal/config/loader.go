package config

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
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

	// Validate apiVersion before parsing condition expressions: v1
	// blueprints carry text/template `when` strings that won't parse as
	// HCL2, and we want the apiVersion-rejection error to surface
	// instead of an obscure parse diagnostic.
	if err := ValidateBlueprint(&bp); err != nil {
		return nil, fmt.Errorf("validating blueprint %s: %w", path, err)
	}

	if err := parseConditionExpressions(bp.Conditions, path); err != nil {
		return nil, err
	}

	return &bp, nil
}

// parseConditionExpressions parses each Condition.WhenSource string into an
// hcl.Expression and stores it on Condition.When. The YAML loader runs this
// after yaml.Unmarshal so downstream code can rely on When being populated
// regardless of which loader produced the Blueprint.
func parseConditionExpressions(conditions []Condition, path string) error {
	for i := range conditions {
		src := conditions[i].WhenSource
		if src == "" {
			continue
		}

		expr, diags := hclsyntax.ParseExpression([]byte(src), path, hcl.InitialPos)
		if diags.HasErrors() {
			return fmt.Errorf("parsing condition %d when expression in %s: %s", i, path, diags.Error())
		}

		conditions[i].When = expr
	}

	return nil
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
