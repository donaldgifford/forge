---
id: IMPL-0001
title: "Forge Phased Build Plan"
status: Completed
author: Donald Gifford
created: 2026-05-07
---
<!-- markdownlint-disable-file MD025 MD041 -->

# IMPL 0001: Forge Phased Build Plan

**Status:** Completed
**Author:** Donald Gifford
**Date:** 2026-05-07

<!--toc:start-->
- [Objective](#objective)
- [Scope](#scope)
  - [In Scope](#in-scope)
  - [Out of Scope](#out-of-scope)
- [Implementation Phases](#implementation-phases)
  - [Phase 1: Foundation](#phase-1-foundation)
    - [1.1 — Project Skeleton and Entry Point [DONE]](#11--project-skeleton-and-entry-point-done)
    - [1.2 — YAML Schema Types and Config Loader [DONE]](#12--yaml-schema-types-and-config-loader-done)
    - [1.3 — Test Fixtures [DONE]](#13--test-fixtures-done)
    - [1.4 — Source Fetcher (go-getter) [DONE]](#14--source-fetcher-go-getter-done)
    - [1.5 — Registry Resolver [DONE]](#15--registry-resolver-done)
    - [1.6 — Defaults Inheritance Resolver [DONE]](#16--defaults-inheritance-resolver-done)
    - [1.7 — Template Rendering Engine [DONE]](#17--template-rendering-engine-done)
    - [1.8 — Interactive Prompt Engine [DONE]](#18--interactive-prompt-engine-done)
    - [1.9 — Lockfile Manager [DONE]](#19--lockfile-manager-done)
    - [1.10 — forge create Command [DONE]](#110--forge-create-command-done)
    - [1.11 — Registry Cache [DONE]](#111--registry-cache-done)
  - [Phase 2: Registry Browsing, Authoring, and Tools](#phase-2-registry-browsing-authoring-and-tools)
    - [2.1 — forge list Command [DONE]](#21--forge-list-command-done)
    - [2.2 — forge search Command [DONE]](#22--forge-search-command-done)
    - [2.3 — Global Config and Multi-Registry Support [DONE]](#23--global-config-and-multi-registry-support-done)
    - [2.4 — Conditional File Inclusion [DONE]](#24--conditional-file-inclusion-done)
    - [2.5 — Post-Create Hooks [DONE]](#25--post-create-hooks-done)
    - [2.6 — forge init Command [DONE]](#26--forge-init-command-done)
    - [2.7 — Tool Manifest Parser and Inheritance [DONE]](#27--tool-manifest-parser-and-inheritance-done)
    - [2.8 — Platform Resolver and Tool Downloader [DONE]](#28--platform-resolver-and-tool-downloader-done)
    - [2.9 — forge tools Commands [DONE]](#29--forge-tools-commands-done)
    - [2.10 — Wire Tools into forge create [DONE]](#210--wire-tools-into-forge-create-done)
  - [Phase 3: Sync Engine](#phase-3-sync-engine)
    - [3.1 — forge check Command [DONE]](#31--forge-check-command-done)
    - [3.2 — Sync Engine — Overwrite Strategy [DONE]](#32--sync-engine--overwrite-strategy-done)
    - [3.3 — Sync Engine — Three-Way Merge Strategy [DONE]](#33--sync-engine--three-way-merge-strategy-done)
    - [3.4 — forge sync Command [DONE]](#34--forge-sync-command-done)
    - [3.5 — Conflict Resolution UX [DONE]](#35--conflict-resolution-ux-done)
  - [Phase 4: Polish and Release](#phase-4-polish-and-release)
    - [4.1 — Error Handling and UX [DONE]](#41--error-handling-and-ux-done)
    - [4.2 — forge info Command [DONE]](#42--forge-info-command-done)
    - [4.3 — forge cache clean Command [DONE]](#43--forge-cache-clean-command-done)
    - [4.4 — Comprehensive Test Suite [DONE]](#44--comprehensive-test-suite-done)
    - [4.5 — Documentation [DONE]](#45--documentation-done)
    - [4.6 — Reference Registry [DONE]](#46--reference-registry-done)
  - [Phase 5: Miscellaneous](#phase-5-miscellaneous)
    - [5.1 — License Compliance CI Job [DONE]](#51--license-compliance-ci-job-done)
    - [5.2 — Add go-licenses to Makefile and mise.toml [DONE]](#52--add-go-licenses-to-makefile-and-misetoml-done)
- [File Changes](#file-changes)
- [Testing Plan](#testing-plan)
- [Dependency Graph](#dependency-graph)
- [Dependencies](#dependencies)
- [References](#references)
<!--toc:end-->

## Objective

Break the forge specification (RFC-0001) into concrete, ordered
implementation steps. Each step produces working, testable code.
Dependencies between steps are explicit — nothing starts before its
prerequisites are done. All tasks below are marked DONE.

**Implements:** [RFC-0001 — Forge: Project Scaffolding CLI](../rfc/0001-forge-project-scaffolding-cli.md)

## Scope

### In Scope

- Phase 1 (Foundation): scaffolding flow end-to-end with registry +
  defaults inheritance.
- Phase 2 (Registry browsing/authoring + tools).
- Phase 3 (Sync engine: check, sync, three-way merge, conflict
  resolution).
- Phase 4 (Polish: error UX, tests, docs, reference registry).
- Phase 5 (Misc: license-compliance CI).

### Out of Scope

- Post-MVP features tracked in RFC-0001's "Future Considerations" section
  (composition, TUI, semver constraints, plugin system).
- CLI wiring gaps surfaced after this plan (covered by IMPL-0002).
- Registry maintenance commands `registry blueprint`/`registry update`
  (covered by PLAN-0001 and IMPL-0003).

## Implementation Phases

### Phase 1: Foundation

**Goal:** `forge create go/api` resolves from a local or remote registry,
inherits defaults, prompts for variables, renders templates, and writes a
working project.

#### 1.1 — Project Skeleton and Entry Point [DONE]

Set up the Cobra CLI scaffold with a root command, version flag, and
global config structure.

**Files:**

- `cmd/forge/main.go` — entry point, calls `cmd.Execute()`
- `cmd/root.go` — root command with `--verbose`, `--no-color`, `--config`
- `cmd/version.go` — `forge version` subcommand, prints build-time
  ldflags

**Details:**

- Wire ldflags in the Makefile (`-X main.version=...`,
  `-X main.commit=...`).
- Use `cobra-cli` to generate initial command stubs, then clean up.
- Root command initialises a `*slog.Logger` based on `--verbose`.

**Verification:** `make build && ./build/bin/forge version` prints
version info. `make lint` passes.

#### 1.2 — YAML Schema Types and Config Loader [DONE]

Define Go structs for `blueprint.yaml` and `registry.yaml`, plus a loader
that reads, unmarshals, and validates them.

**Files:**

- `internal/config/blueprint.go` — `Blueprint` struct
- `internal/config/registry.go` — `Registry` struct
- `internal/config/loader.go` — `LoadBlueprint(path)`,
  `LoadRegistry(path)`
- `internal/config/validate.go` — required fields, regex on `apiVersion`,
  variable type checking
- `internal/config/loader_test.go`
- `internal/config/validate_test.go`

**Details:**

- Use `gopkg.in/yaml.v3` for parsing.
- Struct field tags: `yaml:"field_name"`.
- Validate `apiVersion: v1`, required `name`, variable type ∈ `string |
  bool | choice | int`.
- Variable `validate` field holds a regex pattern — compile and test
  during validation.
- `defaults.exclude` is `[]string`; `defaults.override_strategy` is
  `map[string]string`.
- Tool entries include `condition` field (raw template string, evaluated
  later).

**Verification:** Unit tests load sample YAML fixtures from `testdata/`
and assert parsed struct fields. Invalid YAML returns structured errors.

#### 1.3 — Test Fixtures [DONE]

Create a minimal test registry on disk that all subsequent tests can
use.

**Files:**

- `testdata/registry/registry.yaml`
- `testdata/registry/_defaults/.editorconfig`
- `testdata/registry/_defaults/.gitignore.tmpl`
- `testdata/registry/_defaults/LICENSE.tmpl`
- `testdata/registry/go/_defaults/.golangci.yml`
- `testdata/registry/go/_defaults/scripts/lint.sh`
- `testdata/registry/go/api/blueprint.yaml`
- `testdata/registry/go/api/{{project_name}}/cmd/main.go.tmpl`
- `testdata/registry/go/api/{{project_name}}/go.mod.tmpl`
- `testdata/registry/go/api/{{project_name}}/README.md.tmpl`

**Details:** Keep fixtures minimal — just enough to exercise all three
inheritance layers, template rendering, and conditional inclusion.

**Verification:** Files exist and are valid YAML/templates. Loader tests
from 1.2 use these fixtures.

#### 1.4 — Source Fetcher (go-getter) [DONE]

Wrap `hashicorp/go-getter` to fetch registries (and later tool
binaries).

**Files:**

- `internal/getter/getter.go` — `Getter` with `Fetch(src, dest, opts)`
  and `FetchFile(src, dest, opts)`
- `internal/getter/url.go` — `RegistryURL`, `ToolURL` helpers
- `internal/getter/getter_test.go`
- `internal/getter/url_test.go`

**Details:**

- go-getter URL format: `github.com/owner/repo//subpath?ref=tag` —
  double-slash separates repo from subpath, aligning with forge
  resolution syntax.
- `FetchOpts`: `Ref string` (`?ref=`), `Checksum string`
  (`?checksum=sha256:...`).
- For registry fetches, go-getter clones via system git and extracts
  subpath natively (no sparse checkout needed).
- Auth via system git credential helpers and SSH agent.
- All operations use `slog` for debug logging.

**Verification:** Integration test fetches a subpath from a local bare
git repo. Test URL construction helpers.

#### 1.5 — Registry Resolver [DONE]

Resolve `go/api`, `go/api@v2.1.0`, or a full URL to a concrete registry
URL + blueprint path + ref.

**Files:**

- `internal/registry/resolver.go` — `Resolve(input, cfg)
  (*ResolvedBlueprint, error)`
- `internal/registry/index.go` — `LoadIndex(registryPath)`
- `internal/registry/resolver_test.go`
- `internal/registry/index_test.go`

**Details:**

- Parse formats:
  - `go/api` → default registry + path + latest
  - `go/api@v2.1.0` → pin to ref
  - `github.com/acme/blueprints//go/api[?ref=v2.1.0]` → explicit URL
  - `git@github.com:someone/standalone-blueprint.git` → standalone
- Convert internal `@v2.1.0` shorthand to go-getter's `?ref=v2.1.0`.
- `LoadIndex` reads `registry.yaml` and finds the matching blueprint
  entry, returning a clear error listing available blueprints if not
  found.

**Verification:** Unit tests cover all input formats. Index tests use
`testdata/registry/registry.yaml`.

#### 1.6 — Defaults Inheritance Resolver [DONE]

Walk the registry tree and merge layered `_defaults/` directories with
the blueprint's own files, applying exclusions.

**Files:**

- `internal/defaults/resolver.go` — `Resolve(registryRoot,
  blueprintPath, exclusions) (*FileSet, error)`
- `internal/defaults/resolver_test.go`

**Details:**

- Algorithm: collect `/_defaults/` files → walk category `_defaults/` →
  walk blueprint dir → remove exclusions.
- Skip `_defaults` when collecting blueprint files.
- Track `SourceLayer` (registry, category, blueprint) for lockfile
  provenance.
- A file is a template if its name ends in `.tmpl`.

**Verification:** Unit tests against `testdata/registry/`. Excluded
files absent from result.

#### 1.7 — Template Rendering Engine [DONE]

Render `.tmpl` files using Go `text/template` with custom funcs. Handle
directory name templating.

**Files:**

- `internal/template/funcs.go` — custom `FuncMap`
- `internal/template/renderer.go` — `RenderFile`, `RenderString`,
  `RenderPath`
- `internal/template/funcs_test.go`
- `internal/template/renderer_test.go`

**Details:**

- Custom funcs: `snakeCase`, `camelCase`, `pascalCase`, `kebabCase`,
  `upper`, `lower`, `title`, `replace`, `trimPrefix`, `trimSuffix`,
  `now`, `env`, `default`.
- `RenderFile` uses `missingkey=zero` (so `default` works); `render`
  uses `missingkey=error`.
- `RenderPath` normalises `{{varname}}` → `{{.varname}}` for path
  convenience.
- Strip `.tmpl` extension after rendering.

**Verification:** Unit tests render samples with known vars; assert
output. Test all custom functions.

#### 1.8 — Interactive Prompt Engine [DONE]

Prompt the user for blueprint variables.

**Files:**

- `internal/prompt/prompt.go` — `PromptForVariables(vars, overrides)`
- `internal/prompt/prompt_test.go`

**Details:**

- `charmbracelet/huh` for the interactive UI.
- Skip prompting when `--set key=value` overrides are provided.
- Type dispatch: `string` (text), `bool` (confirm), `choice` (select),
  `int` (text + int validation).
- Render `default` field as a template (can reference earlier vars).
- `--defaults` skips all prompts.

**Verification:** Unit tests use overrides to bypass interactive
prompts.

#### 1.9 — Lockfile Manager [DONE]

Generate and read `.forge-lock.yaml` to track blueprint provenance,
variable values, default file sources, and tool versions.

**Files:**

- `internal/lockfile/lock.go` — `Lockfile` struct, `Write`, `Read`
- `internal/lockfile/lock_test.go`

**Details:**

- Fields: `Blueprint`, `CreatedAt`, `LastSynced`, `ForgeVersion`,
  `Variables`, `Defaults`, `ManagedFiles`, `Tools`.
- `Write` marshals to YAML with a `# DO NOT EDIT MANUALLY` header.

**Verification:** Round-trip test.

#### 1.10 — `forge create` Command [DONE]

Wire all pieces together.

**Files:**

- `cmd/create.go`
- `internal/create/create.go`

**Details:**

Flags: `--set key=value`, `--output-dir`/`-o`, `--defaults`,
`--no-tools`.

Orchestration: resolve → fetch → load index → load blueprint → validate
→ prompt → resolve defaults → evaluate conditions → build file list →
render and write → apply renames → write lockfile → log skipped
hooks.

**Verification:** Integration test against `testdata/registry/`.

#### 1.11 — Registry Cache [DONE]

Cache fetched registries locally.

**Files:**

- `internal/registry/cache.go` — `Cache` with `GetOrFetch`,
  `Invalidate`
- `internal/registry/cache_test.go`

**Details:**

- Hash registry URL (SHA256, truncated) as cache dir name.
- Store ref in `.forge-cache-meta` for staleness checks.
- Respect `XDG_CACHE_HOME`.

**Verification:** Test cache hit and ref-change re-fetch.

### Phase 2: Registry Browsing, Authoring, and Tools

**Goal:** Users can browse registries, author new blueprints, and manage
remote tools.

#### 2.1 — `forge list` Command [DONE]

List blueprints from one or more registries.

- `cmd/list.go`
- `internal/list/list.go`

Flags: `--tag`, `--registry`, `--output` (`table | json`). Use
`tablewriter` and `lipgloss`.

#### 2.2 — `forge search` Command [DONE]

Case-insensitive substring match across `name`, `description`, `tags`.

- `cmd/search.go`
- `internal/search/search.go`

#### 2.3 — Global Config and Multi-Registry Support [DONE]

- `internal/config/global.go` — `GlobalConfig` (`Registries`,
  `CacheDir`, `DefaultRegistry`)
- Load from `~/.config/forge/config.yaml` (respect `XDG_CONFIG_HOME`)

#### 2.4 — Conditional File Inclusion [DONE]

- `internal/create/conditions.go` — `EvaluateConditions(...)`
- Render `when` template, apply glob `exclude` to `FileSet`.

#### 2.5 — Post-Create Hooks [DONE]

- `internal/hooks/hooks.go` — `RunPostCreate(hooks, workDir)`
- Run via `exec.Command("sh", "-c", ...)`. Failures warn but don't
  abort.

#### 2.6 — `forge init` Command [DONE]

- `cmd/init.go`
- `internal/initcmd/init.go` — current dir, registry-mode, `--from`
  reverse-engineering.

#### 2.7 — Tool Manifest Parser and Inheritance [DONE]

- `internal/tools/manifest.go` — merge registry → category → blueprint
  tools (last wins by `name`); evaluate `condition` per tool.

#### 2.8 — Platform Resolver and Tool Downloader [DONE]

- `internal/tools/platform.go` — `DetectPlatform`,
  `ResolveAssetURL`
- `internal/tools/downloader.go` — go-getter for downloads + extraction
  + checksum; `exec` for `go-install`/`npm`/`cargo-install`/`script`.
- `internal/tools/cache.go` — `~/.cache/forge/tools/<name>/<version>/`.

#### 2.9 — `forge tools` Commands [DONE]

- `cmd/tools.go`, `cmd/tools_install.go`, `cmd/tools_list.go`,
  `cmd/tools_check.go`, `cmd/tools_update.go`.

#### 2.10 — Wire Tools into `forge create` [DONE]

Resolve, download, install tools after file generation. Skip if
`--no-tools`.

### Phase 3: Sync Engine

**Goal:** `forge check` detects drift; `forge sync` updates managed
files and defaults using overwrite or three-way merge.

#### 3.1 — `forge check` Command [DONE]

- `cmd/check.go` (`--output text|json`)
- `internal/check/check.go` — fetch registry at synced + latest commits,
  diff defaults/managed files; render styled output.

#### 3.2 — Sync Engine — Overwrite Strategy [DONE]

- `internal/sync/engine.go` — `Sync(opts)` returns
  Updated/Skipped/Conflicts.
- `internal/sync/overwrite.go` — `Overwrite(localPath,
  sourceContent)`.
- Re-render `.tmpl` content using lockfile vars before writing.

#### 3.3 — Sync Engine — Three-Way Merge Strategy [DONE]

- `internal/sync/merge.go` — `ThreeWayMerge(base, local, remote)` →
  merged content + conflict list.
- `internal/sync/diff.go` — `go-diff` wrapper.
- Conflicts produce git-style markers.

#### 3.4 — `forge sync` Command [DONE]

- `cmd/sync.go` (`--dry-run`, `--force`, `--file`, `--include-tools`)
- Fetch base + latest, apply per-file strategy, update lockfile.

#### 3.5 — Conflict Resolution UX [DONE]

- `internal/sync/conflict.go` — `ResolveConflicts(conflicts,
  interactive)`
- Interactive: keep local / accept remote / keep markers / open editor.
- Non-interactive (CI): write markers, exit non-zero.

### Phase 4: Polish and Release

#### 4.1 — Error Handling and UX [DONE]

- `internal/ui/output.go` — `Success`, `Warning`, `Error`, `Info`,
  progress indicators.
- Respect `--no-color` and `NO_COLOR`.
- Wrap errors with context: `fmt.Errorf("failed to clone registry %s:
  %w", url, err)`.

#### 4.2 — `forge info` Command [DONE]

- `cmd/info.go`, `internal/info/info.go`
- Show description, version, variables (types/defaults), inherited files
  with source layer, tools.

#### 4.3 — `forge cache clean` Command [DONE]

- `cmd/cache.go` — `forge cache clean [--registries] [--tools] [--all]`

#### 4.4 — Comprehensive Test Suite [DONE]

- End-to-end integration tests in `internal/create/`,
  `internal/sync/`, `internal/tools/`. Target 60% coverage (matches
  `.codecov.yml`).

#### 4.5 — Documentation [DONE]

- `README.md`, `docs/BLUEPRINT_AUTHORING.md` (now DESIGN-0001),
  `docs/REGISTRY_SETUP.md` (now DESIGN-0002), `docs/TOOLS_GUIDE.md`.

#### 4.6 — Reference Registry [DONE]

- Companion repo (or `examples/`) with `registry.yaml`, `_defaults/`, at
  least `go/cli` and `go/api`, tool declarations for `golangci-lint`
  and `goreleaser`.

### Phase 5: Miscellaneous

#### 5.1 — License Compliance CI Job [DONE]

- `.github/workflows/license-check.yml` running `google/go-licenses`
  against allowlist: `Apache-2.0`, `MIT`, `BSD-2-Clause`,
  `BSD-3-Clause`, `ISC`, `MPL-2.0`.
- Generates a CSV report as a build artifact.

#### 5.2 — Add `go-licenses` to Makefile and `mise.toml` [DONE]

- `mise.toml` — `go:github.com/google/go-licenses`.
- `Makefile` — `license-check`, `license-report` targets; wire into
  `make ci`.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `cmd/*.go` | Create | Cobra command definitions |
| `internal/*` | Create | Core packages (config, registry, defaults, getter, template, prompt, create, sync, check, hooks, lockfile, list, search, info, initcmd, ui, tools) |
| `testdata/registry/` | Create | Test fixtures |
| `.github/workflows/license-check.yml` | Create | License compliance CI |
| `Makefile` | Modify | Add license targets |
| `mise.toml` | Modify | Add `go-licenses` |

## Testing Plan

- Unit tests for all exported functions in each `internal/` package.
- Integration tests using `t.TempDir()` and `testdata/registry/`.
- Table-driven tests for resolver, conditions, template funcs.
- E2E CLI tests for `create`, `sync`, `check`.

## Dependency Graph

```
1.1 (skeleton)
 │
 ├── 1.2 (config types) ──── 1.3 (test fixtures)
 │    │
 │    ├── 1.5 (registry resolver) ── 1.11 (registry cache)
 │    ├── 1.6 (defaults resolver)
 │    ├── 1.8 (prompt engine)
 │    └── 1.9 (lockfile)
 │
 ├── 1.4 (go-getter) ◄── also used by 2.8 / 3.1 / 3.4
 │
 └── 1.7 (template engine)

All Phase 1 ──► 1.10 (forge create)

1.10 ──► 2.1 list / 2.2 search / 2.3 global config
1.10 ──► 2.4 conditions / 2.5 hooks / 2.6 init
1.10 ──► 2.7 tool manifest ── 2.8 downloader ── 2.9 commands ── 2.10 wire

2.10 ──► 3.1 check ── 3.2 sync overwrite ── 3.3 sync merge ── 3.4 sync command ── 3.5 conflict UX

3.5 ──► 4.1–4.6 (polish) ──► 5.1–5.2 (compliance)
```

## Dependencies

| Dependency | Purpose |
|------------|---------|
| Go 1.25.4 | Language runtime |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/spf13/viper` | Config file handling |
| `gopkg.in/yaml.v3` | YAML parsing |
| `github.com/hashicorp/go-getter` | Source fetching, archive extraction, checksums |
| `github.com/charmbracelet/huh` | Interactive prompts |
| `github.com/charmbracelet/lipgloss` | Terminal styling |
| `github.com/olekukonez/tablewriter` | CLI table output |
| `github.com/sergi/go-diff` | Diff computation |
| `github.com/stretchr/testify` | Test assertions |

## References

- [RFC-0001 — Forge: Project Scaffolding CLI](../rfc/0001-forge-project-scaffolding-cli.md)
- [DESIGN-0001 — Blueprint Authoring](../design/0001-blueprint-authoring.md)
- [DESIGN-0002 — Registry Layout & Defaults Inheritance](../design/0002-registry-layout-and-defaults-inheritance.md)
- [IMPL-0002 — MVP CLI Gap Closure](0002-mvp-cli-gap-closure.md)
