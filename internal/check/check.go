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
)

// Opts configures the check operation.
type Opts struct {
	// ProjectDir is the root of the scaffolded project (defaults to ".").
	ProjectDir string
	// RegistryDir is the local path to the current registry content.
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
	StatusUpToDate FileStatus = "up-to-date"
	StatusModified FileStatus = "modified"
	StatusMissing  FileStatus = "missing"
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

	result := &Result{}

	// Check defaults.
	for i := range lock.Defaults {
		d := &lock.Defaults[i]
		localPath := filepath.Join(projectDir, d.Path)
		update := checkFile(localPath, d.Path, d.Source)
		result.DefaultsUpdates = append(result.DefaultsUpdates, update)
	}

	// Check managed files.
	for i := range lock.ManagedFiles {
		mf := &lock.ManagedFiles[i]
		localPath := filepath.Join(projectDir, mf.Path)
		update := checkFile(localPath, mf.Path, mf.Strategy)
		result.ManagedUpdates = append(result.ManagedUpdates, update)
	}

	return result, renderResult(opts.Writer, opts.OutputFormat, result)
}

func checkFile(localPath, relPath, source string) FileUpdate {
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		return FileUpdate{Path: relPath, Status: StatusMissing, Source: source}
	}

	return FileUpdate{Path: relPath, Status: StatusUpToDate, Source: source}
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
	case StatusModified:
		return "modified"
	case StatusMissing:
		return "MISSING"
	default:
		return string(s)
	}
}
