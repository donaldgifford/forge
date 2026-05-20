package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRejectVarFileOnCheck covers the IMPL-0008 OQ-5 contract:
// `forge check` is lockfile-driven by design, so --var-file has no
// meaningful drift-detection semantic. The flag is registered solely
// so the rejection message is actionable (rather than Cobra's generic
// "unknown flag" diagnostic). This test pins the exact wording the
// CLI surfaces.
func TestRejectVarFileOnCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		varFiles  []string
		wantError bool
	}{
		{
			name:      "no var-files passes",
			varFiles:  nil,
			wantError: false,
		},
		{
			name:      "empty slice passes",
			varFiles:  []string{},
			wantError: false,
		},
		{
			name:      "single var-file rejected",
			varFiles:  []string{"vars.forge-vars.hcl"},
			wantError: true,
		},
		{
			name:      "multiple var-files rejected",
			varFiles:  []string{"a.forge-vars.hcl", "b.forge-vars.hcl"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := rejectVarFileOnCheck(tt.varFiles)

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "--var-file is not supported on `forge check`")
				assert.Contains(t, err.Error(), "`forge sync --var-file FILE --force --dry-run`")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
