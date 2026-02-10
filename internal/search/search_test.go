package search_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/search"
)

const testRegistryDir = "../../testdata/registry"

func TestRun_SearchByName(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &search.Opts{
		Query:        "api",
		RegistryDir:  testRegistryDir,
		OutputFormat: "table",
		Writer:       &buf,
	}

	err := search.Run(opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "go/api")
	assert.NotContains(t, output, "go/cli")
}

func TestRun_SearchByDescription(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &search.Opts{
		Query:        "cobra",
		RegistryDir:  testRegistryDir,
		OutputFormat: "table",
		Writer:       &buf,
	}

	err := search.Run(opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "go/cli")
	assert.NotContains(t, output, "go/api")
}

func TestRun_SearchByTag(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &search.Opts{
		Query:        "grpc",
		RegistryDir:  testRegistryDir,
		OutputFormat: "table",
		Writer:       &buf,
	}

	err := search.Run(opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "go/api")
}

func TestRun_SearchCaseInsensitive(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &search.Opts{
		Query:        "CLI",
		RegistryDir:  testRegistryDir,
		OutputFormat: "table",
		Writer:       &buf,
	}

	err := search.Run(opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "go/cli")
}

func TestRun_SearchNoMatch(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &search.Opts{
		Query:        "python",
		RegistryDir:  testRegistryDir,
		OutputFormat: "table",
		Writer:       &buf,
	}

	err := search.Run(opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "NAME")
	assert.NotContains(t, output, "go/api")
	assert.NotContains(t, output, "go/cli")
}

func TestRun_SearchJSONOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &search.Opts{
		Query:        "api",
		RegistryDir:  testRegistryDir,
		OutputFormat: "json",
		Writer:       &buf,
	}

	err := search.Run(opts)
	require.NoError(t, err)

	var results []search.Result
	err = json.Unmarshal(buf.Bytes(), &results)
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, "go/api", results[0].Name)
}

func TestRun_EmptyQuery(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := &search.Opts{
		Query:        "",
		RegistryDir:  testRegistryDir,
		OutputFormat: "table",
		Writer:       &buf,
	}

	err := search.Run(opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "go/api")
	assert.Contains(t, output, "go/cli")
}
