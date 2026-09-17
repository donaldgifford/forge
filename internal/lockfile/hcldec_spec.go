package lockfile

// hcldec schemas for .forge-lock.hcl.
//
// The lockfile HCL grammar mirrors the in-memory Lockfile struct:
//
//   blueprint {
//     registry_url = "..."
//     name         = "..."
//     path         = "..."
//     ref          = "..."    # optional
//     commit       = "..."    # optional
//   }
//
//   created_at    = "2026-05-18T10:15:30Z"
//   last_synced   = "2026-05-18T10:15:30Z"
//   forge_version = "0.5.0"
//
//   variables {
//     project_name = "mockta"
//     use_docker   = true
//   }
//
//   default "path/to/file" {
//     source        = "..."
//     strategy      = "..."
//     hash          = "sha256:..."
//     synced_commit = "..."
//   }
//
//   managed_file "path/to/managed" {
//     strategy      = "..."
//     hash          = "sha256:..."
//     synced_commit = "..."
//   }
//
// The eager spec covers everything *except* the `variables` block,
// which is decoded by hand in the loader because its attribute set is
// dynamic (one attribute per blueprint-declared variable). Mirrors the
// pattern used in internal/config/hcldec_spec.go for variable / condition
// / rename blocks.

import (
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/zclconf/go-cty/cty"
)

// lockfileEagerSpec decodes the parts of .forge-lock.hcl that resolve
// against an empty EvalContext. The `variables` block is excluded
// because its attribute set is dynamic; the loader hand-decodes it via
// JustAttributes() after PartialContent extracts the block.
var lockfileEagerSpec = hcldec.ObjectSpec{
	"created_at":    &hcldec.AttrSpec{Name: "created_at", Type: cty.String, Required: true},
	"last_synced":   &hcldec.AttrSpec{Name: "last_synced", Type: cty.String, Required: true},
	"forge_version": &hcldec.AttrSpec{Name: "forge_version", Type: cty.String, Required: true},
	"blueprint":     &hcldec.BlockSpec{TypeName: "blueprint", Nested: blueprintRefSpec, Required: true},
	"defaults":      &hcldec.BlockListSpec{TypeName: "default", Nested: defaultEntrySpec},
	"managed_files": &hcldec.BlockListSpec{TypeName: "managed_file", Nested: managedFileEntrySpec},
}

// blueprintRefSpec covers the body of the singleton `blueprint { ... }`
// block. Mirrors the BlueprintRef struct in lock.go.
var blueprintRefSpec = hcldec.ObjectSpec{
	"registry_url": &hcldec.AttrSpec{Name: "registry_url", Type: cty.String, Required: true},
	"name":         &hcldec.AttrSpec{Name: "name", Type: cty.String, Required: true},
	"path":         &hcldec.AttrSpec{Name: "path", Type: cty.String, Required: true},
	"ref":          &hcldec.AttrSpec{Name: "ref", Type: cty.String},
	"commit":       &hcldec.AttrSpec{Name: "commit", Type: cty.String},
}

// defaultEntrySpec covers the body of each `default "path" { ... }`
// block. The label becomes the DefaultEntry.Path field.
var defaultEntrySpec = hcldec.ObjectSpec{
	"path":          &hcldec.BlockLabelSpec{Index: 0, Name: "path"},
	"source":        &hcldec.AttrSpec{Name: "source", Type: cty.String, Required: true},
	"strategy":      &hcldec.AttrSpec{Name: "strategy", Type: cty.String, Required: true},
	"hash":          &hcldec.AttrSpec{Name: "hash", Type: cty.String},
	"synced_commit": &hcldec.AttrSpec{Name: "synced_commit", Type: cty.String},
}

// managedFileEntrySpec covers the body of each
// `managed_file "path" { ... }` block. The label becomes the
// ManagedFileEntry.Path field.
var managedFileEntrySpec = hcldec.ObjectSpec{
	"path":          &hcldec.BlockLabelSpec{Index: 0, Name: "path"},
	"strategy":      &hcldec.AttrSpec{Name: "strategy", Type: cty.String, Required: true},
	"hash":          &hcldec.AttrSpec{Name: "hash", Type: cty.String},
	"synced_commit": &hcldec.AttrSpec{Name: "synced_commit", Type: cty.String},
}
