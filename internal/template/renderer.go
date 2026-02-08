package template

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

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
func (r *Renderer) RenderFile(tmplPath string, vars map[string]any) ([]byte, error) {
	data, err := os.ReadFile(tmplPath) //nolint:gosec // template paths are from registry content, not untrusted user input
	if err != nil {
		return nil, fmt.Errorf("reading template %s: %w", tmplPath, err)
	}

	name := filepath.Base(tmplPath)

	return r.render(name, string(data), vars)
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
// Path segments use {{ }} delimiters but are rendered individually per segment
// to handle filesystem path separators properly.
func (r *Renderer) RenderPath(path string, vars map[string]any) (string, error) {
	if !strings.Contains(path, "{{") {
		return path, nil
	}

	result, err := r.RenderString(path, vars)
	if err != nil {
		return "", fmt.Errorf("rendering path %q: %w", path, err)
	}

	return result, nil
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
	tmpl, err := template.New(name).
		Funcs(r.funcMap).
		Option("missingkey=error").
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
