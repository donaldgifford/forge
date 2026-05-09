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

// HCLRenderer renders forge templates using HashiCorp HCL2
// (`hashicorp/hcl/v2`) syntax: `${expr}` interpolation and `%{ if … ~}`
// directives. After Phase C of IMPL-0004, this is the sole renderer in the
// codebase.
type HCLRenderer struct {
	funcs map[string]function.Function
}

// NewHCLRenderer constructs an HCLRenderer wired with the forge custom
// function map (`internal/template/hcl_funcs.go`).
func NewHCLRenderer() *HCLRenderer {
	return &HCLRenderer{funcs: HCLFuncs()}
}

// RenderFile parses and renders a template file using the HCL2 engine.
func (r *HCLRenderer) RenderFile(path string, vars map[string]cty.Value) ([]byte, error) {
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
func (r *HCLRenderer) RenderString(tmpl string, vars map[string]cty.Value) (string, error) {
	return r.renderTemplate("inline", []byte(tmpl), vars)
}

// RenderPath renders template expressions in file/directory path segments.
// HCL2 already accepts `${name}` directly, so no normalization is needed —
// the v1 `{{name}}` shorthand fallback is the migration tool's
// responsibility (Phase B).
func (r *HCLRenderer) RenderPath(path string, vars map[string]cty.Value) (string, error) {
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
func (r *HCLRenderer) EvaluateBool(expr string, vars map[string]cty.Value) (bool, error) {
	parsed, diags := hclsyntax.ParseExpression([]byte(expr), "condition", hcl.InitialPos)
	if diags.HasErrors() {
		return false, fmt.Errorf("parsing expression %q: %s", expr, diags.Error())
	}

	result, diags := parsed.Value(r.evalContext(vars))
	if diags.HasErrors() {
		return false, fmt.Errorf("evaluating expression %q: %s", expr, diags.Error())
	}

	asBool, err := convert.Convert(result, cty.Bool)
	if err != nil {
		return false, fmt.Errorf("expression %q is not a bool: %w", expr, err)
	}

	return asBool.True(), nil
}

func (r *HCLRenderer) renderTemplate(name string, src []byte, vars map[string]cty.Value) (string, error) {
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

func (r *HCLRenderer) evalContext(vars map[string]cty.Value) *hcl.EvalContext {
	return &hcl.EvalContext{
		Variables: vars,
		Functions: r.funcs,
	}
}
