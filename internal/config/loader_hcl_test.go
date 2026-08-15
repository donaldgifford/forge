package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/config"
)

// TestLoadBlueprintHCL covers happy-path decoding of a blueprint.hcl
// containing every block kind the schema supports: top-level attrs,
// defaults, variables (with templated default + a validation block),
// conditions (with a parsed when expression), hooks, sync (with
// managed_files), and a rename map.
func TestLoadBlueprintHCL(t *testing.T) {
	t.Parallel()

	const src = `
name        = "go-api"
description = "Go API service"
version     = "1.0.0"
tags        = ["go", "api"]

defaults {
  exclude           = [".pre-commit-config.yaml"]
  override_strategy = { "renovate.json" = "merge" }
}

variable "project_name" {
  description = "Name of the project"
  type        = string
  required    = true
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]*$", var.project_name))
    error_message = "project_name must be kebab-case."
  }
}

variable "go_module" {
  type    = string
  default = "github.com/example/${project_name}"
}

variable "license" {
  type    = string
  default = "Apache-2.0"
  validation {
    condition     = contains(["MIT", "Apache-2.0", "BSD-3-Clause", "none"], var.license)
    error_message = "license must be one of: MIT, Apache-2.0, BSD-3-Clause, none."
  }
}

condition {
  when    = !use_grpc
  exclude = ["proto/"]
}

hooks {
  post_create = ["go mod tidy"]
}

sync {
  ignore = ["dist/"]

  managed_file "Makefile" {
    strategy = "merge"
  }
}

rename {
  entry {
    from = "${project_name}/"
    to   = "."
  }
}
`

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.hcl")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	bp, err := config.LoadBlueprintHCL(path)
	require.NoError(t, err)
	require.NotNil(t, bp)

	assert.Equal(t, "go-api", bp.Name)
	assert.Equal(t, "Go API service", bp.Description)
	assert.Equal(t, "1.0.0", bp.Version)
	assert.Equal(t, []string{"go", "api"}, bp.Tags)

	assert.Contains(t, bp.Defaults.Exclude, ".pre-commit-config.yaml")
	assert.Equal(t, "merge", bp.Defaults.OverrideStrategy["renovate.json"])

	require.Len(t, bp.Variables, 3)
	assert.Equal(t, "project_name", bp.Variables[0].Name)
	assert.True(t, cty.String.Equals(bp.Variables[0].Type))
	assert.True(t, bp.Variables[0].Required)
	require.Len(t, bp.Variables[0].Validations, 1,
		"validation block must decode into Variable.Validations")
	assert.Equal(t, "project_name must be kebab-case.",
		bp.Variables[0].Validations[0].ErrorMessage)
	require.NotNil(t, bp.Variables[0].Validations[0].Condition,
		"validation.condition must be a parsed hcl.Expression")

	assert.Equal(t, "go_module", bp.Variables[1].Name)
	assert.Equal(t, "github.com/example/${project_name}", bp.Variables[1].DefaultSource,
		"templated default round-trips with outer quotes stripped (sourceText behaviour, prompt-renderer-compatible)")
	require.NotNil(t, bp.Variables[1].DefaultExpr,
		"default must be captured as a parsed hcl.Expression")

	assert.Equal(t, "license", bp.Variables[2].Name)
	assert.True(t, cty.String.Equals(bp.Variables[2].Type),
		"license declared as `type = string` — choice → string is the v0.7 path")
	require.Len(t, bp.Variables[2].Validations, 1)

	require.Len(t, bp.Conditions, 1)
	require.NotNil(t, bp.Conditions[0].When, "condition.when must be a parsed hcl.Expression")
	assert.Equal(t, "!use_grpc", bp.Conditions[0].WhenSource)
	assert.Equal(t, []string{"proto/"}, bp.Conditions[0].Exclude)

	// The parsed When expression must be evaluable.
	val, diags := bp.Conditions[0].When.Value(nil)
	assert.True(t, diags.HasErrors(), "evaluating without vars should fail")
	_ = val

	assert.Contains(t, bp.Hooks.PostCreate, "go mod tidy")

	require.Len(t, bp.Sync.ManagedFiles, 1)
	assert.Equal(t, "Makefile", bp.Sync.ManagedFiles[0].Path)
	assert.Equal(t, "merge", bp.Sync.ManagedFiles[0].Strategy)
	assert.Equal(t, []string{"dist/"}, bp.Sync.Ignore)

	assert.Equal(t, ".", bp.Rename["${project_name}/"])
}

