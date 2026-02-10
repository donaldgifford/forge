// Package info displays detailed blueprint information.
package info

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/donaldgifford/forge/internal/config"
)

// Opts configures the info command.
type Opts struct {
	// Blueprint is the loaded blueprint configuration.
	Blueprint *config.Blueprint
	// Writer is the output destination.
	Writer io.Writer
	// OutputFormat is "text" or "json".
	OutputFormat string
}

// Run displays blueprint information.
func Run(opts *Opts) error {
	switch opts.OutputFormat {
	case "json":
		return renderJSON(opts.Writer, opts.Blueprint)
	default:
		return renderText(opts.Writer, opts.Blueprint)
	}
}

func renderText(w io.Writer, bp *config.Blueprint) error {
	if err := renderHeader(w, bp); err != nil {
		return err
	}

	if err := renderSections(w, bp); err != nil {
		return err
	}

	return nil
}

func renderHeader(w io.Writer, bp *config.Blueprint) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintf(tw, "Name:\t%s\n", bp.Name); err != nil {
		return err
	}

	if bp.Version != "" {
		if _, err := fmt.Fprintf(tw, "Version:\t%s\n", bp.Version); err != nil {
			return err
		}
	}

	if bp.Description != "" {
		if _, err := fmt.Fprintf(tw, "Description:\t%s\n", bp.Description); err != nil {
			return err
		}
	}

	if len(bp.Tags) > 0 {
		if _, err := fmt.Fprintf(tw, "Tags:\t%s\n", strings.Join(bp.Tags, ", ")); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func renderSections(w io.Writer, bp *config.Blueprint) error {
	if len(bp.Variables) > 0 {
		if _, err := fmt.Fprintln(w, "\nVariables:"); err != nil {
			return err
		}

		if err := renderVariables(w, bp.Variables); err != nil {
			return err
		}
	}

	if len(bp.Sync.ManagedFiles) > 0 {
		if _, err := fmt.Fprintln(w, "\nManaged Files:"); err != nil {
			return err
		}

		if err := renderManagedFiles(w, bp.Sync.ManagedFiles); err != nil {
			return err
		}
	}

	return nil
}

func renderVariables(w io.Writer, vars []config.Variable) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(tw, "  NAME\tTYPE\tDEFAULT\tREQUIRED"); err != nil {
		return err
	}

	for _, v := range vars {
		required := ""
		if v.Required {
			required = "yes"
		}

		if _, err := fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", v.Name, v.Type, v.Default, required); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func renderManagedFiles(w io.Writer, files []config.ManagedFile) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(tw, "  PATH\tSTRATEGY"); err != nil {
		return err
	}

	for i := range files {
		f := &files[i]
		if _, err := fmt.Fprintf(tw, "  %s\t%s\n", f.Path, f.Strategy); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func renderJSON(w io.Writer, bp *config.Blueprint) error {
	out := jsonOutput{
		Name:        bp.Name,
		Version:     bp.Version,
		Description: bp.Description,
		Tags:        bp.Tags,
		Variables:   bp.Variables,
	}

	for i := range bp.Sync.ManagedFiles {
		f := &bp.Sync.ManagedFiles[i]
		out.ManagedFiles = append(out.ManagedFiles, jsonManagedFile{
			Path:     f.Path,
			Strategy: f.Strategy,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(out)
}

type jsonOutput struct {
	Name         string            `json:"name"`
	Version      string            `json:"version,omitempty"`
	Description  string            `json:"description,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	Variables    []config.Variable `json:"variables,omitempty"`
	ManagedFiles []jsonManagedFile `json:"managed_files,omitempty"`
}

type jsonManagedFile struct {
	Path     string `json:"path"`
	Strategy string `json:"strategy"`
}
