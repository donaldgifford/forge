package config

import (
	"fmt"
	"strings"
)

// validSyncStrategies are the allowed sync strategies.
var validSyncStrategies = map[string]bool{
	"overwrite": true,
	"merge":     true,
}

// ValidateBlueprint checks a Blueprint for required fields and valid
// values. Format gating (HCL-only) lives in LoadBlueprint, not here —
// this validator runs against the in-memory shape and is encoding-agnostic.
//
// Per-variable type validation is owned by ParseVariableType (vartype.go);
// this validator only enforces blueprint-level invariants that aren't
// expressible in the load-time HCL schema (variable name non-empty,
// sync strategies in the allowed set, managed file paths non-empty).
func ValidateBlueprint(bp *Blueprint) error {
	if strings.TrimSpace(bp.Name) == "" {
		return fmt.Errorf("blueprint name is required")
	}

	for i := range bp.Variables {
		if err := validateVariable(&bp.Variables[i], i); err != nil {
			return err
		}
	}

	for path, strategy := range bp.Defaults.OverrideStrategy {
		if !validSyncStrategies[strategy] {
			return fmt.Errorf("invalid override_strategy %q for path %q, must be one of: overwrite, merge", strategy, path)
		}
	}

	for i, mf := range bp.Sync.ManagedFiles {
		if strings.TrimSpace(mf.Path) == "" {
			return fmt.Errorf("managed_files[%d]: path is required", i)
		}
		if mf.Strategy != "" && !validSyncStrategies[mf.Strategy] {
			return fmt.Errorf("managed_files[%d]: invalid strategy %q, must be one of: overwrite, merge", i, mf.Strategy)
		}
	}

	return nil
}

// ValidateRegistry checks a Registry for required fields and valid
// values. Same rationale as ValidateBlueprint: format gating lives in
// LoadRegistry, not here.
func ValidateRegistry(reg *Registry) error {
	if strings.TrimSpace(reg.Name) == "" {
		return fmt.Errorf("registry name is required")
	}

	for i, bp := range reg.Blueprints {
		if strings.TrimSpace(bp.Name) == "" {
			return fmt.Errorf("blueprints[%d]: name is required", i)
		}
		if strings.TrimSpace(bp.Path) == "" {
			return fmt.Errorf("blueprints[%d] (%s): path is required", i, bp.Name)
		}
	}

	return nil
}

func validateVariable(v *Variable, index int) error {
	if strings.TrimSpace(v.Name) == "" {
		return fmt.Errorf("variables[%d]: name is required", index)
	}

	return nil
}
