package template

import (
	"errors"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// errHCLNotImplemented marks HCLRenderer methods that will be filled in by
// IMPL-0004 task A.4 (renderer impl + unit tests). The skeleton exists in
// A.2 so the interface, call sites, and CLI flag plumbing can be wired
// before the engine is real.
var errHCLNotImplemented = errors.New("HCL2 renderer not implemented yet (IMPL-0004 A.4)")

// HCLRenderer renders forge templates using HashiCorp HCL2
// (`hashicorp/hcl/v2`) syntax: `${expr}` interpolation and `%{ if … ~}`
// directives. After Phase C of IMPL-0004, this is the sole renderer in the
// codebase.
type HCLRenderer struct {
	funcs map[string]function.Function
}

// NewHCLRenderer constructs an HCLRenderer wired with the forge custom
// function map (`internal/template/hcl_funcs.go`, added in A.3).
func NewHCLRenderer() *HCLRenderer {
	return &HCLRenderer{funcs: HCLFuncs()}
}

// RenderFile parses and renders a template file using the HCL2 engine.
//
//nolint:revive // receiver referenced once A.4 lands the implementation
func (r *HCLRenderer) RenderFile(path string, vars map[string]cty.Value) ([]byte, error) {
	_ = path
	_ = vars

	return nil, errHCLNotImplemented
}

// RenderString parses and renders an inline template string using the HCL2
// engine.
//
//nolint:revive // receiver referenced once A.4 lands the implementation
func (r *HCLRenderer) RenderString(tmpl string, vars map[string]cty.Value) (string, error) {
	_ = tmpl
	_ = vars

	return "", errHCLNotImplemented
}

// RenderPath renders template expressions in file/directory path segments.
//
//nolint:revive // receiver referenced once A.4 lands the implementation
func (r *HCLRenderer) RenderPath(path string, vars map[string]cty.Value) (string, error) {
	_ = path
	_ = vars

	return "", errHCLNotImplemented
}

// EvaluateBool parses an HCL2 expression and returns its bool evaluation.
// Used for `condition.when:` evaluation.
//
//nolint:revive // receiver referenced once A.4 lands the implementation
func (r *HCLRenderer) EvaluateBool(expr string, vars map[string]cty.Value) (bool, error) {
	_ = expr
	_ = vars

	return false, errHCLNotImplemented
}
