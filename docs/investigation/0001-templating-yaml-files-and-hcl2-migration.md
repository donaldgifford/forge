---
id: INV-0001
title: "Templating YAML Files and HCL2 Migration"
status: Concluded
author: Donald Gifford
created: 2026-05-07
---

<!-- markdownlint-disable-file MD025 MD041 -->

# INV 0001: Templating YAML Files and HCL2 Migration

**Status:** Concluded **Author:** Donald Gifford **Date:** 2026-05-07

<!--toc:start-->
- [Question](#question)
- [Hypothesis](#hypothesis)
- [Context](#context)
- [Approach](#approach)
  - [Reproduce hypothesis 1 (delimiter collision)](#reproduce-hypothesis-1-delimiter-collision)
  - [Reproduce hypothesis 2 (blueprint.yaml not rendered)](#reproduce-hypothesis-2-blueprintyaml-not-rendered)
  - [Reproduce hypothesis 3 (extension naming)](#reproduce-hypothesis-3-extension-naming)
  - [Map possible mitigations within text/template](#map-possible-mitigations-within-texttemplate)
  - [Evaluate HCL2 migration](#evaluate-hcl2-migration)
  - [Compare alternatives](#compare-alternatives)
- [Environment](#environment)
- [Findings](#findings)
  - [Observation 1 — actual failure mode for .yaml.tmpl](#observation-1--actual-failure-mode-for-yamltmpl)
  - [Observation 2 — blueprint.yaml template behavior today](#observation-2--blueprintyaml-template-behavior-today)
  - [Observation 3 — extension-naming behavior](#observation-3--extension-naming-behavior)
  - [Observation 4 — HCL2 prototype results](#observation-4--hcl2-prototype-results)
- [Conclusion](#conclusion)
- [Recommendation](#recommendation)
  - [Follow-ups (separate tickets, not blocking)](#follow-ups-separate-tickets-not-blocking)
- [References](#references)
<!--toc:end-->

## Question

Two related questions:

1. **Why does forge's templating engine break when we try to render YAML files
   (`.yaml.tmpl` / `.yml.tmpl`)?** Is the failure caused by the blueprint being
   a YAML file, by the contents of the rendered YAML file colliding with Go
   `text/template` syntax, or by something else in the parse/render pipeline?
2. **Would migrating from Go `text/template` to HCL2 (`hashicorp/hcl/v2`) solve
   the problem?** What does the migration cost — both for forge internals and
   for blueprint authors who would need to rewrite templates?

## Hypothesis

There are three plausible failure modes; the investigation needs to confirm
which one(s) actually fire:

1. **Delimiter collision in YAML template content.** Many YAML files we would
   want to ship as blueprints contain `{{ }}`-style placeholders meant for
   _another_ template engine (Helm charts, Argo workflow templates, Kustomize
   replacements, GitHub Actions reusable workflows). When forge's renderer
   parses these as Go templates, it tries to evaluate `{{ .Values.replicas }}`
   etc. against forge's variable map and fails with
   `template: ... map has no entry for key "Values"` or similar. This is the
   most likely culprit.
2. **`blueprint.yaml` is not itself rendered.** A blueprint's `blueprint.yaml`
   is parsed by `gopkg.in/yaml.v3` directly — it never passes through the
   template renderer. If a user expects to put `{{ ... }}` _inside_
   `blueprint.yaml` and have it expanded before parsing, that expectation is
   wrong today. (See `default` field of variables, which _is_ rendered later,
   but that's the only field that gets template treatment.)
3. **Path/extension stripping.** Forge strips a single trailing `.tmpl`
   extension. A file named `ci.yaml.tmpl` becomes `ci.yaml`, which is correct.
   But authors sometimes name files `ci.tmpl.yaml` expecting the same behavior —
   `IsTemplate()` only checks for `.tmpl` as a _suffix_, so `ci.tmpl.yaml` is
   treated as static and _not_ rendered. This is naming-convention-driven, not a
   parsing failure, but easy to misdiagnose as "templating broke".

The HCL2 question turns largely on hypothesis 1 above. HCL2 uses `${var}`
interpolation and `%{ if }` directives — these don't collide with the
Helm/Argo/Kustomize ecosystem. HCL2 _would_ solve hypothesis 1; it would
not change the `blueprint.yaml`-isn't-rendered situation (hypothesis 2)
and is orthogonal to extension naming (hypothesis 3).

## Context

The forge templating engine lives in `internal/template/renderer.go:107` and
uses Go `text/template` with `missingkey=zero` for file rendering and
`missingkey=error` for inline strings. Custom funcs (`snakeCase`, `camelCase`,
etc.) live in `internal/template/funcs.go`.

The render flow for a blueprint create is:

- `internal/create/create.go:228` — `renderFiles()` walks the resolved
  `FileSet`.
- `internal/create/create.go:270` — for each file with `IsTemplate == true`
  (`.tmpl` suffix), call `renderer.RenderFile()`.
- Files without `.tmpl` are copied verbatim — no template parsing happens to
  them at all.

**Triggered by:** Real-world blueprint authoring revealed that we can't ship
blueprints containing YAML files with embedded Helm-style `{{ }}` syntax. The
Go-1.26.2 dependency refresh made it a reasonable moment to revisit the
templating engine choice before more blueprints lock us in.

## Approach

### Reproduce hypothesis 1 (delimiter collision)

1. Add a YAML template at
   `testdata/registry/go/api/{{project_name}}/values.yaml.tmpl` whose content
   contains a Helm-style placeholder:

   ```yaml
   replicas: { { .Values.replicas | default 1 } }
   image: nginx
   ```

2. Run
   `forge create go/api --registry-dir ./testdata/registry --defaults --set project_name=t -o /tmp/inv-yaml --force`
   and capture the exact error output.
3. Confirm whether the error originates in `template.Parse` (parse error) or
   `template.Execute` (missing-key error). Distinguish the two — they require
   different fixes.

### Reproduce hypothesis 2 (`blueprint.yaml` not rendered)

1. Place a `{{ "{{" }} .org {{ "}}" }}` reference in a `blueprint.yaml` field
   outside `variable.default` (e.g., in `description` or `tags`). Run
   `forge create` and confirm the literal `{{ ... }}` text shows up unexpanded
   in `registry.yaml`/lockfile output.
2. Trace the load path:
   - `internal/config/loader.go` → `LoadBlueprint()`.
   - Which fields, if any, are template-rendered after load? (Today:
     `variable.default` and `condition.when` — see `internal/prompt/prompt.go`
     and `internal/create/conditions.go`.)
3. Decide whether expanding template support in `blueprint.yaml` is in scope or
   a separate question.

### Reproduce hypothesis 3 (extension naming)

1. Create `foo.tmpl.yaml` and `foo.yaml.tmpl` side-by-side in a blueprint
   directory.
2. Run `forge create` and check output: only the `.tmpl`-suffixed file should be
   rendered. Confirm and document.
3. If both naming conventions matter, decide whether
   `internal/template/renderer.go:99` should match `.tmpl` anywhere in the chain
   or only as a final suffix.

### Map possible mitigations within `text/template`

For each mitigation, describe the API change cost and the compatibility cost:

| Mitigation                      | Idea                                                                                       | Cost                                                               | Notes                                                                        |
| ------------------------------- | ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------ | ---------------------------------------------------------------------------- |
| Custom delimiters               | Use `{{=` `=}}` or `[[` `]]` per `template.Delims()` so `{{ }}` is literal in YAML output  | Low engine; medium for users (existing `{{ }}` blueprints rewrite) | Configurable per-blueprint via `blueprint.yaml`?                             |
| Per-file opt-out                | A `passthrough: ["**/*.helm.tmpl"]` glob in `blueprint.yaml` that skips template rendering | Low                                                                | Works for Helm/Argo files but loses access to forge variables in those files |
| Raw blocks                      | Document the `{{` "`{{`" `}}` literal escape pattern; ship a snippet in DESIGN-0001        | Zero                                                               | Painful to read, error-prone                                                 |
| Twin-pass / fenced blocks       | Pre-process `<%raw%>` ... `<%/raw%>` markers before rendering                              | Medium                                                             | Custom syntax forge users would have to learn                                |
| Switch to Sprig + custom delims | Add `Masterminds/sprig` for richer funcs while changing delims                             | Medium                                                             | Sprig is widely used in Helm; familiar                                       |

### Evaluate HCL2 migration

1. Build a tiny prototype in a scratch dir that:
   - Reads a `blueprint.hcl` file with `variable` blocks and string attributes.
   - Renders an HCL file with `${project_name}` and
     `%{ if use_grpc }...%{ endif }` content using
     `github.com/hashicorp/hcl/v2/hclsyntax` + `cty.Value`.
   - Compares behavior against the equivalent `text/template` blueprint.
2. Inventory features forge currently uses and confirm HCL2 parity:
   - Variable types (`string`, `bool`, `int`, `choice`) → HCL `cty` types
     (`string`, `bool`, `number`, plus enums via validation).
   - Custom funcs (`snakeCase`, `camelCase`, …) → HCL function map
     (`function.New`).
   - `default` field rendering for variables → HCL `default` attribute.
   - `condition.when` template → HCL expression evaluated to bool.
   - File path templating (`{{project_name}}/cmd/main.go`) → HCL string
     interpolation in path strings.
3. Estimate migration footprint:
   - Files in `internal/template/` (~280 LoC) replaced by HCL2 bindings.
   - All blueprint authors must rewrite `{{ .x }}` → `${x}`.
   - `blueprint.yaml` either stays YAML (HCL only renders templated content) or
     migrates to `blueprint.hcl` (bigger change).
4. Run `forge create` against a small HCL-rendered blueprint and measure
   cold-start cost; HCL2 parser is heavier than `text/template`.

### Compare alternatives

Consider third options that may dominate HCL2 for our use case:

| Option                                                       | Pros                                                                                       | Cons                                             |
| ------------------------------------------------------------ | ------------------------------------------------------------------------------------------ | ------------------------------------------------ |
| `text/template` + custom delimiters (`<%` `%>` or `[[` `]]`) | Tiny diff, keeps stdlib                                                                    | Users still write Go-template syntax             |
| `text/template` + Sprig                                      | Richer funcs, no migration of existing blueprints                                          | Same `{{ }}` collision unless delims also change |
| Pongo2 (Django/Jinja2 syntax)                                | Familiar to Python/Ansible users                                                           | Adds dep; new syntax for users                   |
| `hashicorp/hcl/v2`                                           | Distinct delimiter syntax; type-checked vars                                               | Big migration; HCL is unfamiliar to many         |
| Two-engine hybrid                                            | Use HCL for `blueprint.hcl` (config) + keep `text/template` for content with custom delims | Most flexible; most complexity                   |

## Environment

| Component          | Version / Value                                                                                                                                |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| Go                 | 1.26.2                                                                                                                                         |
| `gopkg.in/yaml.v3` | latest pinned in `go.mod`                                                                                                                      |
| Renderer           | `text/template` with `missingkey=zero` for files                                                                                               |
| Inline renderer    | `text/template` with `missingkey=error`                                                                                                        |
| Custom funcs       | `snakeCase`, `camelCase`, `pascalCase`, `kebabCase`, `upper`, `lower`, `title`, `replace`, `trimPrefix`, `trimSuffix`, `now`, `env`, `default` |

## Findings

All reproductions were run against forge built from
`feat/template-improvements` (`v0.2.0-4-g68d6f12-dirty`) on Go 1.26.2.
Hermetic registry fixtures lived under `/tmp/forge-inv-h{1,3}/` so
`testdata/` stayed clean.

### Observation 1 — actual failure mode for `.yaml.tmpl`

**Hypothesis 1 confirmed**, with a sharper diagnosis than the
hypothesis stated.

Fixture (`/tmp/forge-inv-h1/registry/helm/chart/{{project_name}}/values.yaml.tmpl`):

```yaml
project: {{ .project_name }}
replicas: {{ .Values.replicas | default 1 }}
image: {{ .Values.image.repository }}:{{ .Values.image.tag }}
```

Running `forge create helm/chart --registry-dir … --set
project_name=demo …` produced:

```
Error: writing file {{project_name}}/values.yaml.tmpl: rendering template
.../values.yaml.tmpl: executing template "values.yaml.tmpl":
template: values.yaml.tmpl:4:20: executing "values.yaml.tmpl"
at <.Values.replicas>: nil pointer evaluating interface {}.replicas
```

Key points:

- The error is at **`template.Execute`**, not `template.Parse`. The
  Helm-style content parses cleanly because `{{ }}` is forge's own
  delimiter and `default` happens to exist as a custom function.
- `missingkey=zero` (set in
  `internal/template/renderer.go:106-110`) is a **leaky abstraction
  for nested keys**. A bare `{{ .Values }}` resolves to `<no value>`
  and succeeds; a *chained* access like `{{ .Values.replicas }}`
  panics because `.Values` is `nil` and you cannot index nil. The
  follow-up probe replacing the file with just `{{ .Values }}`
  rendered to `top_level_missing: <no value>` and exit 0 — confirming
  `missingkey=zero` masks the failure only at depth 1.
- The "raw block" mitigation works but is awful to read. Replacing
  the file with `{{ "{{" }} .Values.replicas | default 1 {{ "}}" }}`
  succeeded and produced the literal `{{ .Values.replicas | default 1
  }}` in the output, but every Helm token now requires four `{{ "{{"
  }}` / `{{ "}}" }}` escapes.
- The **custom-delimiter** mitigation works cleanly. A standalone Go
  probe (`text/template` + `Delims("[[", "]]")`) rendered:

  ```
  project: [[ .project_name ]]
  replicas: {{ .Values.replicas | default 1 }}
  ```

  to:

  ```
  project: demo
  replicas: {{ .Values.replicas | default 1 }}
  ```

  with `missingkey=error` and no panic. `{{ }}` content is invisible
  to the parser when delimiters are `[[` `]]`.

### Observation 2 — `blueprint.yaml` template behavior today

**Hypothesis 2 confirmed.** Code-grep + live test:

Render call sites (excluding tests) found via
`grep -rn "Render(String|File|Path)" internal/`:

- `internal/create/create.go:252` — `RenderPath(entry.RelPath, …)`
- `internal/create/create.go:272` — `RenderFile(entry.AbsPath, …)`
  (only when `entry.IsTemplate`)
- `internal/create/create.go:299-307` — rename pattern + replacement
- `internal/create/conditions.go:38` — `condition.when`
- `internal/sync/engine.go:267` — file content during sync
- `internal/check/check.go:220` — file content during check
- `internal/prompt/prompt.go:138-152` — `variable.default` (parses its
  own `text/template` directly, not via the renderer)

So **rendered fields**: file content (when `.tmpl`), file/directory
paths, rename patterns, `condition.when`, `variable.default`. **Not
rendered**: `apiVersion`, `name`, `description`, `tags`, `version`,
and every other top-level `blueprint.yaml` field.

Live verification: a fixture with
`description: "Project for {{ .project_name }} — should remain
literal"` and a `tag-{{ .project_name }}` tag survived `forge create`
without error and `forge info /tmp/.../blueprint.yaml` showed:

```
Name:         helm-chart
Version:      0.1.0
Description:  Project for {{ .project_name }} — should remain literal
Tags:         tag-{{ .project_name }}, helm
```

The `{{ }}` is stored as a literal string and never expanded. Whether
this is a bug or a feature is a design call (the lockfile and
registry index would otherwise depend on the user's variable
choices), but the current behavior is **silent** — there is no
warning that the placeholder didn't expand.

### Observation 3 — extension-naming behavior

**Hypothesis 3 confirmed.** Fixture with two files in the same
blueprint:

- `foo.yaml.tmpl` containing `name: {{ .project_name }}` →
  rendered to `foo.yaml` with content `name: demo`.
- `bar.tmpl.yaml` containing `name: {{ .project_name }}` → copied
  verbatim as `bar.tmpl.yaml` with content unchanged.

Source: `internal/template/renderer.go:99` —
`func IsTemplate(path string) bool { return
strings.HasSuffix(path, ".tmpl") }`. Only a *terminal* `.tmpl` is
recognised. `bar.tmpl.yaml` looks like a template to humans but is
silently treated as static. No warning, no error.

### Observation 4 — HCL2 prototype results

Prototype at `/tmp/forge-inv-hcl/main.go` parses the same
Helm-laden YAML rewritten to HCL2 template syntax:

```
project: ${project_name}
replicas: {{ .Values.replicas | default 1 }}
image: {{ .Values.image.repository }}:{{ .Values.image.tag }}
upper: ${upper(project_name)}
%{ if use_grpc ~}
grpc: enabled
%{ else ~}
grpc: disabled
%{ endif ~}
```

with `EvalContext{Variables: {project_name: "demo", use_grpc:
true}, Functions: {upper, lower}}` and produced:

```
project: demo
replicas: {{ .Values.replicas | default 1 }}
image: {{ .Values.image.repository }}:{{ .Values.image.tag }}
upper: DEMO
grpc: enabled
```

Findings:

- **Delimiter collision is structurally impossible.** HCL2's
  template syntax (`${expr}`, `%{ if … ~}`) shares no delimiters
  with Helm/Argo/Kustomize/Mustache `{{ }}` ecosystems. The Helm
  content survives byte-for-byte.
- **Function map parity** with our current custom funcs is achievable
  but not free. `cty/function/stdlib` provides upper/lower, regex,
  format, join, split, substr, length, equal/notequal/lt/lte, sort —
  but **none** of `snakeCase`, `camelCase`, `pascalCase`,
  `kebabCase`, `now`, `env`, `default`. We would re-implement these
  as `function.Function` definitions — comparable LoC to the
  existing `internal/template/funcs.go`.
- **Type system upgrade.** `cty.Value` carries an explicit type. The
  HCL2 path naturally enforces `string`/`bool`/`number` without our
  current "everything is `any`" coercion. The trade-off is that
  variables must be declared with types up front (we already do this
  via `blueprint.yaml`, so the cost is marginal).
- **Cost surface.** Adding `hashicorp/hcl/v2` would pull
  `zclconf/go-cty` and `golang.org/x/...` deps that weren't in the
  module before. License: MPL-2.0 (HCL) and MIT (cty) — both already
  on the forge license allowlist (MPL-2.0 is allowed for `go-getter`).
  No checksum-validation change needed.
- **Migration size.** Every existing blueprint with `{{ }}` content
  (registry-wide) would need rewriting to `${ }`. That is a hard
  break for any external blueprint registries already pinning to
  forge's templating contract. An IMPL doc would need to specify a
  parallel-format support window or a one-shot migration tool.

## Conclusion

**Answer:** Concluded.

- **Hypothesis 1 is the dominant breakage.** Blueprints that ship YAML
  with embedded Helm/Argo/Kustomize `{{ }}` content fail at
  `template.Execute` time the moment the placeholder uses chained
  field access (one level is enough — `.Values.replicas` is the
  smallest reproducer). `missingkey=zero` only protects depth-1
  access; it does *not* save us.
- **Hypothesis 2 is real but secondary.** Most `blueprint.yaml`
  fields are stored verbatim and are not template-rendered.
  `description`, `tags`, `name`, `version` all silently keep `{{ }}`
  in them. This is a separate user-experience bug (no warning) but
  doesn't drive the engine choice.
- **Hypothesis 3 is real but minor.** Only a terminal `.tmpl` triggers
  rendering. `foo.tmpl.yaml` is silently copied. Easy doc/lint fix;
  doesn't drive the engine choice either.
- **HCL2 fully solves Hypothesis 1.** `${expr}` / `%{ … }` syntax
  shares no tokens with the `{{ }}` ecosystem. The prototype
  rendered Helm-laden YAML correctly with no escaping at all.
  Function-map parity is achievable but requires re-implementing the
  case-conversion funcs (`snakeCase`, `camelCase`, …).
- **Custom delimiters in `text/template` also solve Hypothesis 1**, at
  a far smaller engineering cost than HCL2 — one `template.Delims()`
  call away. But this fix doesn't *converge* templating; it just
  swaps forge's delimiter (`[[ ]]`) so it doesn't collide with the
  downstream tool's (`{{ }}`). Authors still maintain two template
  languages in a single blueprint.

## Recommendation

**Converge on HCL2.** Treat this as a one-shot breaking change to the
template engine and migrate every `.tmpl` file in the same release.
Open ADR-0001 to capture the decision and DESIGN-0003 to specify the
cutover.

Reasoning, in priority order:

1. **Migration is forced either way.** Both the custom-delim option
   (`{{ .x }}` → `[[ .x ]]`) and the HCL2 option (`{{ .x }}` →
   `${x}`) require rewriting every `.tmpl` file forge has ever
   shipped. Once we accept that cost, "smaller engine diff today" is
   a sunk-cost argument and not a long-term one. We should pick the
   destination we'd want to be at in five years, not the one closest
   to where we are today.

2. **HCL2 is the better destination.** Specifically:
   - **Typed values via `cty`.** `string`/`bool`/`number` are first
     class. We can drop our hand-rolled type coercion and the
     `missingkey=zero`/`missingkey=error` split.
   - **Native conditionals and iteration.** `%{ if … ~}` and `%{ for
     … ~}` are explicit directives, not awkward `{{ if }} … {{ end
     }}` chains hidden in YAML.
   - **Diagnostic-quality errors.** HCL2's `hcl.Diagnostics`
     produces source-pointing errors with line/column ranges. Today
     a missing variable surfaces as `template: foo.tmpl:4:20:
     executing "foo.tmpl" at <.x>: …`; HCL2 surfaces it with the
     exact source range and a suggestion.
   - **Familiarity.** Anyone who has touched Terraform, Packer,
     Nomad, or Vault has used HCL2. Go `text/template` is mostly
     familiar to Go developers; the audience for forge is broader
     than that.
   - **Distinct enough that collisions are structurally
     impossible.** `${ }` and `%{ … }` share zero tokens with
     Helm/Argo/Kustomize/Mustache `{{ }}`. We will not have to
     revisit this question for the next ecosystem to use `{{ }}`.

3. **The cost is bounded and acceptable.**
   - Two new direct deps: `hashicorp/hcl/v2` (MPL-2.0, already
     allow-listed for `go-getter`) and `zclconf/go-cty` (MIT). No
     license review needed.
   - Custom funcs (`snakeCase`, `camelCase`, `pascalCase`,
     `kebabCase`, `now`, `env`, `default`) re-implemented as
     `function.Function` definitions — comparable LoC to today's
     `internal/template/funcs.go`.
   - Conditions (`condition.when`) and variable defaults
     (`variable.default`) move to HCL expressions evaluated against
     `cty.Value`. The schema field shape doesn't have to change in
     `blueprint.yaml`.

4. **Choose a one-shot rewrite over parallel-format support.** Don't
   ship a "v1 and v2 both supported" window. Bump
   `apiVersion: v2` in `blueprint.yaml`, refuse to load v1
   blueprints, and ship a `forge migrate templates` helper that
   rewrites `{{ .x }}` → `${x}`, `{{ if … }}` → `%{ if … ~}`, etc.,
   plus updates `apiVersion`. Rationale:
   - The forge user base is small enough today that a clean break
     is feasible.
   - Parallel support means *every* call site (`renderer`,
     `conditions`, `prompt.renderDefault`, `sync`, `check`) carries
     two engines forever. The maintenance tax of dual-rendering far
     outweighs the cutover pain.
   - A one-shot migration tool is a small, well-scoped artefact;
     dual-engine code paths are not.

DESIGN-0003 specifies the cutover details (apiVersion bump, which
fields move, what `forge migrate templates` does, how registries
declare HCL2 compliance). ADR-0001 captures the decision so the
trade-off is durable.

### Follow-ups (separate tickets, not blocking)

- Update DESIGN-0001 once DESIGN-0003 lands — the new contract
  replaces the current `text/template` description.
- Fix Hypothesis 2 (literal `{{ }}` in `description`/`tags`)
  becomes "literal `${ }` in `description`/`tags`" under HCL2.
  Either render those fields against resolved variables or
  validate-and-error at load time. Pick one; document it.
- Fix Hypothesis 3 (extension naming) in passing during the cutover
  — settle whether `.tmpl.yaml` should also render, or warn at
  load.

## References

- [ADR-0001 — Use HCL2 as the Template Engine](../adr/0001-use-hcl2-as-the-template-engine.md)
- [DESIGN-0003 — Migrate Template Engine to HCL2](../design/0003-migrate-template-engine-to-hcl2.md)
- [DESIGN-0001 — Blueprint Authoring](../design/0001-blueprint-authoring.md)
  (template syntax contract — superseded by DESIGN-0003 once HCL2 lands)
- [DESIGN-0002 — Registry Layout & Defaults Inheritance](../design/0002-registry-layout-and-defaults-inheritance.md)
- [RFC-0001 — Forge: Project Scaffolding CLI](../rfc/0001-forge-project-scaffolding-cli.md)
- `internal/template/renderer.go` — current rendering implementation
- `internal/template/funcs.go` — custom function map
- `internal/create/create.go:228` — `renderFiles()` orchestrator
- [Go `text/template`](https://pkg.go.dev/text/template) — `template.Delims()`,
  `Option("missingkey=...")`
- [`hashicorp/hcl/v2`](https://github.com/hashicorp/hcl) — HCL2 library
- [`Masterminds/sprig`](https://github.com/Masterminds/sprig) — Helm function
  library (alternative to writing our own funcs)
- [`flosch/pongo2`](https://github.com/flosch/pongo2) — Jinja2-like syntax for
  Go (alternative engine)
