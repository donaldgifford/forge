package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadBlueprint reads and parses a blueprint config file. HCL is the
// only supported format post-IMPL-0005:
//
//   - A `.hcl` path loads via the HCL loader.
//   - A `.yaml` path with a sibling `.hcl` transparently upgrades to
//     the HCL sibling (callers that hardcoded the legacy filename keep
//     working without code changes — important for the dispatcher path
//     into existing tests/fixtures during the cutover window).
//   - Any other `.yaml` path returns a migration-pointer error directing
//     the user to `forge migrate config` and docs/MIGRATION.md.
func LoadBlueprint(path string) (*Blueprint, error) {
	if hclPath, ok := preferHCLSibling(path); ok {
		return LoadBlueprintHCL(hclPath)
	}

	if strings.HasSuffix(path, ".yaml") {
		return nil, yamlNoLongerSupportedError("blueprint", path)
	}

	return nil, fmt.Errorf(
		"loading blueprint %s: unrecognized config format (expected .hcl)",
		path,
	)
}

// LoadRegistry reads and parses a registry config file. Same dispatch
// shape as LoadBlueprint: HCL only, with a transparent .yaml→.hcl
// sibling upgrade and a migration-pointer error for .yaml paths with
// no .hcl alongside.
func LoadRegistry(path string) (*Registry, error) {
	if hclPath, ok := preferHCLSibling(path); ok {
		return LoadRegistryHCL(hclPath)
	}

	if strings.HasSuffix(path, ".yaml") {
		return nil, yamlNoLongerSupportedError("registry", path)
	}

	return nil, fmt.Errorf(
		"loading registry %s: unrecognized config format (expected .hcl)",
		path,
	)
}

// preferHCLSibling returns the .hcl path to load instead of the input
// when:
//   - the input path itself is .hcl, or
//   - a `<basename>.hcl` sibling exists next to a .yaml input.
//
// Returns ("", false) when no HCL file is available for the input.
func preferHCLSibling(path string) (string, bool) {
	if strings.HasSuffix(path, ".hcl") {
		return path, true
	}

	if !strings.HasSuffix(path, ".yaml") {
		return "", false
	}

	hclPath := strings.TrimSuffix(path, ".yaml") + ".hcl"
	if _, err := os.Stat(hclPath); err == nil {
		return hclPath, true
	}

	return "", false
}

// yamlNoLongerSupportedError returns the canonical error surfaced when
// a caller asks for a YAML config file with no HCL sibling on disk.
// The kind argument is used in the message ("blueprint" or "registry")
// so the error reads naturally for both code paths.
func yamlNoLongerSupportedError(kind, path string) error {
	return fmt.Errorf(
		"%s file %s: YAML config files are no longer supported. "+
			"Run `forge migrate config --path %s` to convert this "+
			"file to %s. See docs/MIGRATION.md in the forge repository "+
			"for the YAML→HCL migration guide",
		kind,
		path,
		filepath.Dir(path),
		strings.TrimSuffix(filepath.Base(path), ".yaml")+".hcl",
	)
}
