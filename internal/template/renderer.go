// Package template provides the HCL2 template rendering engine for forge
// blueprints. The single Renderer interface is implemented by an unexported
// hclRenderer that uses `hashicorp/hcl/v2` for `${expr}` interpolation and
// `%{ if … ~}` directives.
package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"
)

// Renderer is the abstraction the orchestrator (`internal/create`,
// `internal/sync`, `internal/check`) depends on. Tests can substitute a
// fake; production code uses NewRenderer.
//
// EvaluateBool takes an expression as a string and parses it on each call.
// EvaluateBoolExpr takes a pre-parsed hcl.Expression — preferred when the
// caller already holds a parsed expression (e.g. `Condition.When`
// populated by the HCL loader at parse time).
type Renderer interface {
	RenderFile(path string, vars map[string]cty.Value) ([]byte, error)
	RenderString(tmpl string, vars map[string]cty.Value) (string, error)
	RenderPath(path string, vars map[string]cty.Value) (string, error)
	EvaluateBool(expr string, vars map[string]cty.Value) (bool, error)
	EvaluateBoolExpr(expr hcl.Expression, vars map[string]cty.Value) (bool, error)
}

// hclRenderer is the production HCL2-backed Renderer.
type hclRenderer struct {
	funcs map[string]function.Function
}

// NewRenderer constructs the production HCL2 renderer wired with forge's
// custom function map (`funcs.go`).
func NewRenderer() Renderer {
	return &hclRenderer{funcs: hclFuncs()}
}

// RenderFile parses and renders a template file using the HCL2 engine.
func (r *hclRenderer) RenderFile(path string, vars map[string]cty.Value) ([]byte, error) {
	src, err := os.ReadFile(path) //nolint:gosec // template paths are from registry content, not untrusted user input
	if err != nil {
		return nil, fmt.Errorf("reading template %s: %w", path, err)
	}

	out, err := r.renderTemplate(filepath.Base(path), src, vars)
	if err != nil {
		return nil, err
	}

	return []byte(out), nil
}

// RenderString parses and renders an inline template string using the HCL2
// engine.
func (r *hclRenderer) RenderString(tmpl string, vars map[string]cty.Value) (string, error) {
	return r.renderTemplate("inline", []byte(tmpl), vars)
}

// RenderPath renders template expressions in file/directory path segments.
// HCL2 already accepts `${name}` directly, so paths without HCL2 markers are
// returned unchanged.
func (r *hclRenderer) RenderPath(path string, vars map[string]cty.Value) (string, error) {
	if !strings.Contains(path, "${") && !strings.Contains(path, "%{") {
		return path, nil
	}

	out, err := r.renderTemplate("path", []byte(path), vars)
	if err != nil {
		return "", fmt.Errorf("rendering path %q: %w", path, err)
	}

	return out, nil
}

// EvaluateBool parses an HCL2 expression and returns its bool evaluation.
// Used for `condition.when:` expressions where the source is a bare
// expression (e.g. `use_grpc == true`) rather than a template.
func (r *hclRenderer) EvaluateBool(expr string, vars map[string]cty.Value) (bool, error) {
	parsed, diags := hclsyntax.ParseExpression([]byte(expr), "condition", hcl.InitialPos)
	if diags.HasErrors() {
		return false, fmt.Errorf("parsing expression %q: %s", expr, diags.Error())
	}

	return r.EvaluateBoolExpr(parsed, vars)
}

// EvaluateBoolExpr evaluates a pre-parsed HCL expression against the given
// variables and converts the result to bool. Preferred over EvaluateBool
// when the caller already holds an hcl.Expression — avoids re-parsing on
// every call and surfaces evaluation diagnostics with the original source
// range.
func (r *hclRenderer) EvaluateBoolExpr(expr hcl.Expression, vars map[string]cty.Value) (bool, error) {
	result, diags := expr.Value(r.evalContext(vars))
	if diags.HasErrors() {
		return false, fmt.Errorf("evaluating expression: %s", diags.Error())
	}

	asBool, err := convert.Convert(result, cty.Bool)
	if err != nil {
		return false, fmt.Errorf("expression is not a bool: %w", err)
	}

	return asBool.True(), nil
}

func (r *hclRenderer) renderTemplate(name string, src []byte, vars map[string]cty.Value) (string, error) {
	parsed, diags := hclsyntax.ParseTemplate(src, name, hcl.InitialPos)
	if diags.HasErrors() {
		return "", fmt.Errorf("parsing template %q: %s", name, diags.Error())
	}

	result, diags := parsed.Value(r.evalContext(vars))
	if diags.HasErrors() {
		return "", fmt.Errorf("rendering template %q: %s", name, diags.Error())
	}

	asString, err := convert.Convert(result, cty.String)
	if err != nil {
		return "", fmt.Errorf("template %q produced non-string value: %w", name, err)
	}

	return asString.AsString(), nil
}

func (r *hclRenderer) evalContext(vars map[string]cty.Value) *hcl.EvalContext {
	return &hcl.EvalContext{
		Variables: vars,
		Functions: r.funcs,
	}
}

// StripTemplateExtension removes the .tmpl extension from a filename.
func StripTemplateExtension(path string) string {
	return strings.TrimSuffix(path, ".tmpl")
}

// IsTemplate returns true if the path ends with .tmpl.
func IsTemplate(path string) bool {
	return strings.HasSuffix(path, ".tmpl")
}
