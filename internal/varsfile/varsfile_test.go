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
// bool with default false, port is number with default 8080.
func declaredVars() []config.Variable {
	return []config.Variable{
		{Name: "project_name", Type: cty.String, Required: true},
		{Name: "use_grpc", Type: cty.Bool, DefaultSource: "false"},
		{Name: "port", Type: cty.Number, DefaultSource: "8080"},
	}
}

// structuredVars are the IMPL-0009 Phase D fixtures: object (flat +
// nested), list(number), map(string). Used by the object-types
// fixture suite.
func structuredVars() []config.Variable {
	return []config.Variable{
		{
			Name: "git_provider",
			Type: cty.Object(map[string]cty.Type{
				"repo_type":   cty.String,
				"repo_url":    cty.String,
				"project_org": cty.String,
			}),
		},
		{
			Name: "service",
			Type: cty.Object(map[string]cty.Type{
				"name": cty.String,
				"addr": cty.Object(map[string]cty.Type{
					"host": cty.String,
					"port": cty.Number,
				}),
			}),
		},
		{
			Name: "exposed_ports",
			Type: cty.List(cty.Number),
		},
		{
			Name: "build_targets",
			Type: cty.Map(cty.String),
		},
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

// --- IMPL-0009 Phase D: structured-type fixtures ---

// TestLoad_StructuredType_ObjectFlat covers the canonical
// object-replacement pattern from DESIGN-0006 — the renovate-config
// use case that motivates the whole feature.
func TestLoad_StructuredType_ObjectFlat(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "object-types", "object-flat.forge-vars.hcl")

	resolved, unknown, err := varsfile.Load([]string{path}, structuredVars())
	require.NoError(t, err)
	assert.Empty(t, unknown)

	got, ok := resolved["git_provider"]
	require.True(t, ok, "git_provider must be in the resolved map")
	require.True(t, got.Type().IsObjectType())
	assert.Equal(t, cty.StringVal("github"), got.GetAttr("repo_type"))
	assert.Equal(t, cty.StringVal("github.com"), got.GetAttr("repo_url"))
	assert.Equal(t, cty.StringVal("donaldgifford"), got.GetAttr("project_org"))
}

// TestLoad_StructuredType_ObjectNested covers a two-level object
// (service.addr.host/port). Important because the loader must
// recursively type-coerce.
func TestLoad_StructuredType_ObjectNested(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "object-types", "object-nested.forge-vars.hcl")

	resolved, _, err := varsfile.Load([]string{path}, structuredVars())
	require.NoError(t, err)

	svc, ok := resolved["service"]
	require.True(t, ok)
	assert.Equal(t, cty.StringVal("api"), svc.GetAttr("name"))

	addr := svc.GetAttr("addr")
	assert.Equal(t, cty.StringVal("0.0.0.0"), addr.GetAttr("host"))

	port, _ := addr.GetAttr("port").AsBigFloat().Int64()
	assert.Equal(t, int64(8080), port)
}

// TestLoad_StructuredType_ListOfNumbers covers the list(number) shape
// — the exposed_ports use case from the DESIGN-0006 examples.
func TestLoad_StructuredType_ListOfNumbers(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "object-types", "list-of-numbers.forge-vars.hcl")

	resolved, _, err := varsfile.Load([]string{path}, structuredVars())
	require.NoError(t, err)

	got, ok := resolved["exposed_ports"]
	require.True(t, ok)
	require.True(t, got.Type().IsListType())

	var ports []int64
	for it := got.ElementIterator(); it.Next(); {
		_, v := it.Element()
		i, _ := v.AsBigFloat().Int64()
		ports = append(ports, i)
	}

	assert.Equal(t, []int64{8080, 9090, 9091}, ports)
}

// TestLoad_StructuredType_MapOfStrings covers the map(string) shape.
func TestLoad_StructuredType_MapOfStrings(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "object-types", "map-of-strings.forge-vars.hcl")

	resolved, _, err := varsfile.Load([]string{path}, structuredVars())
	require.NoError(t, err)

	got, ok := resolved["build_targets"]
	require.True(t, ok)
	require.True(t, got.Type().IsMapType())
	assert.Equal(t, cty.StringVal("amd64"), got.Index(cty.StringVal("linux")))
	assert.Equal(t, cty.StringVal("arm64"), got.Index(cty.StringVal("darwin")))
}

// TestLoad_StructuredType_ObjectShapeMismatchErrors verifies a
// structured-value mismatch (declared string field supplied as
// number) aborts with a vars-file-anchored error rather than silently
// coercing or panicking.
func TestLoad_StructuredType_ObjectShapeMismatchErrors(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "object-types", "object-mismatch.forge-vars.hcl")

	_, _, err := varsfile.Load([]string{path}, structuredVars())
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "object-mismatch.forge-vars.hcl",
		"error must point at the source file")
	assert.Contains(t, msg, "git_provider",
		"error must name the offending variable")
}

// TestLoad_StructuredType_ListElementMismatchErrors covers the
// list(T) element-type mismatch case: list(number) declared, mixed
// types supplied. cty.Convert refuses the coercion.
func TestLoad_StructuredType_ListElementMismatchErrors(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "object-types", "list-mismatch.forge-vars.hcl")

	_, _, err := varsfile.Load([]string{path}, structuredVars())
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "list-mismatch.forge-vars.hcl")
	assert.Contains(t, msg, "exposed_ports")
}
