package create_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/create"
	"github.com/donaldgifford/forge/internal/lockfile"
)

// buildStructuredRegistry stamps out a tiny synthetic registry under
// dir with one blueprint declaring an object variable (git_provider),
// a list(number) variable (exposed_ports), and a map(string) variable
// (build_targets). Used by the Phase D object-literal / list-map
// rejection tests to keep the test self-contained.
func buildStructuredRegistry(t *testing.T, dir string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "registry.hcl"), []byte(`
name        = "phase-d-test"
description = "synthetic registry for IMPL-0009 Phase D --set integration tests"

blueprint "demo/api" {
  path        = "demo/api"
  description = "structured-type demo"
  version     = "0.1.0"
}
`), 0o600))

	bpDir := filepath.Join(dir, "demo", "api")
	require.NoError(t, os.MkdirAll(bpDir, 0o750))

	require.NoError(t, os.WriteFile(filepath.Join(bpDir, "blueprint.hcl"), []byte(`
name        = "demo-api"
description = "structured-type demo"
version     = "0.1.0"

variable "project_name" {
  type     = string
  required = true
}

variable "git_provider" {
  type = object({
    repo_type   = string
    repo_url    = string
    project_org = string
  })
}

variable "exposed_ports" {
  type    = list(number)
  default = [8080]
}

variable "build_targets" {
  type    = map(string)
  default = { linux = "amd64" }
}
`), 0o600))

	require.NoError(t, os.WriteFile(filepath.Join(bpDir, "README.md.tmpl"), []byte(`# ${project_name}
`), 0o600))
}

// TestCreate_SetObjectLiteral covers IMPL-0009 D.4: --set on an
// object-typed variable accepts an HCL object literal and the
// resolved value flows through to the lockfile.
func TestCreate_SetObjectLiteral(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	regDir := filepath.Join(tmp, "reg")
	require.NoError(t, os.MkdirAll(regDir, 0o750))
	buildStructuredRegistry(t, regDir)

	outputDir := filepath.Join(tmp, "out")
	opts := &create.Opts{
		BlueprintRef: "demo/api",
		OutputDir:    outputDir,
		RegistryDir:  regDir,
		UseDefaults:  true,
		NoHooks:      true,
		ForgeVersion: "test-objectset",
		Overrides: map[string]string{
			"project_name": "demo",
			"git_provider": `{repo_type = "github", repo_url = "github.com", project_org = "me"}`,
		},
	}

	_, err := create.Run(opts)
	require.NoError(t, err)

	lock, err := lockfile.LoadLockfile(outputDir)
	require.NoError(t, err)

	got, ok := lock.Variables["git_provider"]
	require.True(t, ok, "git_provider must appear in the lockfile")

	gp, ok := got.(cty.Value)
	require.True(t, ok, "structured variables should round-trip as cty.Value (got %T)", got)
	require.True(t, gp.Type().IsObjectType())
	assert.Equal(t, cty.StringVal("github"), gp.GetAttr("repo_type"))
	assert.Equal(t, cty.StringVal("github.com"), gp.GetAttr("repo_url"))
	assert.Equal(t, cty.StringVal("me"), gp.GetAttr("project_org"))
}

// TestCreate_SetRejectsList covers IMPL-0009 D.5: --set on a
// list-typed variable surfaces the documented error pointing at
// --var-file.
func TestCreate_SetRejectsList(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	regDir := filepath.Join(tmp, "reg")
	require.NoError(t, os.MkdirAll(regDir, 0o750))
	buildStructuredRegistry(t, regDir)

	outputDir := filepath.Join(tmp, "out")
	opts := &create.Opts{
		BlueprintRef: "demo/api",
		OutputDir:    outputDir,
		RegistryDir:  regDir,
		UseDefaults:  true,
		NoHooks:      true,
		ForgeVersion: "test-objectset",
		Overrides: map[string]string{
			"project_name":  "demo",
			"exposed_ports": "[8080, 9090]",
		},
	}

	_, err := create.Run(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exposed_ports")
	assert.Contains(t, err.Error(), "not supported")
	assert.Contains(t, err.Error(), "--var-file",
		"the rejection error must point users at the --var-file escape hatch")
}

// TestCreate_SetRejectsMap mirrors TestCreate_SetRejectsList for
// map-typed variables — same error format, different declared shape.
func TestCreate_SetRejectsMap(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	regDir := filepath.Join(tmp, "reg")
	require.NoError(t, os.MkdirAll(regDir, 0o750))
	buildStructuredRegistry(t, regDir)

	outputDir := filepath.Join(tmp, "out")
	opts := &create.Opts{
		BlueprintRef: "demo/api",
		OutputDir:    outputDir,
		RegistryDir:  regDir,
		UseDefaults:  true,
		NoHooks:      true,
		ForgeVersion: "test-objectset",
		Overrides: map[string]string{
			"project_name":  "demo",
			"build_targets": `{linux = "amd64"}`,
		},
	}

	_, err := create.Run(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build_targets")
	assert.Contains(t, err.Error(), "not supported")
	assert.Contains(t, err.Error(), "--var-file")
}
