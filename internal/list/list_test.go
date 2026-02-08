package list_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/list"
)

const testRegistryDir = "../../testdata/registry"

func TestRun_TableOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &list.Opts{
		RegistryDir:  testRegistryDir,
		OutputFormat: "table",
		Writer:       &buf,
	}

	err := list.Run(opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "VERSION")
	assert.Contains(t, output, "go/api")
	assert.Contains(t, output, "go/cli")
}

func TestRun_JSONOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &list.Opts{
		RegistryDir:  testRegistryDir,
		OutputFormat: "json",
		Writer:       &buf,
	}

	err := list.Run(opts)
	require.NoError(t, err)

	var infos []list.BlueprintInfo
	err = json.Unmarshal(buf.Bytes(), &infos)
	require.NoError(t, err)

	assert.Len(t, infos, 2)
	assert.Equal(t, "go/api", infos[0].Name)
	assert.Equal(t, "go/cli", infos[1].Name)
}

func TestRun_FilterByTag(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &list.Opts{
		RegistryDir:  testRegistryDir,
		OutputFormat: "table",
		TagFilter:    "cli",
		Writer:       &buf,
	}

	err := list.Run(opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "go/cli")
	assert.NotContains(t, output, "go/api")
}

func TestRun_FilterByTag_NoMatch(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &list.Opts{
		RegistryDir:  testRegistryDir,
		OutputFormat: "table",
		TagFilter:    "python",
		Writer:       &buf,
	}

	err := list.Run(opts)
	require.NoError(t, err)

	output := buf.String()
	// Header only, no blueprints.
	assert.Contains(t, output, "NAME")
	assert.NotContains(t, output, "go/api")
	assert.NotContains(t, output, "go/cli")
}

func TestRun_FilterByTag_CaseInsensitive(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &list.Opts{
		RegistryDir:  testRegistryDir,
		OutputFormat: "table",
		TagFilter:    "GRPC",
		Writer:       &buf,
	}

	err := list.Run(opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "go/api")
}

func TestRun_InvalidRegistry(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &list.Opts{
		RegistryDir:  "/nonexistent/path",
		OutputFormat: "table",
		Writer:       &buf,
	}

	err := list.Run(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading registry")
}
