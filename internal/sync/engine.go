// Package sync implements file synchronization between a local project and its source blueprint.
package sync

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/lockfile"
	tmpl "github.com/donaldgifford/forge/internal/template"
	"github.com/donaldgifford/forge/internal/varsfile"
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
	// Force skips confirmation prompts. When VarsFiles is non-empty,
	// the CLI requires Force=true before reaching this point (per
	// IMPL-0008 OQ-4) — sync trusts its caller to have enforced that.
	Force bool
	// FileFilter limits sync to a single file path.
	FileFilter string
	// VarsFiles is the ordered list of `--var-file` paths supplied at
	// the CLI (IMPL-0008). When non-empty, sync.Run overlays the
	// loaded values onto the lockfile-derived variable map and
	// rewrites the lockfile with the merged result.
	VarsFiles []string
}

// Result holds the outcome of a sync operation.
type Result struct {
	Updated       []string
	Skipped       []string
	Conflicts     []string
	ConflictFiles []ConflictFile

	// UnknownVarsFileKeys lists keys declared in any --var-file input
	// that don't correspond to a declared blueprint variable. The CLI
	// surfaces these as a warning (IMPL-0008 OQ-7).
	UnknownVarsFileKeys []string

	// Deprecations are non-fatal v0.7 transition notices produced by
	// the blueprint loader (today: the `int`-as-alias-for-`number`
	// warning). The CLI surfaces these via ui.Warningf per
	// IMPL-0009 OQ-3.
	Deprecations []config.Deprecation
}

// Run executes the sync workflow.
func Run(opts *Opts) (*Result, error) {
	projectDir := opts.ProjectDir
	if projectDir == "" {
		projectDir = "."
	}

	lockPath := filepath.Join(projectDir, lockfile.HCLFileName)

	lock, err := lockfile.LoadLockfile(projectDir)
	if err != nil {
		return nil, fmt.Errorf("reading lockfile: %w", err)
	}

	result := &Result{}
	renderer := tmpl.NewRenderer()

	bpVars, deprecations := loadBlueprintVariables(opts.RegistryDir, lock.Blueprint.Path)
	result.Deprecations = deprecations

	ctyVars, err := lockfile.ToCtyValues(lock.Variables, bpVars)
	if err != nil {
		return nil, fmt.Errorf("converting lockfile variables to cty: %w", err)
	}

	// IMPL-0008 Phase C: overlay --var-file values onto the
	// lockfile-derived map. The CLI enforces the --force requirement
	// before reaching here; sync just needs to merge and persist.
	if err := applyVarsFileOverlay(opts.VarsFiles, bpVars, ctyVars, result); err != nil {
		return nil, err
	}

	// IMPL-0009 Phase C: re-validate against the (possibly
	// vars-file-overlaid) scope before touching any files. Sync paths
	// that flow through here treat the lockfile as authoritative
	// state, so a bad overlay should abort early.
	if errs := config.EvaluateValidations(bpVars, ctyVars); len(errs) > 0 {
		return nil, fmt.Errorf("validating variables: %w", config.JoinErrors(errs))
	}

	// Sync defaults.
	for i := range lock.Defaults {
		d := &lock.Defaults[i]

		if opts.FileFilter != "" && d.Path != opts.FileFilter {
			continue
		}

		if err := syncDefault(opts, d, ctyVars, renderer, result); err != nil {
			return nil, fmt.Errorf("syncing default %s: %w", d.Path, err)
		}
	}

	// Sync managed files.
	for i := range lock.ManagedFiles {
		mf := &lock.ManagedFiles[i]

		if opts.FileFilter != "" && mf.Path != opts.FileFilter {
			continue
		}

		if err := syncManagedFile(opts, mf, lock, ctyVars, renderer, result); err != nil {
			return nil, fmt.Errorf("syncing managed file %s: %w", mf.Path, err)
		}
	}

	if err := persistLockfile(opts, lockPath, lock, ctyVars, result); err != nil {
		return nil, err
	}

	return result, nil
}

// persistLockfile rewrites the lockfile to record updated timestamps,
// content hashes, and (when --var-file was used) the overlaid
// variable values. No-op when DryRun is set or there's nothing to
// record. Vars-file overlays force a rewrite even when no managed
// files changed — the new variable values themselves are persistent
// state worth recording.
func persistLockfile(
	opts *Opts,
	lockPath string,
	lock *lockfile.Lockfile,
	ctyVars map[string]cty.Value,
	result *Result,
) error {
	varsFileChanged := len(opts.VarsFiles) > 0
	if opts.DryRun || (len(result.Updated) == 0 && !varsFileChanged) {
		return nil
	}

	lock.LastSynced = time.Now().UTC()

	updateFileHashes(opts.ProjectDir, lock)

	if varsFileChanged {
		lock.Variables = lockfile.FromCtyValues(ctyVars)
	}

	if err := lockfile.WriteHCL(lockPath, lock); err != nil {
		return fmt.Errorf("updating lockfile: %w", err)
	}

	return nil
}

