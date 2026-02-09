// Package check compares a local project against its source blueprint for drift.
package check

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/donaldgifford/forge/internal/lockfile"
	tmpl "github.com/donaldgifford/forge/internal/template"
)

// Opts configures the check operation.
type Opts struct {
	// ProjectDir is the root of the scaffolded project (defaults to ".").
	ProjectDir string
	// RegistryDir is the local path to the current registry content.
	// When set, enables three-way comparison (local vs lockfile vs registry).
	RegistryDir string
	// OutputFormat is "text" or "json".
	OutputFormat string
	// Writer is the output destination.
	Writer io.Writer
}

// FileStatus indicates the drift state of a file.
type FileStatus string

// File drift statuses.
const (
	StatusUpToDate        FileStatus = "up-to-date"
	StatusModified        FileStatus = "modified"
	StatusMissing         FileStatus = "missing"
	StatusModifiedLocally FileStatus = "modified-locally"
	StatusUpstreamChanged FileStatus = "upstream-changed"
	StatusBothChanged     FileStatus = "both-changed"
)

// FileUpdate describes the drift state of a single file.
type FileUpdate struct {
	Path   string     `json:"path"`
	Status FileStatus `json:"status"`
	Source string     `json:"source"`
}

// Result holds the check comparison results.
type Result struct {
	DefaultsUpdates []FileUpdate `json:"defaults_updates"`
	ManagedUpdates  []FileUpdate `json:"managed_updates"`
}

// Run executes the check workflow.
func Run(opts *Opts) (*Result, error) {
	projectDir := opts.ProjectDir
	if projectDir == "" {
		projectDir = "."
	}

	lockPath := filepath.Join(projectDir, lockfile.FileName)

	lock, err := lockfile.Read(lockPath)
	if err != nil {
		return nil, fmt.Errorf("reading lockfile: %w (is this a forge project?)", err)
	}

	renderer := tmpl.NewRenderer()
	result := &Result{}

	// Check defaults.
	for i := range lock.Defaults {
		d := &lock.Defaults[i]
		// Strip .tmpl extension from path to match rendered output file.
		renderedPath := tmpl.StripTemplateExtension(d.Path)
		localPath := filepath.Join(projectDir, renderedPath)

		registryHash := resolveRegistryHash(opts.RegistryDir, d.Path, lock.Variables, renderer)
		update := checkFile(localPath, renderedPath, d.Source, d.Hash, registryHash)
		result.DefaultsUpdates = append(result.DefaultsUpdates, update)
	}

	// Check managed files.
	for i := range lock.ManagedFiles {
		mf := &lock.ManagedFiles[i]
		localPath := filepath.Join(projectDir, mf.Path)

		registryHash := resolveRegistryHashForManaged(
			opts.RegistryDir, lock.Blueprint.Path, mf.Path, lock.Variables, renderer,
		)
		update := checkFile(localPath, mf.Path, mf.Strategy, mf.Hash, registryHash)
		result.ManagedUpdates = append(result.ManagedUpdates, update)
	}

	return result, renderResult(opts.Writer, opts.OutputFormat, result)
}

// checkFile determines the drift status of a file.
// lockfileHash is the hash stored at create/sync time.
// registryHash is the hash of the current registry source (empty if no registry).
func checkFile(localPath, relPath, source, lockfileHash, registryHash string) FileUpdate {
	content, err := os.ReadFile(filepath.Clean(localPath))
	if err != nil {
		return FileUpdate{Path: relPath, Status: StatusMissing, Source: source}
	}

	// If no hash stored in lockfile, existence is sufficient.
	if lockfileHash == "" {
		return FileUpdate{Path: relPath, Status: StatusUpToDate, Source: source}
	}

	currentHash := lockfile.ContentHash(content)
	localChanged := currentHash != lockfileHash

	// Without registry, only compare local vs lockfile.
	if registryHash == "" {
		if localChanged {
			return FileUpdate{Path: relPath, Status: StatusModified, Source: source}
		}

		return FileUpdate{Path: relPath, Status: StatusUpToDate, Source: source}
	}

	// Three-way comparison: local vs lockfile vs registry.
	upstreamChanged := registryHash != lockfileHash

	switch {
	case localChanged && upstreamChanged:
		return FileUpdate{Path: relPath, Status: StatusBothChanged, Source: source}
	case localChanged:
		return FileUpdate{Path: relPath, Status: StatusModifiedLocally, Source: source}
	case upstreamChanged:
		return FileUpdate{Path: relPath, Status: StatusUpstreamChanged, Source: source}
	default:
		return FileUpdate{Path: relPath, Status: StatusUpToDate, Source: source}
	}
}

