package tools_test

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/donaldgifford/forge/internal/tools"
)

func TestDetectPlatform(t *testing.T) {
	t.Parallel()

	p := tools.DetectPlatform()
	assert.Equal(t, runtime.GOOS, p.OS)
	assert.Equal(t, runtime.GOARCH, p.Arch)
}

func TestResolveAssetURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pattern  string
		version  string
		platform tools.Platform
		expected string
	}{
		{
			name:     "github release pattern",
			pattern:  "actionlint_{{version}}_{{os}}_{{arch}}.tar.gz",
			version:  "1.7.7",
			platform: tools.Platform{OS: "linux", Arch: "amd64"},
			expected: "actionlint_1.7.7_linux_amd64.tar.gz",
		},
		{
			name:     "darwin arm64",
			pattern:  "tool-{{version}}-{{os}}-{{arch}}",
			version:  "2.0.0",
			platform: tools.Platform{OS: "darwin", Arch: "arm64"},
			expected: "tool-2.0.0-darwin-arm64",
		},
		{
			name:     "capitalized OS",
			pattern:  "tool-{{version}}-{{OS}}-{{ARCH}}",
			version:  "1.0.0",
			platform: tools.Platform{OS: "linux", Arch: "amd64"},
			expected: "tool-1.0.0-Linux-Amd64",
		},
		{
			name:     "goos goarch aliases",
			pattern:  "tool_{{goos}}_{{goarch}}",
			version:  "1.0.0",
			platform: tools.Platform{OS: "darwin", Arch: "arm64"},
			expected: "tool_darwin_arm64",
		},
		{
			name:     "no variables",
			pattern:  "tool.tar.gz",
			version:  "1.0.0",
			platform: tools.Platform{OS: "linux", Arch: "amd64"},
			expected: "tool.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tools.ResolveAssetURL(tt.pattern, tt.version, tt.platform)
			assert.Equal(t, tt.expected, result)
		})
	}
}
