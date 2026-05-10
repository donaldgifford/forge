package create_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/create"
	tmpl "github.com/donaldgifford/forge/internal/template"
)

const testV2RegistryDir = "../../testdata/v2-registry"

// TestRun_V2_GoAPI exercises the experimental HCL2 path against the
// v2-registry fixture: ${project_name} path templating, !use_grpc
// condition expression, and the rename rule that strips the
// ${project_name}/ prefix all need to resolve through HCLRenderer.
func TestRun_V2_GoAPI(t *testing.T) {
	t.Parallel()

	outputDir := filepath.Join(t.TempDir(), "my-api")

	opts := create.Opts{
		BlueprintRef: "go/api",
		OutputDir:    outputDir,
		RegistryDir:  testV2RegistryDir,
		Renderer:     tmpl.NewHCLRenderer(),
		UseDefaults:  true,
		ForgeVersion: "0.0.0-v2-test",
		Overrides: map[string]string{
			"project_name": "my-api",
			"go_module":    "github.com/example/my-api",
			"use_grpc":     "false",
		},
	}

	result, err := create.Run(&opts)
	require.NoError(t, err)
	assert.Positive(t, result.FilesCreated)

	main, err := os.ReadFile(filepath.Join(outputDir, "cmd", "main.go"))
	require.NoError(t, err)
	assert.Contains(t, string(main), `Hello from my-api`)

	gomod, err := os.ReadFile(filepath.Join(outputDir, "go.mod"))
	require.NoError(t, err)
	assert.Contains(t, string(gomod), "github.com/example/my-api")
}

// TestRun_V2_HelmChart is the proof-of-life for the entire migration:
// the deployment.yaml.tmpl carries verbatim {{ .Values.x }} content
// alongside ${project_name} interpolations. After rendering, the
// {{ }} content must survive byte-for-byte while the ${ } expansions
// resolve.
func TestRun_V2_HelmChart(t *testing.T) {
	t.Parallel()

	outputDir := filepath.Join(t.TempDir(), "my-chart")

	opts := create.Opts{
		BlueprintRef: "helm/chart",
		OutputDir:    outputDir,
		RegistryDir:  testV2RegistryDir,
		Renderer:     tmpl.NewHCLRenderer(),
		UseDefaults:  true,
		ForgeVersion: "0.0.0-v2-test",
		Overrides: map[string]string{
			"project_name": "my-chart",
			"app_image":    "nginx",
		},
	}

	result, err := create.Run(&opts)
	require.NoError(t, err)
	assert.Positive(t, result.FilesCreated)

	chart, err := os.ReadFile(filepath.Join(outputDir, "Chart.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(chart), "name: my-chart")

	values, err := os.ReadFile(filepath.Join(outputDir, "values.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(values), "repository: nginx")

	deploy, err := os.ReadFile(filepath.Join(outputDir, "templates", "deployment.yaml"))
	require.NoError(t, err)
	deployContent := string(deploy)

	assert.Contains(t, deployContent, "name: my-chart")
	assert.Contains(t, deployContent, "{{ .Values.replicas }}")
	assert.Contains(t, deployContent, `{{ include "my-chart.name" . }}`)
	assert.Contains(t, deployContent, `image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"`)
	assert.Contains(t, deployContent, "{{ .Values.image.pullPolicy }}")
}
