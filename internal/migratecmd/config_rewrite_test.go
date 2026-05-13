package migratecmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/migratecmd"
)

// TestRewriteBlueprintYAMLToHCL_RoundTrip is the core proof: take a
// real v2 blueprint YAML, rewrite to HCL, load the HCL back, and
// confirm the resulting Blueprint matches the YAML-loaded source.
func TestRewriteBlueprintYAMLToHCL_RoundTrip(t *testing.T) {
	t.Parallel()

	src := []byte(`
apiVersion: v2
name: "go-api"
description: "Test"
version: "2.0.0"
tags: ["go", "api"]

defaults:
  exclude: [".pre-commit-config.yaml"]
  override_strategy:
    "renovate.json": "merge"

variables:
  - name: project_name
    description: "Name of the project"
    type: string
    required: true
    validate: "^[a-z][a-z0-9-]*$"

  - name: go_module
    type: string
    default: "github.com/example/${project_name}"

  - name: license
    type: choice
    choices: ["MIT", "Apache-2.0", "BSD-3-Clause", "none"]

conditions:
  - when: "!use_grpc"
    exclude:
      - "proto/"

hooks:
  post_create:
    - "go mod tidy"

sync:
  ignore: ["dist/"]
  managed_files:
    - path: "Makefile"
      strategy: "merge"

rename:
  "${project_name}/": "."
`)

	hclBytes, err := migratecmd.RewriteBlueprintYAMLToHCL(src)
	require.NoError(t, err)
	require.NotEmpty(t, hclBytes)

	// Write the HCL to disk so the loader's source-range slicing has
	// real bytes to work with.
	dir := t.TempDir()
	hclPath := filepath.Join(dir, "blueprint.hcl")
	require.NoError(t, os.WriteFile(hclPath, hclBytes, 0o600))

	// Load the rewritten HCL back. apiVersion is dropped, so the HCL
	// loader's validateBlueprintFields path runs.
	bp, err := config.LoadBlueprintHCL(hclPath)
	require.NoError(t, err, "rewritten HCL did not parse — output was:\n%s", hclBytes)

	assert.Equal(t, "go-api", bp.Name)
	assert.Equal(t, "Test", bp.Description)
	assert.Equal(t, "2.0.0", bp.Version)
	assert.Equal(t, []string{"go", "api"}, bp.Tags)

	assert.Contains(t, bp.Defaults.Exclude, ".pre-commit-config.yaml")
	assert.Equal(t, "merge", bp.Defaults.OverrideStrategy["renovate.json"])

	require.Len(t, bp.Variables, 3)
	assert.Equal(t, "project_name", bp.Variables[0].Name)
	assert.True(t, bp.Variables[0].Required)
	assert.Equal(t, "github.com/example/${project_name}", bp.Variables[1].Default,
		"templated default must round-trip with $ syntax intact")
	assert.Equal(t, []string{"MIT", "Apache-2.0", "BSD-3-Clause", "none"}, bp.Variables[2].Choices)

	require.Len(t, bp.Conditions, 1)
	assert.Equal(t, "!use_grpc", bp.Conditions[0].WhenSource)
	assert.Equal(t, []string{"proto/"}, bp.Conditions[0].Exclude)

	assert.Contains(t, bp.Hooks.PostCreate, "go mod tidy")

	require.Len(t, bp.Sync.ManagedFiles, 1)
	assert.Equal(t, "Makefile", bp.Sync.ManagedFiles[0].Path)
	assert.Equal(t, "merge", bp.Sync.ManagedFiles[0].Strategy)

	assert.Equal(t, ".", bp.Rename["${project_name}/"])
}

