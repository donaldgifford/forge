package lockfile_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/lockfile"
)

// TestHCLRoundTrip_StructToFileAndBack verifies that
// WriteLockfileHCL → LoadLockfileHCL preserves every field of an
// in-memory Lockfile. Pins format equivalence for the loader/emitter
// pair.
func TestHCLRoundTrip_StructToFileAndBack(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Nanosecond)

	original := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			RegistryURL: "github.com/acme/blueprints",
			Name:        "go-api",
			Path:        "go/api",
			Ref:         "v1.0.0",
			Commit:      "abc123",
		},
		CreatedAt:    now,
		LastSynced:   now,
		ForgeVersion: "0.5.0",
		Variables: map[string]any{
			"project_name": "my-api",
			"go_module":    "github.com/example/my-api",
			"use_grpc":     false,
			"port":         int64(8080),
		},
		Defaults: []lockfile.DefaultEntry{
			{Path: ".editorconfig", Source: "registry-default", Strategy: "overwrite", Hash: "sha256:abc"},
			{Path: ".golangci.yml", Source: "category-default", Strategy: "overwrite"},
		},
		ManagedFiles: []lockfile.ManagedFileEntry{
			{Path: "Makefile", Strategy: "merge", SyncedCommit: "abc123"},
			{Path: "Dockerfile", Strategy: "overwrite", Hash: "sha256:def"},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, lockfile.WriteLockfileHCL(&buf, original))

	dir := t.TempDir()
	path := filepath.Join(dir, lockfile.HCLFileName)
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))

	loaded, err := lockfile.LoadLockfileHCL(path)
	require.NoError(t, err)

	assert.Equal(t, original.Blueprint, loaded.Blueprint)
	assert.Equal(t, original.ForgeVersion, loaded.ForgeVersion)
	assert.True(t, original.CreatedAt.Equal(loaded.CreatedAt), "CreatedAt mismatch: %v vs %v", original.CreatedAt, loaded.CreatedAt)
	assert.True(t, original.LastSynced.Equal(loaded.LastSynced), "LastSynced mismatch: %v vs %v", original.LastSynced, loaded.LastSynced)
	assert.Equal(t, original.Variables, loaded.Variables)
	assert.Equal(t, original.Defaults, loaded.Defaults)
	assert.Equal(t, original.ManagedFiles, loaded.ManagedFiles)
}

// TestHCLRoundTrip_StructuredVariables covers IMPL-0009 Phase F.2/F.3:
// a lockfile carrying object, list, map, and nested-object variables
// must survive WriteLockfileHCL → on-disk → LoadLockfileHCL → ToCtyValues
// with semantically equivalent cty values per declared variable type.
//
// We compare via cty.RawEquals rather than reflect.DeepEqual because
// cty number values carry a big.Float whose precision differs between
// directly-constructed `NumberIntVal(...)` and string→number coercions
// performed by cty.Convert.
func TestHCLRoundTrip_StructuredVariables(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Nanosecond)

	gitProviderType := cty.Object(map[string]cty.Type{
		"repo_type": cty.String,
		"repo_url":  cty.String,
		"meta": cty.Object(map[string]cty.Type{
			"team": cty.String,
		}),
	})

	gitProvider := cty.ObjectVal(map[string]cty.Value{
		"repo_type": cty.StringVal("github"),
		"repo_url":  cty.StringVal("github.com/acme/app"),
		"meta": cty.ObjectVal(map[string]cty.Value{
			"team": cty.StringVal("platform"),
		}),
	})

	exposedPorts := cty.ListVal([]cty.Value{
		cty.NumberIntVal(8080),
		cty.NumberIntVal(9090),
	})

	buildTargets := cty.MapVal(map[string]cty.Value{
		"linux":  cty.StringVal("amd64"),
		"darwin": cty.StringVal("arm64"),
	})

	original := &lockfile.Lockfile{
		Blueprint: lockfile.BlueprintRef{
			RegistryURL: "github.com/acme/blueprints",
			Name:        "go-api",
			Path:        "go/api",
			Ref:         "v1.0.0",
			Commit:      "abc123",
		},
		CreatedAt:    now,
		LastSynced:   now,
		ForgeVersion: "0.7.0",
		Variables: map[string]any{
			"project_name":  "my-api",
			"git_provider":  gitProvider,
			"exposed_ports": exposedPorts,
			"build_targets": buildTargets,
		},
	}

	var buf bytes.Buffer
	require.NoError(t, lockfile.WriteLockfileHCL(&buf, original))

	dir := t.TempDir()
	path := filepath.Join(dir, lockfile.HCLFileName)
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))

	loaded, err := lockfile.LoadLockfileHCL(path)
	require.NoError(t, err)

	// Coerce back via the declared variable types — same path
	// create.Run + sync.Run take post-load.
	declared := []config.Variable{
		{Name: "project_name", Type: cty.String},
		{Name: "git_provider", Type: gitProviderType},
		{Name: "exposed_ports", Type: cty.List(cty.Number)},
		{Name: "build_targets", Type: cty.Map(cty.String)},
	}

	resolved, err := lockfile.ToCtyValues(loaded.Variables, declared)
	require.NoError(t, err)

	assert.True(t,
		resolved["project_name"].RawEquals(cty.StringVal("my-api")),
		"project_name mismatch: %#v", resolved["project_name"])

	assert.True(t,
		resolved["git_provider"].Equals(gitProvider).True(),
		"git_provider object did not round-trip: %#v", resolved["git_provider"])

	gpMeta := resolved["git_provider"].GetAttr("meta")
	assert.Equal(t, "platform", gpMeta.GetAttr("team").AsString(),
		"nested object attribute did not round-trip")

	assert.True(t,
		resolved["exposed_ports"].Equals(exposedPorts).True(),
		"exposed_ports list did not round-trip: %#v", resolved["exposed_ports"])
	assert.True(t,
		resolved["exposed_ports"].Type().Equals(cty.List(cty.Number)),
		"declared list type lost: %s", resolved["exposed_ports"].Type().FriendlyName())

	assert.True(t,
		resolved["build_targets"].Equals(buildTargets).True(),
		"build_targets map did not round-trip: %#v", resolved["build_targets"])
	assert.True(t,
		resolved["build_targets"].Type().Equals(cty.Map(cty.String)),
		"declared map type lost: %s", resolved["build_targets"].Type().FriendlyName())
}
