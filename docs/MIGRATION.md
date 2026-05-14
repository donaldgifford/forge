# Migrating forge blueprints

This guide walks blueprint authors through the on-disk format changes
shipped between releases. Forge has two cumulative migration steps:

1. **v1 → v2 (template engine):** `text/template` → HCL2. The tool is
   `forge migrate templates`. See
   [Migrating template syntax (v0.2.x → v0.3.x)](#migrating-template-syntax-v02x--v03x).
2. **v2 YAML → v2 HCL (config files):** `blueprint.yaml` /
   `registry.yaml` → `blueprint.hcl` / `registry.hcl`. The tool is
   `forge migrate config`. See
   [Migrating config files from YAML to HCL (v0.3.x → v0.4.x)](#migrating-config-files-from-yaml-to-hcl-v03x--v04x).

If you are coming from **v0.2.x or earlier**, run **both** steps in
order — `forge migrate templates` first, then `forge migrate config`.
If you are already on v0.3.x (HCL2 templates, YAML config), only the
second step is required. If you ran the steps **out of order** and
ended up with v1 template syntax stranded in a `blueprint.hcl`-rooted
registry, `forge migrate templates` (v0.4.1+) walks HCL-rooted
blueprints too — re-run it against your registry to clean up.

## Migrating template syntax (v0.2.x → v0.3.x)

This step rewrites template expressions inside `*.tmpl` files and
expression-bearing fields (`variable.default`, `condition.when`) from
Go `text/template` to HCL2. After upgrading forge to a release
containing this change, v1 blueprints **will not load**. Run the
migration tool against your registry, review the result, commit, and
you're done.

### Why the change

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

### The migration command

```sh
forge migrate templates --path <registry-or-blueprint>
```

`--path` accepts either a registry root (rewrites every blueprint
under it, plus registry-wide and category-level `_defaults/`) or a
single blueprint directory.

#### Useful flags

| Flag | Behaviour |
|------|-----------|
| `--dry-run` | Print the rewrite plan without modifying any files. |
| `--strict` | Exit non-zero if any file contains an untranslatable construct. |
| `--force` | Skip the dirty-worktree guard. The tool refuses to run on a registry with uncommitted changes by default — commit or stash first. |

#### Example: registry-wide migration

```sh
cd path/to/your-registry
git status                                       # working tree must be clean
forge migrate templates --path . --dry-run       # preview the changes
forge migrate templates --path .                 # apply
git diff                                         # review
git commit -am "migrate: blueprints to HCL2 templates (apiVersion v2)"
```

The tool bumps `apiVersion: v1 → v2` in `blueprint.yaml` and
`registry.yaml`, rewrites all `*.tmpl` files, and renames any
`{{varname}}` directories to `${varname}`. **It does not change the
file format** — `blueprint.yaml` stays YAML at this step. The
follow-up `forge migrate config` step converts the configs to HCL.

#### Example: single blueprint

```sh
forge migrate templates --path go/api
```

Useful when you want to migrate one blueprint at a time during
review.

### Rewrite rules

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

### Manual fixes the tool does not attempt

The migration tool warns about the following constructs and leaves
them alone — they need a human translation.

#### `{{ range … }}` blocks

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

#### `{{ with … }}` blocks

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

#### Helper sub-templates

`{{ define "x" }} … {{ end }}` and `{{ template "x" . }}` have no
direct HCL equivalent. Inline the helper, or split it into a
separate file and reference it from blueprint hooks.

#### Literal `${...}` for downstream tools (goreleaser, shell, etc.)

A few downstream consumers use `${name}` as their own variable
syntax — goreleaser, GitHub Actions expressions, shell parameter
expansion. v1 forge ignored these (only `{{ }}` was the
substitution syntax). Under v2, HCL2 treats every `${...}` as
substitution, so unescaped occurrences fail strict-vars
validation:

```text
Error: rendering template ".goreleaser.yml.tmpl":
  Unknown variable; There is no variable named "signature".
```

The migration tool does not rewrite these because the choice
between *"this is a forge variable I forgot to declare"* and
*"this is a downstream variable that should be a literal"* is
domain-specific. The fix is to escape with `$$`:

```text
v1 (no escape needed — forge ignored ${} ):
  - "${signature}"
  - "${artifact}"

v2 (escape so HCL2 emits a literal `${...}` ):
  - "$${signature}"
  - "$${artifact}"
```

Same rule applies to GitHub Actions templates (`$${{ secrets.X }}`)
and shell scripts with parameter expansion. Audit any `.tmpl`
file that mixes forge variables with downstream `${...}` syntax
after running `forge migrate templates`.

#### Three-or-more-arg pipes

```text
v1:
  {{ .name | replace "-" "_" | trimSuffix "_old" | upper }}

v2:
  ${upper(trimSuffix(replace(name, "-", "_"), "_old"))}
```

The tool handles single-arg pipes deterministically. Longer chains
are left for manual rewriting because the argument-order convention
diverges between Go pipes and HCL function calls.

### Verifying the template migration

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

### Rolling back the template migration

The migration is a regular git change. If something goes wrong:

```sh
git reset --hard HEAD~1   # or whichever commit precedes the migration
```

Older forge releases (the last `text/template` versions) continue to
load v1 blueprints — pin to the previous minor if you need to delay
the cutover for a specific consumer.

## Migrating config files from YAML to HCL (v0.3.x → v0.4.x)

This step rewrites `blueprint.yaml` and `registry.yaml` files as
their HCL equivalents (`blueprint.hcl`, `registry.hcl`). The
`apiVersion` field is dropped — the file extension is now the version
signal. Templated fields (`variable.default`, `condition.when`,
`rename` entries) round-trip with their HCL syntax intact.

After upgrading to a release containing this change, bare YAML
config files **will not load** — `forge` rejects them at load time
with a pointer to the migration command.

### Why the change

The HCL2 cutover (v0.3.x) left blueprints in an awkward
two-format-per-file state: the *outer* config was YAML, but the
*inner* expression strings (`default:`, `when:`, `rename: from/to`)
were HCL2 source. YAML's quoting and escaping rules then layered on
top of HCL's, producing fields like:

```yaml
default: "${ \"github.com/\" + org + \"/\" + project_name }"
```

Moving to a single format eliminates the double escaping and means a
single grammar (HCL2) covers every authoring surface — config,
templates, and rename rules.

See [DESIGN-0004](design/0004-unify-config-file-format-after-hcl2-cutover.md)
for the design and IMPL-0005 for the rollout plan.

### The migration command

```sh
forge migrate config --path <registry-or-blueprint>
```

`--path` accepts either a registry root (rewrites every
`blueprint.yaml` and `registry.yaml` under it) or a single blueprint
directory.

#### Useful flags

| Flag | Behaviour |
|------|-----------|
| `--dry-run` | Print the rewrite plan without modifying any files. |
| `--strict` | Exit non-zero if any file fails to migrate. |
| `--force` | Skip the dirty-worktree guard. |

#### Example: registry-wide migration

```sh
cd path/to/your-registry
git status                                   # working tree must be clean
forge migrate config --path . --dry-run      # preview the changes
forge migrate config --path .                # apply
git diff                                     # review
git commit -am "migrate: config files to HCL"
```

The tool emits a sibling `.hcl` file for every `.yaml` it finds, then
deletes the original. The output passes through `hclwrite.Format` so
the result is canonically formatted.

### What changes on disk

| Before | After |
|--------|-------|
| `registry.yaml` | `registry.hcl` |
| `blueprint.yaml` | `blueprint.hcl` |
| `apiVersion: v2` line | (removed; extension carries the version signal) |
| `variables: [ { name: …, type: …, default: … } ]` | One `variable "<name>" { type = …, default = … }` block per variable. |
| `conditions: [ { when: …, exclude: […] } ]` | One `condition { when = …, exclude = [...] }` block per condition. |
| `hooks: { post_create: [...] }` | `hooks { post_create = [...] }` block. |
| `sync: { managed_files: { F: { strategy: … } } }` | `sync { managed_file "F" { strategy = … } }` blocks. |
| `rename: [ { from: …, to: … } ]` | `rename { entry { from = …, to = … } }` blocks. |
| `defaults: { exclude: [...] }` | `defaults { exclude = [...] }` block. |
| `blueprints: [ { name: foo, path: …, … } ]` (registry) | `blueprint "foo" { path = …, … }` blocks. |

The migration is round-trip-safe in both directions for *content* —
the same blueprint loads to the same in-memory `Blueprint` struct
either way.

### Manual fixes the tool does not attempt

#### Comment preservation

YAML comments are **not** preserved by the migrator. The HCL emitter
is structural: it walks the parsed `Blueprint` / `Registry` and
writes them out cleanly. Any `# …` lines in the source YAML are lost.
Re-add author comments by hand after migration. (See IMPL-0005 OQ-3
for the rationale — comment round-trip would have required a parallel
custom YAML AST walker, which is not worth the maintenance cost for
a one-shot migration.)

#### Mixed-format directories

If both `blueprint.yaml` and `blueprint.hcl` already exist side by
side in the same directory, the tool refuses to touch it. Clean up
the partial migration first (`git rm blueprint.yaml` or `git rm
blueprint.hcl`, depending on which is current) and re-run.

### Verifying the config migration

```sh
cd /tmp
forge create your-org/api --registry-dir path/to/your-registry --defaults
# Confirm a project still scaffolds correctly. The output should be
# byte-identical to a v0.3.x scaffold from the same inputs.
```

If anything looks off, the most likely cause is a hand-edited
expression field that did not round-trip cleanly. Inspect the
relevant `blueprint.hcl` and adjust the expression by hand.

### Rolling back the config migration

```sh
git reset --hard HEAD~1   # or whichever commit precedes the migration
```

Older forge releases (the last YAML-config versions) continue to
load `blueprint.yaml` / `registry.yaml` — pin to v0.3.x if you need
to delay the cutover.

## Troubleshooting

### `apiVersion v1 is no longer supported`

You're on a forge release that requires v2 but the registry is still
v1. Run `forge migrate templates --path <registry>`, commit the
result, retry.

### `YAML config files are no longer supported`

The full message reads:

```text
blueprint file path/to/blueprint.yaml: YAML config files are no longer
supported. Run `forge migrate config --path path/to` to convert this
file to blueprint.hcl. See docs/MIGRATION.md in the forge repository
for the YAML→HCL migration guide
```

You're on a forge release that requires HCL config files but the
registry still has YAML. Run the suggested `forge migrate config`
command, commit the result, retry. The same applies to bare
`registry.yaml` files.

### `parsing template "..." Unknown variable`

HCL2 evaluates strict-vars mode by default — every `${name}`
reference must correspond to a declared variable in `blueprint.hcl`.
v1 was lenient and substituted empty strings for missing keys.

Fix: add the variable to `blueprint.hcl` (with a sensible default if
optional), or remove the reference.

### `working tree has uncommitted changes`

The migration tool refuses to overwrite uncommitted work. Either
commit / stash the changes, or pass `--force` if you really know
what you're doing.

### `both blueprint.yaml and blueprint.hcl exist`

A previous `forge migrate config` run was interrupted, or the
directory was hand-edited. Inspect both files, decide which is
authoritative, `git rm` the other, and re-run the migration if
needed.

## References

- [ADR-0001](adr/0001-use-hcl2-as-the-template-engine.md) — Decision record (template engine).
- [DESIGN-0001](design/0001-blueprint-authoring.md) — Blueprint authoring contract (HCL2).
- [DESIGN-0002](design/0002-registry-layout-and-defaults-inheritance.md) — Registry layout (HCL).
- [DESIGN-0003](design/0003-migrate-template-engine-to-hcl2.md) — Engine swap design.
- [DESIGN-0004](design/0004-unify-config-file-format-after-hcl2-cutover.md) — Config-format unification.
- [IMPL-0004](impl/0004-migrate-template-engine-to-hcl2.md) — Engine cutover implementation.
- [IMPL-0005](impl/0005-unify-config-file-format-to-hcl2.md) — Config-format cutover implementation.
