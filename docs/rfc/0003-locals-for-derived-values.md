---
id: RFC-0003
title: "Locals for derived values"
status: Draft
author: Donald Gifford
created: 2026-05-18
---
<!-- markdownlint-disable-file MD025 MD041 -->

# RFC 0003: Locals for derived values

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-05-18

<!--toc:start-->
- [Summary](#summary)
- [Problem Statement](#problem-statement)
- [Proposed Solution](#proposed-solution)
- [Design](#design)
  - [Block Syntax](#block-syntax)
  - [Evaluation Scope and Order](#evaluation-scope-and-order)
  - [Type Inference](#type-inference)
  - [Template Access](#template-access)
  - [Lockfile Representation](#lockfile-representation)
  - [Interaction with --var-file and --set](#interaction-with---var-file-and---set)
  - [Interaction with forge info](#interaction-with-forge-info)
  - [Interaction with forge check / forge sync](#interaction-with-forge-check--forge-sync)
  - [Conditions and Excludes](#conditions-and-excludes)
- [Migration](#migration)
- [Alternatives Considered](#alternatives-considered)
- [Implementation Phases](#implementation-phases)
  - [Phase 1: Parse and evaluate locals; switch scope to namespaced](#phase-1-parse-and-evaluate-locals-switch-scope-to-namespaced)
  - [Phase 2: Template and condition access](#phase-2-template-and-condition-access)
  - [Phase 3: Lockfile + drift detection](#phase-3-lockfile--drift-detection)
  - [Phase 4: Documentation](#phase-4-documentation)
- [Risks and Mitigations](#risks-and-mitigations)
- [Success Criteria](#success-criteria)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Summary

Add a `locals { ... }` block to `blueprint.hcl` for derived values
computed from variables. Locals are never prompted, never user-supplied,
and never appear in `--var-file` or `--set`. They give authors a clean
home for intermediate computations that today are awkwardly expressed
as "variables with a computed default that we hope no one supplies a
value for."

## Problem Statement

forge variables today serve two distinct purposes in real blueprints:

1. **User input** — the thing the author wants the user to choose
   (project name, license, git provider).
2. **Derived value** — the thing the author wants to compute from
   user inputs and use in templates (a git host URL derived from a
   provider choice, a renovate config repo path derived from an org).

Both are declared as `variable` blocks today. The pattern for case 2
looks like this:

```hcl
# Pre-RFC-0002/0003 status quo: choice type, bare-name references in
# derived defaults
variable "git_provider_kind" {
  type    = "choice"
  choices = ["github", "forgejo"]
  default = "github"
}

variable "git_provider_host" {
  type    = "string"
  default = "${git_provider_kind == \"github\" ? \"github.com\" : \"git.fartlab.dev\"}"
}

variable "project_org" {
  type    = "string"
  default = "${git_provider_kind == \"github\" ? \"donaldgifford\" : \"homelab\"}"
}

variable "renovate_config_repo" {
  type    = "string"
  default = "${git_provider_kind == \"github\" ? \"renovate-config\" : \"renovate-config\"}"
}
```

Three problems with this:

- **Schema lies.** `git_provider_host`, `project_org`, and
  `renovate_config_repo` show up in `forge info` as user-overridable
  variables even though the author intends them to be derived. A
  user passing `--set git_provider_host=example.com` silently breaks
  the cross-variable invariant the author was trying to express.
- **Prompt-skip pattern is fragile.** Authors rely on the user
  accepting the default at the interactive prompt or running with
  `--defaults`. There's no schema-level way to say "this variable
  is *always* derived, don't surface it."
- **RFC-0002 makes it worse.** RFC-0002 drops `choice` and replaces
  it with a `validation { condition = ..., error_message = ... }`
  block on a `string` variable. That removes the "tool-provided
  locals via primitives" framing — `choice` no longer plays the
  derived-value role at all. Authors will reach for derived
  defaults more often, not less, because the language gives them
  fewer in-band tools for derivation.

The forge-registry renovate-config blueprint is the canonical
real-world example: one user-facing variable (`git_provider_kind`),
three derived values, all currently masquerading as variables.

## Proposed Solution

Adopt Terraform's `locals { ... }` block, scoped down to the
single-blueprint case (no cross-file references, no module system).
Locals are attributes whose values are HCL expressions evaluated
against the resolved variable scope. They appear in templates as
`${local.foo}`, with variables correspondingly accessed as
`${var.foo}` — the namespacing is the same change applied to both
sides (see Template Access below).

```hcl
variable "git_provider_kind" {
  type    = "string"
  default = "github"
  validation {                                        # per RFC-0002
    condition     = contains(["github", "forgejo"], var.git_provider_kind)
    error_message = "git_provider_kind must be one of: github, forgejo."
  }
}

locals {
  git_provider_host    = var.git_provider_kind == "github" ? "github.com" : "git.fartlab.dev"
  project_org          = var.git_provider_kind == "github" ? "donaldgifford" : "homelab"
  renovate_config_repo = "renovate-config"
}
```

The `locals` block is:

- Author-only — no CLI surface, no prompts.
- Side-effect-free — pure expressions over variables (and other
  locals declared earlier).
- Recorded in the lockfile as resolved values, for round-trip and
  drift detection.

## Design

### Block Syntax

A single top-level `locals { ... }` block per `blueprint.hcl`.
Multiple blocks rejected at load time (avoid Terraform's "all blocks
merge silently" footgun). Each attribute inside the block is one
local.

```hcl
locals {
  service_name = "${var.project_name}-svc"
  image_tag    = "${var.project_name}:${var.version}"
  build_args   = ["--platform=linux/amd64", "--platform=linux/arm64"]
}
```

Attribute names follow the same identifier rules as variable names
(snake_case, no leading digits). Collisions between a local and a
variable name are *allowed* — namespacing disambiguates them. The
load-time check is only for duplicate locals within the `locals`
block (each name must be unique to its namespace).

### Evaluation Scope and Order

Locals are evaluated in declaration order, after all variables are
resolved. The scope visible to a local expression contains:

1. All resolved variables (the same scope visible to variable
   defaults).
2. All locals declared earlier in the same `locals` block.

Locals cannot reference themselves or later locals — forward refs
are a load-time error with file/line/column. This is strictly
weaker than Terraform's "graph-based" evaluation but avoids the
"why is this evaluating in this order?" confusion.

```hcl
locals {
  base_name    = lower(var.project_name)           # OK — references a variable
  service_name = "${local.base_name}-svc"          # OK — references earlier local
  bad_ref      = "${local.other_thing}"            # ERROR — undefined
  self_ref     = "${local.self_ref}-x"             # ERROR — self-reference
}
```

### Type Inference

Locals are not explicitly typed. The resolved `cty.Value` carries
the type derived from the expression. This is simpler than the
variable schema (which gains explicit `type = ...` in RFC-0002)
because locals are author-internal — no need for input validation.

If a local expression produces an unexpected type, template-side
errors surface naturally at render time with the existing HCL
"can't index string by string" diagnostics.

### Template Access

**Chosen: namespaced scopes — `var.foo` and `local.foo`.** Every
variable reference becomes `${var.NAME}`; every local reference
becomes `${local.NAME}`. The namespace prefix is required at every
reference site (templates, `condition.when`, variable `default`
expressions, local expressions).

```text
host:    ${local.git_provider_host}    # derived
name:    ${var.project_name}           # user input
org:     ${var.git_provider.project_org}   # nested var (RFC-0002 object type)
```

Pros:

- **Clarity at every reference site.** A template reader can tell
  user input from derived value without cross-referencing the
  blueprint schema.
- **No collision rules to learn.** Variables and locals live in
  separate namespaces, so reusing a name (`var.git_provider` and
  `local.git_provider`) is allowed and well-defined.
- **Strict-vars catches typos earlier.** A bare `${project_name}`
  is now a parse error pointing the author at `${var.project_name}`,
  instead of a runtime "unknown variable" surprise.
- **Future-proof for additional namespaces.** If forge later grows
  a `template`/`resource`-style block (declarative template-file
  output, analogous to Terraform resources), it slots in as a
  third namespace (`${template.foo}` or similar) without ambiguity
  against `var` or `local`.
- **Idiomatic with the cty/HCL2 world.** Matches Terraform's
  `var.foo` / `local.foo` / `module.foo` convention, lowering the
  cognitive load for users coming from that ecosystem.

Cons:

- **Breaking change to existing templates.** Every `${NAME}`
  reference in every blueprint template must become `${var.NAME}`
  (or `${local.NAME}` if migrated to a local). Per ADR-0002 there
  is no in-tool migrator — authors hand-port templates per the
  [Migration](#migration) section. The pin-to-v0.6.x escape hatch
  covers users who can't port immediately.
- **More keystrokes per reference.** Real cost, but tiny — and
  paid back the first time a template reader doesn't have to
  bounce to `blueprint.hcl` to find out where a name comes from.

**Rejected: flat scope.** The original framing of this RFC
proposed bare `${foo}` for both. Rejected on reflection because:

- The collision rule (load-time error on shared names) is a hack
  to paper over the fundamental ambiguity instead of fixing it.
- It locks forge out of the namespace-extension path —
  introducing `template.` or `resource.` later would force a
  second breaking migration that namespacing-now avoids.
- The "no migration friction" pro is illusory: real authors
  reading templates lose more time to "is this a var or a local?"
  ambiguity than they save by skipping a one-shot migration.

`condition.when` expressions and variable `default` expressions
follow the same rule:

```hcl
variable "git_provider_host" {
  type    = "string"
  default = var.git_provider_kind == "github" ? "github.com" : "git.fartlab.dev"
}

exclude {
  path = ".github/**"
  condition {
    when = "${var.git_provider_kind != \"github\"}"
  }
}
```

### Lockfile Representation

Locals are recorded in the lockfile alongside variables, in a
separate `locals { ... }` block, so the lockfile carries the
authoritative "what derived values were produced at create time":

```hcl
# .forge-lock.hcl (post-IMPL-0006)
variables {
  project_name      = "mockta"
  git_provider_kind = "github"
}

locals {
  git_provider_host    = "github.com"
  project_org          = "donaldgifford"
  renovate_config_repo = "renovate-config"
}
```

This serves three purposes:

- **Drift detection.** `forge check` re-evaluates locals against
  the current `blueprint.hcl` and compares against the lockfile —
  any divergence is flagged the same way variable drift is.
- **Reproducibility.** A reader of the lockfile sees the full
  computed state without needing to re-run evaluation.
- **`forge info --json` parity.** Locals show up in machine-readable
  output the same way variables do, separately labeled.

### Interaction with `--var-file` and `--set`

Locals are **never** valid keys in a vars file or a `--set`
argument. Vars files use bare attribute names (they're declaring
variable *values*, not referencing names — no namespace prefix in
the file format), and the parser rejects any key that matches a
declared local:

```text
Error: ./project.forge-vars.hcl:3,1-21: "git_provider_host" is a local,
not a variable. Locals are computed from variables and cannot be
supplied via --var-file. Edit the relevant variable instead
(git_provider_kind).
```

This is the entire point of the feature — no need for a CLI
override path, because that path is exactly what locals exist to
*replace*.

### Interaction with `forge info`

`forge info` gains a separate "Locals" section after the variables
table. Default text format groups variables (user-facing) above
locals (computed). JSON output puts them in distinct top-level
keys so consumers can ignore locals if they're only interested in
the user-input surface.

### Interaction with `forge check` / `forge sync`

`forge check` re-resolves variables from the lockfile, then
re-evaluates locals against the current `blueprint.hcl`. If a local
expression changed upstream (registry update), the recomputed value
is compared against the lockfile and surfaced as drift. This makes
locals first-class for the same upstream-change detection that
already works for variables.

`forge sync` uses the recomputed locals when re-rendering managed
files, so a registry-side change to a local expression propagates
on the next sync without requiring a re-prompt.

### Conditions and Excludes

`condition.when` expressions (file excludes in `blueprint.hcl`) can
reference locals and variables under their respective namespaces:

```hcl
exclude {
  path = ".github/**"
  condition {
    when = "${var.git_provider_kind != \"github\"}"
  }
}

# Or, cleaner, using a local:
locals {
  is_github = var.git_provider_kind == "github"
}

exclude {
  path = ".github/**"
  condition {
    when = "${!local.is_github}"
  }
}
```

The second form is the better authoring pattern — name the
condition once, reuse it across multiple excludes.

## Migration

**No in-tool migrator.** Per [ADR-0002](../adr/0002-forge-does-not-ship-in-tool-migrators.md),
forge does not ship migration commands. The namespacing change
(`${NAME}` → `${var.NAME}`) and the locals introduction are
breaking for every existing blueprint that uses bare-name
references, but the project handles this the same way it handles
every other breaking change starting with v0.5+:

1. **Authors re-author** their `blueprint.hcl` and `.tmpl` files
   against the new namespacing rules. The transformation is
   mechanical (every bare `${X}` referencing a declared variable
   becomes `${var.X}`) but is not provided as a tool — authors
   either do it by hand or write a project-local script.
2. **Users rescaffold** projects from re-authored blueprints, or
   pin forge to a pre-v0.7 release. Existing `.forge-lock.hcl`
   files from v0.5/v0.6 remain readable; what changes is how the
   *blueprint* expresses references, not how the lockfile records
   resolved values.
3. **MIGRATION.md gains a v0.6.x → v0.7.x section** documenting:
   - The exact reference-syntax change with before/after examples.
   - The `choices = [...]` → `validation { ... }` re-declaration
     pattern (per RFC-0002).
   - Optional: the derived-default-variable → local promotion
     pattern for blueprints that want to take advantage of the
     new locals block.
   - The pin-to-v0.6.x escape hatch for stragglers.
4. **Load-time errors** on bare-name references (or legacy
   `choices = [...]`) include a pointer to MIGRATION.md. The
   "rescaffold or pin" rescue path is the consistent shape.

This is a real one-time cost for blueprint authors. ADR-0002 makes
the case that across the v0.5→v0.7 release window, the cumulative
migrator complexity outweighed the migration value — a position
explicitly weighed against in-tool tooling and rejected.

## Alternatives Considered

- **Keep the derived-default-variable pattern.** Status quo. Pros:
  zero new schema surface. Cons: the schema-lies problem
  (`forge info` exposes derived values as overridable),
  prompt-skip fragility, no clean separation between input and
  derivation. Becomes worse as RFC-0002's choice-as-validation
  reframing lands.

- **A `derived = true` flag on variables.** Mark certain variables
  as "derived only" — they have a computed default, never prompt,
  reject `--set` and `--var-file` input. Pros: smaller schema
  change. Cons: still conflates two concepts under one block, still
  forces "type" and "validate" fields on values that have neither;
  the schema reads as "a variable that pretends to be a local."
  Rejected as a worse-named version of this proposal.

- **Compute locals lazily inside templates.** Use HCL's `let` or
  function-call patterns to derive values inline in `.tmpl` files.
  Pros: no `blueprint.hcl` change. Cons: derivations repeat across
  every template that needs them; no single source of truth; no
  lockfile recording; conditions in `blueprint.hcl` can't reference
  inline-derived values. Strictly worse than a `locals` block.

- **Keep the existing flat-scope template syntax and namespace
  only locals.** Variables stay bare (`${NAME}`), locals get a
  prefix (`${local.NAME}`). Pros: smaller migration (no template
  rewrite for variables). Cons: asymmetric — the reader still has
  to scan for the `local.` prefix to know what kind of value a
  reference is, and a future `template.`/`resource.` block would
  re-litigate the same decision. Rejected for the same reason
  flat scope was rejected: the asymmetry is a worse long-term
  shape than a one-time mechanical migration.

- **Allow `locals` to reference each other in any order
  (Terraform-style graph eval).** Convenience for authors,
  complexity for the implementation and for readers. Declaration
  order is strict enough for the use cases driving this RFC and
  matches the existing "variables resolved top to bottom" model.

## Implementation Phases

### Phase 1: Parse and evaluate locals; switch scope to namespaced

- Add `Locals map[string]hcl.Expression` to `internal/config/blueprint.go`.
- Extend `internal/config/hcldec_spec.go` to accept a `locals` block.
- Reject multiple `locals` blocks per file; reject duplicate local
  names within a block.
- Resolve locals after variables in `internal/create/create.go`,
  detecting forward references and self-references at evaluation
  time.
- Switch the template/condition/default evaluation context from
  flat scope to two-namespace scope: `var.*` is the resolved
  variable map, `local.*` is the resolved local map. Bare names
  become a parse error with a "use `var.NAME` instead" hint.
- Variables and locals are allowed to share names (different
  namespaces); the only collision check is duplicate locals.

### Phase 2: Template and condition access

- Update the template render scope to include both namespaces.
- Update `condition.when` evaluation to use the same context.
- Update strict-vars validation to accept `var.*` and `local.*`
  references; reject bare-name references with the "use `var.NAME`"
  hint.

### Phase 3: Lockfile + drift detection

- Add `Locals` map to the `Lockfile` struct (assumes IMPL-0006 has
  shipped or is co-developing).
- Emit locals as a separate `locals { ... }` block in the HCL
  lockfile.
- `forge check` re-evaluates locals and surfaces drift the same way
  it does for variables.
- `forge sync` uses recomputed locals on re-render.

### Phase 4: Documentation

- DESIGN-0001 gains a "Locals" section and updates every variable
  reference example to the namespaced form.
- DESIGN-0005, RFC-0002 examples updated to namespaced form
  (cross-RFC consistency).
- `forge info` example output updated.
- `docs/MIGRATION.md` gains a "v0.6.x → v0.7.x — namespaced
  variable references" section.
- README + release notes.

## Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Breaking change with no migrator — authors must hand-port templates | High | High | Per ADR-0002, accepted by design. MIGRATION.md walks through the mechanical transformation with concrete before/after examples. The pin-to-v0.6.x escape hatch covers any author who can't port immediately. forge-registry is the canonical reference for the post-cutover shape. |
| Authors hand-port templates incorrectly (forget the namespace) | Medium | High | Strict-vars rejects bare names with a clear "use `var.NAME` instead" diagnostic, file:line:col on the offending site. Same diagnostic pattern as the IMPL-0004 missing-migration error. |
| Authors over-use locals as a "programming environment" | Medium | Low | Locals are pure expressions only — no functions beyond cty stdlib, no side effects. Same constraints as variable defaults today. |
| Drift detection for locals is noisy when expressions reference moving values (e.g., timestamps) | Low | Low | Locals can only reference variables and earlier locals; no `timestamp()` or similar impure functions in scope. |
| Lockfile schema change collides with IMPL-0006 in flight | Medium | Medium | RFC-0003 explicitly sequences after IMPL-0006 (HCL lockfile). If RFC-0003 lands first by accident, fall back to recording locals in the YAML lockfile as a nested map. |

## Success Criteria

- An author can declare `locals { git_provider_host = ... }` and
  reference it from templates as `${git_provider_host}`.
- The renovate-config use case (3 derived-default variables today)
  collapses to 1 variable + 3 locals, with no `forge info` schema
  pollution.
- `forge check` flags drift for locals the same way it does for
  variables.
- A user attempting `--set git_provider_host=...` gets a clear
  "that's a local, not a variable" error.
- No regression for existing blueprints using the
  derived-default-variable pattern; they continue to work
  unchanged.

## Open Questions

- **OQ-1: Template access — flat scope or namespaced?**
  **Decision: namespaced (`var.foo` + `local.foo`).** Locked in
  Template Access above. Reasoning: clarity at every reference
  site, no collision rules to learn, future-proof for additional
  namespaces (a possible `template.`/`resource.` block for
  declarative template-file output is the motivating example),
  idiomatic with the cty/HCL2 world. Tradeoff: per ADR-0002,
  no in-tool migrator — authors hand-port (see [Migration](#migration)).
- **OQ-2: Should locals support functions like `lower()`,
  `upper()`, `join()`?** They're available in cty's stdlib, easy
  to wire. **Decision: yes.** Locals exist precisely to centralise
  these small transformations; the same cty stdlib function set
  available in variable defaults is available in local expressions.
  No new in-tool helper functions in v1 — only what cty provides.
- **OQ-3: Do locals need a `description` field for `forge info`
  output?** Top-level locals are author-internal but `forge info`
  exposes them for reproducibility. **Decision: no for v1.**
  Locals show up in `forge info` output by name + resolved value
  only; a follow-up RFC can add `description` if `forge info`
  becomes a primary discovery surface.
- **OQ-4: Should the OQ-1 namespacing change be its own RFC?**
  Namespacing every variable reference is a substantial breaking
  change that arguably stands on its own merits independent of
  locals. Folding it into RFC-0003 keeps the migration as one cut
  (rather than two), and the design forcing function is locals
  itself. **Decision: keep folded into RFC-0003.** Locals is what
  *requires* the namespace decision, so RFC-0003 is the right
  anchor. If RFC-0003 itself slips or is rejected, the namespacing
  question re-opens as its own proposal.
- **OQ-5: Versioning.** Locals are additive *to the schema* but the
  namespacing change is breaking. **Decision: minor bump.**
  Probably v0.7.0+ assuming the sequencing DESIGN-0005 → v0.5.0,
  IMPL-0006 → v0.6.0, RFC-0002/RFC-0003 → v0.7.0. Load-time
  rejection points at `docs/MIGRATION.md` (no in-tool migrator per
  ADR-0002) — the rescaffold/pin pattern established by IMPL-0006
  applies here too.

## References

- [ADR-0002 — Forge does not ship in-tool migrators](../adr/0002-forge-does-not-ship-in-tool-migrators.md) — drops the originally-planned `forge migrate refs` command; informs the [Migration](#migration) section.
- [RFC-0002 — Object and collection variable types](0002-object-and-collection-variable-types.md) — the type-system work this RFC composes with; RFC-0002 OQ-5 deferred the locals question here.
- [DESIGN-0006 — Object and collection variable types](../design/0006-object-and-collection-variable-types.md) — RFC-0002's design; co-ships with this RFC in v0.7.0 per DESIGN-0006 OQ-1.
- [DESIGN-0001 — Blueprint Authoring](../design/0001-blueprint-authoring.md) — variable declaration schema that gains the `locals` block.
- [DESIGN-0005 — Variable input via vars file](../design/0005-variable-input-via-vars-file.md) — input-side surface that locals deliberately stay out of.
- [IMPL-0006 — Migrate lockfile from YAML to HCL](../impl/0006-migrate-lockfile-from-yaml-to-hcl.md) — lockfile format that the locals round-trip depends on.
- [Terraform `locals` documentation](https://developer.hashicorp.com/terraform/language/values/locals) — direct prior art; this RFC adopts a scoped-down subset.
- `internal/config/blueprint.go` — `Blueprint` struct that gains the `Locals` field.
- `internal/config/hcldec_spec.go` — schema that gains the `locals` block.
- `internal/create/create.go` — resolution flow that gains the locals evaluation step.
- `internal/lockfile/lock.go` — `Lockfile` struct that gains the `Locals` map.
