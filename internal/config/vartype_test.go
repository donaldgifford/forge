package config_test

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/config"
)

// parseExpr is the test-only helper that takes the source text of a
// single HCL expression (e.g. `list(string)` or `object({port = number})`)
// and returns the parsed hcl.Expression. Tests pass the result directly
// to ParseVariableType. Keeps the per-case noise out of the table.
func parseExpr(t *testing.T, src string) hcl.Expression {
	t.Helper()

	expr, diags := hclsyntax.ParseExpression([]byte(src), "test.hcl", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "parsing %q: %s", src, diags.Error())

	return expr
}

// TestParseVariableType_AcceptedForms covers every type expression
// shape forge supports in v0.7: bareword scalars, list, map, and
// object (including a nested object).
func TestParseVariableType_AcceptedForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		src    string
		expect cty.Type
	}{
		{name: "string scalar", src: "string", expect: cty.String},
		{name: "bool scalar", src: "bool", expect: cty.Bool},
		{name: "number scalar", src: "number", expect: cty.Number},
		{name: "list of string", src: "list(string)", expect: cty.List(cty.String)},
		{name: "list of number", src: "list(number)", expect: cty.List(cty.Number)},
		{name: "map of string", src: "map(string)", expect: cty.Map(cty.String)},
		{
			name:   "object with two fields",
			src:    `object({port = number, host = string})`,
			expect: cty.Object(map[string]cty.Type{"port": cty.Number, "host": cty.String}),
		},
		{
			name: "nested object",
			src:  `object({addr = object({host = string, port = number})})`,
			expect: cty.Object(map[string]cty.Type{
				"addr": cty.Object(map[string]cty.Type{"host": cty.String, "port": cty.Number}),
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ty, diags := config.ParseVariableType("v", parseExpr(t, tt.src))

			require.False(t, diags.HasErrors(), "unexpected diagnostics: %s", diags.Error())
			assert.True(t, tt.expect.Equals(ty),
				"type mismatch: want %s, got %s", tt.expect.FriendlyName(), ty.FriendlyName())
		})
	}
}

// TestParseVariableType_LegacyQuotedScalars covers the v0.7
// transition shim accepting `type = "string"` / `"bool"` / `"number"`.
// `"int"` and `"choice"` have their own dedicated tests because they
// emit deprecation warnings / errors.
func TestParseVariableType_LegacyQuotedScalars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		src    string
		expect cty.Type
	}{
		{name: "quoted string", src: `"string"`, expect: cty.String},
		{name: "quoted bool", src: `"bool"`, expect: cty.Bool},
		{name: "quoted number", src: `"number"`, expect: cty.Number},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ty, diags := config.ParseVariableType("v", parseExpr(t, tt.src))

			require.False(t, diags.HasErrors(), "unexpected diagnostics: %s", diags.Error())
			assert.True(t, tt.expect.Equals(ty))
		})
	}
}

