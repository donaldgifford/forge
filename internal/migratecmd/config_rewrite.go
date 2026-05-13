package migratecmd

// YAML→HCL config rewriter for `forge migrate config`.
//
// The rewriter emits HCL text directly via a strings.Builder, then runs
// hclwrite.Format on the result for canonical formatting. We don't use
// hclwrite.SetAttributeValue because it goes through cty.StringVal +
// TokensForValue, which escapes `$` to `$$` to suppress template
// interpolation — exactly the wrong thing for fields that *are*
// templates (variable.default, rename entries). Going string-first
// keeps full control of the emitted source.
//
// Per OQ-3, comments in the input YAML are not preserved.

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"gopkg.in/yaml.v3"

	"github.com/donaldgifford/forge/internal/config"
)

// RewriteBlueprintYAMLToHCL parses a blueprint.yaml byte slice and
// returns the equivalent blueprint.hcl bytes. The apiVersion field is
// dropped on emit (per IMPL-0005 OQ-2: file extension is the version
// signal). Templated fields (variable.default, condition.when,
// rename entries) round-trip as HCL templates / expressions.
func RewriteBlueprintYAMLToHCL(src []byte) ([]byte, error) {
	var ybp yamlBlueprint
	if err := yaml.Unmarshal(src, &ybp); err != nil {
		return nil, fmt.Errorf("parsing YAML blueprint: %w", err)
	}

	bp := ybp.toBlueprint()

	var b strings.Builder

	writeStringAttr(&b, "name", bp.Name)
	writeStringAttr(&b, "description", bp.Description)
	writeStringAttr(&b, "version", bp.Version)
	writeStringListAttr(&b, "tags", bp.Tags)

	writeDefaultsBlock(&b, bp.Defaults)

	for i := range bp.Variables {
		writeVariableBlock(&b, &bp.Variables[i])
	}

	for _, c := range bp.Conditions {
		writeConditionBlock(&b, c)
	}

	writeHooksBlock(&b, bp.Hooks)
	writeSyncBlock(&b, bp.Sync)
	writeRenameBlock(&b, bp.Rename)

	return formatHCL(b.String())
}

// RewriteRegistryYAMLToHCL parses a registry.yaml byte slice and
// returns the equivalent registry.hcl bytes. apiVersion is dropped.
// Registry content is fully eager (no templates), so this rewriter is
// simpler than the blueprint one.
func RewriteRegistryYAMLToHCL(src []byte) ([]byte, error) {
	var yreg yamlRegistry
	if err := yaml.Unmarshal(src, &yreg); err != nil {
		return nil, fmt.Errorf("parsing YAML registry: %w", err)
	}

	reg := yreg.toRegistry()

	var b strings.Builder

	writeStringAttr(&b, "name", reg.Name)
	writeStringAttr(&b, "description", reg.Description)

	for _, m := range reg.Maintainers {
		b.WriteString("\nmaintainer {\n")
		writeStringAttr(&b, "name", m.Name)
		writeStringAttr(&b, "email", m.Email)
		b.WriteString("}\n")
	}

	if reg.Defaults.SyncStrategy != "" || reg.Defaults.Managed {
		b.WriteString("\ndefaults {\n")
		writeStringAttr(&b, "sync_strategy", reg.Defaults.SyncStrategy)

		if reg.Defaults.Managed {
			b.WriteString("managed = true\n")
		}

		b.WriteString("}\n")
	}

	for _, e := range reg.Blueprints {
		fmt.Fprintf(&b, "\nblueprint %s {\n", quoteHCLString(e.Name))
		writeStringAttr(&b, "path", e.Path)
		writeStringAttr(&b, "description", e.Description)
		writeStringAttr(&b, "version", e.Version)
		writeStringListAttr(&b, "tags", e.Tags)
		writeStringAttr(&b, "latest_commit", e.LatestCommit)
		b.WriteString("}\n")
	}

	return formatHCL(b.String())
}

// writeStringAttr emits `name = "value"` if value is non-empty.
func writeStringAttr(b *strings.Builder, name, value string) {
	if value == "" {
		return
	}

	fmt.Fprintf(b, "%s = %s\n", name, quoteHCLString(value))
}

// writeStringListAttr emits `name = ["v1", "v2"]` if the slice is
// non-empty.
func writeStringListAttr(b *strings.Builder, name string, values []string) {
	if len(values) == 0 {
		return
	}

	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = quoteHCLString(v)
	}

	fmt.Fprintf(b, "%s = [%s]\n", name, strings.Join(parts, ", "))
}

// writeDefaultsBlock emits the `defaults { ... }` block if any field is
// set. Distinct from the registry-level defaults — this one carries
// exclude lists and override_strategy maps.
func writeDefaultsBlock(b *strings.Builder, d config.Defaults) {
	if len(d.Exclude) == 0 && len(d.OverrideStrategy) == 0 {
		return
	}

	b.WriteString("\ndefaults {\n")
	writeStringListAttr(b, "exclude", d.Exclude)

	if len(d.OverrideStrategy) > 0 {
		b.WriteString("override_strategy = {\n")

		for k, v := range d.OverrideStrategy {
			fmt.Fprintf(b, "%s = %s\n", quoteHCLString(k), quoteHCLString(v))
		}

		b.WriteString("}\n")
	}

	b.WriteString("}\n")
}

