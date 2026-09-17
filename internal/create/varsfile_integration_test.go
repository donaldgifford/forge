package create_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/create"
	"github.com/donaldgifford/forge/internal/lockfile"
)

// writeVarsFile is a tiny helper used by the vars-file integration
// tests. Centralising it avoids repeating the byte-cast + permission
// boilerplate across six tests.
func writeVarsFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// newVarsFileOpts is a minimal Opts shape for vars-file tests: no
// overrides (the vars file supplies all values), --defaults so prompts
// stay disabled, and the standard test registry.
func newVarsFileOpts(t *testing.T, outputDir string, varsFiles ...string) *create.Opts {
	t.Helper()

	return &create.Opts{
		BlueprintRef: "go/api",
		OutputDir:    outputDir,
		RegistryDir:  absTestRegistryDir(t),
		VarsFiles:    varsFiles,
		UseDefaults:  true,
		NoHooks:      true,
		ForceCreate:  false,
		ForgeVersion: "test-vars-file",
	}
}

// TestCreate_VarsFile_BasicScaffold verifies that a single --var-file
// produces the same scaffold a CLI invocation with equivalent --set
// flags would. The lockfile is the cleanest assertion target: it
// preserves the resolved variable map.
func TestCreate_VarsFile_BasicScaffold(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	varsPath := writeVarsFile(t, tmp, "vars.forge-vars.hcl", `
project_name = "my-api"
go_module    = "github.com/example/my-api"
use_grpc     = true
license      = "MIT"
`)

	outputDir := filepath.Join(tmp, "my-api")
	opts := newVarsFileOpts(t, outputDir, varsPath)

	result, err := create.Run(opts)
	require.NoError(t, err)
	assert.Empty(t, result.UnknownVarsFileKeys)

	lock, err := lockfile.LoadLockfile(outputDir)
	require.NoError(t, err)
	assert.Equal(t, "my-api", lock.Variables["project_name"])
	assert.Equal(t, "github.com/example/my-api", lock.Variables["go_module"])
	assert.Equal(t, true, lock.Variables["use_grpc"])
	assert.Equal(t, "MIT", lock.Variables["license"])
}

// TestCreate_VarsFile_LastWins verifies left-to-right override
// semantics across multiple --var-file inputs. The override file
// changes use_grpc; everything else inherits from the base.
func TestCreate_VarsFile_LastWins(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	base := writeVarsFile(t, tmp, "base.forge-vars.hcl", `
project_name = "my-api"
go_module    = "github.com/example/my-api"
use_grpc     = false
license      = "MIT"
`)
	override := writeVarsFile(t, tmp, "override.forge-vars.hcl", `
use_grpc = true
`)

	outputDir := filepath.Join(tmp, "my-api")
	opts := newVarsFileOpts(t, outputDir, base, override)

	_, err := create.Run(opts)
	require.NoError(t, err)

	lock, err := lockfile.LoadLockfile(outputDir)
	require.NoError(t, err)
	assert.Equal(t, true, lock.Variables["use_grpc"])
	assert.Equal(t, "my-api", lock.Variables["project_name"])
}

// TestCreate_VarsFile_UnknownKeys_NonFatal verifies that an unknown
// key surfaces via Result.UnknownVarsFileKeys but does not fail the
// scaffold (IMPL-0008 OQ-7).
func TestCreate_VarsFile_UnknownKeys_NonFatal(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	varsPath := writeVarsFile(t, tmp, "extra.forge-vars.hcl", `
project_name      = "my-api"
go_module         = "github.com/example/my-api"
license           = "MIT"
this_key_does_not_exist = "ignored"
`)

	outputDir := filepath.Join(tmp, "my-api")
	opts := newVarsFileOpts(t, outputDir, varsPath)

	result, err := create.Run(opts)
	require.NoError(t, err)

	assert.Contains(t, result.UnknownVarsFileKeys, "this_key_does_not_exist")

	// The known keys still landed in the lockfile.
	lock, err := lockfile.LoadLockfile(outputDir)
	require.NoError(t, err)
	assert.Equal(t, "my-api", lock.Variables["project_name"])

	// And the unknown key is NOT persisted.
	_, hasUnknown := lock.Variables["this_key_does_not_exist"]
	assert.False(t, hasUnknown)
}

// TestCreate_VarsFile_TypeMismatch_AbortsScaffold verifies that a
// vars-file value that can't coerce to the declared type fails the
// run before any files are written.
func TestCreate_VarsFile_TypeMismatch_AbortsScaffold(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	varsPath := writeVarsFile(t, tmp, "bad.forge-vars.hcl", `
project_name = "my-api"
go_module    = "github.com/example/my-api"
use_grpc     = "not-a-bool"
license      = "MIT"
`)

	outputDir := filepath.Join(tmp, "my-api")
	opts := newVarsFileOpts(t, outputDir, varsPath)

	_, err := create.Run(opts)
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "bad.forge-vars.hcl")
	assert.Contains(t, msg, "use_grpc")

	// Scaffold should not have happened.
	_, statErr := os.Stat(outputDir)
	assert.True(t, os.IsNotExist(statErr), "output directory should not be created on type-coercion failure")
}

// TestCreate_VarsFile_PartialFile_FallsThroughToDefaults verifies that
// a vars file that omits some variables doesn't force those into the
// "missing required" error path when defaults exist. The blueprint's
// default for go_module references project_name, so the rendered
// default must still work.
func TestCreate_VarsFile_PartialFile_FallsThroughToDefaults(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	varsPath := writeVarsFile(t, tmp, "partial.forge-vars.hcl", `
project_name = "my-api"
license      = "MIT"
`)

	outputDir := filepath.Join(tmp, "my-api")
	opts := newVarsFileOpts(t, outputDir, varsPath)

	_, err := create.Run(opts)
	require.NoError(t, err)

	lock, err := lockfile.LoadLockfile(outputDir)
	require.NoError(t, err)
	// project_name from vars file; go_module from default template
	// `github.com/example/${project_name}`.
	assert.Equal(t, "my-api", lock.Variables["project_name"])
	assert.Equal(t, "github.com/example/my-api", lock.Variables["go_module"])
}

// TestCreate_VarsFile_BadExtension_Rejected verifies the OQ-8 rule:
// only `.hcl` extensions are accepted on --var-file paths.
func TestCreate_VarsFile_BadExtension_Rejected(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	varsPath := writeVarsFile(t, tmp, "vars.txt", `project_name = "my-api"`)

	outputDir := filepath.Join(tmp, "my-api")
	opts := newVarsFileOpts(t, outputDir, varsPath)

	_, err := create.Run(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".hcl")
}
