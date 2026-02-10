package tools_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/tools"
)

func TestDownload_CacheHit(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	destDir := filepath.Join(t.TempDir(), "dest")

	// Pre-populate cache.
	cachedToolDir := filepath.Join(cacheDir, "mytool", "1.0.0")
	require.NoError(t, os.MkdirAll(cachedToolDir, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(cachedToolDir, "mytool"),
		[]byte("cached-binary"),
		0o755,
	))

	opts := &tools.DownloadOpts{
		Tool: tools.ResolvedTool{
			Name:    "mytool",
			Version: "1.0.0",
			Source:  config.ToolSource{Type: "github-release"},
		},
		Platform: tools.Platform{OS: "linux", Arch: "amd64"},
		DestDir:  destDir,
		CacheDir: cacheDir,
	}

	err := tools.Download(t.Context(), opts)
	require.NoError(t, err)

	// Verify file was copied from cache.
	content, err := os.ReadFile(filepath.Join(destDir, "mytool"))
	require.NoError(t, err)
	assert.Equal(t, "cached-binary", string(content))
}

func TestDownload_UnsupportedType(t *testing.T) {
	t.Parallel()

	opts := &tools.DownloadOpts{
		Tool: tools.ResolvedTool{
			Name:    "tool",
			Version: "1.0.0",
			Source:  config.ToolSource{Type: "unsupported"},
		},
		DestDir: t.TempDir(),
	}

	err := tools.Download(t.Context(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported tool source type")
}

func TestDownload_GoInstallMissingModule(t *testing.T) {
	t.Parallel()

	opts := &tools.DownloadOpts{
		Tool: tools.ResolvedTool{
			Name:    "tool",
			Version: "1.0.0",
			Source:  config.ToolSource{Type: "go-install"},
		},
		DestDir: t.TempDir(),
	}

	err := tools.Download(t.Context(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires module field")
}
