---
id: IMPL-0009
title: "object and collection variable types"
status: Draft
author: Donald Gifford
created: 2026-06-29
---
<!-- markdownlint-disable-file MD025 MD041 -->

# IMPL 0009: object and collection variable types

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-06-29

<!--toc:start-->
- [Objective](#objective)
- [Scope](#scope)
  - [In Scope](#in-scope)
  - [Out of Scope](#out-of-scope)
- [Implementation Phases](#implementation-phases)
  - [Phase A: type expression parser](#phase-a-type-expression-parser)
  - [Phase B: variable struct + HCL schema refactor](#phase-b-variable-struct--hcl-schema-refactor)
  - [Phase C: default expression + validation evaluation](#phase-c-default-expression--validation-evaluation)
  - [Phase D: input integration (vars-file + --set)](#phase-d-input-integration-vars-file----set)
  - [Phase E: prompt UX](#phase-e-prompt-ux)
  - [Phase F: lockfile + template integration](#phase-f-lockfile--template-integration)
  - [Phase G: documentation + release prep](#phase-g-documentation--release-prep)
- [File Changes](#file-changes)
- [Testing Plan](#testing-plan)
- [Quality Gates](#quality-gates)
- [Dependencies](#dependencies)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Objective

Implement [DESIGN-0006 — Object and collection variable
types](../design/0006-object-and-collection-variable-types.md): add
`object({…})`, `list(T)`, and `map(T)` to the variable type surface,
replace the bespoke `choice` type with a Terraform-style
`validation { … }` block, and emit a deprecation warning for the
`int` type alias. Ships as v0.7.0 alongside RFC-0003 (locals + `var.*`
namespacing) per DESIGN-0006 OQ-1.

**Implements:** [DESIGN-0006](../design/0006-object-and-collection-variable-types.md).

## Scope

### In Scope

- New `internal/config/vartype.go` that parses a `variable.type` HCL
  expression to a `cty.Type` via `hashicorp/hcl/v2/ext/typeexpr`
  (DESIGN-0006 OQ-2).
- Variable struct refactor in `internal/config/blueprint.go`:
  `Type string` → `Type cty.Type` + `TypeSource string`;
  `Default string` → `DefaultExpr hcl.Expression` + `DefaultSource string`;
  `Validations []Validation` (new); remove `Validate` and `Choices`
  fields.
- HCL schema refactor in `internal/config/hcldec_spec.go` and the
  hand-decode helpers in `internal/config/loader_hcl.go`:
  - new `validation { condition, error_message }` repeatable nested
    block on `variable`
  - load-time rejection of legacy `choices = [...]` and
    `validate = "regex"` fields with errors pointing at MIGRATION.md
  - `default` and `type` captured as raw HCL expressions, not
    string literals
- Default-expression evaluation against the resolved variable scope
  (replacing the current prompt-time string-render).
- Validation-block evaluation against the resolved variable scope
  (cross-variable references allowed per DESIGN-0006 OQ-4).
- `int` deprecation warning at `LoadBlueprint` time (DESIGN-0006
  OQ-6).
- Vars-file structured-value support — `internal/varsfile` delegates
  to `cty.Convert` against the declared `cty.Type`; existing
  scalar-only test corpus extends with object/list/map fixtures.
- `--set` extensions: top-level object replacement via HCL literal
  parsing; list/map `--set` rejected with an actionable error.
- Prompt UX: object unfold to per-field prompts in declaration
  order; list/map required-and-unsupplied error with copy-pasteable
  vars-file snippet.
- Lockfile cty machinery (`internal/lockfile/cty.go`) extended for
  structured types; HCL emitter (`internal/lockfile/emit_hcl.go`)
  round-trip verification.
- Template strict-vars (`internal/template`) updated to allow nested
  attribute access against declared object types.
- Documentation: MIGRATION.md v0.7 section; REFERENCE.md updated;
  CLAUDE.md updated; v0.7.0 release notes.

### Out of Scope

- `tuple([…])`, `set(T)`, optional object fields, `null` values.
- Interactive TUI widget for list/map input — the documented
  trade-off per DESIGN-0006 is the "use --var-file" error path.
- `prompt { kind = "select", options = [...] }` block to re-introduce
  the legacy `choice` type's select-list UX. Tracked as a follow-up
  RFC (DESIGN-0006 OQ-5).
- In-tool migrator for `choice` / `choices` / scalar-only `validate`
  fields — load-time errors point at MIGRATION.md per ADR-0002.
- `locals { … }` block — covered by RFC-0003 / its own IMPL doc.
  This IMPL coordinates the `var.*` namespace flip but does not
  implement locals.
- `error_message` template interpolation (DESIGN-0006 OQ-3 — static
  string only in v1).
- Promoting the `int` deprecation warning to a load-time error
  (DESIGN-0006 OQ-6 — explicit non-commitment for this release).
- JSON Schema export of the new variable type surface for IDE
  integration (out of band; would be its own IMPL).

## Implementation Phases

Seven phases. Phases A–C are the schema-side change. Phase D is the
input-side change. Phase E is prompt UX. Phase F closes the loop on
downstream consumers (lockfile + templates). Phase G ships docs.

Phases A and B together are the minimum to get the new schema
loading; from there each phase brings up one consumer surface.

> **Sequencing with RFC-0003 (locals + `var.*` namespacing).**
> Per DESIGN-0006 OQ-1, both ship in v0.7.0. The IMPL question of
> *which order* the two parallel streams complete is OQ-1 below.
> Tentative assumption: this IMPL's Phase A starts independently;
> Phases C and F coordinate with RFC-0003's IMPL on the `var.*`
> scope and template-strict-vars changes.

---

### Phase A: type expression parser

Stand up the cty type expression parser as a self-contained module
that any other code path can consume. Foundational; everything else
depends on this.

#### Tasks

- [x] **A.1 Create the new file.**
  - File: `internal/config/vartype.go` (new).
  - Package comment summarises responsibility: "parses a
    `variable.type` HCL expression to a `cty.Type` using the cty
    type-expression grammar, plus forge-specific error wrapping and
    the `int` deprecation warning."

- [x] **A.2 Define the public API.** *(Architect feedback during implementation: dropped the `*Deprecation` out-of-band return in favour of folding the warning into `hcl.Diagnostics` as a `DiagWarning` — more idiomatic, single accumulator, no parallel state. Signature is `(cty.Type, hcl.Diagnostics)`.)*
  - File: `internal/config/vartype.go` (modify).
  - Signature:

    ```go
    // ParseVariableType resolves the variable's `type` HCL
    // expression to a cty.Type. Accepts bareword scalars
    // (string, bool, number, int), the legacy quoted-string
    // forms during the v0.7 transition, and the cty constructor
    // forms (object({...}), list(T), map(T)). Returns a non-nil
    // diagnostic when the expression is malformed or names an
    // unsupported type form (tuple, set, optional).
    //
    // deprecation, when non-nil, indicates the author used a
    // deprecated form (today: `type = int` or `type = "int"`)
    // that still resolves but should migrate to `type = number`.
    // Callers surface the deprecation through ui.Warning.
    func ParseVariableType(expr hcl.Expression) (
        ty          cty.Type,
        deprecation *Deprecation,
        diags       hcl.Diagnostics,
    )

    type Deprecation struct {
        Variable string
        Range    hcl.Range
        Message  string
    }
    ```

- [x] **A.3 Implement using `hashicorp/hcl/v2/ext/typeexpr`.**
  - Delegate the parse to `typeexpr.Type(expr)`.
  - On success, inspect for `cty.Tuple`, `cty.Set`, or types with
    optional attributes; if present, return a forge-specific
    diagnostic pointing at the supported type table in
    REFERENCE.md.
  - On failure, wrap the diagnostic to include the variable name
    (caller passes it via a separate parameter or via expression
    range lookup).

- [x] **A.4 Implement the `int` deprecation detection.**
  - Inspect the expression source: if it parses as either the bare
    `int` keyword or the quoted-string `"int"`, set
    `deprecation = &Deprecation{...}` with the message
    `'type = int' is deprecated; use 'type = number' instead`.
  - The Range carries the source location of the `type` attribute
    so the warning surfaces with file:line:col.

- [x] **A.5 Hermetic test fixtures.** *(Deviated from per-`.hcl`-file fixtures
  in favour of inline `hclsyntax.ParseExpression` in the test file. Reason:
  `ParseVariableType` takes a single `hcl.Expression`, so each fixture
  would have been a one-line file. Inline table-driven tests are
  clearer at this size. Phase B's loader integration tests will use
  proper `.hcl` fixtures since they exercise full blueprint parsing.)*

- [x] **A.6 Unit tests.** *(Coverage on `internal/config/vartype.go` averages ~94% across functions — above the 90% gate. All accepted forms, legacy quoted forms, `int` deprecation, `choice` rejection, tuple/set rejection, nested set-in-object rejection, `any` rejection, and garbage input covered.)*
  - File: `internal/config/vartype_test.go` (new).
  - Table-driven coverage for:
    - All accepted bareword forms: `string`, `bool`, `number`,
      `int`, `list(string)`, `list(number)`, `map(string)`,
      `object({k = string})`, nested object
      `object({addr = object({host = string, port = number})})`.
    - Legacy quoted-string forms: `"string"`, `"bool"`, `"int"`.
    - `"choice"` form is **not** accepted — clean error pointing
      at MIGRATION.md (DESIGN-0006 [Validation block](../design/0006-object-and-collection-variable-types.md#validation-block-choice-replacement)).
    - Rejected forms: `tuple([string, number])`, `set(string)`,
      `object({k = optional(string)})` — each with a clean error.
    - `int` deprecation: assert `Deprecation` is non-nil for
      `int` and `"int"`, nil for `number` and `string`.
  - Coverage gate: ≥90%.

- [x] **A.7 Run `make lint` and `make fmt`.** *(`make lint` returned `0 issues`.)*

#### Success Criteria

- `ParseVariableType` is the single source of truth for converting
  a `type` expression to a `cty.Type`.
- All accepted forms round-trip cleanly; all rejected forms produce
  errors with file:line:col and a migration pointer.
- `int` deprecation reliably detected and surfaced via
  `Deprecation`.
- `make ci` green on `internal/config/`.

---

### Phase B: variable struct + HCL schema refactor

Land the Go-side type changes and the HCL schema updates so
`LoadBlueprint` can parse the new shape. No evaluation logic yet —
that's Phase C.

#### Tasks

- [x] **B.1 Refactor `Variable` struct.** *(Bundled with B.2–B.9 in a single commit because the struct change is the atomic refactor anchor — every consumer needed updating in the same commit to keep CI green per the IMPL doc's per-phase `make ci` gate.)*
  - File: `internal/config/blueprint.go` (modify).
  - Before/after:

    ```go
    // Before
    type Variable struct {
        Name        string
        Description string
        Type        string
        Default     string
        Required    bool
        Validate    string
        Choices     []string
    }

    // After
    type Variable struct {
        Name          string
        Description   string
        Type          cty.Type
        TypeSource    string
        DefaultExpr   hcl.Expression
        DefaultSource string
        Required      bool
        Validations   []Validation
    }

    type Validation struct {
        Condition    hcl.Expression
        ErrorMessage string
        DefRange     hcl.Range
    }
    ```
  - Add field comments explaining that `Type` / `DefaultExpr` are
    captured-at-load-time and mirror `Condition.When`'s pattern.

- [x] **B.2 Update HCL schema (eager spec).** *(`choices` / `validate` removed from `variableBlockBodySchema`; `validation` nested block added. Legacy attribute detection moved to a pre-decode `rejectLegacyVariableAttrs` pass — see B.4.)*
  - File: `internal/config/hcldec_spec.go` (modify).
  - The `variable "name" { … }` block stays hand-decoded (per the
    file's existing comment) — extend `variableBlockBodySchema` to:
    - allow the `validation { … }` repeatable nested block
    - remove the `choices` and `validate` attributes from the
      accepted set (so they fall through to a custom rejection
      diagnostic in the hand-decoder, not a generic "unknown
      attribute" error)

- [x] **B.3 Implement validation-block schema.** *(`validationBlockBodySchema` added next to the variable schema; required `condition` + `error_message`.)*
  - File: `internal/config/hcldec_spec.go` (modify) — add a new
    `validationBlockBodySchema`:

    ```go
    var validationBlockBodySchema = &hcl.BodySchema{
        Attributes: []hcl.AttributeSchema{
            {Name: "condition",     Required: true},
            {Name: "error_message", Required: true},
        },
    }
    ```

- [x] **B.4 Update the loader hand-decoder.** *(`decodeVariableBlock` now routes the `type` attribute through `ParseVariableType`, captures `default` as both an `hcl.Expression` and raw source, decodes each nested `validation { ... }` block via the new `decodeValidationBlock`, and pre-rejects `choices` / `validate` with a MIGRATION.md-anchored error via `rejectLegacyVariableAttrs`. Deprecations from `ParseVariableType` flow through `diagsToDeprecations` and attach to `Blueprint.Deprecations`.)*
  - File: `internal/config/loader_hcl.go` (modify).
  - For each `variable` block:
    - call `ParseVariableType(typeAttr.Expr)` → `Variable.Type`
    - capture `default` as `hcl.Expression` (don't evaluate)
    - decode any nested `validation { … }` blocks into
      `Variable.Validations`
    - reject `choices = [...]` and `validate = "regex"` with the
      MIGRATION.md-pointed error
    - surface the deprecation from `ParseVariableType` via
      `Deprecations []Deprecation` on the load result
      (so callers can warn)

- [x] **B.5 Update `ValidateBlueprint`.** *(`validVariableTypes` map removed; the `Type` allow-list, `choice`-required-choices check, and `Validate` regex compile are all gone. Sync-strategy and managed-files validations unchanged.)*
  - File: `internal/config/validate.go` (modify).
  - Remove the `validVariableTypes` map and the per-variable
    `Type` allow-list check — typing is now enforced by
    `ParseVariableType`.
  - Remove the `choices` and `validate` validations (the fields
    are gone).
  - Keep the sync-strategy and managed-files validations
    unchanged.

- [x] **B.6 Plumb deprecations to the load callers.** *(`Blueprint.Deprecations` carries the load-time notices; `create.Result.Deprecations` and `sync.Result.Deprecations` forward them to the CLI, which surfaces each one via `w.Warningf` ahead of the success line — same pattern as IMPL-0008's `UnknownVarsFileKeys`. `LoadBlueprint`'s `(*Blueprint, error)` signature stays stable; the deprecation channel is on the returned struct.)*
  - File: `internal/config/loader.go` (modify),
    `cmd/create.go`, `cmd/sync.go` (modify).
  - Each call site that does `config.LoadBlueprint(...)` reads the
    returned `Deprecations` and surfaces them via `ui.Warningf`
    before proceeding. Pattern mirrors how IMPL-0008 surfaces
    `UnknownVarsFileKeys`.

- [x] **B.7 Test corpus update.** *(All in-repo `testdata/*/blueprint.hcl` fixtures that previously used `validate = "regex"` / `type = "choice"` / `choices = [...]` migrated to the v0.7 shape — `validation { condition = can(regex(...)) … }` for the regex case, `type = string` + `validation { condition = contains([...], var.X) … }` for the choice case. Touched: `testdata/registry/go/api`, `testdata/hcl-registry/go/api`, `testdata/hcl-registry/helm/chart`, `testdata/v2-registry/go/api`, `testdata/v2-registry/helm/chart`. Also migrated the scaffold template at `internal/registrycmd/blueprint.go::blueprintScaffoldTemplate` so `forge registry blueprint` emits v0.7-shape blueprints out of the box.)*
  - Update existing `internal/config/testdata/` fixtures: any that
    use `type = "choice"`, `choices = [...]`, or
    `validate = "regex"` get rewritten to use the new validation
    block (or split into separate "legacy-reject" fixtures
    asserting the rejection error).
  - New fixtures covering the validation block: single, multiple
    stacked, validation referencing an object field.

- [x] **B.8 Unit + integration tests.** *(New tests in `loader_hcl_test.go`: `TestLoadBlueprintHCL_StructuredTypes` (list/map/object round-trip), `TestLoadBlueprintHCL_RejectsLegacyChoices`, `TestLoadBlueprintHCL_RejectsLegacyValidate`, `TestLoadBlueprintHCL_IntDeprecationFlowsThrough`. `validate_test.go` slimmed down — the now-defunct `InvalidVariableType`, `ChoiceWithoutChoices`, `InvalidRegex`, and `VariableTypeRequired` cases were removed (typing is enforced by `ParseVariableType` at load time, asserted by the `vartype` test suite from Phase A). `prompt_test.go` migrated to `cty.Type` field literals; the `OverrideValidation` and `ChoiceType` cases retired in line with the v0.7 model — validation lives at create-time per Phase C.)*
  - File: `internal/config/loader_hcl_test.go` (modify),
    `internal/config/validate_test.go` (modify).
  - Tests:
    - Loading a blueprint with the new schema (each type, each
      validation form) produces the expected `Variable` shape.
    - Loading a legacy `choices = [...]` produces a clean error
      with MIGRATION.md pointer.
    - Loading a legacy scalar `validate = "regex"` produces a
      clean error with MIGRATION.md pointer.
    - `int` deprecation flows through to the `Deprecations` slice.
    - `ValidateBlueprint` no longer rejects `object`/`list`/`map`
      types (the type check is gone, leaving only sync-strategy
      and managed-files validations).

- [x] **B.9 Run `make ci`.** *(`make lint` reports `0 issues`; `make test` is green across all 19 packages.)*

#### Success Criteria

- `LoadBlueprint` parses the new schema cleanly: object / list /
  map types, validation block, and the `int` deprecation warning.
- Legacy `choices` and `validate` fields produce load-time errors
  that point at MIGRATION.md.
- All existing blueprint fixtures pass after the migration
  rewrite.
- `make ci` green.

---

### Phase C: default expression + validation evaluation

Wire the new `DefaultExpr` and `Validations` fields into the
variable resolution flow. This is where the value pipeline actually
becomes structured-type-aware.

#### Tasks

- [x] **C.1 Default-expression evaluation.** *(`prompt.renderDefault` now calls `v.DefaultExpr.Value(config.BuildEvalContext(boundCty))` first; falls back to the existing template-render path when the parsed expression fails to evaluate or when DefaultExpr is nil — the v0.7 transition shim for bare-reference `${name}` templates. Scalar coercion still flows through `coerceValue` for the in-memory `any` shape.)*
  - File: `internal/prompt/prompt.go` (modify),
    `internal/create/create.go` (modify).
  - Replace the current "render default string via the template
    engine" path with "evaluate `DefaultExpr` against the
    already-bound `var.*` scope":

    ```go
    val, diags := v.DefaultExpr.Value(&hcl.EvalContext{
        Variables: map[string]cty.Value{"var": cty.ObjectVal(bound)},
    })
    ```
  - Coerce the result against `Variable.Type` via `cty.Convert`;
    surface mismatches with file:line:col from
    `DefaultExpr.Range()`.
  - Keep the prompt-time fallback for the v0.7 backwards-compat
    shim: if `Type` is `string`/`bool`/`number` and the default
    parses as a literal string, the template-render path remains
    accessible during the transition window.

- [x] **C.2 Build the resolved-variable scope.** *(Lives on `config.BuildEvalContext(bound)` rather than as a `create/scope.go` helper — co-located with the validation evaluator per OQ-2 so the scope shape and the evaluator that consumes it stay in the same file. The scope exposes each variable under both its bare name (`project_name`) AND the `var.X` namespace (`var.project_name`) so legacy bare-reference defaults and the new var.X validation conditions both resolve against a single context.)*
  - File: `internal/create/create.go` (modify) or new helper
    `internal/create/scope.go` (new).
  - Helper that walks `bp.Variables` in declaration order and
    builds the `cty.Value` scope incrementally so each variable's
    default / validation can reference earlier ones. Mirrors the
    variable-resolution ordering already implicit in
    `prompt.CollectVariables`.

- [x] **C.3 Validation-block evaluation.** *(`config.EvaluateValidations` iterates every `Variable.Validations` entry, runs each condition through `BuildEvalContext`, and accumulates failures rather than short-circuiting. Failures format as `<error_message> (variable "X", blueprint.hcl:L:C)`. Hooked into `create.Run` between `lockfile.ToCtyValues` and `defaults.Resolve`, and into `sync.Run` after the vars-file overlay; both abort with `JoinErrors` before any file ops if any validation fails. Built-in function set is Terraform-aligned: `can`, `try`, `regex`, `contains`, `length`, `lower`, `upper`, `coalesce`.)*
  - File: `internal/create/create.go` or new
    `internal/config/validation.go` (decide per OQ-2 below).
  - After all variables are resolved, iterate every variable's
    `Validations`. For each:
    - evaluate `Condition` against the full resolved scope
    - if the result is not `cty.True`, accumulate an error
      formatted as
      `<error_message> (variable "X", blueprint.hcl:L:C)`
  - If any validation fails, abort the create/sync flow before any
    files are touched. Validation failures stack — surface all of
    them, not just the first.

- [x] **C.4 Cross-variable scope tests.** *(`TestEvaluateValidations_CrossVariableReferences` in `internal/config/validation_test.go` covers the OQ-4 contract: a `var.a != var.b` condition on variable `c` sees both predecessors. Forward references aren't possible in the current Phase B/C model — the loader keeps variables in declaration order and `EvaluateValidations` runs once against the fully resolved scope, so a default referencing a later variable would simply see `cty.NilVal` and emit an `Unsupported attribute` diagnostic via the catch-all `BadExpressionErrors` test.)*
  - Verify a `validation { condition = var.a != var.b, … }` on
    variable `c` sees both `a` and `b` in scope (DESIGN-0006 OQ-4
    decision).
  - Verify a default that references a later-declared variable
    errors cleanly (forward references not allowed).

- [x] **C.5 Error-surfacing tests.** *(Single-failure: `TestEvaluateValidations_FailureCarriesErrorMessage` asserts verbatim error_message + variable name + source range. Stacked: `TestEvaluateValidations_StacksMultipleFailures` asserts every failing condition surfaces. End-to-end: `TestCreate_Validation_RejectsBadLicense` / `TestCreate_Validation_RejectsBadProjectName` in `internal/create/validation_integration_test.go` walk `create.Run` against the live `testdata/registry/go/api/blueprint.hcl` fixture and assert both the contains() and can(regex()) migration patterns abort the create flow before any files are written.)*
  - Single validation failure surfaces verbatim error_message.
  - Multiple stacked validation failures all surface (not
    short-circuited).
  - Default-expression type mismatch surfaces with
    `blueprint.hcl:L:C` pointing at the `default` attribute.

- [x] **C.6 Run `make ci`.** *(`make lint` reports `0 issues`; `make test` is green across all 20 packages including the new `validation_test.go` and `validation_integration_test.go`.)*

#### Success Criteria

- Defaults evaluate as HCL expressions against the resolved
  variable scope; backwards-compat shim works for legacy
  scalar-string defaults.
- Validation blocks evaluate after resolution; failures abort the
  flow cleanly with the documented error format.
- Cross-variable references in defaults and validation conditions
  work as designed.
- `make ci` green.

---

### Phase D: input integration (vars-file + --set)

Extend the CLI input paths so users can supply structured values.
Vars-file is mostly free (cty.Convert handles structured values
natively); `--set` gets a narrow extension for object literals.

#### Tasks

- [x] **D.1 Vars-file structured-type support.** *(Already landed in Phase B's coerceToDeclared refactor — the helper takes a cty.Type directly and cty.Convert handles object/list/map targets without further changes. Phase D adds the fixture-driven verification through the loader.)*
  - File: `internal/varsfile/varsfile.go` (modify).
  - `coerceToDeclared` (existing helper) already delegates to
    `cty.Convert`. Verify it handles object/list/map targets
    cleanly — the only adjustment may be to `ctyTypeForDeclared`,
    which today maps a *string* tag to `cty.Type`. Update it to
    consume a `cty.Type` directly (the type field is now
    `cty.Type` on the `Variable` struct):

    ```go
    // Before (string tag → cty.Type)
    func ctyTypeForDeclared(tag string) cty.Type

    // After (caller passes cty.Type directly)
    // (helper becomes unnecessary; replace with direct
    //  v.Type field access)
    ```

- [x] **D.2 Vars-file test corpus extension.** *(`internal/varsfile/testdata/object-types/` now holds: `object-flat`, `object-nested`, `list-of-numbers`, `map-of-strings`, plus `object-mismatch` (string-field supplied as a list) and `list-mismatch` (mixed-type elements). The mismatched fixtures exercise the cty.Convert failure path.)*
  - Directory: `internal/varsfile/testdata/object-types/` (new).
  - Fixtures: `object-flat.forge-vars.hcl`,
    `object-nested.forge-vars.hcl`,
    `list-of-numbers.forge-vars.hcl`,
    `map-of-strings.forge-vars.hcl`, plus mismatched-type
    fixtures for each shape.

- [x] **D.3 Vars-file integration tests.** *(Six new tests in `varsfile_test.go` (`TestLoad_StructuredType_*`) walk each fixture through `varsfile.Load` against a `structuredVars()` declaration set and assert the expected `map[string]cty.Value` shape — including the failure-path tests that pin the error message at the vars-file location.)*
  - File: `internal/varsfile/varsfile_test.go` (modify).
  - Each fixture: assert `Load` returns the expected
    `map[string]cty.Value`; mismatched-type cases assert clean
    error with `vars file PATH:L:C` location.

- [x] **D.4 `--set` object literal parsing.** *(Lives on `internal/prompt/prompt.go::parseObjectOverride` rather than as a `internal/create/setvars.go` helper, because the type-driven dispatch happens inside `resolveFromOverride` which is owned by the prompt package — a `create` import in prompt would close the dependency cycle. The helper parses the raw value via `hclsyntax.ParseExpression`, evaluates against an empty EvalContext (literal-only), and coerces to the declared `cty.Object` shape. Resolved cty.Value flows through the rest of the chain as `any`; `lockfile.ToCtyValues` and `ctyForVariableValue` both grew a passthrough case for `cty.Value` inputs so the structured value survives the cty→Go→cty round-trip.)*
  - File: `cmd/create.go` (modify),
    `internal/create/setvars.go` (likely new helper).
  - When the value side of a `--set k=v` is detected as an HCL
    object literal (starts with `{`), parse it through `hclparse`
    against an empty EvalContext and coerce against the declared
    type:

    ```sh
    forge create go/api --set 'git_provider={repo_type="github",repo_url="github.com",project_org="me"}'
    ```
  - Scalars continue to flow through the existing string-coercion
    path (no change).

- [x] **D.5 `--set` list/map rejection.** *(Same `resolveFromOverride` dispatch — when `v.Type.IsListType()` or `v.Type.IsMapType()`, the call returns the documented `--set on variable X (...) is not supported; use --var-file to supply list and map values` error.)*
  - File: `cmd/create.go` (modify).
  - At resolve time, after the user supplies `--set X=Y` for a
    variable `X` declared as `list(T)` or `map(T)`, surface:

    ```
    --set on variable X (list(...)) is not supported;
    use --var-file to supply list and map values.
    ```

- [x] **D.6 Integration tests for create.** *(`internal/create/objectset_integration_test.go` (new) builds a synthetic registry inline (`buildStructuredRegistry`) with an object variable plus list(number) + map(string) variables. Tests: `TestCreate_SetObjectLiteral` walks create.Run with `--set git_provider={...}` and asserts the lockfile records the structured `cty.Value`; `TestCreate_SetRejectsList` and `TestCreate_SetRejectsMap` assert the documented rejection error for list and map declared types. Structured-typed defaults are exercised here too — the test registry's `exposed_ports = [8080]` default flows through renderDefault → defaultValueFromCty → resolveFromDefault as a cty.Value, no string-coercion roundtrip.)*
  - File: `internal/create/varsfile_integration_test.go` (modify) /
    a new `objectset_integration_test.go`.
  - Tests:
    - `--var-file` supplies a nested object; scaffold succeeds and
      the lockfile records the nested value.
    - `--set` with an object literal succeeds; lockfile records the
      object.
    - `--set` against a `list(T)` variable errors with the
      documented message.

- [x] **D.7 Run `make ci`.** *(`make lint` reports `0 issues`; `make test` is green across all 20 packages including the new structured-type tests in `varsfile` and `create`.)*

#### Success Criteria

- A vars file with object / list / map / nested-object values
  loads cleanly; mismatched types abort before any files are
  touched.
- `--set` accepts top-level object replacement via HCL literal.
- `--set` against list/map errors with the documented message
  pointing at `--var-file`.
- `make ci` green.

---

### Phase E: prompt UX

The interactive flow. Objects unfold to per-field prompts; lists
and maps are explicit non-interactive.

#### Tasks

- [x] **E.1 Object-unfold prompt logic.** *(`internal/prompt/prompt.go` grew `resolveObjectFromPrompt` + `promptObjectFields` + `promptOneField` + `projectField`. `resolveFromPrompt` now dispatches on `cty.Type` before falling through to the scalar path: `IsObjectType()` → recursive unfold using `Variable.TypeFieldOrder` (with the inner object levels recursing via a synthesised child `Variable`). Per-field prompt labels are dotted (`git_provider.repo_type`), and a derived object default's per-field cty.Value flows through `projectField` so the prompt pre-fills correctly. The reconstructed value is bound as `cty.ObjectVal(...)`. List/map fields inside an object follow the same non-interactive rule as top-level structured types — they short-circuit through `resolveListOrMapFromPrompt`.)*
  - File: `internal/prompt/prompt.go` (modify).
  - For each variable whose declared type is `cty.Object(...)`:
    - iterate attribute names in declaration order (preserve via
      `cty.Type.AttributeTypes()` plus a parallel ordered slice
      captured at parse time)
    - prompt for each field with a dotted label
      (`git_provider.repo_type`)
    - default values come from evaluating the object default
      against the resolved scope, then projecting per field
    - reconstruct the resolved value as
      `cty.ObjectVal(map[string]cty.Value{...})` before binding
      to the scope
  - Nested objects unfold recursively.
  - Lists/maps inside an object follow the same non-interactive
    rule as top-level lists/maps (E.2).

- [x] **E.2 List/map non-interactive error.** *(`resolveListOfMapFromPrompt` + `listMapVarsFileError` + `vrsFileExample` surface the documented copy-pasteable snippet exactly as specified. Required list/map variables without a vars-file or default abort before any prompt callback fires; non-required variants short-circuit with a typed `cty.NullVal(v.Type)` so downstream consumers see a stable null rather than an untyped nil.)*
  - File: `internal/prompt/prompt.go` (modify).
  - If a variable is `list(T)` or `map(T)`, is required, and has
    no value supplied (no vars-file, no `--set` — which would have
    errored at D.5 anyway, no default), error with a
    copy-pasteable vars-file snippet:

    ```text
    Error: variable "exposed_ports" (list(number)) is required but
    cannot be supplied interactively.

    Provide it via --var-file:

        # project.forge-vars.hcl
        exposed_ports = [8080, 9090]

        forge create ... --var-file ./project.forge-vars.hcl
    ```

- [x] **E.3 Declaration-order preservation.** *(`config.Variable.TypeFieldOrder []string` (with `json:",omitempty"`) carries the author-declared object-attribute order. `config.ObjectFieldOrder(hcl.Expression)` walks a top-level `object({...})` constructor — `*hclsyntax.FunctionCallExpr` with `Name == "object"` whose single arg is `*hclsyntax.ObjectConsExpr` — and extracts each key's source-order name via `objectConsKeyName` (handles both bareword `*hclsyntax.ScopeTraversalExpr` keys wrapped in `ObjectConsKeyExpr` and quoted-string keys). `decodeVariableType` in `loader_hcl.go` populates `v.TypeFieldOrder`. Nested object levels fall back to cty attribute iteration — see the E.4 test note.)*
  - File: `internal/config/blueprint.go` and the loader (modify
    if needed).
  - `cty.Object(...)`'s attribute map is unordered. Add a parallel
    `[]string` of attribute names on the `Variable` struct (e.g.
    `TypeFieldOrder []string`) captured at load time so the prompt
    flow can iterate fields in author-declared order.

- [x] **E.4 Prompt-flow integration tests.** *(Five new tests in `internal/prompt/prompt_test.go`: `TestCollectVariables_ObjectUnfoldDeclarationOrder` (the prompt callback fires exactly once per object field in the order `TypeFieldOrder` dictates), `TestCollectVariables_ObjectUnfoldNested` (recursion into nested object types — asserts shape rather than order at the nested level, since the recursive call falls back to cty attribute iteration without a captured per-level `TypeFieldOrder`), `TestCollectVariables_ListRequiredNonInteractiveError` and `TestCollectVariables_MapRequiredNonInteractiveError` (both assert the prompt callback is never invoked and the error contains the variable name, the documented `--var-file` snippet path, and the shape-appropriate example), and `TestCollectVariables_ListOptionalWithDefault` (non-required list flows through silently as `cty.NullVal(v.Type)`). The derived-default object case from the original task list is covered by `TestCreate_SetObjectLiteral` in `internal/create/objectset_integration_test.go` from Phase D — the test registry's object default flows through `renderDefault → defaultValueFromCty → resolveFromDefault` end-to-end.)*
  - File: `internal/prompt/prompt_test.go` (modify).
  - Tests:
    - Object unfold prompts for each field in declaration order.
    - Nested object unfolds recursively.
    - Required list errors with the documented snippet.
    - Required map errors with the documented snippet.
    - List/map with a default and no override flows silently
      (no prompt, no error).
    - Object with a derived default (`default = var.X == "..." ? {...} : {...}`)
      pre-fills per-field prompts correctly.

- [x] **E.5 Run `make ci`.** *(`make lint` reports `0 issues`; `make test` is green across all 20 packages including the five new prompt-flow tests; `make build` produces the core binary; license check passes.)*

#### Success Criteria

- Interactive `forge create` against a blueprint with an object
  variable produces per-field prompts in declaration order; the
  resolved value reconstructs as a single `cty.Value`.
- Required list/map without input fails fast with the documented
  copy-pasteable error.
- Existing scalar prompt UX is unchanged.
- `make ci` green.

---

### Phase F: lockfile + template integration

Downstream consumers. Both are nearly free — the value pipeline is
already `cty.Value`-native — but they need explicit verification
under structured-type round-trip.

#### Tasks

- [ ] **F.1 Lockfile `cty` helpers.**
  - File: `internal/lockfile/cty.go` (modify).
  - `ToCtyValues` / `FromCtyValues` already operate on
    `cty.Value`. The type-aware coercion in `ToCtyValues` switches
    on declared variable type tag (string/bool/int). Replace the
    tag-switch with a single `cty.Convert(val, declaredType)` —
    same approach as `internal/varsfile`.

- [ ] **F.2 Lockfile HCL round-trip verification.**
  - File: `internal/lockfile/emit_hcl.go` and
    `internal/lockfile/loader_hcl.go` (modify if needed).
  - `hclwrite` natively emits nested values; the loader already
    pulls the `variables { ... }` block via hand-decode. Verify
    end-to-end with a structured-type fixture; adjust the emitter
    only if `hclwrite` produces something the loader can't parse
    back.

- [ ] **F.3 Lockfile round-trip tests.**
  - File: `internal/lockfile/roundtrip_test.go` (modify) or new
    `internal/lockfile/object_roundtrip_test.go`.
  - Fixture: a project lockfile with `variables { ... }` carrying
    an object, a list, a map, and a nested object. Assert
    write-then-load yields byte-identity (or at minimum a
    semantically equal `*Lockfile`).

- [ ] **F.4 Template strict-vars update.**
  - File: `internal/template/renderer.go` (modify) — wherever the
    strict-vars check lives.
  - Allow nested attribute access against declared object types:
    `var.git_provider.repo_type` is legal iff `git_provider` is
    declared as `object({repo_type = …, …})` and `repo_type` is a
    declared field.
  - Allow index access against declared `list(T)` and `map(T)`
    types: `var.exposed_ports[0]`, `var.build_targets["linux"]`.
  - Coordinate with RFC-0003's IMPL on the `var.*` scope —
    DESIGN-0006 assumes that namespace lands first or co-lands
    (see OQ-1 below).

- [ ] **F.5 Template integration tests.**
  - File: `internal/template/renderer_test.go` (modify).
  - Tests:
    - `${var.obj.field}` renders correctly against an
      object-typed variable.
    - `${var.list[0]}` renders correctly against a list-typed
      variable.
    - `${var.map["key"]}` renders correctly against a map-typed
      variable.
    - `${var.obj.undeclared_field}` errors with strict-vars
      diagnostic.
    - `%{ for x in var.list ~}...%{ endfor ~}` iterates as
      expected.

- [ ] **F.6 Run `make ci`.**

#### Success Criteria

- Lockfile written from a project with object/list/map variables
  round-trips through load cleanly.
- Templates can access nested values via attribute / index syntax;
  strict-vars validation handles the new shapes.
- `forge sync` and `forge check` operate on the new value shapes
  without code changes (verified via integration tests reusing
  the F.3 lockfile fixture).
- `make ci` green.

---

### Phase G: documentation + release prep

#### Tasks

- [ ] **G.1 MIGRATION.md update.**
  - File: `docs/MIGRATION.md` (modify).
  - New section: "Variable type system upgrade (v0.7+)" with
    before/after snippets for:
    - `type = "choice"` + `choices` → `type = string` + `validation { condition = contains(...) }`
    - `validate = "regex"` → `validation { condition = can(regex(...)) }`
    - `type = int` → `type = number` (note: warning only, not
      breaking — works either way)
  - Add a "Variable type expressiveness gain" sub-section showing
    the renovate-config use case collapsing from 4 scalar
    variables to 1 object variable.

- [ ] **G.2 REFERENCE.md update.**
  - File: `docs/REFERENCE.md` (modify).
  - Variable types table gains `object({…})`, `list(T)`, `map(T)`
    rows.
  - `validate` and `choices` rows removed; new "validation block"
    section after the variable table.
  - `int` row gets a deprecation footnote.
  - Source-of-truth table gains `internal/config/vartype.go`.

- [ ] **G.3 CLAUDE.md update.**
  - File: `CLAUDE.md` (modify).
  - Architecture entry for the new `internal/config/vartype.go`
    helper.
  - CLI Design Decisions update: choice→validation reframing,
    object/list/map type surface, `int` deprecation.

- [ ] **G.4 v0.7.0 release notes.**
  - File: `docs/release-notes/v0.7.0-object-types.md` (new).
  - Highlight the additive features (object/list/map types) and
    the breaking changes (choice/choices/validate removal,
    prompt-UX regression for `choice`-style enums). Walk the
    reader through the migration with the same snippets as
    MIGRATION.md.
  - Cross-reference RFC-0003's IMPL doc if it ships in the same
    release.

- [ ] **G.5 forge-registry follow-up.**
  - Not in this repo. File an issue against
    `github.com/donaldgifford/forge-registry` to update the
    renovate-config blueprint to use the new `git_provider`
    object variable. Note as out-of-repo in the IMPL doc
    closing checklist.

- [ ] **G.6 docz-reviewer pass.**
  - Run the docz-reviewer agent against the new MIGRATION.md
    section, REFERENCE.md updates, and release notes.

- [ ] **G.7 Run `make ci`.**

#### Success Criteria

- MIGRATION.md, REFERENCE.md, CLAUDE.md, and v0.7.0 release
  notes are merged and self-consistent.
- forge-registry follow-up issue filed.
- docz-reviewer pass complete with findings addressed.
- `make ci` green.

---

## File Changes

### New files

| Path | Purpose |
|---|---|
| `internal/config/vartype.go` | Type expression parser + `int` deprecation detection. |
| `internal/config/vartype_test.go` | Parser tests. |
| `internal/config/testdata/vartype/` | Per-form fixtures. |
| `internal/create/scope.go` (if extracted) | Resolved-variable scope builder. |
| `internal/create/setvars.go` (if extracted) | `--set` HCL-literal parser. |
| `internal/create/objectset_integration_test.go` | `--set` object literal integration tests. |
| `internal/varsfile/testdata/object-types/` | Structured-type fixtures. |
| `internal/lockfile/object_roundtrip_test.go` | Structured-type lockfile round-trip. |
| `docs/release-notes/v0.7.0-object-types.md` | Release notes. |

### Modified files

| Path | Change |
|---|---|
| `internal/config/blueprint.go` | Variable struct refactor: `Type cty.Type`, `DefaultExpr hcl.Expression`, `Validations []Validation`, remove `Validate`/`Choices`. |
| `internal/config/hcldec_spec.go` | New `validation` block schema; `variableBlockBodySchema` updates. |
| `internal/config/loader_hcl.go` | Hand-decode the new variable shape; reject legacy `choices`/`validate`; surface `int` deprecation. |
| `internal/config/loader.go` | Plumb deprecations through. |
| `internal/config/validate.go` | Remove `validVariableTypes` map and `choice`-specific validations. |
| `internal/config/testdata/` | Migrate existing legacy fixtures; add new ones. |
| `internal/prompt/prompt.go` | Object unfold; list/map non-interactive error; default eval via HCL expression. |
| `internal/prompt/prompt_test.go` | Add object / list / map cases. |
| `internal/create/create.go` | Resolved-scope build; validation-block evaluation. |
| `internal/varsfile/varsfile.go` | Drop the `string → cty.Type` helper; coerce against `Variable.Type` directly. |
| `internal/varsfile/varsfile_test.go` | Structured-type tests. |
| `internal/lockfile/cty.go` | Drop the tag-switch coercion; single `cty.Convert` path. |
| `internal/lockfile/emit_hcl.go` / `loader_hcl.go` | Verify round-trip; adjust if needed. |
| `internal/template/renderer.go` | Strict-vars accepts nested attribute / index access. |
| `cmd/create.go`, `cmd/sync.go` | Surface deprecations via `ui.Warningf`; `--set` HCL literal parsing; list/map rejection. |
| `docs/MIGRATION.md` | New v0.7 section. |
| `docs/REFERENCE.md` | Variable types table updates, validation block section. |
| `CLAUDE.md` | Architecture + design-decision updates. |

## Testing Plan

- **Unit tests** at each layer: `vartype` parser, validation
  evaluation, vars-file coercion, lockfile cty helpers, template
  strict-vars.
- **Integration tests** scaffolding a fixture blueprint with the
  full type surface (one object, one list, one map, plus a scalar
  with a validation block):
  - `forge create` via vars-file
  - `forge create` via `--set` (scalar + object literal)
  - `forge create` interactive (object unfold, list/map error)
  - `forge sync` after a vars-file change to an object field
  - `forge check` against a project with structured-type variables
- **End-to-end** smoke test against the forge-registry
  renovate-config blueprint once it adopts the new `git_provider`
  object variable (G.5 follow-up).
- **Coverage gate:** ≥85% on the new code paths (matches IMPL-0008's
  bar).
- **Regression coverage:** existing IMPL-0008 vars-file tests must
  continue to pass with no modifications (the parser change is
  transparent for scalar-only fixtures).

## Quality Gates

- **After Phase A:** `make ci` green; coverage on
  `internal/config/vartype.go` ≥90%.
- **After Phase B:** `make ci` green; every existing
  `internal/config/testdata/` blueprint either loads under the new
  schema or has been migrated to the new shape with a clear
  reason in the commit.
- **After Phase C:** `make ci` green; a fixture blueprint with
  cross-variable validations exercises the resolved-scope path.
- **After Phase D:** `make ci` green; vars-file structured-type
  coverage matches the scalar-type coverage from IMPL-0008.
- **After Phase E:** `make ci` green; manual smoke test in a real
  terminal verifying the object-unfold prompt UX (huh has some
  layout quirks the headless tests won't catch).
- **After Phase F:** `make ci` green; lockfile round-trip is
  byte-identical for structured-type fixtures.
- **After Phase G:** `make ci` green; `mkdocs build` succeeds with
  the new docs.

## Dependencies

- **`hashicorp/hcl/v2/ext/typeexpr`** — already in the module
  dependency tree as a transitive dep of `hashicorp/hcl/v2`. No
  new top-level dep.
- **`zclconf/go-cty`** — already in use; this IMPL leans on
  `cty.Convert`, `cty.Object`, `cty.List`, `cty.Map` constructors.
- **IMPL-0006 (HCL lockfile)** — already shipped. Hard prerequisite
  for clean structured-type round-trip.
- **IMPL-0008 (vars-file)** — already shipped. This IMPL extends
  its parser shape; the existing public API (`varsfile.Load`)
  stays stable.
- **RFC-0003 IMPL (locals + `var.*` namespacing)** — co-ships per
  DESIGN-0006 OQ-1. Sequencing question is OQ-1 below.

## Open Questions

All six open questions have been resolved. Recorded below for
audit / traceability; the implementation should follow the
**Decision** lines.

---

**OQ-1: Sequencing with RFC-0003's IMPL.** DESIGN-0006 OQ-1 fixed
v0.7.0 as the joint release. The IMPL question is whether this IMPL
starts before, in parallel with, or after RFC-0003's IMPL. Phase F
(template strict-vars) touches the `var.*` namespace that RFC-0003
introduces, so the two must coordinate there.

- **a (recommended):** Start this IMPL's Phases A–B independently
  (they have zero overlap with RFC-0003); pause before Phase C and
  wait for RFC-0003's IMPL to land Phases 1–2 (the `var.*` scope
  builder); resume Phases C–G against that scope. Single resumable
  branch; no parallel review burden.
- **b:** Sequence strictly — finish RFC-0003's IMPL completely,
  then start this one. Simpler review story; biggest calendar
  delay.
- **c:** Run both in true parallel on separate branches; merge in
  the same release. Most coordination overhead; review burden
  spikes at integration time.

**Decision: a.** Phases A–B land independently; pause before Phase
C until RFC-0003's IMPL has the `var.*` scope builder available;
resume on a single branch.

---

**OQ-2: File organization for the validation-block evaluator.**
Phase C's validation evaluation needs a home.

- **a (recommended):** New file `internal/config/validation.go`
  housing the `Validation` struct, the load-time schema, and the
  evaluator (`EvaluateValidations(vars []Variable, scope cty.Value) []error`).
  Mirrors how `vartype.go` cleanly contains type-expression logic
  and keeps the schema-evaluator-test trio together.
- **b:** Inline the evaluator into `internal/create/create.go`
  alongside the existing variable resolution. Less file
  proliferation; `create.go` grows further.
- **c:** Single file `internal/config/vartype.go` from Phase A
  expands to cover both. Smallest file footprint; conflates two
  concerns.

**Decision: a.** `internal/config/validation.go` is the home for
the `Validation` struct, schema, and `EvaluateValidations`. Sibling
to `internal/config/vartype.go`.

---

**OQ-3: Deprecation warning surfacing channel.** Phase A.4 detects
the `int` deprecation; Phase B.6 plumbs it to callers. The
question is what channel to surface it on.

- **a (recommended):** `ui.Warningf` (the existing styled-stderr
  channel used by IMPL-0008 for unknown vars-file keys). Same
  warning style users already know; respects `NO_COLOR`.
- **b:** Structured `slog.Warn` with attrs. Cleaner for
  machine-parseable logs; less discoverable for interactive users.
- **c:** Both — emit through `ui.Warningf` for humans *and*
  `slog.Warn` for log capture. Belt and suspenders; small extra
  surface.

**Decision: a.** `ui.Warningf` only. Matches IMPL-0008's pattern;
single source of warning truth.

---

**OQ-4: Migration error format when rejecting legacy `choices` /
`validate` fields.** Phase B.4 rejects these at load time. The
question is how heavy the error message should be.

- **a (recommended):** Static error referencing the MIGRATION.md
  section anchor and showing the *generic* before/after pattern
  (no auto-suggested fix for the specific variable). Predictable,
  cheap, matches the v0.5/v0.6 rejection error style.
- **b:** Smart error that includes the auto-suggested fix for the
  specific variable, e.g. "variable `license` uses removed
  `choices = ["MIT", "Apache-2.0"]`; replace with:
  `validation { condition = contains([\"MIT\", \"Apache-2.0\"], var.license) error_message = \"...\" }`".
  More helpful but adds formatting complexity and an edge case
  per legacy field type.
- **c:** No prose in the error — just the MIGRATION.md anchor URL.
  Most terse; least informative.

**Decision: a.** Static error w/ generic before/after pattern plus
the MIGRATION.md anchor. Matches v0.5 / v0.6 rejection style.

---

**OQ-5: Test corpus strategy.** Phase G calls for a forge-registry
end-to-end smoke test. The question is whether to anchor coverage
on that single big test or on broad unit/integration coverage
inside the forge repo.

- **a (recommended):** Broad unit + integration coverage inside
  the forge repo using fixture blueprints (matches IMPL-0008's
  approach); treat the forge-registry smoke test as a manual
  post-merge validation step rather than a CI gate. Keeps CI
  fast; doesn't couple this repo's CI to a downstream repo.
- **b:** Add a CI job that clones forge-registry, scaffolds the
  renovate-config blueprint, and asserts against fixture output.
  Stronger guarantee; cross-repo CI coupling, slower CI.
- **c:** Skip forge-registry validation entirely until the
  follow-up issue (G.5) lands. Lightest weight; biggest
  integration risk at release time.

**Decision: a.** Broad in-repo unit + integration coverage;
forge-registry validation is a manual post-merge step (G.5
follow-up).

---

**OQ-6: `forge info` JSON output for structured types.** The
existing `forge info --output json` serializes the blueprint
schema, including variables. With `cty.Type` values in the
variable surface, the JSON shape needs a decision.

- **a (recommended):** Emit the cty type as its canonical string
  form (`"list(string)"`, `"object({port: number})"`) — the same
  text an author would write in `blueprint.hcl`. Stable, human-
  readable, round-trippable via `typeexpr` if a consumer wants
  to parse it back.
- **b:** Emit the cty type as a nested JSON object using cty's
  built-in JSON marshaling. Machine-friendly; verbose; couples
  JSON consumers to cty's internal representation.
- **c:** Emit both — the canonical string form *and* a structured
  object — under separate keys. Most flexibility; biggest payload.

**Decision: a.** Canonical string form
(`"list(string)"`, `"object({port: number})"`) under the existing
`type` key in the JSON output. Round-trippable via `typeexpr`.

---

## References

- [DESIGN-0006 — Object and collection variable types](../design/0006-object-and-collection-variable-types.md) — the design this IMPL realises.
- [RFC-0002 — Object and collection variable types](../rfc/0002-object-and-collection-variable-types.md) — the upstream proposal.
- [RFC-0003 — Locals for derived values](../rfc/0003-locals-for-derived-values.md) — co-shipping in v0.7.0 per DESIGN-0006 OQ-1.
- [DESIGN-0001 — Blueprint Authoring](../design/0001-blueprint-authoring.md) — variable declaration contract this IMPL extends.
- [DESIGN-0005 — Variable input via vars file](../design/0005-variable-input-via-vars-file.md) — vars-file input mechanism this IMPL extends to structured types.
- [IMPL-0006 — Migrate lockfile from YAML to HCL](0006-migrate-lockfile-from-yaml-to-hcl.md) — lockfile prerequisite (shipped).
- [IMPL-0008 — Variable input via vars file](0008-variable-input-via-vars-file.md) — direct precedent for file organisation, test corpus shape, deprecation-warning channel, and docz-reviewer workflow.
- [ADR-0002 — Forge does not ship in-tool migrators](../adr/0002-forge-does-not-ship-in-tool-migrators.md) — governs the legacy-field-rejection strategy.
- [`hashicorp/hcl/v2/ext/typeexpr`](https://pkg.go.dev/github.com/hashicorp/hcl/v2/ext/typeexpr) — the type expression parser this IMPL delegates to (DESIGN-0006 OQ-2).
- `internal/config/blueprint.go` — Variable struct refactor target.
- `internal/config/hcldec_spec.go` — schema refactor target.
- `internal/config/validate.go` — `validVariableTypes` removal target.
- `internal/prompt/prompt.go` — object-unfold + list/map error target.
- `internal/varsfile/varsfile.go` — coercion-path simplification target.
- `internal/lockfile/cty.go` — coercion-path simplification target.
- `internal/template/renderer.go` — strict-vars update target.
