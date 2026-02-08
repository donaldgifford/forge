// Package template provides the Go text/template rendering engine with custom functions.
package template

import (
	"os"
	"strings"
	"text/template"
	"time"
	"unicode"
)

// FuncMap returns the custom template function map used by forge templates.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"snakeCase":  snakeCase,
		"camelCase":  camelCase,
		"pascalCase": pascalCase,
		"kebabCase":  kebabCase,
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"title":      strings.ToTitle,
		"replace":    replace,
		"trimPrefix": trimPrefix,
		"trimSuffix": trimSuffix,
		"now":        now,
		"env":        os.Getenv,
		"default":    defaultVal,
	}
}

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

// replace replaces all occurrences of old with repl in s.
// Argument order is (old, repl, s) to support piping: {{ "foo-bar" | replace "-" "_" }}.
func replace(old, repl, s string) string {
	return strings.ReplaceAll(s, old, repl)
}

// trimPrefix removes the given prefix from s.
// Argument order is (prefix, s) to support piping: {{ "v1.2.3" | trimPrefix "v" }}.
func trimPrefix(prefix, s string) string {
	return strings.TrimPrefix(s, prefix)
}

// trimSuffix removes the given suffix from s.
// Argument order is (suffix, s) to support piping: {{ "file.tmpl" | trimSuffix ".tmpl" }}.
func trimSuffix(suffix, s string) string {
	return strings.TrimSuffix(s, suffix)
}

// now returns the current time formatted with the given Go layout.
func now(layout string) string {
	return time.Now().Format(layout)
}

// defaultVal returns val if it's non-empty, otherwise returns def.
func defaultVal(def, val string) string {
	if val != "" {
		return val
	}

	return def
}

// splitWords splits a string into words by separators and casing transitions.
func splitWords(s string) []string {
	var words []string
	var current []rune

	for i, r := range s {
		switch {
		case r == '_' || r == '-' || r == ' ' || r == '.':
			if len(current) > 0 {
				words = append(words, string(current))
				current = nil
			}
		case unicode.IsUpper(r) && i > 0 && len(current) > 0:
			words = append(words, string(current))
			current = []rune{r}
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