// TestLoadBlueprintHCL_MissingRequired surfaces a diagnostic when a
// required attribute is omitted.
func TestLoadBlueprintHCL_MissingRequired(t *testing.T) {
	t.Parallel()

	const src = `
description = "missing name"
`

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.hcl")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	_, err := config.LoadBlueprintHCL(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

// TestLoadBlueprintHCL_ConditionWhenIsParsed confirms the condition.when
// expression is evaluable against a populated context — proves the
// load-time parse delivered a usable hcl.Expression, not a placeholder.
func TestLoadBlueprintHCL_ConditionWhenIsParsed(t *testing.T) {
	t.Parallel()

	const src = `
name = "x"

condition {
  when    = use_grpc == true
  exclude = ["proto/"]
}
`

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.hcl")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	bp, err := config.LoadBlueprintHCL(path)
	require.NoError(t, err)
	require.Len(t, bp.Conditions, 1)

	cond := bp.Conditions[0]
	require.NotNil(t, cond.When)

	// Evaluate the expression against a real context.
	ctx := &hcl.EvalContext{
		Variables: map[string]cty.Value{"use_grpc": cty.True},
	}

	v, diags := cond.When.Value(ctx)
	require.False(t, diags.HasErrors(), "evaluation diags: %s", diags.Error())
	assert.True(t, v.True())
}

// TestLoadBlueprintHCL_MalformedConditionWhen verifies a syntactically
// broken `condition.when` surfaces a parse diagnostic at load time
// (per OQ-7) rather than waiting for the first evaluation.
func TestLoadBlueprintHCL_MalformedConditionWhen(t *testing.T) {
	t.Parallel()

	const src = `
name = "x"

condition {
  when    = use_grpc ==
  exclude = ["proto/"]
}
`

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.hcl")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	_, err := config.LoadBlueprintHCL(path)
	require.Error(t, err, "broken when expression must fail at load time")
}

// TestLoadBlueprintHCL_VariableMissingType exercises the field-level
// validator (Variable.Type required) on the HCL path.
func TestLoadBlueprintHCL_VariableMissingType(t *testing.T) {
	t.Parallel()

	const src = `
name = "x"

variable "missing_type" {
  description = "no type set"
}
`

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.hcl")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	_, err := config.LoadBlueprintHCL(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type is required")
}

// TestLoadBlueprintHCL_StructuredTypes verifies object/list/map type
// declarations decode into the expected cty.Type shape on Variable.Type.
// This is the load-time half of IMPL-0009 Phase B's acceptance criteria;
// Phases C–F bring up the matching consumer paths.
func TestLoadBlueprintHCL_StructuredTypes(t *testing.T) {
	t.Parallel()

	const src = `
name = "x"

variable "exposed_ports" {
  type = list(number)
}

variable "build_targets" {
  type = map(string)
}

variable "git_provider" {
  type = object({
    repo_type    = string
    repo_url     = string
    project_org  = string
  })
}
`

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.hcl")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	bp, err := config.LoadBlueprintHCL(path)
	require.NoError(t, err)
	require.Len(t, bp.Variables, 3)

	assert.True(t, cty.List(cty.Number).Equals(bp.Variables[0].Type),
		"list(number) declaration must round-trip as cty.List(cty.Number)")
	assert.True(t, cty.Map(cty.String).Equals(bp.Variables[1].Type),
		"map(string) declaration must round-trip as cty.Map(cty.String)")

	wantObj := cty.Object(map[string]cty.Type{
		"repo_type":   cty.String,
		"repo_url":    cty.String,
		"project_org": cty.String,
	})
	assert.True(t, wantObj.Equals(bp.Variables[2].Type),
		"object({...}) declaration must round-trip as cty.Object(...)")
	assert.Equal(t, "list(number)", bp.Variables[0].TypeSource,
		"TypeSource preserves the raw author text for diagnostics")
}