// writeVariableBlock emits `variable "name" { ... }` with eager attrs
// quoted normally and templated attrs (default, validate) emitted as
// HCL string templates with $-syntax preserved.
func writeVariableBlock(b *strings.Builder, v *config.Variable) {
	fmt.Fprintf(b, "\nvariable %s {\n", quoteHCLString(v.Name))
	writeStringAttr(b, "description", v.Description)
	writeStringAttr(b, "type", v.Type)

	if v.Required {
		b.WriteString("required = true\n")
	}

	writeStringListAttr(b, "choices", v.Choices)

	if v.Default != "" {
		// Default may contain template syntax (${other_var}) — emit
		// inside double quotes without escaping the $-sequences.
		fmt.Fprintf(b, "default = %s\n", quoteHCLTemplate(v.Default))
	}

	if v.Validate != "" {
		fmt.Fprintf(b, "validate = %s\n", quoteHCLTemplate(v.Validate))
	}

	b.WriteString("}\n")
}

// writeConditionBlock emits `condition { when = ..., exclude = [...] }`.
// The when source is already a valid HCL bool expression (it has been
// since v0.3.0), so it's written verbatim — no quoting.
func writeConditionBlock(b *strings.Builder, c config.Condition) {
	source := c.WhenSource
	if source == "" {
		// Fall back to the parsed expression's text if WhenSource is
		// empty (e.g. constructed in-memory).
		source = "false"
	}

	b.WriteString("\ncondition {\n")
	fmt.Fprintf(b, "when = %s\n", source)
	writeStringListAttr(b, "exclude", c.Exclude)
	b.WriteString("}\n")
}

// writeHooksBlock emits the `hooks { post_create = [...] }` block if
// any hooks are defined.
func writeHooksBlock(b *strings.Builder, h config.Hooks) {
	if len(h.PostCreate) == 0 {
		return
	}

	b.WriteString("\nhooks {\n")
	writeStringListAttr(b, "post_create", h.PostCreate)
	b.WriteString("}\n")
}

// writeSyncBlock emits `sync { ignore = [...]; managed_file "p" { strategy = "..." } }`.
func writeSyncBlock(b *strings.Builder, s config.SyncConfig) {
	if len(s.Ignore) == 0 && len(s.ManagedFiles) == 0 {
		return
	}

	b.WriteString("\nsync {\n")
	writeStringListAttr(b, "ignore", s.Ignore)

	for _, mf := range s.ManagedFiles {
		fmt.Fprintf(b, "managed_file %s {\n", quoteHCLString(mf.Path))
		writeStringAttr(b, "strategy", mf.Strategy)
		b.WriteString("}\n")
	}

	b.WriteString("}\n")
}

// writeRenameBlock emits the `rename { entry { from = "..." to = "..." } }`
// shape. From and to may both contain template syntax (${var}); they
// round-trip via quoteHCLTemplate.
func writeRenameBlock(b *strings.Builder, rename map[string]string) {
	if len(rename) == 0 {
		return
	}

	b.WriteString("\nrename {\n")

	for from, to := range rename {
		b.WriteString("entry {\n")
		fmt.Fprintf(b, "from = %s\n", quoteHCLTemplate(from))
		fmt.Fprintf(b, "to = %s\n", quoteHCLTemplate(to))
		b.WriteString("}\n")
	}

	b.WriteString("}\n")
}

// quoteHCLString renders a Go string as an HCL string literal where
// `$`, `%`, `"`, `\`, and newlines are escaped. Use this for non-
// templated values (names, paths, descriptions).
func quoteHCLString(s string) string {
	var b strings.Builder

	b.WriteByte('"')

	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '$':
			// Escape to suppress HCL template interpolation.
			b.WriteString(`$$`)
		case '%':
			b.WriteString(`%%`)
		default:
			b.WriteRune(r)
		}
	}

	b.WriteByte('"')

	return b.String()
}

// quoteHCLTemplate renders a Go string as an HCL string literal but
// preserves `${...}` template-interpolation sequences and `%{...}`
// directive sequences exactly. Use this for fields that carry HCL
// templates (variable.default, rename keys/values).
func quoteHCLTemplate(s string) string {
	var b strings.Builder

	b.WriteByte('"')

	for i := 0; i < len(s); i++ {
		c := s[i]

		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(c)
		}
	}

	b.WriteByte('"')

	return b.String()
}

// formatHCL runs the assembled string through hclwrite.Format for
// canonical indentation, alignment, and whitespace.
func formatHCL(src string) ([]byte, error) {
	formatted := hclwrite.Format([]byte(src))

	// hclwrite.Format does not surface parse errors — it returns the
	// input unchanged on syntax errors. To catch malformed output, also
	// run a parse pass.
	_, diags := hclwrite.ParseConfig(formatted, "rewrite-output.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("rewriter produced invalid HCL: %s", diags.Error())
	}

	return formatted, nil
}
