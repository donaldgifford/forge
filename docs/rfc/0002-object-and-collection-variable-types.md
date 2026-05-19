---
id: RFC-0002
title: "Object and collection variable types"
status: Draft
author: Donald Gifford
created: 2026-05-18
---
<!-- markdownlint-disable-file MD025 MD041 -->

# RFC 0002: Object and collection variable types

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-05-18

<!--toc:start-->
- [Summary](#summary)
- [Problem Statement](#problem-statement)
- [Proposed Solution](#proposed-solution)
- [Design](#design)
  - [Type System](#type-system)
  - [Declaration Syntax](#declaration-syntax)
  - [Default Values](#default-values)
  - [Input via --var-file](#input-via---var-file)
  - [Input via --set](#input-via---set)
  - [Prompt UX](#prompt-ux)
  - [Template Access](#template-access)
  - [Lockfile Representation](#lockfile-representation)
  - [Validation](#validation)
- [Alternatives Considered](#alternatives-considered)
- [Implementation Phases](#implementation-phases)
  - [Phase 1: Core type-expression support](#phase-1-core-type-expression-support)
  - [Phase 2: Validation block](#phase-2-validation-block)
  - [Phase 3: Default value evaluation for non-scalars](#phase-3-default-value-evaluation-for-non-scalars)
  - [Phase 4: Input — vars-file and --set](#phase-4-input--vars-file-and---set)
  - [Phase 5: Prompt UX](#phase-5-prompt-ux)
  - [Phase 6: Lockfile round-trip](#phase-6-lockfile-round-trip)
  - [Phase 7: Documentation](#phase-7-documentation)
- [Risks and Mitigations](#risks-and-mitigations)
- [Success Criteria](#success-criteria)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Summary

Extend forge's variable system from scalars-only (`string`, `bool`,
`choice`, `int`) to include object (`object({...})`), list
(`list(T)`), and map (`map(T)`) types. Authors can group related
fields under a single named variable (`git_provider.repo_type`,
`git_provider.project_org`), iterate over lists in templates, and
keep blueprint declarations clean as the number of related inputs
grows.

## Problem Statement

forge variables today are flat scalars. As blueprints grow more
expressive — multi-environment configs, conditional toolchain wiring,
git-provider awareness — author-side duplication compounds:

```hcl
# blueprint.hcl — today, fanout of related variables (pre-RFC-0003,
# bare-name references in defaults)
variable "git_provider" {
  type    = "choice"
  choices = ["github", "forgejo"]
  default = "github"
}

variable "git_provider_host" {
  type    = "string"
  default = "${git_provider == \"github\" ? \"github.com\" : \"git.fartlab.dev\"}"
}

variable "project_org" {
  type    = "string"
  default = "${git_provider == \"github\" ? \"donaldgifford\" : \"homelab\"}"
}

variable "renovate_config_repo" {
  type    = "string"
  default = "${git_provider == \"github\" ? \"github\" : \"git.fartlab.dev\"}"
}

# ... three or more conditions referencing all four variables
```

The same pattern repeats across every blueprint in the registry that
wants git-provider awareness, with the only differences being which
fields each blueprint actually uses. The variable surface area is
mostly noise; the logical concept ("which git provider?") is
splintered across four variables.

A real-world driver: forge-registry's renovate config template needs
both a host (`github` vs `git.fartlab.dev`) and an org
(`donaldgifford` vs `homelab`), both derived from the same git
provider choice. Today that's two derived-default variables; with a
fuller config (CI workflow tokens, default labels, etc.) it'd be
six or more.

Lists and maps unblock other natural shapes:

```hcl
variable "exposed_ports" {
  type    = list(number)
  default = [8080, 9090]
}

variable "build_targets" {
  type    = map(string)  # platform → goarch
  default = {
    linux   = "amd64"
    darwin  = "arm64"
    windows = "amd64"
  }
}
```

Currently authors work around this with comma-separated strings or
JSON-encoded blobs — both ugly, neither type-checked.

## Proposed Solution

Lift forge's variable type model from the current four-string-tag
enum to cty's full type system (subset of it). Authors declare
variables with cty type expressions; values flow through the
existing `cty.Value` pipeline (already in `internal/lockfile/cty.go`)
to templates, lockfile, and validation.

The user-facing additions:

- New `type` values: `object({field = T, ...})`, `list(T)`,
  `map(T)`, plus the existing scalars.
- Templates access nested fields via standard HCL2 attribute
  syntax under the `var.` namespace established by RFC-0003:
  `${var.git_provider.repo_type}`, `${var.exposed_ports[0]}`,
  `${var.build_targets["linux"]}`.
- `--var-file` accepts nested values via natural HCL grammar
  (no syntax extensions needed).
- Prompt UX unfolds objects into per-field prompts; lists/maps
  fall through to non-interactive defaults (no good TUI widget for
  arbitrary-length sequences in charmbracelet/huh).
- `--set` gains dotted-path support for object fields only
  (`--set git_provider.repo_type=github`); lists/maps require a
  vars file.

The complexity surface is mostly on the input/prompt side. Templates
and lockfile storage are essentially free because `cty.Value`
already handles all of this.

## Design

### Type System

forge adopts a subset of cty's type expression grammar:

| Type form | Example | Notes |
|-----------|---------|-------|
| `string` | `type = "string"` | Existing. |
| `bool` | `type = "bool"` | Existing. |
| `int` / `number` | `type = "int"` | Existing; `number` aliases to int for now (no rationale for floats yet). |
| `object({...})` | `type = object({repo_type = string, project_org = string})` | New. Nested fields each have their own scalar/object type. |
| `list(T)` | `type = list(string)` | New. Homogeneous, variable length. |
| `map(T)` | `type = map(string)` | New. String-keyed, homogeneous values. |

**Note: `choice` is removed.** The legacy `type = "choice", choices
= [...]` form is replaced with a `validation { condition = ... }`
block on a `string` variable (see [Validation](#validation) below).
This unblocks enum-style constraints inside object fields and brings
forge's validation model in line with Terraform's. Per ADR-0002,
existing `choice` declarations are not migrated in-tool — authors
re-declare them under the new form when adopting v0.7.

Out of scope for the initial cut: `tuple([...])` (heterogeneous
fixed-length), `set(T)` (deduplicated unordered), optional fields
within objects. Add later if demand surfaces.

`null` is not a value forge accepts — every declared variable must
have a non-null resolved value, either via input or default.

### Declaration Syntax

```hcl
# Object
variable "git_provider" {
  description = "Which git provider this project targets"
  type = object({
    repo_type   = string
    repo_url    = string
    project_org = string
  })
  default = {
    repo_type   = "github"
    repo_url    = "github.com"
    project_org = "donaldgifford"
  }
}

# List
variable "exposed_ports" {
  description = "TCP ports the service exposes"
  type        = list(number)
  default     = [8080]
}

# Map
variable "build_targets" {
  description = "OS → goarch matrix"
  type        = map(string)
  default     = {
    linux  = "amd64"
    darwin = "arm64"
  }
}

# Required object — no default; user must supply
variable "auth_provider" {
  type = object({
    kind   = string
    issuer = string
  })
  required = true
}
```

The `type` field becomes an HCL expression captured at load time
(not a string scalar) so cty can parse it into a `cty.Type`. This
mirrors how `Condition.When` already works (`hcl.Expression` parsed
at load time, source kept on `WhenSource`).

### Default Values

Defaults for object/list/map types are HCL expressions, evaluated
in the same scope as scalar defaults (earlier variables in scope).
This means defaults can derive from prior variables — the same
ternary pattern that already works for scalars composes naturally:

```hcl
variable "git_provider_kind" {
  type    = "string"
  default = "github"
  validation {
    condition     = contains(["github", "forgejo"], var.git_provider_kind)
    error_message = "git_provider_kind must be one of: github, forgejo."
  }
}

variable "git_provider" {
  type = object({
    repo_type   = string
    repo_url    = string
    project_org = string
  })
  default = var.git_provider_kind == "github" ? {
    repo_type   = "github"
    repo_url    = "github.com"
    project_org = "donaldgifford"
  } : {
    repo_type   = "forgejo"
    repo_url    = "git.fartlab.dev"
    project_org = "homelab"
  }
}
```

This is the "best of both worlds" case from the original
problem statement: one user-facing scalar choice, one derived
object, clean access in templates.

### Input via `--var-file`

Vars files (DESIGN-0005) need no syntax additions — HCL natively
supports nested values:

```hcl
# project.forge-vars.hcl
project_name = "mockta"
exposed_ports = [8080, 9090]
git_provider = {
  repo_type   = "github"
  repo_url    = "github.com"
  project_org = "donaldgifford"
}
build_targets = {
  linux  = "amd64"
  darwin = "arm64"
}
```

This is the primary intended input path for non-scalar values.

### Input via `--set`

`--set` gains dotted-path syntax for **object field overrides
only**:

```sh
forge create go/ext \
  --var-file ./project.forge-vars.hcl \  # mutually exclusive with --set per DESIGN-0005
                                          # so this combo wouldn't work; see below
  --set git_provider.project_org=mockta-org
```

But wait — DESIGN-0005 makes `--var-file` and `--set` mutually
exclusive. For object overrides without a vars file, only top-level
object replacement is possible:

```sh
# Replace the whole object inline — readable but verbose:
forge create go/ext --set 'git_provider={repo_type="github",repo_url="github.com",project_org="me"}'
```

The dotted-path syntax (`--set obj.field=value`) only makes sense
inside `--var-file` composition (where overrides can be applied to a
loaded object). **Open question (OQ-1 below):** should we revisit the
mutual-exclusion rule from DESIGN-0005 for the narrow case of
object-field overrides? Or accept that `--set` is scalar-only and
push users toward composing multiple `--var-file` invocations?

Lists and maps: no `--set` support. Vars file only. The CLI surface
for sequences/mappings is too noisy to justify.

### Prompt UX

Interactive prompting needs the most thought.

**Objects:** unfold into per-field prompts in declaration order, with
dotted prompt labels:

```text
? git_provider.repo_type [github]: ▮
? git_provider.repo_url [github.com]:
? git_provider.project_org [donaldgifford]:
```

This keeps the existing huh-based prompt flow working with no new
widget type. The dotted labels signal "this is part of a structured
variable" and the resolved value is reconstructed before being passed
to the renderer.

**Lists and maps:** do not prompt interactively. If a list/map
variable is required and not supplied via input, error with a
pointer:

```text
Error: variable "exposed_ports" (list(number)) is required but cannot be
prompted interactively. Provide it via --var-file:
    exposed_ports = [8080, 9090]
```

This is a deliberate trade-off — list/map TUI input is hard to do
well and forge's "make CI ergonomic" priority means vars files are
the better path anyway.

### Template Access

Standard HCL2 attribute and index syntax, which the renderer already
supports because `cty.Value` is the underlying value type:

```text
# In a .tmpl file (var.* namespace per RFC-0003):
host:    ${var.git_provider.repo_url}
org:     ${var.git_provider.project_org}
primary: ${var.exposed_ports[0]}
linux:   ${var.build_targets["linux"]}

# Iteration via existing HCL2 for directive:
%{ for port in var.exposed_ports ~}
- containerPort: ${port}
%{ endfor ~}

# Conditional access:
%{ if var.git_provider.repo_type == "github" ~}
.github/...
%{ endif ~}
```

The template engine already evaluates all of this — no renderer
changes required. The only adjustment is variable validation:
the strict-vars check must allow nested attribute access against
declared object types.

### Lockfile Representation

The lockfile (post-IMPL-0006, in HCL form) carries object/list/map
values natively:

```hcl
variables {
  project_name  = "mockta"
  exposed_ports = [8080, 9090]
  git_provider = {
    repo_type   = "github"
    repo_url    = "github.com"
    project_org = "donaldgifford"
  }
  build_targets = {
    linux  = "amd64"
    darwin = "arm64"
  }
}
```

`cty.Value` round-trips cleanly through `hclwrite`. `forge check`
and `forge sync` re-derive values from this representation exactly as
they do for scalars today.

If the lockfile is still YAML when this RFC ships (i.e. IMPL-0006
hasn't merged), nested types still work — YAML naturally nests
maps. But the HCL lockfile is the cleaner story.

### Validation

Validation has two layers:

**1. Type validation.** Moves from "is `Type` field in the
allow-list?" to "does the resolved `cty.Value` conform to the
declared `cty.Type`?". cty already provides this via
`cty.Convert(val, ty)`. This layer is automatic — no author opt-in
required.

**2. Custom validation via the `validation` block.** Adopts the
Terraform-style `validation { condition = ..., error_message =
... }` block on a variable. This **replaces** the legacy `choice`
type and the existing scalar-only `validate` regex field; both go
away.

```hcl
# Enum constraint (replaces type = "choice"):
variable "git_provider_kind" {
  type    = "string"
  default = "github"
  validation {
    condition     = contains(["github", "forgejo"], var.git_provider_kind)
    error_message = "git_provider_kind must be one of: github, forgejo."
  }
}

# Range check on a number:
variable "max_replicas" {
  type    = "int"
  default = 3
  validation {
    condition     = var.max_replicas >= 1 && var.max_replicas <= 100
    error_message = "max_replicas must be between 1 and 100."
  }
}

# Regex check (subsumes the old `validate` field):
variable "service_name" {
  type = "string"
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]*$", var.service_name))
    error_message = "service_name must be lowercase kebab-case."
  }
}

# Multiple validations stack:
variable "image_tag" {
  type = "string"
  validation {
    condition     = length(var.image_tag) <= 128
    error_message = "image_tag must be at most 128 characters."
  }
  validation {
    condition     = can(regex("^[a-zA-Z0-9._-]+$", var.image_tag))
    error_message = "image_tag may only contain alphanumerics, dots, underscores, and hyphens."
  }
}
```

The block's `condition` is an HCL2 expression that must evaluate to
a `bool`. `error_message` is a static string surfaced verbatim when
the condition fails (no interpolation in v1 — keep the error
surface predictable).

**Validation inside object fields.** Because the validation block
lives on a variable (not inside the type expression), object fields
get enum-style constraints by referencing the field through the
condition:

```hcl
variable "git_provider" {
  type = object({
    kind = string
    url  = string
  })
  validation {
    condition     = contains(["github", "forgejo"], var.git_provider.kind)
    error_message = "git_provider.kind must be one of: github, forgejo."
  }
}
```

This is the architectural payoff of the choice→validation
reframing: enum constraints compose into any value path without
needing a special `choice` type to thread through cty's type system.

**Prompt UX impact.** The legacy `choice` type provided an
automatic select-list prompt. Under the validation model, prompts
default to free-text entry. Authors who want a select list declare
it via a separate, optional `prompt` block (deferred to a
follow-up RFC) — or, for the v0.7 release, accept free-text input
with validation rejecting bad values. Documented as a known
trade-off; the cleaner type system is worth the temporary UX
regression for the enum-constrained string case.

**Lists and maps.** Validation works element-wise via standard cty
functions (`alltrue([for x in var.list : ...])`). Same pattern as
Terraform.

## Alternatives Considered

- **Stay scalars-only forever.** Honest option. Authors who hit
  duplication pain write multi-variable derived-default fanouts and
  live with it. Pro: zero churn. Con: pain scales linearly with
  registry size; the renovate-config use case in the wild is a
  three-blueprint problem today, plausibly a ten-blueprint problem
  in 18 months.

- **JSON-blob-in-string convention.** Authors stuff structured data
  into a single string variable (`git_provider = '{"kind":"github",...}'`)
  and parse with a template function. Pro: no schema changes. Con:
  no type safety, no validation, ugly template access, hidden from
  schema introspection (`forge info`).

- **Registry-level shared variable definitions.** Define common
  variables once at the registry level, inherited by all blueprints.
  Solves duplication without adding types. Pro: keeps the type system
  simple. Con: introduces implicit inheritance that contradicts the
  current "every blueprint is self-describing" property. Composition
  via explicit imports (a related-but-different feature) is the
  better long-term answer for sharing, and is orthogonal to type
  expressiveness.

- **Adopt Terraform's full type system verbatim.** Includes
  `tuple([...])`, `set(T)`, optional object fields, type coercion
  semantics. Pro: well-understood; existing cty machinery supports
  it. Con: significant scope creep for the actual problem; can be
  added incrementally if real demand emerges.

## Implementation Phases

### Phase 1: Core type-expression support

- Parse `type = …` as an HCL expression, not a string scalar.
- Implement `parseVariableType(expr hcl.Expression) (cty.Type, error)`
  covering: string, bool, number/int, object, list, map. **No
  `choice` support** — removed per the Validation reframing.
- Update `validVariableTypes` validation to delegate to the new
  parser.
- Remove the legacy `choices = [...]` field and the scalar-only
  `validate` regex field from the variable schema.
- Schema-level tests against fixtures with each type form.

### Phase 2: Validation block

- Add `validation { condition = ..., error_message = ... }` block
  to the variable schema.
- Allow multiple validation blocks per variable; all must pass.
- Evaluate `condition` after type validation, against the resolved
  variable scope. Failure surfaces `error_message` verbatim with
  variable name and file/line/column of the failed block.
- Reject the legacy `choices = [...]` and scalar-only `validate`
  fields at load time (per ADR-0002, no in-tool migration —
  authors re-declare under the new form).

### Phase 3: Default value evaluation for non-scalars

- Extend `renderDefault` to handle expressions whose result is
  not a string scalar (objects, lists, maps).
- Type-check the default against the declared type at load time;
  surface mismatches with file/line/column.
- Update prompt-time default rendering to handle the new shapes.

### Phase 4: Input — vars-file and --set

- `--var-file`: nested HCL parses naturally; no work needed beyond
  what DESIGN-0005 already does. Verify with fixture tests.
- `--set`: scalar-only (per OQ-1 decision). Confirm error messages
  for object/list/map values via `--set` are clear.

### Phase 5: Prompt UX

- Object-unfolding: per-field prompts with dotted labels.
- List/map non-interactivity: clean error with vars-file pointer
  when required-and-unsupplied.
- Update charmbracelet/huh integration; preserve dry-run /
  `--defaults` semantics.
- **Known regression:** the legacy `choice` type's automatic
  select-list prompt is no longer available — enum-constrained
  strings default to free-text input + validation. Documented in
  the release notes; addressed in a follow-up RFC for an optional
  `prompt { kind = "select" }` block.

### Phase 6: Lockfile round-trip

- Verify cty.Value emission/parse through the HCL lockfile
  (IMPL-0006 must have shipped for the full story).
- Migration of existing fixtures.

### Phase 7: Documentation

- DESIGN-0001 gains object/list/map sections (with the
  renovate-config example) and a Validation block section.
- README updated; release notes flag the breaking changes
  (no `choice`, no `choices`, no scalar-only `validate`).
- `docs/MIGRATION.md` v0.6.x→v0.7.x section documents the
  re-declaration steps for blueprints using `choice` / `choices`
  / scalar `validate`.

## Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Prompt UX for objects feels clunky | Medium | Medium | Dotted labels are clear, the existing prompt flow handles per-field state well. Worst case, document the limitation and steer object-heavy blueprints toward `--var-file`. |
| List/map non-interactivity surprises users | Low | Medium | Loud error with a copy-pasteable vars-file snippet; document in DESIGN-0001 and MIGRATION.md. |
| Removal of `choice` breaks existing blueprints | High | High | Per ADR-0002, no in-tool migration. MIGRATION.md walks authors through the re-declaration pattern (single-line `validation { condition = contains([...], var.X) }`); load-time error on the legacy `choices = [...]` field points at MIGRATION.md. |
| Loss of `choice`'s select-list prompt is a real UX regression | Medium | High | Documented as a known v0.7 regression. Follow-up RFC adds an optional `prompt { kind = "select", options = [...] }` block to re-introduce select-list UX without coupling it to the type system. |
| cty type coercion produces confusing errors | Medium | Medium | Wrap cty errors with field path + source location before surfacing. |
| Lockfile-loaded objects don't round-trip cleanly on older forge versions | Medium | Low | Requires IMPL-0006 (HCL lockfile) for the full story. RFC sequencing makes this clear. |
| Scope creep — users want tuples, sets, optional fields, etc. | Medium | High | Stick to the documented subset for v1; add later types behind separate proposals. |

## Success Criteria

- An author can declare `variable "git_provider" { type = object({...}) }`
  and use `${var.git_provider.repo_type}` in templates.
- The renovate-config use case from the problem statement collapses
  from 4 variables to 1.
- An author can express an enum constraint inside an object field
  via the `validation { condition = contains([...], var.X.field) }`
  pattern.
- A user with a `.forge-vars.hcl` containing nested objects can
  scaffold without writing any `--set` flags.
- `forge create` in interactive mode prompts cleanly for object
  fields, errors helpfully for list/map fields without input.
- Legacy `type = "string"` / `bool` / `int` declarations continue
  to work unchanged. Legacy `choice` / `choices = [...]` /
  scalar-only `validate` declarations produce a load-time error
  pointing at MIGRATION.md.

## Open Questions

- **OQ-1: Revisit `--var-file` + `--set` mutual exclusion?**
  DESIGN-0005 makes them mutually exclusive to avoid precedence
  rules. But dotted-path `--set` for object field overrides on top
  of a vars file is a natural pattern — "load my base config, tweak
  one field." Three options:
  1. Hold the line — `--set` stays scalar, vars-file-only for
     objects; users compose multiple `--var-file`s with HCL
     overrides.
  2. Relax mutual exclusion narrowly for dotted-path `--set`
     overrides against a `--var-file`-loaded base.
  3. Add an explicit `--set-override path=val` flag that's
     vars-file-only and clearly distinct from `--set`.
  **Decision: option 1.** Keep mutual exclusion explicit and
  separate for now; users compose with multiple `--var-file`s
  (HCL override files) when they want layered values. Revisit only
  if the override pattern surfaces as real friction.

- **OQ-2: Should object types support a `description` per field?**
  Today only top-level variables have descriptions. Per-field
  descriptions inside objects would improve `forge info` output but
  add schema surface. **Decision: no.** Top-level descriptions
  only; field-level descriptions can be revisited if `forge info`
  becomes a primary discovery surface.

- **OQ-3: Should `choices` work inside object fields?**
  **Resolved: drop `choice` entirely; replace with a
  Terraform-style `validation { condition = ..., error_message = ... }`
  block.** Specified in [Validation](#validation) above. Per ADR-0002,
  no in-tool migration — authors re-declare. The reframing payoff:
  the constraint composes into object fields naturally because it
  lives on the variable's validation block rather than inside the
  `cty.Type` expression.

- **OQ-4: Versioning.** RFC-0002 introduces additive type-system
  capabilities (object, list, map) but the OQ-3 decision to drop
  `choice` and the `choices = [...]` / scalar `validate` fields is
  breaking — load-time errors on legacy declarations, no in-tool
  migrator per ADR-0002. **Decision: minor bump with a documented
  rescaffold/re-author path.** Probably v0.7.0 alongside RFC-0003,
  per the sequencing DESIGN-0005 → v0.5.0, IMPL-0006/IMPL-0007 →
  v0.5.0, RFC-0002/RFC-0003 → v0.7.0; concrete number set when the
  work lands.

- **OQ-5: Do we need a `locals` equivalent?** forge today expresses
  "derived value computed from other variables" by declaring a
  variable with a computed default and never prompting for it. That
  works but conflates two concepts (user input vs derived value).
  Terraform's answer is a `locals { ... }` block. The original
  forge intent was that *tool-provided* primitives (like `choice`)
  would cover the common derived-value cases without needing a
  generic locals mechanism. Two threads to resolve here:
  1. **Type coverage:** support the full set of primitive (string,
     bool, number) and complex (object, list, map) HCL types. Any
     gap is a real limitation. **Decided: yes — the type-system
     section of this RFC already covers all of this.**
  2. **Should we add a `locals { ... }` block?** **Decision: yes,
     tracked separately as [RFC-0003 — Locals for derived
     values](0003-locals-for-derived-values.md).** With the
     OQ-3 reframing of choice as validation-on-string, the
     "tool-provided locals" pattern weakens — `choice` no longer
     plays the locals role, so authors need a first-class home for
     derived values. RFC-0003 composes with RFC-0002 (locals can
     reference any variable type) but ships as its own proposal so
     each RFC stays focused.

## References

- [ADR-0002 — Forge does not ship in-tool migrators](../adr/0002-forge-does-not-ship-in-tool-migrators.md) — establishes the "no migrator" principle that governs the `choice` removal in this RFC.
- [DESIGN-0001 — Blueprint Authoring](../design/0001-blueprint-authoring.md) — variable declaration schema this RFC extends.
- [DESIGN-0005 — Variable input via vars file](../design/0005-variable-input-via-vars-file.md) — input mechanism that unblocks non-scalar values on the CLI side.
- [IMPL-0006 — Migrate lockfile from YAML to HCL](../impl/0006-migrate-lockfile-from-yaml-to-hcl.md) — lockfile format that nesting depends on for clean round-trips.
- [RFC-0003 — Locals for derived values](0003-locals-for-derived-values.md) — companion proposal for derived-value separation; co-evolves with this RFC.
- [Terraform `validation` block](https://developer.hashicorp.com/terraform/language/values/variables#custom-validation-rules) — direct prior art for the syntax and semantics adopted here.
- [`zclconf/go-cty` type expressions](https://github.com/zclconf/go-cty/blob/main/docs/types.md) — type system this RFC adopts a subset of.
- [Terraform variable type constraints](https://developer.hashicorp.com/terraform/language/values/variables#type-constraints) — direct prior art for the type expression grammar.
- `internal/config/blueprint.go` — Variable struct that gains the parsed type.
- `internal/config/hcldec_spec.go` — variable block schema to extend.
- `internal/prompt/prompt.go` — prompt flow that gains object unfolding.
- `internal/lockfile/cty.go` — value coercion machinery already in place.
