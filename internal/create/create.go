// Package create orchestrates the forge create workflow.
package create

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/defaults"
	"github.com/donaldgifford/forge/internal/lockfile"
	"github.com/donaldgifford/forge/internal/prompt"
	"github.com/donaldgifford/forge/internal/registry"
	tmpl "github.com/donaldgifford/forge/internal/template"
	"github.com/donaldgifford/forge/internal/tools"
)

// Opts holds the options for the create command.
type Opts struct {
	// BlueprintRef is the user-provided blueprint reference (e.g., "go/api", "go/api@v1.0.0").
	BlueprintRef string

	// OutputDir is the target directory. If empty, derived from project_name variable.
	OutputDir string

	// Overrides are --set key=value pairs from the CLI.
	Overrides map[string]string

	// UseDefaults skips interactive prompts and uses default values.
	UseDefaults bool

	// NoTools skips tool installation.
	NoTools bool

	// NoHooks skips post-create hook execution.
	NoHooks bool

	// ForceCreate allows overwriting a non-empty output directory.
	ForceCreate bool

	// DefaultRegistryURL is the default registry URL from global config.
	DefaultRegistryURL string

	// RegistryDir is a local path to a pre-fetched registry (bypasses remote fetch).
	// Used in tests and when working with local registries.
	RegistryDir string

	// RegistryURL is the canonical URL to store in the lockfile for sync.
	// For local registries, this is the absolute path. For remote registries,
	// this is the go-getter URL. If empty, falls back to RegistryDir.
	RegistryURL string

	// ForgeVersion is the current forge build version for lockfile recording.
	ForgeVersion string

	// PromptFn is the function used for interactive prompting.
	// If nil, prompting is skipped (variables must come from overrides or defaults).
	PromptFn prompt.PromptFn

	// Logger for debug output.
	Logger *slog.Logger
}

// Result holds the output of a successful create operation.
type Result struct {
	OutputDir    string
	FilesCreated int
	Blueprint    string
}

// Run executes the create workflow.
func Run(opts *Opts) (*Result, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// 1-5. Resolve references and load config.
	resolved, bp, err := resolveAndLoad(opts)
	if err != nil {
		return nil, err
	}

	logger.Debug("loaded blueprint", "name", bp.Name, "version", bp.Version)

	// 6. Collect variables.
	vars, err := prompt.CollectVariables(bp.Variables, opts.Overrides, opts.UseDefaults, opts.PromptFn)
	if err != nil {
		return nil, fmt.Errorf("collecting variables: %w", err)
	}

	// 7. Resolve defaults inheritance.
	fileSet, err := defaults.Resolve(opts.RegistryDir, resolved.BlueprintPath, bp.Defaults.Exclude)
	if err != nil {
		return nil, fmt.Errorf("resolving defaults: %w", err)
	}

	// 7b. Evaluate conditions to exclude files.
	if err := EvaluateConditions(bp.Conditions, vars, fileSet); err != nil {
		return nil, fmt.Errorf("evaluating conditions: %w", err)
	}

	logger.Debug("resolved files", "count", fileSet.Len())

	// 8. Determine and create output directory.
	outputDir := resolveOutputDir(opts.OutputDir, vars, bp.Name)

	// Guard: refuse to write into a non-empty directory without --force.
	if !opts.ForceCreate {
		if err := checkOutputDirEmpty(outputDir); err != nil {
			return nil, err
		}
	}

	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating output directory %s: %w", outputDir, err)
	}

	// 9. Render and write files.
	filesCreated, err := renderFiles(fileSet, vars, outputDir, bp.Rename)
	if err != nil {
		return nil, err
	}

	// 10. Resolve and install tools.
	resolvedTools, err := resolveTools(opts, bp, vars, logger)
	if err != nil {
		return nil, err
	}

	// 11. Generate lockfile with content hashes.
	lockPath := filepath.Join(outputDir, lockfile.FileName)
	lock := buildLockfile(resolved, bp, vars, fileSet, resolvedTools, opts.ForgeVersion, opts.RegistryURL)
	computeFileHashes(outputDir, lock)

	if err := lockfile.Write(lockPath, lock); err != nil {
		return nil, fmt.Errorf("writing lockfile: %w", err)
	}

	logger.Info("project created", "dir", outputDir, "files", filesCreated)

	return &Result{
		OutputDir:    outputDir,
		FilesCreated: filesCreated,
		Blueprint:    bp.Name,
	}, nil
}

