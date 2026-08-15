// Package varsfile parses one or more `.forge-vars.hcl` files into a
// map[string]cty.Value keyed by variable name, scoped to a blueprint's
// declared variables.
//
// The package implements DESIGN-0005 — Variable input via vars file
// (IMPL-0008). Vars files are attribute-only HCL documents:
//
//	# example.forge-vars.hcl
//	project_name = "my-api"
//	use_grpc     = true
//	port         = 8080
//
// They compose left-to-right (later files override earlier ones on key
// collision), enforce strict literal values (no function calls, no
// references — see IMPL-0008 OQ-1), and require the `.hcl` file
// extension (IMPL-0008 OQ-8). Attribute values are coerced against the
// declared variable types from the target blueprint.
//
// The package exposes a single entry point, Load. Parsing, composition,
// and coercion stay internal so callers cannot reach into half-resolved
// state.
package varsfile

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/donaldgifford/forge/internal/config"
)

// requiredExt is the only file extension accepted for vars files
// (per IMPL-0008 OQ-8).
const requiredExt = ".hcl"

// Load parses every path in order and merges the resulting attribute
// maps left-to-right: later files override earlier files on key
// collision. Values are coerced against the declared variable types in
// vars. Unknown keys (present in a file but not declared in vars) are
// returned in the unknown slice for the caller to surface as a warning.
//
// Returns an error if any file has the wrong extension, fails to read,
// fails to parse, contains a block, contains a non-literal expression
// (function call, traversal, …), or fails type coercion. The error
// includes file:line:col when the underlying HCL diagnostic carries
// that information.
func Load(paths []string, vars []config.Variable) (resolved map[string]cty.Value, unknown []string, err error) {
	if len(paths) == 0 {
		return map[string]cty.Value{}, nil, nil
	}

	parser := hclparse.NewParser()

	merged := make(map[string]*hcl.Attribute)

	for _, path := range paths {
		attrs, parseErr := parseFile(parser, path)
		if parseErr != nil {
			return nil, nil, parseErr
		}

		maps.Copy(merged, attrs)
	}

	resolved, unknown, err = coerce(merged, vars)
	if err != nil {
		return nil, nil, err
	}

	return resolved, unknown, nil
}

// parseFile reads path, parses it as HCL, and returns the
// top-level attribute set. It rejects:
//
//   - paths without a .hcl extension (IMPL-0008 OQ-8);
//   - parse errors from hclparse;
//   - top-level blocks (vars files are attribute-only).
//
// The returned map is keyed by attribute name, value is the parsed
// hcl.Attribute (so callers see the source range when reporting
// downstream errors).
func parseFile(parser *hclparse.Parser, path string) (map[string]*hcl.Attribute, error) {
	if ext := filepath.Ext(path); ext != requiredExt {
		return nil, fmt.Errorf(
			"vars file %q: only %s extensions are supported; got %q",
			path, requiredExt, ext,
		)
	}

	src, err := os.ReadFile(path) //nolint:gosec // path comes from user-supplied --var-file inputs, matching the existing config loader's posture
	if err != nil {
		return nil, fmt.Errorf("reading vars file %s: %w", path, err)
	}

	file, diags := parser.ParseHCL(src, path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing vars file %s: %s", path, diags.Error())
	}

	// Reject any top-level blocks before extracting attributes —
	// JustAttributes only complains via a diagnostic, not a hard
	// error, so an explicit block scan gives a cleaner message.
	if blockErr := rejectBlocks(file.Body, path); blockErr != nil {
		return nil, blockErr
	}

	attrs, attrDiags := file.Body.JustAttributes()
	if attrDiags.HasErrors() {
		return nil, fmt.Errorf("vars file %s: %s", path, attrDiags.Error())
	}

	return attrs, nil
}

// rejectBlocks walks body looking for top-level blocks and surfaces
// a clear error if any are present. Vars files are attribute-only by
// design (DESIGN-0005 File Format). Concrete `*hclsyntax.Body` is
// what `hclparse` produces; reaching into it directly gives a
// cleaner error than letting JustAttributes' generic "Blocks are not
// allowed here" diagnostic leak out.
func rejectBlocks(body hcl.Body, path string) error {
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return nil
	}

	for _, block := range syntaxBody.Blocks {
		return fmt.Errorf(
			"vars file %s: contains block %q at %s:%d,%d — vars files accept attribute assignments only, not blocks",
			path,
			block.Type,
			path,
			block.DefRange().Start.Line,
			block.DefRange().Start.Column,
		)
	}

	return nil
}

// coerce evaluates each attribute against the declared variable type
// in vars and returns the resolved cty.Value map plus a list of
// unknown keys (attributes in the file that don't match any declared
// variable). Evaluation uses an empty EvalContext: no functions, no
// variables, no traversals are accessible — only literal values pass
// (IMPL-0008 OQ-1).
func coerce(attrs map[string]*hcl.Attribute, vars []config.Variable) (map[string]cty.Value, []string, error) {
	declared := make(map[string]config.Variable, len(vars))
	for i := range vars {
		declared[vars[i].Name] = vars[i]
	}

	resolved := make(map[string]cty.Value, len(attrs))

	var unknown []string

	for name, attr := range attrs {
		v, ok := declared[name]
		if !ok {
			unknown = append(unknown, name)

			continue
		}

		val, err := evalLiteral(attr)
		if err != nil {
			return nil, nil, err
		}

		coerced, err := coerceToDeclared(val, v.Type, attr)
		if err != nil {
			return nil, nil, err
		}

		resolved[name] = coerced
	}

	return resolved, unknown, nil
}

// evalLiteral evaluates attr.Expr in an empty EvalContext, rejecting
// any expression that depends on variables, functions, or traversals.
// Returns a literal cty.Value or a file:line:col-located error.
func evalLiteral(attr *hcl.Attribute) (cty.Value, error) {
	if traversals := attr.Expr.Variables(); len(traversals) > 0 {
		t := traversals[0]
		rng := t.SourceRange()

		return cty.NilVal, fmt.Errorf(
			"vars file %s:%d,%d: variable %q only accepts literal values; references and traversals are not supported",
			rng.Filename, rng.Start.Line, rng.Start.Column, attr.Name,
		)
	}

	val, diags := attr.Expr.Value(&hcl.EvalContext{})
	if diags.HasErrors() {
		// Empty EvalContext intentionally rejects function calls;
		// surface the first diagnostic with its source location for
		// the caller.
		rng := attr.Expr.Range()

		return cty.NilVal, fmt.Errorf(
			"vars file %s:%d,%d: variable %q only accepts literal values: %s",
			rng.Filename, rng.Start.Line, rng.Start.Column, attr.Name, diags.Error(),
		)
	}

	return val, nil
}

// coerceToDeclared converts the literal val to the declared cty.Type
// from the blueprint. cty.Convert handles string→number and string→bool
// coercions a vars-file author would write (e.g. `port = "42"` against
// a declared `number`) as well as the structured-type targets
// (object/list/map) shipped in IMPL-0009. Errors carry the vars-file
// source location of the attribute that supplied val.
func coerceToDeclared(val cty.Value, target cty.Type, attr *hcl.Attribute) (cty.Value, error) {
	converted, err := convert.Convert(val, target)
	if err == nil {
		return converted, nil
	}

	rng := attr.Expr.Range()

	return cty.NilVal, fmt.Errorf(
		"vars file %s:%d,%d: variable %q expects %s, got %s: %w",
		rng.Filename, rng.Start.Line, rng.Start.Column,
		attr.Name, target.FriendlyName(), val.Type().FriendlyName(), err,
	)
}
