package registrycmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/donaldgifford/forge/internal/config"
)

// BlueprintOpts configures the blueprint scaffolding operation.
type BlueprintOpts struct {
	// RegistryDir is the registry root directory (must contain registry.hcl).
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
	// BlueprintHCL is the absolute path to the created blueprint.hcl.
	BlueprintHCL string
	// RegistryHCL is the absolute path to the updated registry.hcl.
	RegistryHCL string
}

const blueprintScaffoldTemplate = `name        = "%s"
description = "%s"
version     = "0.1.0"
tags        = [%s]

variable "project_name" {
  description = "Name of the project"
  type        = string
  required    = true
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]*$", var.project_name))
    error_message = "project_name must be lowercase letters, digits, or hyphens, starting with a letter."
  }
}

variable "license" {
  description = "License type"
  type        = string
  default     = "Apache-2.0"
  validation {
    condition     = contains(["MIT", "Apache-2.0", "BSD-3-Clause", "none"], var.license)
    error_message = "license must be one of: MIT, Apache-2.0, BSD-3-Clause, none."
  }
}

# condition {
#   when    = some_variable
#   exclude = ["optional-dir/"]
# }

hooks {
  post_create = ["git init"]
}

rename {
  entry {
    from = "${project_name}/"
    to   = "."
  }
}
`

const starterReadmeTemplate = `# ${project_name}

A new blueprint.

## Getting Started

TODO: Add getting started instructions.
`

// ParseBlueprintPath splits a "category/name" string into its components.
func ParseBlueprintPath(arg string) (category, name string, err error) {
	if arg == "" {
		return "", "", fmt.Errorf("blueprint path is required")
	}

	parts := strings.SplitN(arg, "/", 3)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid blueprint path %q: expected format category/name", arg)
	}

	return parts[0], parts[1], nil
}

// RunBlueprint scaffolds a new blueprint directory inside a registry.
func RunBlueprint(opts *BlueprintOpts) (*BlueprintResult, error) {
	if opts.RegistryDir == "" {
		return nil, fmt.Errorf("registry directory is required")
	}

	registryDir, err := filepath.Abs(opts.RegistryDir)
	if err != nil {
		return nil, fmt.Errorf("resolving registry path %s: %w", opts.RegistryDir, err)
	}

	registryHCL := filepath.Join(registryDir, "registry.hcl")
	if _, err := os.Stat(registryHCL); err != nil {
		return nil, fmt.Errorf("registry.hcl not found at %s; run forge registry init first", registryDir)
	}

	if opts.Category == "" || opts.Name == "" {
		return nil, fmt.Errorf("both category and name are required")
	}

	bpRelPath := opts.Category + "/" + opts.Name
	bpDir := filepath.Join(registryDir, opts.Category, opts.Name)
	bpHCLPath := filepath.Join(bpDir, "blueprint.hcl")

	if _, err := os.Stat(bpHCLPath); err == nil {
		return nil, fmt.Errorf("blueprint.hcl already exists at %s", bpHCLPath)
	}

	// Apply defaults.
	description := opts.Description
	if description == "" {
		description = "TODO: Add a description for this blueprint"
	}

	tags := opts.Tags
	if len(tags) == 0 {
		tags = []string{opts.Category}
	}

	bpName := opts.Category + "-" + opts.Name

	// Create blueprint directory.
	if err := os.MkdirAll(bpDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating blueprint directory %s: %w", bpDir, err)
	}

	if err := writeBlueprintHCL(bpHCLPath, bpName, description, tags); err != nil {
		return nil, err
	}

	if err := createStarterTemplate(bpDir); err != nil {
		return nil, err
	}

	if err := ensureCategoryDefaults(registryDir, opts.Category); err != nil {
		return nil, err
	}

	if err := appendBlueprint(registryDir, bpRelPath, description, tags); err != nil {
		return nil, err
	}

	return &BlueprintResult{
		BlueprintDir: bpDir,
		BlueprintHCL: bpHCLPath,
		RegistryHCL:  registryHCL,
	}, nil
}

func writeBlueprintHCL(path, name, description string, tags []string) error {
	content := fmt.Sprintf(blueprintScaffoldTemplate, name, description, formatTags(tags))

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing blueprint.hcl: %w", err)
	}

	if _, err := config.LoadBlueprintHCL(path); err != nil {
		return fmt.Errorf("internal error: scaffolded blueprint.hcl failed to load: %w", err)
	}

	return nil
}

func formatTags(tags []string) string {
	quoted := make([]string, len(tags))
	for i, t := range tags {
		quoted[i] = fmt.Sprintf("%q", t)
	}

	return strings.Join(quoted, ", ")
}

func createStarterTemplate(blueprintDir string) error {
	tmplDir := filepath.Join(blueprintDir, "${project_name}")

	if err := os.MkdirAll(tmplDir, 0o750); err != nil {
		return fmt.Errorf("creating template directory: %w", err)
	}

	readmePath := filepath.Join(tmplDir, "README.md.tmpl")
	if err := os.WriteFile(readmePath, []byte(starterReadmeTemplate), 0o644); err != nil {
		return fmt.Errorf("writing starter README.md.tmpl: %w", err)
	}

	return nil
}

func ensureCategoryDefaults(registryDir, category string) error {
	defaultsDir := filepath.Join(registryDir, category, "_defaults")

	if _, err := os.Stat(defaultsDir); err == nil {
		return nil // Already exists.
	}

	if err := os.MkdirAll(defaultsDir, 0o750); err != nil {
		return fmt.Errorf("creating category defaults %s: %w", defaultsDir, err)
	}

	gitkeepPath := filepath.Join(defaultsDir, ".gitkeep")
	if err := os.WriteFile(gitkeepPath, []byte(""), 0o644); err != nil {
		return fmt.Errorf("writing .gitkeep for category %s: %w", category, err)
	}

	return nil
}

func appendBlueprint(registryDir, bpRelPath, description string, tags []string) error {
	indexPath := filepath.Join(registryDir, "registry.hcl")

	reg, err := config.LoadRegistryHCL(indexPath)
	if err != nil {
		return fmt.Errorf("loading registry.hcl: %w", err)
	}

	for i := range reg.Blueprints {
		if reg.Blueprints[i].Path == bpRelPath {
			return fmt.Errorf("blueprint %s already exists in registry.hcl", bpRelPath)
		}
	}

	reg.Blueprints = append(reg.Blueprints, config.BlueprintEntry{
		Name:        bpRelPath,
		Path:        bpRelPath,
		Description: description,
		Version:     "0.1.0",
		Tags:        tags,
	})

	var buf bytes.Buffer
	if err := config.WriteRegistryHCL(&buf, reg); err != nil {
		return fmt.Errorf("rendering registry.hcl: %w", err)
	}

	if err := os.WriteFile(indexPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing registry.hcl: %w", err)
	}

	return nil
}
