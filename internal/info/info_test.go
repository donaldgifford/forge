package info_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/info"
)

func sampleBlueprint() *config.Blueprint {
	return &config.Blueprint{
		APIVersion:  "v1",
		Name:        "go-api",
		Description: "A Go API starter",
		Version:     "1.0.0",
		Tags:        []string{"go", "api"},
		Variables: []config.Variable{
			{Name: "project_name", Type: "string", Required: true},
			{Name: "use_docker", Type: "bool", Default: "true"},
		},
		Tools: []config.Tool{
			{Name: "golangci-lint", Version: "v1.60.0", Source: config.ToolSource{Type: "github-release"}},
		},
		Sync: config.SyncConfig{
			ManagedFiles: []config.ManagedFile{
				{Path: "Makefile", Strategy: "merge"},
			},
		},
	}
}

func TestRun_TextOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	opts := &info.Opts{
		Blueprint:    sampleBlueprint(),
		Writer:       &buf,
		OutputFormat: "text",
	}

	err := info.Run(opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "go-api")
	assert.Contains(t, output, "1.0.0")
	assert.Contains(t, output, "A Go API starter")
	assert.Contains(t, output, "go, api")
	assert.Contains(t, output, "Variables:")
	assert.Contains(t, output, "project_name")
	assert.Contains(t, output, "Tools:")
	assert.Contains(t, output, "golangci-lint")
	assert.Contains(t, output, "Managed Files:")
	assert.Contains(t, output, "Makefile")
}

func TestRun_JSONOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	opts := &info.Opts{
		Blueprint:    sampleBlueprint(),
		Writer:       &buf,
		OutputFormat: "json",
	}

	err := info.Run(opts)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.Equal(t, "go-api", result["name"])
	assert.Equal(t, "1.0.0", result["version"])
}

func TestRun_MinimalBlueprint(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	opts := &info.Opts{
		Blueprint: &config.Blueprint{
			Name: "minimal",
		},
		Writer:       &buf,
		OutputFormat: "text",
	}

	err := info.Run(opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "minimal")
	assert.NotContains(t, output, "Variables:")
	assert.NotContains(t, output, "Tools:")
}
