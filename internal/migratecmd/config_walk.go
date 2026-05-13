package migratecmd

// File walker for `forge migrate config`. Finds every blueprint.yaml
// and registry.yaml under the given root, calls the YAML→HCL rewriter
// for each, writes the .hcl sibling, and (in non-dry-run mode) deletes
// the source .yaml.
//
// Per OQ-5, the walker refuses to touch a file when both .yaml and
// .hcl already exist in the same directory — manual mid-migration
// state is a footgun and we want the operator to clean up first.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// runMigrateConfig drives the YAML→HCL config rewrite. Called by
// RunMigrateConfig after the dirty-worktree guard.
func runMigrateConfig(opts *MigrateOpts) (*MigrateConfigResult, error) {
	root := opts.Path
	if root == "" {
		root = "."
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving path %q: %w", root, err)
	}

	yamlFiles, err := findYAMLConfigs(abs)
	if err != nil {
		return nil, fmt.Errorf("scanning for YAML config files: %w", err)
	}

	result := &MigrateConfigResult{Files: make([]ConfigFileReport, 0, len(yamlFiles))}

	for _, yamlPath := range yamlFiles {
		report := rewriteConfigFile(yamlPath, opts.DryRun)
		result.Files = append(result.Files, report)
	}

	return result, nil
}

// findYAMLConfigs walks root and returns every path matching
// blueprint.yaml or registry.yaml.
func findYAMLConfigs(root string) ([]string, error) {
	var out []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if name == blueprintFileName || name == registryFileName {
			out = append(out, path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// rewriteConfigFile handles one .yaml file: collision check, rewrite,
// write .hcl, delete .yaml. All errors are captured in the
// ConfigFileReport so the walker continues to the next file.
func rewriteConfigFile(yamlPath string, dryRun bool) ConfigFileReport {
	report := ConfigFileReport{Path: yamlPath}

	hclPath := strings.TrimSuffix(yamlPath, ".yaml") + ".hcl"

	// Collision check (OQ-5): refuse if both files exist.
	if _, err := os.Stat(hclPath); err == nil {
		report.Skipped = true
		report.SkipReason = "sibling .hcl already exists"
		report.Errors = append(report.Errors, fmt.Errorf(
			"refusing to overwrite existing %s — clean up the partial migration before re-running",
			hclPath,
		))

		return report
	} else if !errors.Is(err, fs.ErrNotExist) {
		report.Errors = append(report.Errors, fmt.Errorf("checking sibling %s: %w", hclPath, err))
		return report
	}

	src, err := os.ReadFile(yamlPath) //nolint:gosec // G304: walker rewrites user-supplied YAML configs in place
	if err != nil {
		report.Errors = append(report.Errors, fmt.Errorf("reading %s: %w", yamlPath, err))
		return report
	}

	out, err := rewriteByName(filepath.Base(yamlPath), src)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Errorf("rewriting %s: %w", yamlPath, err))
		return report
	}

	report.Output = hclPath

	if dryRun {
		return report
	}

	if err := os.WriteFile(hclPath, out, 0o644); err != nil { //nolint:gosec // G306: writing the HCL sibling next to the user's YAML
		report.Errors = append(report.Errors, fmt.Errorf("writing %s: %w", hclPath, err))
		return report
	}

	if err := os.Remove(yamlPath); err != nil {
		report.Errors = append(report.Errors, fmt.Errorf("removing %s: %w", yamlPath, err))
		return report
	}

	report.Migrated = true

	return report
}

// rewriteByName dispatches to the right rewriter based on the source
// filename.
func rewriteByName(name string, src []byte) ([]byte, error) {
	switch name {
	case blueprintFileName:
		return RewriteBlueprintYAMLToHCL(src)
	case registryFileName:
		return RewriteRegistryYAMLToHCL(src)
	default:
		return nil, fmt.Errorf("unsupported config file name %q (expected blueprint.yaml or registry.yaml)", name)
	}
}
