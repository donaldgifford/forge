package varsfile_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/varsfile"
)

// declaredVars is the standard variable declaration set used across
// table-driven tests. project_name is required string, use_grpc is
// bool with default false, port is int with default 8080.
func declaredVars() []config.Variable {
	return []config.Variable{
		{Name: "project_name", Type: "string", Required: true},
		{Name: "use_grpc", Type: "bool", Default: "false"},
		{Name: "port", Type: "int", Default: "8080"},
	}
}

func TestLoad_HappyPath_ScalarTypes(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "basic.forge-vars.hcl")

	resolved, unknown, err := varsfile.Load([]string{path}, declaredVars())
	require.NoError(t, err)
	assert.Empty(t, unknown)

	assert.Equal(t, cty.StringVal("my-api"), resolved["project_name"])
	assert.Equal(t, cty.True, resolved["use_grpc"])

	port, _ := resolved["port"].AsBigFloat().Int64()
	assert.Equal(t, int64(8080), port)
}

func TestLoad_Composition_LastWins(t *testing.T) {
	t.Parallel()

	base := filepath.Join("testdata", "basic.forge-vars.hcl")
	override := filepath.Join("testdata", "override.forge-vars.hcl")

	resolved, unknown, err := varsfile.Load([]string{base, override}, declaredVars())
	require.NoError(t, err)
	assert.Empty(t, unknown)

	// project_name overridden.
	assert.Equal(t, cty.StringVal("my-api-staging"), resolved["project_name"])

	// port overridden.
	port, _ := resolved["port"].AsBigFloat().Int64()
	assert.Equal(t, int64(9090), port)

	// use_grpc preserved from base.
	assert.Equal(t, cty.True, resolved["use_grpc"])
}

func TestLoad_Composition_ThreeFiles(t *testing.T) {
	t.Parallel()

	// Synthesize a third file that overrides project_name again.
	third := filepath.Join(t.TempDir(), "third.forge-vars.hcl")
	require.NoError(t, writeFile(third, `project_name = "from-third"`+"\n"))

	resolved, _, err := varsfile.Load([]string{
		filepath.Join("testdata", "basic.forge-vars.hcl"),
		filepath.Join("testdata", "override.forge-vars.hcl"),
		third,
	}, declaredVars())
	require.NoError(t, err)

	assert.Equal(t, cty.StringVal("from-third"), resolved["project_name"])
}

func TestLoad_Coercion_StringToInt(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "stringy-int.forge-vars.hcl")
	require.NoError(t, writeFile(path, `port = "42"`+"\n"))

	resolved, _, err := varsfile.Load([]string{path}, declaredVars())
	require.NoError(t, err)

	got, _ := resolved["port"].AsBigFloat().Int64()
	assert.Equal(t, int64(42), got)
}

func TestLoad_Coercion_TypeMismatch_PointsAtSource(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "wrong-type.forge-vars.hcl")

	_, _, err := varsfile.Load([]string{path}, declaredVars())
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "wrong-type.forge-vars.hcl")
	assert.Contains(t, msg, "use_grpc")
	assert.Contains(t, msg, "bool")
}

func TestLoad_UnknownKeys_ReturnedNotErrored(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "unknown-keys.forge-vars.hcl")

	resolved, unknown, err := varsfile.Load([]string{path}, declaredVars())
	require.NoError(t, err)

	sort.Strings(unknown)
	assert.Equal(t, []string{"extra_one", "extra_two"}, unknown)

	// project_name still resolved.
	assert.Equal(t, cty.StringVal("my-api"), resolved["project_name"])

	// Unknown keys are not silently added to the resolved map.
	_, hasExtra := resolved["extra_one"]
	assert.False(t, hasExtra)
}

func TestLoad_Malformed_PointsAtFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "malformed.forge-vars.hcl")

	_, _, err := varsfile.Load([]string{path}, declaredVars())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed.forge-vars.hcl")
}

func TestLoad_Blocks_Rejected(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "with-blocks.forge-vars.hcl")

	_, _, err := varsfile.Load([]string{path}, declaredVars())
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "with-blocks.forge-vars.hcl")
	assert.Contains(t, msg, "attribute assignments only, not blocks")
}

func TestLoad_FunctionCall_Rejected(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "with-function-call.forge-vars.hcl")

	_, _, err := varsfile.Load([]string{path}, declaredVars())
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "with-function-call.forge-vars.hcl")
	assert.Contains(t, msg, "literal values")
}

func TestLoad_BadExtension_Rejected(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "bad-extension.vars")

	_, _, err := varsfile.Load([]string{path}, declaredVars())
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "bad-extension.vars")
	assert.Contains(t, msg, ".hcl")
}

func TestLoad_MissingFile_WrapsError(t *testing.T) {
	t.Parallel()

	_, _, err := varsfile.Load([]string{"/nonexistent/path.forge-vars.hcl"}, declaredVars())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading vars file")
}

func TestLoad_EmptyFile_NoError(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "empty.forge-vars.hcl")

	resolved, unknown, err := varsfile.Load([]string{path}, declaredVars())
	require.NoError(t, err)
	assert.Empty(t, resolved)
	assert.Empty(t, unknown)
}

func TestLoad_NoPaths_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	resolved, unknown, err := varsfile.Load(nil, declaredVars())
	require.NoError(t, err)
	assert.NotNil(t, resolved)
	assert.Empty(t, resolved)
	assert.Empty(t, unknown)
}

func TestLoad_Coercion_StringToBool(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "stringy-bool.forge-vars.hcl")
	require.NoError(t, writeFile(path, `use_grpc = "true"`+"\n"))

	resolved, _, err := varsfile.Load([]string{path}, declaredVars())
	require.NoError(t, err)
	assert.Equal(t, cty.True, resolved["use_grpc"])
}

func TestLoad_TraversalReference_Rejected(t *testing.T) {
	t.Parallel()

	// `var.foo` is a traversal — vars files must not reach outside
	// themselves (IMPL-0008 OQ-1, strict literals).
	path := filepath.Join(t.TempDir(), "traversal.forge-vars.hcl")
	require.NoError(t, writeFile(path, `project_name = var.foo`+"\n"))

	_, _, err := varsfile.Load([]string{path}, declaredVars())
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "traversal.forge-vars.hcl")
	assert.Contains(t, msg, "literal values")
}

func TestLoad_OverrideSourceLocation_PointsAtWinningFile(t *testing.T) {
	t.Parallel()

	// Two files where the second one supplies a type-mismatched
	// value — the error must point at file two, not file one.
	good := filepath.Join(t.TempDir(), "good.forge-vars.hcl")
	require.NoError(t, writeFile(good, `use_grpc = true`+"\n"))

	bad := filepath.Join(t.TempDir(), "bad.forge-vars.hcl")
	require.NoError(t, writeFile(bad, `use_grpc = "not a bool"`+"\n"))

	_, _, err := varsfile.Load([]string{good, bad}, declaredVars())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad.forge-vars.hcl")
}

// writeFile is a tiny helper so the tests stay readable; using
// os.WriteFile inline would push every test over the package's line
// budget.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
