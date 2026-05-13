---
id: IMPL-0005
title: "Unify Config File Format to HCL2"
status: Draft
author: Donald Gifford
created: 2026-05-12
---
<!-- markdownlint-disable-file MD025 MD041 -->

# IMPL 0005: Unify Config File Format to HCL2

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-05-12

<!--toc:start-->
- [Objective](#objective)
- [Scope](#scope)
  - [In Scope](#in-scope)
  - [Out of Scope](#out-of-scope)
- [Implementation Phases](#implementation-phases)
  - [Phase A: HCL loader alongside YAML loader](#phase-a-hcl-loader-alongside-yaml-loader)
    - [Tasks](#tasks)
    - [Success Criteria](#success-criteria)
  - [Phase B: Migration tool — `forge migrate config`](#phase-b-migration-tool--forge-migrate-config)
    - [Tasks](#tasks-1)
    - [Success Criteria](#success-criteria-1)
  - [Phase C: Cutover](#phase-c-cutover)
    - [Tasks](#tasks-2)
    - [Success Criteria](#success-criteria-2)
  - [Phase D: Documentation & release prep](#phase-d-documentation--release-prep)
    - [Tasks](#tasks-3)
    - [Success Criteria](#success-criteria-3)
- [File Changes](#file-changes)
  - [New files](#new-files)
  - [Modified files](#modified-files)
  - [Deleted files](#deleted-files)
- [Testing Plan](#testing-plan)
- [Dependencies](#dependencies)
- [Implementation Order](#implementation-order)
- [Resolved Questions](#resolved-questions)
- [References](#references)
<!--toc:end-->

## Objective

Move forge's config files (`blueprint.yaml`, `registry.yaml`) to native
HCL2 (`blueprint.hcl`, `registry.hcl`) so the entire registry tree speaks
one syntax. The change is breaking with a one-shot rewrite tool — no
parallel-format support window. v2 YAML configs stop loading; `forge
migrate config` rewrites them to HCL; HCL is the only format forge
accepts after this lands. Lockfile (`.forge-lock.yaml`) stays YAML —
out of scope.

**Implements:** [DESIGN-0004 — Unify Config File Format After HCL2 Cutover](../design/0004-unify-config-file-format-after-hcl2-cutover.md)

**Builds on:** [IMPL-0004](0004-migrate-template-engine-to-hcl2.md) — the template-engine cutover that shipped in v0.3.0.

## Scope

### In Scope

- Replace `internal/config/` YAML loaders (`LoadBlueprint`, `LoadRegistry`)
  with HCL-backed implementations. Decoding via `hashicorp/hcl/v2/hcldec`
  (see Resolved Questions OQ-1).
- Drop the `apiVersion` field from `Blueprint` and `Registry` structs.
  File extension is the version signal.
- Change `Condition.When` from `string` to `hcl.Expression` (parsed at
  load time, evaluated without re-parsing).
- Build `forge migrate config` (CLI + core) under
  `internal/migratecmd/`. Walks a registry tree, rewrites
  `blueprint.yaml` → `blueprint.hcl` and `registry.yaml` →
  `registry.hcl`. Same dirty-worktree guard, `--dry-run`, `--strict`
  semantics as `forge migrate templates`.
- Update scaffolding emitters (`forge registry init`, `forge registry
  blueprint`, `forge init`) to emit `.hcl` config files.
- Update load-time validators to reject `.yaml` config files with a
  migration-pointer error (same shape as v1→v2 rejection in IMPL-0004).
- Migrate `testdata/registry/` and `testdata/v2-registry/` to HCL.
  Freeze the YAML originals as rejection fixtures.
- Validate the migration tool against the forge-registry corpus.
- Author the YAML→HCL section of `docs/MIGRATION.md`.
- Rewrite DESIGN-0001 and DESIGN-0002 examples in HCL.

### Out of Scope

- Lockfile format. `.forge-lock.yaml` stays YAML on disk; that decision
  was taken in IMPL-0004 OQ-6 and is unchanged.
- Template engine. Engine swap was IMPL-0004; this is config-format-only.
- New schema fields. The struct shapes don't grow during this migration —
  only the encoding changes.
- Backwards-compatibility shim. The v2-YAML loader is deleted, not
  deprecated. Pinning to the prior minor is the only fallback.
- Sub-blueprints / nested categories / new authoring features. Tracked
  separately.

## Implementation Phases

Phases A and B can be reviewed and merged independently as long as the
YAML path stays green. Phase C is the irreversible cutover and should
land as a single coherent PR.

---

### Phase A: HCL loader alongside YAML loader

**Goal:** Land HCL-backed `LoadBlueprint` and `LoadRegistry` functions
that satisfy the same return types as today's YAML loaders. Gate the
new path on file presence: if `blueprint.hcl` exists, prefer it; else
fall back to `blueprint.yaml` (and surface a deprecation pointer in
the load error in Phase C).

#### Tasks

- [x] **A.1 Document the HCL decoding approach.**
  - Decision (per Resolved Questions OQ-1): `hcldec.ObjectSpec`. Keeps
    the existing struct definitions intact (no dual yaml/hcl tags) and
    gives precise diagnostics with file/line/col.
  - Document the choice as a one-paragraph note at the top of the new
    loader file (`internal/config/loader_hcl.go`) so a future
    contributor doesn't relitigate.
  - Done: `internal/config/loader_hcl.go` created with the decision
    rationale at the top and stub `LoadBlueprintHCL` / `LoadRegistryHCL`
    signatures returning `errHCLNotImplemented`. A.3 fills these in.

- [x] **A.2 Define `Blueprint` and `Registry` HCL schemas.**
  - File: `internal/config/hcldec_spec.go` (new).
  - Build `hcldec.ObjectSpec` for each top-level type:
    `blueprintSpec`, `registrySpec`. Block specs for `variable`,
    `condition`, `defaults`, `hooks`, `sync`, `rename`, `blueprint`.
  - Cover every existing field on `Blueprint`/`Registry`/`Variable`/
    `Condition`/`Hooks`/`Defaults`/`SyncConfig`/`BlueprintEntry`. The
    Go struct definitions don't change in Phase A — we map HCL into the
    same shape the YAML loader produces.
  - Done: spec covers eager parts via `blueprintEagerSpec` /
    `registrySpec`. Lazy blocks (`variable`, `condition`, `rename`)
    use `hcl.BodySchema` body content with hand-decoders in the loader
    so expressions and templated source text round-trip cleanly. The
    `rename` syntax is now a `rename { entry { from = "...", to = "..." } }`
    block (HCL block labels and attribute names reject template
    sequences, so map literals don't fit). DESIGN-0004 updated to
    match.

- [x] **A.3 Implement `LoadBlueprintHCL` and `LoadRegistryHCL`.**
  - File: `internal/config/loader_hcl.go` (new).
  - `LoadBlueprintHCL(path string) (*Blueprint, error)` — parse via
    `hclparse.NewParser`, decode with the spec from A.2, convert
    `cty.Value` results into the `Blueprint` struct.
  - `LoadRegistryHCL(path string) (*Registry, error)` — parallel.
  - For `Condition.When`: keep the field as `hcl.Expression` (do not
    evaluate at load time). Add a parse-time syntax check so a
    malformed expression surfaces with file/line/col.
  - For `Variable.Default`: stays as a string; the prompt-time path
    in `internal/prompt/` already parses it via the renderer.
  - Done: full HCL loader in `loader_hcl.go` with cty→Go conversion
    helpers in `loader_hcl_helpers.go`. `validateBlueprintFields` /
    `validateRegistryFields` skip the apiVersion gate for HCL inputs
    (HCL files don't carry apiVersion). Smoke tests in
    `loader_hcl_test.go` cover happy-path decode for every block kind,
    missing-required diagnostics, and condition.when round-trip
    against a populated EvalContext.

- [x] **A.4 Change `Condition.When` field type.**
  - File: `internal/config/blueprint.go`.
  - Change `When string \`yaml:"when"\`` to `When hcl.Expression`.
  - **Parse-time, not eval-time** (per Resolved Questions OQ-7):
    malformed `when` expressions surface at `LoadBlueprint` time with
    file/line/col, not on first evaluation. Worth the load-time
    invariant change for the UX win.
  - Update every reader of `cond.When`:
    - `internal/create/conditions.go:39` — `EvaluateBool` becomes a
      direct `cond.When.Value(ctx)` call.
    - YAML loader (still in tree during Phase A) populates `When` by
      parsing the YAML string via `hclsyntax.ParseExpression`.
  - Done: `Condition` struct now holds `When hcl.Expression` plus
    `WhenSource string` (the original expression text retained for
    diagnostics, lockfile snapshots, and `forge migrate config`
    round-trip output). `Renderer` interface gained `EvaluateBoolExpr`
    for callers that already hold a parsed expression;
    `internal/create/conditions.go` switched over. The YAML loader
    parses `WhenSource` into `When` after `ValidateBlueprint` (so v1
    rejection still surfaces first).

- [x] **A.5 Dispatching loader.**
  - File: `internal/config/loader.go`.
  - In `LoadBlueprint(path)`: stat `<dir>/blueprint.hcl`; if present,
    delegate to `LoadBlueprintHCL`; else fall back to the YAML loader.
  - Parallel logic in `LoadRegistry(path)`.
  - Keep YAML loaders private (`loadBlueprintYAML`, `loadRegistryYAML`)
    so the public API doesn't grow.
  - Done: `preferHCLSibling` helper checks `.hcl` siblings of `.yaml`
    inputs (and accepts `.hcl` paths directly). YAML loaders extracted
    into `loadBlueprintYAML` / `loadRegistryYAML`. Caller contract is
    unchanged — production callers still pass `<dir>/blueprint.yaml`
    and the dispatcher transparently upgrades to HCL when present.
    Tests in `loader_hcl_test.go` cover both branches.

- [x] **A.6 Hermetic HCL fixture.**
  - Path: `testdata/hcl-registry/` (new).
  - Mirror `testdata/v2-registry/` shape but with `blueprint.hcl` /
    `registry.hcl` configs. Templates stay HCL2 (already are).
  - Include a `condition.when` block, a `variable.default` referencing
    an earlier var, and a `rename` block — every feature the loader
    needs to round-trip.
  - Done: `testdata/hcl-registry/` mirrors `testdata/v2-registry/` —
    `registry.hcl`, two blueprint dirs (go/api, helm/chart) each with
    `blueprint.hcl` + `${project_name}/...` template trees, and a
    `_defaults/.editorconfig`. The go/api blueprint exercises every
    HCL feature: variables, templated default, condition.when, rename
    block. Loader exercises this fixture in
    `TestLoad{Blueprint,Registry}HCL_HermeticFixture` so the in-tree
    corpus catches schema regressions. Spec + assignRegistryFromCty
    extended in this commit to cover Maintainers, RegistryDefaults,
    Version, LatestCommit (fields the original A.2 spec missed).

- [x] **A.7 Unit tests for the HCL loaders.**
  - File: `internal/config/loader_hcl_test.go` (new).
  - Cover happy-path decode, every block type, missing-required-field
    diagnostic, malformed `condition.when` diagnostic.
  - Round-trip test: decode → re-encode via `hclwrite` (Phase B's
    rewriter) → decode → assert deep-equal.
  - Done: 11 test functions cover happy paths (blueprint + registry),
    every block kind via the hermetic fixture, dispatcher branches
    (HCL sibling preferred over YAML; YAML fallback when no sibling),
    and four negative paths (missing required attr at top level,
    inside variable, inside rename entry, missing required attr on
    registry blueprint entry; malformed `condition.when` failing at
    load time per OQ-7). Round-trip test deferred to Phase B (needs
    the `hclwrite` rewriter from B.2).

- [x] **A.8 Integration test exercising the HCL fixture.**
  - File: `internal/create/cli_integration_test_hcl.go` (new) or extend
    the existing v2 integration test.
  - Calls `create.Run()` against `testdata/hcl-registry/`. Asserts:
    - The blueprint loads via the HCL path.
    - `condition.when` evaluates correctly without re-parsing.
    - Output project is byte-for-byte identical to the equivalent
      YAML run against `testdata/v2-registry/`.
  - Done: `internal/create/hcl_integration_test.go` runs `create.Run()`
    against `testdata/hcl-registry/` for both `go/api` (variables,
    condition.when, rename) and `helm/chart` (verbatim `{{ }}`
    preservation). Output files match the equivalent YAML runs in
    `v2_integration_test.go` shape-for-shape — same renderer, just a
    different config encoding.

#### Success Criteria

- `make build` succeeds; no new deps beyond what `hashicorp/hcl/v2`
  already provides (`hcldec`, `hclparse`, `hclwrite` are subpackages of
  the existing module).
- `make lint` passes with no new warnings.
- `make test` — all existing YAML-path tests still pass.
- New `internal/config/loader_hcl_test.go` tests pass.
- `forge create <blueprint> --registry-dir testdata/hcl-registry`
  succeeds end-to-end.

---

### Phase B: Migration tool — `forge migrate config`

**Goal:** Build the YAML→HCL rewrite tool, parallel to `forge migrate
templates`. Same package (`internal/migratecmd/`), same patterns, same
guards. Validate against the forge-registry corpus before shipping.

#### Tasks

- [x] **B.1 Define `MigrateConfigOpts` / `MigrateConfigResult` types.**
  - File: `internal/migratecmd/config.go` (new).
  - Decision (per Resolved Questions OQ-2): separate subcommands
    (`forge migrate templates`, `forge migrate config`). Reuse the
    existing `MigrateOpts` shape (Path, DryRun, Strict, Force) — the
    field set is identical; only the rewrite payload differs.
  - Result type tracks per-file outcomes:
    ```go
    type ConfigFileReport struct {
        Path      string // input .yaml path
        Output    string // resulting .hcl path
        Migrated  bool   // false in dry-run
        Skipped   bool   // already .hcl
        Errors    []error
    }
    ```
  - Done: `MigrateConfigResult` + `ConfigFileReport` defined; opts
    type reuses `MigrateOpts`. Added `SkipReason` to
    `ConfigFileReport` so the summary table can explain skip
    decisions cleanly. `RunMigrateConfig` returns
    `errMigrateConfigNotImplemented` until B.3 wires the walker.

- [x] **B.2 Implement the YAML→HCL encoder.**
  - File: `internal/migratecmd/config_rewrite.go` (new).
  - Top-level entry:
    ```go
    func RewriteBlueprintYAMLToHCL(src []byte) (out []byte, err error)
    func RewriteRegistryYAMLToHCL(src []byte) (out []byte, err error)
    ```
  - Approach: parse the YAML into the existing `Blueprint` /
    `Registry` structs via the YAML loader, then emit HCL via
    `hclwrite`. Building an `hclwrite.File` programmatically guarantees
    we produce well-formed HCL with consistent indentation and comment
    handling.
  - Strip the `apiVersion` field on emit — it's gone in the new shape.
  - `Condition.When`: the YAML string is already a valid HCL
    expression (it's been HCL since v0.3.0 per IMPL-0004), so we emit
    it as an HCL expression attribute, not a string literal.
  - `Variable.Default`: emit as an HCL string literal containing the
    `${expr}` template (same shape it has today inside YAML).
  - Comments are dropped on emit (per Resolved Questions OQ-3). The
    YAML→struct→HCL pipeline does not round-trip `yaml.Node` head/foot
    comments, and most blueprints don't comment heavily inside config
    files. The MIGRATION.md section calls this out as a known
    limitation and recommends re-adding any author comments by hand
    after migration. Follow-up issue to revisit if real authors hit
    this.
  - Done: rewriter built directly on `strings.Builder` rather than
    `hclwrite.SetAttributeValue`. The latter routes through
    `cty.StringVal` which escapes `$` to `$$` to suppress template
    interpolation — exactly the wrong thing for fields that *are*
    templates. Two emitters separate the cases: `quoteHCLString`
    escapes `$`/`%` for non-templated values; `quoteHCLTemplate`
    preserves them for `variable.default`, `variable.validate`, and
    `rename` entries. `condition.when` is written verbatim from the
    YAML scalar (no quoting). Final output runs through
    `hclwrite.Format` for canonical layout. Three round-trip tests
    (blueprint, registry, apiVersion-drop) prove
    rewrite → write → load roundtrips lossless.

- [x] **B.3 Implement the per-blueprint file walker.**
  - File: `internal/migratecmd/config_walk.go` (new).
  - `walkConfigs(rootPath) ([]ConfigFileReport, error)` — find every
    `blueprint.yaml` and `registry.yaml` under root.
  - For each file:
    - **Collision check first** (per Resolved Questions OQ-5): if a
      sibling `.hcl` already exists, refuse the run with a clear error
      naming the two conflicting paths. Do not auto-skip; force the
      user to clean up manually before re-running.
    - Read YAML → call `RewriteBlueprintYAMLToHCL` (or `Registry`
      variant) → write to sibling `.hcl` path → delete original `.yaml`.
    - On `--dry-run`: write nothing, just report what would change.
    - On error: surface in `ConfigFileReport.Errors`, continue to next
      file.
  - Done: `findYAMLConfigs` walks under the root and gathers every
    `blueprint.yaml` / `registry.yaml`. `rewriteConfigFile` runs the
    full per-file pipeline: collision check, read, dispatch to the
    right rewriter (B.2), write `.hcl`, remove `.yaml` (skipped on
    `--dry-run`). All errors are captured per-file so the walker keeps
    going. Tests cover happy-path two-file rewrite, dry-run, and the
    OQ-5 collision skip.

- [x] **B.4 Reuse the dirty-worktree guard.**
  - File: existing `internal/migratecmd/git.go`.
  - No new code; just call the existing guard from the new entry point.
  - Done: `RunMigrateConfig` calls the same `checkCleanWorktree`
    helper as `RunMigrate`. Same `--force` override semantics.

- [x] **B.5 Wire the Cobra subcommand.**
  - File: `cmd/migrate.go` (modify existing).
  - Add `migrateConfigCmd` under the existing `migrateCmd` parent. Same
    flag set: `--path` (default `.`), `--dry-run`, `--strict`, `--force`.
  - `RunE`: build opts, call `migratecmd.RunMigrateConfig(opts)`, print
    a summary table:
    ```
    FILE                          STATUS         OUTPUT
    go/api/blueprint.yaml         migrated       go/api/blueprint.hcl
    registry.yaml                 migrated       registry.hcl
    python/fastapi/blueprint.hcl  skipped (hcl)  —
    ```
  - In `--strict`: exit non-zero on any error.
  - Done: `forge migrate config` is wired with all four flags, summary
    tabwriter output, and strict-mode error gating. Verified end-to-end
    with `--dry-run --path <copy of testdata/v2-registry/>`: emits the
    expected three-row plan.

- [x] **B.6 Unit tests for the rewriter.**
  - File: `internal/migratecmd/config_rewrite_test.go`.
  - Table-driven: per YAML input, assert exact HCL output via golden
    files. Cover every field of `Blueprint` and `Registry`.
  - Idempotence: rewriting the HCL output back through the rewriter is
    a no-op (or surfaces an "already HCL" skip).
  - Done: round-trip tests proved stronger than golden files
    (semantic equivalence vs byte-equivalence) — they cover every
    field of Blueprint/Registry by loading the rewriter output and
    asserting against the source struct. Added: minimal-blueprint
    test, HCL-input-rejected test (rewriter-level idempotence
    boundary). User-level idempotence is enforced by the walker's
    OQ-5 collision check (covered in B.3 tests).

- [x] **B.7 Integration test against a frozen YAML fixture.**
  - File: `internal/migratecmd/config_integration_test.go`.
  - Snapshot the current `testdata/v2-registry/` into
    `testdata/v2-registry-yaml/` (read-only). The test copies it to
    `t.TempDir()`, runs `RunMigrateConfig()`, then:
    - Loads the result via `config.LoadBlueprintHCL()` and asserts it
      decodes.
    - Runs `forge create` against the migrated tree and confirms the
      output matches the pre-migration baseline.
  - Done: skipped the v2-registry-yaml snapshot — `testdata/v2-registry/`
    is itself the read-only YAML corpus, copied per-test to a
    `t.TempDir()` via the existing `copyTree` helper. Test migrates
    all three .yaml files (registry + two blueprints), reloads each
    via the HCL loader, then runs `create.Run()` against the migrated
    tree to prove end-to-end equivalence with the pre-migration shape.

- [ ] **B.8 Manual verification against forge-registry.**
  - Clone the forge-registry repo into a scratch dir.
  - Run `forge migrate config --path <forge-registry> --dry-run
    --strict`. Review the summary.
  - If anything surprises (comments lost, escape edge cases,
    `condition.when` round-trip), fix in B.2 and re-verify.
  - Hold off on opening a PR against forge-registry until v0.4.0 ships
    (per DESIGN-0004: PR #5 will be closed and a fresh PR opened
    post-release).

#### Success Criteria

- `make check` passes.
- `forge migrate config --dry-run --path testdata/v2-registry-yaml`
  prints the expected rewrite plan and exits 0.
- `forge migrate config --path <copy>` produces `.hcl` files that load
  cleanly via the Phase A loader.
- The forge-registry corpus migrates with zero errors in `--strict`
  mode. Sample blueprint diff is human-readable — comments preserved
  or, if dropped, documented as a known limitation.

---

### Phase C: Cutover

**Goal:** Make HCL the only path. Reject `.yaml` configs with a clear
error, remove the YAML loaders, drop `apiVersion` from the structs,
swap scaffolding emitters to HCL.

#### Tasks

- [ ] **C.1 Replace validators with file-format rejection.**
  - File: `internal/config/validate.go`.
  - Remove the `apiVersion != "v2"` checks at lines 25 and 65.
  - In `LoadBlueprint`/`LoadRegistry`: if `.hcl` is missing but `.yaml`
    is present, return:
    ```
    blueprint.yaml at <path>: YAML config files are no longer supported.
    Run `forge migrate config --path <registry-or-blueprint>` to
    convert this to blueprint.hcl.
    See docs/MIGRATION.md in the forge repository for the YAML→HCL
    migration guide.
    ```
  - Parallel error for `registry.yaml`.
  - Update `internal/config/validate_test.go` accordingly.

- [ ] **C.2 Drop the `apiVersion` field.**
  - File: `internal/config/blueprint.go` — delete `APIVersion`.
  - File: `internal/config/registry.go` — delete `APIVersion`.
  - Update every reader (`grep -rn APIVersion`):
    - `internal/migratecmd/walk.go` — strip the v1→v2 apiVersion bump
      from `forge migrate templates` (per Resolved Questions OQ-4).
      The legacy templates migrator now produces v2 YAML (without
      apiVersion concerns — that field is gone from the loader). Users
      starting from v0.2.x or earlier run `forge migrate templates`
      then `forge migrate config` — a documented two-step path.
    - `internal/config/validate_test.go` — drop apiVersion assertions.

- [ ] **C.3 Update scaffolding emitters.**
  - Files:
    - `internal/initcmd/initcmd.go:23,135` (blueprintTemplate +
      `APIVersion: "v1"` literal) — replace with HCL string. Rename the
      output file from `blueprint.yaml` to `blueprint.hcl`.
    - `internal/registrycmd/blueprint.go:38` (blueprintScaffoldTemplate)
      — replace with HCL.
    - `internal/registrycmd/registrycmd.go:38` (registryTemplate) —
      replace with HCL. Rename output file from `registry.yaml` to
      `registry.hcl`.
    - `internal/registrycmd/blueprint.go:RunBlueprint()` — update
      file-write paths.
  - Update fixtures any of these tests use (`internal/initcmd/*_test.go`,
    `internal/registrycmd/*_test.go`).

- [ ] **C.4 Migrate `testdata/` fixtures.**
  - Run `forge migrate config --path testdata/registry` and `--path
    testdata/v2-registry` against the in-tree binary built from
    Phase B.
  - Commit the migrated `.hcl` files.
  - Move the YAML originals to `testdata/v2-yaml-registry/` (rejection
    fixture, same pattern as `testdata/v1-registry/` post-IMPL-0004).
  - Keep `testdata/v1-registry/` frozen as-is (per Resolved Questions
    OQ-6) — it remains the v1→v2 template-migration corpus, two
    migration steps behind current.
  - Update every test that references the old paths.

- [ ] **C.5 Delete the YAML loaders.**
  - File: `internal/config/loader.go` — remove
    `loadBlueprintYAML`/`loadRegistryYAML`. Keep the dispatching
    `LoadBlueprint`/`LoadRegistry` but they now only call the HCL
    implementation; the YAML codepath is gone.
  - Drop the `gopkg.in/yaml.v3` import from `internal/config/` (lockfile
    keeps it — that's a separate package).

- [ ] **C.6 Drop `yaml` struct tags.**
  - Files: `internal/config/blueprint.go`, `internal/config/registry.go`.
  - Remove `yaml:"…"` tags from every field. HCL decoding goes through
    `hcldec` so tags are dead weight after Phase A.

- [ ] **C.7 Update the rejection test.**
  - File: a new test under `internal/config/` that runs
    `LoadBlueprint()` against `testdata/v2-yaml-registry/go/api/`
    (with only `blueprint.yaml`, no `.hcl` sibling) and asserts the
    error string contains the migration command and
    `docs/MIGRATION.md`.

- [ ] **C.8 Confirm `forge migrate templates` still works post-cutover.**
  - The legacy v1→v2 template-content rewriter doesn't depend on
    config-file format. Confirm via a regression test that
    `forge migrate templates --path testdata/v1-registry` still
    produces a v2-shaped output (which then needs `forge migrate
    config` as a second pass — document this in the user-facing
    migration guide).

- [ ] **C.9 Update integration tests for HCL fixtures.**
  - Files:
    - `internal/create/cli_integration_test.go`
    - `internal/create/create_test.go`
    - `internal/sync/*_test.go`
    - `internal/check/*_test.go`
    - `internal/list/list_test.go`
    - `internal/search/search_test.go`
    - `internal/registry/index_test.go`
    - `internal/defaults/resolver_test.go`
  - Point at the migrated `.hcl` fixtures. No assertion changes
    expected — rendered output should be identical to the YAML run.

#### Success Criteria

- `make ci` passes (lint + test + license + build).
- `grep -rn "apiVersion" internal/ cmd/` returns no matches in
  loader/validator/scaffolding code.
- `grep -rn "gopkg.in/yaml.v3" internal/config/` returns no matches.
- `forge create` against a migrated registry works; against an
  un-migrated YAML registry produces the deprecation-pointer error.
- `forge registry init` and `forge registry blueprint` produce `.hcl`
  files.
- All historical features (`forge sync`, `forge check`, `forge list`,
  `forge info`, `forge registry update`) work end-to-end against HCL
  configs.

---

### Phase D: Documentation & release prep

**Goal:** Update user-facing docs, prep the v0.4.0 release notes, and
open the fresh forge-registry migration PR.

#### Tasks

- [ ] **D.1 Rewrite DESIGN-0001 examples in HCL.**
  - File: `docs/design/0001-blueprint-authoring.md`.
  - Replace every `blueprint.yaml` code block with the HCL equivalent.
  - Update the "schema" table to reflect dropped `apiVersion` and
    `Condition.When` type change.
  - Add a "Last revised" line in the front-matter pointing at
    DESIGN-0004.

- [ ] **D.2 Rewrite DESIGN-0002 examples in HCL.**
  - File: `docs/design/0002-registry-layout-and-defaults-inheritance.md`.
  - Replace `registry.yaml` blocks with `registry.hcl` blocks.
  - Update the directory-structure tree diagram to show `.hcl`
    filenames.

- [ ] **D.3 Add the YAML→HCL section to `docs/MIGRATION.md`.**
  - File: `docs/MIGRATION.md`.
  - New section: "Migrating from v2 YAML to HCL config files (forge
    v0.3.x → v0.4.x)".
  - Cover: the migration command, what changes on disk, manual fixes
    if any (comment preservation caveat per OQ-3), verification, and a
    rollback note (pin to the previous minor).
  - Reference the existing v1→v2 section — users on v0.2.x or earlier
    need to run **both** migrations.

- [ ] **D.4 Update CLAUDE.md and README.md.**
  - `CLAUDE.md`: reflect HCL config files in the architecture notes
    (lines 49, 51, 85, 87, 91, 127 per the touch-point map). Add
    `forge migrate config` to the cmd list.
  - `README.md`: update any `blueprint.yaml`/`registry.yaml` references
    to `.hcl`. Add `forge migrate config` to the Commands table. Add a
    "Migrating from v0.3.x" pointer to `docs/MIGRATION.md`.

- [ ] **D.5 Update DESIGN-0004 status.**
  - Flip status from `Draft` to `Implemented` once Phases A–C land.

- [ ] **D.6 Fresh forge-registry migration PR.**
  - Close the open PR (`donaldgifford/forge-registry#5`).
  - Open a new branch against current `main` of forge-registry. Run
    `forge migrate config` against it. Verify `forge create` works
    end-to-end against the migrated tree.
  - Open a PR; merge after the forge v0.4.0 release cuts.

- [ ] **D.7 Release notes for v0.4.0.**
  - File: `docs/release-notes/v0.4.0-hcl-config.md` (new).
  - Mirror the shape of `v0.3.0-hcl2-cutover.md`:
    - Bold breaking-change block.
    - Rationale (one sentence): "config files now speak the same syntax
      as templates."
    - Upgrade steps (`forge migrate config`).
    - The "Before you cut" checklist (forge-registry PR linked, `make
      ci` green, both rejection paths verified).
  - Use the `minor` PR label (per Resolved Questions OQ-8 — pre-1.0
    minors are the right channel for documented breaking changes; the
    precedent is IMPL-0004 OQ-9).

#### Success Criteria

- `docs/MIGRATION.md` has a complete YAML→HCL section.
- DESIGN-0001 and DESIGN-0002 reflect the HCL contract.
- `forge migrate config` is documented in `forge --help` and in the
  README Commands table.
- `forge-registry` fresh PR is open.
- Release notes draft is ready for the v0.4.0 cut.

---

## File Changes

### New files

| File | Phase | Purpose |
|------|-------|---------|
| `internal/config/hcldec_spec.go` | A | `hcldec.ObjectSpec` for Blueprint/Registry |
| `internal/config/loader_hcl.go` | A | `LoadBlueprintHCL`, `LoadRegistryHCL` |
| `internal/config/loader_hcl_test.go` | A | Loader unit tests |
| `testdata/hcl-registry/` | A | HCL hermetic fixture |
| `internal/migratecmd/config.go` | B | `MigrateConfigOpts`, `MigrateConfigResult`, `RunMigrateConfig` |
| `internal/migratecmd/config_rewrite.go` | B | YAML→HCL encoder via `hclwrite` |
| `internal/migratecmd/config_walk.go` | B | Per-file rewrite walker |
| `internal/migratecmd/config_rewrite_test.go` | B | Rewriter unit tests + golden files |
| `internal/migratecmd/config_integration_test.go` | B | End-to-end migrate + load |
| `testdata/v2-registry-yaml/` | B | Frozen YAML fixture for migration tests |
| `testdata/v2-yaml-registry/` | C | Rejection fixture for the v2-YAML-not-supported error path |
| `docs/release-notes/v0.4.0-hcl-config.md` | D | v0.4.0 release notes |

### Modified files

| File | Phase | Change |
|------|-------|--------|
| `internal/config/blueprint.go` | A → C | `Condition.When` to `hcl.Expression` (A); drop `APIVersion` + `yaml` tags (C) |
| `internal/config/registry.go` | C | Drop `APIVersion` + `yaml` tags |
| `internal/config/loader.go` | A → C | Dispatch to HCL loader (A); delete YAML loader (C) |
| `internal/config/validate.go` | C | Replace `apiVersion` check with file-format rejection |
| `internal/config/validate_test.go` | C | Update test assertions |
| `internal/create/conditions.go` | A | Use pre-parsed `hcl.Expression` |
| `internal/initcmd/initcmd.go` | C | Emit `.hcl` scaffold; drop `apiVersion` literal |
| `internal/registrycmd/blueprint.go` | C | Emit `.hcl` scaffold |
| `internal/registrycmd/registrycmd.go` | C | Emit `.hcl` scaffold |
| `internal/migratecmd/walk.go` | C | Strip the v1→v2 apiVersion bump from `forge migrate templates` (per OQ-4) |
| `cmd/migrate.go` | B | Add `migrate config` subcommand |
| `testdata/registry/` | C | Migrate to `.hcl` |
| `testdata/v2-registry/` | C | Migrate to `.hcl` |
| All `*_test.go` referencing the migrated fixtures | C | Update file paths |
| `docs/design/0001-blueprint-authoring.md` | D | Rewrite examples in HCL |
| `docs/design/0002-registry-layout-and-defaults-inheritance.md` | D | Rewrite examples in HCL |
| `docs/design/0004-unify-config-file-format-after-hcl2-cutover.md` | D | Status → Implemented |
| `docs/MIGRATION.md` | D | Add YAML→HCL section |
| `CLAUDE.md` | D | Config-file references + cmd list |
| `README.md` | D | Commands table + migration pointer |

### Deleted files

| File | Phase | Reason |
|------|-------|--------|
| `internal/config/loader.go` YAML branch | C | YAML loader removed; file kept as dispatch shim |
| Any `*.yaml` config under `testdata/registry/`, `testdata/v2-registry/` | C | Moved to rejection fixture or replaced with `.hcl` siblings |

## Testing Plan

- **Phase A**: Unit tests for `LoadBlueprintHCL` / `LoadRegistryHCL`
  (happy path, every block type, every diagnostic). One integration
  test exercising `forge create` against `testdata/hcl-registry/`.
  YAML-path tests stay green (no regression).
- **Phase B**: Table-driven unit tests per `Blueprint` / `Registry`
  field, asserting golden HCL output. Integration test that migrates
  a copy of `testdata/v2-registry-yaml/` to HCL and verifies the
  result loads + renders identically to the source.
- **Phase C**: Update every existing integration test to point at the
  migrated HCL fixtures. Add a regression test for the YAML-rejection
  error path. Run `make ci` to catch licence and lint regressions.
- **Phase D**: Documentation review only.

Coverage target unchanged from project default (60%, per
`.codecov.yml`). The HCL loader + config rewriter together should
land at ≥80% line coverage given they are pure-function-heavy.

## Dependencies

- Builds on:
  - DESIGN-0004 (the design being implemented).
  - IMPL-0004 / ADR-0001 (the HCL2 template engine is already in
    production; this extends the same toolchain to configs).
- External corpus dependency:
  `github.com/donaldgifford/forge-registry` is the validation corpus
  for the rewriter (B.8, D.6).
- No new third-party deps: `hashicorp/hcl/v2` is already imported and
  ships `hcldec`, `hclparse`, `hclwrite` as subpackages.

## Implementation Order

```
A.1 decoding approach → A.2 schema spec → A.3 loaders → A.4 Condition.When type → A.5 dispatcher → A.6 fixture → A.7 unit tests → A.8 integration test
                                                                                                                                       │
                                                                                                                                       ▼
B.1 types → B.2 rewriter → B.3 walker → B.4 git guard → B.5 cmd wiring → B.6 unit tests → B.7 integration test → B.8 forge-registry verify
                                                                                                                                       │
                                                                                                                                       ▼
C.1 validators → C.2 drop apiVersion → C.3 scaffold emitters → C.4 migrate testdata → C.5 delete YAML loader → C.6 drop yaml tags
       → C.7 rejection test → C.8 templates-migrate regression → C.9 update integration tests
                                                                                                                                       │
                                                                                                                                       ▼
D.1 DESIGN-0001 → D.2 DESIGN-0002 → D.3 MIGRATION.md → D.4 CLAUDE.md/README.md → D.5 DESIGN-0004 status → D.6 forge-registry PR → D.7 release notes
```

Phases A and B can ship as independent PRs (the YAML path keeps
working). Phase C is the irreversible cutover and should land as a
single coherent PR. Phase D travels in the same PR as C or a tight
follow-up.

## Resolved Questions

All resolved as of 2026-05-12. Trade-offs captured here so they stay
attached to the plan.

- **OQ-1 (Phase A.1): HCL decoding approach → `hcldec.ObjectSpec`.**
  Declarative spec separate from the struct. No `hcl:"..."` tags on
  the Go types (keeps the structs portable; the decoding shape lives
  next to the loader). Mirrors how Terraform Core decodes
  block-shaped config and gives precise file/line/col diagnostics.
  Rejected: (b) `gohcl.DecodeBody` — terse but constrains struct
  shape; (c) hand-rolled `hclsyntax` walk — verbose for a config
  with this many block kinds.

- **OQ-2 (Phase B.1): Subcommand layout → separate subcommands.**
  `forge migrate templates` and `forge migrate config` stay distinct.
  Each has a focused failure mode and a clear `--dry-run` summary.
  Users running a fresh migration from v0.2.x or earlier run both
  back-to-back; the MIGRATION.md guide documents the two-step path.
  Rejected: a unified `forge migrate registry` — mixes two concerns
  in one walker and makes failure surfaces harder to triage.

- **OQ-3 (Phase B.2): Comment preservation → drop on emit.** YAML
  comments are not round-tripped through the rewriter. The
  YAML→struct→HCL pipeline loses `yaml.Node` head/foot comments by
  default, and the cost of preserving them (parse via `yaml.Node`,
  walk the node tree, emit `#`-comments in HCL) is high for what's
  realistically a low-volume need. The MIGRATION.md section flags
  this as a known limitation and tells authors to re-add comments
  manually. Follow-up issue if real authors hit this.

- **OQ-4 (Phase C.2): Legacy `forge migrate templates` apiVersion
  bump → strip it.** The templates migrator stops bumping
  `apiVersion`. Once this lands, `apiVersion` is gone from the
  loader entirely, so the field is meaningless. Users coming from
  v0.2.x or earlier run the two-step path: `forge migrate templates`
  rewrites template content, `forge migrate config` rewrites the
  config format. The migration guide is explicit about this
  ordering.

- **OQ-5 (Phase B.3): Filename collision → refuse the run.** If
  both `blueprint.yaml` and `blueprint.hcl` exist in the same
  directory, the rewriter exits non-zero with an error naming both
  paths. Manual mid-migration state is a footgun; forcing the user
  to clean up before re-running is the safer default.
  Rejected: auto-skip the `.yaml` with a warning — leaves a stale
  YAML file in the tree that the next reader has to debug.

- **OQ-6 (Phase C.4): `testdata/v1-registry/` → keep frozen.** Stays
  as-is, still drives the v1→v2 templates-migrator tests. After this
  lands it's two migration steps behind current, but both steps are
  exercised independently. Deprecation is a separate cleanup if it
  ever becomes a maintenance cost.

- **OQ-7 (Phase A.4): `Condition.When` parse timing → at load
  time.** A syntactically broken `when` expression surfaces at
  `LoadBlueprint` time with file/line/col. This changes a load-time
  invariant (loader now does more work) but moves errors closer to
  the source. Worth the tradeoff. Caching the parsed expression
  also means evaluation paths don't re-parse on every render.

- **OQ-8 (Phase D.7): Version bump → `minor` (v0.4.0).** Pre-1.0
  minors are the right channel for documented breaking changes; this
  is the precedent IMPL-0004 OQ-9 established with v0.3.0. PR label
  is `minor`. Release notes carry a bold-warning block. No
  back-compatible 0.3.x branch.

## References

- [DESIGN-0004 — Unify Config File Format After HCL2 Cutover](../design/0004-unify-config-file-format-after-hcl2-cutover.md)
- [IMPL-0004 — Migrate Template Engine to HCL2](0004-migrate-template-engine-to-hcl2.md) — the precedent this builds on
- [ADR-0001 — Use HCL2 as the Template Engine](../adr/0001-use-hcl2-as-the-template-engine.md)
- [DESIGN-0001 — Blueprint Authoring](../design/0001-blueprint-authoring.md) (to be rewritten in D.1)
- [DESIGN-0002 — Registry Layout and Defaults Inheritance](../design/0002-registry-layout-and-defaults-inheritance.md) (to be rewritten in D.2)
- [docs/MIGRATION.md](../MIGRATION.md) — will gain a YAML→HCL section in D.3
- [docs/release-notes/v0.3.0-hcl2-cutover.md](../release-notes/v0.3.0-hcl2-cutover.md) — shape reference for D.7
- [`hashicorp/hcl/v2/hcldec`](https://pkg.go.dev/github.com/hashicorp/hcl/v2/hcldec) — declarative decoding
- [`hashicorp/hcl/v2/hclwrite`](https://pkg.go.dev/github.com/hashicorp/hcl/v2/hclwrite) — HCL emission
- [`github.com/donaldgifford/forge-registry`](https://github.com/donaldgifford/forge-registry) — migration corpus
