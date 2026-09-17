package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRequireSingleVarSource_MutualExclusion covers the IMPL-0008
// OQ-2 contract: --set and --var-file cannot both be set on the
// same invocation. The check is intentionally a manual one (not
// Cobra's MarkFlagsMutuallyExclusive) because DESIGN-0005's
// error message is more actionable than Cobra's generic phrasing.
func TestRequireSingleVarSource_MutualExclusion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setFlags  []string
		varFiles  []string
		wantError bool
	}{
		{
			name:      "both empty",
			setFlags:  nil,
			varFiles:  nil,
			wantError: false,
		},
		{
			name:      "only --set",
			setFlags:  []string{"k=v"},
			varFiles:  nil,
			wantError: false,
		},
		{
			name:      "only --var-file",
			setFlags:  nil,
			varFiles:  []string{"a.forge-vars.hcl"},
			wantError: false,
		},
		{
			name:      "both set",
			setFlags:  []string{"k=v"},
			varFiles:  []string{"a.forge-vars.hcl"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := requireSingleVarSource(tt.setFlags, tt.varFiles)

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "--var-file and --set cannot be combined")
				assert.Contains(t, err.Error(), "one input source per invocation")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
