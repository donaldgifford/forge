// Package list implements the forge list command for browsing registry blueprints.
package list

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/registry"
)

// Opts configures the list operation.
type Opts struct {
	// RegistryDir is the local path to the registry directory.
	RegistryDir string
	// TagFilter limits output to blueprints matching this tag.
	TagFilter string
	// OutputFormat is "table" or "json".
	OutputFormat string
	// Writer is the output destination.
	Writer io.Writer
}

// BlueprintInfo represents a blueprint in list output.
type BlueprintInfo struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Path        string   `json:"path"`
}

// Run lists blueprints from a registry.
func Run(opts *Opts) error {
	reg, err := registry.LoadIndex(opts.RegistryDir)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	entries := filterByTag(reg.Blueprints, opts.TagFilter)

	switch opts.OutputFormat {
	case "json":
		return renderJSON(opts.Writer, entries)
	default:
		return renderTable(opts.Writer, entries)
	}
}

func filterByTag(entries []config.BlueprintEntry, tag string) []config.BlueprintEntry {
	if tag == "" {
		return entries
	}

	var filtered []config.BlueprintEntry

	for i := range entries {
		if hasTag(entries[i].Tags, tag) {
			filtered = append(filtered, entries[i])
		}
	}

	return filtered
}

func hasTag(tags []string, target string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, target) {
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
	infos := make([]BlueprintInfo, 0, len(entries))

	for i := range entries {
		e := &entries[i]
		infos = append(infos, BlueprintInfo{
			Name:        e.Name,
			Version:     e.Version,
			Description: e.Description,
			Tags:        e.Tags,
			Path:        e.Path,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(infos)
}
