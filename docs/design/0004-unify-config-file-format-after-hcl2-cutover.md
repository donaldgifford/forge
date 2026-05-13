---
id: DESIGN-0004
title: "Unify Config File Format After HCL2 Cutover"
status: Draft
author: Donald Gifford
created: 2026-05-12
---
<!-- markdownlint-disable-file MD025 MD041 -->

# DESIGN 0004: Unify Config File Format After HCL2 Cutover

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-05-12

<!--toc:start-->
- [Overview](#overview)
- [Goals and Non-Goals](#goals-and-non-goals)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Background](#background)
- [Detailed Design](#detailed-design)
- [API / Interface Changes](#api--interface-changes)
- [Data Model](#data-model)
- [Testing Strategy](#testing-strategy)
- [Migration / Rollout Plan](#migration--rollout-plan)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Overview

After the v0.3.0 HCL2 cutover ([ADR-0001](../adr/0001-use-hcl2-as-the-template-engine.md), [DESIGN-0003](0003-migrate-template-engine-to-hcl2.md)), every `.tmpl` file is HCL2 — but `blueprint.yaml` and `registry.yaml` remained YAML with embedded HCL expressions in select string fields. The result is a hybrid surface that asks authors to context-switch syntax every few lines within the same file. **Decision: drop YAML entirely.** Both `blueprint.yaml` and `registry.yaml` become `blueprint.hcl` and `registry.hcl`. The engine, the templates, and the configs all speak HCL2.

## Goals and Non-Goals

### Goals

- Make the lived authoring experience match the engine: when an author opens a blueprint, the syntax in `blueprint.hcl` is the same as the syntax in `${project_name}/main.go.tmpl`.
- Define the migration tool extension and the rejection path for legacy YAML files.
- Land the change as one bundled breaking release (`v0.4.0`) so downstream consumers — including the forge-registry corpus — do a single migration, not two.

### Non-Goals

- Re-litigating the engine choice. HCL2 won in ADR-0001 and the engine is in production.
- Inventing a third syntax (e.g. TOML, JSON, a custom forge DSL).
- Designing the lockfile format. `.forge-lock.yaml` stays YAML for human-grep-ability; that decision was already taken in IMPL-0004 OQ-6 (part c) and is independent of this.
- Maintaining a YAML compatibility shim. The v2 YAML loader gets deleted, not deprecated.

## Background

### What the hybrid looks like today

A current `blueprint.yaml` mixes three syntactic worlds in a single file:

```yaml
apiVersion: v2                          # ← YAML scalar
name: "go-api"
version: "1.0.0"

variables:
  - name: project_name
    type: string
    required: true
    validate: "^[a-z][a-z0-9-]*$"
  - name: go_module
    type: string
    default: "github.com/example/${project_name}"   # ← HCL template inside YAML string

conditions:
  - when: '!use_grpc'                   # ← bare HCL bool expression inside YAML string
    exclude:
      - "proto/"

rename:
  "${project_name}/": "."               # ← HCL expression as a YAML KEY
```

And then the actual content files are pure HCL2:

```
# ${project_name}/main.go.tmpl
package main

func main() {
    fmt.Println("Hello from ${project_name}")
    %{ if use_grpc ~}
    startGRPC()
    %{ endif ~}
}
```

The author flips between YAML mental model (significant indentation, anchors, scalar typing) and HCL mental model (`${expr}` interpolation, `%{ if … ~}` directives, function-call argument order) within the same logical unit (the blueprint).

### What IMPL-0004 already decided (OQ-6)

> *"Config files stay YAML on disk; only template content and in-config expression fields move to HCL. Lockfile YAML scalars in, cty.Value in memory."*

That resolution was reasonable when written — it minimised the migration scope and let us ship a smaller cutover. But it pre-dates the production experience of looking at a v2 `blueprint.yaml`, where the *amount* of embedded HCL surprises authors.

### What's actually HCL inside the YAML today

| Location | Field | Content | Evaluated as |
|----------|-------|---------|--------------|
| `blueprint.yaml` | `variables[].default` | HCL template (`${earlier_var}`) | template, via `prompt.renderDefault` |
| `blueprint.yaml` | `conditions[].when` | bare HCL bool expression | `EvaluateBool` |
| `blueprint.yaml` | `rename` keys | HCL template (path pattern) | `RenderPath` |
| `blueprint.yaml` | `rename` values | HCL template (path pattern) | `RenderString` |
| `blueprint.yaml` | `hooks.post_create[]` | (currently plain strings; could become templated) | shell exec |
| `registry.yaml` | — | none. Pure YAML. | — |

So the pain is concentrated in `blueprint.yaml`. `registry.yaml` is YAML-without-expressions and has no overlap with the engine at all.

## Detailed Design

### Target shape — `blueprint.hcl` + `registry.hcl`

Both config files move to native HCL2. The same engine parses configs and template content; there is no YAML left in the registry tree.

**`blueprint.hcl`**

```hcl
name        = "go-api"
description = "Go API service with standard tooling"
version     = "1.0.0"
tags        = ["go", "api", "grpc"]

variable "project_name" {
  type     = string
  required = true
  validate = "^[a-z][a-z0-9-]*$"
}

variable "go_module" {
  type    = string
  default = "github.com/example/${project_name}"
}

variable "use_grpc" {
  type    = bool
  default = false
}

condition {
  when    = !use_grpc
  exclude = ["proto/"]
}

defaults {
  exclude = [".github/CODEOWNERS"]
}

rename {
  entry {
    from = "${project_name}/"
    to   = "."
  }
}

hooks {
  post_create = [
    "go mod tidy",
    "gofmt -w .",
  ]
}
```

**`registry.hcl`**

```hcl
name        = "my-registry"
description = "Company blueprint registry"

blueprint "go-api" {
  path        = "go/api"
  description = "Go API service with standard tooling"
  tags        = ["go", "api", "grpc"]
}

blueprint "go-cli" {
  path        = "go/cli"
  description = "Go CLI application"
  tags        = ["go", "cli"]
}
```

**Why this shape**

- One syntax everywhere. No mental context switch between blueprint config and the templates it ships.
- `condition.when` is an HCL expression at parse time, not a string we re-parse at evaluation. Errors surface with file/line/column from the same diagnostic source as template errors.
- `variable "name" { … }` and `blueprint "name" { … }` use HCL block typing natively — closer to how the Terraform-shaped audience already thinks.
- Comments are `#` or `//`. Multi-line values use HCL heredocs instead of YAML block scalars.

**Trade-offs we accept**

- A second breaking migration inside one minor-version window (v1→v2 in v0.3.0, v2-YAML→HCL in v0.4.0). The forge-registry corpus pays the cost twice. Acceptable because the alternative is permanent hybrid pain in a pre-1.0 product.
- HCL has weaker editor/lint defaults than YAML in some environments (GitHub web rendering in particular). The HCL ecosystem is mature enough — `hashicorp/hcl/v2`, `hclwrite`, `hcldec`, and `terraform-ls` LSP — that the gap is one-time setup, not ongoing friction.
- Programmatic emission of blueprint files now requires `hclwrite` rather than any YAML library. Forge tooling already imports `hashicorp/hcl/v2` so this is no new dependency.

### apiVersion handling

No `v3` axis. The on-disk format itself signals the version: `.hcl` config means "the new contract", `.yaml` config means "legacy and rejected." The loader picks the file extension by glob (`blueprint.hcl` preferred, fall back to `blueprint.yaml` only to surface a migration-pointer error). `apiVersion` as an in-file attribute goes away entirely — it was a workaround for YAML's lack of out-of-band schema versioning, and HCL's file-shape-as-schema makes it redundant.

### Rejected alternatives

- **Status quo + better docs.** Considered and rejected: documentation is a workaround for the syntax mix, not a fix. Authors still have to learn "which YAML strings are HCL strings."
- **Hybrid `blueprint.hcl` + `registry.yaml`.** Considered and rejected: inconsistent file formats across the registry tree, and the registry-format change is cheap to bundle into the same migration.
- **`apiVersion: v3` inside HCL.** Considered and rejected: redundant with file extension; adds a second schema-version axis the loader has to reconcile.

## API / Interface Changes

- **CLI:** extend `forge migrate` with a `config` subcommand (or fold a second pass into `forge migrate templates`) that rewrites `blueprint.yaml` → `blueprint.hcl` and `registry.yaml` → `registry.hcl`. Same dirty-worktree guard, `--dry-run`, and `--strict` semantics as the template rewriter. Delete the old `.yaml` files on success; leave them in place on `--dry-run`.
- **Loader (`internal/config/`):** replace `LoadBlueprint` and `LoadRegistry` with HCL-backed implementations using `hcldec.Decode` against a schema spec. If the loader encounters `blueprint.yaml` or `registry.yaml`, it returns a load-time error pointing at `forge migrate config` and `docs/MIGRATION.md` — same pattern as the v1→v2 rejection path.
- **`internal/registrycmd/`:** `forge registry init` and `forge registry blueprint` emit `.hcl` scaffolding instead of `.yaml`. Update tests/fixtures.
- **Removed surfaces:** the YAML parsers (`gopkg.in/yaml.v3` for config files), `apiVersion` field on both structs, and the v2 schema validators in `internal/config/validate.go` all go away. (Lockfile YAML stays untouched — it's a separate concern.)

## Data Model

The Go struct schema in `internal/config/blueprint.go` and `internal/config/registry.go` stays close to today's shape, with two changes:

- **`apiVersion` field dropped** from both `Blueprint` and `Registry`. The file extension is the version signal.
- **`Condition.When`** becomes an `hcl.Expression` (parsed at load time) rather than a `string` that the evaluator re-parses on every call. Small perf win and stronger error locality.

```
Blueprint:
  name: string
  description: string
  version: string
  tags: []string
  variables: []Variable
  defaults: Defaults
  conditions: []Condition       # Condition.When is hcl.Expression
  hooks: Hooks
  sync: SyncConfig
  rename: map[string]string

Registry:
  name: string
  description: string
  blueprints: []BlueprintEntry
```

Decoding is via `hcldec.ObjectSpec` rather than struct tags. We can keep the existing struct definitions and add a parallel spec, or move to fully `hcl:"…"`-tagged structs — to be decided during implementation. Either way the consumers of these structs (`internal/create/`, `internal/sync/`, `internal/check/`, `internal/registrycmd/`) need no changes beyond import paths.

## Testing Strategy

- `testdata/registry/` and `testdata/v2-registry/` become rejection fixtures (same pattern as `testdata/v1-registry/` post-v0.3.0). A new `testdata/hcl-registry/` is the canonical happy-path fixture.
- Loader tests: assert `blueprint.yaml`/`registry.yaml` are rejected with the migration-pointer error string.
- Migration-tool tests: a new table walks YAML fixtures through `forge migrate config` and asserts the HCL output matches a golden file. The existing v1→v2 template-content rewrite tests stay valid.
- forge-registry corpus: PR #5 will be closed; the downstream repo gets a fresh PR running the unified `forge migrate` (templates + config) once v0.4.0 cuts.

## Migration / Rollout Plan

Phased, mirroring IMPL-0004. Bundle the cutover as `v0.4.0` — pre-1.0 minors are the right channel for a documented breaking change.

| Phase | Work |
|-------|------|
| A | New HCL loader + `hcldec` decoder in `internal/config/`. Side-by-side with the YAML loader, gated by file presence (`blueprint.hcl` wins; `blueprint.yaml` errors with a migration pointer). |
| B | Migration-tool extension: `forge migrate config --path <registry>` (or fold into `forge migrate templates`). Rewrites `blueprint.yaml` → `blueprint.hcl` and `registry.yaml` → `registry.hcl`. Same dirty-worktree guard, `--dry-run`, `--strict` semantics. |
| C | Delete the YAML loader. Delete `apiVersion` from both structs. `forge registry init` / `blueprint` emit `.hcl`. Update all `testdata/` fixtures. |
| D | Docs: rewrite DESIGN-0001 and DESIGN-0002 for HCL configs; new section in `docs/MIGRATION.md` covering `forge migrate config`; release notes drafted alongside the merge. Close forge-registry PR #5 and open a fresh PR running the full migration against the current downstream tree. |

## Open Questions

All four open questions are resolved as of 2026-05-12:

- **OQ-1 (resolved):** Option A — both `blueprint.yaml` and `registry.yaml` move to HCL. Full move, no hybrid. No YAML config files in the registry tree.
- **OQ-2 (resolved):** No `apiVersion: v3` axis. The file extension is the version signal; the `apiVersion` field is dropped from both structs.
- **OQ-3 (resolved):** forge-registry PR #5 will be closed and a fresh PR opened post-v0.4.0 — the downstream repo has accumulated enough merges between the original migration PR and now that re-running the unified migration against current `main` is cleaner than rebasing #5.
- **OQ-4 (resolved):** No external `registry.yaml` consumers to protect — all HCL.

## References

- [ADR-0001 — Use HCL2 as the template engine](../adr/0001-use-hcl2-as-the-template-engine.md)
- [DESIGN-0001 — Blueprint Authoring](0001-blueprint-authoring.md) (current authoring contract under v2)
- [DESIGN-0002 — Registry Layout & Defaults Inheritance](0002-registry-layout-and-defaults-inheritance.md)
- [DESIGN-0003 — Migrate template engine to HCL2](0003-migrate-template-engine-to-hcl2.md)
- [IMPL-0004 — Migrate template engine to HCL2](../impl/0004-migrate-template-engine-to-hcl2.md) (OQ-6 captures the YAML-stays decision being reconsidered here)
- [docs/MIGRATION.md](../MIGRATION.md) — current v1 → v2 migration guide; this design adds a "YAML → HCL config" section
- [forge-registry PR #5](https://github.com/donaldgifford/forge-registry/pull/5)
- `internal/config/blueprint.go` — current loader
- `hashicorp/hcl/v2` and `hashicorp/hcl/v2/hcldec` — the alternative decoding pipeline
