package migratecmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/donaldgifford/forge/internal/migratecmd"
)

// fixtureBlueprintV1 builds a small v1 blueprint tree under root for the
// walker tests.
func fixtureBlueprintV1(t *testing.T, root string) {
	t.Helper()

	bpDir := filepath.Join(root, "go", "api")
	require.NoError(t, os.MkdirAll(filepath.Join(bpDir, "{{project_name}}"), 0o755))

	registry := []byte(`apiVersion: v1
name: "test"
blueprints:
  - name: go/api
    path: go/api
`)
	require.NoError(t, os.WriteFile(filepath.Join(root, "registry.yaml"), registry, 0o644))

	blueprint := []byte(`apiVersion: v1
name: "go-api"
description: "Go API"
version: "1.0.0"

variables:
  - name: project_name
    type: string

  - name: go_module
    type: string
    default: "github.com/example/{{ .project_name }}"

conditions:
  - when: "{{ not .use_grpc }}"
    exclude:
      - "proto/"

rename:
  "{{project_name}}/": "."
`)
	require.NoError(t, os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), blueprint, 0o644))

	tmpl := []byte("# {{ .project_name }}\n")
	require.NoError(t, os.WriteFile(filepath.Join(bpDir, "{{project_name}}", "README.md.tmpl"), tmpl, 0o644))
}

// TestRunMigrate_RenamesDottedDirForm covers the `{{ .name }}` dotted form
// that some authors used in directory names (e.g. forge-registry's
// go/_defaults/cmd/{{ .project_name }}/). The shorthand-only renamer
// missed these; this fixture pins the regression.
func TestRunMigrate_RenamesDottedDirForm(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	bpDir := filepath.Join(root, "go", "api")
	require.NoError(t, os.MkdirAll(filepath.Join(bpDir, "cmd", "{{ .project_name }}"), 0o755))

	registry := []byte(`apiVersion: v1
name: "test"
blueprints:
  - name: go/api
    path: go/api
`)
	require.NoError(t, os.WriteFile(filepath.Join(root, "registry.yaml"), registry, 0o644))

	blueprint := []byte(`apiVersion: v1
name: "go-api"
version: "1.0.0"

variables:
  - name: project_name
    type: string
`)
	require.NoError(t, os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), blueprint, 0o644))

	mainTmpl := []byte("package main\n")
	require.NoError(t, os.WriteFile(
		filepath.Join(bpDir, "cmd", "{{ .project_name }}", "main.go.tmpl"),
		mainTmpl, 0o644,
	))

	_, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: root, Force: true})
	require.NoError(t, err)

	renamedPath := filepath.Join(bpDir, "cmd", "${project_name}", "main.go.tmpl")
	assert.FileExists(t, renamedPath, "dotted-form directory should be renamed to ${name}")

	_, err = os.Stat(filepath.Join(bpDir, "cmd", "{{ .project_name }}"))
	assert.True(t, os.IsNotExist(err), "original dotted-form directory should be gone")
}

func TestRunMigrate_HappyPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fixtureBlueprintV1(t, root)

	result, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: root, Force: true})
	require.NoError(t, err)
	require.Len(t, result.Blueprints, 1)

	report := result.Blueprints[0]
	assert.True(t, report.Migrated)
	assert.False(t, report.AlreadyV2)
	assert.Empty(t, report.UntranslatedHits)

	// Per OQ-4 the templates migrator no longer touches apiVersion —
	// the field is meaningless to the loader and migrate config drops
	// it on emit. registry.yaml stays untouched (no expression fields
	// there); blueprint.yaml retains its v1 apiVersion but every
	// expression field is now HCL2 syntax.
	bpData, err := os.ReadFile(filepath.Join(root, "go", "api", "blueprint.yaml"))
	require.NoError(t, err)

	var bp struct {
		Variables []struct {
			Name    string `yaml:"name"`
			Default string `yaml:"default"`
		} `yaml:"variables"`
		Conditions []struct {
			When string `yaml:"when"`
		} `yaml:"conditions"`
		Rename map[string]string `yaml:"rename"`
	}
	require.NoError(t, yaml.Unmarshal(bpData, &bp))

	assert.Equal(t, "github.com/example/${project_name}", bp.Variables[1].Default)
	assert.Equal(t, "!use_grpc", bp.Conditions[0].When)
	assert.Equal(t, ".", bp.Rename["${project_name}/"])

	tmplPath := filepath.Join(root, "go", "api", "${project_name}", "README.md.tmpl")
	tmpl, err := os.ReadFile(tmplPath)
	require.NoError(t, err)
	assert.Equal(t, "# ${project_name}\n", string(tmpl))
}

func TestRunMigrate_DryRunWritesNothing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fixtureBlueprintV1(t, root)

	bpPath := filepath.Join(root, "go", "api", "blueprint.yaml")
	before, err := os.ReadFile(bpPath)
	require.NoError(t, err)

	result, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{
		Path:   root,
		DryRun: true,
	})
	require.NoError(t, err)
	require.Len(t, result.Blueprints, 1)
	assert.False(t, result.Blueprints[0].Migrated)

	after, err := os.ReadFile(bpPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

func TestRunMigrate_AlreadyV2Skips(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	bpDir := filepath.Join(root, "v2")
	require.NoError(t, os.MkdirAll(bpDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(bpDir, "blueprint.yaml"),
		[]byte("apiVersion: v2\nname: \"already\"\n"),
		0o644,
	))

	result, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: root, Force: true})
	require.NoError(t, err)
	require.Len(t, result.Blueprints, 1)
	assert.True(t, result.Blueprints[0].AlreadyV2)
	assert.False(t, result.Blueprints[0].Migrated)
}

