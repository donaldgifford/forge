package template

import "github.com/zclconf/go-cty/cty/function"

// HCLFuncs returns the forge custom function map for the HCL2 renderer.
// IMPL-0004 task A.3 fills this in with `function.Function` definitions
// equivalent to the v1 FuncMap (snakeCase, camelCase, pascalCase, kebabCase,
// title, trimPrefix, trimSuffix, now, env) plus cty/stdlib funcs (upper,
// lower, replace, coalesce).
func HCLFuncs() map[string]function.Function {
	return map[string]function.Function{}
}
