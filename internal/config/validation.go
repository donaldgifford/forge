package config

// validation.go is the home for the validation-block evaluator
// (DESIGN-0006 / IMPL-0009 Phase C, OQ-2). Validation conditions stay
// as unevaluated hcl.Expression values from load time and run here
// after every variable has been resolved through the
// vars-file → override → default → prompt chain.
//
// The eval context is built by BuildEvalContext, which wraps the
// bound variable map under the `var.*` namespace (RFC-0003 shape)
// while also exposing the same names at the top level so the legacy
// bare-reference style used by default expressions during the v0.7
// transition window continues to evaluate cleanly.

import (
	"errors"
	"fmt"
	"maps"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

// BuildEvalContext returns an hcl.EvalContext that resolves both bare
// references (`project_name`) and the `var.X` namespace (RFC-0003 /
// DESIGN-0006).
//
// Default expressions written during the v0.7 transition window keep
// using bare references — `"github.com/example/${project_name}"`. New
// validation conditions and any post-RFC-0003 expressions use the
// `var.X` form. One eval context resolves both shapes.
func BuildEvalContext(bound map[string]cty.Value) *hcl.EvalContext {
	ctx := &hcl.EvalContext{
		Variables: make(map[string]cty.Value, len(bound)+1),
		Functions: builtinFunctions(),
	}

	maps.Copy(ctx.Variables, bound)

	if len(bound) > 0 {
		ctx.Variables["var"] = cty.ObjectVal(bound)
	}

	return ctx
}

// EvaluateValidations runs every variable's `validation { ... }` block
// against the fully resolved variable scope. Failures accumulate into
// the returned slice rather than short-circuiting on the first one —
// authors see every constraint violation in a single pass, the same
// way Terraform surfaces stacked plan diagnostics.
//
// Returns nil when no variables are declared, when no validation
// blocks exist, or when every block's condition evaluates to true.
func EvaluateValidations(vars []Variable, bound map[string]cty.Value) []error {
	if len(vars) == 0 {
		return nil
	}

	ctx := BuildEvalContext(bound)

	var errs []error

	for i := range vars {
		v := &vars[i]
		for j := range v.Validations {
			if err := evaluateOneValidation(v, &v.Validations[j], ctx); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errs
}

// evaluateOneValidation runs a single validation block's condition and
// returns a forge-formatted error when the result is non-true, the
// condition can't evaluate, or the result isn't a bool. The error
// format is `<error_message> (variable "X", blueprint.hcl:L:C)` per
// IMPL-0009 C.3.
func evaluateOneValidation(v *Variable, val *Validation, ctx *hcl.EvalContext) error {
	result, diags := val.Condition.Value(ctx)
	if diags.HasErrors() {
		return fmt.Errorf(
			"validating variable %q: %s (at %s)",
			v.Name, diags.Error(), val.DefRange.String(),
		)
	}

	if result.IsNull() || !result.IsKnown() {
		return fmt.Errorf(
			"validating variable %q: condition produced null/unknown value (at %s)",
			v.Name, val.DefRange.String(),
		)
	}

	asBool, err := convert.Convert(result, cty.Bool)
	if err != nil {
		return fmt.Errorf(
			"validating variable %q: condition must yield a bool, got %s (at %s)",
			v.Name, result.Type().FriendlyName(), val.DefRange.String(),
		)
	}

	if asBool.True() {
		return nil
	}

	return fmt.Errorf(
		`%s (variable %q, %s)`,
		val.ErrorMessage, v.Name, val.DefRange.String(),
	)
}

// JoinErrors composes a multi-error from a slice of validation errors,
// producing a single error per validation on its own line. Used by
// create/sync callers that want one Go error value back from
// EvaluateValidations.
func JoinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	return errors.Join(errs...)
}

// builtinFunctions returns the function map exposed to default and
// validation expressions. Keep this set narrowly Terraform-aligned —
// validation conditions are a constrained surface, not a general
// scripting environment:
//
//   - `can(expr)` / `try(expr, ...)` for graceful expression failure
//     (tryfunc), used in the canonical `validation { condition =
//     can(regex(...)) ... }` migration pattern.
//   - `regex(pattern, str)` for the regex-condition use case
//     superseding the legacy `validate = "regex"` field.
//   - `contains(list, val)` / `length(coll)` for the choice-enum
//     migration pattern (was `type = "choice"` + `choices = [...]`).
//
// Template-side helpers (snakeCase, env, now, etc.) intentionally
// stay in internal/template — they're a rendering concern, not a
// validation one.
func builtinFunctions() map[string]function.Function {
	return map[string]function.Function{
		"can":      tryfunc.CanFunc,
		"try":      tryfunc.TryFunc,
		"regex":    stdlib.RegexFunc,
		"contains": stdlib.ContainsFunc,
		"length":   stdlib.LengthFunc,
		"lower":    stdlib.LowerFunc,
		"upper":    stdlib.UpperFunc,
		"coalesce": stdlib.CoalesceFunc,
	}
}
