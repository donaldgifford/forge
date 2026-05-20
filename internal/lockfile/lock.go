// Package lockfile manages .forge-lock.hcl state tracking for
// scaffolded projects. HCL is the only accepted on-disk format from
// v0.5+; the FileName constant for the legacy YAML name is retained
// solely so LoadLockfile can surface the rescaffold/pin error per
// ADR-0002.
package lockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// FileName is the legacy YAML lockfile name. Retained so
	// LoadLockfile can detect bare .forge-lock.yaml files and emit
	// the rescaffold-or-pin error required by ADR-0002.
	FileName = ".forge-lock.yaml"

	// HCLFileName is the canonical lockfile name from v0.5 onwards.
	HCLFileName = ".forge-lock.hcl"
)

// Lockfile tracks the provenance, variables, and sync state of a scaffolded project.
type Lockfile struct {
	Blueprint    BlueprintRef
	CreatedAt    time.Time
	LastSynced   time.Time
	ForgeVersion string
	Variables    map[string]any
	Defaults     []DefaultEntry
	ManagedFiles []ManagedFileEntry
}

// BlueprintRef identifies the source blueprint.
type BlueprintRef struct {
	RegistryURL string
	Name        string
	Path        string
	Ref         string
	Commit      string
}

// DefaultEntry tracks an inherited default file.
type DefaultEntry struct {
	Path         string
	Source       string
	Strategy     string
	Hash         string
	SyncedCommit string
}

// ManagedFileEntry tracks a file managed for ongoing sync.
type ManagedFileEntry struct {
	Path         string
	Strategy     string
	Hash         string
	SyncedCommit string
}

// ContentHash computes the SHA256 hash of content in the format "sha256:<hex>".
func ContentHash(content []byte) string {
	h := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(h[:])
}

// pinTag is the most recent v0.4.x release — the last forge version
// that shipped `forge migrate`. The rejection error and
// docs/MIGRATION.md both reference this tag. Mirrors the const of
// the same name in internal/config/loader.go.
const pinTag = "v0.4.1"

// LoadLockfile reads the lockfile from dir. Only the canonical HCL
// form (.forge-lock.hcl) is supported from v0.5 onwards. A bare
// .forge-lock.yaml triggers the rescaffold-or-pin error per
// ADR-0002 — the in-tool `forge migrate` command was removed in
// IMPL-0007, so the message points at the pinned-v0.4.x rescue
// path (IMPL-0007 OQ-3).
func LoadLockfile(dir string) (*Lockfile, error) {
	hclPath := filepath.Join(dir, HCLFileName)

	if _, err := os.Stat(hclPath); err == nil {
		return LoadLockfileHCL(hclPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat %s: %w", hclPath, err)
	}

	yamlPath := filepath.Join(dir, FileName)
	if _, err := os.Stat(yamlPath); err == nil {
		return nil, fmt.Errorf(
			"lockfile %s: YAML lockfiles are no longer supported in this version of forge.\n"+
				"\n"+
				"To upgrade:\n"+
				"  - Rescaffold this project from the current blueprint, OR\n"+
				"  - Pin forge to %s:\n"+
				"      go install github.com/donaldgifford/forge@%s\n"+
				"\n"+
				"See docs/MIGRATION.md",
			yamlPath,
			pinTag,
			pinTag,
		)
	}

	return nil, fmt.Errorf("no lockfile found in %s (expected %s)", dir, HCLFileName)
}
