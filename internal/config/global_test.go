package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/config"
)

func TestLoadGlobalConfig(t *testing.T) {
	t.Parallel()

	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")

	content := `
registries:
  - name: acme
    url: github.com/acme/blueprints
    ref: main
  - name: internal
    url: github.com/corp/templates
    ref: v2.0.0
cache_dir: /tmp/forge-cache
default_registry: acme
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	cfg, err := config.LoadGlobalConfig(cfgPath)
	require.NoError(t, err)

	assert.Len(t, cfg.Registries, 2)
	assert.Equal(t, "acme", cfg.Registries[0].Name)
	assert.Equal(t, "github.com/acme/blueprints", cfg.Registries[0].URL)
	assert.Equal(t, "main", cfg.Registries[0].Ref)
	assert.Equal(t, "/tmp/forge-cache", cfg.CacheDir)
	assert.Equal(t, "acme", cfg.DefaultRegistry)
}

func TestLoadGlobalConfig_NotFound(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadGlobalConfig("/nonexistent/config.yaml")
	require.NoError(t, err)
	assert.Empty(t, cfg.Registries)
	assert.Empty(t, cfg.DefaultRegistry)
}

func TestLoadGlobalConfig_InvalidYAML(t *testing.T) {
	t.Parallel()

	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")

	require.NoError(t, os.WriteFile(cfgPath, []byte("{{invalid"), 0o644))

	_, err := config.LoadGlobalConfig(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config")
}

func TestFindRegistry_ByName(t *testing.T) {
	t.Parallel()

	cfg := &config.GlobalConfig{
		Registries: []config.RegistryConfig{
			{Name: "acme", URL: "github.com/acme/blueprints"},
			{Name: "internal", URL: "github.com/corp/templates"},
		},
		DefaultRegistry: "acme",
	}

	reg, err := cfg.FindRegistry("internal")
	require.NoError(t, err)
	assert.Equal(t, "internal", reg.Name)
	assert.Equal(t, "github.com/corp/templates", reg.URL)
}

func TestFindRegistry_Default(t *testing.T) {
	t.Parallel()

	cfg := &config.GlobalConfig{
		Registries: []config.RegistryConfig{
			{Name: "acme", URL: "github.com/acme/blueprints"},
			{Name: "internal", URL: "github.com/corp/templates"},
		},
		DefaultRegistry: "internal",
	}

	reg, err := cfg.FindRegistry("")
	require.NoError(t, err)
	assert.Equal(t, "internal", reg.Name)
}

func TestFindRegistry_FallbackToFirst(t *testing.T) {
	t.Parallel()

	cfg := &config.GlobalConfig{
		Registries: []config.RegistryConfig{
			{Name: "acme", URL: "github.com/acme/blueprints"},
		},
	}

	reg, err := cfg.FindRegistry("")
	require.NoError(t, err)
	assert.Equal(t, "acme", reg.Name)
}

func TestFindRegistry_NotFound(t *testing.T) {
	t.Parallel()

	cfg := &config.GlobalConfig{
		Registries: []config.RegistryConfig{
			{Name: "acme", URL: "github.com/acme/blueprints"},
		},
	}

	_, err := cfg.FindRegistry("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFindRegistry_NoRegistries(t *testing.T) {
	t.Parallel()

	cfg := &config.GlobalConfig{}

	_, err := cfg.FindRegistry("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no registries configured")
}

func TestDefaultConfigDir(t *testing.T) {
	dir := config.DefaultConfigDir()
	assert.Contains(t, dir, "forge")
}

func TestDefaultConfigDir_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")

	dir := config.DefaultConfigDir()
	assert.Equal(t, "/custom/config/forge", dir)
}
