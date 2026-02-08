// Package sync implements file synchronization between a local project and its source blueprint.
package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/donaldgifford/forge/internal/lockfile"
	tmpl "github.com/donaldgifford/forge/internal/template"
)

// Opts configures the sync operation.
type Opts struct {
	// ProjectDir is the root of the scaffolded project.
	ProjectDir string
	// RegistryDir is the local path to the current registry content.
	RegistryDir string
	// BaseDir is the local path to the base (last synced) registry content.
	// Used as the common ancestor for three-way merge.
	BaseDir string
	// DryRun prints what would change without writing.
	DryRun bool
	// Force skips confirmation prompts.
	Force bool
	// FileFilter limits sync to a single file path.
	FileFilter string
}

// Result holds the outcome of a sync operation.
type Result struct {
	Updated   []string
	Skipped   []string
	Conflicts []string
}

// Run executes the sync workflow.
func Run(opts *Opts) (*Result, error) {
	projectDir := opts.ProjectDir
	if projectDir == "" {
		projectDir = "."
	}

	lockPath := filepath.Join(projectDir, lockfile.FileName)

	lock, err := lockfile.Read(lockPath)
	if err != nil {
		return nil, fmt.Errorf("reading lockfile: %w", err)
	}

	result := &Result{}
	renderer := tmpl.NewRenderer()

	// Sync defaults.
	for i := range lock.Defaults {
		d := &lock.Defaults[i]

		if opts.FileFilter != "" && d.Path != opts.FileFilter {
			continue
		}

		if err := syncDefault(opts, d, lock.Variables, renderer, result); err != nil {
			return nil, fmt.Errorf("syncing default %s: %w", d.Path, err)
		}
	}

	// Sync managed files.
	for i := range lock.ManagedFiles {
		mf := &lock.ManagedFiles[i]

		if opts.FileFilter != "" && mf.Path != opts.FileFilter {
			continue
		}

		if err := syncManagedFile(opts, mf, lock, renderer, result); err != nil {
			return nil, fmt.Errorf("syncing managed file %s: %w", mf.Path, err)
		}
	}

	// Update lockfile if not dry-run.
	if !opts.DryRun && len(result.Updated) > 0 {
		lock.LastSynced = time.Now().UTC()

		if err := lockfile.Write(lockPath, lock); err != nil {
			return nil, fmt.Errorf("updating lockfile: %w", err)
		}
	}

	return result, nil
}

func syncDefault(
	opts *Opts,
	d *lockfile.DefaultEntry,
	vars map[string]any,
	renderer *tmpl.Renderer,
	result *Result,
) error {
	sourcePath := findSourceFile(opts.RegistryDir, d.Path)
	if sourcePath == "" {
		result.Skipped = append(result.Skipped, d.Path)

		return nil
	}

	sourceContent, err := readSourceContent(sourcePath, vars, renderer)
	if err != nil {
		return err
	}

	localPath := filepath.Join(opts.ProjectDir, d.Path)

	return applyOverwrite(localPath, sourceContent, opts.DryRun, result)
}

func syncManagedFile(
	opts *Opts,
	mf *lockfile.ManagedFileEntry,
	lock *lockfile.Lockfile,
	renderer *tmpl.Renderer,
	result *Result,
) error {
	sourcePath := findSourceFile(opts.RegistryDir, mf.Path)
	if sourcePath == "" {
		// Check in blueprint directory.
		sourcePath = findBlueprintFile(opts.RegistryDir, lock.Blueprint.Path, mf.Path)
	}

	if sourcePath == "" {
		result.Skipped = append(result.Skipped, mf.Path)

		return nil
	}

	sourceContent, err := readSourceContent(sourcePath, lock.Variables, renderer)
	if err != nil {
		return err
	}

	localPath := filepath.Join(opts.ProjectDir, mf.Path)

	if mf.Strategy == "merge" {
		return applyMerge(opts, mf, lock, renderer, localPath, sourceContent, result)
	}

	return applyOverwrite(localPath, sourceContent, opts.DryRun, result)
}

func applyMerge(
	opts *Opts,
	mf *lockfile.ManagedFileEntry,
	lock *lockfile.Lockfile,
	renderer *tmpl.Renderer,
	localPath string,
	remoteContent []byte,
	result *Result,
) error {
	// Read local file.
	localContent, err := os.ReadFile(filepath.Clean(localPath))
	if err != nil {
		if os.IsNotExist(err) {
			// No local file — accept remote content directly.
			return applyOverwrite(localPath, remoteContent, opts.DryRun, result)
		}

		return fmt.Errorf("reading local file %s: %w", localPath, err)
	}

	// Resolve base content from the base registry directory.
	baseContent, err := resolveBaseContent(opts, mf, lock, renderer)
	if err != nil {
		// If base is unavailable, fall back to overwrite.
		return applyOverwrite(localPath, remoteContent, opts.DryRun, result)
	}

	merged := ThreeWayMerge(baseContent, localContent, remoteContent)

	if merged.HasConflicts {
		result.Conflicts = append(result.Conflicts, mf.Path)
	}

	return applyOverwrite(localPath, merged.Content, opts.DryRun, result)
}

func resolveBaseContent(
	opts *Opts,
	mf *lockfile.ManagedFileEntry,
	lock *lockfile.Lockfile,
	renderer *tmpl.Renderer,
) ([]byte, error) {
	if opts.BaseDir == "" {
		return nil, fmt.Errorf("no base directory configured")
	}

	basePath := findSourceFile(opts.BaseDir, mf.Path)
	if basePath == "" {
		basePath = findBlueprintFile(opts.BaseDir, lock.Blueprint.Path, mf.Path)
	}

	if basePath == "" {
		return nil, fmt.Errorf("base file not found for %s", mf.Path)
	}

	return readSourceContent(basePath, lock.Variables, renderer)
}

// findBlueprintFile looks for a file in the blueprint's own directory.
func findBlueprintFile(registryDir, blueprintPath, relPath string) string {
	candidate := filepath.Join(registryDir, blueprintPath, relPath)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	return ""
}

// findSourceFile looks for a file in known registry locations.
func findSourceFile(registryDir, relPath string) string {
	// Check direct path first.
	candidate := filepath.Join(registryDir, relPath)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	// Check _defaults/.
	candidate = filepath.Join(registryDir, "_defaults", relPath)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	return ""
}

func readSourceContent(sourcePath string, vars map[string]any, renderer *tmpl.Renderer) ([]byte, error) {
	if tmpl.IsTemplate(sourcePath) {
		content, err := renderer.RenderFile(sourcePath, vars)
		if err != nil {
			return nil, fmt.Errorf("rendering template %s: %w", sourcePath, err)
		}

		return content, nil
	}

	content, err := os.ReadFile(filepath.Clean(sourcePath))
	if err != nil {
		return nil, fmt.Errorf("reading source %s: %w", sourcePath, err)
	}

	return content, nil
}
