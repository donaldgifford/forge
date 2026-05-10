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

func TestRunMigrate_HappyPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fixtureBlueprintV1(t, root)

	result, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: root})
	require.NoError(t, err)
	require.Len(t, result.Blueprints, 1)

	report := result.Blueprints[0]
	assert.True(t, report.Migrated)
	assert.False(t, report.AlreadyV2)
	assert.Empty(t, report.UntranslatedHits)

	registry, err := os.ReadFile(filepath.Join(root, "registry.yaml"))
	require.NoError(t, err)

	var rg struct {
		APIVersion string `yaml:"apiVersion"`
	}
	require.NoError(t, yaml.Unmarshal(registry, &rg))
	assert.Equal(t, "v2", rg.APIVersion)

	bpData, err := os.ReadFile(filepath.Join(root, "go", "api", "blueprint.yaml"))
	require.NoError(t, err)

	var bp struct {
		APIVersion string `yaml:"apiVersion"`
		Variables  []struct {
			Name    string `yaml:"name"`
			Default string `yaml:"default"`
		} `yaml:"variables"`
		Conditions []struct {
			When string `yaml:"when"`
		} `yaml:"conditions"`
		Rename map[string]string `yaml:"rename"`
	}
	require.NoError(t, yaml.Unmarshal(bpData, &bp))

	assert.Equal(t, "v2", bp.APIVersion)
	assert.Equal(t, "github.com/example/${project_name}", bp.Variables[1].Default)
	assert.Equal(t, "!use_grpc", bp.Conditions[0].When)
	assert.Equal(t, ".", bp.Rename["${project_name}/"])

	tmplPath := filepath.Join(root, "go", "api", "{{project_name}}", "README.md.tmpl")
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

	result, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: root})
	require.NoError(t, err)
	require.Len(t, result.Blueprints, 1)
	assert.True(t, result.Blueprints[0].AlreadyV2)
	assert.False(t, result.Blueprints[0].Migrated)
}
