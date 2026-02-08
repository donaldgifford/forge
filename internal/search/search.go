// Package search implements the forge search command for finding blueprints.
package search

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/registry"
)

// Opts configures the search operation.
type Opts struct {
	// Query is the search string to match against name, description, and tags.
	Query string
	// RegistryDir is the local path to the registry directory.
	RegistryDir string
	// OutputFormat is "table" or "json".
	OutputFormat string
	// Writer is the output destination.
	Writer io.Writer
}

// Result represents a matched blueprint.
type Result struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Path        string   `json:"path"`
}

// Run searches for blueprints matching the query.
func Run(opts *Opts) error {
	reg, err := registry.LoadIndex(opts.RegistryDir)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	matches := search(reg.Blueprints, opts.Query)

	switch opts.OutputFormat {
	case "json":
		return renderJSON(opts.Writer, matches)
	default:
		return renderTable(opts.Writer, matches)
	}
}

func search(entries []config.BlueprintEntry, query string) []config.BlueprintEntry {
	if query == "" {
		return entries
	}

	q := strings.ToLower(query)
	var matches []config.BlueprintEntry

	for i := range entries {
		if matchesEntry(&entries[i], q) {
			matches = append(matches, entries[i])
		}
	}

	return matches
}

func matchesEntry(entry *config.BlueprintEntry, query string) bool {
	if strings.Contains(strings.ToLower(entry.Name), query) {
		return true
	}

	if strings.Contains(strings.ToLower(entry.Description), query) {
		return true
	}

	for _, tag := range entry.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}

	return false
}

func renderTable(w io.Writer, entries []config.BlueprintEntry) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(tw, "NAME\tVERSION\tDESCRIPTION\tTAGS"); err != nil {
		return err
	}

	for i := range entries {
		e := &entries[i]
		tags := strings.Join(e.Tags, ", ")

		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Name, e.Version, e.Description, tags); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func renderJSON(w io.Writer, entries []config.BlueprintEntry) error {
	results := make([]Result, 0, len(entries))

	for i := range entries {
		e := &entries[i]
		results = append(results, Result{
			Name:        e.Name,
			Version:     e.Version,
			Description: e.Description,
			Tags:        e.Tags,
			Path:        e.Path,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(results)
}