// TestLoadBlueprintHCL_RejectsLegacyChoices confirms `choices = [...]`
// errors at load time with a MIGRATION.md pointer per IMPL-0009 OQ-4.
func TestLoadBlueprintHCL_RejectsLegacyChoices(t *testing.T) {
	t.Parallel()

	const src = `
name = "x"

variable "license" {
  type    = string
  choices = ["MIT", "Apache-2.0"]
}
`

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.hcl")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	_, err := config.LoadBlueprintHCL(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "choices", "error must name the removed field")
	assert.Contains(t, err.Error(), "validation", "error must point at the new validation block")
	assert.Contains(t, err.Error(), "MIGRATION.md", "error must carry a MIGRATION.md anchor")
}

// TestLoadBlueprintHCL_RejectsLegacyValidate confirms the same pattern
// for `validate = "regex"`.
func TestLoadBlueprintHCL_RejectsLegacyValidate(t *testing.T) {
	t.Parallel()

	const src = `
name = "x"

variable "project_name" {
  type     = string
  validate = "^[a-z][a-z0-9-]*$"
}
`

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.hcl")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	_, err := config.LoadBlueprintHCL(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validate")
	assert.Contains(t, err.Error(), "can(regex(")
	assert.Contains(t, err.Error(), "MIGRATION.md")
}

// TestLoadBlueprintHCL_IntDeprecationFlowsThrough covers the
// load-to-Deprecations contract: a bareword `int` resolves to
// cty.Number (so the blueprint still loads) and emits a single
// Deprecation entry naming the variable, the int→number recommendation,
// and the MIGRATION.md anchor. IMPL-0009 OQ-3.
func TestLoadBlueprintHCL_IntDeprecationFlowsThrough(t *testing.T) {
	t.Parallel()

	const src = `
name = "x"

variable "port" {
  type = int
}
`

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.hcl")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	bp, err := config.LoadBlueprintHCL(path)
	require.NoError(t, err)
	assert.True(t, cty.Number.Equals(bp.Variables[0].Type))

	require.Len(t, bp.Deprecations, 1, "int alias must produce exactly one deprecation")
	d := bp.Deprecations[0]
	assert.Equal(t, "port", d.Variable)
	assert.Contains(t, d.Summary, "deprecated")
	assert.Contains(t, d.Detail, "type = number")
	assert.Contains(t, d.Detail, "MIGRATION.md")
	assert.Positive(t, d.Subject.Start.Line,
		"deprecation must carry a source range for file:line:col diagnostics")
}

// TestLoadBlueprintHCL_RenameEntryMissingTo confirms required-attr
// diagnostics surface from inside lazy blocks too.
func TestLoadBlueprintHCL_RenameEntryMissingTo(t *testing.T) {
	t.Parallel()

	const src = `
name = "x"

rename {
  entry {
    from = "src/"
  }
}
`

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.hcl")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	_, err := config.LoadBlueprintHCL(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "to")
}

