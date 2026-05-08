---
id: ADR-0001
title: "Use HCL2 as the Template Engine"
status: Accepted
author: Donald Gifford
created: 2026-05-07
---
<!-- markdownlint-disable-file MD025 MD041 -->

# 0001. Use HCL2 as the Template Engine

<!--toc:start-->
- [Status](#status)
- [Context](#context)
- [Decision](#decision)
- [Consequences](#consequences)
  - [Positive](#positive)
  - [Negative](#negative)
  - [Neutral](#neutral)
- [Alternatives Considered](#alternatives-considered)
  - [Custom delimiters in text/template (rejected)](#custom-delimiters-in-texttemplate-rejected)
  - [Per-file passthrough opt-out (rejected)](#per-file-passthrough-opt-out-rejected)
  - [Sprig + custom delimiters (rejected)](#sprig--custom-delimiters-rejected)
  - [Pongo2 (Jinja2-style for Go, rejected)](#pongo2-jinja2-style-for-go-rejected)
  - [Defer (do nothing, rejected)](#defer-do-nothing-rejected)
- [References](#references)
<!--toc:end-->

## Status

Accepted

## Context

Forge currently uses Go `text/template` (with custom funcs in
`internal/template/funcs.go`) to render `.tmpl` files. INV-0001
investigated a class of failures where blueprints containing YAML
files with embedded `{{ }}` placeholders for downstream tools
(Helm, Argo Workflows, Kustomize, Mustache, GitHub Actions reusable
workflows, etc.) cannot be shipped as forge blueprints.

The investigation confirmed three findings:

1. The collision is real. `{{ .Values.replicas }}` in a `.tmpl` file
   crashes `forge create` at template-execute time with `nil pointer
   evaluating interface {}.replicas`. `missingkey=zero` only protects
   depth-1 access; chained field access still panics.
2. Both options on the table (custom delimiters in `text/template`,
   migration to HCL2) require rewriting every `.tmpl` file in every
   existing blueprint. There is no zero-migration fix.
3. The HCL2 prototype rendered the same Helm-laden YAML correctly with
   no escaping, using `${expr}` and `%{ if … ~}` template syntax that
   shares no tokens with `{{ }}`.

Once migration is forced either way, the question becomes: what
template engine do we want to be on five years from now? The two
options have different long-term properties:

| Property | Custom delimiters in `text/template` | HCL2 (`hashicorp/hcl/v2`) |
|----------|--------------------------------------|---------------------------|
| Solves Hypothesis 1 | Yes | Yes |
| Forces blueprint rewrite | Yes (`{{ }}` → `[[ ]]`) | Yes (`{{ }}` → `${ }`) |
| Type system | None — everything is `any` | `cty.Value` (`string`/`bool`/`number`) |
| Conditionals | `{{ if }} … {{ end }}` | `%{ if … ~}` directives |
| Error quality | Line/column inside the template | `hcl.Diagnostics` with source ranges |
| Familiarity | Go developers | Anyone who has used Terraform/Packer/Nomad/Vault |
| New deps | None | `hashicorp/hcl/v2` (MPL-2.0), `zclconf/go-cty` (MIT) |
| Engineering diff today | Small (one `Delims()` call) | Larger (renderer rewrite + func re-impl) |

The smaller engineering diff for custom delimiters is a sunk-cost
argument once we accept that migration is forced. The longer-term
properties favour HCL2.

## Decision

**Migrate the forge template engine from Go `text/template` to HCL2
(`hashicorp/hcl/v2`).**

The migration is a **breaking change** with a one-shot rewrite, not a
parallel-format support window:

- Bump `apiVersion: v1` → `apiVersion: v2` in `blueprint.yaml`. v1
  blueprints fail to load with a clear error pointing to the
  migration tool.
- Ship `forge migrate templates` to rewrite every `.tmpl` file and
  every `blueprint.yaml` field that contains template syntax
  (`variable.default`, `condition.when`, `rename` patterns).
- Add `hashicorp/hcl/v2` and `zclconf/go-cty` as direct deps.
- Re-implement the custom function map (`snakeCase`, `camelCase`,
  `pascalCase`, `kebabCase`, `now`, `env`) as `function.Function`
  definitions exposed via `EvalContext.Functions`. Drop the v1
  `default(val, fallback)` custom function in favour of HCL2's
  built-in `coalesce(val, fallback)` from `cty/stdlib`.
- Replace `internal/template/renderer.go` with an HCL2-backed renderer
  while keeping the same `Renderer` API surface from the orchestrator's
  point of view (`RenderFile`, `RenderString`, `RenderPath`).

DESIGN-0003 specifies the cutover details. An IMPL doc will follow
DESIGN-0003 review and break the work into phases.

## Consequences

### Positive

- **Helm/Argo/Kustomize/GitHub Actions YAML can be shipped as
  blueprints.** The single biggest authoring restriction goes away.
- **Typed variables.** `cty.Value` carries a real type. We drop the
  `missingkey=zero`/`missingkey=error` split and the ad-hoc string-vs-bool
  coercion in the prompt package.
- **Better diagnostics.** HCL2's diagnostic system points at the
  source range with column-level precision, instead of
  `text/template`'s opaque "executing template at <.x>" messages.
- **Familiar syntax.** Anyone who has touched Terraform, Packer,
  Nomad, or Vault recognises `${var}` and `%{ if … ~}`. This widens
  the audience that can author forge blueprints without learning a
  new template language.
- **Future-proofing.** `${ }` and `%{ … }` share no delimiters with
  any other template ecosystem we care about. We will not have to
  revisit the collision question for the next downstream tool.
- **Versioned schema upgrade path.** The `apiVersion` bump establishes
  the precedent for schema evolution in `blueprint.yaml` /
  `registry.yaml`.

### Negative

- **All existing blueprints must be migrated.** External registries
  pinning to forge's templating contract are forced to re-publish.
  We mitigate with `forge migrate templates`, but the social cost
  (announcement, migration window, support burden) is real.
- **`forge migrate templates` is itself non-trivial.** Translating
  `{{ if .x }} … {{ end }}` to `%{ if x ~} … %{ endif ~}` is more
  than a regex sed; some constructs (custom func calls,
  `range`/`with`, deep nesting) need a real walker.
- **Two new dependencies.** Both are licence-allowlist-compatible
  and stable, but the supply-chain surface grows. MPL-2.0 (HCL) is
  already accepted; no new licence review needed.
- **Loss of `text/template` knowledge.** Contributors familiar with
  Go's stdlib templating but not HCL2 will need to ramp up.
- **Path templating semantics change.** `{{project_name}}/cmd/main.go`
  becomes `${project_name}/cmd/main.go`. The "shorthand `{{var}}` →
  `{{.var}}`" path-template normaliser in `renderer.go:79` goes away;
  HCL2 expressions don't need the dot prefix.

### Neutral

- **Function map size stays roughly the same.** Re-implementing
  custom funcs as `function.Function` is comparable LoC to the
  current `funcs.go`. Some functions (`upper`, `lower`, `replace`,
  `trimPrefix`, `trimSuffix`, `regexp`-style) come for free from
  `cty/function/stdlib`.
- **`blueprint.yaml` itself stays YAML.** The schema fields are
  unchanged; only the *expression* fields inside (`variable.default`,
  `condition.when`, etc.) move from Go-template syntax to HCL
  expression syntax.

## Alternatives Considered

### Custom delimiters in `text/template` (rejected)

Set `template.Delims("[[", "]]")` so forge's substitutions don't
collide with downstream `{{ }}`. This is the smallest possible
engine change and keeps us on stdlib `text/template`.

Rejected because:

- It still requires every existing blueprint to be rewritten
  (`{{ x }}` → `[[ x ]]`). Migration cost is comparable to HCL2.
- The destination is worse: no type system, no diagnostic upgrade,
  unfamiliar delimiters (`[[ ]]` is not a standard anywhere).
- It treats the symptom (collision) instead of the underlying
  weakness (Go `text/template` is a low-level engine for a
  general-purpose scaffolding tool).
- It defers the HCL2 question without resolving it. We would likely
  revisit the engine choice within a few releases anyway.

### Per-file `passthrough` opt-out (rejected)

Add a `sync.passthrough: ["**/*.helm.tmpl"]` glob to `blueprint.yaml`
that skips template rendering for matched files.

Rejected because:

- Authors would lose access to forge variables in those files.
  Helm chart blueprints often want to inject `${project_name}` into
  `Chart.yaml` AND keep `{{ .Values.x }}` literal — this option
  forces an all-or-nothing choice per file.
- It's a workaround, not a fix. The underlying collision remains
  for any author who doesn't know to add the opt-out.

### Sprig + custom delimiters (rejected)

Add `Masterminds/sprig` for richer funcs while changing delimiters.

Rejected because:

- Sprig itself uses `{{ }}` (it's a `text/template` function
  library), so it would inherit the same collision problem if
  delimiters stayed default.
- With delimiters changed, Sprig becomes "Helm funcs without Helm
  syntax", which is confusing in its own right.
- The function-map gap between forge today and HCL2's `cty/stdlib`
  is small; we don't need Sprig's surface area.

### Pongo2 (Jinja2-style for Go, rejected)

`flosch/pongo2` provides Jinja2 syntax (`{% if … %}`, `{{ var }}`)
for Go.

Rejected because:

- Same `{{ }}` collision as `text/template`.
- Adds a dep without solving the core problem.
- Unfamiliar to most Go developers and not part of any major
  infrastructure ecosystem.

### Defer (do nothing, rejected)

Document the limitation, recommend authors avoid `{{ }}` in YAML
blueprints, and revisit later.

Rejected because:

- The limitation is silently learned: authors hit a runtime panic,
  not a load-time validation error.
- Every Helm/Argo/Kustomize/GitHub-Actions-flavoured blueprint we
  *don't* ship is a missed authoring use case.
- Deferring locks more blueprints into the current contract,
  making the eventual migration harder.

## References

- [INV-0001 — Templating YAML Files and HCL2 Migration](../investigation/0001-templating-yaml-files-and-hcl2-migration.md)
  (the investigation that drove this decision)
- [DESIGN-0003 — Migrate Template Engine to HCL2](../design/0003-migrate-template-engine-to-hcl2.md)
  (the cutover design)
- [DESIGN-0001 — Blueprint Authoring](../design/0001-blueprint-authoring.md)
  (current contract; will be superseded by DESIGN-0003)
- [`hashicorp/hcl/v2`](https://github.com/hashicorp/hcl) — HCL2 library
- [`zclconf/go-cty`](https://github.com/zclconf/go-cty) — value/type
  system used by HCL2
- [`cty/function/stdlib`](https://pkg.go.dev/github.com/zclconf/go-cty/cty/function/stdlib)
  — built-in function library
