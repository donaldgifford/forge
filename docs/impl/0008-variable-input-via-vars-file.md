---
id: IMPL-0008
title: "Variable input via vars file"
status: Draft
author: Donald Gifford
created: 2026-05-19
---
<!-- markdownlint-disable-file MD025 MD041 -->

# IMPL 0008: Variable input via vars file

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-05-19

<!--toc:start-->
- [Objective](#objective)
- [Scope](#scope)
  - [In Scope](#in-scope)
  - [Out of Scope](#out-of-scope)
- [Implementation Phases](#implementation-phases)
  - [Phase A: internal/varsfile/ package](#phase-a-internalvarsfile-package)
    - [Tasks](#tasks)
    - [Success Criteria](#success-criteria)
  - [Phase B: --var-file on forge create](#phase-b---var-file-on-forge-create)
    - [Tasks](#tasks-1)
    - [Success Criteria](#success-criteria-1)
  - [Phase C: --var-file on forge sync](#phase-c---var-file-on-forge-sync)
    - [Tasks](#tasks-2)
    - [Success Criteria](#success-criteria-2)
  - [Phase D: --var-file on forge check](#phase-d---var-file-on-forge-check)
    - [Tasks](#tasks-3)
    - [Success Criteria](#success-criteria-3)
  - [Phase E: Documentation and release prep](#phase-e-documentation-and-release-prep)
    - [Tasks](#tasks-4)
    - [Success Criteria](#success-criteria-4)
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

Implement [DESIGN-0005 — Variable input via vars file](../design/0005-variable-input-via-vars-file.md):
add a `--var-file PATH` flag to `forge create` (then `forge sync` and
`forge check`) that loads variable values from one or more HCL
documents. `--var-file` is repeatable, composes left-to-right with
last-wins semantics on key collision, and is mutually exclusive with
`--set` on any single invocation.

The work introduces a new `internal/varsfile/` package, wires the
flag into the three commands, and updates docs. No schema or lockfile
changes — vars files are an additional *input path* into the
existing resolution flow that already terminates in
`lockfile.ToCtyValues`.

**Implements:** [DESIGN-0005 — Variable input via vars file](../design/0005-variable-input-via-vars-file.md)

## Scope

### In Scope

- New `internal/varsfile/` package: parser + composer for one or
  more `.forge-vars.hcl` files, producing a
  `map[string]cty.Value` keyed by variable name. Strict literals
  only — no cty stdlib function calls (per OQ-1).
- `--var-file` flag on `forge create` and `forge sync`
  (StringArrayVar — repeatable). On `forge check` the flag is
  registered solely to emit a clear "not supported on check" error
  (per OQ-5).
- `.hcl` extension is **required** for every path passed to
  `--var-file` (per OQ-8). No process-substitution / stdin path —
  callers must materialise inputs as real `.hcl` files.
- Mutual-exclusion enforcement between `--var-file` and `--set` via
  a manual check in each command's `RunE`, surfacing the
  DESIGN-0005 error message (per OQ-2).
- Integration into the existing variable-resolution flow:
  `--var-file` values flow as `map[string]cty.Value` end-to-end via
  a new `Opts.VarsFileValues` field on `create.Opts` (per OQ-3).
  Vars-file values short-circuit prompts the same way `--set`
  overrides do today.
- On `forge sync`, `--var-file` overrides require `--force` to
  rewrite the lockfile (per OQ-4). Without `--force`, error.
- Unknown-key warning (vars file declares a value for a variable
  not present in the blueprint): emit a stderr warning listing the
  unknown keys; do not error. No `--strict-vars` flag in v1
  (per OQ-7).
- Type-coercion errors that surface HCL source file:line:column.
- Hermetic testdata fixtures (sample `.forge-vars.hcl` files).
- Documentation: README Quick Start, MIGRATION.md "preferred input
  pattern" note, command help text.

### Out of Scope

- **Auto-loading.** No `*.auto.forge-vars.hcl` discovery (DESIGN-0005
  Non-Goals, OQ-1 decided No).
- **Stdin support via `--var-file -`** (DESIGN-0005 OQ-2 decided No).
- **Process-substitution paths via `--var-file <(...)`.** Originally
  documented in DESIGN-0005 as the inline-override workaround;
  dropped here per IMPL-0008 OQ-8 — strict `.hcl` extension makes
  parsing simpler and avoids special-casing `/dev/fd/*` paths.
  Users who want inline overrides write to a real `.hcl` tempfile.
- **JSON variant `.forge-vars.json`** (DESIGN-0005 Non-Goals — a
  follow-up).
- **HCL function calls inside vars files** (OQ-1) — strict
  literals only.
- **Object / list / map values.** This IMPL ships with scalar-only
  parsing; the parser is structured so RFC-0002's types drop in via
  the existing `cty.Value` plumbing without re-architecting.
- **`--strict-vars` flag** to promote unknown-key warnings to
  errors (OQ-7) — not in v1.
- **Recording the vars file path in the lockfile** (DESIGN-0005
  OQ-3 decided No).
- **Combining `--var-file` and `--set`** in any form (DESIGN-0005
  OQ-4 decided No).
- **`forge check --var-file FILE`** as a meaningful drift-detection
  operation (OQ-5) — the flag is registered only to emit a clear
  rejection error rather than Cobra's generic "unknown flag" diag.

## Implementation Phases

Five phases. Phases A and B together constitute the MVP (`forge
create` only — DESIGN-0005 Migration/Rollout Plan step 1). Phases
C and D extend the surface to `forge sync` and `forge check` in the
next minor release. Phase E lands docs alongside whichever release
introduces the flag.

---

### Phase A: `internal/varsfile/` package

Establish the parser + composer foundation. This phase produces a
self-contained package that any command can consume; no CLI wiring
yet.

#### Tasks

- [x] **A.1 Create the package directory and stub file.**
  - File: `internal/varsfile/varsfile.go` (new).
  - Package comment summarising responsibility: "parse one or more
    `.forge-vars.hcl` files into a `map[string]cty.Value` keyed by
    variable name, scoped to a blueprint's declared variables."

- [x] **A.2 Define the public API.**
  - File: `internal/varsfile/varsfile.go` (modify).
  - Exported entry point (proposed signature):

    ```go
    // Load parses every path in order and merges the resulting
    // attribute maps left-to-right. Later files override earlier
    // files on key collision. Values are coerced against the
    // declared variable types in `vars`. Unknown keys (present in
    // a file but not declared in `vars`) are returned in the
    // `unknown` slice for the caller to surface as a warning.
    func Load(paths []string, vars []config.Variable) (
        resolved map[string]cty.Value,
        unknown  []string,
        err      error,
    )
    ```
  - The exported function is the *only* entry point — internal
    helpers (parse, merge, coerce) stay unexported.

- [x] **A.3 Implement per-file parsing.**
  - Internal helper: `parseFile(path string) (hcl.Attributes, hcl.Diagnostics)`.
  - **Reject paths that don't end in `.hcl`** with a clear error
    before opening the file (per OQ-8). Message: "vars file 'X':
    only .hcl extensions are supported; got 'Y'."
  - Uses `hashicorp/hcl/v2/hclparse` (mirrors
    `internal/config/loader_hcl.go` lines 69–74).
  - Reads the file as bytes, calls `parser.ParseHCL(src, path)`,
    returns `file.Body.JustAttributes()` — vars files are
    attribute-only (no blocks allowed per DESIGN-0005 File Format).
  - Rejects bare blocks with a clear error: "vars files contain
    attribute assignments only, not blocks; got block 'X' at
    file:line:col."

- [x] **A.4 Implement composition (multiple files).**
  - Internal helper: `compose(files []hcl.Attributes) hcl.Attributes`.
  - Walks files left-to-right; later attributes overwrite earlier
    on key collision. Preserves the source `hcl.Attribute` (with
    its expression and range) for the value that wins — so type
    coercion errors point at the file that actually supplied the
    value.

- [x] **A.5 Implement type coercion against declared types.**
  - Internal helper: `coerce(attrs hcl.Attributes, vars []config.Variable) (map[string]cty.Value, []string, error)`.
  - For each declared variable in `vars`, if the attribute is
    present:
    - Evaluate the attribute's expression in an empty `hcl.EvalContext`
      (per OQ-1: strict literals only — no functions, no
      variables, no traversals; an expression that requires
      evaluation context is rejected with a clear "vars files only
      accept literal values" error at file:line:col).
    - Use the declared type tag (`"string"` / `"bool"` / `"int"` /
      `"choice"`) to drive coercion through the same logic as
      `lockfile.ToCtyValues` (`internal/lockfile/cty.go` lines 20–49).
    - Coercion errors include the file:line:col of the source
      attribute and the declared type for the variable.
  - For attributes present in the file but absent from `vars`:
    collect into the `unknown` slice.
  - Variables absent from the file: leave unset (callers fall
    through to prompt / default).

- [x] **A.6 Hermetic test fixtures.**
  - File: `internal/varsfile/testdata/basic.forge-vars.hcl` (new).
  - File: `internal/varsfile/testdata/override.forge-vars.hcl` (new).
  - File: `internal/varsfile/testdata/wrong-type.forge-vars.hcl` (new).
  - File: `internal/varsfile/testdata/malformed.forge-vars.hcl` (new).
  - File: `internal/varsfile/testdata/with-blocks.forge-vars.hcl` (new).
  - File: `internal/varsfile/testdata/unknown-keys.forge-vars.hcl` (new).
  - File: `internal/varsfile/testdata/with-function-call.forge-vars.hcl` (new) — covers OQ-1 rejection (`project_name = upper("foo")`).
  - File: `internal/varsfile/testdata/bad-extension.vars` (new) — covers OQ-8 rejection (correct content, wrong extension).
  - Each fixture is minimal — single-purpose for one test case.

- [x] **A.7 Unit tests.**
  - File: `internal/varsfile/varsfile_test.go` (new).
  - Test cases (table-driven where structure allows):
    - `Load` happy path: single file, three scalar vars, declared
      types match → expected `cty.Value` map.
    - Composition: `Load([base, override], ...)` → override values
      win; unchanged keys preserved from base.
    - Composition with three files: last wins all the way.
    - Type coercion: `"42"` → declared int → `cty.NumberIntVal(42)`.
    - Type coercion error: `"not a bool"` → declared bool →
      error includes `wrong-type.forge-vars.hcl:N,M`.
    - Unknown keys: file declares `foo = "bar"`, blueprint declares
      no `foo` → returned in `unknown` slice, no error.
    - Malformed HCL: parse error includes file:line:col.
    - Bare blocks rejected: `with-blocks.forge-vars.hcl` →
      "attribute assignments only" error.
    - Function call rejected: `with-function-call.forge-vars.hcl`
      → "literal values only" error (OQ-1).
    - Bad extension rejected: `bad-extension.vars` →
      ".hcl extensions only" error (OQ-8).
    - Missing file: clear "no such file" wrap, no panic.
    - Empty file: returns empty map, no error.
  - Use `testify` assertions per CLAUDE.md.

- [x] **A.8 Run `make lint` and `make fmt`.**
  - Result: coverage 97.1% (target >=90%); 0 lint issues.

#### Success Criteria

- `go build ./...` succeeds.
- `make test-pkg PKG=./internal/varsfile` passes.
- Package has no exported helpers beyond `Load`.
- All unit tests cover the happy path + each error class.
- `make lint` green (no new linter warnings).
- Coverage target: >=90% for the new package (it's a small,
  well-defined surface).

---

### Phase B: `--var-file` on `forge create`

Wire the new package into the `forge create` command. Mutual
exclusion with `--set`, integration with `prompt.CollectVariables`.

#### Tasks

- [x] **B.1 Add the `--var-file` flag to `forge create`.**
  - File: `cmd/create.go` (modify around lines 20–50).
  - Declare `varFiles []string` alongside the existing `setVars`.
  - Register: `createCmd.Flags().StringArrayVar(&varFiles, "var-file", nil, "Load variable values from an HCL document. Repeatable; later files override earlier ones on key collision. Mutually exclusive with --set.")`.

- [x] **B.2 Enforce mutual exclusion with `--set` (manual check
      per OQ-2).**
  - File: `cmd/create.go` (modify).
  - Inside `RunE`, after flag parsing:

    ```go
    if len(setVars) > 0 && len(varFiles) > 0 {
        return errors.New(
            "--var-file and --set cannot be combined. " +
            "Use one input source per invocation:\n" +
            "  - For one-off overrides: forge create … --set k=v\n" +
            "  - For multiple values:   forge create … --var-file path/to/foo.forge-vars.hcl",
        )
    }
    ```
  - Verbatim match to DESIGN-0005 Mutual Exclusion section.
  - Same pattern is repeated in `cmd/sync.go` (Phase C); factor
    into a shared helper in `cmd/internal/` if duplication grows
    annoying, but inline duplication is fine for two call sites.

- [x] **B.3 Load the vars file(s) and flow values into `create.Run`
      (per OQ-3 — extend `Opts` rather than stringify).**
  - Implementation note: rather than load in the CLI and pass the
    resolved `map[string]cty.Value` in, the CLI passes the raw
    `--var-file` *paths* via a new `Opts.VarsFiles []string` field
    and `create.Run` calls `varsfile.Load(opts.VarsFiles,
    bp.Variables)` itself. This keeps the load co-located with the
    blueprint parse (which has to happen first to know the declared
    types) and means the CLI doesn't have to load the blueprint
    twice. Unknown keys flow back to the CLI on a new
    `Result.UnknownVarsFileKeys` field, where `cmd/create.go`
    surfaces them via `ui.Warningf`.
  - File: `internal/create/create.go` (modify).
  - Extend `create.Opts` with a new field:

    ```go
    // VarsFileValues holds variable values loaded from one or more
    // --var-file inputs. Mutually exclusive with Overrides at the
    // CLI layer.
    VarsFileValues map[string]cty.Value
    ```
  - File: `cmd/create.go` (modify).
  - When `len(varFiles) > 0`, call
    `varsfile.Load(varFiles, bp.Variables)` and pass the resolved
    map into `Opts.VarsFileValues`.
  - Surface unknown keys via `ui.Warning` styled output
    (`internal/ui/`).

- [x] **B.4 Integrate with `prompt.CollectVariables`.**
  - File: `internal/prompt/prompt.go` (modify) and/or
    `internal/create/create.go` (modify) — pick whichever layer is
    cleanest given the existing signature.
  - Vars-file values must short-circuit the prompt (same behaviour
    as `--set` overrides). The existing overrides-skip-prompt path
    (`internal/prompt/prompt.go` lines 30–50) becomes the model.
  - Two implementation options:
    1. **Extend `CollectVariables` signature** to take a second map
       (`varsFileValues map[string]cty.Value`). For each variable:
       check vars-file first → check overrides → render default →
       prompt. Per OQ-3 the types stay `cty.Value` on the vars-file
       branch.
    2. **Pre-merge in `create.Run`** before calling
       `CollectVariables`: synthesise an `overrides` equivalent
       from `VarsFileValues` by stringifying — but this contradicts
       OQ-3 (keep types) so reject this option.
  - Go with option 1.
  - If a vars file supplies a key that the blueprint declares, the
    prompt is skipped for that key. If a required variable is not
    in the vars file and we're in non-interactive mode (e.g.
    `--defaults`), the existing "missing required variable" error
    path fires unchanged.

- [x] **B.5 Type-coercion error surfacing.**
  - Errors from `varsfile.Load` must be returned from the command
    `RunE` so Cobra prints them with the standard non-zero exit.
  - Verify the error message renders with file:line:col against a
    real fixture (use the `wrong-type.forge-vars.hcl` fixture from
    Phase A but run end-to-end through `forge create`).

- [x] **B.6 Integration tests.**
  - File: `cmd/create_test.go` (modify or new — confirm naming
    convention via `cmd/` listing).
  - Test cases:
    - `forge create … --var-file basic.forge-vars.hcl --defaults
      --force` produces the same scaffold as the equivalent
      `--set` invocation.
    - `--var-file foo --var-file bar`: override semantics
      end-to-end against the hermetic HCL fixture
      (`testdata/hcl-registry/`).
    - `--var-file foo --set k=v`: error matches the expected
      mutual-exclusion message; exit code non-zero.
    - `--var-file path-to-file-with-unknown-keys`: scaffold
      succeeds; stderr contains the warning listing the unknown
      keys.
    - `--var-file path-to-file-with-wrong-types`: scaffold fails
      with file:line:col coercion error; no scaffold output
      written.
    - `--var-file foo` partial (declares some required vars, omits
      others): in interactive mode the omitted ones prompt; in
      `--defaults` mode the existing "missing required variable"
      error path fires.

- [x] **B.7 Add the testdata fixtures for the integration tests.**
  - Integration tests use `t.TempDir()` + inline strings rather
    than a `cmd/testdata/varsfile/` corpus — the fixtures are
    small enough to read inline at the test site, and using
    TempDir keeps them hermetic without an extra directory to
    maintain.
  - Files under `testdata/hcl-registry/` (use the existing
    `hcl-registry/go/api/` blueprint or add a small dedicated
    blueprint for vars-file tests).
  - Files under `cmd/testdata/varsfile/` (proposed; verify with
    convention used by existing tests).

- [x] **B.8 Run `make ci`.**

#### Success Criteria

- `forge create … --var-file FILE` end-to-end produces an
  equivalent scaffold to `forge create … --set k=v` with the same
  effective inputs (lockfile content byte-equal modulo
  timestamps).
- `forge create … --var-file F --set k=v` errors with the
  documented mutual-exclusion message; no scaffold written.
- `--var-file` with unknown keys produces a stderr warning but
  succeeds; the lockfile contains only declared-variable values.
- `--var-file` with type-mismatched values fails with a clear
  HCL-source-located error.
- `forge create --help` documents `--var-file` in the standard
  Cobra help layout.
- `make ci` green.

---

### Phase C: `--var-file` on `forge sync`

Extend the flag to `forge sync`. Per OQ-4: `--var-file` overrides
lockfile values **only when `--force` is also passed**. Without
`--force`, the sync errors out before doing any work — sync's
default behaviour stays "lockfile is the source of truth."

#### Tasks

- [x] **C.1 Add the `--var-file` flag to `forge sync`.**
  - File: `cmd/sync.go` (modify).
  - StringArrayVar registration mirroring create.go.

- [x] **C.2 Enforce mutual exclusion with `--set` on sync.**
  - `forge sync` does not currently have a `--set` flag. Decision:
    do not add `--set` to sync as part of this IMPL (out of scope
    — sync stays lockfile-driven by default). The mutual-exclusion
    check collapses to "no work needed" here; if sync ever gains
    `--set`, that future IMPL adds the matching check.

- [x] **C.3 Enforce `--force` requirement for `--var-file` on sync
      (per OQ-4).**
  - File: `cmd/sync.go` (modify).
  - Inside `RunE`, after flag parsing:

    ```go
    if len(varFiles) > 0 && !force {
        return errors.New(
            "--var-file on `forge sync` rewrites the lockfile with the " +
            "new resolved values. Pass --force to confirm:\n" +
            "  forge sync … --var-file FILE --force",
        )
    }
    ```
  - This makes the lockfile rewrite explicit and prevents
    accidental drift between the lockfile and project state.

- [x] **C.4 Load vars-file and merge with lockfile.**
  - File: `cmd/sync.go` (modify) and `internal/sync/` as needed.
  - When `--var-file --force` is set, call
    `varsfile.Load(varFiles, bp.Variables)`, then overlay the
    resolved values on top of the lockfile-loaded variables before
    rendering.
  - **Persistence:** rewrite the lockfile with the new resolved
    values after a successful sync. Same shape as create-time
    persistence (no vars-file path recorded — see DESIGN-0005 OQ-3).
  - Surface unknown keys via `ui.Warning`.
  - Type-coercion errors abort the sync before any files are
    touched.

- [x] **C.5 Integration tests for sync.**
  - File: `cmd/sync_test.go` (modify).
  - Tests:
    - Sync with no `--var-file` → unchanged behaviour (regression
      coverage).
    - Sync with `--var-file FILE` (no `--force`) → error matches
      the documented message; exit code non-zero; no files
      written; lockfile unchanged.
    - Sync with `--var-file FILE --force` overriding one key →
      resulting lockfile reflects the new value; downstream
      rendered files pick it up; managed-file three-way merge
      still works.
    - Sync with `--var-file FILE --force` introducing an unknown
      key → warning, no error; unknown key not persisted.
    - Sync with `--var-file FILE --force` and a type-mismatch in
      the vars file → coercion error; lockfile unchanged.

- [x] **C.6 Run `make ci`.**

#### Success Criteria

- `forge sync … --var-file FILE` without `--force` errors with the
  documented message; no side effects.
- `forge sync … --var-file FILE --force` rewrites the lockfile and
  re-renders managed files with the overridden values.
- `make ci` green.
- Lockfile is updated consistently after sync (no orphan keys).

---

### Phase D: `--var-file` on `forge check` (rejection)

Per OQ-5, `forge check --var-file FILE` has no meaningful
drift-detection semantic — `check` is lockfile-driven by design.
This phase registers the flag solely so the rejection error is
actionable (DESIGN-0005-aligned), rather than Cobra's generic
"unknown flag" diagnostic.

#### Tasks

- [ ] **D.1 Register the `--var-file` flag on `forge check`.**
  - File: `cmd/check.go` (modify).
  - StringArrayVar registration so the flag is *recognised*; help
    text explicitly notes the flag is rejected on check.

- [ ] **D.2 Reject the flag with a clear error.**
  - File: `cmd/check.go` (modify).
  - Inside `RunE`, after flag parsing:

    ```go
    if len(varFiles) > 0 {
        return errors.New(
            "--var-file is not supported on `forge check`. " +
            "`forge check` is a drift-detection command and " +
            "reads variables from the lockfile only. " +
            "To preview drift against alternative values, run " +
            "`forge sync --var-file FILE --force --dry-run` instead " +
            "(if --dry-run is supported on sync; otherwise use a " +
            "throwaway worktree).",
        )
    }
    ```
  - (Note: the `--dry-run` hint depends on whether `forge sync`
    supports `--dry-run` today — verify before finalising the
    message. If not, drop that sub-clause.)

- [ ] **D.3 Integration tests for check.**
  - File: `cmd/check_test.go` (modify).
  - Tests:
    - `forge check` (no `--var-file`) → unchanged behaviour
      (regression).
    - `forge check --var-file FILE` → error matches the documented
      rejection message; exit code non-zero; no drift report
      printed.

- [ ] **D.4 Run `make ci`.**

#### Success Criteria

- `forge check --var-file FILE` errors with the actionable
  rejection message; no `forge check` drift output produced.
- `forge check --help` shows `--var-file` with the "not supported
  on check" annotation.
- `make ci` green.

---

### Phase E: Documentation and release prep

#### Tasks

- [ ] **E.1 Update `docs/MIGRATION.md`.**
  - New "Preferred input pattern" note (not deprecating `--set`,
    recommending `.forge-vars.hcl` for non-trivial cases).
  - **Do NOT document the process-substitution pattern** that
    DESIGN-0005 OQ-4 originally suggested — it's now dropped
    per IMPL-0008 OQ-8 (strict `.hcl` extension). Instead document
    the "write a tempfile" pattern for inline overrides:

    ```sh
    cat > /tmp/override.forge-vars.hcl <<EOF
    project_name = "mockta-staging"
    EOF
    forge create go/ext \
      --var-file ./base.forge-vars.hcl \
      --var-file /tmp/override.forge-vars.hcl
    ```

- [ ] **E.1b Update `docs/design/0005-variable-input-via-vars-file.md`.**
  - Replace the process-substitution example with the tempfile
    pattern (parity with E.1).
  - Add a note at the bottom of the Mutual Exclusion section
    cross-referencing IMPL-0008 OQ-8 for the rationale.

- [ ] **E.2 Update `README.md`.**
  - Quick Start: add a `.forge-vars.hcl` example alongside the
    existing `--set` example.
  - Commands table: note `--var-file` on create/sync/check.

- [ ] **E.3 Update `CLAUDE.md`.**
  - Architecture entries: add `internal/varsfile/` package.
  - CLI Design Decisions: add the `--var-file` / `--set` mutual
    exclusion convention.

- [ ] **E.4 Release notes.**
  - File: `docs/release-notes/v0.X.0-vars-file.md` (new — version
    set when work lands).
  - Highlight the additive feature; document the mutual-exclusion
    rule and the process-substitution escape hatch.

- [ ] **E.5 Update forge-registry blueprint READMEs (downstream).**
  - Not in this repo — note as a follow-up issue against
    `github.com/donaldgifford/forge-registry`.

- [ ] **E.6 docz-reviewer pass on the doc changes.**

#### Success Criteria

- README has a worked `.forge-vars.hcl` example.
- MIGRATION.md surfaces the recommended pattern.
- Release notes draft is review-ready.
- `mkdocs build` succeeds.

---

## File Changes

### New files

| File | Phase | Purpose |
|------|-------|---------|
| `internal/varsfile/varsfile.go` | A | Public `Load` entry point + parser + composer + coercion |
| `internal/varsfile/varsfile_test.go` | A | Unit tests |
| `internal/varsfile/testdata/basic.forge-vars.hcl` | A | Happy-path fixture |
| `internal/varsfile/testdata/override.forge-vars.hcl` | A | Composition fixture |
| `internal/varsfile/testdata/wrong-type.forge-vars.hcl` | A | Type-coercion error fixture |
| `internal/varsfile/testdata/malformed.forge-vars.hcl` | A | Parse-error fixture |
| `internal/varsfile/testdata/with-blocks.forge-vars.hcl` | A | "Attributes-only" rejection fixture |
| `internal/varsfile/testdata/unknown-keys.forge-vars.hcl` | A | Unknown-keys warning fixture |
| `cmd/testdata/varsfile/...` | B | Integration test fixtures (path TBD per `cmd/` convention) |
| `docs/release-notes/v0.X.0-vars-file.md` | E | Release notes |

### Modified files

| File | Phase | Change |
|------|-------|--------|
| `cmd/create.go` | B | Add `--var-file` flag, mutual-exclusion check, vars-file load + integration |
| `cmd/create_test.go` | B | Integration tests for `--var-file` on create |
| `cmd/sync.go` | C | Add `--var-file` flag and sync-time semantics (per OQ-4) |
| `cmd/sync_test.go` | C | Integration tests for sync |
| `cmd/check.go` | D | Add `--var-file` flag (or omit per OQ-5) |
| `cmd/check_test.go` | D | Integration tests for check |
| `internal/create/create.go` | B | Possibly extend `Opts` to carry the vars-file map (per OQ-3) |
| `docs/MIGRATION.md` | E | Preferred-input-pattern note |
| `README.md` | E | Vars-file example in Quick Start |
| `CLAUDE.md` | E | Architecture entry + CLI design decision |
| `mkdocs.yml` | E | Release-notes nav entry |

## Testing Plan

- **Unit tests** in `internal/varsfile/` (Phase A): happy path,
  composition, coercion errors with source location, unknown keys,
  malformed HCL, block-rejection, missing file.
- **Integration tests** in `cmd/` (Phases B/C/D): end-to-end
  `forge create` / `forge sync` / `forge check` invocations against
  the hermetic HCL fixture in `testdata/hcl-registry/`. Assert
  scaffold output, lockfile contents, exit codes, stderr content
  for warnings.
- **CLI tests** (Phase B): flag wiring (help text, mutual
  exclusion), error messages.
- **No new mocks needed.** The vars-file parser is a pure function
  over file paths + variable declarations; tests use real
  `t.TempDir()` files following the existing project convention
  (CLAUDE.md).
- **Coverage target:** >=80% per CLAUDE.md; vars-file package
  targets >=90% given its small surface.

## Quality Gates

- **After Phase A:** `make test-pkg PKG=./internal/varsfile` green;
  go-review subagent pass on the new package; coverage >=90%.
- **After Phase B:** `make ci` green; manual smoke test against a
  forge-registry blueprint scaffolded with `--var-file` matches the
  equivalent `--set` scaffold byte-for-byte (modulo timestamps);
  go-review subagent pass on `cmd/create.go` changes.
- **After Phase C:** `make ci` green; sync semantics behave per
  OQ-4 decision.
- **After Phase D:** `make ci` green; check semantics behave per
  OQ-5 decision.
- **After Phase E:** docz-reviewer pass on MIGRATION.md and README
  changes; release notes match the established shape.

## Dependencies

- **DESIGN-0005** is the design source.
- **No blocking IMPLs.** This work is additive and can ship at any
  release boundary.
- **External libraries** (`hashicorp/hcl/v2`, `zclconf/go-cty`) are
  already in `go.mod` — no new dependencies.
- **Composes with future work:**
  - **RFC-0002** (object/list/map types) — when those types land,
    the vars-file parser's existing `cty.Value` plumbing extends to
    nested HCL natively. No change to the package public API.
  - **IMPL-0006** (HCL lockfile) — both files become HCL2; the
    parser/emitter machinery is shared.
  - **RFC-0003** (locals + namespacing) — vars-file keys remain
    bare attribute names (declarations, not references), so
    RFC-0003 does not affect this IMPL.

## Open Questions

- **OQ-1: HCL function support in vars files.** A vars file is a
  set of attribute assignments. Should the parser allow expressions
  that call into the cty stdlib (`upper("foo")`, `env("HOME")`,
  etc.), or strictly accept literal values? Pros of allowing
  functions: more expressive for environment-driven inputs. Cons:
  vars files become harder to reason about; a vars file is supposed
  to be a transparent "here are the values" artifact. **Decision:
  strict literals for v1.** No cty stdlib, no traversals, no
  references — just literal values. Re-evaluate if a real use case
  surfaces. Reflected in Phase A.5 (eval with empty
  `hcl.EvalContext`) and A.7 (function-call rejection test).

- **OQ-2: Mutual exclusion — Cobra `MarkFlagsMutuallyExclusive` or
  manual check in `RunE`?** Cobra's built-in helper emits a
  generic "flags in the group … cannot all be set" message;
  DESIGN-0005's spelled-out error is more actionable.
  **Decision: manual check in `RunE`** with the verbatim
  DESIGN-0005 error string. Reflected in Phase B.2 and (the
  no-op equivalent of) Phase C.2.

- **OQ-3: Where does the vars-file map flow into `create.Run`?**
  Today `create.Opts.Overrides` is `map[string]string` (the
  `--set` parsed form). **Decision: extend `create.Opts` with a
  new `VarsFileValues map[string]cty.Value` field.** Preserves
  type fidelity end-to-end and matches the DESIGN-0005 statement
  that values flow as `cty.Value` everywhere. Reflected in
  Phase B.3 and B.4.

- **OQ-4: Sync semantics for `--var-file`.** Today `forge sync`
  reads variables exclusively from the lockfile. **Decision:
  override-with-force (option 2).** `--var-file` overrides
  lockfile values for the listed keys *only* when `--force` is
  also passed; without `--force`, the sync errors out before
  doing any work. Safer than the unconditional override (no
  accidental lockfile rewrites), and matches the existing
  `--force` semantics elsewhere in forge. Reflected in Phase C.3.

- **OQ-5: Check semantics for `--var-file`.** **Decision: refuse
  the flag (option 3).** `forge check` is lockfile-driven drift
  detection; vars-file input has no clean semantic here. The flag
  is *registered* on `check` solely so the rejection error is
  actionable rather than Cobra's generic "unknown flag"
  diagnostic. Reflected in Phase D.

- **OQ-6: Phase split across releases.** **Decision: keep all
  five phases in this IMPL and ship in one PR / one release.**
  The work is logically one unit; splitting just adds doc
  overhead and risks the sync/check phases drifting out of date
  before they ship.

- **OQ-7: Unknown-key warning vs `--strict-vars`.** The design
  says unknown keys are a warning, not an error. **Decision: no
  `--strict-vars` flag in v1.** Keep the surface minimal; CI
  users who want strict behaviour can grep stderr for the
  warning. Revisit if real demand surfaces.

- **OQ-8: Cross-platform `.forge-vars.hcl` extension handling.**
  Should the parser accept any extension, or restrict to `.hcl`?
  **Decision: require `.hcl` extension** for every
  `--var-file` path. Cleaner consistency with the "HCL
  everywhere" story (per DESIGN-0004 / IMPL-0006) and simpler
  parsing — no special-casing for `/dev/fd/*` or other oddities.
  Side effect: drops the process-substitution pattern
  (`--var-file <(echo 'k = v')`) that DESIGN-0005 OQ-4 originally
  recommended; users who want inline overrides write to a real
  `.hcl` tempfile instead. DESIGN-0005 is updated in Phase E.1b
  to match. Reflected in Phase A.3 (extension check at
  `parseFile` entry).

## References

- [DESIGN-0005 — Variable input via vars file](../design/0005-variable-input-via-vars-file.md) — the design this IMPL realises.
- [RFC-0002 — Object and collection variable types](../rfc/0002-object-and-collection-variable-types.md) — composing feature (object/list/map values in vars files).
- [RFC-0003 — Locals for derived values](../rfc/0003-locals-for-derived-values.md) — namespacing convention (vars files keep bare keys, RFC-0003 affects templates/conditions only).
- [IMPL-0006 — Migrate lockfile from YAML to HCL](0006-migrate-lockfile-from-yaml-to-hcl.md) — sibling HCL2 work; shares parsing patterns.
- `internal/config/loader_hcl.go` — HCL2 parsing reference pattern (lines 69–74).
- `internal/lockfile/cty.go` — `ToCtyValues` coercion machinery (lines 20–49) the new package mirrors.
- `internal/config/blueprint.go` — `Variable` struct (lines 32–41) that drives type coercion.
- `internal/prompt/prompt.go` — `CollectVariables` (lines 30–50); existing overrides-skip-prompt behaviour that vars-file values reuse.
- `cmd/create.go` — current `--set` flag wiring (lines 20–50); parallel structure for `--var-file`.
- `cmd/sync.go`, `cmd/check.go` — extension targets in Phases C and D.
- [Terraform `-var-file` documentation](https://developer.hashicorp.com/terraform/language/values/variables#variable-definitions-tfvars-files) — direct prior art.
