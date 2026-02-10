package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// GlobalConfig represents the user's forge configuration file.
type GlobalConfig struct {
	Registries      []RegistryConfig `yaml:"registries"`
	CacheDir        string           `yaml:"cache_dir"`
	DefaultRegistry string           `yaml:"default_registry"`
}

// RegistryConfig identifies a named registry source.
type RegistryConfig struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	Ref  string `yaml:"ref"`
}

// DefaultConfigDir returns the default configuration directory, respecting XDG_CONFIG_HOME.
func DefaultConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "forge")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "forge")
	}

	return filepath.Join(home, ".config", "forge")
}

// LoadGlobalConfig reads the global config from the given path.
// If the file doesn't exist, it returns a zero-value config (no error).
func LoadGlobalConfig(path string) (*GlobalConfig, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return &GlobalConfig{}, nil
		}

		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg GlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	return &cfg, nil
}

// FindRegistry looks up a registry by name. If name is empty, it returns the
// default registry. Returns an error if the registry is not found.
func (c *GlobalConfig) FindRegistry(name string) (*RegistryConfig, error) {
	if name == "" {
		name = c.DefaultRegistry
	}

	if name == "" && len(c.Registries) > 0 {
		return &c.Registries[0], nil
	}

	for i := range c.Registries {
		if c.Registries[i].Name == name {
			return &c.Registries[i], nil
		}
	}

	if name == "" {
		return nil, fmt.Errorf("no registries configured")
	}

	return nil, fmt.Errorf("registry %q not found in config", name)
}
