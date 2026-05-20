package config

import (
	"fmt"
	"os"
	"strings"
)

// pinTag is the most recent v0.4.x release — the last forge version
// that shipped `forge migrate`. Rejection-error messages and
// docs/MIGRATION.md both reference this tag.
const pinTag = "v0.4.1"

// LoadBlueprint reads and parses a blueprint config file. HCL is the
// only supported format post-IMPL-0005:
//
//   - A `.hcl` path loads via the HCL loader.
//   - A `.yaml` path with a sibling `.hcl` transparently upgrades to
//     the HCL sibling (callers that hardcoded the legacy filename keep
//     working without code changes — important for the dispatcher path
//     into existing tests/fixtures during the cutover window).
//   - Any other `.yaml` path returns a rescaffold-or-pin error (the
//     `forge migrate` command was removed in IMPL-0007 per ADR-0002).
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
//
// Per IMPL-0007 OQ-3, the message includes the literal `go install`
// command (pinned to the most recent v0.4.x tag) so a v0.2.x/v0.3.x
// user can fix the failure with a single copy-paste rather than
// reading docs first.
func yamlNoLongerSupportedError(kind, path string) error {
	return fmt.Errorf(
		"%s file %s: YAML config files are no longer supported in this version of forge.\n"+
			"\n"+
			"To upgrade:\n"+
			"  - Rescaffold from the current blueprint, OR\n"+
			"  - Pin forge to %s and run `forge migrate config`:\n"+
			"      go install github.com/donaldgifford/forge@%s\n"+
			"\n"+
			"See docs/MIGRATION.md for the full upgrade guide",
		kind,
		path,
		pinTag,
		pinTag,
	)
}