// resolveAndLoad resolves the blueprint reference, loads the registry index, and loads the blueprint config.
func resolveAndLoad(opts *Opts) (*registry.ResolvedBlueprint, *config.Blueprint, error) {
	registryDir := opts.RegistryDir
	if registryDir == "" {
		return nil, nil, fmt.Errorf(
			"no registry directory provided — use --registry-dir or configure a default registry in ~/.config/forge/config.yaml",
		)
	}

	// When a registry directory is provided, we can resolve short names
	// (e.g., "go/api") without needing a default registry URL — the
	// blueprint path is extracted directly from the input.
	resolved, err := registry.Resolve(opts.BlueprintRef, opts.DefaultRegistryURL)
	if err != nil {
		// If resolution failed due to missing default registry but we have a
		// local registry dir, try resolving with a placeholder URL since we
		// only need the BlueprintPath.
		resolved, err = registry.Resolve(opts.BlueprintRef, registryDir)
		if err != nil {
			return nil, nil, fmt.Errorf("resolving blueprint: %w", err)
		}
	}

	if err := validateRegistry(registryDir, resolved); err != nil {
		return nil, nil, err
	}

	bpPath := filepath.Join(registryDir, resolved.BlueprintPath, "blueprint.yaml")

	bp, err := config.LoadBlueprint(bpPath)
	if err != nil {
		return nil, nil, fmt.Errorf("loading blueprint config: %w", err)
	}

	return resolved, bp, nil
}

// validateRegistry loads the registry index and checks the blueprint exists.
func validateRegistry(registryDir string, resolved *registry.ResolvedBlueprint) error {
	reg, err := registry.LoadIndex(registryDir)
	if err != nil && !resolved.Standalone {
		return fmt.Errorf("loading registry: %w", err)
	}

	if reg != nil && !resolved.Standalone {
		_, err = registry.FindBlueprint(reg, resolved.BlueprintPath)
		if err != nil {
			return err
		}
	}

	return nil
}

// checkOutputDirEmpty returns an error if the directory exists and is non-empty.
func checkOutputDirEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Directory doesn't exist — that's fine, it will be created.
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("checking output directory: %w", err)
	}

	if len(entries) > 0 {
		return fmt.Errorf("output directory %s is not empty — use --force to overwrite", dir)
	}

	return nil
}

// resolveOutputDir determines the output directory from explicit option, project_name variable, or blueprint name.
func resolveOutputDir(explicit string, vars map[string]any, bpName string) string {
	if explicit != "" {
		return explicit
	}

	if name, ok := vars["project_name"].(string); ok && name != "" {
		return name
	}

	return bpName
}

// renderFiles renders all files from the FileSet to the output directory.
func renderFiles(fileSet *defaults.FileSet, vars map[string]any, outputDir string, rename map[string]string) (int, error) {
	renderer := tmpl.NewRenderer()
	filesCreated := 0

	for _, entry := range fileSet.Entries() {
		if err := writeFile(renderer, entry, vars, outputDir, rename); err != nil {
			return 0, fmt.Errorf("writing file %s: %w", entry.RelPath, err)
		}

		filesCreated++
	}

	return filesCreated, nil
}

// writeFile renders a single file and writes it to the output directory.
func writeFile(
	renderer *tmpl.Renderer,
	entry *defaults.FileEntry,
	vars map[string]any,
	outputDir string,
	rename map[string]string,
) error {
	// Render path templates (e.g., {{project_name}}/cmd/main.go).
	renderedPath, err := renderer.RenderPath(entry.RelPath, vars)
	if err != nil {
		return fmt.Errorf("rendering path %q: %w", entry.RelPath, err)
	}

	// Apply rename rules.
	renderedPath = applyRename(renderedPath, rename, vars)

	// Strip .tmpl extension.
	renderedPath = tmpl.StripTemplateExtension(renderedPath)

	destPath := filepath.Join(outputDir, renderedPath)

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
		return fmt.Errorf("creating directory for %s: %w", destPath, err)
	}

	if entry.IsTemplate {
		// Render template content.
		content, err := renderer.RenderFile(entry.AbsPath, vars)
		if err != nil {
			return fmt.Errorf("rendering template %s: %w", entry.AbsPath, err)
		}

		return os.WriteFile(destPath, content, 0o644)
	}

	// Copy verbatim.
	content, err := os.ReadFile(entry.AbsPath)
	if err != nil {
		return fmt.Errorf("reading source %s: %w", entry.AbsPath, err)
	}

	return os.WriteFile(destPath, content, 0o644)
}

// applyRename applies rename rules to a rendered path.
// Rename rules map template patterns to replacement patterns.
func applyRename(path string, rename map[string]string, vars map[string]any) string {
	if len(rename) == 0 {
		return path
	}

	renderer := tmpl.NewRenderer()

	for pattern, replacement := range rename {
		// Render the pattern with variables using RenderPath to support
		// shorthand {{varname}} syntax (without dot prefix).
		renderedPattern, err := renderer.RenderPath(pattern, vars)
		if err != nil {
			continue
		}

		// Render the replacement with variables.
		renderedReplacement, err := renderer.RenderString(replacement, vars)
		if err != nil {
			continue
		}

		if len(path) >= len(renderedPattern) && path[:len(renderedPattern)] == renderedPattern {
			remainder := path[len(renderedPattern):]

			// "." means "current directory" — just use the remainder directly.
			if renderedReplacement == "." {
				path = remainder
			} else {
				path = renderedReplacement + remainder
			}
		}
	}

	return path
}

