package template

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/zclconf/go-cty/cty"
)

// pathVarPattern matches {{varname}} without a leading dot, pipe, or space-prefixed content.
// It normalizes shorthand path variables to Go template syntax: {{varname}} → {{.varname}}.
var pathVarPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

// Renderer is the durable abstraction over the forge template engine. The
// orchestrator (`internal/create`, `internal/sync`, `internal/check`) depends
// only on this interface. During the v1→v2 transition (IMPL-0004) two
// implementations exist: TextRenderer (text/template) and HCLRenderer (HCL2).
// Phase C deletes TextRenderer; the interface stays.
type Renderer interface {
	RenderFile(path string, vars map[string]cty.Value) ([]byte, error)
	RenderString(tmpl string, vars map[string]cty.Value) (string, error)
	RenderPath(path string, vars map[string]cty.Value) (string, error)
	EvaluateBool(expr string, vars map[string]cty.Value) (bool, error)
}

// TextRenderer renders v1 Go text/template content with the forge custom
// function map. It satisfies the Renderer interface during the HCL2
// transition by converting cty.Values back to plain Go values internally.
// Phase C of IMPL-0004 deletes this type.
type TextRenderer struct {
	funcMap template.FuncMap
}

// NewTextRenderer constructs a TextRenderer with the standard forge function
// map.
func NewTextRenderer() *TextRenderer {
	return &TextRenderer{funcMap: FuncMap()}
}

// NewRenderer is retained as a v1-compatibility alias for NewTextRenderer.
// Phase C of IMPL-0004 removes it.
func NewRenderer() *TextRenderer {
	return NewTextRenderer()
}

// RenderFile reads a template file and renders it with the given variables.
// File templates use missingkey=zero to allow the default function to work
// with optional variables.
func (r *TextRenderer) RenderFile(tmplPath string, vars map[string]cty.Value) ([]byte, error) {
	data, err := os.ReadFile(tmplPath) //nolint:gosec // template paths are from registry content, not untrusted user input
	if err != nil {
		return nil, fmt.Errorf("reading template %s: %w", tmplPath, err)
	}

	name := filepath.Base(tmplPath)

	return r.renderWithOption(name, string(data), FromCtyValues(vars), "missingkey=zero")
}

// RenderString renders an inline template string with the given variables.
func (r *TextRenderer) RenderString(tmpl string, vars map[string]cty.Value) (string, error) {
	result, err := r.render("inline", tmpl, FromCtyValues(vars))
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
func (r *TextRenderer) RenderPath(path string, vars map[string]cty.Value) (string, error) {
	if !strings.Contains(path, "{{") {
		return path, nil
	}

	normalized := normalizePathTemplate(path)

	result, err := r.RenderString(normalized, vars)
	if err != nil {
		return "", fmt.Errorf("rendering path %q: %w", path, err)
	}

	return result, nil
}

// EvaluateBool renders an expression as a string and parses it as "true"/false.
// TextRenderer treats this as a thin wrapper over RenderString to keep parity
// with v1 condition.when evaluation; HCLRenderer evaluates expressions as
// cty.Bool natively.
func (r *TextRenderer) EvaluateBool(expr string, vars map[string]cty.Value) (bool, error) {
	rendered, err := r.RenderString(expr, vars)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(rendered) == "true", nil
}

// normalizePathTemplate converts shorthand {{varname}} to {{.varname}} in path templates.
// This allows directory names like "{{project_name}}" to work without requiring the
// Go template dot prefix. Expressions that already use dot notation (e.g., {{.varname}})
// or contain function calls/pipes are left unchanged.
func normalizePathTemplate(path string) string {
	return pathVarPattern.ReplaceAllStringFunc(path, func(match string) string {
		inner := strings.TrimSpace(match[2 : len(match)-2])

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

func (r *TextRenderer) render(name, text string, vars map[string]any) ([]byte, error) {
	return r.renderWithOption(name, text, vars, "missingkey=error")
}

func (r *TextRenderer) renderWithOption(name, text string, vars map[string]any, option string) ([]byte, error) {
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
