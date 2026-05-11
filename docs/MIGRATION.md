# Migrating forge blueprints from v1 to v2 (HCL2)

This guide walks blueprint authors through upgrading a registry from
`apiVersion: v1` (Go `text/template`) to `apiVersion: v2` (HCL2).
After upgrading forge to a release containing this change, v1
blueprints **will not load**. Run the migration tool against your
registry, review the result, commit, and you're done.

## Why the change

Forge v1 used Go's `text/template` engine, whose `{{ }}` delimiters
collide with downstream tools that *also* use `{{ }}` — Helm, Argo
CD ApplicationSets, kustomize replacements, the Hashicorp config
DSLs. Authoring a blueprint that produces a Helm chart meant escaping
every line with `{{ "{{" }} .Values.replicas {{ "}}" }}`, which is
unreadable and error-prone.

HCL2 uses `${expr}` for interpolation and `%{ if … ~}` for
directives. `{{ }}` becomes ordinary literal text, so a blueprint
that scaffolds a Helm chart can carry verbatim Helm template
content.

See [ADR-0001](adr/0001-use-hcl2-as-the-template-engine.md) for the
decision record and [DESIGN-0003](design/0003-migrate-template-engine-to-hcl2.md)
for the engine-swap design.

## The migration command

```sh
forge migrate templates --path <registry-or-blueprint>
```

`--path` accepts either a registry root (rewrites every blueprint
under it, plus registry-wide and category-level `_defaults/`) or a
single blueprint directory.

### Useful flags

| Flag | Behaviour |
|------|-----------|
| `--dry-run` | Print the rewrite plan without modifying any files. |
| `--strict` | Exit non-zero if any file contains an untranslatable construct. |
| `--force` | Skip the dirty-worktree guard. The tool refuses to run on a registry with uncommitted changes by default — commit or stash first. |

### Example: registry-wide migration

```sh
cd path/to/your-registry
git status                                       # working tree must be clean
forge migrate templates --path . --dry-run       # preview the changes
forge migrate templates --path .                 # apply
git diff                                         # review
git commit -am "migrate: blueprints to HCL2 (apiVersion v2)"
```

The tool bumps `apiVersion: v1 → v2` in `blueprint.yaml` and
`registry.yaml`, rewrites all `*.tmpl` files, and renames any
`{{varname}}` directories to `${varname}`.

### Example: single blueprint

```sh
forge migrate templates --path go/api
```

Useful when you want to migrate one blueprint at a time during
review.

## Rewrite rules

The tool applies a deterministic set of rewrites. Anything outside
the rule set is left in place and surfaced in the summary so you
can fix it by hand.

| v1 syntax | v2 syntax | Notes |
|-----------|-----------|-------|
| `{{ .name }}` / `{{ name }}` | `${name}` | Bare or dotted identifier — leading dot is trimmed. |
| `{{ .a.b }}` | `${a.b}` | Nested attribute access translates verbatim. |
| `{{ funcname .a }}` | `${funcname(a)}` | Single-arg function call. |
| `{{ .a \| funcname }}` | `${funcname(a)}` | Pipe call → positional call. |
| `{{ .a \| funcname "arg" }}` | `${funcname(a, "arg")}` | Pipe with extra arg → reordered positional. |
| `{{ if .x }}` … `{{ end }}` | `%{ if x ~}` … `%{ endif ~}` | Conditional. |
| `{{ if .x }}` … `{{ else }}` … `{{ end }}` | `%{ if x ~}` … `%{ else ~}` … `%{ endif ~}` | If/else. |
| `{{ eq .x "y" }}` (in `when:`) | `x == "y"` | `condition.when` is evaluated as an HCL bool expression — not as a template. |
| `{{ ne .x "y" }}` | `x != "y"` | as above. |
| `{{ not .x }}` | `!x` | as above. |
| `{{ "{{" }}` | `{{` | Literal-emit-`{{` escape becomes plain literal text — this is the whole reason for the swap. |
| `{{ "}}" }}` | `}}` | as above. |
| `{{project_name}}` (in path) | `${project_name}` | Path shorthand removed — HCL doesn't need it. |
| `{{ default .x "fallback" }}` / `{{ .x \| default "fallback" }}` | `${coalesce(x, "fallback")}` | v1 `default` becomes HCL `coalesce`. Argument order is unchanged. |

The custom function map (`snakeCase`, `camelCase`, `pascalCase`,
`kebabCase`, `now`, `env`, `upper`, `lower`, `title`, `replace`,
`trimPrefix`, `trimSuffix`) is preserved by name.

## Manual fixes the tool does not attempt

The migration tool warns about the following constructs and leaves
them alone — they need a human translation.

### `{{ range … }}` blocks

v1 templates rarely use these, but when they do, the iteration shape
is closer to HCL's `%{ for … }` than a regex sed can handle.

```text
v1:
  {{- range .modules }}
  - {{ . }}
  {{- end }}

v2:
  %{ for module in modules ~}
  - ${module}
  %{ endfor ~}
```

### `{{ with … }}` blocks

```text
v1:
  {{- with .config }}
  Endpoint: {{ .endpoint }}
  {{- end }}

v2:
  %{ if config != null ~}
  Endpoint: ${config.endpoint}
  %{ endif ~}
```

### Helper sub-templates

`{{ define "x" }} … {{ end }}` and `{{ template "x" . }}` have no
direct HCL equivalent. Inline the helper, or split it into a
separate file and reference it from blueprint hooks.

### Three-or-more-arg pipes

```text
v1:
  {{ .name | replace "-" "_" | trimSuffix "_old" | upper }}

v2:
  ${upper(trimSuffix(replace(name, "-", "_"), "_old"))}
```

The tool handles single-arg pipes deterministically. Longer chains
are left for manual rewriting because the argument-order convention
diverges between Go pipes and HCL function calls.

## Verification

After migration, scaffold a project from each blueprint to confirm
nothing regressed:

```sh
cd /tmp
forge create your-org/api --registry-dir path/to/your-registry --defaults
forge create your-org/grpc-service --registry-dir path/to/your-registry --defaults
# Inspect the output, run the project's tests, commit if happy.
```

If the migration tool reported `UntranslatedHits`, address each one
in the relevant `.tmpl` file before re-running the verification.

## Rollback

The migration is a regular git change. If something goes wrong:

```sh
git reset --hard HEAD~1   # or whichever commit precedes the migration
```

Older forge releases (the last `text/template` versions) continue to
load v1 blueprints — pin to the previous minor if you need to delay
the cutover for a specific consumer.

## Troubleshooting

### `apiVersion v1 is no longer supported`

You're on a forge release that requires v2 but the registry is still
v1. Run `forge migrate templates --path <registry>`, commit the
result, retry.

### `parsing template "..." Unknown variable`

HCL2 evaluates strict-vars mode by default — every `${name}`
reference must correspond to a declared variable in `blueprint.yaml`.
v1 was lenient and substituted empty strings for missing keys.

Fix: add the variable to `blueprint.yaml` (with a sensible default if
optional), or remove the reference.

### `working tree has uncommitted changes`

The migration tool refuses to overwrite uncommitted work. Either
commit / stash the changes, or pass `--force` if you really know
what you're doing.

## References

- [ADR-0001](adr/0001-use-hcl2-as-the-template-engine.md) — Decision record.
- [DESIGN-0003](design/0003-migrate-template-engine-to-hcl2.md) — Engine swap design.
- [DESIGN-0001](design/0001-blueprint-authoring.md) — Blueprint authoring contract (HCL2).
