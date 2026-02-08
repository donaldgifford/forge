// Package tools handles tool manifest parsing, resolution, and management.
package tools

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/donaldgifford/forge/internal/config"
	tmpl "github.com/donaldgifford/forge/internal/template"
)

// ResolvedTool represents a fully resolved tool after inheritance merging.
type ResolvedTool struct {
	Name        string
	Version     string
	Description string
	Source      config.ToolSource
	InstallPath string
	Checksum    config.Checksum
	SourceLayer string
}

// categoryTools is the YAML structure for _defaults/tools.yaml.
type categoryTools struct {
	Tools []config.Tool `yaml:"tools"`
}

// ResolveTools merges tool declarations using layered inheritance:
// registry tools → category tools → blueprint tools (last wins by name).
// Tools with conditions that evaluate to false are excluded.
func ResolveTools(
	registryTools []config.Tool,
	categoryToolsPath string,
	blueprintTools []config.Tool,
	vars map[string]any,
) ([]ResolvedTool, error) {
	merged := make(map[string]*ResolvedTool)
	var order []string

	// Layer 1: Registry tools.
	for i := range registryTools {
		t := &registryTools[i]
		addTool(merged, &order, t, "registry")
	}

	// Layer 2: Category tools from _defaults/tools.yaml.
	catTools, err := loadCategoryTools(categoryToolsPath)
	if err != nil {
		return nil, err
	}

	for i := range catTools {
		addTool(merged, &order, &catTools[i], "category")
	}

	// Layer 3: Blueprint tools.
	for i := range blueprintTools {
		t := &blueprintTools[i]
		addTool(merged, &order, t, "blueprint")
	}

	// Evaluate conditions and build result.
	renderer := tmpl.NewRenderer()
	result := make([]ResolvedTool, 0, len(order))

	for _, name := range order {
		tool, ok := merged[name]
		if !ok {
			continue
		}

		// Find the original config.Tool to check condition.
		cond := findCondition(registryTools, blueprintTools, catTools, name)
		if cond != "" {
			active, err := evaluateCondition(renderer, cond, vars)
			if err != nil {
				return nil, err
			}

			if !active {
				continue
			}
		}

		result = append(result, *tool)
	}

	return result, nil
}

func addTool(merged map[string]*ResolvedTool, order *[]string, t *config.Tool, layer string) {
	if _, exists := merged[t.Name]; !exists {
		*order = append(*order, t.Name)
	}

	merged[t.Name] = &ResolvedTool{
		Name:        t.Name,
		Version:     t.Version,
		Description: t.Description,
		Source:      t.Source,
		InstallPath: t.InstallPath,
		Checksum:    t.Checksum,
		SourceLayer: layer,
	}
}

func loadCategoryTools(path string) ([]config.Tool, error) {
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, err
	}

	var ct categoryTools
	if err := yaml.Unmarshal(data, &ct); err != nil {
		return nil, err
	}

	return ct.Tools, nil
}

func findCondition(registry, blueprint, category []config.Tool, name string) string {
	// Blueprint wins, then category, then registry.
	for i := range blueprint {
		if blueprint[i].Name == name {
			return blueprint[i].Condition
		}
	}

	for i := range category {
		if category[i].Name == name {
			return category[i].Condition
		}
	}

	for i := range registry {
		if registry[i].Name == name {
			return registry[i].Condition
		}
	}

	return ""
}

func evaluateCondition(renderer *tmpl.Renderer, cond string, vars map[string]any) (bool, error) {
	result, err := renderer.RenderString(cond, vars)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(result) == "true", nil
}
