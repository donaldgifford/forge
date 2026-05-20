---
id: IMPL-0007
title: "Remove forge migrate command"
status: Draft
author: Donald Gifford
created: 2026-05-18
---
<!-- markdownlint-disable-file MD025 MD041 -->

# IMPL 0007: Remove forge migrate command

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-05-18

<!--toc:start-->
- [Objective](#objective)
- [Scope](#scope)
  - [In Scope](#in-scope)
  - [Out of Scope](#out-of-scope)
- [Implementation Phases](#implementation-phases)
  - [Phase 1: Delete the migrate code](#phase-1-delete-the-migrate-code)
    - [Tasks](#tasks)
    - [Success Criteria](#success-criteria)
  - [Phase 2: Update load-time rejection errors](#phase-2-update-load-time-rejection-errors)
    - [Tasks](#tasks-1)
    - [Success Criteria](#success-criteria-1)
  - [Phase 3: Documentation and release prep](#phase-3-documentation-and-release-prep)
    - [Tasks](#tasks-2)
    - [Success Criteria](#success-criteria-2)
- [File Changes](#file-changes)
  - [Files removed](#files-removed)
  - [Files modified](#files-modified)
- [Testing Plan](#testing-plan)
- [Quality Gates](#quality-gates)
- [Dependencies](#dependencies)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Objective

Remove the `forge migrate` command and its supporting
`internal/migratecmd/` package from the codebase, implementing the
"no in-tool migrators" principle established by ADR-0002.

Users on v0.2.x or v0.3.x format files who want to upgrade past
v0.5.x first run `forge migrate` on a pinned v0.4.x binary, then
upgrade. The migration commands continue to exist in the v0.4.x
release artifacts indefinitely; this work only removes them from
the main branch going forward.

**Implements:** [ADR-0002 — Forge does not ship in-tool
migrators](../adr/0002-forge-does-not-ship-in-tool-migrators.md)

## Scope

### In Scope

- Delete `internal/migratecmd/` package in full.
- Delete `cmd/migrate.go`.
- Remove the migrate subcommand registration from `cmd/root.go`.
- Update the load-time rejection errors in `internal/config/loader.go`
  and (per IMPL-0006) `internal/lockfile/lock.go` to drop the
  "run `forge migrate ...`" pointer and replace with a rescaffold
  pointer to `docs/MIGRATION.md`.
- Drop `gopkg.in/yaml.v3` from `go.mod` (only viable once IMPL-0006
  ships — the lockfile is the other YAML consumer).
- Update `docs/MIGRATION.md` to reflect the new "pin v0.4.x then
  upgrade" path for v0.2.x/v0.3.x users.
- Update `CLAUDE.md` and `README.md` to remove migrate-command
  references.
- Update fixtures that exercise migrate flows to assert the new
  rejection-only behavior.

### Out of Scope

- Building any new migrators (forbidden by ADR-0002).
- Backporting the migrate commands to v0.4.x — they already exist
  there; the v0.4.x release binary is the migration tool now.
- Schema or format changes — IMPL-0006 (lockfile YAML→HCL),
  RFC-0002 (types), RFC-0003 (locals + namespacing) all live in
  their own docs.
- A `forge upgrade` consolidation command — rejected as an
  alternative in ADR-0002.

## Implementation Phases

Three phases: delete the code, fix the user-facing error messages,
update docs.

---

### Phase 1: Delete the migrate code

Remove the migrate command, its package, its fixtures, and its
tests.

#### Tasks

- [x] **1.1 Delete `internal/migratecmd/` in full.**
  - Removes: `walk.go`, `rules.go`, `walk_test.go`, `rules_test.go`,
    `lockfile.go` (if landed via IMPL-0006 Phase B before this
    work — confirm not present), `yaml_types.go`, all related
    fixtures under `testdata/`.
  - The fixture directories (`testdata/v1-template/`,
    `testdata/v2-yaml-registry/`) are deleted along with the
    package.
  - Result: deleted all 15 `internal/migratecmd/*.go` files plus
    `testdata/v2-yaml-registry/`. `testdata/v1-registry/` was
    preserved — `internal/config/loader_test.go` still uses it for
    the v1-shape YAML-rejection assertion.
- [x] **1.2 Delete `cmd/migrate.go`.**
- [x] **1.3 Drop the migrate subcommand registration in `cmd/root.go`.**
  - The migrate subcommand self-registered via an `init()` block in
    `cmd/migrate.go` — deleting that file removed the registration
    automatically. `cmd/root.go` never referenced migrate directly,
    so no edits there.
- [x] **1.4 Remove migrate-related references in test helpers.**
  - Audit `cmd/*_test.go` and `internal/*_test.go` for any
    fixtures or test cases that exercise `forge migrate` flows.
    Delete those tests outright (they're testing a deleted
    feature).
  - No surviving migrate-test references outside migratecmd itself
    (which was deleted wholesale). The remaining `forge migrate`
    strings live in `internal/config/loader.go`'s rejection error
    and its test; Phase 2 owns that rewrite.
- [~] **1.5 Drop `gopkg.in/yaml.v3` from `go.mod` and `go.sum`.**
  - Conditional on IMPL-0006 having landed first; otherwise the
    lockfile loader still imports yaml.v3 and the dependency stays
    until that work ships.
  - Run `go mod tidy` to confirm.
  - **Not achievable in this work.** Two non-legacy callers still
    import `gopkg.in/yaml.v3`: `internal/config/global.go` (global
    forge config) and `internal/registry/cache.go` (registry cache
    metadata). Those YAML files are current state, not legacy
    formats — converting them is out of scope for IMPL-0007.
    `go mod tidy` kept yaml.v3 in `go.mod` after the migratecmd
    deletion. A future RFC can pick up "drop yaml.v3 entirely" if
    desired.
- [x] **1.6 `make ci` passes with the package deleted.**
  - Any remaining import-cycle or unused-import failures are
    surfaced as part of this task and fixed.

#### Success Criteria

- `grep -r "migratecmd\|forge migrate" cmd/ internal/` returns no
  hits.
- `go build ./...` succeeds.
- `make ci` green.
- `forge --help` no longer lists `migrate` as a subcommand.

---

### Phase 2: Update load-time rejection errors

Existing rejection paths for `blueprint.yaml` / `registry.yaml`
(IMPL-0005) and `.forge-lock.yaml` (IMPL-0006) point at the
migrate command. Replace those pointers with rescaffold
instructions and a MIGRATION.md link.

#### Tasks

- [x] **2.1 Update `internal/config/loader.go` rejection error.**
  - Old: "blueprint.yaml: YAML configs are no longer supported.
    Run `forge migrate config --path .` to convert. See
    docs/MIGRATION.md."
  - New (per OQ-3 — include the exact `go install` command):
    > "blueprint.yaml: YAML configs are no longer supported in
    > this version of forge.
    >
    > To upgrade:
    >   - Rescaffold from the current blueprint, OR
    >   - Pin forge to v0.4.x and run `forge migrate config`:
    >       go install github.com/donaldgifford/forge@v0.4.x
    >
    > See docs/MIGRATION.md for the full upgrade guide."
  - The `v0.4.x` placeholder is replaced with the most recent
    v0.4.x tag at the time of the v0.5.0 cut.
- [x] **2.2 Update `internal/lockfile/lock.go` rejection error
      (IMPL-0006 Phase B output).**
  - Old (IMPL-0006 as-was): "lockfile .forge-lock.yaml: YAML
    lockfiles are no longer supported. Run `forge migrate
    lockfile` ..."
  - New (per OQ-3):
    > "lockfile .forge-lock.yaml: YAML lockfiles are no longer
    > supported in this version of forge.
    >
    > To upgrade:
    >   - Rescaffold this project from the current blueprint, OR
    >   - Pin forge to v0.4.x:
    >       go install github.com/donaldgifford/forge@v0.4.x
    >
    > See docs/MIGRATION.md."
- [x] **2.3 Update unit tests** in
      `internal/config/loader_test.go` and
      `internal/lockfile/lock_test.go` to assert the new error
      strings.
  - Note: `internal/lockfile/lock_test.go` was deleted in IMPL-0006
    B.2 (YAML round-trip tests retired). The lockfile rejection
    test now lives in `internal/lockfile/loader_hcl_test.go`
    (`TestLoadLockfile_RejectsYAMLOnly`) and was updated here to
    assert the new rescaffold/pin-with-go-install error string.
- [x] **2.4 Audit `internal/ui/` and `cmd/` for any other
      migrate-pointer strings.**
  - `grep -r "forge migrate" .` should return zero hits in
    runtime code (only in docs).
  - Result: cleaned three stale comments in
    `internal/config/blueprint.go`, `internal/config/hclemit.go`,
    and `internal/registry/index.go`. The remaining `forge
    migrate` occurrences in runtime code are intentional — they
    live inside the rejection-error format strings (OQ-3 says the
    error tells the user to "Pin forge to v0.4.1 and run `forge
    migrate config`") and in doc comments explaining the IMPL-0007
    rationale.

#### Success Criteria

- All rejection-path tests pass with the new error strings.
- No runtime-code reference to `forge migrate` remains.

---

### Phase 3: Documentation and release prep

#### Tasks

- [ ] **3.1 Update `docs/MIGRATION.md`.**
  - Restructure the top-of-document guidance:
    - For v0.2.x users: "Install forge v0.4.x, run `forge migrate
      templates`, then `forge migrate config`, then upgrade to
      current forge." Include the exact `mise use` or
      `go install` command for pinning v0.4.x.
    - For v0.3.x users: "Install forge v0.4.x, run `forge migrate
      config`, then upgrade."
    - For v0.4.x users: "No supported migration path. Rescaffold
      your project from the current blueprint."
  - Remove the in-tool `forge migrate` examples — they're now
    valid only against the pinned v0.4.x binary.
- [ ] **3.2 Update `CLAUDE.md`.**
  - Architecture entries for `internal/migratecmd/` deleted.
  - `cmd/` listing: drop the migrate subcommand entry.
  - "CLI Design Decisions" section: drop the
    `forge migrate templates --path <registry>` and
    `forge migrate config --path <registry>` bullets.
- [ ] **3.3 Update `README.md`.**
  - Commands table: drop migrate rows.
  - "Migrating from older releases" section: replace migrate
    instructions with the "pin v0.4.x then upgrade" pattern.
- [ ] **3.4 Release notes for v0.5.0** (or whichever release ships
      this work).
  - Highlight the breaking change: the migrate command is gone.
  - Include the v0.4.x pinning instructions for stragglers.
- [ ] **3.5 Update `mise.toml` references in MIGRATION.md** to
      point at a known-good v0.4.x version.

#### Success Criteria

- `docs/MIGRATION.md` is self-contained: a user with a v0.2.x
  project can read it and know exactly what to do.
- `CLAUDE.md`, `README.md`, and release notes are consistent with
  the "no migrate command in main" reality.

---

## File Changes

### Files removed

| File / directory | Reason |
|---|---|
| `internal/migratecmd/` (entire package) | Per ADR-0002. |
| `internal/migratecmd/yaml_types.go` | Shadow types for legacy YAML decoding, no longer needed. |
| `internal/migratecmd/walk.go`, `walk_test.go` | Template migrator walker + tests. |
| `internal/migratecmd/rules.go`, `rules_test.go` | Template rewrite rules + tests. |
| `internal/migratecmd/git.go` | Dirty-worktree guard, no longer needed. |
| `cmd/migrate.go` | Cobra subcommand wiring. |
| `testdata/v1-template/` | Fixtures for `forge migrate templates`. |
| `testdata/v2-yaml-registry/` | Fixtures for `forge migrate config`. |

### Files modified

| File | Change |
|---|---|
| `cmd/root.go` | Drop migrate subcommand registration. |
| `internal/config/loader.go` | Rejection error: drop `forge migrate` pointer, add rescaffold/pin pointer. |
| `internal/lockfile/lock.go` | Same change for the lockfile rejection error (depends on IMPL-0006 having landed). |
| `internal/config/loader_test.go` | Assert new error string. |
| `internal/lockfile/lock_test.go` | Assert new error string. |
| `go.mod`, `go.sum` | Drop `gopkg.in/yaml.v3` (after IMPL-0006 ships). |
| `docs/MIGRATION.md` | Restructure for "pin v0.4.x" path. |
| `CLAUDE.md` | Remove migratecmd architecture entries and CLI design decisions. |
| `README.md` | Remove migrate command from commands table; restructure migration guidance. |
| `docs/release-notes/v0.5.x-or-later.md` | New section: migrate command removed. |

## Testing Plan

- **No new tests** — this is a delete-and-fix-error-strings work
  package. The tests being deleted are the ones that exercised the
  removed feature.
- **Rejection-path tests updated** to assert the new error strings
  (no migrate pointer, rescaffold/pin pointer).
- **`make ci` clean** is the headline gate: any leftover migrate
  reference will surface as an unused-import or test failure.

## Quality Gates

- After Phase 1: `make ci` green; `forge --help` no longer lists
  `migrate`; `go.mod` no longer carries `gopkg.in/yaml.v3` (if
  IMPL-0006 already shipped).
- After Phase 2: All rejection-path tests pass; `grep -r "forge
  migrate" cmd/ internal/` returns zero hits.
- After Phase 3: docz-reviewer pass on MIGRATION.md; README and
  CLAUDE.md reviewed for consistency.

## Dependencies

- **ADR-0002** establishes the principle.
- **IMPL-0006 (Phase A+C only; Phase B dropped per ADR-0002)** is
  the immediate predecessor. The lockfile-loader rejection-error
  update in Phase 2 of this IMPL depends on IMPL-0006's Phase C
  having landed. Order: IMPL-0006 → IMPL-0007.
- **No external dependencies.** All work is deletion + error
  message updates.

## Open Questions

- **OQ-1: Cut in the same release as IMPL-0006, or one release
  later?** Cutting both in v0.5.0 means a single big "format
  cleanup" release. Splitting into v0.5.0 (IMPL-0006) and v0.6.0
  (IMPL-0007) lets v0.5.0 still ship a working `forge migrate
  lockfile` for stragglers — but ADR-0002 forbids that migrator
  from being built at all. **Decision: cut together in v0.5.0.**
  The ADR-0002 principle is the same either way; splitting just
  means one extra release with a half-removed feature.
- **OQ-2: Preserve the migratecmd tests in a `legacy/` branch?**
  Some value in keeping the tests around for future reference if
  forge ever revisits the migrator question. **Decision: no.** Git
  history is the right place for that; the deletion is reversible
  via revert if it ever becomes necessary.
- **OQ-3: Should the rejection error include the exact
  `go install github.com/donaldgifford/forge@v0.4.x` invocation?**
  Makes the rescue path one copy-paste away. **Decision: yes.**
  The error wording in Phase 2 explicitly includes the `go install`
  command (pinned to the most recent v0.4.x tag at the time of the
  v0.5.0 cut) so the user's next-question gap is closed at the
  point of failure.

## References

- [ADR-0002 — Forge does not ship in-tool migrators](../adr/0002-forge-does-not-ship-in-tool-migrators.md) — the decision this IMPL implements.
- [IMPL-0006 — Migrate lockfile from YAML to HCL](0006-migrate-lockfile-from-yaml-to-hcl.md) — Phase B dropped per ADR-0002; this IMPL fixes the rejection-error pointer left by IMPL-0006's Phase C.
- [IMPL-0004 — HCL2 cutover](0004-hcl2-cutover.md) — original home of `forge migrate templates`.
- [IMPL-0005 — Unify config file format to HCL2](0005-unify-config-file-format-to-hcl2.md) — original home of `forge migrate config`.
- [RFC-0003 — Locals for derived values](../rfc/0003-locals-for-derived-values.md) — Phase 4 (`forge migrate refs`) dropped per ADR-0002.
- [docs/MIGRATION.md](../MIGRATION.md) — primary user-facing artifact for this work package.
- `internal/migratecmd/` — the package being deleted.
- `cmd/migrate.go` — the Cobra wiring being deleted.
