package config

import (
	"fmt"
	"regexp"
	"strings"
)

// validVariableTypes are the allowed types for blueprint variables.
var validVariableTypes = map[string]bool{
	"string": true,
	"bool":   true,
	"choice": true,
	"int":    true,
}

// validSyncStrategies are the allowed sync strategies.
var validSyncStrategies = map[string]bool{
	"overwrite": true,
	"merge":     true,
}

// validToolSourceTypes are the allowed tool source types.
var validToolSourceTypes = map[string]bool{
	"github-release": true,
	"url":            true,
	"go-install":     true,
	"npm":            true,
	"cargo-install":  true,
	"script":         true,
}

// ValidateBlueprint checks a Blueprint for required fields and valid values.
func ValidateBlueprint(bp *Blueprint) error {
	if bp.APIVersion != "v1" {
		return fmt.Errorf("unsupported apiVersion %q, expected \"v1\"", bp.APIVersion)
	}

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

	for i := range bp.Tools {
		if err := validateTool(&bp.Tools[i], i); err != nil {
			return err
		}
	}

	return nil
}

// ValidateRegistry checks a Registry for required fields and valid values.
func ValidateRegistry(reg *Registry) error {
	if reg.APIVersion != "v1" {
		return fmt.Errorf("unsupported apiVersion %q, expected \"v1\"", reg.APIVersion)
	}

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

	for i := range reg.Tools {
		if err := validateTool(&reg.Tools[i], i); err != nil {
			return err
		}
	}

	return nil
}

func validateVariable(v *Variable, index int) error {
	if strings.TrimSpace(v.Name) == "" {
		return fmt.Errorf("variables[%d]: name is required", index)
	}

	if v.Type == "" {
		return fmt.Errorf("variables[%d] (%s): type is required", index, v.Name)
	}

	if !validVariableTypes[v.Type] {
		return fmt.Errorf("variables[%d] (%s): invalid type %q, must be one of: string, bool, choice, int", index, v.Name, v.Type)
	}

	if v.Type == "choice" && len(v.Choices) == 0 {
		return fmt.Errorf("variables[%d] (%s): choices are required for type \"choice\"", index, v.Name)
	}

	if v.Validate != "" {
		if _, err := regexp.Compile(v.Validate); err != nil {
			return fmt.Errorf("variables[%d] (%s): invalid validate regex %q: %w", index, v.Name, v.Validate, err)
		}
	}

	return nil
}

func validateTool(t *Tool, index int) error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("tools[%d]: name is required", index)
	}

	if strings.TrimSpace(t.Version) == "" {
		return fmt.Errorf("tools[%d] (%s): version is required", index, t.Name)
	}

	if t.Source.Type != "" && !validToolSourceTypes[t.Source.Type] {
		return fmt.Errorf(
			"tools[%d] (%s): invalid source type %q, must be one of: github-release, url, go-install, npm, cargo-install, script",
			index, t.Name, t.Source.Type,
		)
	}

	return nil
}
