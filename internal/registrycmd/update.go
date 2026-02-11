package registrycmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/donaldgifford/forge/internal/config"
)

// BlueprintStatus represents the sync state of a blueprint entry in registry.yaml.
type BlueprintStatus string

const (
	// StatusUpToDate means the registry entry matches the blueprint and git state.
	StatusUpToDate BlueprintStatus = "up-to-date"
	// StatusVersionChanged means the blueprint.yaml version differs from registry.yaml.
	StatusVersionChanged BlueprintStatus = "version-changed"
	// StatusFilesChanged means the git commit differs but the version is unchanged.
	StatusFilesChanged BlueprintStatus = "files-changed"
	// StatusBothChanged means both version and git commit differ.
	StatusBothChanged BlueprintStatus = "both-changed"
	// StatusMissing means the blueprint path in registry.yaml does not exist on disk.
	StatusMissing BlueprintStatus = "missing"
)

// UpdateOpts configures the registry update operation.
type UpdateOpts struct {
	// RegistryDir is the registry root directory (must contain registry.yaml).
	RegistryDir string
	// Check enables check-only mode: no files are written, exit 1 if stale.
	Check bool
}

// BlueprintReport holds the status of a single blueprint entry.
type BlueprintReport struct {
	// Path is the blueprint path in the registry (e.g., "go/api").
	Path string
	// Status is the detected sync state.
	Status BlueprintStatus
	// RegistryVersion is the version currently in registry.yaml.
	RegistryVersion string
	// BlueprintVersion is the version currently in blueprint.yaml.
	BlueprintVersion string
	// RegistryCommit is the commit hash currently in registry.yaml.
	RegistryCommit string
	// LatestCommit is the actual latest git commit for the blueprint path.
	LatestCommit string
}

// UpdateResult holds the outcome of a registry update operation.
type UpdateResult struct {
	// Reports contains the status of each blueprint in the registry.
	Reports []BlueprintReport
	// Updated is the number of entries updated (0 in check mode).
	Updated int
	// Stale is the number of entries that are out of date.
	Stale int
}

// RunUpdate walks all blueprints in a registry, detects metadata drift,
// and updates registry.yaml (unless in check mode).
func RunUpdate(opts *UpdateOpts) (*UpdateResult, error) {
	if opts.RegistryDir == "" {
		return nil, fmt.Errorf("registry directory is required")
	}

	registryDir, err := filepath.Abs(opts.RegistryDir)
	if err != nil {
		return nil, fmt.Errorf("resolving registry path %s: %w", opts.RegistryDir, err)
	}

	registryYAML := filepath.Join(registryDir, "registry.yaml")
	if _, err := os.Stat(registryYAML); err != nil {
		return nil, fmt.Errorf("registry.yaml not found at %s; run forge registry init first", registryDir)
	}

	if !isGitRepo(registryDir) {
		return nil, fmt.Errorf("registry update requires a git repository")
	}

	reg, err := config.LoadRegistry(registryYAML)
	if err != nil {
		return nil, fmt.Errorf("loading registry.yaml: %w", err)
	}

	reports := make([]BlueprintReport, 0, len(reg.Blueprints))
	for i := range reg.Blueprints {
		report := detectStatus(registryDir, &reg.Blueprints[i])
		reports = append(reports, report)
	}

	stale := 0
	for i := range reports {
		if reports[i].Status != StatusUpToDate && reports[i].Status != StatusMissing {
			stale++
		}
	}

	result := &UpdateResult{
		Reports: reports,
		Stale:   stale,
	}

	if !opts.Check && stale > 0 {
		result.Updated = updateRegistryEntries(reg, reports)

		if err := writeRegistry(registryDir, reg); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func isGitRepo(dir string) bool {
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-parse", "--git-dir")
	cmd.Stdout = nil
	cmd.Stderr = nil

	return cmd.Run() == nil
}

func latestCommitForPath(registryDir, bpPath string) (string, error) {
	args := []string{
		"-C", registryDir,
		"log", "-1", "--format=%H",
		"--", bpPath + "/",
	}
	//nolint:gosec // args are from validated registry paths
	cmd := exec.CommandContext(context.Background(), "git", args...)

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("registry update requires a git repository: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

func detectStatus(registryDir string, entry *config.BlueprintEntry) BlueprintReport {
	report := BlueprintReport{
		Path:            entry.Path,
		RegistryVersion: entry.Version,
		RegistryCommit:  entry.LatestCommit,
	}

	bpYAMLPath := filepath.Join(registryDir, entry.Path, "blueprint.yaml")
	if _, err := os.Stat(bpYAMLPath); err != nil {
		report.Status = StatusMissing

		return report
	}

	bp, err := config.LoadBlueprint(bpYAMLPath)
	if err != nil {
		report.Status = StatusMissing

		return report
	}

	report.BlueprintVersion = bp.Version

	commit, err := latestCommitForPath(registryDir, entry.Path)
	if err != nil {
		// If git fails for this path, treat as missing.
		report.Status = StatusMissing

		return report
	}

	report.LatestCommit = commit

	versionMatch := entry.Version == bp.Version
	commitMatch := entry.LatestCommit == commit

	switch {
	case versionMatch && commitMatch:
		report.Status = StatusUpToDate
	case !versionMatch && commitMatch:
		report.Status = StatusVersionChanged
	case versionMatch && !commitMatch:
		report.Status = StatusFilesChanged
	default:
		report.Status = StatusBothChanged
	}

	return report
}

func updateRegistryEntries(reg *config.Registry, reports []BlueprintReport) int {
	updated := 0

	for i := range reports {
		r := &reports[i]
		if r.Status == StatusUpToDate || r.Status == StatusMissing {
			continue
		}

		for j := range reg.Blueprints {
			if reg.Blueprints[j].Path == r.Path {
				reg.Blueprints[j].Version = r.BlueprintVersion
				reg.Blueprints[j].LatestCommit = r.LatestCommit
				updated++

				break
			}
		}
	}

	return updated
}

func writeRegistry(registryDir string, reg *config.Registry) error {
	data, err := yaml.Marshal(reg)
	if err != nil {
		return fmt.Errorf("marshaling registry.yaml: %w", err)
	}

	indexPath := filepath.Join(registryDir, "registry.yaml")
	if err := os.WriteFile(indexPath, data, 0o644); err != nil {
		return fmt.Errorf("writing registry.yaml: %w", err)
	}

	return nil
}
