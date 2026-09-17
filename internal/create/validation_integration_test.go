package create_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/create"
)

// TestCreate_Validation_RejectsBadLicense exercises the IMPL-0009
// Phase C wiring end-to-end: the test registry's go/api blueprint
// carries a `validation { condition = contains([...], var.license) }`
// block; supplying a license outside that set must abort the create
// flow before any files are written.
func TestCreate_Validation_RejectsBadLicense(t *testing.T) {
	t.Parallel()

	outputDir := filepath.Join(t.TempDir(), "my-api")
	opts := &create.Opts{
		BlueprintRef: "go/api",
		OutputDir:    outputDir,
		RegistryDir:  absTestRegistryDir(t),
		UseDefaults:  true,
		NoHooks:      true,
		ForgeVersion: "test-validation",
		Overrides: map[string]string{
			"project_name": "my-api",
			"go_module":    "github.com/example/my-api",
			"use_grpc":     "false",
			"license":      "Proprietary",
		},
	}

	_, err := create.Run(opts)
	require.Error(t, err, "license outside the contains() set must abort create")
	assert.Contains(t, err.Error(), "license",
		"error must name the failing variable")
	assert.Contains(t, err.Error(), "MIT",
		"error must surface the verbatim error_message from the validation block")
}

// TestCreate_Validation_RejectsBadProjectName covers the
// can(regex(...)) migration pattern: supplying a project_name that
// fails the kebab-case regex aborts cleanly.
func TestCreate_Validation_RejectsBadProjectName(t *testing.T) {
	t.Parallel()

	outputDir := filepath.Join(t.TempDir(), "BadName")
	opts := &create.Opts{
		BlueprintRef: "go/api",
		OutputDir:    outputDir,
		RegistryDir:  absTestRegistryDir(t),
		UseDefaults:  true,
		NoHooks:      true,
		ForgeVersion: "test-validation",
		Overrides: map[string]string{
			"project_name": "INVALID_CAPS",
			"go_module":    "github.com/example/x",
			"use_grpc":     "false",
			"license":      "MIT",
		},
	}

	_, err := create.Run(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project_name")
	assert.Contains(t, err.Error(), "lowercase letters",
		"verbatim error_message from the fixture's validation block must surface")
}
