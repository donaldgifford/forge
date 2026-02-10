package template

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

// pathVarPattern matches {{varname}} without a leading dot, pipe, or space-prefixed content.
// It normalizes shorthand path variables to Go template syntax: {{varname}} → {{.varname}}.
var pathVarPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

// Renderer renders Go text/templates with the forge custom function map.
type Renderer struct {
	funcMap template.FuncMap
}

// NewRenderer creates a Renderer with the standard forge function map.
func NewRenderer() *Renderer {
	return &Renderer{
		funcMap: FuncMap(),
	}
}

// RenderFile reads a template file and renders it with the given variables.
// File templates use missingkey=zero to allow the default function to work
// with optional variables.
func (r *Renderer) RenderFile(tmplPath string, vars map[string]any) ([]byte, error) {
	data, err := os.ReadFile(tmplPath) //nolint:gosec // template paths are from registry content, not untrusted user input
	if err != nil {
		return nil, fmt.Errorf("reading template %s: %w", tmplPath, err)
	}

	name := filepath.Base(tmplPath)

	return r.renderWithOption(name, string(data), vars, "missingkey=zero")
}

// RenderString renders an inline template string with the given variables.
func (r *Renderer) RenderString(tmpl string, vars map[string]any) (string, error) {
	result, err := r.render("inline", tmpl, vars)
	if err != nil {
		return "", err
	}

	return string(result), nil
}

// RenderPath renders template expressions in file/directory path segments.
// For example, "{{project_name}}/cmd/main.go" with vars["project_name"]="my-api"
// becomes "my-api/cmd/main.go".
//
// Path templates support shorthand syntax: {{varname}} is normalized to {{.varname}}
// so that directory names like "{{project_name}}" work without requiring the dot prefix.
func (r *Renderer) RenderPath(path string, vars map[string]any) (string, error) {
	if !strings.Contains(path, "{{") {
		return path, nil
	}

	// Normalize {{varname}} → {{.varname}} for path convenience.
	normalized := normalizePathTemplate(path)

	result, err := r.RenderString(normalized, vars)
	if err != nil {
		return "", fmt.Errorf("rendering path %q: %w", path, err)
	}

	return result, nil
}

// normalizePathTemplate converts shorthand {{varname}} to {{.varname}} in path templates.
// This allows directory names like "{{project_name}}" to work without requiring the
// Go template dot prefix. Expressions that already use dot notation (e.g., {{.varname}})
// or contain function calls/pipes are left unchanged.
func normalizePathTemplate(path string) string {
	return pathVarPattern.ReplaceAllStringFunc(path, func(match string) string {
		inner := strings.TrimSpace(match[2 : len(match)-2])

		// Already has a dot prefix — leave it alone.
		if strings.HasPrefix(inner, ".") {
			return match
		}

		return "{{." + inner + "}}"
	})
}

// StripTemplateExtension removes the .tmpl extension from a filename.
func StripTemplateExtension(path string) string {
	return strings.TrimSuffix(path, ".tmpl")
}

// IsTemplate returns true if the path ends with .tmpl.
func IsTemplate(path string) bool {
	return strings.HasSuffix(path, ".tmpl")
}

func (r *Renderer) render(name, text string, vars map[string]any) ([]byte, error) {
	return r.renderWithOption(name, text, vars, "missingkey=error")
}

func (r *Renderer) renderWithOption(name, text string, vars map[string]any, option string) ([]byte, error) {
	tmpl, err := template.New(name).
		Funcs(r.funcMap).
		Option(option).
		Parse(text)
	if err != nil {
		return nil, fmt.Errorf("parsing template %q: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("executing template %q: %w", name, err)
	}

	return buf.Bytes(), nil
}
