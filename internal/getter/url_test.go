package getter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/donaldgifford/forge/internal/getter"
)

func TestRegistryURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		baseURL  string
		subpath  string
		ref      string
		expected string
	}{
		{
			name:     "with ref",
			baseURL:  "github.com/acme/blueprints",
			subpath:  "go/api",
			ref:      "v2.1.0",
			expected: "github.com/acme/blueprints//go/api?ref=v2.1.0",
		},
		{
			name:     "without ref",
			baseURL:  "github.com/acme/blueprints",
			subpath:  "go/api",
			ref:      "",
			expected: "github.com/acme/blueprints//go/api",
		},
		{
			name:     "nested subpath",
			baseURL:  "github.com/acme/blueprints",
			subpath:  "go/api/v2",
			ref:      "main",
			expected: "github.com/acme/blueprints//go/api/v2?ref=main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := getter.RegistryURL(tt.baseURL, tt.subpath, tt.ref)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToolReleaseURL(t *testing.T) {
	t.Parallel()

	result := getter.ToolReleaseURL("golangci/golangci-lint", "1.62.2", "golangci-lint-1.62.2-linux-amd64.tar.gz")
	expected := "https://github.com/golangci/golangci-lint/releases/download/v1.62.2/golangci-lint-1.62.2-linux-amd64.tar.gz"
	assert.Equal(t, expected, result)
}
