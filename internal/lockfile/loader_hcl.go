package lockfile

// HCL loader for .forge-lock.hcl.
//
// The loader follows the same hybrid pattern as
// internal/config/loader_hcl.go: hcldec.Decode covers the eagerly-
// evaluable attributes and blocks, while the dynamic `variables`
// block is hand-decoded via JustAttributes() because its attribute
// set varies per blueprint.

import (
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// LoadLockfileHCL reads and parses a .forge-lock.hcl file, returning
// the same Lockfile shape Read produces from YAML.
func LoadLockfileHCL(path string) (*Lockfile, error) {
	src, err := os.ReadFile(path) //nolint:gosec // lockfile path comes from known project directories, matching the YAML loader's existing posture
	if err != nil {
		return nil, fmt.Errorf("reading lockfile %s: %w", path, err)
	}

	parser := hclparse.NewParser()

	file, diags := parser.ParseHCL(src, path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing lockfile %s: %s", path, diags.Error())
	}

	lock, err := decodeLockfileBody(file.Body)
	if err != nil {
		return nil, fmt.Errorf("decoding lockfile %s: %w", path, err)
	}

	return lock, nil
}

// decodeLockfileBody splits the body into the dynamic `variables`
// block (hand-decoded) and everything else (decoded via hcldec).
func decodeLockfileBody(body hcl.Body) (*Lockfile, error) {
	lazyContent, eagerBody, diags := body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "variables"},
		},
	})
	if diags.HasErrors() {
		return nil, fmt.Errorf("splitting lockfile body: %s", diags.Error())
	}

	eagerVal, diags := hcldec.Decode(eagerBody, lockfileEagerSpec, nil)
	if diags.HasErrors() {
		return nil, fmt.Errorf("decoding lockfile attributes: %s", diags.Error())
	}

	lock := &Lockfile{}
	if err := assignEager(eagerVal, lock); err != nil {
		return nil, err
	}

	vars, err := decodeVariablesBlocks(lazyContent.Blocks.OfType("variables"))
	if err != nil {
		return nil, err
	}

	lock.Variables = vars

	return lock, nil
}

// assignEager copies the eager cty.Object into the Lockfile struct.
// Timestamps are parsed from RFC3339 strings; blueprint and entry
// blocks are flattened into their struct counterparts.
func assignEager(val cty.Value, lock *Lockfile) error {
	createdAt, err := parseTimeAttr(val, "created_at")
	if err != nil {
		return err
	}

	syncedAt, err := parseTimeAttr(val, "last_synced")
	if err != nil {
		return err
	}

	lock.CreatedAt = createdAt
	lock.LastSynced = syncedAt
	lock.ForgeVersion = ctyToString(val.GetAttr("forge_version"))

	if bpVal := val.GetAttr("blueprint"); !bpVal.IsNull() {
		lock.Blueprint = BlueprintRef{
			RegistryURL: ctyToString(bpVal.GetAttr("registry_url")),
			Name:        ctyToString(bpVal.GetAttr("name")),
			Path:        ctyToString(bpVal.GetAttr("path")),
			Ref:         ctyToString(bpVal.GetAttr("ref")),
			Commit:      ctyToString(bpVal.GetAttr("commit")),
		}
	}

	lock.Defaults = assignDefaults(val.GetAttr("defaults"))
	lock.ManagedFiles = assignManagedFiles(val.GetAttr("managed_files"))

	return nil
}

// decodeVariablesBlocks walks each `variables { ... }` block (only
// one is expected; multiples are rejected) and converts its
// attribute set into map[string]any using the same shape the YAML
// loader produces. An empty map is returned when the block is absent
// so callers can range over the result without nil-checking.
func decodeVariablesBlocks(blocks []*hcl.Block) (map[string]any, error) {
	if len(blocks) == 0 {
		return map[string]any{}, nil
	}

	if len(blocks) > 1 {
		return nil, fmt.Errorf(
			"lockfile contains %d `variables` blocks; expected at most one",
			len(blocks),
		)
	}

	attrs, diags := blocks[0].Body.JustAttributes()
	if diags.HasErrors() {
		return nil, fmt.Errorf("decoding variables block: %s", diags.Error())
	}

	out := make(map[string]any, len(attrs))

	for name, attr := range attrs {
		val, diags := attr.Expr.Value(nil)
		if diags.HasErrors() {
			return nil, fmt.Errorf("variable %q: %s", name, diags.Error())
		}

		out[name] = fromCty(val)
	}

	return out, nil
}

func assignDefaults(val cty.Value) []DefaultEntry {
	if val.IsNull() || !val.CanIterateElements() {
		return nil
	}

	entries := make([]DefaultEntry, 0, val.LengthInt())

	it := val.ElementIterator()
	for it.Next() {
		_, entry := it.Element()
		entries = append(entries, DefaultEntry{
			Path:         ctyToString(entry.GetAttr("path")),
			Source:       ctyToString(entry.GetAttr("source")),
			Strategy:     ctyToString(entry.GetAttr("strategy")),
			Hash:         ctyToString(entry.GetAttr("hash")),
			SyncedCommit: ctyToString(entry.GetAttr("synced_commit")),
		})
	}

	return entries
}

func assignManagedFiles(val cty.Value) []ManagedFileEntry {
	if val.IsNull() || !val.CanIterateElements() {
		return nil
	}

	entries := make([]ManagedFileEntry, 0, val.LengthInt())

	it := val.ElementIterator()
	for it.Next() {
		_, entry := it.Element()
		entries = append(entries, ManagedFileEntry{
			Path:         ctyToString(entry.GetAttr("path")),
			Strategy:     ctyToString(entry.GetAttr("strategy")),
			Hash:         ctyToString(entry.GetAttr("hash")),
			SyncedCommit: ctyToString(entry.GetAttr("synced_commit")),
		})
	}

	return entries
}

func parseTimeAttr(val cty.Value, name string) (time.Time, error) {
	s := ctyToString(val.GetAttr(name))
	if s == "" {
		return time.Time{}, nil
	}

	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		// Fall back to RFC3339 (no fractional seconds) for forward-
		// compat with hand-written or older-format timestamps.
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s: invalid timestamp %q: %w", name, s, err)
		}
	}

	return t, nil
}

func ctyToString(val cty.Value) string {
	if val.IsNull() || !val.IsKnown() {
		return ""
	}

	if val.Type() != cty.String {
		return ""
	}

	return val.AsString()
}
