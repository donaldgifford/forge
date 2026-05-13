package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"gopkg.in/yaml.v3"
)

// LoadBlueprint reads and parses a blueprint config file from the given
// path. It dispatches based on what's on disk:
//
//   - If `path` already ends in `.hcl`, it's loaded via the HCL loader.
//   - If `path` ends in `.yaml` and a sibling `blueprint.hcl` exists,
//     the HCL sibling wins (Phase A side-by-side preference: HCL is the
//     new format and Phase C makes it the only format).
//   - Otherwise the YAML loader handles it.
//
// Callers continue to pass the canonical YAML path; the dispatcher
// transparently upgrades to HCL when present.
func LoadBlueprint(path string) (*Blueprint, error) {
	if hclPath, ok := preferHCLSibling(path); ok {
		return LoadBlueprintHCL(hclPath)
	}

	return loadBlueprintYAML(path)
}

// preferHCLSibling returns the .hcl path to load instead of the input
// when:
//   - the input path itself is .hcl, or
//   - a `<basename>.hcl` sibling exists next to a .yaml input.
//
// Returns ("", false) when the YAML loader should handle the input.
func preferHCLSibling(path string) (string, bool) {
	if strings.HasSuffix(path, ".hcl") {
		return path, true
	}

	if !strings.HasSuffix(path, ".yaml") {
		return "", false
	}

	hclPath := strings.TrimSuffix(path, ".yaml") + ".hcl"
	if _, err := os.Stat(hclPath); err == nil {
		return hclPath, true
	}

	return "", false
}

// loadBlueprintYAML is the legacy YAML loader. Deleted in Phase C once
// the migration tool has rewritten the in-tree fixtures and the
// downstream forge-registry corpus.
func loadBlueprintYAML(path string) (*Blueprint, error) {
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

// LoadRegistry reads and parses a registry config file. Mirrors the
// LoadBlueprint dispatch — a sibling registry.hcl wins over a
// registry.yaml when both exist.
func LoadRegistry(path string) (*Registry, error) {
	if hclPath, ok := preferHCLSibling(path); ok {
		return LoadRegistryHCL(hclPath)
	}

	return loadRegistryYAML(path)
}

// loadRegistryYAML is the legacy YAML loader. Deleted in Phase C.
func loadRegistryYAML(path string) (*Registry, error) {
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
