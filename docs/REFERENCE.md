# Forge HCL reference

This is the canonical reference for the four HCL file formats forge
reads and writes:

| File | Authored by | Read by | Section |
|------|-------------|---------|---------|
| `blueprint.hcl` | Blueprint author | `forge create`, `forge sync`, `forge check`, `forge info` | [blueprint.hcl](#blueprinthcl) |
| `registry.hcl` | Registry maintainer | `forge list`, `forge search`, `forge create` (for resolution) | [registry.hcl](#registryhcl) |
| `.forge-lock.hcl` | `forge create` / `forge sync` (generated) | `forge sync`, `forge check`, `forge info` | [.forge-lock.hcl](#forge-lockhcl) |
| `.forge-vars.hcl` | Project consumer | `forge create --var-file`, `forge sync --var-file` | [.forge-vars.hcl](#forge-varshcl) |

> **Source of truth.** This document is hand-maintained but every
> claim is traceable to a Go source file. If the implementation and
> this doc diverge, the implementation wins — file a bug. See
> [INV-0002](investigation/0002-auto-generate-hcl-reference-doc.md) for an
> evaluation of auto-generation approaches we considered.

> **Format gating.** HCL is the only accepted on-disk format from
> v0.5+ for all four files. Bare `.yaml` inputs are rejected at load
> time with a rescaffold-or-pin pointer (see
> [docs/MIGRATION.md](MIGRATION.md) and
> [ADR-0002](adr/0002-forge-does-not-ship-in-tool-migrators.md)).

<!--toc:start-->
- [blueprint.hcl](#blueprinthcl)
  - [Top-level attributes](#top-level-attributes)
  - [variable](#variable)
  - [condition](#condition)
  - [defaults](#defaults)
  - [hooks](#hooks)
  - [sync](#sync)
  - [rename](#rename)
- [registry.hcl](#registryhcl)
- [.forge-lock.hcl](#forge-lockhcl)
- [.forge-vars.hcl](#forge-varshcl)
- [Variable types](#variable-types)
- [Sync strategies](#sync-strategies)
- [CLI flags affecting config](#cli-flags-affecting-config)
- [Source-of-truth files](#source-of-truth-files)
<!--toc:end-->

---

## blueprint.hcl

The blueprint config file lives at the root of a blueprint directory
inside the registry (e.g. `go/api/blueprint.hcl`). It declares the
blueprint's identity, the variables it prompts for, conditional file
inclusion rules, post-create hooks, and the managed-file sync
manifest.

### Top-level attributes

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | **yes** | Blueprint identifier (e.g. `"go-api"`). Surfaced by `forge list`. |
| `description` | string | no | Short human-readable summary. Surfaced by `forge info`. |
| `version` | string | no | Author-controlled semver. Recorded in the lockfile. |
| `tags` | list(string) | no | Tag list for `forge search` filtering. |

```hcl
name        = "go-api"
description = "Production Go service with metrics, tracing, and Helm chart"
version     = "1.2.0"
tags        = ["go", "service", "api"]
```

### variable

`variable "name" { … }` declares a user-prompted variable. The block
label becomes the variable name. Repeat the block once per variable.

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `type` | string | **yes** | One of [`string`, `bool`, `choice`, `int`](#variable-types). |
| `description` | string | no | Shown to the user during interactive prompts. |
| `default` | expression | no | Default value. May be a template (`"${other_var}-api"`) — evaluated after the variables it references are bound. |
| `required` | bool | no | When `true`, the user cannot accept an empty value. |
| `choices` | list(string) | conditional | **Required when `type = "choice"`**; the menu options. Ignored for other types. |
| `validate` | string | no | Regular expression (Go `regexp` syntax) the resolved value must match. Compiled at load time. |

```hcl
variable "project_name" {
  type        = "string"
  description = "Service name; used in module paths and helm release name"
  required    = true
  validate    = "^[a-z][a-z0-9-]*$"
}

variable "go_module" {
  type    = "string"
  default = "github.com/example/${project_name}"
}

variable "use_grpc" {
  type    = "bool"
  default = "false"
}

variable "license" {
  type    = "choice"
  choices = ["Apache-2.0", "MIT", "BSD-3-Clause"]
  default = "Apache-2.0"
}

variable "port" {
  type    = "int"
  default = "8080"
}
```

> **Why `default` is stringly-typed.** It is held as raw source text
> so the prompt renderer can re-evaluate it after other variables are
> bound (the template `"${project_name}-api"` only resolves once
> `project_name` is known). Coercion to the declared type happens
> when the resolved value is read.

### condition

`condition { when = … exclude = […] }` conditionally excludes files
from the rendered output. Repeat the block once per rule.

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `when` | expression | **yes** | HCL2 expression evaluated against the resolved variable set. Excludes apply when this evaluates to `true`. Captured at load time so syntax errors surface with file:line:col. |
| `exclude` | list(string) | no | Glob patterns (relative to the blueprint root) to exclude when `when` is true. |

```hcl
condition {
  when    = use_grpc == false
  exclude = ["proto/**", "Makefile.grpc"]
}
```

### defaults

`defaults { … }` controls inherited `_defaults/` files
(see [DESIGN-0002](design/0002-registry-layout-and-defaults-inheritance.md)).

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `exclude` | list(string) | no | Relative paths under `_defaults/` to omit from this blueprint. |
| `override_strategy` | map(string→string) | no | Per-path sync-strategy override. Values must be a valid [sync strategy](#sync-strategies). |

```hcl
defaults {
  exclude = [".github/dependabot.yml"]
  override_strategy = {
    "Makefile" = "merge"
  }
}
```

### hooks

`hooks { post_create = […] }` configures lifecycle hooks.

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `post_create` | list(string) | no | Shell commands run after `forge create` completes. Each command runs through `sh -c` in the project root. Cancellation via the create context propagates. |

```hcl
hooks {
  post_create = [
    "go mod tidy",
    "make fmt",
  ]
}
```

### sync

`sync { … }` declares the managed-file manifest used by `forge sync`
and `forge check`.

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `ignore` | list(string) | no | Glob patterns excluded from drift detection and sync entirely. |

| Nested block | Required | Description |
|--------------|----------|-------------|
| `managed_file "path" { strategy = … }` | conditional | One per managed file. Label is the project-relative path. |

The nested `managed_file` block:

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `strategy` | string | **yes** | One of [`overwrite`, `merge`](#sync-strategies). |

```hcl
sync {
  ignore = ["dist/**", "vendor/**"]

  managed_file "Makefile" {
    strategy = "merge"
  }

  managed_file ".github/workflows/ci.yml" {
    strategy = "overwrite"
  }
}
```

### rename

`rename { entry { from = …, to = … } … }` rewrites output paths.
Children are unlabeled `entry` blocks so both `from` and `to` can
carry templates (block labels and attribute names reject template
sequences).

| `entry` attribute | Type | Required | Description |
|-------------------|------|----------|-------------|
| `from` | string | **yes** | Source path glob. May contain `${var}` references. |
| `to` | string | **yes** | Destination path. May contain `${var}` references. |

```hcl
rename {
  entry {
    from = "cmd/app/main.go"
    to   = "cmd/${project_name}/main.go"
  }
}
```

---

## registry.hcl

The index file at the root of a registry repo. Maintained by
`forge registry update` after blueprint additions/changes, but
hand-editable.

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | **yes** | Registry display name. |
| `description` | string | no | Short summary. |

| Nested block | Required | Description |
|--------------|----------|-------------|
| `maintainer { name = …, email = … }` | no | Repeatable. |
| `defaults { sync_strategy = …, managed = … }` | no | Registry-wide defaults. Distinct from the per-blueprint `defaults` block in `blueprint.hcl`. |
| `blueprint "name" { … }` | no | Repeatable catalog entry. Label is the blueprint name. |

`defaults`:

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `sync_strategy` | string | no | Default sync strategy applied to inherited `_defaults/` files. One of [`overwrite`, `merge`](#sync-strategies). |
| `managed` | bool | no | When `true`, registry-wide defaults are treated as managed files for sync. |

`blueprint` catalog entry:

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | **yes** | Relative path from the registry root to the blueprint directory. |
| `description` | string | no | Short summary. |
| `version` | string | no | Author-controlled semver (mirrors the value in `blueprint.hcl`). |
| `tags` | list(string) | no | Tag list for search. |
| `latest_commit` | string | no | Commit SHA when the catalog entry was last refreshed. Set by `forge registry update`. |

```hcl
name        = "Example Registry"
description = "Internal blueprints for the platform team"

maintainer {
  name  = "Platform Team"
  email = "platform@example.com"
}

defaults {
  sync_strategy = "merge"
  managed       = true
}

blueprint "go-api" {
  path        = "go/api"
  description = "Production Go service"
  version     = "1.2.0"
  tags        = ["go", "service"]
}
```

---

## .forge-lock.hcl

The lockfile records the provenance of a scaffolded project and the
state needed by `forge sync` / `forge check`. It is **generated** by
`forge create` and **rewritten** by `forge sync`. Do not edit by
hand — the file carries a `# DO NOT EDIT MANUALLY` header.

A bare `.forge-lock.yaml` (the legacy format) is rejected at load
time with a rescaffold-or-pin error per
[ADR-0002](adr/0002-forge-does-not-ship-in-tool-migrators.md).

| Attribute | Type | Description |
|-----------|------|-------------|
| `created_at` | string (RFC3339 UTC) | When the project was scaffolded. |
| `last_synced` | string (RFC3339 UTC) | When `forge sync` last touched the project. Equal to `created_at` on a fresh scaffold. |
| `forge_version` | string | The forge binary version that wrote this lockfile. |

| Nested block | Description |
|--------------|-------------|
| `blueprint { … }` | The source blueprint reference (one per lockfile). |
| `variables { … }` | The resolved variable map used to render this project. Keys are dynamic. |
| `default "path" { … }` | Repeatable. One entry per inherited default file. |
| `managed_file "path" { … }` | Repeatable. One entry per managed file. |

`blueprint`:

| Attribute | Type | Description |
|-----------|------|-------------|
| `registry_url` | string | Go-getter URL or local path of the registry. |
| `name` | string | Blueprint name (from `blueprint.hcl`). |
| `path` | string | Blueprint path within the registry. |
| `ref` | string | Git ref (branch/tag) at scaffold time. |
| `commit` | string | Commit SHA at scaffold time. Used as the base in three-way merge during sync. |

`default "path"`:

| Attribute | Type | Description |
|-----------|------|-------------|
| `source` | string | Which layer the default came from: `registry-default`, `<category>-default`, etc. |
| `strategy` | string | One of [`overwrite`, `merge`](#sync-strategies). |
| `hash` | string | `sha256:<hex>` of the rendered file content. Used by `forge check` for drift detection. |
| `synced_commit` | string | Commit SHA last successfully synced for this file. |

`managed_file "path"`:

| Attribute | Type | Description |
|-----------|------|-------------|
| `strategy` | string | One of [`overwrite`, `merge`](#sync-strategies). |
| `hash` | string | `sha256:<hex>` of the file content. Used by `forge check` for drift detection. |
| `synced_commit` | string | Commit SHA last successfully synced for this file. |

`variables` carries native HCL primitives (string / number / bool /
list / map) — types are preserved end-to-end from
`blueprint.hcl`.

```hcl
# .forge-lock.hcl — DO NOT EDIT MANUALLY
# Generated by forge. Changes will be overwritten on sync.

blueprint {
  registry_url = "github.com/example/forge-registry"
  name         = "go-api"
  path         = "go/api"
  ref          = "v1.2.0"
  commit       = "a3f9c1d"
}

created_at    = "2026-05-19T10:15:30Z"
last_synced   = "2026-05-19T10:15:30Z"
forge_version = "0.6.0"

variables {
  project_name = "my-api"
  go_module    = "github.com/example/my-api"
  use_grpc     = false
  port         = 8080
}

default ".editorconfig" {
  source   = "registry-default"
  strategy = "overwrite"
  hash     = "sha256:abc123..."
}

managed_file "Makefile" {
  strategy = "merge"
  hash     = "sha256:def456..."
}
```

---

## .forge-vars.hcl

A `.forge-vars.hcl` file supplies variable values to `forge create`
and `forge sync` via `--var-file`. The format is intentionally
narrow: **attributes only, literal values only**. No blocks,
function calls, or traversals. The `.hcl` extension is **required**
on the path.

See [DESIGN-0005](design/0005-variable-input-via-vars-file.md) for
the contract and
[IMPL-0008](impl/0008-variable-input-via-vars-file.md) for the
implementation details.

```hcl
# my-svc.forge-vars.hcl
project_name = "mockta-staging"
go_module    = "github.com/example/mockta"
use_grpc     = true
port         = 8080
tags         = ["beta", "staging"]
```

| Rule | Detail |
|------|--------|
| File extension | Must be `.hcl`. Process substitution (`<(…)`) fails this check; use the tempfile pattern instead. |
| Document grammar | Top-level attributes only. Blocks (`foo { … }`) are rejected with a `vars file may not contain blocks` error. |
| Expression grammar | Values must be literal: strings, numbers, bools, lists, maps. Function calls (`upper("x")`), references (`other_var`), and computed expressions are rejected with a `variable only accepts literal values` error. |
| Composition | `--var-file` is repeatable; later files override earlier files on key collision. |
| Type coercion | Values are coerced against the declared blueprint variable type via `cty/convert`. Coercion failures abort before any files are written. |
| Unknown keys | Keys that are not declared in `blueprint.hcl` surface as a CLI **warning**, not an error, and are silently dropped from the resolved map. |
| Mutual exclusion | `--var-file` and `--set` cannot both appear on a single invocation. The CLI rejects the combination. |
| `forge sync` | `--var-file` requires `--force` because it rewrites the lockfile with the new resolved values. |
| `forge check` | `--var-file` is rejected with an actionable error pointing at `forge sync --var-file FILE --force --dry-run`. |

---

## Variable types

The four types accepted by the `type` attribute of a `variable`
block. Defined in
[`internal/config/validate.go`](../internal/config/validate.go):

| Type | Description | Native cty type |
|------|-------------|-----------------|
| `string` | Free-text string. Optionally constrained via `validate` regex. | `cty.String` |
| `bool` | Boolean. Accepts `true`/`false` literals or stringly-typed coercions (e.g. `"true"`). | `cty.Bool` |
| `choice` | String constrained to one of `choices`. **`choices` is required.** | `cty.String` |
| `int` | Integer. Stored as `cty.Number` natively; coerced from string vars-file inputs (`port = "8080"` works). | `cty.Number` |

Adding a new type means touching three sites:

1. [`internal/config/validate.go`](../internal/config/validate.go) — `validVariableTypes` map.
2. [`internal/lockfile/cty.go`](../internal/lockfile/cty.go) — `ToCtyValues` / `FromCtyValues`.
3. [`internal/varsfile/varsfile.go`](../internal/varsfile/varsfile.go) — `ctyTypeForDeclared`.

---

## Sync strategies

The two strategies accepted by `strategy` attributes on `managed_file`
blocks and the `override_strategy` map. Defined in
[`internal/config/validate.go`](../internal/config/validate.go):

| Strategy | Behaviour |
|----------|-----------|
| `overwrite` | `forge sync` replaces the local file with the rendered upstream version, no merge attempted. |
| `merge` | `forge sync` runs a three-way merge using the lockfile's `synced_commit` as the base, the local file as `ours`, and the rendered upstream as `theirs`. Conflicts are reported via `forge sync` exit code and surfaced through `ReportConflicts`. |

---

## CLI flags affecting config

A non-exhaustive list of flags that change how forge reads or writes
the files above:

| Flag | Commands | Effect |
|------|----------|--------|
| `--set key=value` | `create` | Inline variable value. Repeatable. Mutually exclusive with `--var-file`. |
| `--var-file PATH` | `create`, `sync` | Load variable values from a `.forge-vars.hcl` file. Repeatable. On `sync`, requires `--force`. Rejected on `check`. |
| `--force` | `create`, `sync` | On `create`: write into a non-empty directory. On `sync`: required when `--var-file` is supplied (acknowledges the lockfile rewrite). |
| `--registry-dir` | `create`, `sync`, `check` | Override the registry source. Accepts local paths and go-getter URLs (auto-detected). |
| `--ref` | `sync` | Pin sync against a specific registry version/ref. |
| `--dry-run` | `sync` | Print what would change without writing. Compose with `--var-file --force` for the preview workflow. |
| `--file PATH` | `sync` | Sync only a specific file path. |

---

## Source-of-truth files

Every claim in this document maps to one or more of:

| Concern | File |
|---------|------|
| Blueprint Go schema | [`internal/config/blueprint.go`](../internal/config/blueprint.go) |
| Registry Go schema | [`internal/config/registry.go`](../internal/config/registry.go) |
| Lockfile Go schema | [`internal/lockfile/lock.go`](../internal/lockfile/lock.go) |
| HCL decode specs (blueprint + registry) | [`internal/config/hcldec_spec.go`](../internal/config/hcldec_spec.go) |
| Allowed `type` and `strategy` values | [`internal/config/validate.go`](../internal/config/validate.go) |
| Variable-to-cty type mapping | [`internal/lockfile/cty.go`](../internal/lockfile/cty.go) |
| Vars-file parser + coercion | [`internal/varsfile/varsfile.go`](../internal/varsfile/varsfile.go) |
| Lockfile emitter (HCL output shape) | [`internal/lockfile/emit_hcl.go`](../internal/lockfile/emit_hcl.go) |
| Registry emitter (HCL output shape) | [`internal/config/hclemit.go`](../internal/config/hclemit.go) |

If you change the schema in any of these files, update this document
in the same PR. See
[INV-0002](investigation/0002-auto-generate-hcl-reference-doc.md) for the
auto-generation analysis — until that lands, this doc is
hand-maintained.

## References

- [DESIGN-0001 — Blueprint Authoring](design/0001-blueprint-authoring.md) — the authoring contract these specs implement.
- [DESIGN-0002 — Registry layout and defaults inheritance](design/0002-registry-layout-and-defaults-inheritance.md) — `_defaults/` semantics.
- [DESIGN-0004 — Unify config file format after HCL2 cutover](design/0004-unify-config-file-format-after-hcl2-cutover.md) — why HCL everywhere.
- [DESIGN-0005 — Variable input via vars file](design/0005-variable-input-via-vars-file.md) — `.forge-vars.hcl` contract.
- [ADR-0001 — Use HCL2 as the template engine](adr/0001-use-hcl2-as-the-template-engine.md).
- [ADR-0002 — Forge does not ship in-tool migrators](adr/0002-forge-does-not-ship-in-tool-migrators.md).
- [docs/MIGRATION.md](MIGRATION.md) — upgrade paths between format generations.