// TestRewriteRegistryYAMLToHCL_RoundTrip mirrors the blueprint test
// for registry.yaml.
func TestRewriteRegistryYAMLToHCL_RoundTrip(t *testing.T) {
	t.Parallel()

	src := []byte(`
apiVersion: v2
name: "test-registry"
description: "Test"

maintainers:
  - name: "Team"
    email: "team@example.com"

defaults:
  sync_strategy: overwrite
  managed: true

blueprints:
  - name: go/api
    path: go/api
    description: "Go API"
    version: "2.0.0"
    tags: ["go", "api"]
    latest_commit: "abc123"
`)

	hclBytes, err := migratecmd.RewriteRegistryYAMLToHCL(src)
	require.NoError(t, err)

	dir := t.TempDir()
	hclPath := filepath.Join(dir, "registry.hcl")
	require.NoError(t, os.WriteFile(hclPath, hclBytes, 0o600))

	reg, err := config.LoadRegistryHCL(hclPath)
	require.NoError(t, err, "rewritten HCL did not parse — output was:\n%s", hclBytes)

	assert.Equal(t, "test-registry", reg.Name)
	require.Len(t, reg.Maintainers, 1)
	assert.Equal(t, "Team", reg.Maintainers[0].Name)
	assert.True(t, reg.Defaults.Managed)

	require.Len(t, reg.Blueprints, 1)
	assert.Equal(t, "go/api", reg.Blueprints[0].Name)
	assert.Equal(t, []string{"go", "api"}, reg.Blueprints[0].Tags)
	assert.Equal(t, "abc123", reg.Blueprints[0].LatestCommit)
}

// TestRewriteBlueprintYAMLToHCL_DropsAPIVersion confirms apiVersion is
// stripped on emit (per OQ-2: file extension is the version signal).
func TestRewriteBlueprintYAMLToHCL_DropsAPIVersion(t *testing.T) {
	t.Parallel()

	src := []byte(`
apiVersion: v2
name: "x"
`)

	hclBytes, err := migratecmd.RewriteBlueprintYAMLToHCL(src)
	require.NoError(t, err)

	assert.NotContains(t, string(hclBytes), "apiVersion",
		"apiVersion should be dropped on HCL emit")
	assert.NotContains(t, string(hclBytes), "api_version")
}

// TestRewriteBlueprintYAMLToHCL_MinimalInput exercises the smallest
// valid blueprint: a name and nothing else. Confirms the rewriter
// doesn't crash on empty defaults/variables/conditions and produces
// loadable HCL.
func TestRewriteBlueprintYAMLToHCL_MinimalInput(t *testing.T) {
	t.Parallel()

	src := []byte(`
apiVersion: v2
name: "minimal"
`)

	hclBytes, err := migratecmd.RewriteBlueprintYAMLToHCL(src)
	require.NoError(t, err)

	dir := t.TempDir()
	hclPath := filepath.Join(dir, "blueprint.hcl")
	require.NoError(t, os.WriteFile(hclPath, hclBytes, 0o600))

	bp, err := config.LoadBlueprintHCL(hclPath)
	require.NoError(t, err, "minimal HCL output should still load: %s", hclBytes)
	assert.Equal(t, "minimal", bp.Name)
	assert.Empty(t, bp.Variables)
	assert.Empty(t, bp.Conditions)
}

// TestRewriteBlueprintYAMLToHCL_RejectsHCLInput verifies the rewriter
// fails (rather than silently misinterpreting) when fed an HCL byte
// slice instead of YAML. The walker's collision check covers the
// user-level idempotence story, but the rewriter itself shouldn't try
// to parse HCL as YAML.
func TestRewriteBlueprintYAMLToHCL_RejectsHCLInput(t *testing.T) {
	t.Parallel()

	hclSrc := []byte(`
name = "x"

variable "foo" {
  type = "string"
}
`)

	// gopkg.in/yaml.v3 parses bare-word HCL surprisingly leniently —
	// the test must accept either an explicit error or a result that
	// fails to round-trip. We check the latter: if no error, the result
	// must not load as a valid blueprint.
	out, err := migratecmd.RewriteBlueprintYAMLToHCL(hclSrc)
	if err != nil {
		return
	}

	dir := t.TempDir()
	hclPath := filepath.Join(dir, "blueprint.hcl")
	require.NoError(t, os.WriteFile(hclPath, out, 0o600))

	if _, loadErr := config.LoadBlueprintHCL(hclPath); loadErr == nil {
		t.Errorf("expected HCL input to fail rewrite or fail re-load, got valid output: %s", out)
	}
}