// TestLoadRegistryHCL_MissingPath confirms the field-level validator
// runs on the HCL path.
func TestLoadRegistryHCL_MissingPath(t *testing.T) {
	t.Parallel()

	const src = `
name = "r"

blueprint "go-api" {
  description = "missing path"
}
`

	dir := t.TempDir()
	path := filepath.Join(dir, "registry.hcl")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	_, err := config.LoadRegistryHCL(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"path" is required`)
}

// TestLoadBlueprint_PrefersHCLSibling verifies the dispatcher picks the
// HCL sibling when a caller passes the legacy blueprint.yaml path.
// Important during the cutover window so callers that hardcoded the
// YAML filename keep working transparently against migrated trees.
func TestLoadBlueprint_PrefersHCLSibling(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "blueprint.yaml")
	hclPath := filepath.Join(dir, "blueprint.hcl")

	require.NoError(t, os.WriteFile(yamlPath, []byte("name: from-yaml\n"), 0o600))
	require.NoError(t, os.WriteFile(hclPath, []byte(`name = "from-hcl"`+"\n"), 0o600))

	bp, err := config.LoadBlueprint(yamlPath)
	require.NoError(t, err)
	assert.Equal(t, "from-hcl", bp.Name, "HCL sibling must win over the YAML input")
}

// TestLoadRegistry_PrefersHCLSibling mirrors the blueprint check.
func TestLoadRegistry_PrefersHCLSibling(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "registry.yaml")
	hclPath := filepath.Join(dir, "registry.hcl")

	require.NoError(t, os.WriteFile(yamlPath, []byte("name: from-yaml\n"), 0o600))
	require.NoError(t, os.WriteFile(hclPath, []byte(`name = "from-hcl"`+"\n"), 0o600))

	reg, err := config.LoadRegistry(yamlPath)
	require.NoError(t, err)
	assert.Equal(t, "from-hcl", reg.Name)
}

// TestLoadBlueprintHCL_HermeticFixture exercises the loader against
// the in-tree testdata/hcl-registry/ fixture. Acts as a regression
// guard so future schema changes can't silently break the canonical
// HCL config corpus.
func TestLoadBlueprintHCL_HermeticFixture(t *testing.T) {
	t.Parallel()

	bp, err := config.LoadBlueprintHCL("../../testdata/hcl-registry/go/api/blueprint.hcl")
	require.NoError(t, err)

	assert.Equal(t, "go-api", bp.Name)
	assert.Equal(t, "0.4.0", bp.Version)
	assert.Equal(t, []string{"go", "api", "grpc"}, bp.Tags)

	require.Len(t, bp.Variables, 3)
	assert.Equal(t, "project_name", bp.Variables[0].Name)
	assert.Equal(t, "github.com/example/${project_name}", bp.Variables[1].DefaultSource)

	require.Len(t, bp.Conditions, 1)
	assert.Equal(t, "!use_grpc", bp.Conditions[0].WhenSource)

	assert.Equal(t, ".", bp.Rename["${project_name}/"])
}

// TestLoadRegistryHCL_HermeticFixture mirrors the blueprint fixture
// test for the registry-level config.
func TestLoadRegistryHCL_HermeticFixture(t *testing.T) {
	t.Parallel()

	reg, err := config.LoadRegistryHCL("../../testdata/hcl-registry/registry.hcl")
	require.NoError(t, err)

	assert.Equal(t, "test-blueprints-hcl", reg.Name)
	require.Len(t, reg.Maintainers, 1)
	assert.Equal(t, "Test Team", reg.Maintainers[0].Name)
	assert.Equal(t, "overwrite", reg.Defaults.SyncStrategy)
	assert.True(t, reg.Defaults.Managed)

	require.Len(t, reg.Blueprints, 2)
	assert.Equal(t, "go/api", reg.Blueprints[0].Name)
	assert.Equal(t, "go/api", reg.Blueprints[0].Path)
	assert.Equal(t, "0.4.0", reg.Blueprints[0].Version)
	assert.Equal(t, []string{"go", "api", "grpc"}, reg.Blueprints[0].Tags)
	assert.Equal(t, "hcl-fixture", reg.Blueprints[0].LatestCommit)
}

// TestLoadRegistryHCL covers happy-path registry decoding.
func TestLoadRegistryHCL(t *testing.T) {
	t.Parallel()

	const src = `
name        = "my-registry"
description = "Test registry"

blueprint "go-api" {
  path        = "go/api"
  description = "Go API service"
  tags        = ["go", "api"]
}

blueprint "go-cli" {
  path = "go/cli"
}
`

	dir := t.TempDir()
	path := filepath.Join(dir, "registry.hcl")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	reg, err := config.LoadRegistryHCL(path)
	require.NoError(t, err)
	require.NotNil(t, reg)

	assert.Equal(t, "my-registry", reg.Name)
	assert.Equal(t, "Test registry", reg.Description)

	require.Len(t, reg.Blueprints, 2)
	assert.Equal(t, "go-api", reg.Blueprints[0].Name)
	assert.Equal(t, "go/api", reg.Blueprints[0].Path)
	assert.Equal(t, []string{"go", "api"}, reg.Blueprints[0].Tags)

	assert.Equal(t, "go-cli", reg.Blueprints[1].Name)
	assert.Equal(t, "go/cli", reg.Blueprints[1].Path)
}
