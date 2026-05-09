package template

import (
	"os"
	"time"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

// HCLFuncs returns the forge custom function map for the HCL2 renderer.
// Custom forge functions (snakeCase/camelCase/pascalCase/kebabCase/now/env)
// are defined locally; the rest (upper/lower/title/replace/trimPrefix/
// trimSuffix/coalesce) come from `cty/function/stdlib` to avoid
// reinventing the wheel.
//
// `coalesce` replaces the v1 `default(val, fallback)` custom function per
// ADR-0001.
func HCLFuncs() map[string]function.Function {
	return map[string]function.Function{
		"snakeCase":  snakeCaseFunc,
		"camelCase":  camelCaseFunc,
		"pascalCase": pascalCaseFunc,
		"kebabCase":  kebabCaseFunc,
		"now":        nowFunc,
		"env":        envFunc,

		"upper":      stdlib.UpperFunc,
		"lower":      stdlib.LowerFunc,
		"title":      stdlib.TitleFunc,
		"replace":    stdlib.ReplaceFunc,
		"trimPrefix": stdlib.TrimPrefixFunc,
		"trimSuffix": stdlib.TrimSuffixFunc,
		"coalesce":   stdlib.CoalesceFunc,
	}
}

var snakeCaseFunc = function.New(&function.Spec{
	Description: "Converts a string to snake_case.",
	Params: []function.Parameter{
		{Name: "str", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		return cty.StringVal(snakeCase(args[0].AsString())), nil
	},
})

var camelCaseFunc = function.New(&function.Spec{
	Description: "Converts a string to camelCase.",
	Params: []function.Parameter{
		{Name: "str", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		return cty.StringVal(camelCase(args[0].AsString())), nil
	},
})

var pascalCaseFunc = function.New(&function.Spec{
	Description: "Converts a string to PascalCase.",
	Params: []function.Parameter{
		{Name: "str", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		return cty.StringVal(pascalCase(args[0].AsString())), nil
	},
})

var kebabCaseFunc = function.New(&function.Spec{
	Description: "Converts a string to kebab-case.",
	Params: []function.Parameter{
		{Name: "str", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		return cty.StringVal(kebabCase(args[0].AsString())), nil
	},
})

var nowFunc = function.New(&function.Spec{
	Description: "Returns the current time formatted with the given Go layout.",
	Params: []function.Parameter{
		{Name: "layout", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		return cty.StringVal(time.Now().Format(args[0].AsString())), nil
	},
})

var envFunc = function.New(&function.Spec{
	Description: "Returns the value of the named environment variable, or empty string if unset.",
	Params: []function.Parameter{
		{Name: "key", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		return cty.StringVal(os.Getenv(args[0].AsString())), nil
	},
})
