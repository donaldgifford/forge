package config

// HCL emitters for the in-memory Registry struct.
//
// Used by `forge init`, `forge registry init`, `forge registry
// blueprint`, and `forge registry update` to write registry.hcl files
// from a *Registry — append a new entry, refresh metadata, and so on.
// Registry entries don't carry templated values, so the hclwrite path
// (no special handling for `${...}` sequences) is sufficient here.

import (
	"fmt"
	"io"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// WriteRegistryHCL emits reg as registry.hcl-shaped HCL into w. The
// produced output round-trips cleanly through LoadRegistryHCL.
func WriteRegistryHCL(w io.Writer, reg *Registry) error {
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	body.SetAttributeValue("name", cty.StringVal(reg.Name))

	if reg.Description != "" {
		body.SetAttributeValue("description", cty.StringVal(reg.Description))
	}

	for i := range reg.Maintainers {
		body.AppendNewline()
		writeMaintainerBlock(body, &reg.Maintainers[i])
	}

	if reg.Defaults.SyncStrategy != "" || reg.Defaults.Managed {
		body.AppendNewline()
		writeRegistryDefaultsBlock(body, &reg.Defaults)
	}

	for i := range reg.Blueprints {
		body.AppendNewline()
		writeRegistryBlueprintBlock(body, &reg.Blueprints[i])
	}

	if _, err := f.WriteTo(w); err != nil {
		return fmt.Errorf("writing registry hcl: %w", err)
	}

	return nil
}

func writeMaintainerBlock(body *hclwrite.Body, m *Maintainer) {
	block := body.AppendNewBlock("maintainer", nil).Body()

	if m.Name != "" {
		block.SetAttributeValue("name", cty.StringVal(m.Name))
	}

	if m.Email != "" {
		block.SetAttributeValue("email", cty.StringVal(m.Email))
	}
}

func writeRegistryDefaultsBlock(body *hclwrite.Body, d *RegistryDefaults) {
	block := body.AppendNewBlock("defaults", nil).Body()

	if d.SyncStrategy != "" {
		block.SetAttributeValue("sync_strategy", cty.StringVal(d.SyncStrategy))
	}

	block.SetAttributeValue("managed", cty.BoolVal(d.Managed))
}

func writeRegistryBlueprintBlock(body *hclwrite.Body, e *BlueprintEntry) {
	block := body.AppendNewBlock("blueprint", []string{e.Name}).Body()
	block.SetAttributeValue("path", cty.StringVal(e.Path))

	if e.Description != "" {
		block.SetAttributeValue("description", cty.StringVal(e.Description))
	}

	if e.Version != "" {
		block.SetAttributeValue("version", cty.StringVal(e.Version))
	}

	if len(e.Tags) > 0 {
		tags := make([]cty.Value, len(e.Tags))
		for i, t := range e.Tags {
			tags[i] = cty.StringVal(t)
		}

		block.SetAttributeValue("tags", cty.ListVal(tags))
	}

	if e.LatestCommit != "" {
		block.SetAttributeValue("latest_commit", cty.StringVal(e.LatestCommit))
	}
}
