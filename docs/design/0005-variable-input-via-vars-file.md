---
id: DESIGN-0005
title: "Variable input via vars file"
status: Draft
author: Donald Gifford
created: 2026-05-18
---
<!-- markdownlint-disable-file MD025 MD041 -->

# DESIGN 0005: Variable input via vars file

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-05-18

<!--toc:start-->
- [Overview](#overview)
- [Goals and Non-Goals](#goals-and-non-goals)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Background](#background)
- [Detailed Design](#detailed-design)
  - [File Format](#file-format)
  - [CLI Surface](#cli-surface)
  - [Precedence and Composition](#precedence-and-composition)
  - [Mutual Exclusion with --set](#mutual-exclusion-with---set)
  - [Lockfile Integration](#lockfile-integration)
- [API / Interface Changes](#api--interface-changes)
  - [CLI](#cli)
  - [Programmatic / Internal](#programmatic--internal)
- [Data Model](#data-model)
- [Testing Strategy](#testing-strategy)
- [Migration / Rollout Plan](#migration--rollout-plan)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Overview

Add a `--var-file` flag to `forge create` (and `forge sync` / `forge check`
where it applies) that loads variable values from an HCL document. This
replaces the long `--set k=v --set k=v ...` chains that currently
dominate non-trivial scaffold commands, and lays the input-side
groundwork for object/list/map variable types (see RFC-0002, designed in DESIGN-0006).

## Goals and Non-Goals

### Goals

- Provide a file-based alternative to `--set` for passing variable
  values into `forge create` and `forge sync`. (`forge check`
  registers the flag solely to emit a clear rejection error — see
  OQ-5 for the rationale.)
- Use HCL2 as the file format so the input grammar matches the rest
  of forge (blueprint.hcl, registry.hcl) and naturally handles nested
  structures when object/list/map types eventually land.
- Make `--var-file` and `--set` mutually exclusive on a single
  invocation — fail fast with a clear error rather than silently
  applying precedence rules.
- Preserve the existing "missing required variable" error path: if a
  blueprint declares a required variable and neither `--var-file` nor
  `--set` supplies it, `forge create` fails the way it does today.
- Record resolved variable values in the lockfile regardless of input
  source (already happens — make sure the var-file path doesn't
  regress it).

### Non-Goals

- **Auto-loading.** Terraform auto-loads `terraform.tfvars` and
  `*.auto.tfvars`. forge stays explicit-only initially — users pass
  `--var-file PATH`. Removes the "where did this value come from?"
  surprise. Could be added later behind an opt-in if demand surfaces.
- **JSON variant.** `*.forge-vars.json` is a natural follow-up for
  tools that emit JSON (CI, web UIs), but ships separately. The HCL
  parser handles it for free via hcldec's JSON support — wiring is
  trivial but adds CLI surface and tests. Defer to a follow-up.
- **Object / list / map variable types.** Out of scope here — covered
  by RFC-0002 (designed in DESIGN-0006). This DESIGN ships with
  scalar-only support and the file format naturally extends when
  object types land.
- **Lockfile format change.** Out of scope — covered by IMPL-0006.

## Background

Today `forge create` collects variable values from three sources, in
precedence order:

1. **`--set k=v` CLI flags** — repeated per variable.
2. **Interactive prompts** (`internal/prompt/`) — for any variable
   not supplied via flag.
3. **Blueprint defaults** — used when the user accepts the prompt or
   skips it with `--defaults`.

This works fine for one-shot scaffolds with two or three variables
but degrades quickly. A representative `forge create` against
forge-registry's `go/ext` blueprint looks like:

```sh
forge create go/ext \
  --registry-dir ./forge-registry \
  --output-dir /tmp/mockta \
  --set project_name=mockta \
  --set project_owner=donaldgifford \
  --set project_description="A lightweight, embeddable Okta mock for Terraform tests" \
  --set git_provider=github \
  --defaults --force
```

Issues:

- **CI/automation friction.** Scripts that scaffold projects from
  forge accumulate brittle `--set` chains; quoting rules for strings
  with spaces or shell metacharacters become a real source of bugs.
- **Reproducibility.** No durable record of what inputs produced a
  given scaffolded project, separate from the lockfile (which lives
  inside the project).
- **No path for non-scalar values.** `--set k=v` is fundamentally
  scalar. When object/list/map types land (RFC-0002 / DESIGN-0006),
  there's no CLI surface that can express them ergonomically.
- **Documentation surface.** A `.forge-vars.hcl` checked in next to
  a scaffolded project is a self-documenting input artifact.

Terraform's `.tfvars` pattern solves the same problem for the same
reasons. Forge can adopt a directly analogous mechanism.

## Detailed Design

### File Format

A vars file is an HCL2 document containing top-level attribute
assignments, one per variable. No block syntax, no `variable "name"
{ … }` wrappers — those are blueprint declarations. Vars files only
carry values.

```hcl
# mockta.forge-vars.hcl
project_name        = "mockta"
project_owner       = "donaldgifford"
project_description = "A lightweight, embeddable Okta mock for Terraform tests"
git_provider        = "github"

# When object types land (RFC-0002 / DESIGN-0006), this just works:
# git_provider = {
#   repo_type   = "github"
#   repo_url    = "github.com"
#   project_org = "donaldgifford"
# }
```

Conventional extension: **`.forge-vars.hcl`**. The `.hcl` suffix lets
editors syntax-highlight correctly and matches the
`blueprint.hcl`/`registry.hcl` naming convention. No restriction on
the prefix — users name their files however helps them
(`prod.forge-vars.hcl`, `staging.forge-vars.hcl`, etc.).

Keys in a vars file are **bare attribute names**, not namespaced.
A vars file declares variable *values*; it doesn't reference variable
names in expressions. RFC-0003's `var.NAME` / `local.NAME` namespacing
is a template/condition/default-expression concern, and doesn't apply
here.

The file is parsed via `hashicorp/hcl/v2`. Coercion from HCL values
to forge's in-memory variable map uses the same `cty.Value` plumbing
already in `internal/lockfile/cty.go`, driven by the blueprint's
declared variable types.

### CLI Surface

```sh
forge create <blueprint> --var-file PATH [--var-file PATH ...] [other flags]
forge sync                --var-file PATH [--var-file PATH ...] [other flags]
forge check               --var-file PATH [--var-file PATH ...] [other flags]
```

`--var-file` is repeatable. Multiple files compose left-to-right with
last-wins semantics on key collision. This mirrors how multiple `-var-file`
arguments compose in Terraform and how layered configs typically work
in Unix tools.

```sh
# Base config + environment override:
forge create go/ext \
  --var-file ./base.forge-vars.hcl \
  --var-file ./staging.forge-vars.hcl \
  --defaults --force
```

The blueprint argument and other flags (`--registry-dir`,
`--output-dir`, `--defaults`, `--force`, `--ref`) behave unchanged.

### Precedence and Composition

The resolved value for a variable is determined in this order
(highest precedence wins):

1. `--var-file` (when present) — composed left-to-right, last wins
   across files.
2. Interactive prompt — for any variable still unresolved after step
   1, prompt as today.
3. Blueprint default — used when the prompt is skipped (`--defaults`)
   or accepted unchanged.

`--set` does not appear in this chain because it is mutually
exclusive with `--var-file` (see next section).

If neither `--var-file` nor `--set` is provided, behavior is
unchanged from today: interactive prompts for any required variable,
defaults for the rest.

### Mutual Exclusion with `--set`

A single `forge create` invocation may use **either** `--var-file` or
`--set`, never both. Mixing the two produces a load-time error:

```text
Error: --var-file and --set cannot be combined. Use one input source per invocation:
  - For one-off overrides: forge create … --set k=v
  - For multiple values:   forge create … --var-file path/to/foo.forge-vars.hcl
```

Rationale:

- **No precedence rules to memorise.** Either the file is the source
  of truth, or the flags are. Avoids the "did the file override the
  flag or vice versa?" confusion that has tripped Terraform users.
- **Simpler implementation.** No merge step, no per-key precedence
  bookkeeping at the CLI layer.
- **Easier mental model for `--var-file` composition.** Within
  `--var-file`, the "later wins" rule is intuitive and follows
  ordering on the command line.

Users who want to override a single field of a vars-file-driven
scaffold should compose with a second `--var-file` pointed at a
tempfile:

```sh
cat > /tmp/override.forge-vars.hcl <<EOF
project_name = "mockta-staging"
EOF
forge create go/ext \
  --var-file ./base.forge-vars.hcl \
  --var-file /tmp/override.forge-vars.hcl
```

The tempfile pattern keeps the "files only" model intact while still
giving a clean one-off override path. Process substitution
(`<(...)`) is **not supported** — IMPL-0008 OQ-8 settled on a strict
`.hcl` extension check, and `/dev/fd/63`-style paths from
`<(...)` fail that check (see the
[Mutual Exclusion](#mutual-exclusion-with---set) discussion below).

### Lockfile Integration

Lockfile behavior is **unchanged**. The `Variables` field of
`.forge-lock.{yaml,hcl}` records the *resolved* values used at create
time, irrespective of how they were provided. A scaffold from
`--var-file foo.hcl` writes the same lockfile shape as a scaffold
from `--set k=v` with the same effective inputs.

The lockfile does **not** record the path of the vars file used, by
design: the file may move, be deleted, or be regenerated. The
authoritative record of "what values produced this project" lives in
the lockfile's `Variables` field.

When the lockfile migrates from YAML to HCL (IMPL-0006), the parser
and emitter machinery shared with `--var-file` is reused — a single
`.forge-vars.hcl` parse path covers both inputs and persisted state.

## API / Interface Changes

### CLI

New flag on `forge create`, `forge sync`, `forge check`:

```text
--var-file PATH    Load variable values from an HCL document. Repeatable;
                   later files override earlier ones on key collision.
                   Mutually exclusive with --set.
```

No deprecation of `--set`. Both remain first-class; users pick the
right tool per invocation.

### Programmatic / Internal

- New package `internal/varsfile/` (proposed) — single responsibility:
  parse one or more `.forge-vars.hcl` files into the
  `map[string]cty.Value` shape that `internal/prompt/` and
  `internal/create/` already consume.
- `internal/create/` (and `sync`/`check` equivalents) gain an
  alternate input path: when `--var-file` is set, skip `--set`
  parsing and `internal/prompt/` defaulting for any variable
  satisfied by the file; everything downstream of value resolution
  is unchanged.

## Data Model

Vars files are parsed into `map[string]cty.Value` keyed by variable
name. Type coercion against the blueprint's declared variable types
happens at resolution time, using the existing
`lockfile.ToCtyValues`-style machinery. Coercion errors are surfaced
with file/line/column from the HCL source:

```text
Error: ./prod.forge-vars.hcl:4,16-21: variable "use_docker" declared as bool
in blueprint, but the file provides a string "true". Use a bare bool literal:
  use_docker = true
```

Unknown keys (values present in the file but not declared in any
blueprint variable) are a warning, not an error — they may be
intentional comments-as-data or shared across multiple blueprints
that declare different subsets. The warning lists the unknown keys
so typos still surface visibly.

## Testing Strategy

- **Unit tests** (`internal/varsfile/`):
  - Happy path: simple scalar file parses into the expected `cty.Value`
    map.
  - Repeated `--var-file`: composition order, last-wins on collision.
  - Type coercion errors include source location.
  - Unknown keys produce a warning, not an error.
  - Malformed HCL: parse error includes file/line/column.
- **Integration tests** (`internal/create/`):
  - `forge create --var-file path` end-to-end against the hermetic
    HCL fixture (`testdata/hcl-registry/`).
  - `--var-file` + `--set` together → mutual-exclusion error.
  - `--var-file` partial (some required vars missing) → prompts for
    the rest in interactive mode; errors clearly in non-interactive.
- **CLI tests** (`cmd/`): flag wiring, error messages, help text.

## Migration / Rollout Plan

No breaking changes. `--var-file` is purely additive — existing
`--set` users see no behavior change. The minimum-viable rollout:

1. Ship `--var-file` on `forge create` only. Sync and check follow
   in the next minor.
2. Update `docs/MIGRATION.md` with a "preferred input pattern"
   note that doesn't deprecate `--set` but recommends vars files
   for non-trivial cases.
3. Add a vars-file example to README's Quick Start and to each
   blueprint's auto-generated README.

Version: minor bump (v0.5.0 or later — pre-1.0 minors are the right
channel per the precedent set by IMPL-0004 OQ-9 and IMPL-0005 OQ-8).
No release-notes pressure since the feature is additive.

## Open Questions

- **OQ-1: Should `--var-file` accept a directory and auto-glob
  `*.forge-vars.hcl`?** Convenient for layered configs but
  reintroduces some of the auto-loading surprise we're avoiding.
  **Decision: No.** Files must be passed explicitly. Multiple
  `--var-file` flags already cover the layered-config case
  (left-to-right composition, last wins).
- **OQ-2: Should `--var-file` support `-` for stdin?** Useful for
  scripted pipelines. Cheap to support (`if path == "-" { read
  os.Stdin }`). **Decision: No.** `--var-file` takes a real path;
  `--set` already covers the inline / scripted case. Reassess if a
  concrete pipeline use case shows up.
- **OQ-3: Should we lockfile-record the vars file path used?**
  **Decision: No.** The lockfile already records resolved variable
  values, so a vars file can be reconstructed from the lockfile
  either way. Recording a path would just bit-rot when the file
  moves or is regenerated. A follow-up `forge lockfile export-vars`
  helper could reconstruct a `.forge-vars.hcl` from a lockfile if
  that capability becomes useful.
- **OQ-4: Process-substitution UX for single-field overrides.** The
  `<(echo 'k = v')` pattern works but isn't discoverable. Worth a
  README example, or do we revisit and allow a narrow `--set` +
  `--var-file` combination after all? **Decision: No combo.** Keep
  the mutual exclusion clean. Document the override pattern in the
  README; revisit only if real-world friction surfaces.
  **Superseded by IMPL-0008 OQ-8:** process substitution does *not*
  work — IMPL-0008 settled on a strict `.hcl` extension check on
  the input path, and `/dev/fd/63`-style paths from `<(...)` fail
  that check. The documented escape hatch is the *tempfile pattern*
  (see the tempfile example in
  [Mutual Exclusion](#mutual-exclusion-with---set) above, plus
  [MIGRATION.md § Variable input: preferred pattern](../MIGRATION.md#variable-input-preferred-pattern-v06)
  for the user-facing walkthrough).

## References

- [DESIGN-0001 — Blueprint Authoring](0001-blueprint-authoring.md) — variable declaration schema.
- [DESIGN-0004 — Unify config file format after HCL2 cutover](0004-unify-config-file-format-after-hcl2-cutover.md) — precedent for "HCL everywhere" consistency.
- [IMPL-0006 — Migrate lockfile from YAML to HCL](../impl/0006-migrate-lockfile-from-yaml-to-hcl.md) — parallel format consistency work.
- [RFC-0002 — Object and collection variable types](../rfc/0002-object-and-collection-variable-types.md) — the long-tail feature this DESIGN unblocks on the input side.
- [DESIGN-0006 — Object and collection variable types](0006-object-and-collection-variable-types.md) — RFC-0002's design; vars-file parser delegates to `cty.Convert` for structured types per DESIGN-0006's `--var-file` section.
- [RFC-0003 — Locals for derived values](../rfc/0003-locals-for-derived-values.md) — defines the `var.NAME` / `local.NAME` namespacing used in templates and conditions; vars files keep bare keys.
- [Terraform `-var-file` documentation](https://developer.hashicorp.com/terraform/language/values/variables#variable-definitions-tfvars-files) — direct inspiration; the precedence-rules wart in Terraform's design is what motivates the mutual-exclusion rule here.
- `internal/lockfile/cty.go` — existing `cty.Value` coercion machinery this design reuses.
- `internal/prompt/prompt.go` — the resolution flow that gains a new input path.