// resolveTools resolves the merged tool list from registry and blueprint declarations.
func resolveTools(opts *Opts, bp *config.Blueprint, vars map[string]any, logger *slog.Logger) ([]tools.ResolvedTool, error) {
	if opts.NoTools {
		return nil, nil
	}

	// Build category tools path.
	categoryToolsPath := ""
	if opts.RegistryDir != "" {
		parts := strings.SplitN(bp.Name, "-", 2)
		if len(parts) > 0 {
			candidate := filepath.Join(opts.RegistryDir, parts[0], "_defaults", "tools.yaml")
			if _, err := os.Stat(candidate); err == nil {
				categoryToolsPath = candidate
			}
		}
	}

	// Load registry tools.
	var registryTools []config.Tool
	if opts.RegistryDir != "" {
		reg, err := registry.LoadIndex(opts.RegistryDir)
		if err == nil {
			registryTools = reg.Tools
		}
	}

	resolved, err := tools.ResolveTools(registryTools, categoryToolsPath, bp.Tools, vars)
	if err != nil {
		return nil, fmt.Errorf("resolving tools: %w", err)
	}

	logger.Debug("resolved tools", "count", len(resolved))

	return resolved, nil
}

// computeFileHashes reads the written output files and populates SHA256 hashes
// in the lockfile entries. Errors are logged but don't fail the operation since
// hashes are used for drift detection only.
func computeFileHashes(outputDir string, lock *lockfile.Lockfile) {
	for i := range lock.Defaults {
		d := &lock.Defaults[i]
		// Strip .tmpl extension from path to match the rendered output file.
		renderedPath := tmpl.StripTemplateExtension(d.Path)
		content, err := os.ReadFile(filepath.Clean(filepath.Join(outputDir, renderedPath)))

		if err == nil {
			d.Hash = lockfile.ContentHash(content)
		}
	}

	for i := range lock.ManagedFiles {
		mf := &lock.ManagedFiles[i]
		content, err := os.ReadFile(filepath.Clean(filepath.Join(outputDir, mf.Path)))

		if err == nil {
			mf.Hash = lockfile.ContentHash(content)
		}
	}
}

// buildLockfile creates a Lockfile from the create operation results.
func buildLockfile(
	resolved *registry.ResolvedBlueprint,
	bp *config.Blueprint,
	vars map[string]any,
	fileSet *defaults.FileSet,
	resolvedTools []tools.ResolvedTool,
	forgeVersion string,
	registryURL string,
) *lockfile.Lockfile {
	now := time.Now().UTC()

	// Use the explicit registry URL if provided (from --registry-dir),
	// falling back to the resolver's URL.
	lockRegistryURL := resolved.RegistryURL
	if registryURL != "" {
		lockRegistryURL = registryURL
	}

	lock := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			RegistryURL: lockRegistryURL,
			Name:        bp.Name,
			Path:        resolved.BlueprintPath,
			Ref:         resolved.Ref,
		},
		CreatedAt:    now,
		LastSynced:   now,
		ForgeVersion: forgeVersion,
		Variables:    vars,
	}

	// Record default file entries.
	for _, entry := range fileSet.Entries() {
		if entry.SourceLayer != defaults.LayerBlueprint {
			lock.Defaults = append(lock.Defaults, lockfile.DefaultEntry{
				Path:     entry.RelPath,
				Source:   entry.SourceLayer.String(),
				Strategy: "overwrite",
			})
		}
	}

	// Record managed files from blueprint config.
	for i := range bp.Sync.ManagedFiles {
		mf := &bp.Sync.ManagedFiles[i]
		lock.ManagedFiles = append(lock.ManagedFiles, lockfile.ManagedFileEntry{
			Path:     mf.Path,
			Strategy: mf.Strategy,
		})
	}

	// Record resolved tools.
	for i := range resolvedTools {
		t := &resolvedTools[i]
		lock.Tools = append(lock.Tools, lockfile.ToolEntry{
			Name:    t.Name,
			Version: t.Version,
			Source: lockfile.ToolSourceEntry{
				Type:         t.Source.Type,
				Repo:         t.Source.Repo,
				AssetPattern: t.Source.AssetPattern,
				URL:          t.Source.URL,
				Module:       t.Source.Module,
				Package:      t.Source.Package,
				Crate:        t.Source.Crate,
			},
			InstallPath: t.InstallPath,
		})
	}

	return lock
}