// TestParseVariableType_IntDeprecation covers DESIGN-0006 OQ-6:
// both `type = int` (bareword) and `type = "int"` (legacy quoted)
// continue to resolve to cty.Number but emit a DiagWarning.
func TestParseVariableType_IntDeprecation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{name: "bareword int", src: "int"},
		{name: "quoted int", src: `"int"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ty, diags := config.ParseVariableType("port", parseExpr(t, tt.src))

			require.False(t, diags.HasErrors(), "int alias should resolve, not error")
			assert.True(t, cty.Number.Equals(ty), "int should alias to cty.Number")

			require.Len(t, diags, 1, "expected exactly one warning diagnostic")
			assert.Equal(t, hcl.DiagWarning, diags[0].Severity)
			assert.Contains(t, diags[0].Summary, "deprecated")
			assert.Contains(t, diags[0].Detail, `"port"`)
			assert.Contains(t, diags[0].Detail, "type = number")
			assert.Contains(t, diags[0].Detail, "MIGRATION.md")
			require.NotNil(t, diags[0].Subject, "warning must carry source range")
		})
	}
}

// TestParseVariableType_RejectsChoice covers the removed `choice`
// type. Authors are pointed at the validation-block migration
// pattern per DESIGN-0006.
func TestParseVariableType_RejectsChoice(t *testing.T) {
	t.Parallel()

	_, diags := config.ParseVariableType("license", parseExpr(t, `"choice"`))

	require.True(t, diags.HasErrors())
	assert.Contains(t, diags[0].Summary, "choice")
	assert.Contains(t, diags[0].Detail, `"license"`)
	assert.Contains(t, diags[0].Detail, "validation")
	assert.Contains(t, diags[0].Detail, "MIGRATION.md")
}

// TestParseVariableType_RejectsUnsupportedConstructors covers the
// v0.7 non-goals: tuple and set types are valid cty but rejected
// by forge. Optional fields are rejected by typeexpr.Type itself.
func TestParseVariableType_RejectsUnsupportedConstructors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		src            string
		wantInDetail   string
		wantInSummary  string
		wantPointAtRef bool
	}{
		{
			name:           "tuple",
			src:            "tuple([string, number])",
			wantInSummary:  "tuple",
			wantInDetail:   "tuple",
			wantPointAtRef: true,
		},
		{
			name:           "set",
			src:            "set(string)",
			wantInSummary:  "set",
			wantInDetail:   "set",
			wantPointAtRef: true,
		},
		{
			// Optional is rejected by typeexpr.Type itself (it only
			// permits optional inside TypeConstraintWithDefaults).
			// We don't own the wording but verify the rejection.
			name:          "optional inside object",
			src:           "object({k = optional(string)})",
			wantInSummary: "",
			wantInDetail:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, diags := config.ParseVariableType("v", parseExpr(t, tt.src))

			require.True(t, diags.HasErrors(), "expected rejection for %s", tt.name)

			if tt.wantInSummary != "" {
				assert.True(t, anyDiagContains(diags, "Summary", tt.wantInSummary),
					"no diagnostic Summary contained %q; diags=%s", tt.wantInSummary, diags.Error())
			}

			if tt.wantInDetail != "" {
				assert.True(t, anyDiagContains(diags, "Detail", tt.wantInDetail),
					"no diagnostic Detail contained %q; diags=%s", tt.wantInDetail, diags.Error())
			}

			if tt.wantPointAtRef {
				assert.True(t, anyDiagContains(diags, "Detail", "REFERENCE.md"),
					"no diagnostic pointed at REFERENCE.md; diags=%s", diags.Error())
			}
		})
	}
}

// TestParseVariableType_RejectsNestedUnsupported covers the
// recursive walk: a set buried inside an object is still rejected.
func TestParseVariableType_RejectsNestedUnsupported(t *testing.T) {
	t.Parallel()

	_, diags := config.ParseVariableType("v", parseExpr(t, `object({tags = set(string)})`))

	require.True(t, diags.HasErrors())
	assert.True(t, anyDiagContains(diags, "Summary", "set"),
		"nested set inside object must be rejected; diags=%s", diags.Error())
}

// TestParseVariableType_RejectsAny verifies that the cty "any" /
// DynamicPseudoType is rejected — forge variables must have
// concrete declared types. typeexpr.Type (vs typeexpr.TypeConstraint)
// owns this rejection.
func TestParseVariableType_RejectsAny(t *testing.T) {
	t.Parallel()

	_, diags := config.ParseVariableType("v", parseExpr(t, "any"))

	require.True(t, diags.HasErrors(), "expected rejection of `any` type")
}

// TestParseVariableType_RejectsGarbage verifies the parser surfaces
// a clean error for an expression that isn't a type expression at
// all (a number literal in the type slot, in this case). The
// underlying typeexpr error format is not our contract — we just
// verify that errors are surfaced rather than swallowed.
func TestParseVariableType_RejectsGarbage(t *testing.T) {
	t.Parallel()

	_, diags := config.ParseVariableType("v", parseExpr(t, "42"))
	require.True(t, diags.HasErrors())
}

// anyDiagContains reports whether any diagnostic in diags has the
// given field (Summary or Detail) containing substr. Helper for the
// rejection-test assertions where the exact wording of typeexpr's
// own diagnostics isn't our contract — we just want to know "did a
// helpful message reach the user?".
func anyDiagContains(diags hcl.Diagnostics, field, substr string) bool {
	for _, d := range diags {
		var hay string

		switch field {
		case "Summary":
			hay = d.Summary
		case "Detail":
			hay = d.Detail
		}

		if strings.Contains(hay, substr) {
			return true
		}
	}

	return false
}