// resolveRegistryHash computes the content hash of a defaults file from the registry.
// Returns empty string if registry dir is not set or the file cannot be resolved.
func resolveRegistryHash(
	registryDir, relPath string,
	vars map[string]any,
	renderer *tmpl.Renderer,
) string {
	if registryDir == "" {
		return ""
	}

	sourcePath := findSourceFile(registryDir, relPath)
	if sourcePath == "" {
		return ""
	}

	content, err := readSourceContent(sourcePath, vars, renderer)
	if err != nil {
		return ""
	}

	return lockfile.ContentHash(content)
}

// resolveRegistryHashForManaged computes the content hash of a managed file from the registry.
func resolveRegistryHashForManaged(
	registryDir, blueprintPath, relPath string,
	vars map[string]any,
	renderer *tmpl.Renderer,
) string {
	if registryDir == "" {
		return ""
	}

	sourcePath := findSourceFile(registryDir, relPath)
	if sourcePath == "" {
		// Check in blueprint directory.
		sourcePath = findBlueprintFile(registryDir, blueprintPath, relPath)
	}

	if sourcePath == "" {
		return ""
	}

	content, err := readSourceContent(sourcePath, vars, renderer)
	if err != nil {
		return ""
	}

	return lockfile.ContentHash(content)
}

// findSourceFile looks for a file in known registry locations.
func findSourceFile(registryDir, relPath string) string {
	candidate := filepath.Join(registryDir, relPath)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	candidate = filepath.Join(registryDir, "_defaults", relPath)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	return ""
}

// findBlueprintFile looks for a file in the blueprint directory.
func findBlueprintFile(registryDir, blueprintPath, relPath string) string {
	candidate := filepath.Join(registryDir, blueprintPath, relPath)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	return ""
}

// readSourceContent reads a source file, rendering templates if needed.
func readSourceContent(sourcePath string, vars map[string]any, renderer *tmpl.Renderer) ([]byte, error) {
	if tmpl.IsTemplate(sourcePath) {
		return renderer.RenderFile(sourcePath, vars)
	}

	return os.ReadFile(filepath.Clean(sourcePath))
}

func renderResult(w io.Writer, format string, result *Result) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")

		return enc.Encode(result)
	default:
		return renderText(w, result)
	}
}

func renderText(w io.Writer, result *Result) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(tw, "FILE\tSTATUS\tSOURCE"); err != nil {
		return err
	}

	for i := range result.DefaultsUpdates {
		u := &result.DefaultsUpdates[i]
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", u.Path, statusIcon(u.Status), u.Source); err != nil {
			return err
		}
	}

	for i := range result.ManagedUpdates {
		u := &result.ManagedUpdates[i]
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", u.Path, statusIcon(u.Status), u.Source); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func statusIcon(s FileStatus) string {
	switch s {
	case StatusUpToDate:
		return "ok"
	case StatusModified, StatusModifiedLocally:
		return "modified"
	case StatusMissing:
		return "MISSING"
	case StatusUpstreamChanged:
		return "upstream-changed"
	case StatusBothChanged:
		return "both-changed"
	default:
		return string(s)
	}
}
