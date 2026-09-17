---
id: DESIGN-0006
title: "object and collection variable types"
status: Draft
author: Donald Gifford
created: 2026-06-29
---
<!-- markdownlint-disable-file MD025 MD041 -->

# DESIGN 0006: object and collection variable types

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-06-29

<!--toc:start-->
- [Overview](#overview)
- [Goals and Non-Goals](#goals-and-non-goals)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Background](#background)
  - [Current state](#current-state)
  - [Why now](#why-now)
- [Detailed Design](#detailed-design)
  - [Type system](#type-system)
  - [Declaration syntax](#declaration-syntax)
  - [Type expression parser](#type-expression-parser)
  - [Default value evaluation](#default-value-evaluation)
  - [Validation block (`choice` replacement)](#validation-block-choice-replacement)
  - [Input via `--var-file`](#input-via---var-file)
  - [Input via `--set`](#input-via---set)
  - [Prompt UX](#prompt-ux)
  - [Template access](#template-access)
  - [Lockfile representation](#lockfile-representation)
  - [Error surfacing](#error-surfacing)
- [API / Interface Changes](#api--interface-changes)
  - [Blueprint schema (HCL)](#blueprint-schema-hcl)
  - [Go types](#go-types)
  - [CLI](#cli)
- [Data Model](#data-model)
- [Testing Strategy](#testing-strategy)
- [Migration / Rollout Plan](#migration--rollout-plan)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Overview

Lift forge's variable type model from the current four-string-tag enum
(`string`, `bool`, `int`, `choice`) to a subset of cty's type
expression grammar — adding `object({…})`, `list(T)`, and `map(T)` —
and replace the bespoke `choice` type with a Terraform-style
`validation { condition = …, error_message = … }` block. Authors group
related fields under a single structured variable; templates access
nested values via standard HCL2 attribute syntax; vars files carry
nested values natively. Implements [RFC-0002](../rfc/0002-object-and-collection-variable-types.md).

## Goals and Non-Goals

### Goals

- Add `object({…})`, `list(T)`, and `map(T)` to the variable type
  surface alongside the existing `string`/`bool`/`int` scalars.
- Replace `type = "choice"` + `choices = [...]` and the scalar-only
  `validate = "regex"` field with a Terraform-style
  `validation { condition = …, error_message = … }` block. One
  validation mechanism, composes into object fields naturally.
- Keep the change additive on the value-flow side: templates, lockfile,
  and `forge sync`/`check` continue to operate on `cty.Value` without
  new code paths.
- Preserve "make CI ergonomic" — vars-file is the primary input path
  for non-scalar values; the prompt UX degrades gracefully (objects
  unfold to per-field prompts; lists/maps error with a copy-pasteable
  vars-file pointer when required-and-unsupplied).
- Surface type-mismatch errors with HCL file:line:col so authors and
  users see the source of the problem.

### Non-Goals

- `tuple([…])` (heterogeneous fixed-length), `set(T)` (deduplicated),
  and optional object fields. Add later behind separate proposals if
  real demand surfaces.
- `null` values. Every declared variable must have a non-null resolved
  value, either from input or default.
- An interactive TUI widget for list/map input. Deliberate trade-off:
  TUI sequences are hard to do well and vars files are the better path
  for non-scalar input anyway.
- A first-class `prompt { kind = "select", options = [...] }` block to
  re-introduce the legacy `choice` type's select-list UX. The
  validation block replaces `choice`'s constraint role; the lost
  select-list UX is documented as a known v0.7 regression and tracked
  for a follow-up RFC.
- An in-tool migrator for legacy `choice` / `choices` / scalar-only
  `validate` declarations. Per ADR-0002, authors re-declare under the
  new form when adopting v0.7.
- `locals { … }` for derived values — sibling concern, tracked in
  [RFC-0003](../rfc/0003-locals-for-derived-values.md). Composes with
  this DESIGN but ships independently.

## Background

### Current state

forge variables are flat scalars declared in `blueprint.hcl`:

```hcl
variable "project_name" {
  type     = "string"
  required = true
  validate = "^[a-z][a-z0-9-]*$"
}

variable "license" {
  type    = "choice"
  choices = ["Apache-2.0", "MIT", "BSD-3-Clause"]
  default = "Apache-2.0"
}
```

The Go struct (`internal/config/blueprint.go`) and HCL schema
(`internal/config/hcldec_spec.go`) both hard-code this surface:

```go
type Variable struct {
    Name        string
    Description string
    Type        string   // ← string tag from the allow-list
    Default     string
    Required    bool
    Validate    string   // ← scalar-only regex
    Choices     []string // ← lives parallel to Type
}
```

Validation (`internal/config/validate.go`) checks `Type` against a
four-element allow-list (`validVariableTypes`) and gates `Choices`
required-ness when `Type == "choice"`.

Resolved values already flow as `cty.Value` from `internal/prompt`
through `internal/lockfile` and `internal/template` — the
on-the-wire type surface is the constrained part; the in-memory value
plumbing is already nested-value-ready.

`internal/varsfile` (post-IMPL-0008) parses `.forge-vars.hcl`
attributes and coerces them against `Variable.Type` via
`cty/convert`. The parser today bails on non-scalar values; the
package was deliberately structured so RFC-0002's types drop in
without restructuring.

### Why now

Three signals converging:

1. **Real registry pain.** The forge-registry renovate-config use case
   needs four derived-default variables (`git_provider_host`,
   `project_org`, `renovate_config_repo`, …) computed from one user
   choice. The same pattern repeats per blueprint. The variable
   surface is mostly noise; the logical concept is splintered.

2. **`choice` is a half-feature.** It encodes one specific kind of
   string constraint but blocks composition — there's no way to put
   a `choice` constraint on a field inside an object. Replacing it
   with a generic validation block unblocks both object fields *and*
   regex / range / length constraints that `validate` covered
   awkwardly.

3. **The value pipeline is already cty-native.** `cty.Value`
   round-trips cleanly through `hclwrite`, the template engine, and
   the prompt resolver. The constraint that turns this into a
   feature is the *type-declaration* surface — everything downstream
   is essentially free.

## Detailed Design

### Type system

A subset of cty's type expression grammar. The full grammar is more
than forge needs; we limit it to the forms that compose without
surprising authors:

| Form | Example | cty equivalent |
|------|---------|----------------|
| `string` | `type = string` | `cty.String` |
| `bool` | `type = bool` | `cty.Bool` |
| `number` (alias `int`, deprecated) | `type = number` (canonical) or `type = int` (deprecated; warns at load time) | `cty.Number` |
| `list(T)` | `type = list(string)` | `cty.List(T)` |
| `map(T)` | `type = map(string)` | `cty.Map(T)` |
| `object({k = T, …})` | `type = object({port = number, host = string})` | `cty.Object(map[string]cty.Type{…})` |

> **Why no `tuple`/`set`/`optional`?** Smaller surface = fewer edge
> cases in prompts, validation, and lockfile round-trip. Nothing in
> the current registry corpus needs them. Add later if a real use
> case appears — the type parser is structured to grow.

**Backwards compatibility shim.** The existing quoted-string forms
(`type = "string"`, `type = "bool"`, `type = "int"`) continue to parse
during the v0.7 transition window. The unquoted bareword forms are
the new canonical syntax; quoted forms are accepted but the docs
guide authors toward unquoted. `type = "choice"` is **not** accepted
— it triggers a load-time error pointing at the validation-block
migration pattern in MIGRATION.md.

**`int` deprecation warning.** Both `type = int` and the legacy
`type = "int"` continue to resolve to `cty.Number` (per
[OQ-6](#open-questions)) but `LoadBlueprint` emits a one-line
deprecation warning naming the variable and source location, with a
pointer to the canonical `type = number` form. The warning fires
once per declaration. A future release may promote it to a
load-time error — this DESIGN does not commit to when.

> **Type tags vs type expressions.** The legacy schema treated `type`
> as a string literal; the new schema treats `type` as an HCL
> expression captured at load time. The parser inspects the
> expression shape and resolves it to a `cty.Type`. This mirrors how
> `Condition.When` is already captured as an `hcl.Expression` at
> load time with the source kept on `WhenSource` for round-tripping.

### Declaration syntax

```hcl
# Scalar (unchanged on the value side; type form is now an expression)
variable "project_name" {
  description = "Service name"
  type        = string
  required    = true
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]*$", var.project_name))
    error_message = "project_name must be lowercase kebab-case."
  }
}

# Object — required, no default
variable "auth_provider" {
  description = "OIDC issuer configuration"
  type = object({
    kind   = string
    issuer = string
  })
  required = true
}

# Object — derived default from another variable
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
  default = {
    linux  = "amd64"
    darwin = "arm64"
  }
}
```

Required-ness applies uniformly: `required = true` with no `default`
means the user must supply a value (via vars-file, `--set` for
scalars/objects, or prompt for scalars/objects). Required list/map
without input is a clean error (see [Prompt UX](#prompt-ux)).

### Type expression parser

A new internal helper:

```go
// parseVariableType resolves the variable's `type` HCL expression to
// a cty.Type. Accepts bareword scalars (string, bool, number, int),
// the legacy quoted-string forms during the v0.7 transition, and the
// cty constructor forms (object({...}), list(T), map(T)).
func parseVariableType(expr hcl.Expression) (cty.Type, error)
```

Lives in `internal/config/vartype.go` (new file). The implementation
uses cty's `typeexpr` package — it already supports the exact subset
forge wants, including the quoted-string scalar fallback for
backwards compatibility:

```go
import "github.com/hashicorp/hcl/v2/ext/typeexpr"

ty, diags := typeexpr.Type(expr)
```

`typeexpr` returns descriptive errors for `tuple`, `set`, and
optional fields — wrap those with a forge-specific "this type is not
supported in forge variables" message so users don't dead-end on
cty-flavoured diagnostics.

The parser runs at `LoadBlueprint` time so type errors surface with
file:line:col immediately, not on first prompt or first render.

### Default value evaluation

Defaults move from "string captured raw, re-rendered through the
template engine" to "HCL expression captured at load time, evaluated
against the variable scope when the variable is resolved." This
mirrors how `Condition.When` already works.

The new flow:

1. At load time: capture `default` as `hcl.Expression`; keep
   `WhenSource`-equivalent (`DefaultSource`) for diagnostics and the
   lockfile snapshot.
2. At resolve time: evaluate the expression against the already-bound
   `var.*` scope, producing a `cty.Value`.
3. Type-check the result against the declared `cty.Type` via
   `cty.Convert`. Mismatches surface with file:line:col of the
   `default` expression.

This change subsumes the current `renderDefault` call in
`internal/prompt`; the existing path stays as a fallback for the
backwards-compatibility shim during v0.7.

### Validation block (`choice` replacement)

A repeatable nested block on `variable`:

```hcl
variable "git_provider_kind" {
  type    = string
  default = "github"
  validation {
    condition     = contains(["github", "forgejo"], var.git_provider_kind)
    error_message = "git_provider_kind must be one of: github, forgejo."
  }
}
```

| Attribute | Type | Required | Notes |
|---|---|---|---|
| `condition` | `hcl.Expression` (bool) | yes | Evaluated against the resolved variable scope. Must return `bool`. |
| `error_message` | string | yes | Surfaced verbatim on failure. **No interpolation in v1** — keeps the error surface predictable. |

Multiple validation blocks per variable stack; all must pass. The
condition expression is captured at load time (parse-time syntax
errors surface immediately); evaluated at resolve time against the
variable scope.

**Object-field constraints** compose cleanly because the validation
lives on the variable, not inside the type expression:

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

**List/map constraints** work element-wise via standard cty
functions:

```hcl
variable "exposed_ports" {
  type = list(number)
  validation {
    condition     = alltrue([for p in var.exposed_ports : p > 0 && p < 65536])
    error_message = "exposed_ports must all be valid TCP port numbers."
  }
}
```

**Removed fields.** Both `choices = [...]` and the scalar-only
`validate = "regex"` are rejected at load time with an error
pointing at the new validation-block pattern in MIGRATION.md. No
in-tool migrator (ADR-0002).

### Input via `--var-file`

No syntax additions required — HCL natively supports nested values:

```hcl
# project.forge-vars.hcl
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
```

The vars-file parser (`internal/varsfile`) already loads attribute
values as `cty.Value`. Today it bails on non-scalar values during
coercion; with this DESIGN it delegates to `cty.Convert` against the
declared type, which handles structured values transparently. The
strict-literal posture (no function calls, no traversals) stays —
the legality of the value itself is unchanged.

This is the primary intended input path for non-scalar values.

### Input via `--set`

`--set` semantics:

| Variable type | `--set` behaviour |
|---|---|
| `string`, `bool`, `int`, `number` | Unchanged. `--set port=8080`. |
| `object({…})` | Top-level replacement only via HCL literal: `--set 'git_provider={repo_type="github",...}'`. **No dotted-path field overrides** in v1 — they only make sense composed with `--var-file`, which DESIGN-0005 explicitly forbids on the same invocation. |
| `list(T)` | Not supported. Error: `"--set on variable X (list(...)) is not supported; use --var-file"`. |
| `map(T)` | Not supported. Same error. |

Per RFC-0002 OQ-1 (resolved: option 1), the `--var-file` + `--set`
mutex from DESIGN-0005 stays intact. Users who want layered values
compose multiple `--var-file` invocations.

### Prompt UX

`internal/prompt` extends the existing charmbracelet/huh-based flow:

**Scalars** — unchanged. Same widget per type.

**Objects** — unfold into per-field prompts in declaration order:

```text
? git_provider.repo_type [github]: ▮
? git_provider.repo_url [github.com]:
? git_provider.project_org [donaldgifford]:
```

Per-field defaults come from the resolved object default (so the
ternary-driven default pattern under [Declaration syntax](#declaration-syntax)
works as expected). The dotted prompt label is purely cosmetic — the
resolved value is reconstructed as a single `cty.Value` before being
passed to the renderer. No new widget type required.

Nested objects unfold recursively: `git_provider.endpoints.primary`.
Lists/maps inside an object follow the same non-interactive rule as
top-level lists/maps (see below).

**Lists and maps** — non-interactive. If required and unsupplied,
fail with a copy-pasteable vars-file pointer:

```text
Error: variable "exposed_ports" (list(number)) is required but cannot
be supplied interactively.

Provide it via --var-file:

    # project.forge-vars.hcl
    exposed_ports = [8080, 9090]

    forge create ... --var-file ./project.forge-vars.hcl
```

If a list/map has a `default` and the user hasn't overridden it, the
default flows through silently — same as a scalar with a default.

**Lost UX: the legacy `choice` select-list.** Documented v0.7
regression. Enum-constrained strings default to free-text input plus
validation rejecting bad values. Follow-up RFC tracks adding an
optional `prompt { kind = "select", options = [...] }` block to
re-introduce the select-list UX without coupling it to the type
system.

### Template access

Standard HCL2 attribute and index syntax — the renderer already
evaluates this because `cty.Value` is the underlying value type:

```text
# In a .tmpl file (var.* namespace per RFC-0003 — see Open Questions):
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

**Namespace open question.** RFC-0003 introduces the `var.*`
namespace for variable references. This DESIGN assumes that
namespace lands first (or co-lands). If RFC-0003 slips, templates
keep using bare-name references and this DESIGN's examples flip
back. See [Open Questions OQ-1](#open-questions) for the sequencing
choice.

The only renderer adjustment is the strict-vars check —
`internal/template` validates that template-referenced variables are
declared in `blueprint.hcl`. The new check allows nested attribute
access against declared object types: `var.git_provider.repo_type`
is legal iff `git_provider` is declared as
`object({repo_type = …, …})`.

### Lockfile representation

The HCL lockfile (post-IMPL-0006) carries nested values natively:

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

`cty.Value` round-trips through `hclwrite` cleanly. `internal/lockfile/cty.go`
already houses the `ToCtyValues`/`FromCtyValues` pair that this DESIGN
extends — the existing scalar branches stay; object/list/map branches
delegate to `cty/convert` directly. The variable-type-aware coercion
already in `ToCtyValues` is the natural home for the new shapes.

`forge check` and `forge sync` re-derive values from the lockfile
exactly as they do for scalars today. The drift-detection hash logic
is unchanged — it hashes rendered file content, not variable values.

### Error surfacing

Three new error classes; all carry HCL `Range` info:

| Error class | Surfaces at | Message shape |
|---|---|---|
| Type-expression parse error | `LoadBlueprint` | `blueprint.hcl:L:C: invalid type expression for variable "X": <reason>` |
| Default-value type mismatch | `LoadBlueprint` | `blueprint.hcl:L:C: default for variable "X" does not match declared type: expected <T>, got <U>` |
| Validation-condition failure | resolve time | `<error_message> (variable "X", blueprint.hcl:L:C)` |
| `type = int` deprecation (warning, not error) | `LoadBlueprint` | `blueprint.hcl:L:C: variable "X": type = int is deprecated; use type = number` |

Coercion errors from vars files keep the existing format
(`vars file PATH:L:C: variable "X" expects T, got U`).

## API / Interface Changes

### Blueprint schema (HCL)

| Field | Today | After this DESIGN |
|---|---|---|
| `variable.type` | string scalar from `{string, bool, int, choice}` | HCL expression resolving to a `cty.Type` from `{string, bool, number, int, list(T), map(T), object({…})}`. Quoted-string scalars accepted during transition. |
| `variable.choices` | required when `type = "choice"` | **removed** — load-time error pointing at MIGRATION.md |
| `variable.validate` | optional regex string | **removed** — load-time error pointing at MIGRATION.md |
| `variable.default` | string captured raw | HCL expression evaluated against the variable scope at resolve time |
| `validation { condition, error_message }` | does not exist | **new** repeatable block on `variable` |

### Go types

```go
// internal/config/blueprint.go
type Variable struct {
    Name          string
    Description   string
    Type          cty.Type        // ← was string
    TypeSource    string          // raw source for diagnostics
    DefaultExpr   hcl.Expression  // ← was Default string
    DefaultSource string          // raw source for diagnostics
    Required      bool
    Validations   []Validation    // ← new

    // Removed: Validate, Choices
}

type Validation struct {
    Condition     hcl.Expression
    ErrorMessage  string
    DefRange      hcl.Range // for error attribution
}
```

The struct-comment migration story: a one-line comment explains
that the type/default expression-capture mirrors `Condition.When`,
and points readers at `parseVariableType` in
`internal/config/vartype.go`.

### CLI

No new flags. Existing flags get extended error messages:

- `--set` rejects list/map values with the message in
  [Input via `--set`](#input-via---set).
- `--var-file` coercion errors now surface object/list/map type
  mismatches via the existing
  `vars file PATH:L:C: variable "X" expects T, got U` format.

## Data Model

| Surface | Storage | Shape |
|---|---|---|
| Blueprint variable type | in-memory `cty.Type` (from `typeexpr.Type`) | matches cty's `Object`/`List`/`Map` constructors |
| Blueprint variable default | in-memory `hcl.Expression` (lazy-evaluated) | source preserved in `DefaultSource` |
| Resolved variable value | `cty.Value` end-to-end (already true today) | passes through `ToCtyValues` → lockfile → template renderer |
| Lockfile `variables { … }` block | HCL native nested attributes | `hclwrite` round-trips object/list/map without escaping |
| `.forge-vars.hcl` input | HCL native nested attributes | parser unchanged; coercion delegates to `cty.Convert` |

No new on-disk format. No new lockfile schema. The "no schema
change" property is the architectural payoff of having a
`cty.Value`-native value pipeline.

## Testing Strategy

| Layer | Coverage |
|---|---|
| Type expression parser | Table-driven: each accepted form (`string`, `bool`, `number`, `int`, `list(T)`, `map(T)`, `object({…})`, nested object, quoted-string legacy forms). Each rejected form (`tuple`, `set`, `optional`, `"choice"`) with the expected error pointing at MIGRATION.md. |
| Default expression evaluation | Per type: literal defaults, derived-from-another-variable defaults, type-mismatched defaults (expects clean error with file:line:col). |
| Validation block | Single block, multiple stacked blocks, validation against an object field, validation against a list element via `alltrue([for …])`, validation failure surfaces `error_message` verbatim. |
| Removed-field rejection | `choices = [...]` and `validate = "regex"` both produce load-time errors pointing at MIGRATION.md. |
| `internal/varsfile` integration | Vars files supplying object/list/map values; coercion against the declared `cty.Type`; type-mismatch errors with `vars file PATH:L:C` location. |
| `--set` integration | Top-level object replacement via HCL literal; list/map values produce the documented error message. |
| Prompt UX | Object unfold (per-field prompts), nested-object unfold, required-list-with-no-input error with copy-pasteable vars-file snippet, default-flowing list (no prompt). |
| Lockfile round-trip | Write → load cycle on a project with object + list + map + nested-object variables; assert byte-identity. |
| Template strict-vars | `var.obj.field` legal iff field is declared; `var.list[0]` legal iff `list` is declared as `list(T)`; bad references error with file:line:col. |
| End-to-end | Integration test scaffolding the forge-registry renovate-config use case (one `git_provider` object variable replacing four scalar variables); assert the rendered output matches a fixture. |
| Cross-RFC composition | If RFC-0003 (locals) lands first, integration test combining a local that references an object variable's field. |

Test corpus lives in `internal/config/testdata/object-types/` and
`internal/varsfile/testdata/object-types/`; both directories follow
the per-fixture pattern already established by IMPL-0008.

## Migration / Rollout Plan

**Release line.** Minor bump — v0.7.0 per RFC-0002 OQ-4. Bundled
with RFC-0003 (locals) if both land in the same window. Out of band
of any other format change (the v0.5/v0.6 release line covered HCL
format consolidation and vars-file).

**Breaking changes for blueprint authors.**

1. `type = "choice"` → re-declare as `type = string` plus a
   `validation { condition = contains([...], var.X) }` block.
2. `choices = [...]` field → **removed**. Carried by the validation
   block above.
3. `validate = "regex"` field → re-declare as
   `validation { condition = can(regex("...", var.X)) }`.
4. `type = "string"` / `type = "bool"` **continue to work
   silently** during the v0.7 transition (quoted-string fallback in
   the parser). Authors are guided toward the bareword forms in
   docs but the load-time validator does not warn on the quoted
   forms.
5. `type = int` (and the legacy `type = "int"`) continue to work
   but emit a **deprecation warning** at load time. Authors
   migrate to `type = number`. See [OQ-6](#open-questions).

**Author migration walkthrough.** A new section in
`docs/MIGRATION.md` (`## Variable type system upgrade (v0.7+)`)
covers each breaking change with a before/after snippet and a
copy-pasteable pattern.

**No project-side migration.** Existing lockfiles continue to load
unchanged — the `variables { … }` block already carries native HCL
primitives (post-IMPL-0006), and the lockfile loader has no opinion
on whether a variable's declared type is a scalar or a structured
type. A scaffolded project from a pre-v0.7 blueprint stays valid;
re-running `forge sync` after a blueprint adopts new structured
types may surface "variable X newly declared with no default" if
the author also marked it required, but that's a content change,
not a format change.

**Project consumer impact.** None for projects that don't update
their source blueprint. Projects whose source blueprint adopts
v0.7 features see the changes the next time they run
`forge sync`.

**Sequencing.** Three phases for the implementation IMPL doc to
break out, kept abstract here:

1. Type expression + default expression + validation block (the
   schema-side change).
2. Vars-file / `--set` / prompt UX integration (the input-side
   change).
3. Documentation, migration guide, release notes.

The IMPL doc owns the concrete task breakdown and acceptance gates.

## Open Questions

- **OQ-1: Sequencing with RFC-0003 (locals / `var.*` namespace).**
  RFC-0003 introduces `var.*` for variable references in templates
  and defaults. This DESIGN's examples assume `var.*` is available.
  **Decision: option 2 — ship both in v0.7.0 simultaneously.**
  Single migration story for authors; templates and the
  validation-condition scope reference the same `var.*` namespace
  end-to-end. Coordination cost accepted. The IMPL doc owns the
  cross-stream task ordering.

- **OQ-2: Should `parseVariableType` use cty's `typeexpr` package
  directly, or reimplement?** **Decision: use `typeexpr` directly.**
  It covers the exact subset forge wants, is the same parser
  Terraform uses (matching user intuition on error messages), and
  reimplementing buys nothing except control. Wrap errors with
  forge-specific phrasing where the raw cty diagnostic would
  dead-end users (e.g. `tuple`, `set`, optional fields).

- **OQ-3: Validation-block `error_message` interpolation.**
  **Decision: static string only in v1.** Terraform allows
  `${...}` references inside `error_message`; forge does not.
  Keeps the error surface predictable and avoids the "what does
  this template see?" question. Revisit if real demand surfaces.

- **OQ-4: Should validation conditions see *other* variables in
  scope?** **Decision: yes, allow cross-variable references; the
  scope is the same as default-expression evaluation.** Adds zero
  implementation surface (the resolved-variable scope is already
  built for defaults) and unlocks "this value must agree with that
  one" constraints. Matches Terraform's semantics.

- **OQ-5: Lost `choice` select-list UX — release blocker or
  acknowledged regression?** **Decision: ship v0.7 with the UX
  regression; document it; track a follow-up RFC for
  `prompt { kind = "select", options = [...] }`.** The validation
  reframing is the architectural payoff; the prompt regression is
  a cosmetic bug we can fix in a follow-up without re-litigating
  the type system. Release notes call out the regression
  explicitly with the migration path (validation block) and a
  forward pointer to the follow-up RFC slot.

- **OQ-6: What's the policy on `type = "number"` vs `type = "int"`?**
  **Decision: treat `int` as an alias for `number` (current implicit
  behaviour) and emit a deprecation warning at `LoadBlueprint` time
  when authors use `int`.** The warning fires once per blueprint
  load, carries the variable name and source location, and points
  at the migration: `type = int → type = number`. Docs guide
  authors toward `number` as the canonical form. A future release
  (likely v0.8 or v0.9) may promote the warning to a load-time
  error; this DESIGN does not commit to that step.

## References

- [RFC-0002 — Object and collection variable types](../rfc/0002-object-and-collection-variable-types.md) — the proposal this DESIGN implements.
- [RFC-0003 — Locals for derived values](../rfc/0003-locals-for-derived-values.md) — companion proposal for derived-value separation; co-evolves with this DESIGN.
- [DESIGN-0001 — Blueprint Authoring](0001-blueprint-authoring.md) — the variable-declaration contract this DESIGN extends.
- [DESIGN-0005 — Variable input via vars file](0005-variable-input-via-vars-file.md) — input mechanism that unblocks non-scalar values on the CLI side.
- [IMPL-0006 — Migrate lockfile from YAML to HCL](../impl/0006-migrate-lockfile-from-yaml-to-hcl.md) — lockfile format that nesting depends on for clean round-trips. **Already shipped.**
- [IMPL-0008 — Variable input via vars file](../impl/0008-variable-input-via-vars-file.md) — vars-file parser, structured so this DESIGN's types drop in without restructuring.
- [ADR-0001 — Use HCL2 as the template engine](../adr/0001-use-hcl2-as-the-template-engine.md).
- [ADR-0002 — Forge does not ship in-tool migrators](../adr/0002-forge-does-not-ship-in-tool-migrators.md) — governs the `choice` / `choices` / `validate` removal.
- [Terraform `validation` block](https://developer.hashicorp.com/terraform/language/values/variables#custom-validation-rules) — direct prior art for the validation syntax and semantics adopted here.
- [Terraform variable type constraints](https://developer.hashicorp.com/terraform/language/values/variables#type-constraints) — direct prior art for the type expression grammar.
- [`zclconf/go-cty` type expressions](https://github.com/zclconf/go-cty/blob/main/docs/types.md) — type system this DESIGN adopts a subset of.
- [`hashicorp/hcl/v2/ext/typeexpr`](https://pkg.go.dev/github.com/hashicorp/hcl/v2/ext/typeexpr) — the parser this DESIGN delegates to.
- `internal/config/blueprint.go` — Variable struct that gains the parsed type.
- `internal/config/hcldec_spec.go` — variable block schema to extend.
- `internal/config/validate.go` — `validVariableTypes` map that becomes unnecessary.
- `internal/prompt/prompt.go` — prompt flow that gains object unfolding.
- `internal/lockfile/cty.go` — value coercion machinery already in place.
- `internal/varsfile/varsfile.go` — vars-file coercion that gains structured-value support.
