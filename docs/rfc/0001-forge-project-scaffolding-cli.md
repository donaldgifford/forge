---
id: RFC-0001
title: "Forge: Project Scaffolding CLI"
status: Accepted
author: Donald Gifford
created: 2026-05-07
---
<!-- markdownlint-disable-file MD025 MD041 -->

# RFC 0001: Forge: Project Scaffolding CLI

**Status:** Accepted
**Author:** Donald Gifford
**Date:** 2026-05-07

> **Implementation note (2026-05-11):** The template engine and
> `apiVersion` references below describe forge's *original* design
> (v1, Go `text/template`). Forge v0.3.0 cut over to HCL2
> (`hashicorp/hcl/v2`) and now requires `apiVersion: v2`. The
> canonical authoring contract is
> [DESIGN-0001](../design/0001-blueprint-authoring.md) (rewritten
> for HCL2); the engine swap is documented in
> [ADR-0001](../adr/0001-use-hcl2-as-the-template-engine.md) and
> [DESIGN-0003](../design/0003-migrate-template-engine-to-hcl2.md).
> Existing v1 registries migrate via `forge migrate templates` —
> see [docs/MIGRATION.md](../MIGRATION.md). The rest of this RFC
> (registry layout, defaults inheritance, sync semantics, lockfile,
> CLI surface) is unchanged.

