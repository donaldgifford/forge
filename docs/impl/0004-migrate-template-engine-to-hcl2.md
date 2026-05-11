---
id: IMPL-0004
title: "Migrate Template Engine to HCL2"
status: Draft
author: Donald Gifford
created: 2026-05-07
---
<!-- markdownlint-disable-file MD025 MD041 -->

# IMPL 0004: Migrate Template Engine to HCL2

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-05-07

<!--toc:start-->
- [Objective](#objective)
- [Scope](#scope)
  - [In Scope](#in-scope)
  - [Out of Scope](#out-of-scope)
- [Implementation Phases](#implementation-phases)
  - [Phase A: HCL2 renderer alongside text/template](#phase-a-hcl2-renderer-alongside-texttemplate)
    - [Tasks](#tasks)
    - [Success Criteria](#success-criteria)
  - [Phase B: Migration tool](#phase-b-migration-tool)
    - [Tasks](#tasks-1)
    - [Success Criteria](#success-criteria-1)
  - [Phase C: Loader cutover](#phase-c-loader-cutover)
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

Execute the cutover from Go `text/template` to HCL2
(`hashicorp/hcl/v2`) specified in DESIGN-0003. The migration is a
breaking change with a one-shot rewrite tool — no parallel-format
support window. v1 blueprints stop loading; `forge migrate
templates` rewrites them to v2; v2 is the only format forge accepts
after this lands.

**Implements:** [DESIGN-0003 — Migrate Template Engine to HCL2](../design/0003-migrate-template-engine-to-hcl2.md)

**Decision record:** [ADR-0001 — Use HCL2 as the Template Engine](../adr/0001-use-hcl2-as-the-template-engine.md)

## Scope

### In Scope

- Replace `internal/template/` renderer with an HCL2-backed
  implementation while keeping the orchestrator-facing `Renderer`
  API source-compatible (`RenderFile`, `RenderString`,
  `RenderPath`, plus a new `EvaluateBool`).
- Re-implement the custom function map (`snakeCase`, `camelCase`,
  `pascalCase`, `kebabCase`, `now`, `env`, `title`, `trimPrefix`,
  `trimSuffix`) as `function.Function` definitions. Drop v1
  `default` in favour of `cty/stdlib.CoalesceFunc`.
- Switch the variable-map type from `map[string]any` to
  `map[string]cty.Value` across every call site
  (`internal/prompt/`, `internal/create/`, `internal/sync/`,
  `internal/check/`, `internal/lockfile/`).
- Bump `apiVersion` to `v2` in `blueprint.yaml`. Update validators
  and scaffolding templates (`internal/initcmd/`,
  `internal/registrycmd/blueprint.go`).
- Build `forge migrate templates` (CLI + core) per DESIGN-0003's
  rewrite-rule table. Validate against
  `github.com/donaldgifford/forge-registry` corpus before shipping.
- Migrate `testdata/registry/` fixtures to v2; preserve a v1 copy
  under `testdata/v1-registry/` for migration-tool tests and
  v1-rejection tests.
- Author `docs/MIGRATION.md` (the user-facing v1→v2 migration guide
  referenced from the load-time error message).

### Out of Scope

- Hosted docs site for `MIGRATION.md` (deferred, per DESIGN-0003
  Resolved Questions).
- New template features beyond v1 parity (loops over arbitrary
  collections, expression-level imports). Tracked separately.
- Schema changes to `blueprint.yaml`/`registry.yaml` beyond
  `apiVersion` and the expression-syntax fields.
- Changes to `forge create`/`sync`/`check` orchestration logic
  beyond the renderer swap and the vars-map type change.

## Implementation Phases

Five phases, ordered by dependency. Phases A and B run inside an
`--experimental-hcl2` flag so the v1 path keeps working until Phase
C cuts it over.

---

### Phase A: HCL2 renderer alongside text/template

**Goal:** Land an HCL2-backed `Renderer` that satisfies the same
public API as today's text/template renderer. Gate it behind
`forge create --experimental-hcl2` so existing v1 blueprints keep
working during development.

#### Tasks

- [x] **A.1 Add HCL2 dependencies.**
  - `go get github.com/hashicorp/hcl/v2`
  - `go get github.com/zclconf/go-cty`
  - Run `make license-check` — confirm MPL-2.0 (HCL) and MIT (cty)
    pass the existing allowlist.
  - Update `go.mod` / `go.sum`; commit the lockfile changes.
  - Note: `make license-check` and `make lint` had pre-existing
    Go 1.26.2 toolchain incompatibilities (resolved by pinning
    `golangci-lint = "2.11.4"` in `mise.toml`; `make license-check`
    still flags stdlib package detection issues unrelated to the
    new deps). Manual verification of new licences: HCL2 MPL-2.0,
    go-cty MIT, levenshtein Apache-2.0, go-textseg/wordwrap MIT —
    all allowlist-compatible. Five pre-existing gosec warnings in
    `internal/{create,hooks,registrycmd,sync}` are baseline; new
    deps add none.

- [x] **A.2 Define a `Renderer` interface and the v2 implementation.**
  - File: `internal/template/renderer.go` — extract the existing
    concrete `Renderer` struct's methods into a new `Renderer`
    interface in the same file; rename the existing struct to
    `TextRenderer`. Existing call sites bind to the interface.
    ```go
    type Renderer interface {
        RenderFile(path string, vars map[string]cty.Value) ([]byte, error)
        RenderString(tmpl string, vars map[string]cty.Value) (string, error)
        RenderPath(path string, vars map[string]cty.Value) (string, error)
        EvaluateBool(expr string, vars map[string]cty.Value) (bool, error)
    }
    ```
  - File: `internal/template/hcl_renderer.go` (new). Defines
    `HCLRenderer` satisfying the interface.
    ```go
    type HCLRenderer struct { funcs map[string]function.Function }
    func NewHCLRenderer() *HCLRenderer
    ```
  - **The interface stays after the cutover.** Phase C deletes
    `TextRenderer` but keeps `Renderer` as the interface and
    renames `HCLRenderer` to satisfy it directly. This is the
    durable abstraction (per OQ-1).
  - Internals: parse via `hclsyntax.ParseTemplate` (file/string/path)
    or `hclsyntax.ParseExpression` (`when:`). Build
    `hcl.EvalContext` from `vars` + funcs. Surface
    `hcl.Diagnostics` as wrapped errors with file/line/col.
  - Stub these out; tests come in A.4.
  - Note: A.2 also expands the existing `TextRenderer` to satisfy
    the new interface signature (`map[string]any` →
    `map[string]cty.Value`). For Phase A, the
    `TextRenderer.RenderFile` etc. accept `map[string]cty.Value`
    by converting back to `map[string]any` internally so existing
    `text/template` semantics still work. This is a temporary
    shim Phase C deletes.

- [x] **A.3 Re-implement the custom function map.**
  - File: `internal/template/hcl_funcs.go` (new).
  - Provide `function.Function` definitions for: `snakeCase`,
    `camelCase`, `pascalCase`, `kebabCase`, `title`, `trimPrefix`,
    `trimSuffix`, `now(layout)`, `env(key)`.
  - Wire `cty/stdlib` for: `upper`, `lower`, `replace`, `coalesce`
    (replaces v1 `default`).
  - Drop the v1 `default(val, fallback)` custom function — use HCL
    `coalesce(val, fallback)` directly.

- [x] **A.4 Unit tests for the renderer.**
  - File: `internal/template/hcl_renderer_test.go`.
  - Cover `RenderFile`, `RenderString`, `RenderPath`,
    `EvaluateBool` against table-driven fixtures.
  - **Critical fixture**: a `.tmpl` file with verbatim Helm-style
    `{{ .Values.replicas }}` content alongside `${project_name}` —
    asserts the collision is gone.
  - Coverage of every custom func from A.3 (snake/camel/pascal/kebab,
    now/env/title/trimPrefix/trimSuffix).
  - Diagnostic-error path: a syntax-broken template returns an
    error referencing the source range (line/col).

- [x] **A.5 Add `--experimental-hcl2` flag to `forge create`.**
  - File: `cmd/create.go`.
  - When set, `forge create` constructs an `HCLRenderer` and
    passes it via the `Renderer` interface to `create.Run()`.
    When unset, it constructs a `TextRenderer` (also via the
    interface). The orchestrator depends only on the interface;
    no `useHCL bool` switches inside `internal/create/`.
  - Document the flag as experimental in `--help`.

- [x] **A.6 Hermetic v2 fixture for end-to-end test.**
  - Path: `testdata/v2-registry/` (new).
  - Mirror `testdata/registry/` shape but with HCL syntax in
    `blueprint.yaml` (`condition.when`, `variable.default`,
    `rename`) and in `.tmpl` files. Include a Helm-style
    `values.yaml.tmpl` with verbatim `{{ .Values.x }}` content to
    prove the collision is fixed.

- [x] **A.7 Integration test exercising the experimental flag.**
  - File: `internal/create/cli_integration_test_v2.go` (new) or a
    new test func in the existing integration test file.
  - Calls `create.Run()` with the v2 renderer against
    `testdata/v2-registry/`; asserts:
    - All expected files render correctly.
    - Helm-style `{{ }}` content survives byte-for-byte.
    - `condition.when: 'use_grpc == true'` evaluates as bool.
    - Path templating with `${project_name}/cmd/main.go` works.

#### Success Criteria

- `make build` succeeds with the new deps.
- `make lint` passes with no new warnings.
- `make test` — all existing tests still pass (v1 path unchanged).
- New HCL renderer unit tests in `internal/template/` pass.
- `forge create --experimental-hcl2 helm/chart --registry-dir
  testdata/v2-registry …` produces a project where Helm-style
  `{{ }}` content is preserved verbatim and `${ }` substitutions
  resolve.
- `make license-check` passes (no new licence violations).

---

### Phase B: Migration tool

**Goal:** Build `forge migrate templates` per DESIGN-0003. Validate
the rewrite-rule set against the
[forge-registry corpus](https://github.com/donaldgifford/forge-registry)
before shipping. Migration tool runs against v1 blueprints and
emits v2 blueprints in place.

#### Tasks

- [x] **B.1 Survey forge-registry for translatable patterns.**
  - Clone `github.com/donaldgifford/forge-registry` into a scratch
    dir.
  - Walk every `.tmpl` file and every `blueprint.yaml` and catalog
    the v1 expression patterns in use (single-arg pipes, multi-arg
    pipes, `if`/`else`/`end`, `range`, `with`, custom funcs, etc.).
  - Record the catalog as a comment block at the top of
    `internal/migratecmd/rules.go` (added in B.3) so future
    contributors understand the corpus the rule set was derived
    from.
  - Decide whether the rule set in DESIGN-0003 needs additional
    rows. Update DESIGN-0003 if so before continuing.

- [x] **B.2 Define `MigrateOpts` / `MigrateResult` types.**
  - File: `internal/migratecmd/migrate.go` (new package).
  - Types:
    ```go
    type MigrateOpts struct {
        Path    string // blueprint or registry root
        DryRun  bool
        Strict  bool
        Force   bool   // skip dirty-worktree guard
    }

    type MigrateResult struct {
        Blueprints []BlueprintReport
    }

    type BlueprintReport struct {
        Path             string
        Migrated         bool       // false in dry-run or when out-of-scope
        AlreadyV2        bool
        FilesRewritten   []string
        UntranslatedHits []UntranslatedHit
    }

    type UntranslatedHit struct {
        File    string
        Line    int
        Snippet string
        Reason  string  // e.g. "range block", "three-arg pipe"
    }
    ```
  - Stub `RunMigrate(opts *MigrateOpts) (*MigrateResult, error)` that
    returns `nil, nil`.

- [x] **B.3 Implement the rewrite engine via AST walk.**
  - File: `internal/migratecmd/rules.go` (new).
  - **Approach: parse the v1 template to an AST, walk it, emit
    HCL.** Use `text/template/parse` (stdlib) — it returns a
    `*parse.Tree` whose `Root` is a `*parse.ListNode`. Walking
    the AST is more code than regex sed, but it correctly
    handles nested actions, unusual whitespace, multi-line
    expressions, and the rare cases where a regex would
    misfire. (Per OQ-3: "do it right".)
  - Top-level entry point:
    ```go
    func RewriteTemplate(name, src string) (out string, hits []UntranslatedHit, err error)
    ```
    Parses `src` via `parse.New(name).Parse(...)`, walks the
    resulting tree, emits HCL for every node it knows how to
    translate, and surfaces an `UntranslatedHit` for nodes it
    refuses to translate.
  - One walker method per `parse.Node` kind:
    - `*parse.TextNode` → emit verbatim (preserves `{{ }}`-using
      content for downstream tools).
    - `*parse.ActionNode` (which wraps a `*parse.PipeNode`) →
      translate to `${ … }` interpolation. The `PipeNode` may be
      a single command (e.g. `{{ .x }}`) or a chain (e.g.
      `{{ .a | f | g }}`); walk each `*parse.CommandNode` and
      compose function calls inside the HCL output.
    - `*parse.IfNode` → `%{ if EXPR ~} … %{ else ~} … %{ endif ~}`.
    - `*parse.RangeNode`, `*parse.WithNode`, `*parse.TemplateNode`,
      `*parse.BreakNode`, `*parse.ContinueNode` → emit
      `UntranslatedHit` and continue walking siblings (these
      are out-of-scope per DESIGN-0003).
  - Sub-translators (used by the walker):
    - `translateIdentifier(*parse.FieldNode|*parse.IdentifierNode|*parse.VariableNode) string`
      — produce the bare HCL identifier (drop leading `.`).
    - `translateCommand(*parse.CommandNode) string` — produce
      `funcname(arg1, arg2, …)`. Single-command pipes become a
      single function call; chained pipes become nested calls
      (e.g. `{{ .a | f | g "x" }}` → `${g(f(a), "x")}`).
    - `translateComparator(*parse.PipeNode) string` — for
      `condition.when` expressions, translate `eq`/`ne`/`not`
      to `==`/`!=`/`!`.
    - `translateDefault(*parse.PipeNode) string` —
      `{{ default .x "fb" }}` / `{{ .x | default "fb" }}` →
      `${coalesce(x, "fb")}`.
  - Special cases the walker handles inline:
    - String-literal escape `{{ "{{" }}` / `{{ "}}" }}` →
      emit `{{` / `}}` as plain text.
    - Path templating shorthand `{{name}}` (no leading dot,
      one identifier, no whitespace) → `${name}`.
  - The walker is unit-tested per node kind in B.7. Each
    untranslated kind has a negative test.

- [x] **B.4 Implement the file walker.**
  - File: `internal/migratecmd/walk.go` (new).
  - `walkBlueprints(rootPath) ([]string, error)` — find every
    directory containing a `blueprint.yaml` under root.
  - For each blueprint root:
    - If `apiVersion` is already `v2`: skip with `AlreadyV2: true`.
    - **Rewrite `blueprint.yaml` expression fields** (per OQ-7,
      roll forward — no v1 carve-outs anywhere). Pass each
      expression string through the same `RewriteTemplate` AST
      walker from B.3:
      - `variable.default` (every variable).
      - `condition.when` (every condition).
      - `rename` keys and values (every entry).
      Re-marshal `blueprint.yaml` with the rewritten strings.
    - Apply rewrites to every `.tmpl` file under the blueprint
      directory via the same `RewriteTemplate` walker.
    - Bump `apiVersion: v1` → `v2` last (so a mid-run failure
      doesn't leave a partially-converted blueprint claiming v2).
  - **Rewrite `registry.yaml` separately** (per OQ-5): if a
    `registry.yaml` exists at `rootPath`, bump its `apiVersion`
    from `v1` to `v2` in the same run. `registry.yaml` has no
    expression fields, so only the version literal changes.
  - Honour `--dry-run`: collect would-be changes but do not write.

- [x] **B.5 Implement the dirty-worktree guard.**
  - File: `internal/migratecmd/git.go` (new) — small wrapper over
    `git status --porcelain`.
  - When `MigrateOpts.Force` is false and the path lives inside a
    git worktree with uncommitted changes, refuse with:
    `"refusing to migrate inside a dirty git worktree (use --force
    to override)"`.

- [x] **B.6 Wire the Cobra command.**
  - File: `cmd/migrate.go` (new).
  - Define `migrateCmd` (parent) with one subcommand
    `migrateTemplatesCmd`.
  - Flags on `migrate templates`: `--path` (default `.`),
    `--dry-run`, `--strict`, `--force`.
  - `RunE`: build `MigrateOpts`, call
    `migratecmd.RunMigrate(opts)`, print summary table:
    ```
    BLUEPRINT          STATUS         FILES   UNTRANSLATED
    helm/chart         migrated       4       0
    legacy/proto       skipped (v2)   —       —
    weird/range        partial        2       1
    ```
  - In `--strict`, exit non-zero if any blueprint has
    `len(UntranslatedHits) > 0`.

- [x] **B.7 Unit tests for every rewrite rule.**
  - File: `internal/migratecmd/rules_test.go`.
  - Table-driven test per rewriter from B.3, covering:
    - Happy-path translation.
    - Idempotence (running the rewriter against already-v2 input
      yields no change).
    - Negative cases (out-of-scope inputs surface
      `UntranslatedHit`).

- [x] **B.8 Integration test against `testdata/v1-registry/`.**
  - File: `internal/migratecmd/integration_test.go` (new).
  - Snapshot `testdata/registry/` into `testdata/v1-registry/`
    (read-only fixture). The integration test copies it to
    `t.TempDir()`, runs `RunMigrate()`, then loads the result via
    `config.LoadBlueprint()` and confirms it parses as v2.
  - Also runs the v2 renderer against the migrated output to
    confirm the converted templates render correctly.

- [x] **B.9 Manual verification against forge-registry.**

  Verified: `forge migrate templates --path <forge-registry> --dry-run
  --strict` produces 5 "would migrate" rows with **zero**
  `UntranslatedHits`. Strict mode passes. The branch-level migration
  PR against forge-registry is deferred to D.5 (per IMPL-0004
  Phase D plan), to be opened once Phase C cuts over.
  - Run `forge migrate templates --dry-run --path
    /path/to/forge-registry` and review the summary table.
  - Iterate on any out-of-scope hits; if a pattern is broadly
    used, decide whether to extend rules.go (loop back to B.3) or
    leave it as a documented manual-fix.
  - Commit a real migration of forge-registry on a branch (gated
    behind whatever review process forge-registry uses) once
    Phase C lands.

#### Success Criteria

- `make check` passes.
- `forge migrate templates --dry-run --path testdata/v1-registry`
  prints the expected rewrite plan.
- `forge migrate templates --path <copy-of-v1-registry>` produces
  a v2 blueprint that loads cleanly under the v2 renderer (Phase A).
- The forge-registry corpus migrates with zero `UntranslatedHits`
  in `--strict` mode (or each hit is documented and accepted as a
  manual-fix in the corpus survey from B.1).
- Dirty-worktree guard fires correctly without `--force` and
  passes with it.

---

### Phase C: Loader cutover

**Goal:** Make HCL2 the only path. Reject v1 blueprints with a
clear error, remove the `--experimental-hcl2` flag, delete the
text/template renderer.

#### Tasks

- [x] **C.1 Bump validators to require v2 (blueprint and registry).**
  - File: `internal/config/validate.go`.
  - In `ValidateBlueprint`: replace
    `if bp.APIVersion != "v1"` with `if bp.APIVersion != "v2"`.
  - In `ValidateRegistry`: replace
    `if reg.APIVersion != "v1"` with `if reg.APIVersion != "v2"`.
    (Per OQ-5: bump in lock-step. `registry.yaml` has no
    expression fields itself, but a matched pair avoids author
    confusion.)
  - Replace the blueprint error message with:
    ```
    blueprint.yaml at <path>: apiVersion v1 is no longer supported.
    Run `forge migrate templates --path <registry-or-blueprint>` to
    convert this blueprint to v2 (HCL2 templates).
    See docs/MIGRATION.md in the forge repository for the v1→v2
    migration guide.
    ```
  - Replace the registry error message with the same shape,
    pointing at `registry.yaml`.
  - Add regression tests in `internal/config/validate_test.go` for
    both v1 rejection paths (blueprint and registry).

- [x] **C.2 Update scaffolding templates.**
  - Files:
    - `internal/initcmd/initcmd.go:23` (`blueprintTemplate` const)
    - `internal/initcmd/initcmd.go:135` (`APIVersion: "v1"` literal)
    - `internal/registrycmd/blueprint.go:38`
      (`blueprintScaffoldTemplate` const)
    - `internal/registrycmd/registrycmd.go:38` (`registryTemplate`
      const) — bump to `apiVersion: v2` in lock-step with the
      blueprint scaffold.
  - Replace `apiVersion: v1` with `apiVersion: v2` in both
    blueprint and registry scaffolds. Replace template syntax in
    starter files (`{{project_name}}/README.md.tmpl` etc.) with
    HCL syntax (`${project_name}` etc.).
  - Update the migration tool (`internal/migratecmd/walk.go`) to
    bump `registry.yaml` to `v2` alongside `blueprint.yaml` so a
    `forge migrate templates --path <registry-root>` run
    produces a fully v2-compliant tree.

- [x] **C.3 Migrate `testdata/registry/` to v2.**
  - Run `forge migrate templates --path testdata/registry` (using
    the binary built from Phase B).
  - Manually verify the diff is sane.
  - Keep the original at `testdata/v1-registry/` as the migration
    test corpus.

- [ ] **C.4 Switch the variable-map type.**
  - Files (every `map[string]any` for variables):
    - `internal/lockfile/lock.go:28` — `Variables` stays as
      `map[string]any` **on disk** (per OQ-6: config files stay
      YAML; only templates and in-config expression fields move
      to HCL). At load time, convert each lockfile entry to a
      `cty.Value` using the variable's declared type from the
      already-fetched `blueprint.yaml`. New helper:
      ```go
      // internal/lockfile/cty.go
      func ToCtyValues(raw map[string]any, vars []config.Variable) (map[string]cty.Value, error)
      func FromCtyValues(vals map[string]cty.Value) map[string]any
      ```
      `ToCtyValues` is called by `sync.Run`/`check.Run`/
      `create.Run` after `LoadBlueprint` returns; `FromCtyValues`
      is called in `create.buildLockfile` before write. The
      lockfile YAML stays human-readable (scalars remain
      scalars), and round-trip is deterministic because the
      variable type is the source of truth.
    - `internal/prompt/prompt.go:15,29-30,50,109,139` — `PromptFn`,
      `CollectVariables`, `resolveVariable`, `renderDefault`.
      `CollectVariables` returns `map[string]cty.Value`.
      `coerceValue`/`zeroValue` produce `cty.Value`.
    - `internal/sync/engine.go:64,100,140,213,265` — `Sync`,
      `syncDefault`, `readSourceContent`, etc.
    - `internal/check/check.go:79,90,144,167,218` — `Check`,
      `resolveRegistryHash`, `readSourceContent`.
    - `internal/create/create.go:215,228,247,291,356` — `Run`,
      `renderFiles`, `writeFile`, `applyRename`,
      `resolveOutputDir`.
    - `internal/create/conditions.go:16,35` — `EvaluateConditions`,
      condition expression evaluation.

- [ ] **C.5 Remove the `--experimental-hcl2` flag.**
  - File: `cmd/create.go`.
  - Delete the flag and its plumbing. The HCL2 renderer is now
    unconditional.
  - Replace any `if useHCL { … } else { … }` switches in the
    orchestrator with the HCL path only.

- [ ] **C.6 Delete the v1 renderer.**
  - Files to delete:
    - `internal/template/renderer.go`
    - `internal/template/renderer_test.go`
    - `internal/template/funcs.go`
    - `internal/template/funcs_test.go`
  - Rename:
    - `internal/template/hcl_renderer.go` →
      `internal/template/renderer.go`
    - `internal/template/hcl_funcs.go` →
      `internal/template/funcs.go`
    - Test files renamed accordingly.
  - Public type alias: `Renderer = HCLRenderer` (or rename the
    struct to `Renderer`).

- [ ] **C.7 Update `internal/prompt/prompt.go renderDefault`.**
  - The current `renderDefault` uses `text/template` directly
    (line 144). Replace with `template.NewRenderer().RenderString`
    so the v2 engine renders default expressions consistently with
    everything else.
  - Drop the `text/template` import.

- [ ] **C.8 Drop `text/template` imports across the tree.**
  - Run `grep -rn "text/template" .` and remove any remaining
    direct imports outside `internal/template/`. The `migratecmd`
    package may still need them transiently for the rewrite rules
    (it parses v1 templates) — confirm and keep there if so.

- [ ] **C.9 Update integration tests for v2 fixtures.**
  - Files:
    - `internal/create/cli_integration_test.go` — point at the
      migrated `testdata/registry/` (now v2).
    - `internal/create/create_test.go`
    - `internal/sync/*_test.go`
    - `internal/check/*_test.go`
    - `internal/list/list_test.go`
    - `internal/search/search_test.go`
    - `internal/registry/index_test.go`
    - `internal/defaults/resolver_test.go`
  - Adjust assertions for the v2 renderer's output (whitespace
    handling around `%{ if … ~}` directives may differ slightly
    from `text/template`; assertions on rendered file content may
    need small updates).

- [ ] **C.10 Rejection-path integration test.**
  - File: a new test under `internal/config/` or
    `internal/create/` that runs `LoadBlueprint()` against
    `testdata/v1-registry/go/api/blueprint.yaml` and asserts the
    error mentions the migration command and `docs/MIGRATION.md`.

#### Success Criteria

- `make check` passes (lint + tests + build).
- `make ci` passes (lint + test + license + build).
- `grep -rn "text/template" internal/` returns no matches outside
  `internal/migratecmd/` (if still needed there).
- `grep -rn "apiVersion.*v1" internal/ cmd/` returns no matches in
  scaffolding or validators.
- `forge create` (no flag) against the migrated `testdata/registry/`
  works; against `testdata/v1-registry/` produces the v1-rejection
  error.
- `forge migrate templates --path testdata/v1-registry --dry-run`
  still produces a clean migration plan (the migration tool keeps
  working post-cutover).
- All historical features (`forge sync`, `forge check`,
  `forge list`, `forge info`, `forge registry blueprint`,
  `forge registry update`) work end-to-end against v2 blueprints.

---

### Phase D: Documentation & release prep

**Goal:** Author user-facing migration docs, update DESIGN-0001 to
reflect HCL2 as the contract, and prepare release notes.

#### Tasks

- [ ] **D.1 Author `docs/MIGRATION.md`.**
  - User-facing v1→v2 migration guide referenced from the
    load-time error string.
  - Sections:
    - Why the change (1-2 paragraphs; link to ADR-0001).
    - The migration command (`forge migrate templates --path …`)
      with examples for blueprint-level and registry-level usage.
    - The rewrite rules (pull from DESIGN-0003 §"Rewrite rules"
      table).
    - Manual fixes needed when the tool surfaces
      `UntranslatedHits` (range blocks, with blocks, multi-arg
      pipes) — show a v1-vs-v2 example for each common case.
    - Verification steps (`forge create` against the migrated
      blueprint).
    - Rollback note (you can always check out a pre-migration
      commit).

- [ ] **D.2 Rewrite DESIGN-0001 (Blueprint Authoring) for HCL2.**
  - Per OQ-8: rewrite in place; do not supersede with a new doc.
    DESIGN-0001 stays the living authoring contract.
  - Preserve the section structure (Goals, Detailed Design, etc.);
    replace every template-syntax example with HCL.
  - Add a "Superseded by" note at the top of the relevant sections
    referencing DESIGN-0003 (the engine swap) and ADR-0001 (the
    decision record), so readers can trace the history.
  - Update References at the bottom to link DESIGN-0003,
    ADR-0001, and `docs/MIGRATION.md`.

- [ ] **D.3 Update CLAUDE.md and README.md.**
  - `CLAUDE.md`: replace any references to Go `text/template` in
    the architecture notes; add `forge migrate` to the cmd list.
  - `README.md`: update the Quick Start example if it shows
    template syntax; add `forge migrate templates` to the Commands
    table; add a "Migrating from v1" pointer to
    `docs/MIGRATION.md`.

- [ ] **D.4 Update DESIGN-0003 status.**
  - Mark DESIGN-0003 status `Implemented` once Phases A–C land.

- [ ] **D.5 Migrate forge-registry on a branch.**
  - Outside this repo: open a PR against
    `github.com/donaldgifford/forge-registry` running the
    migration tool against the registry. Land it once forge cuts
    a release including Phases A–C.

- [ ] **D.6 Release notes for the cutover minor.**
  - Per OQ-9: ship as a `0.x` minor release (pre-1.0 minors are
    the right channel for a documented breaking change). Use the
    existing PR-label-driven semver workflow with the `minor`
    label.
  - Call out in the release notes:
    - The breaking apiVersion bump (v1 → v2).
    - The migration command and `docs/MIGRATION.md` link.
    - A bold-warning block: "Existing v1 blueprints will not
      load. Run `forge migrate templates` against your registry
      before upgrading."
    - The single-line rationale ("YAML blueprints with embedded
      `{{ }}` content for downstream tools
      (Helm/Argo/Kustomize) now ship without escaping").
    - For users who can't migrate: pin to the prior minor (the
      last `text/template` release). Don't promise a
      back-compatible 1.x.

#### Success Criteria

- `docs/MIGRATION.md` exists and walks an external maintainer
  end-to-end through a real migration.
- DESIGN-0001 reflects the HCL2 contract.
- `forge migrate templates` is documented in `forge --help` and in
  the README Commands table.
- `forge-registry` PR is open or merged.
- Release notes draft is ready for the next forge release.

---

## File Changes

### New files

| File | Phase | Purpose |
|------|-------|---------|
| `internal/template/hcl_renderer.go` | A | HCL2-backed `Renderer` |
| `internal/template/hcl_funcs.go` | A | Custom funcs as `function.Function` |
| `internal/template/hcl_renderer_test.go` | A | Renderer unit tests |
| `internal/template/hcl_funcs_test.go` | A | Function-map unit tests |
| `testdata/v2-registry/` | A | v2 hermetic fixture |
| `internal/migratecmd/migrate.go` | B | `RunMigrate` orchestration |
| `internal/migratecmd/rules.go` | B | Rewrite rules + corpus survey notes |
| `internal/migratecmd/walk.go` | B | Blueprint discovery and per-file walk |
| `internal/migratecmd/git.go` | B | Dirty-worktree guard |
| `internal/migratecmd/rules_test.go` | B | Per-rule unit tests |
| `internal/migratecmd/integration_test.go` | B | Migrate → load → render |
| `cmd/migrate.go` | B | Cobra command |
| `testdata/v1-registry/` | B | Frozen v1 fixture for migration tests |
| `docs/MIGRATION.md` | D | User-facing migration guide |

### Modified files

| File | Phase | Change |
|------|-------|--------|
| `go.mod` / `go.sum` | A | Add `hashicorp/hcl/v2`, `zclconf/go-cty` |
| `cmd/create.go` | A → C | Add `--experimental-hcl2` (A); remove (C) |
| `internal/config/validate.go` | C | Require `apiVersion: v2` |
| `internal/config/validate_test.go` | C | Update for v2 acceptance, v1 rejection |
| `internal/initcmd/initcmd.go` | C | Scaffold v2 blueprints |
| `internal/registrycmd/blueprint.go` | C | Scaffold v2 blueprints |
| `internal/registrycmd/registrycmd.go` | C | (Maybe) bump registry scaffold; see Open Questions |
| `internal/lockfile/lock.go` | C | `Variables map[string]cty.Value` |
| `internal/prompt/prompt.go` | C | `cty.Value` throughout; renderDefault uses v2 renderer |
| `internal/create/create.go` | C | `cty.Value` throughout |
| `internal/create/conditions.go` | C | `EvaluateBool` |
| `internal/sync/engine.go` | C | `cty.Value` throughout |
| `internal/check/check.go` | C | `cty.Value` throughout |
| `testdata/registry/` | C | Run migrate; commit v2 result |
| All existing `*_test.go` referencing `testdata/registry/` | C | Update assertions for v2 output |
| `docs/design/0001-blueprint-authoring.md` | D | Rewrite for HCL2 |
| `docs/design/0003-migrate-template-engine-to-hcl2.md` | D | Status → Implemented |
| `CLAUDE.md` | D | Architecture notes |
| `README.md` | D | Commands table + migration pointer |

### Deleted files

| File | Phase | Reason |
|------|-------|--------|
| `internal/template/renderer.go` | C | Replaced by `hcl_renderer.go` (renamed) |
| `internal/template/renderer_test.go` | C | Replaced by `hcl_renderer_test.go` |
| `internal/template/funcs.go` | C | Replaced by `hcl_funcs.go` |
| `internal/template/funcs_test.go` | C | Replaced by `hcl_funcs_test.go` |

## Testing Plan

- **Phase A**: Unit tests for the HCL renderer and function map;
  one integration test exercising
  `forge create --experimental-hcl2` against
  `testdata/v2-registry/`. v1 path tests stay green (no
  regression).
- **Phase B**: Per-rewrite-rule table-driven tests; an integration
  test that migrates `testdata/v1-registry/` and verifies the
  result loads + renders cleanly under v2.
- **Phase C**: Update every existing integration test to point at
  the migrated `testdata/registry/`. Add a regression test for the
  v1-rejection error path. Run `make ci` to catch licence and
  lint regressions.
- **Phase D**: Documentation review only.

Coverage target unchanged from project default (60%, per
`.codecov.yml`). The HCL renderer + migration tool together should
land at ≥80% line coverage given they are pure-function-heavy.

## Dependencies

- Builds on:
  - DESIGN-0003 (the design being implemented).
  - ADR-0001 (the decision record).
  - INV-0001 (the prior investigation).
- External corpus dependency:
  `github.com/donaldgifford/forge-registry` is the validation
  corpus for the migration rule set (B.1, B.9, D.5).

## Implementation Order

```
A.1 deps → A.2 renderer skeleton → A.3 funcs → A.4 unit tests → A.5 flag → A.6 fixture → A.7 integration test
                                                                                              │
                                                                                              ▼
B.1 corpus survey → B.2 types → B.3 rules → B.4 walker → B.5 git guard → B.6 cmd wiring → B.7 rule tests → B.8 integration test → B.9 forge-registry verify
                                                                                                                                              │
                                                                                                                                              ▼
C.1 validator → C.2 scaffolds → C.3 migrate testdata → C.4 vars-map type swap → C.5 remove flag → C.6 delete v1 renderer
       → C.7 prompt renderDefault → C.8 drop text/template imports → C.9 update integration tests → C.10 v1-rejection test
                                                                                              │
                                                                                              ▼
D.1 MIGRATION.md → D.2 DESIGN-0001 rewrite → D.3 CLAUDE.md/README.md → D.4 DESIGN-0003 status → D.5 forge-registry PR → D.6 release notes
```

Phases A and B can be reviewed and merged independently as long as
the v1 path stays green. Phase C is the irreversible cutover and
should land as a single coherent PR (or a small ordered series with
no v1-path regressions in between).

## Resolved Questions

These were open during drafting and resolved during review.
Captured here so the trade-offs stay attached to the plan.

- **OQ-1 (Phase A.5): Renderer interface vs. boolean gate →
  always interface.** A `Renderer` interface in
  `internal/template/renderer.go` is the durable abstraction;
  `TextRenderer` and `HCLRenderer` are concrete implementations
  during the transition. Phase C deletes `TextRenderer` but
  keeps the interface. No `useHCL bool` in `create.Opts`. See
  task A.2.
- **OQ-2 (Phase B.1): Corpus survey timing → (a) survey first.**
  Walk `github.com/donaldgifford/forge-registry` before writing
  `rules.go` so the rule set fits the patterns actually in use.
  Update DESIGN-0003's rewrite-rule table if the survey surfaces
  patterns the table doesn't cover. See task B.1.
- **OQ-3 (Phase B.3): Regex vs. AST walk → AST, do it right.**
  Use `text/template/parse` (stdlib) to produce a real AST; walk
  it node-by-node and emit HCL. Catches nested actions, unusual
  whitespace, and multi-line expressions that a regex sed would
  misfire on. More code than regex, but the migration tool only
  ever runs once per registry — correctness matters more than
  brevity. See task B.3.
- **OQ-4 (Phase B.5): Non-git worktrees → git only, fail closed.**
  If the migration target is not inside a git worktree, refuse
  the run without `--force`. Don't try to detect Mercurial or
  other VCS — git is the supported case; everything else opts in
  explicitly via `--force`. See task B.5.
- **OQ-5 (Phase C.2): registry.yaml version bump → yes, in
  lock-step.** Bump `registry.yaml` to `apiVersion: v2` alongside
  `blueprint.yaml`. One version number across the contract, no
  mismatched pairs. The migration tool flips both during the
  same run. Validators reject `registry.yaml: apiVersion: v1`
  with a parallel error message. See tasks C.1, C.2.
- **OQ-6 (Phase C.4): Lockfile on-disk shape → (c) keep YAML
  with `map[string]any`; convert at load time.** The migration
  is *just the template engine*. Config files (`blueprint.yaml`,
  `registry.yaml`, `.forge-lock.yaml`) stay YAML — only template
  contents and in-config expression fields move to HCL. The
  lockfile keeps its current human-readable shape; a small
  helper (`internal/lockfile/cty.go`) converts to/from
  `cty.Value` at load/save boundaries using the variable type
  declared in the already-fetched `blueprint.yaml`. No on-disk
  schema change. See task C.4.
- **OQ-7 (Phase B/C): Roll forward, no v1 carve-outs.** Migrate
  every v1 expression — including `variable.default` strings in
  `blueprint.yaml` — to v2 in one shot. The same AST walker that
  rewrites `.tmpl` files also rewrites the expression fields in
  `blueprint.yaml`. Users who want v1 semantics pin an older
  forge release; we don't ship parallel paths. See task B.4
  (walker now explicitly covers blueprint.yaml expression
  fields).
- **OQ-8 (Phase D.2): DESIGN-0001 → rewrite in place, reference
  superseding docs.** Rewrite DESIGN-0001 to reflect HCL2 as the
  authoring contract; add a "Superseded by" note pointing at
  DESIGN-0003 (engine swap) and ADR-0001 (decision record).
  DESIGN-0001 stays the living authoring reference; the history
  is still traceable through the cross-references. See task D.2.
- **OQ-9 (Phase D.6): Release version → minor.** Ship as a `0.x`
  minor release using the existing PR-label semver workflow. No
  `1.0` bump. Release notes carry a bold-warning block; users
  who can't migrate pin the prior minor. No back-compatible 1.x
  promised. See task D.6.

## References

- [DESIGN-0003 — Migrate Template Engine to HCL2](../design/0003-migrate-template-engine-to-hcl2.md)
- [ADR-0001 — Use HCL2 as the Template Engine](../adr/0001-use-hcl2-as-the-template-engine.md)
- [INV-0001 — Templating YAML Files and HCL2 Migration](../investigation/0001-templating-yaml-files-and-hcl2-migration.md)
- [DESIGN-0001 — Blueprint Authoring](../design/0001-blueprint-authoring.md) (to be rewritten in D.2)
- [`hashicorp/hcl/v2`](https://github.com/hashicorp/hcl)
- [`zclconf/go-cty`](https://github.com/zclconf/go-cty)
- [`github.com/donaldgifford/forge-registry`](https://github.com/donaldgifford/forge-registry) — migration corpus
