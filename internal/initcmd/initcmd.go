// Package initcmd implements the forge init command for scaffolding new blueprints.
package initcmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/donaldgifford/forge/internal/config"
)

// Opts configures the init operation.
type Opts struct {
	// Path is the blueprint path (e.g., "go/grpc-gateway"). Empty means current directory.
	Path string
	// RegistryDir is the registry root directory for registry mode. Empty means standalone mode.
	RegistryDir string
}

const blueprintTemplate = `name        = "%s"
description = "TODO: Add a description for this blueprint"
version     = "0.1.0"
tags        = []

variable "project_name" {
  description = "Name of the project"
  type        = "string"
  required    = true
}
`

// Run executes the init workflow.
func Run(opts *Opts) (string, error) {
	if opts.RegistryDir != "" {
		return initRegistry(opts)
	}

	return initStandalone(opts)
}

func initStandalone(opts *Opts) (string, error) {
	dir := "."
	if opts.Path != "" {
		dir = opts.Path
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("creating directory %s: %w", dir, err)
	}

	bpPath := filepath.Join(dir, "blueprint.hcl")
	if _, err := os.Stat(bpPath); err == nil {
		return "", fmt.Errorf("blueprint.hcl already exists at %s", bpPath)
	}

	name := deriveName(dir)
	content := fmt.Sprintf(blueprintTemplate, name)

	if err := os.WriteFile(bpPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("writing blueprint.hcl: %w", err)
	}

	return bpPath, nil
}

func initRegistry(opts *Opts) (string, error) {
	if opts.Path == "" {
		return "", fmt.Errorf("blueprint path is required in registry mode")
	}

	bpDir := filepath.Join(opts.RegistryDir, opts.Path)

	if err := os.MkdirAll(bpDir, 0o750); err != nil {
		return "", fmt.Errorf("creating blueprint directory %s: %w", bpDir, err)
	}

	bpPath := filepath.Join(bpDir, "blueprint.hcl")
	if _, err := os.Stat(bpPath); err == nil {
		return "", fmt.Errorf("blueprint.hcl already exists at %s", bpPath)
	}

	name := deriveName(opts.Path)
	content := fmt.Sprintf(blueprintTemplate, name)

	if err := os.WriteFile(bpPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("writing blueprint.hcl: %w", err)
	}

	if err := appendToRegistryIndex(opts.RegistryDir, opts.Path, name); err != nil {
		return "", fmt.Errorf("updating registry.hcl: %w", err)
	}

	return bpPath, nil
}

func appendToRegistryIndex(registryDir, bpPath, name string) error {
	indexPath := filepath.Join(registryDir, "registry.hcl")

	reg, err := loadOrCreateRegistry(indexPath)
	if err != nil {
		return err
	}

	for i := range reg.Blueprints {
		if reg.Blueprints[i].Path == bpPath {
			return nil
		}
	}

	reg.Blueprints = append(reg.Blueprints, config.BlueprintEntry{
		Name:        name,
		Path:        bpPath,
		Description: "TODO: Add a description",
		Version:     "0.1.0",
	})

	var buf bytes.Buffer
	if err := config.WriteRegistryHCL(&buf, reg); err != nil {
		return fmt.Errorf("rendering registry.hcl: %w", err)
	}

	if err := os.WriteFile(indexPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", indexPath, err)
	}

	return nil
}

func loadOrCreateRegistry(path string) (*config.Registry, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return &config.Registry{Name: "registry"}, nil
		}

		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	return config.LoadRegistryHCL(path)
}

// deriveName extracts a reasonable blueprint name from the path.
func deriveName(path string) string {
	if path == "." || path == "" {
		dir, err := os.Getwd()
		if err != nil {
			return "my-blueprint"
		}

		return filepath.Base(dir)
	}

	// Convert path separators to hyphens: "go/api" → "go-api"
	return strings.ReplaceAll(filepath.ToSlash(path), "/", "-")
}
