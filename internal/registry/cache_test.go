package registry_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/registry"
)

func TestCache_GetOrFetch_ColdCache(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	cache := registry.NewCache(cacheDir, nil)

	fetchCalled := false
	fetchFn := func(dest string) error {
		fetchCalled = true
		// Simulate fetching by writing a file.
		return os.WriteFile(filepath.Join(dest, "registry.yaml"), []byte("test"), 0o644)
	}

	path, err := cache.GetOrFetch("github.com/acme/blueprints", "v1.0.0", fetchFn)
	require.NoError(t, err)

	assert.True(t, fetchCalled)
	assert.DirExists(t, path)
	assert.FileExists(t, filepath.Join(path, "registry.yaml"))
}

func TestCache_GetOrFetch_WarmCache(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	cache := registry.NewCache(cacheDir, nil)

	fetchCount := 0
	fetchFn := func(dest string) error {
		fetchCount++
		return os.WriteFile(filepath.Join(dest, "registry.yaml"), []byte("test"), 0o644)
	}

	// First call: cold cache.
	_, err := cache.GetOrFetch("github.com/acme/blueprints", "v1.0.0", fetchFn)
	require.NoError(t, err)
	assert.Equal(t, 1, fetchCount)

	// Second call: warm cache, same ref.
	path, err := cache.GetOrFetch("github.com/acme/blueprints", "v1.0.0", fetchFn)
	require.NoError(t, err)
	assert.Equal(t, 1, fetchCount, "should not fetch again for same ref")
	assert.DirExists(t, path)
}

func TestCache_GetOrFetch_StaleRef(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	cache := registry.NewCache(cacheDir, nil)

	fetchCount := 0
	fetchFn := func(dest string) error {
		fetchCount++
		return os.WriteFile(filepath.Join(dest, "registry.yaml"), []byte("test"), 0o644)
	}

	// First call with v1.0.0.
	_, err := cache.GetOrFetch("github.com/acme/blueprints", "v1.0.0", fetchFn)
	require.NoError(t, err)
	assert.Equal(t, 1, fetchCount)

	// Second call with v2.0.0 — should re-fetch.
	_, err = cache.GetOrFetch("github.com/acme/blueprints", "v2.0.0", fetchFn)
	require.NoError(t, err)
	assert.Equal(t, 2, fetchCount, "should re-fetch for different ref")
}

func TestCache_Invalidate(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	cache := registry.NewCache(cacheDir, nil)

	fetchFn := func(dest string) error {
		return os.WriteFile(filepath.Join(dest, "registry.yaml"), []byte("test"), 0o644)
	}

	path, err := cache.GetOrFetch("github.com/acme/blueprints", "v1.0.0", fetchFn)
	require.NoError(t, err)
	assert.DirExists(t, path)

	err = cache.Invalidate("github.com/acme/blueprints")
	require.NoError(t, err)
	assert.NoDirExists(t, path)
}

func TestCache_Invalidate_NonExistent(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	cache := registry.NewCache(cacheDir, nil)

	// Should not error even if nothing is cached.
	err := cache.Invalidate("github.com/nonexistent/repo")
	require.NoError(t, err)
}

func TestDefaultCacheDir(t *testing.T) {
	dir := registry.DefaultCacheDir()
	assert.Contains(t, dir, "forge")
}

func TestDefaultCacheDir_XDG(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/custom/cache")

	dir := registry.DefaultCacheDir()
	assert.Equal(t, "/custom/cache/forge", dir)
}
