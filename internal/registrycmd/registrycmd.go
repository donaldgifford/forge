// Package registrycmd implements the forge registry init command for scaffolding new registries.
package registrycmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/donaldgifford/forge/internal/config"
)

// Opts configures the registry init operation.
type Opts struct {
	// Path is the target directory for the new registry.
	Path string
	// Name is the registry display name. Defaults to directory basename if empty.
	Name string
	// Description is the registry description.
	Description string
	// GitInit controls whether to run git init in the new registry.
	GitInit bool
	// Categories is a list of category directories to pre-create.
	Categories []string
}

// Result holds the outcome of a registry init operation.
type Result struct {
	// Dir is the absolute path of the created registry.
	Dir string
	// GitInitialized indicates whether git init was run successfully.
	GitInitialized bool
}

const registryTemplate = `apiVersion: v1
name: "%s"
description: "%s"
blueprints: []
`

const editorConfigContent = `# EditorConfig helps maintain consistent coding styles
# https://editorconfig.org

root = true

[*]
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true
charset = utf-8
indent_style = space
indent_size = 2
`

const gitignoreContent = `# OS files
.DS_Store
Thumbs.db

# Editor files
*.swp
*.swo
*~
.idea/
.vscode/
`

const readmeTemplate = `# %s

%s

## Structure

` + "```" + `
├── registry.yaml        # Registry index
├── _defaults/           # Registry-wide default files
│   ├── .editorconfig
│   └── .gitignore
└── <category>/
    └── _defaults/       # Category-level default files
` + "```" + `

## Adding a Blueprint

` + "```bash" + `
forge init <category>/<name> --registry .
` + "```" + `
`

// Run executes the registry init workflow.
func Run(opts *Opts) (*Result, error) {
	if opts.Path == "" {
		return nil, fmt.Errorf("registry path is required")
	}

	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("resolving path %s: %w", opts.Path, err)
	}

	registryYAML := filepath.Join(absPath, "registry.yaml")
	if _, err := os.Stat(registryYAML); err == nil {
		return nil, fmt.Errorf("registry.yaml already exists at %s", registryYAML)
	}

	if err := os.MkdirAll(absPath, 0o750); err != nil {
		return nil, fmt.Errorf("creating directory %s: %w", absPath, err)
	}

	name := opts.Name
	if name == "" {
		name = filepath.Base(absPath)
	}

	description := opts.Description
	if description == "" {
		description = "A forge blueprint registry"
	}

	if err := writeRegistryYAML(registryYAML, name, description); err != nil {
		return nil, err
	}

	if err := createDefaults(absPath); err != nil {
		return nil, err
	}

	if err := writeReadme(absPath, name, description); err != nil {
		return nil, err
	}

	for _, cat := range opts.Categories {
		if err := createCategory(absPath, cat); err != nil {
			return nil, err
		}
	}

	result := &Result{Dir: absPath}

	if opts.GitInit {
		result.GitInitialized = gitInit(absPath)
	}

	return result, nil
}

func writeRegistryYAML(path, name, description string) error {
	content := fmt.Sprintf(registryTemplate, name, description)

	// Validate the generated YAML by round-tripping through config types.
	var reg config.Registry
	if err := yaml.Unmarshal([]byte(content), &reg); err != nil {
		return fmt.Errorf("internal error: invalid registry YAML: %w", err)
	}

	if err := config.ValidateRegistry(&reg); err != nil {
		return fmt.Errorf("internal error: invalid registry config: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing registry.yaml: %w", err)
	}

	return nil
}

func createDefaults(rootDir string) error {
	defaultsDir := filepath.Join(rootDir, "_defaults")

	if err := os.MkdirAll(defaultsDir, 0o750); err != nil {
		return fmt.Errorf("creating _defaults directory: %w", err)
	}

	editorConfigPath := filepath.Join(defaultsDir, ".editorconfig")
	if err := os.WriteFile(editorConfigPath, []byte(editorConfigContent), 0o644); err != nil {
		return fmt.Errorf("writing .editorconfig: %w", err)
	}

	gitignorePath := filepath.Join(defaultsDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0o644); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	return nil
}

func writeReadme(rootDir, name, description string) error {
	readmePath := filepath.Join(rootDir, "README.md")
	content := fmt.Sprintf(readmeTemplate, name, description)

	if err := os.WriteFile(readmePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing README.md: %w", err)
	}

	return nil
}

func createCategory(rootDir, category string) error {
	catDefaultsDir := filepath.Join(rootDir, category, "_defaults")

	if err := os.MkdirAll(catDefaultsDir, 0o750); err != nil {
		return fmt.Errorf("creating category %s: %w", category, err)
	}

	gitkeepPath := filepath.Join(catDefaultsDir, ".gitkeep")
	if err := os.WriteFile(gitkeepPath, []byte(""), 0o644); err != nil {
		return fmt.Errorf("writing .gitkeep for category %s: %w", category, err)
	}

	return nil
}

func gitInit(dir string) bool {
	cmd := exec.CommandContext(context.Background(), "git", "init", dir)
	cmd.Stdout = nil
	cmd.Stderr = nil

	return cmd.Run() == nil
}