<!--toc:start-->
- [Summary](#summary)
- [Problem Statement](#problem-statement)
- [Proposed Solution](#proposed-solution)
- [Design](#design)
  - [Core Concepts](#core-concepts)
  - [Architecture](#architecture)
  - [Registry Layout](#registry-layout)
  - [_defaults/ Inheritance Model](#defaults-inheritance-model)
  - [registry.yaml Schema](#registryyaml-schema)
  - [blueprint.yaml Schema](#blueprintyaml-schema)
  - [CLI Surface](#cli-surface)
  - [Lockfile Schema](#lockfile-schema)
  - [Template Engine](#template-engine)
  - [Sync Strategies](#sync-strategies)
- [Alternatives Considered](#alternatives-considered)
- [Implementation Phases](#implementation-phases)
  - [Phase 1: Foundation (Weeks 1–3)](#phase-1-foundation-weeks-13)
  - [Phase 2: Registry & Authoring (Weeks 4–6)](#phase-2-registry--authoring-weeks-46)
  - [Phase 3: Sync Engine (Weeks 7–9)](#phase-3-sync-engine-weeks-79)
  - [Phase 4: Polish & Release (Weeks 10–11)](#phase-4-polish--release-weeks-1011)
- [Risks and Mitigations](#risks-and-mitigations)
- [Success Criteria](#success-criteria)
- [Future Considerations (Post-MVP)](#future-considerations-post-mvp)
- [References](#references)
<!--toc:end-->

## Summary

`forge` is a Go CLI (Go 1.25.4, Cobra) that scaffolds new projects from
**blueprints** stored in a Git-based **registry**. It is inspired by Python's
cookiecutter but adds layered defaults inheritance, managed-file sync, and
registry-based browsing. It supports any language or framework and keeps
scaffolded projects aligned with evolving blueprints over time.

## Problem Statement

Teams maintain dozens of nearly-identical project layouts (CI configs,
linting, license boilerplate, scripts, Dockerfiles). Existing tools either:

- generate once and walk away (cookiecutter, yeoman) — projects drift
  immediately;
- bake conventions into a single language stack (`cargo new`, `go mod init`) —
  no shared lint/CI/license story; or
- require heavyweight platform tooling (Backstage, Cookiecutter+CI gluework).

Forge addresses three pain points at once:

1. **Bootstrap speed** — get a new repo with full CI/lint/license tooling in
   one command.
2. **Drift detection** — tell us when a managed file (CI workflow, lint
   config) has diverged from the blueprint upstream.
3. **Layered conventions** — let an org publish base defaults that every
   blueprint inherits, with category-level (`go/`, `rust/`) and
   blueprint-level overrides.

## Proposed Solution

A single-binary Go CLI organised around two artifacts:

- **Blueprint** — a project skeleton with templated files and a
  `blueprint.yaml` config.
- **Registry** — a Git repo cataloging blueprints with `registry.yaml`, plus
  layered `_defaults/` directories.

The CLI handles fetching (via `hashicorp/go-getter`), template rendering
(HCL2 — `hashicorp/hcl/v2`; original design used Go `text/template`, see
implementation note above), interactive prompting (`charmbracelet/huh`),
and ongoing sync via three-way merge.

## Design

### Core Concepts

| Term | Definition |
|------|-----------|
| **Blueprint** | Project skeleton with templated files and a `blueprint.yaml` config. Lives as a directory within a registry. Inherits from `_defaults/` automatically. |
| **Registry** | Git repository containing one or more blueprints, organised by path, with a top-level `registry.yaml` index and a `_defaults/` directory. |
| **`_defaults/`** | Directory containing files that every blueprint inherits automatically. Blueprints override defaults by providing their own version of the same file. |
| **`blueprint.yaml`** | Per-blueprint config: variables, defaults, prompts, hooks, sync-trackable files, and default overrides/exclusions. |
| **`registry.yaml`** | Index file at the registry root cataloging blueprints and declaring registry-wide defaults. |
| **Scaffolded Project** | Local project generated from a blueprint. Contains a `.forge-lock.yaml` tracking origin blueprint and state. |
| **Managed Files** | Files declared in a blueprint's sync manifest that can be kept up-to-date with the source blueprint after scaffolding. |

### Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                         forge CLI                            │
│                      (cobra commands)                         │
├────────┬───────┬───────┬────────┬───────┬────────────────────┤
│ create │  init │  sync │  check │  list │ search             │
├────────┴───────┴───────┴────────┴───────┴────────────────────┤
│                       Core Engine                            │
├─────────┬───────────┬─────────┬────────┬─────────────────────┤
│ Getter  │ Template  │ Config  │  Sync  │ Registry            │
│ (gg)    │ Renderer  │ Parser  │ Engine │  Index              │
└─────────┴───────────┴─────────┴────────┴─────────────────────┘
```

Key packages:

- `cmd/` — Cobra command definitions
- `internal/config/` — `blueprint.yaml` and `registry.yaml` schemas + loader
- `internal/registry/` — index, resolver, cache
- `internal/defaults/` — layered inheritance resolver
- `internal/getter/` — `hashicorp/go-getter` wrapper
- `internal/template/` — HCL2 renderer with custom funcs (originally Go `text/template`; cut over in v0.3.0 — see DESIGN-0003)
- `internal/prompt/` — `charmbracelet/huh` interactive prompts
- `internal/create/`, `internal/sync/`, `internal/check/` — orchestrators
- `internal/lockfile/` — `.forge-lock.yaml` reader/writer

### Registry Layout

```
forge-blueprints/
├── registry.yaml
├── _defaults/                  # Inherited by ALL blueprints
│   ├── .editorconfig
│   ├── .gitignore.tmpl
│   ├── .github/
│   │   └── workflows/forge-check.yml
│   ├── scripts/
│   │   ├── setup.sh
│   │   └── lint.sh
│   ├── .pre-commit-config.yaml
│   ├── CODEOWNERS.tmpl
│   └── LICENSE.tmpl
├── go/
│   ├── _defaults/              # Go-specific (layered on top)
│   ├── api/
│   │   ├── blueprint.yaml
│   │   └── {{project_name}}/...
│   ├── cli/
│   └── operator/
├── rust/
│   ├── _defaults/
│   ├── cli/
│   └── library/
└── typescript/...
```

### `_defaults/` Inheritance Model

Defaults use **layered inheritance**, resolved bottom-up. Resolution order
(last wins):

1. `/_defaults/` — registry-wide
2. `/<category>/_defaults/` — category-level (e.g. `go/_defaults/`)
3. `/<category>/<blueprint>/` — blueprint files (highest priority)

**Rules:**

- Every file in `_defaults/` is included unless the blueprint provides its
  own version at the same relative path.
- Category `_defaults/` layer on top of root `_defaults/`, overriding
  matching paths.
- Blueprints can explicitly **exclude** inherited defaults via
  `blueprint.yaml`.
- Default files can use `.tmpl` extensions and are rendered with the same
  template engine.
- All inherited defaults are managed for sync purposes (default strategy:
  `overwrite`).

Example resolution for `go/api`:

| File | Source | Reason |
|------|--------|--------|
| `.editorconfig` | `/_defaults/.editorconfig` | No override anywhere |
| `.golangci.yml` | `/go/_defaults/.golangci.yml` | Go category override |
| `scripts/lint.sh` | `/go/_defaults/scripts/lint.sh` | Go category overrides root |
| `.github/workflows/ci.yml` | `/go/api/.github/workflows/ci.yml` | Blueprint override |
| `cmd/main.go` | `/go/api/cmd/main.go` | Blueprint-only file |

### `registry.yaml` Schema

```yaml
apiVersion: v1
name: "acme-blueprints"
description: "ACME Corp standard project blueprints"
maintainers:
  - name: "Platform Engineering"
    email: "platform@acme.com"

defaults:
  sync_strategy: overwrite
  managed: true

blueprints:
  - name: go/api
    path: go/api
    description: "Production Go API service with HTTP/gRPC, observability, and Docker"
    version: "2.1.0"
    tags: ["go", "api", "grpc", "docker"]
    latest_commit: "abc123def456"
  - name: go/cli
    path: go/cli
    description: "Go CLI application with Cobra and release automation"
    version: "1.4.0"
    tags: ["go", "cli", "cobra"]
    latest_commit: "abc123def456"
  # ...
```

### `blueprint.yaml` Schema

```yaml
apiVersion: v1
name: "go-api"
description: "Production Go API service with HTTP/gRPC, observability, and Docker"
version: "2.1.0"
tags: ["go", "api", "grpc", "docker"]

defaults:
  exclude:
    - ".pre-commit-config.yaml"
    - "scripts/setup.sh"
  override_strategy:
    "renovate.json": merge

variables:
  - name: project_name
    description: "Name of the project"
    type: string
    required: true
    validate: "^[a-z][a-z0-9-]*$"
  - name: go_module
    description: "Go module path"
    type: string
    default: "github.com/{{ .org }}/{{ .project_name }}"
  - name: use_grpc
    type: bool
    default: false
  - name: ci_provider
    type: choice
    choices: ["github-actions", "gitlab-ci", "none"]
    default: "github-actions"
  - name: license
    type: choice
    choices: ["MIT", "Apache-2.0", "BSD-3-Clause", "none"]
    default: "Apache-2.0"

conditions:
  - when: "{{ not .use_grpc }}"
    exclude:
      - "proto/"
      - "internal/grpc/"
  - when: "{{ eq .ci_provider \"none\" }}"
    exclude:
      - ".github/"

hooks:
  post_create:
    - "git init"
    - "go mod tidy"

sync:
  managed_files:
    - path: "Makefile"
      strategy: merge
  ignore:
    - "*.generated.go"
    - "vendor/"

rename:
  "{{project_name}}/": "."
```

### CLI Surface

| Command | Purpose |
|---------|---------|
| `forge create <blueprint>` | Scaffold a new project from a blueprint |
| `forge init` | Initialise a new blueprint |
| `forge sync` | Pull updated managed files and defaults from source |
| `forge check` | Detect drift between local files and the source blueprint |
| `forge list` | List blueprints in a registry |
| `forge search` | Search across registries |
| `forge info <blueprint>` | Show details + inherited defaults |
| `forge registry init/blueprint/update` | Registry maintenance |
| `forge cache clean` | Clear cached registries/tools |

`forge create` flow:

1. Resolve the blueprint reference.
2. Fetch via go-getter (with cache).
3. Load `registry.yaml` and `blueprint.yaml`.
4. Prompt for variables (or take `--set` overrides).
5. Resolve defaults inheritance.
6. Apply exclusions and conditions.
7. Render `.tmpl` files; copy others verbatim.
8. Write `.forge-lock.yaml` with provenance.
9. Run `post_create` hooks.

### Lockfile Schema

```yaml
# .forge-lock.yaml — DO NOT EDIT MANUALLY
blueprint:
  registry: "https://github.com/acme/forge-blueprints"
  name: "go/api"
  path: "go/api"
  ref: "v2.1.0"
  commit: "abc123def456"
created_at: "2025-02-08T10:30:00Z"
last_synced: "2025-02-08T10:30:00Z"
forge_version: "0.5.0"
variables:
  project_name: "my-api"
  go_module: "github.com/myorg/my-api"
defaults:
  - path: ".editorconfig"
    source: "_defaults/.editorconfig"
    strategy: overwrite
    synced_commit: "abc123def456"
managed_files:
  - path: "Makefile"
    strategy: merge
    synced_commit: "abc123def456"
```

### Template Engine

> Cut over to HCL2 in v0.3.0. See
> [DESIGN-0001](../design/0001-blueprint-authoring.md) for the
> current authoring contract.

Originally specified as Go `text/template`. The current engine is
HCL2 (`hashicorp/hcl/v2`) with `${expr}` interpolation and
`%{ if … ~}` directives. Custom function map: `snakeCase`,
`camelCase`, `pascalCase`, `kebabCase`, `now`, `env`. Standard
library functions from `cty/function/stdlib`: `upper`, `lower`,
`title`, `replace`, `trimPrefix`, `trimSuffix`, `coalesce`
(replaces v1 `default`). File names ending in `.tmpl` are rendered
and the extension is stripped; other files are copied verbatim.
Directory/file names like `${project_name}/` are also rendered.

### Sync Strategies

- **`overwrite`** — fully replace local file with the upstream version.
- **`merge`** — three-way merge using `synced_commit` as the common
  ancestor; conflicts produce git-style markers.

Sync targets:

1. Inherited default files (auto-managed).
2. Blueprint-declared managed files.

CI integration: `.github/workflows/forge-check.yml` (shipped via
`_defaults/`) runs `forge check --output json` on a schedule for drift
detection.

## Alternatives Considered

| Alternative | Why rejected |
|-------------|--------------|
| Cookiecutter wrapper | No sync/drift story; Python runtime dependency |
| Pure shell + git template repos | No structured variable schema, no inheritance, no merge |
| Backstage Software Templates | Too heavyweight for individual/small-team workflows; opinionated platform |
| Helm-style chart for project scaffolding | Helm value resolution is complex and ill-suited to "render once, sync later" |
| Custom Git-only tool (no go-getter) | Lose archive/HTTP/checksum support that go-getter provides for free |

## Implementation Phases

### Phase 1: Foundation (Weeks 1–3)

Core scaffolding with registry and defaults inheritance working
end-to-end. Milestone: `forge create go/api` resolves from a registry,
inherits defaults, and produces a working project.

### Phase 2: Registry & Authoring (Weeks 4–6)

Full registry workflow: `forge list/search/info`, multi-registry global
config, registry caching, `forge init`, `forge validate`, conditional file
inclusion, post-create hooks.

### Phase 3: Sync Engine (Weeks 7–9)

`forge check` (drift detection), `forge sync` with overwrite and three-way
merge, conflict resolution UX, lockfile updates.

### Phase 4: Polish & Release (Weeks 10–11)

Comprehensive test suite, error UX, docs (`README.md`,
`BLUEPRINT_AUTHORING.md`, `REGISTRY_SETUP.md`), Homebrew/goreleaser, `forge
cache clean`, reference registry. Tag v0.1.0.

Detailed task breakdown lives in [IMPL-0001 — Forge Phased Build
Plan](../impl/0001-forge-phased-build-plan.md).

## Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Defaults inheritance complexity (3 layers) | High | Medium | Strict "last wins" rule; `forge info` shows full resolution; extensive tests |
| Three-way merge complexity for diverse file types | High | Medium | Start overwrite-only; add merge incrementally |
| Sparse-checkout / fetch performance on large registries | Medium | Low | Cache index; only fetch blueprint path + relevant `_defaults/` |
| Private repo authentication | Medium | Medium | Lean on system git credential helpers; don't reinvent auth |
| Template rendering breaking non-text files | Medium | Low | Only render `.tmpl` files; copy everything else verbatim |
| Registry index drift | Low | High | Provide `forge registry update` (and `--check` mode for CI) |

## Success Criteria

v0.1.0 is successful when:

1. `forge create go/api` resolves from a registry, inherits all default
   files, and produces a working project.
2. Default files from `_defaults/` and `<category>/_defaults/` are correctly
   merged with blueprint-level overrides winning.
3. Blueprints can exclude specific defaults via `defaults.exclude`.
4. `forge check` detects updates to defaults and managed files.
5. `forge sync` updates managed files and defaults with overwrite and merge
   strategies.
6. End-to-end test suite passes against the reference registry.

## Future Considerations (Post-MVP)

- Blueprint composition (layering multiple blueprints).
- Interactive TUI for browsing registries.
- `forge upgrade` — re-scaffold against a new blueprint version.
- Plugin/extension system.
- Semver constraints (`forge create go/api@^2.0.0`).
- `forge diff` — show what would change if re-scaffolded.

## References

- [DESIGN-0001 — Blueprint Authoring](../design/0001-blueprint-authoring.md)
- [DESIGN-0002 — Registry Layout & Defaults Inheritance](../design/0002-registry-layout-and-defaults-inheritance.md)
- [IMPL-0001 — Forge Phased Build Plan](../impl/0001-forge-phased-build-plan.md)
- [IMPL-0002 — MVP CLI Gap Closure](../impl/0002-mvp-cli-gap-closure.md)
- [PLAN-0001 — Registry Blueprint & Update Commands](../plan/0001-registry-blueprint-and-update-commands.md)
- [IMPL-0003 — Registry Commands Implementation](../impl/0003-registry-commands.md)
- `hashicorp/go-getter` — source fetching
- `charmbracelet/huh` — interactive prompts
- `gopkg.in/yaml.v3` — YAML parsing
