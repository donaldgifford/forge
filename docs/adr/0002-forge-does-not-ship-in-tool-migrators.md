---
id: ADR-0002
title: "Forge does not ship in-tool migrators"
status: Proposed
author: Donald Gifford
created: 2026-05-18
---
<!-- markdownlint-disable-file MD025 MD041 -->

# 0002. Forge does not ship in-tool migrators

<!--toc:start-->
- [Status](#status)
- [Context](#context)
- [Decision](#decision)
- [Consequences](#consequences)
  - [Positive](#positive)
  - [Negative](#negative)
  - [Neutral](#neutral)
- [Alternatives Considered](#alternatives-considered)
- [References](#references)
<!--toc:end-->

## Status

Proposed

## Context

Through v0.4.x, forge has accumulated three in-tool migration
commands and contemplated two more:

- `forge migrate templates` (shipped v0.3, IMPL-0004) — rewrites v1
  `text/template` syntax to v2 (HCL2).
- `forge migrate config` (shipped v0.4, IMPL-0005) — rewrites
  `blueprint.yaml` / `registry.yaml` to their HCL equivalents.
- `forge migrate lockfile` (proposed in IMPL-0006 Phase B) — would
  rewrite `.forge-lock.yaml` to `.forge-lock.hcl`.
- `forge migrate refs` (proposed in RFC-0003 Phase 4) — would
  rewrite bare `${NAME}` references to namespaced `${var.NAME}` /
  `${local.NAME}` form, with an optional auto-promotion sub-step
  for derived-default variables.

The migrate commands have served their purpose well for the
*mechanical* format swaps (template syntax v1→v2, YAML→HCL configs).
But the upcoming work package (RFC-0002 / DESIGN-0006 object/list/map
types and choice→validation block, RFC-0003 locals + namespacing,
IMPL-0006 lockfile format) is **architecturally entangled**:

- Namespacing every variable reference (`${X}` → `${var.X}`) is a
  mechanical transform, but the optional promotion of
  derived-default variables to locals is a *judgement call* that
  varies by blueprint.
- The choice→validation reframing (RFC-0002 OQ-3 decision) is
  semantically significant — `type = "choice", choices = [...]`
  becomes `type = "string", validation { condition = contains(...) }`.
  Rewriting this preserves syntax but loses the prompt-UX shortcut
  the `choice` type currently provides.
- Object/list/map types let authors collapse multiple flat
  variables into one structured variable — a refactoring decision,
  not a mechanical rewrite.
- The migrators themselves carry meaningful complexity: the v0.3
  template walker required a scoped-rewrite fix (PR #25) to avoid
  rewriting downstream-tool syntax, and a similar problem would
  affect any `forge migrate refs` implementation.

Maintaining four-plus migrate commands across the v0.5→v0.7
release window also taxes development bandwidth disproportionately
to the value they provide for a pre-1.0 project with a single-digit
registry footprint.

Per the existing convention recorded in `MEMORY.md` and CLAUDE.md
(`gosec baseline + lint version pinning`), forge is comfortable
making clean breaks where the downstream blast radius is small.
Today the only known production registry is `forge-registry`, which
is maintained alongside forge and has already absorbed two
migration passes.

## Decision

**Forge does not ship in-tool migrators for breaking changes.**

Starting with the v0.5+ release window:

1. **No new migrators will be built.** `forge migrate lockfile`
   (IMPL-0006 Phase B) and `forge migrate refs` (RFC-0003 Phase 4)
   are dropped.
2. **The existing `forge migrate templates` and `forge migrate
   config` commands are removed from the codebase** (IMPL-0007).
   Users on v0.2.x or v0.3.x format files who want to upgrade past
   v0.4.x first run `forge migrate` on a pinned v0.4.x binary, then
   upgrade.
3. **Breaking changes are documented in `docs/MIGRATION.md`** with
   the new file format, the rationale, and the rescaffold or
   pin-to-old-version paths. No migration command is provided.
4. **Load-time rejection of unsupported file formats** continues to
   include a clear error message pointing at MIGRATION.md and
   identifying the last supported forge version for that format.
   The "run forge migrate ..." pointer is removed.

The principle, stated for future readers: **forge favours clean
breaks over migration tooling**. Breaking changes happen, are
documented, and users either pin to the last compatible release or
rescaffold from the new blueprint version.

## Consequences

### Positive

- **Removes a class of complexity** from the codebase
  (`internal/migratecmd/`, `cmd/migrate.go`, the migration-tooling
  test fixtures, the YAML shadow types, the dirty-worktree guard
  machinery). Concrete LOC saved: see IMPL-0007.
- **Drops the `gopkg.in/yaml.v3` dependency** entirely once
  IMPL-0006 lands. The lockfile is the last in-tree YAML consumer
  outside `internal/migratecmd/`; removing both unblocks the
  dependency removal.
- **Frees development bandwidth** for the architectural work
  (RFC-0002 / DESIGN-0006, RFC-0003, IMPL-0006). Designing, building, and testing
  migrators for an entangled multi-RFC change set would consume
  more cycles than the underlying work.
- **Sharper release boundaries.** Each minor release has a clear
  "this is what the file format is now" story, without the
  ambiguity of "and here's the migrator that bridges from the last
  format."
- **Honest about pre-1.0 stability.** Forge is pre-1.0 with a
  documented single-registry production footprint. The migrate
  tooling implied a level of backwards-compatibility commitment
  forge isn't ready to make.

### Negative

- **Users on older versions have no upgrade button.** Anyone with
  a v0.2.x or v0.3.x blueprint file (post-IMPL-0007 deletion) has
  to either pin forge to v0.4.x to run the legacy migrators, or
  re-author the blueprint by hand against the new format.
- **Scaffolded-project users lose lockfile portability across
  major bumps.** A `.forge-lock.yaml` written by v0.4.x cannot be
  read by v0.5.x. Users either rescaffold (losing local changes
  not captured in the lockfile) or pin v0.4.x for the lifetime of
  that project.
- **Higher friction for the next big format change.** If forge
  needs another breaking change post-v1.0, the same "no migrator"
  rule will apply — which is intentional but may be unpopular if
  the user base has grown.
- **Loses the v0.5+ "migration-pointer error" pattern.** Existing
  errors that say "run `forge migrate config`" become errors that
  say "rescaffold or pin v0.4.x" — same diagnostic shape, less
  actionable.

### Neutral

- **The MIGRATION.md document becomes more important** as the
  primary record of breaking changes. Each release that introduces
  a format change adds a section with the new format spec, the
  rationale, and the rescaffold/pin paths.
- **Forge-registry remains the canonical "fresh" baseline.**
  Authors who want to see the current format read forge-registry;
  there's no in-tool "show me the current format" surface beyond
  `forge registry init` / `forge registry blueprint`.

## Alternatives Considered

- **Keep `forge migrate` indefinitely.** Build all four migrators
  (`templates`, `config`, `lockfile`, `refs`), maintain them across
  the v0.5→v0.7 window, and accept the complexity cost. Rejected:
  development bandwidth doesn't match the small user base, and the
  `refs` migrator's auto-promotion logic is too judgement-laden to
  ship as a one-shot transform.
- **Keep mechanical migrators (`lockfile`), drop architectural
  ones (`refs`).** Rule: "build migrators for format swaps with
  unchanged schema; rescaffold for everything else." Considered
  and proposed in the prior round of discussion. Rejected because
  it splits the principle along a hard-to-defend line — the
  v0.4→v0.5 lockfile change is mechanical, but the v0.5→v0.7
  change isn't, and the bookkeeping cost of "which versions have
  migrators?" compounds.
- **Build a single `forge upgrade` command** that performs all
  format conversions end-to-end. More user-friendly, but
  consolidates rather than removes the complexity. Same reasoning
  as the first alternative — too much engineering work for too
  small a user base.
- **Defer the decision.** Ship IMPL-0006 with the migrator, decide
  about RFC-0003's migrator later. Rejected: the decision has to
  be made before IMPL-0006's Phase B starts implementation, and
  the cleanest version of the principle covers both cases.

## References

- [IMPL-0006 — Migrate lockfile from YAML to HCL](../impl/0006-migrate-lockfile-from-yaml-to-hcl.md) — drops Phase B per this decision.
- [IMPL-0007 — Remove forge migrate command](../impl/0007-remove-forge-migrate-command.md) — removes existing migrate commands from the codebase per this decision.
- [RFC-0003 — Locals for derived values](../rfc/0003-locals-for-derived-values.md) — drops Phase 4 (`forge migrate refs`) per this decision.
- [RFC-0002 — Object and collection variable types](../rfc/0002-object-and-collection-variable-types.md) — semantic complexity (choice→validation reframing, object types) that informed the "too entangled for a migrator" argument.
- [DESIGN-0006 — Object and collection variable types](../design/0006-object-and-collection-variable-types.md) — formalises RFC-0002's design and pins the migration story to the rescaffold/re-author pattern established by this ADR.
- [IMPL-0004 — HCL2 cutover](../impl/0004-hcl2-cutover.md) — `forge migrate templates`, scheduled for removal under IMPL-0007.
- [IMPL-0005 — Unify config file format to HCL2](../impl/0005-unify-config-file-format-to-hcl2.md) — `forge migrate config`, scheduled for removal under IMPL-0007.
- [docs/MIGRATION.md](../MIGRATION.md) — the document that absorbs the migrator's former role.
