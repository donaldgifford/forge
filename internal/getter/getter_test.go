package getter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/donaldgifford/forge/internal/getter"
)

func TestAppendQueryParams(t *testing.T) {
	t.Parallel()

	// Test via Fetch with a local file source to verify query param building.
	// Direct unit test of appendQueryParams isn't possible since it's unexported,
	// so we test the URL construction helpers that exercise the same logic.

	tests := []struct {
		name     string
		baseURL  string
		subpath  string
		ref      string
		expected string
	}{
		{
			name:     "ref appended",
			baseURL:  "github.com/acme/reg",
			subpath:  "go/api",
			ref:      "v1.0",
			expected: "github.com/acme/reg//go/api?ref=v1.0",
		},
		{
			name:     "no ref",
			baseURL:  "github.com/acme/reg",
			subpath:  "go/api",
			ref:      "",
			expected: "github.com/acme/reg//go/api",
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

func TestNew(t *testing.T) {
	t.Parallel()

	// Verify New doesn't panic with nil logger.
	g := getter.New(nil)
	assert.NotNil(t, g)
}
