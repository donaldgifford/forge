package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	cacheMetaFile = ".forge-cache-meta"
	registriesDir = "registries"
)

// cacheMeta stores metadata about a cached registry.
type cacheMeta struct {
	URL string `yaml:"url"`
	Ref string `yaml:"ref"`
}

// Cache manages locally cached registry content.
type Cache struct {
	baseDir string
	logger  *slog.Logger
}

// NewCache creates a Cache rooted at the given base directory.
// The base directory is typically ~/.cache/forge/ or $XDG_CACHE_HOME/forge/.
func NewCache(baseDir string, logger *slog.Logger) *Cache {
	if logger == nil {
		logger = slog.Default()
	}

	return &Cache{
		baseDir: baseDir,
		logger:  logger,
	}
}

// DefaultCacheDir returns the default cache directory, respecting XDG_CACHE_HOME.
func DefaultCacheDir() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "forge")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".cache", "forge")
	}

	return filepath.Join(home, ".cache", "forge")
}

// GetOrFetch returns the local path to a cached registry. If the cache is stale
// or missing, fetchFn is called to populate it.
//
// The fetchFn receives the destination directory path and should populate it with
// the registry content (e.g., via go-getter).
func (c *Cache) GetOrFetch(url, ref string, fetchFn func(dest string) error) (string, error) {
	cacheDir := c.cacheDir(url)
	metaPath := filepath.Join(cacheDir, cacheMetaFile)

	// Check if cache exists and matches the requested ref.
	if meta, err := readCacheMeta(metaPath); err == nil {
		if meta.Ref == ref {
			c.logger.Debug("cache hit", "url", url, "ref", ref)

			return cacheDir, nil
		}

		c.logger.Debug("cache stale", "url", url, "cached_ref", meta.Ref, "requested_ref", ref)
	}

	// Remove stale cache.
	if err := os.RemoveAll(cacheDir); err != nil {
		return "", fmt.Errorf("removing stale cache %s: %w", cacheDir, err)
	}

	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return "", fmt.Errorf("creating cache directory %s: %w", cacheDir, err)
	}

	// Fetch fresh content.
	c.logger.Debug("fetching registry", "url", url, "ref", ref, "dest", cacheDir)

	if err := fetchFn(cacheDir); err != nil {
		// Clean up on failure.
		if removeErr := os.RemoveAll(cacheDir); removeErr != nil {
			c.logger.Warn("failed to clean up cache on fetch failure", "err", removeErr)
		}

		return "", fmt.Errorf("fetching registry %s: %w", url, err)
	}

	// Write cache metadata.
	meta := &cacheMeta{URL: url, Ref: ref}
	if err := writeCacheMeta(metaPath, meta); err != nil {
		return "", fmt.Errorf("writing cache metadata: %w", err)
	}

	return cacheDir, nil
}

// Invalidate removes the cached content for the given URL.
func (c *Cache) Invalidate(url string) error {
	cacheDir := c.cacheDir(url)

	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("invalidating cache for %s: %w", url, err)
	}

	c.logger.Debug("cache invalidated", "url", url)

	return nil
}

// cacheDir returns the cache directory path for a given URL.
func (c *Cache) cacheDir(url string) string {
	hash := sha256.Sum256([]byte(url))
	hashStr := hex.EncodeToString(hash[:8]) // First 8 bytes = 16 hex chars

	return filepath.Join(c.baseDir, registriesDir, hashStr)
}

func readCacheMeta(path string) (*cacheMeta, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}

	var meta cacheMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func writeCacheMeta(path string, meta *cacheMeta) error {
	data, err := yaml.Marshal(meta)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}
