package template

import (
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

// hclFuncs returns the forge custom function map for the HCL2 renderer.
// Custom forge functions (snakeCase/camelCase/pascalCase/kebabCase/now/env)
// are defined locally; the rest (upper/lower/title/replace/trimPrefix/
// trimSuffix/coalesce) come from `cty/function/stdlib` to avoid
// reinventing the wheel.
//
// `coalesce` replaces the v1 `default(val, fallback)` custom function per
// ADR-0001.
func hclFuncs() map[string]function.Function {
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

// snakeCase converts a string to snake_case.
func snakeCase(s string) string {
	return strings.ToLower(toDelimited(s, '_'))
}

// camelCase converts a string to camelCase.
func camelCase(s string) string {
	words := splitWords(s)
	if len(words) == 0 {
		return ""
	}

	result := strings.ToLower(words[0])
	for _, w := range words[1:] {
		result += capitalize(w)
	}

	return result
}

// pascalCase converts a string to PascalCase.
func pascalCase(s string) string {
	words := splitWords(s)
	var result string

	for _, w := range words {
		result += capitalize(w)
	}

	return result
}

// kebabCase converts a string to kebab-case.
func kebabCase(s string) string {
	return strings.ToLower(toDelimited(s, '-'))
}

// splitWords splits a string into words by separators and casing transitions.
func splitWords(s string) []string {
	var words []string
	current := make([]rune, 0, len(s))

	for i, r := range s {
		switch {
		case r == '_' || r == '-' || r == ' ' || r == '.':
			if len(current) > 0 {
				words = append(words, string(current))
				current = current[:0]
			}
		case unicode.IsUpper(r) && i > 0 && len(current) > 0:
			words = append(words, string(current))
			current = current[:0]
			current = append(current, r)
		default:
			current = append(current, r)
		}
	}

	if len(current) > 0 {
		words = append(words, string(current))
	}

	return words
}

// toDelimited converts a string to a delimited format.
func toDelimited(s string, delim rune) string {
	words := splitWords(s)
	parts := make([]string, len(words))

	for i, w := range words {
		parts[i] = strings.ToLower(w)
	}

	return strings.Join(parts, string(delim))
}

// capitalize uppercases the first letter of a string.
func capitalize(s string) string {
	if s == "" {
		return ""
	}

	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])

	return string(runes)
}