// applyVarsFileOverlay loads any --var-file inputs and merges the
// resolved values into ctyVars in place. Unknown keys land on
// result.UnknownVarsFileKeys for the caller to surface. Returns nil
// (no-op) when no vars-file paths were supplied.
func applyVarsFileOverlay(
	paths []string,
	bpVars []config.Variable,
	ctyVars map[string]cty.Value,
	result *Result,
) error {
	if len(paths) == 0 {
		return nil
	}

	overlay, unknown, err := varsfile.Load(paths, bpVars)
	if err != nil {
		return fmt.Errorf("loading vars file: %w", err)
	}

	maps.Copy(ctyVars, overlay)
	result.UnknownVarsFileKeys = unknown

	return nil
}

func syncDefault(
	opts *Opts,
	d *lockfile.DefaultEntry,
	vars map[string]cty.Value,
	renderer tmpl.Renderer,
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

	// The lockfile stores the source path (e.g. "greet.txt.tmpl"), but
	// the rendered output lives at the stripped name ("greet.txt").
	localPath := filepath.Join(opts.ProjectDir, tmpl.StripTemplateExtension(d.Path))

	return applyOverwrite(localPath, sourceContent, opts.DryRun, result)
}

func syncManagedFile(
	opts *Opts,
	mf *lockfile.ManagedFileEntry,
	lock *lockfile.Lockfile,
	vars map[string]cty.Value,
	renderer tmpl.Renderer,
	result *Result,
) error {
	sourcePath := findSourceFile(opts.RegistryDir, mf.Path)
	if sourcePath == "" {
		sourcePath = findBlueprintFile(opts.RegistryDir, lock.Blueprint.Path, mf.Path)
	}

	if sourcePath == "" {
		result.Skipped = append(result.Skipped, mf.Path)

		return nil
	}

	sourceContent, err := readSourceContent(sourcePath, vars, renderer)
	if err != nil {
		return err
	}

	localPath := filepath.Join(opts.ProjectDir, mf.Path)

	if mf.Strategy == "merge" {
		return applyMerge(opts, mf, lock, vars, renderer, localPath, sourceContent, result)
	}

	return applyOverwrite(localPath, sourceContent, opts.DryRun, result)
}

func applyMerge(
	opts *Opts,
	mf *lockfile.ManagedFileEntry,
	lock *lockfile.Lockfile,
	vars map[string]cty.Value,
	renderer tmpl.Renderer,
	localPath string,
	remoteContent []byte,
	result *Result,
) error {
	localContent, err := os.ReadFile(filepath.Clean(localPath))
	if err != nil {
		if os.IsNotExist(err) {
			return applyOverwrite(localPath, remoteContent, opts.DryRun, result)
		}

		return fmt.Errorf("reading local file %s: %w", localPath, err)
	}

	baseContent, err := resolveBaseContent(opts, mf, lock, vars, renderer)
	if err != nil {
		return applyOverwrite(localPath, remoteContent, opts.DryRun, result)
	}

	merged := ThreeWayMerge(baseContent, localContent, remoteContent)

	if merged.HasConflicts {
		result.Conflicts = append(result.Conflicts, mf.Path)
		result.ConflictFiles = append(result.ConflictFiles, ConflictFile{
			Path:      mf.Path,
			Conflicts: merged.Conflicts,
		})
	}

	return applyOverwrite(localPath, merged.Content, opts.DryRun, result)
}

func resolveBaseContent(
	opts *Opts,
	mf *lockfile.ManagedFileEntry,
	lock *lockfile.Lockfile,
	vars map[string]cty.Value,
	renderer tmpl.Renderer,
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

	return readSourceContent(basePath, vars, renderer)
}

// updateFileHashes recomputes SHA256 hashes for all tracked files in the lockfile.
func updateFileHashes(projectDir string, lock *lockfile.Lockfile) {
	for i := range lock.Defaults {
		d := &lock.Defaults[i]
		renderedPath := tmpl.StripTemplateExtension(d.Path)
		content, err := os.ReadFile(filepath.Clean(filepath.Join(projectDir, renderedPath)))

		if err == nil {
			d.Hash = lockfile.ContentHash(content)
		}
	}

	for i := range lock.ManagedFiles {
		mf := &lock.ManagedFiles[i]
		content, err := os.ReadFile(filepath.Clean(filepath.Join(projectDir, mf.Path)))

		if err == nil {
			mf.Hash = lockfile.ContentHash(content)
		}
	}
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

// loadBlueprintVariables reads the blueprint.hcl under the registry directory
// to recover the declared variable types plus any non-fatal Deprecation
// notices for the caller to surface. Returns nil/nil when no registry is
// configured or the blueprint cannot be loaded — callers fall back to runtime
// type inference in that case.
func loadBlueprintVariables(registryDir, blueprintPath string) ([]config.Variable, []config.Deprecation) {
	if registryDir == "" || blueprintPath == "" {
		return nil, nil
	}

	bpPath := filepath.Join(registryDir, blueprintPath, "blueprint.hcl")

	bp, err := config.LoadBlueprint(bpPath)
	if err != nil {
		return nil, nil
	}

	return bp.Variables, bp.Deprecations
}

func readSourceContent(sourcePath string, vars map[string]cty.Value, renderer tmpl.Renderer) ([]byte, error) {
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
