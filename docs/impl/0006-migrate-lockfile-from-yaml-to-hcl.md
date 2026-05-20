---
id: IMPL-0006
title: "Migrate lockfile from YAML to HCL"
status: Draft
author: Donald Gifford
created: 2026-05-18
---
<!-- markdownlint-disable-file MD025 MD041 -->

# IMPL 0006: Migrate lockfile from YAML to HCL

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-05-18

<!--toc:start-->
- [Objective](#objective)
- [Scope](#scope)
  - [In Scope](#in-scope)
  - [Out of Scope](#out-of-scope)
- [Implementation Phases](#implementation-phases)
  - [Phase A: HCL loader + emitter alongside YAML](#phase-a-hcl-loader--emitter-alongside-yaml)
    - [Tasks](#tasks)
    - [Success Criteria](#success-criteria)
  - [Phase B: Cutover — HCL only, YAML rejected](#phase-b-cutover--hcl-only-yaml-rejected)
    - [Tasks](#tasks-1)
    - [Success Criteria](#success-criteria-1)
  - [Phase C: Documentation and release prep](#phase-c-documentation-and-release-prep)
    - [Tasks](#tasks-2)
    - [Success Criteria](#success-criteria-2)
- [File Changes](#file-changes)
  - [New files](#new-files)
  - [Modified files](#modified-files)
- [Testing Plan](#testing-plan)
- [Quality Gates](#quality-gates)
- [Dependencies](#dependencies)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Objective

Move forge's lockfile from `.forge-lock.yaml` to `.forge-lock.hcl` so
every file in forge's contract is HCL2. The lockfile is the last
YAML-formatted file in the project — moving it eliminates the "we
migrated off YAML except for one file" caveat and unifies the parser,
emitter, and on-disk grammar story.

**Implements:** consistency cleanup motivated by DESIGN-0004
(unify config file format). No separate DESIGN doc — this work is
mechanical format migration; the rationale lives entirely in the
"why" below.

## Scope

### In Scope

- New HCL loader and emitter for the existing `Lockfile` /
  `BlueprintRef` / `DefaultEntry` / `ManagedFileEntry` structs.
- Load-time rejection of bare `.forge-lock.yaml` after the cutover,
  with a **rescaffold/pin** pointer (per ADR-0002 — no in-tool
  migrator is built).
- Updates to every reader/writer of the lockfile path
  (`internal/lockfile/`, `internal/create/`, `internal/sync/`,
  `internal/check/`).
- Testdata fixtures migrated; frozen YAML fixture retained for
  rejection-path tests.
- Documentation: MIGRATION.md gains a YAML→HCL lockfile section
  describing the rescaffold path; CLAUDE.md and README.md updated.

### Out of Scope

- **`forge migrate lockfile` migration command.** Per ADR-0002,
  forge does not ship in-tool migrators. Users on v0.4.x lockfiles
  either rescaffold their projects from the current blueprint or
  pin forge to v0.4.x.
- Lockfile schema changes — fields stay the same; only the
  serialisation format changes.
- Object/list/map variable support — covered by RFC-0002. The HCL
  lockfile naturally extends to nested values when those types land,
  but the cutover here is single-format-swap only.
- Backwards-compatible dual-format reader after the cutover.
  v0.5.x rejects YAML lockfiles outright and points the user at
  `docs/MIGRATION.md`.

## Implementation Phases

Three phases: HCL loader/emitter, cutover, docs. The four-phase
shape from IMPL-0005 collapses to three because **Phase B
(`forge migrate lockfile`) is dropped per ADR-0002** — there's no
migration tool to build between the loader work and the cutover.

---

### Phase A: HCL loader + emitter alongside YAML

Add an HCL loader and a writer for the existing lockfile structs.
YAML stays the default during this phase; HCL is opt-in via filename
detection (`.forge-lock.hcl` preferred over `.forge-lock.yaml` when
both exist).

#### Tasks

- [x] **A.1 Define `hcldec.ObjectSpec` for `Lockfile`.**
  - File: `internal/lockfile/hcldec_spec.go` (new).
  - Mirrors `internal/config/hcldec_spec.go`. `BlueprintRef` is a
    single nested block; `Defaults` and `ManagedFiles` are
    `BlockListSpec`s with `path` as the block label.
- [x] **A.2 Implement `LoadLockfileHCL(path) (*Lockfile, error)`.**
  - File: `internal/lockfile/loader_hcl.go` (new).
  - Uses `hashicorp/hcl/v2/hclparse` + `hcldec.Decode`.
  - `Variables` decodes as `cty.Value` of object type; conversion to
    `map[string]any` happens at the boundary for backwards
    compatibility with existing consumers.
- [x] **A.3 Implement `WriteLockfileHCL(w io.Writer, lf *Lockfile) error`.**
  - File: `internal/lockfile/emit_hcl.go` (new).
  - Uses `hashicorp/hcl/v2/hclwrite` for canonical formatting.
  - Variables are emitted as a single `variables { ... }` block with
    one attribute per variable (see OQ-1 — block form is the
    decided shape); nested types serialise naturally.
- [x] **A.4 Dispatching loader.**
  - File: `internal/lockfile/lock.go` (modify).
  - `LoadLockfile(dir)` stats `.forge-lock.hcl` first; falls back to
    `.forge-lock.yaml` if absent. Preserves current behavior during
    transition.
- [x] **A.5 Hermetic HCL fixture.**
  - File: `testdata/lockfile-hcl/.forge-lock.hcl` (new).
  - Covers all field shapes: BlueprintRef, Variables (string/bool/int),
    Defaults, ManagedFiles. Used by loader unit tests.
- [x] **A.6 Unit tests for the HCL loader.**
  - File: `internal/lockfile/loader_hcl_test.go` (new).
  - Happy path, missing-fields, type-mismatch, unknown-attribute.
- [x] **A.7 Round-trip test.**
  - Load YAML fixture → re-emit as HCL → load HCL → assert struct
    equality with original. Pins format equivalence.

#### Success Criteria

- `go build ./...` succeeds.
- `make test-pkg PKG=./internal/lockfile` passes.
- A fixture project with `.forge-lock.hcl` loads correctly via
  `forge check` / `forge sync` (manual smoke test).

---

### Phase B: Cutover — HCL only, YAML rejected

Flip the loader to HCL-only. Bare `.forge-lock.yaml` produces a
load-time rescaffold/pin error (per ADR-0002).

#### Tasks

- [x] **B.1 Replace the YAML loader with a rejection error.**
  - File: `internal/lockfile/lock.go` (modify).
  - `LoadLockfile(dir)` stats `.forge-lock.hcl` and loads it.
    If only `.forge-lock.yaml` exists, return:
    > "lockfile .forge-lock.yaml: YAML lockfiles are no longer
    > supported in this version of forge. Either rescaffold this
    > project from the current blueprint, or pin forge to v0.4.x.
    > See docs/MIGRATION.md."
  - **Note:** the v0.4.x pinning path works because v0.4.x still
    has `forge migrate config` and writes YAML lockfiles. IMPL-0007
    will further refine this error when the rejection path becomes
    the only path.
- [x] **B.2 Drop `gopkg.in/yaml.v3` from `internal/lockfile/`.**
  - The only remaining YAML import in the package disappears.
  - The migratecmd shadow types still import yaml.v3 today, but
    IMPL-0007 deletes that package entirely. Once both this work
    and IMPL-0007 land, the yaml.v3 dependency can be dropped from
    `go.mod`.
- [x] **B.3 Update writers.**
  - `internal/create/`, `internal/sync/`, `internal/check/` — every
    site that writes the lockfile switches from `WriteLockfile` (YAML)
    to `WriteLockfileHCL`.
- [x] **B.4 Migrate testdata fixtures.**
  - Every `testdata/.../forge-lock.yaml` becomes `.forge-lock.hcl`.
    Frozen YAML fixture preserved at `testdata/v0-lockfile-yaml/`
    for the rejection-path test.
  - Result: no `forge-lock.yaml` files existed under `testdata/` to
    migrate — the canonical HCL fixture at
    `testdata/lockfile-hcl/.forge-lock.hcl` is the only on-disk
    lockfile fixture, and the rejection test (B.6) writes its YAML
    payload inline, so no frozen YAML fixture file was needed.
- [x] **B.5 Update integration tests.**
  - `internal/create/`, `internal/sync/`, `internal/check/` —
    integration tests that assert lockfile contents now read HCL.
- [x] **B.6 Add the rejection test.**
  - `internal/lockfile/loader_hcl_test.go` — load a directory with
    only `.forge-lock.yaml` and assert the rescaffold/pin error.

#### Success Criteria

- `make ci` green.
- `forge create` produces `.forge-lock.hcl`.
- `forge check` against a YAML lockfile surfaces the rescaffold/pin
  error.
- A scaffolded project from forge-registry produces a clean
  `.forge-lock.hcl` round-trip through `forge check` / `forge sync`.

---

### Phase C: Documentation and release prep

#### Tasks

- [x] **C.1 Update DESIGN-0004 references.**
  - DESIGN-0004's scope was config files; add a "see also IMPL-0006"
    note for the lockfile follow-up.
- [x] **C.2 Update docs/MIGRATION.md.**
  - New section: "Migrating lockfiles from YAML to HCL (v0.4.x →
    v0.5.x)". Per ADR-0002, this describes the rescaffold or
    pin-to-v0.4.x paths — not an in-tool migrator. Include the
    `go install github.com/donaldgifford/forge@<v0.4.x-tag>`
    invocation for the pin path.
- [x] **C.3 Update CLAUDE.md.**
  - Architecture entries for `internal/lockfile/` reflect the HCL
    loader/emitter.
  - Key Concepts updated where they mention `.forge-lock.yaml`.
  - **No new `forge migrate lockfile` entries** — that command is
    explicitly out of scope per ADR-0002.
- [x] **C.4 Update README.md.**
  - "Migrating from older releases" section adds the v0.4.x→v0.5.x
    step (rescaffold or pin).
- [x] **C.5 Release notes for v0.5.0.**
  - File: `docs/release-notes/v0.5.0-hcl-lockfile.md` (new).
  - Mirror IMPL-0005's release-notes shape: breaking-change block,
    rationale, upgrade steps (rescaffold/pin), behaviour changes,
    "before you cut" checklist.
- [x] **C.6 Fresh forge-registry verification.**
  - Scaffold a fresh project from forge-registry, confirm the new
    lockfile loads/saves correctly through `forge sync` and
    `forge check`.
  - Result: scaffolded `rust/std` against the local
    `github.com/donaldgifford/forge-registry` checkout (26 files);
    `.forge-lock.hcl` written with the expected `blueprint { … }`,
    `variables { … }`, and labelled `default "<path>" { … }` blocks.
    `forge check` reported every file `ok`; `forge sync` reran cleanly
    and advanced `last_synced` (4 updated, 0 conflicts, 21 skipped).
    A simulated v0.4.x project with only `.forge-lock.yaml` surfaced
    the documented rescaffold/pin error.

#### Success Criteria

- `docs/MIGRATION.md` has a complete YAML→HCL lockfile section that
  documents the rescaffold/pin path (no in-tool migrator).
- Release notes draft ready for the v0.5.0 cut.

---

## File Changes

### New files

| File | Phase | Purpose |
|------|-------|---------|
| `internal/lockfile/hcldec_spec.go` | A | `hcldec.ObjectSpec` for Lockfile |
| `internal/lockfile/loader_hcl.go` | A | `LoadLockfileHCL` |
| `internal/lockfile/emit_hcl.go` | A | `WriteLockfileHCL` via `hclwrite` |
| `internal/lockfile/loader_hcl_test.go` | A | Loader unit tests |
| `testdata/lockfile-hcl/.forge-lock.hcl` | A | HCL hermetic fixture |
| `testdata/v0-lockfile-yaml/.forge-lock.yaml` | B | Frozen YAML fixture for rejection-path tests |
| `docs/release-notes/v0.5.0-hcl-lockfile.md` | C | Release notes |

### Modified files

| File | Phase | Change |
|------|-------|--------|
| `internal/lockfile/lock.go` | A → B | Dispatching loader (A); delete YAML loader (B) |
| `internal/create/create.go` | B | Switch to `WriteLockfileHCL` |
| `internal/sync/engine.go` | B | Switch to `WriteLockfileHCL` |
| `internal/check/check.go` | B | Read HCL lockfile |
| `docs/MIGRATION.md` | C | YAML→HCL lockfile section (rescaffold/pin) |
| `CLAUDE.md` | C | HCL lockfile in architecture notes |
| `README.md` | C | Migration note |
| `docs/design/0004-unify-config-file-format-after-hcl2-cutover.md` | C | "see also" cross-link |
| `mkdocs.yml` | C | Release-notes nav |

## Testing Plan

- **Unit tests:** Loader (happy path, malformed, missing fields,
  type mismatch); emitter (canonical output, round-trip).
- **Integration tests:** `forge create` → `forge sync` →
  `forge check` cycle with the new HCL lockfile.
- **Rejection-path tests:** Direct `LoadLockfile` against a directory
  with only `.forge-lock.yaml` returns the rescaffold/pin error.

## Quality Gates

- After Phase A: `make test-pkg PKG=./internal/lockfile` green;
  go-review subagent pass on new loader/emitter code.
- After Phase B: `make ci` green; existing integration tests pass
  with the YAML→HCL writer swap; rejection-path test green.
- After Phase C: docs reviewed via docz-reviewer subagent; release
  notes match v0.4.0 release-notes shape.

## Dependencies

- **ADR-0002 — Forge does not ship in-tool migrators.** Codifies the
  decision to drop the originally-planned `forge migrate lockfile`
  command.
- **IMPL-0007 — Remove forge migrate command.** Removes the
  existing `cmd/migrate.go` and `internal/migratecmd/` package.
  IMPL-0006's Phase B rejection error initially uses the
  "pin v0.4.x" pointer; IMPL-0007 keeps that pointer consistent
  with the post-removal state.
- **DESIGN-0005 (preferred but not required).** If DESIGN-0005
  ships first, the `--var-file` HCL parser machinery is already in
  place and can be referenced for shared patterns.
- No external dependencies. All required libs
  (`hashicorp/hcl/v2`, `zclconf/go-cty`) already in `go.mod`.

## Open Questions

- **OQ-1: Should `Variables` be a top-level block or attributes?**
  Block form: `variables { project_name = "mockta" }` — clearer
  visual grouping, matches how blueprint configs structure variable
  declarations. Attribute form: each variable at the top level —
  flatter, less ceremony. **Decision: block form.** Reflected in
  task A.3.
- **OQ-2: Preserve YAML comments during migration?** N/A — per
  ADR-0002 there is no in-tool migration step. Frozen YAML
  fixtures used for rejection-path testing don't need comment
  fidelity. **Decision: dropped.**
- **OQ-3: Drop `gopkg.in/yaml.v3` from `go.mod` entirely?**
  **Decision: yes, after IMPL-0007 lands.** Once Phase B of this
  work removes the lockfile's yaml.v3 usage and IMPL-0007 removes
  `internal/migratecmd/`, the dependency has no in-tree consumers
  and can be dropped from `go.mod` via `go mod tidy`.
- **OQ-4: Version bump.** Per the IMPL-0005 precedent (OQ-8) and
  IMPL-0004 (OQ-9), pre-1.0 minors are the right channel for
  documented breaking changes. **Decision: v0.5.0.**

## References

- [ADR-0002 — Forge does not ship in-tool migrators](../adr/0002-forge-does-not-ship-in-tool-migrators.md) — the principle that drops the originally-planned `forge migrate lockfile` command.
- [IMPL-0007 — Remove forge migrate command](0007-remove-forge-migrate-command.md) — removes the existing migrate command; coordinated with this IMPL on the rejection-error wording.
- [DESIGN-0004 — Unify config file format after HCL2 cutover](../design/0004-unify-config-file-format-after-hcl2-cutover.md) — motivating consistency story.
- [IMPL-0005 — Unify config file format to HCL2](0005-unify-config-file-format-to-hcl2.md) — precedent format-swap work; same loader/emitter pattern.
- [DESIGN-0005 — Variable input via vars file](../design/0005-variable-input-via-vars-file.md) — parallel HCL-everywhere work on the input side.
- [docs/MIGRATION.md](../MIGRATION.md) — where the v0.4.x → v0.5.x section will be added.
- `internal/lockfile/lock.go` — current Lockfile struct definitions.
- `internal/lockfile/cty.go` — existing cty.Value coercion machinery.
- `hashicorp/hcl/v2/hclwrite` — emitter library; used by `internal/config/hclemit.go`.