// TestRunMigrate_HCLRootedBlueprint covers the case where a registry
// has already been through `forge migrate config` (so its config
// files are blueprint.hcl, not blueprint.yaml) but still has v1
// template syntax stranded in .tmpl files. Surfaced by
// donaldgifford/forge-registry — running `migrate config` before
// `migrate templates` left v1 `{{ .project_name }}` syntax in 15
// files because the walker only matched `blueprint.yaml`-rooted
// directories.
func TestRunMigrate_HCLRootedBlueprint(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	bpDir := filepath.Join(root, "go", "api")
	require.NoError(t, os.MkdirAll(filepath.Join(bpDir, "{{project_name}}"), 0o755))

	hclConfig := []byte(`name        = "go-api"
description = "Go API"
version     = "1.0.0"

variable "project_name" {
  type = "string"
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(bpDir, "blueprint.hcl"), hclConfig, 0o644))

	tmpl := []byte("# {{ .project_name }}\nmodule {{ .project_name }}\n")
	require.NoError(t, os.WriteFile(
		filepath.Join(bpDir, "{{project_name}}", "README.md.tmpl"),
		tmpl, 0o644,
	))

	result, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: root, Force: true})
	require.NoError(t, err)
	require.Len(t, result.Blueprints, 1)

	report := result.Blueprints[0]
	assert.True(t, report.Migrated, "HCL-rooted blueprint with v1 .tmpl content should migrate")
	assert.False(t, report.AlreadyV2, "AlreadyV2 only applies to YAML configs with apiVersion: v2")

	// Template content rewritten in place.
	renamedTmpl := filepath.Join(bpDir, "${project_name}", "README.md.tmpl")
	got, err := os.ReadFile(renamedTmpl)
	require.NoError(t, err)
	assert.Equal(t, "# ${project_name}\nmodule ${project_name}\n", string(got))

	// blueprint.hcl is untouched — its expression fields are already
	// HCL2; the migrator has no business rewriting them.
	hclAfter, err := os.ReadFile(filepath.Join(bpDir, "blueprint.hcl"))
	require.NoError(t, err)
	assert.Equal(t, string(hclConfig), string(hclAfter), "blueprint.hcl should not be modified")

	// Original {{project_name}} directory is gone.
	_, err = os.Stat(filepath.Join(bpDir, "{{project_name}}"))
	assert.True(t, os.IsNotExist(err), "v1-shape template directory should be renamed")
}

// TestRunMigrate_HCLRootedLeavesDownstreamSyntaxAlone pins the
// variable-scoped rewriter: `{{ .ProjectName }}` and `{{ .Env.FOO }}`
// in a .goreleaser-style template must pass through verbatim — they
// are goreleaser variables, not forge variables. Forge variables
// (single-level, snake_case identifiers declared in blueprint.hcl)
// still rewrite normally.
func TestRunMigrate_HCLRootedLeavesDownstreamSyntaxAlone(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	bpDir := filepath.Join(root, "go", "ext")
	require.NoError(t, os.MkdirAll(bpDir, 0o755))

	hclConfig := []byte(`name = "go-ext"

variable "project_name" {
  type = "string"
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(bpDir, "blueprint.hcl"), hclConfig, 0o644))

	// .goreleaser.yml.tmpl-shaped content: forge vars are v1 dot-form
	// (need migration); goreleaser's CamelCase fields and Env chain
	// must be preserved as literal text.
	goreleaserTmpl := []byte(`builds:
  - id: {{ .project_name }}
    binary: {{ .project_name }}

archives:
  - name_template: '{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}'

signs:
  - args: ['--local-user', '{{ .Env.GPG_FINGERPRINT }}']
`)
	require.NoError(t, os.WriteFile(filepath.Join(bpDir, ".goreleaser.yml.tmpl"), goreleaserTmpl, 0o644))

	_, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: root, Force: true})
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(bpDir, ".goreleaser.yml.tmpl"))
	require.NoError(t, err)

	want := `builds:
  - id: ${project_name}
    binary: ${project_name}

archives:
  - name_template: '{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}'

signs:
  - args: ['--local-user', '{{ .Env.GPG_FINGERPRINT }}']
`
	assert.Equal(t, want, string(got))
}

// TestRunMigrate_HCLRootedIdempotent covers re-running the migrator
// against a fully-v2 HCL-rooted blueprint: the .tmpl rewriter is
// idempotent so the run reports unchanged status.
func TestRunMigrate_HCLRootedIdempotent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	bpDir := filepath.Join(root, "go", "api")
	require.NoError(t, os.MkdirAll(filepath.Join(bpDir, "${project_name}"), 0o755))

	hclConfig := []byte(`name = "go-api"

variable "project_name" {
  type = "string"
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(bpDir, "blueprint.hcl"), hclConfig, 0o644))

	tmpl := []byte("# ${project_name}\n")
	require.NoError(t, os.WriteFile(
		filepath.Join(bpDir, "${project_name}", "README.md.tmpl"),
		tmpl, 0o644,
	))

	result, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: root, Force: true})
	require.NoError(t, err)
	require.Len(t, result.Blueprints, 1)

	report := result.Blueprints[0]
	assert.False(t, report.Migrated, "fully-v2 HCL blueprint should report unchanged on re-run")
	assert.Empty(t, report.FilesRewritten)
}
