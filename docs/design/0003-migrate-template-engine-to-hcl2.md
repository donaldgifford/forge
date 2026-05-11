---
id: DESIGN-0003
title: "Migrate Template Engine to HCL2"
status: Implemented
author: Donald Gifford
created: 2026-05-07
updated: 2026-05-08
---
<!-- markdownlint-disable-file MD025 MD041 -->

# DESIGN 0003: Migrate Template Engine to HCL2

**Status:** Implemented
**Author:** Donald Gifford
**Date:** 2026-05-07
**Implemented:** 2026-05-08 (IMPL-0004 Phases A–C landed; D.5 forge-registry
migration is an external follow-up tracked separately).

<!--toc:start-->
- [Overview](#overview)
- [Goals and Non-Goals](#goals-and-non-goals)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Background](#background)
- [Detailed Design](#detailed-design)
  - [Why one-shot, not parallel](#why-one-shot-not-parallel)
  - [Template syntax — v1 vs v2](#template-syntax--v1-vs-v2)
    - [File content (.tmpl files)](#file-content-tmpl-files)
    - [File and directory paths](#file-and-directory-paths)
    - [variable.default](#variabledefault)
    - [condition.when](#conditionwhen)
    - [rename patterns](#rename-patterns)
  - [Engine architecture](#engine-architecture)
  - [Function map](#function-map)
  - [apiVersion: v2 semantics](#apiversion-v2-semantics)
  - [forge migrate templates](#forge-migrate-templates)
    - [Usage](#usage)
    - [Behavior](#behavior)
    - [Rewrite rules](#rewrite-rules)
    - [Out-of-scope rewrites (warn, don't translate)](#out-of-scope-rewrites-warn-dont-translate)
    - [Error mode](#error-mode)
  - [Migration UX](#migration-ux)
- [API / Interface Changes](#api--interface-changes)
  - [Public CLI](#public-cli)
  - [Public Go packages](#public-go-packages)
- [Data Model](#data-model)
- [Testing Strategy](#testing-strategy)
  - [Unit tests](#unit-tests)
  - [Integration tests](#integration-tests)
  - [Compatibility tests](#compatibility-tests)
- [Migration / Rollout Plan](#migration--rollout-plan)
  - [Suggested implementation phases (for IMPL-0004)](#suggested-implementation-phases-for-impl-0004)
- [Resolved Questions](#resolved-questions)
- [References](#references)
<!--toc:end-->

## Overview

This design specifies the cutover from Go `text/template` to HCL2
(`hashicorp/hcl/v2`) as forge's blueprint template engine. ADR-0001
captures the decision; this document describes the surface change
authors will see, the engine swap inside `internal/template/`, and
the one-shot migration tool that converts existing v1 blueprints to
v2.

The cutover is a **breaking change** with no parallel-format support
window. v1 blueprints stop loading; `forge migrate templates` rewrites
them to v2; v2 is the only format forge accepts after this lands.

## Goals and Non-Goals

### Goals

- Replace the Go `text/template`-based renderer with an HCL2-backed
  renderer while keeping the orchestrator-facing `Renderer` API
  (`RenderFile`, `RenderString`, `RenderPath`) source-compatible.
- Define the v2 `apiVersion` semantics for `blueprint.yaml` and
  `registry.yaml`: which fields change, which stay the same, and how
  load-time validation errors point users at the migration tool.
- Specify `forge migrate templates` — input, output, the rewrite
  rules it applies, and what it explicitly does *not* try to convert
  (where human review is required).
- Re-implement the existing custom function map as
  `function.Function` definitions so authors lose no functionality.

### Non-Goals

- Phased / parallel-format support. v1 blueprints are intentionally
  no longer accepted post-cutover. (See "Why one-shot, not parallel"
  below for the reasoning.)
- Schema changes to `blueprint.yaml` or `registry.yaml` beyond the
  `apiVersion` bump and the expression-syntax fields. Top-level
  fields (`name`, `description`, `tags`, `version`,
  `defaults.exclude`, `sync.managed_files`, etc.) keep their
  current shape and types.
- Adding new template features (loops over arbitrary collections,
  expression-level imports, etc.). Anything beyond parity with the
  v1 contract is out of scope and goes into a follow-up design.
- Changes to `forge create` / `sync` / `check` orchestration logic
  beyond the renderer swap.

## Background

INV-0001 confirmed that Go `text/template` collides with the `{{ }}`
syntax used by Helm/Argo/Kustomize/GitHub Actions reusable workflows
and that even `missingkey=zero` only protects depth-1 access. Both
candidate fixes (custom delimiters, HCL2) require rewriting every
existing `.tmpl` file. ADR-0001 chose HCL2 because the migration
cost is comparable but the destination is materially better
(typed values, native conditionals, diagnostic-quality errors,
familiar syntax for the Terraform/Packer/Nomad audience).

## Detailed Design

### Why one-shot, not parallel

The user explicitly chose a clean break over a parallel-support
window. The reasoning, captured here so it stays with the design:

- **Forever-tax of dual rendering.** Every call site in
  `internal/create/`, `internal/sync/`, `internal/check/`,
  `internal/prompt/`, and `internal/template/` would carry both
  engines. Each new feature would need to be implemented twice or
  gated behind apiVersion checks. The maintenance tax dwarfs the
  cutover pain.
- **Two engines means two test matrices.** Integration tests would
  need v1 and v2 fixtures for every behavior. CI runtime and review
  surface roughly double for the lifetime of the dual-support
  window.
- **Migration tools are easier to ship than dual code paths.** A
  one-shot rewrite is a small, well-scoped artefact authored once
  and used a handful of times per registry. Dual engines are
  permanent.
- **The forge user base is small enough today.** The blast radius
  of a clean break is bounded. Every additional release we wait
  expands the population of v1 blueprints we'd need to migrate.

The cost of this choice is that registry maintainers must run
`forge migrate templates` (or rewrite by hand) before they can
use the post-cutover forge release. This is documented in
`forge migrate --help`, the changelog, the README, and a load-time
error message that points directly at the migration command.

### Template syntax — v1 vs v2

Side-by-side reference for the four expression contexts blueprints
care about today.

#### File content (`.tmpl` files)

```
# v1 (text/template)
module {{ .go_module }}

# Hello, {{ .project_name | upper }}!
{{ if .use_grpc }}
import "google.golang.org/grpc"
{{ end }}
```

```
# v2 (HCL2)
module ${go_module}

# Hello, ${upper(project_name)}!
%{ if use_grpc ~}
import "google.golang.org/grpc"
%{ endif ~}
```

Notes:

- Variables are bare identifiers (no `.` prefix).
- Function calls are `name(arg, …)`, not piped (`arg | name`).
- Conditionals use `%{ if … ~}` / `%{ else ~}` / `%{ endif ~}`
  directives. The `~` strips trailing whitespace, matching the
  common HCL convention.
- Iteration: `%{ for x in xs ~} … %{ endfor ~}`. Not used in v1
  blueprints today, but supported.

#### File and directory paths

```
# v1
{{project_name}}/cmd/main.go.tmpl
```

```
# v2
${project_name}/cmd/main.go.tmpl
```

The path renderer's "shorthand `{{var}}` → `{{.var}}`" normaliser at
`internal/template/renderer.go:79` is removed. HCL2 expressions
reference variables by bare name, so no normaliser is needed.

#### `variable.default`

```yaml
# v1
variables:
  - name: go_module
    type: string
    default: "github.com/{{ .org }}/{{ .project_name }}"
```

```yaml
# v2
variables:
  - name: go_module
    type: string
    default: "github.com/${org}/${project_name}"
```

The `default` field stays a string, but its expression syntax is HCL.
It is rendered after earlier variables resolve, exactly as today.

#### `condition.when`

```yaml
# v1
conditions:
  - when: '{{ eq .ci_provider "none" }}'
    exclude:
      - .github/
```

```yaml
# v2
conditions:
  - when: 'ci_provider == "none"'
    exclude:
      - .github/
```

`when` becomes a real HCL expression that evaluates to `bool`.
This is a strict upgrade — the v1 form required the template to emit
the literal string `"true"` or `"false"`. v2 evaluates the
expression directly via `cty.Value`, so type errors are caught at
parse time.

#### `rename` patterns

```yaml
# v1
rename:
  "{{project_name}}/": "."
```

```yaml
# v2
rename:
  "${project_name}/": "."
```

Same shape; just HCL interpolation syntax.

### Engine architecture

The internal API stays small and source-compatible from the
orchestrator's point of view. The renderer's responsibilities are:

```go
package template

type Renderer struct {
    funcs map[string]function.Function // cty stdlib + forge custom
}

func NewRenderer() *Renderer
func (r *Renderer) RenderFile(path string, vars map[string]cty.Value) ([]byte, error)
func (r *Renderer) RenderString(tmpl string, vars map[string]cty.Value) (string, error)
func (r *Renderer) RenderPath(path string, vars map[string]cty.Value) (string, error)
func (r *Renderer) EvaluateBool(expr string, vars map[string]cty.Value) (bool, error)
```

The `vars` type changes from `map[string]any` to
`map[string]cty.Value`. `internal/prompt/` is responsible for
producing properly-typed `cty.Value`s from user input
(strings/bools/numbers). All downstream renderers consume them
unchanged.

`EvaluateBool` is new — it replaces the v1 trick of "render a
template that emits `true`/`false` and parse the result". HCL2 lets
us evaluate a `when:` expression to a real `cty.Bool` and check it
directly. `EvaluateBool` evaluates against **the same `EvalContext`
as `RenderFile`/`RenderString`/`RenderPath`** — `condition.when`
sees every variable that file content sees, with no scoping
restrictions. This keeps the mental model simple ("conditions can
reference any variable") and matches v1 behavior.

Internally, the renderer:

1. Parses the input via `hclsyntax.ParseTemplate` (for files,
   strings, and paths) or `hclsyntax.ParseExpression` (for `when:`).
2. Builds an `hcl.EvalContext` from `vars` and the configured
   function map.
3. Calls `expr.Value(ctx)` and converts the resulting `cty.Value` to
   a `string` (file content, paths) or `bool` (`when:`).
4. Surfaces `hcl.Diagnostics` as native errors with file/line/col
   ranges.

### Function map

The v2 function map exposes the same names authors use today, plus
the cty stdlib subset that's broadly useful.

| Function | Source | Notes |
|----------|--------|-------|
| `upper` | `cty/stdlib.UpperFunc` | Drop-in |
| `lower` | `cty/stdlib.LowerFunc` | Drop-in |
| `title` | re-implemented | cty stdlib doesn't have title-case |
| `replace` | `cty/stdlib.ReplaceFunc` | Argument order matches HCL convention (`replace(s, old, new)`); migration tool reorders v1 pipes |
| `trimPrefix` | re-implemented (small wrapper over `strings.TrimPrefix`) | cty stdlib has `trim`/`trimspace` but not prefix/suffix |
| `trimSuffix` | re-implemented | as above |
| `now` | re-implemented | takes a Go time-format string |
| `env` | re-implemented | wraps `os.Getenv` |
| `coalesce` | `cty/stdlib.CoalesceFunc` | Replaces v1 `default(val, fallback)` — returns first non-null arg. Aligns with HCL idiom |
| `snakeCase` | re-implemented | from current `internal/template/funcs.go` |
| `camelCase` | re-implemented | as above |
| `pascalCase` | re-implemented | as above |
| `kebabCase` | re-implemented | as above |

Re-implementations live in a new `internal/template/funcs.go`
(replacing the current file) as `function.Function` instances.
Argument-handling for the case-conversion funcs is straightforward
since they all take a single `string` and return a `string`.

### `apiVersion: v2` semantics

Only one v1→v2 schema change is required: the `apiVersion` field
itself.

```yaml
# v2 blueprint.yaml
apiVersion: v2
name: go-api
# … rest unchanged in shape
```

Loader behaviour:

- `LoadBlueprint(path)` reads `apiVersion`. If the value is `v1`,
  return a load error:

  ```
  blueprint.yaml at <path>: apiVersion v1 is no longer supported.
  Run `forge migrate templates --path <registry-or-blueprint>` to
  convert this blueprint to v2 (HCL2 templates).
  See docs/MIGRATION.md in the forge repository for the v1→v2
  migration guide.
  ```

  The migration guide ships as `docs/MIGRATION.md` in the forge
  repository (a plain markdown file readable on GitHub) rather than
  a hosted docs site. We can promote it to a hosted page later if
  the wiki/MkDocs deployment lands; the in-repo path stays canonical
  in the meantime.

- If the value is anything other than `v2`, return a load error
  listing the supported versions.
- All other validation (variable types, required fields, regex
  compilability) runs unchanged.

`registry.yaml` keeps its current `apiVersion: v1`. Only
`blueprint.yaml` cares about the engine change because that's where
the templated expressions live. (If we later need to bump
`registry.yaml` for unrelated reasons, we'll do it then.)

### `forge migrate templates`

A new subcommand under `forge migrate`. Its job is to mechanically
rewrite a v1 blueprint (or a registry containing many v1 blueprints)
into v2.

#### Usage

```bash
# Migrate one blueprint
forge migrate templates --path ./my-blueprint

# Migrate all blueprints under a registry
forge migrate templates --path ./my-registry

# Dry-run: print what would change, don't write
forge migrate templates --path ./my-registry --dry-run

# Be loud about anything the tool refuses to convert
forge migrate templates --path ./my-registry --strict
```

#### Behavior

1. **Discover blueprints.** Walk `--path`. For each `blueprint.yaml`
   found, treat it as a blueprint root.
2. **For each blueprint root:**
   a. **Refuse to migrate already-v2 blueprints.** If `apiVersion`
      is already `v2`, skip with an info message.
   b. **Rewrite `blueprint.yaml` expression fields.** For
      `variable.default`, `condition.when`, `rename` keys/values,
      apply the rewrite rules below.
   c. **Walk the blueprint directory tree.** For each file ending
      in `.tmpl`, apply the rewrite rules.
   d. **Bump `apiVersion: v1` → `v2`** as the last step in
      `blueprint.yaml`. (We do this last so a tool failure mid-run
      doesn't leave a partially-converted blueprint claiming to be
      v2.)
3. **Print a summary.** Files rewritten, files skipped, expressions
   the tool refused to translate (see "Out-of-scope rewrites"
   below).

#### Rewrite rules

The tool ships a deterministic set of rewrites. Anything outside the
rule set is left alone and surfaced in the summary so the author can
fix it by hand.

| v1 syntax | v2 syntax | Notes |
|-----------|-----------|-------|
| `{{ .name }}` / `{{ name }}` | `${name}` | Bare or dotted identifier; trim leading dot |
| `{{ .a.b }}` | `${a.b}` | Nested attribute access — translates verbatim |
| `{{ funcname .a }}` | `${funcname(a)}` | Single-arg function call |
| `{{ .a \| funcname }}` | `${funcname(a)}` | Pipe call → positional call |
| `{{ .a \| funcname "arg" }}` | `${funcname(a, "arg")}` | Pipe with extra arg → reordered positional |
| `{{ if .x }}` … `{{ end }}` | `%{ if x ~}` … `%{ endif ~}` | Conditional |
| `{{ if .x }}` … `{{ else }}` … `{{ end }}` | `%{ if x ~}` … `%{ else ~}` … `%{ endif ~}` | If/else |
| `{{ eq .x "y" }}` (in `when:`) | `x == "y"` | `when:` expressions evaluated as HCL bool, not template string |
| `{{ ne .x "y" }}` | `x != "y"` | as above |
| `{{ not .x }}` | `!x` | as above |
| `{{ "{{" }}` | `{{` | Literal-emit-`{{` escape becomes a plain literal |
| `{{ "}}" }}` | `}}` | as above |
| `{{project_name}}` (in path) | `${project_name}` | Path shorthand stripped — HCL doesn't need it |
| `{{ default .x "fallback" }}` / `{{ .x \| default "fallback" }}` | `${coalesce(x, "fallback")}` | v1 `default` becomes HCL `coalesce`. Argument order does not change. |

The rewrite-rule set will be **validated against
[`github.com/donaldgifford/forge-registry`](https://github.com/donaldgifford/forge-registry)
before the tool ships** — that is the first registry we will migrate,
and it serves as the corpus for deciding whether to extend the rules
(e.g. three-arg pipe handling, currently out-of-scope below). If the
corpus exposes patterns the table above doesn't cover and a regex sed
can reliably translate them, the rule set grows; otherwise the
patterns stay out-of-scope and the tool warns.

#### Out-of-scope rewrites (warn, don't translate)

The tool will not attempt to translate the following automatically.
It logs each occurrence with file/line and continues.

- `{{ range … }}` blocks. v1 forge templates rarely use these, but
  when they do, the iteration shape is closer to HCL's `%{ for … }`
  than a regex sed can handle. Author rewrites by hand.
- `{{ with … }}` blocks. Same reasoning.
- Templates that define helper sub-templates with `{{ define "x" }}`
  / `{{ template "x" }}`. v1 forge doesn't expose template-include
  semantics; if a blueprint reaches for them, the author should
  redesign for HCL.
- Custom-func calls that take more than two arguments and use a
  pipe in the middle (`{{ .a | f .b | g .c }}`). The tool can't
  reliably re-order without knowing each func's signature. Author
  rewrites by hand; the summary lists the file/line.

#### Error mode

- The tool runs each blueprint independently. A failure on one
  blueprint reports the failure and continues with the next.
- `--strict` causes the tool to exit non-zero if *any* blueprint
  has out-of-scope expressions, so CI can gate registry pushes on
  a clean migration.
- The tool writes files in place. Authors are expected to commit
  before running it; the tool refuses to run inside a dirty git
  worktree unless `--force` is set.

### Migration UX

Concrete user journey for an external registry maintainer the day
the cutover ships:

1. Update `forge` (any post-cutover version).
2. Run `forge migrate templates --path ./my-registry --dry-run` to
   see what will change.
3. Run `forge migrate templates --path ./my-registry` to apply.
4. Run `forge create` against a v2 blueprint to verify.
5. Commit, push, and announce to consumers.

For consumers (people running `forge create` against the registry):
no action required, as long as their registry has been migrated.
If they hit a v1 blueprint after the cutover, they get a load
error that points them at the registry maintainer.

## API / Interface Changes

### Public CLI

- New: `forge migrate templates --path <dir> [--dry-run] [--strict] [--force]`.
- Changed: `forge create`, `forge sync`, `forge check` reject v1
  blueprints with the load-time error described above. No flag
  changes.

### Public Go packages

- `internal/template`: API shape preserved (`NewRenderer`,
  `RenderFile`, `RenderString`, `RenderPath`); implementation
  rewritten on HCL2; new `EvaluateBool`. The `vars` parameter type
  changes from `map[string]any` to `map[string]cty.Value`.
- `internal/prompt`: produces `map[string]cty.Value` instead of
  `map[string]any`. Type coercion that today happens scattered
  through the codebase moves into the prompt package.
- `internal/config`: `apiVersion` validator accepts `v2` only.
- `internal/migratecmd` (new): hosts `RunMigrate(opts) (*Result,
  error)` plus the rewrite engine.
- `cmd/migrate.go` (new): Cobra wiring for `forge migrate templates`.

## Data Model

`blueprint.yaml`/`registry.yaml` field shapes are unchanged. The
only on-disk diff is:

- `blueprint.yaml`: `apiVersion: v1` → `v2`; expression strings
  inside `variable.default`, `condition.when`, and `rename` use HCL
  syntax.
- Every `.tmpl` file: contents use HCL template syntax.

Internally, variables flowing through the renderer change type from
`map[string]any` to `map[string]cty.Value`.

## Testing Strategy

### Unit tests

- `internal/template/renderer_test.go` (rewrite): cover `RenderFile`,
  `RenderString`, `RenderPath`, `EvaluateBool` against fixture
  templates. Include a fixture with `${ }` substitution and verbatim
  `{{ }}` content to confirm the collision is gone.
- `internal/template/funcs_test.go` (rewrite): table-driven tests for
  every custom function (`snakeCase`, `camelCase`, …, `default`,
  `now`, `env`).
- `internal/migratecmd/migrate_test.go` (new): table-driven tests
  for every rewrite rule, plus negative tests for the out-of-scope
  patterns (assert they're left alone and surfaced in the summary).

### Integration tests

- `internal/create/cli_integration_test.go` (update): convert the
  existing `testdata/registry/` fixtures to v2 and re-run. Confirm
  the existing assertions hold. Add a new fixture
  `testdata/registry/helm/chart/` exercising verbatim `{{ }}`
  content in YAML.
- `internal/migratecmd/integration_test.go` (new): run
  `RunMigrate()` against a copy of the existing v1 fixtures (kept
  in `testdata/v1-registry/` for migration testing only) and assert
  the result loads cleanly under v2.

### Compatibility tests

- A single v1 blueprint kept under `testdata/v1-registry/` to
  exercise the load-time error path: assert that
  `config.LoadBlueprint()` returns an error mentioning the migration
  command.

## Migration / Rollout Plan

1. **Land DESIGN-0003 review.** Capture any further constraints.
2. **Land IMPL-0004** (separate doc, not blocking) breaking the work
   into phases — see "Implementation Phases" below.
3. **Cutover release.**
   - Pre-release candidates (`v0.x-rc.1`, etc.) ship behind a
     `--experimental-hcl2` flag for early adopters to test against
     their registries.
   - Final release removes the flag and makes HCL2 the only path.
   - Announcement explicitly tells maintainers to run `forge
     migrate templates` before upgrading.
4. **Post-release window.** Keep `forge migrate templates` in the
   binary indefinitely. It costs nothing to leave in and helps any
   maintainer who upgrades late.

### Suggested implementation phases (for IMPL-0004)

These are not part of this design's scope but inform the rollout
plan:

- Phase A — Engine swap. Implement the HCL2 renderer alongside the
  existing one behind a `forge create --experimental-hcl2` flag, so
  v1 blueprints still work during development.
- Phase B — Migration tool. Implement `forge migrate templates` and
  validate it against the testdata fixtures (a v1 copy and a
  hand-converted v2 reference).
- Phase C — Loader cutover. Make `LoadBlueprint()` reject `v1`,
  remove the `--experimental-hcl2` flag, delete the old renderer.
- Phase D — Docs cleanup. Update DESIGN-0001 to reflect HCL2 as the
  contract; archive the old text/template references.

## Resolved Questions

These were open during drafting and resolved during review. Captured
here so the trade-offs stay attached to the design.

- **`default` semantics → align with HCL.** Drop the v1 `default(val,
  fallback)` custom function in favour of HCL2's built-in
  `coalesce(val, fallback)` from `cty/stdlib.CoalesceFunc`. Argument
  order is the same. This removes one re-implementation from the
  function map (the `default` row in the table above is replaced by
  `coalesce`) and aligns forge's idiom with the broader HCL
  ecosystem.
- **Pipe-call translation in the migration tool → look at the corpus
  first.** The rewrite-rule table is the baseline. Before the tool
  ships, validate the rule set against
  [`github.com/donaldgifford/forge-registry`](https://github.com/donaldgifford/forge-registry)
  — the first registry we'll migrate. Patterns that show up in that
  corpus and are reliably regex-translatable get added to the rule
  set; patterns that need a real walker stay out-of-scope and the
  tool warns. The corpus survey gates the depth of the rule set,
  not the cutover.
- **Error message catalog → repo-local markdown.** The load-time
  error for a v1 blueprint points at `docs/MIGRATION.md` in the
  forge repository (readable on GitHub) rather than a hosted docs
  site. Defer the hosted/GitHub Pages site decision; the in-repo
  path is canonical until that lands. If MkDocs/TechDocs deploys
  later, the URL can be promoted without changing the load-time
  error string (the markdown file remains the source of truth).
- **Variable scoping in HCL conditions → same map as file content.**
  `condition.when` evaluates against the same `EvalContext` as
  `RenderFile`/`RenderString`/`RenderPath`. No scoping restriction.
  Documented in the Engine architecture section. We can revisit if
  a future need for derived/computed values forces a stratified
  context, but until then the simplest model wins.

## References

- [ADR-0001 — Use HCL2 as the Template Engine](../adr/0001-use-hcl2-as-the-template-engine.md)
- [INV-0001 — Templating YAML Files and HCL2 Migration](../investigation/0001-templating-yaml-files-and-hcl2-migration.md)
- [DESIGN-0001 — Blueprint Authoring](0001-blueprint-authoring.md)
  (current contract; superseded after this lands)
- [DESIGN-0002 — Registry Layout & Defaults Inheritance](0002-registry-layout-and-defaults-inheritance.md)
- [`hashicorp/hcl/v2`](https://github.com/hashicorp/hcl)
- [`zclconf/go-cty`](https://github.com/zclconf/go-cty)
- [HCL Template Syntax](https://github.com/hashicorp/hcl/blob/main/hclsyntax/spec.md#templates)
