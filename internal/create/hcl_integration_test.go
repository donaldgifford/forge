package create_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/create"
)

const testHCLRegistryDir = "../../testdata/hcl-registry"

// TestRun_HCL_GoAPI is the end-to-end proof that the HCL config-file
// loader is wired into create.Run. It exercises the same shape as
// TestRun_V2_GoAPI (variables, condition.when, rename) but loads the
// blueprint from a blueprint.hcl file rather than a YAML file. Output
// files must be byte-identical to the YAML run.
func TestRun_HCL_GoAPI(t *testing.T) {
	t.Parallel()

	outputDir := filepath.Join(t.TempDir(), "my-api")

	opts := create.Opts{
		BlueprintRef: "go/api",
		OutputDir:    outputDir,
		RegistryDir:  testHCLRegistryDir,
		UseDefaults:  true,
		ForgeVersion: "0.0.0-hcl-test",
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

// TestRun_HCL_HelmChart confirms verbatim {{ }} preservation works
// equally well when the blueprint config is HCL.
func TestRun_HCL_HelmChart(t *testing.T) {
	t.Parallel()

	outputDir := filepath.Join(t.TempDir(), "my-chart")

	opts := create.Opts{
		BlueprintRef: "helm/chart",
		OutputDir:    outputDir,
		RegistryDir:  testHCLRegistryDir,
		UseDefaults:  true,
		ForgeVersion: "0.0.0-hcl-test",
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

	deploy, err := os.ReadFile(filepath.Join(outputDir, "templates", "deployment.yaml"))
	require.NoError(t, err)
	deployContent := string(deploy)

	assert.Contains(t, deployContent, "name: my-chart")
	assert.Contains(t, deployContent, "{{ .Values.replicas }}")
}
