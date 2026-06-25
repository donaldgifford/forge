---
id: INV-0002
title: "auto-generate HCL reference doc"
status: Open
author: Donald Gifford
created: 2026-06-25
---
<!-- markdownlint-disable-file MD025 MD041 -->

# INV 0002: auto-generate HCL reference doc

**Status:** Open
**Author:** Donald Gifford
**Date:** 2026-06-25

<!--toc:start-->
- [Question](#question)
- [Hypothesis](#hypothesis)
- [Context](#context)
- [Approach](#approach)
- [Findings](#findings)
  - [Observation 1: hcldec specs are already structured](#observation-1-hcldec-specs-are-already-structured)
  - [Observation 2: hand-decoded schemas need a parallel path](#observation-2-hand-decoded-schemas-need-a-parallel-path)
  - [Observation 3: prose can't be derived from code](#observation-3-prose-cant-be-derived-from-code)
  - [Observation 4: CI staleness check is the cheap win](#observation-4-ci-staleness-check-is-the-cheap-win)
- [Conclusion](#conclusion)
- [Recommendation](#recommendation)
- [References](#references)
<!--toc:end-->

## Question

Can `docs/REFERENCE.md` be auto-generated from the Go source of truth
(struct definitions, `hcldec` specs, validation constants) so the
documented HCL surface stays in lockstep with the implementation
without hand-editing every time the schema changes?

## Hypothesis

Yes for the *tables* (attributes, types, required/optional, allowed
values) since those come from declarative Go data. No for the
*prose* (the "Why this attribute exists" / "How it composes" / the
code examples) — those are author intent, not derivable from the
type system. A practical solution probably splits the doc into
generated table sections and hand-authored narrative.

## Context

`docs/REFERENCE.md` was added in the same PR that opened this
investigation — a hand-written reference covering `blueprint.hcl`,
`registry.hcl`, `.forge-lock.hcl`, and `.forge-vars.hcl`. The
maintenance risk is real: every IMPL that touches the HCL surface
(IMPL-0004, IMPL-0005, IMPL-0006, IMPL-0008 in the last six months)
would need to update REFERENCE.md or it rots silently. Three of the
four files have already added or removed attributes during this
period.

**Triggered by:** REFERENCE.md initial draft (this branch).

## Approach

Evaluate three approaches without writing a generator yet:

1. **Reflect over `hcldec.ObjectSpec` values at runtime** and emit
   tables. The specs already encode attribute name, type, and
   required-ness.
2. **Walk Go AST + struct tags** with `go/ast` so we can pick up
   field comments too.
3. **Status quo + CI staleness check** — keep the hand-authored doc
   but add a CI gate that fails if any of the source-of-truth files
   change without REFERENCE.md changing.

For each: estimate effort, accuracy, and what it can and cannot
cover.

## Findings

### Observation 1: hcldec specs are already structured

`internal/config/hcldec_spec.go` declares every blueprint and
registry attribute as a `hcldec.ObjectSpec` value. Each entry
carries:

- the HCL attribute name (`Name` field on `AttrSpec`)
- the cty type (`Type` field — gives us `cty.String`,
  `cty.List(cty.String)`, etc.)
- the `Required` flag

Nested blocks are tagged with `hcldec.BlockSpec.TypeName`. Block
labels are `hcldec.BlockLabelSpec`. **All of this is reachable
through plain reflection at runtime** — no AST walk needed for the
spec-driven parts of the schema.

Estimated effort to emit the blueprint + registry attribute tables
from these specs: ~1 day, including unit tests.

### Observation 2: hand-decoded schemas need a parallel path

Two block kinds are *not* in the eager `hcldec` specs:

- `variable "name" { … }` — `default` and `validate` must be
  captured as raw source bytes (templates referencing later-bound
  variables). The schema lives in `variableBlockBodySchema`
  (a plain `hcl.BodySchema`).
- `condition { when = … }` — same reason. Lives in
  `conditionBlockBodySchema`.

Plus the `rename` block uses an outer/inner schema pair
(`renameOuterBodySchema` + `renameEntryBodySchema`) for the same
reason.

These `hcl.BodySchema` values *are* still reflectable
(`Attributes` slice with `Name` and `Required` fields), they just
need a second code path because the type isn't `hcldec.ObjectSpec`.
Doable but adds complexity.

The `Lockfile` struct and the registry shape are fully covered by
their eager specs, so no parallel-path issue there.

### Observation 3: prose can't be derived from code

About 40% of REFERENCE.md is prose:

- "Why `default` is stringly-typed" (paragraph after the variable
  table)
- "How three-way merge works" (sync strategy descriptions)
- Code examples for every section
- Cross-references to design docs
- The "Adding a new type means touching three sites" guide

None of this is derivable from the type system. A generator would
either:

- emit empty placeholders the author fills in (regenerating wipes
  the prose), or
- merge generated tables into hand-authored prose via markers
  (`<!-- AUTOGEN:blueprint-attributes -->` … `<!-- /AUTOGEN -->`),
  preserving prose between sentinel pairs

The marker approach is doable but ugly to review (giant diffs every
time a type changes; hard to tell what a human meant vs what the
generator chose to emit).

### Observation 4: CI staleness check is the cheap win

A shell one-liner in CI can catch the most common rot:

```sh
# .github/workflows/ci.yml
- name: REFERENCE.md staleness check
  run: |
    SCHEMA_FILES=(
      internal/config/blueprint.go
      internal/config/registry.go
      internal/config/hcldec_spec.go
      internal/config/validate.go
      internal/lockfile/lock.go
      internal/lockfile/cty.go
      internal/lockfile/emit_hcl.go
      internal/varsfile/varsfile.go
    )
    if git diff --name-only origin/main...HEAD | grep -qE "$(IFS='|'; echo "${SCHEMA_FILES[*]}")"; then
      git diff --name-only origin/main...HEAD | grep -q '^docs/REFERENCE.md$' \
        || { echo "::error::Schema files changed without REFERENCE.md update"; exit 1; }
    fi
```

Effort: ~30 minutes. Catches 80% of the rot risk (forgetting to
update the doc when changing a schema). Doesn't catch "you updated
both files but the doc and the code now disagree" — that one needs
either generation or a careful reviewer.

## Conclusion

**Answer:** Partial. The attribute tables in REFERENCE.md *can* be
auto-generated from the `hcldec` specs (Observation 1) plus a
parallel path for the three hand-decoded schemas (Observation 2),
but the prose and examples cannot (Observation 3). A full generator
would need a marker-based merge strategy to preserve narrative
content. The staleness-check (Observation 4) is the highest
value-per-hour option and catches the most common failure mode at
near-zero cost.

## Recommendation

Ship in two phases:

1. **Now (this PR or a follow-up chore PR):** add the CI staleness
   check from Observation 4. ~30 min of work. Catches "forgot to
   update the doc when I added an attribute" — the most common
   silent rot mode.
2. **Later (separate IMPL when REFERENCE.md drift becomes painful):**
   build a `cmd/gen-reference/` tool that reflects over the
   `hcldec.ObjectSpec` and `hcl.BodySchema` values, emits attribute
   tables between `<!-- AUTOGEN:* -->` markers, and exposes
   `make docs-reference` + a CI check that regeneration produces no
   diff. Skip until we feel the pain — REFERENCE.md is one file
   and the schema doesn't change weekly.

The staleness-check unblocks the rot risk today without committing
to a generator we may never need.

## References

- [docs/REFERENCE.md](../REFERENCE.md) — the document this investigation is about.
- [`internal/config/hcldec_spec.go`](../../internal/config/hcldec_spec.go) — the structured spec data a generator would reflect over.
- [`internal/config/validate.go`](../../internal/config/validate.go) — the validation constants (`validVariableTypes`, `validSyncStrategies`).
- [`internal/lockfile/lock.go`](../../internal/lockfile/lock.go), [`internal/lockfile/emit_hcl.go`](../../internal/lockfile/emit_hcl.go) — lockfile shape.
- [`internal/varsfile/varsfile.go`](../../internal/varsfile/varsfile.go) — vars-file schema and coercion.
