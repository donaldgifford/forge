# Project Plan: `forge` — A Go CLI Project Scaffolding Tool

## Executive Summary

**forge** is a Go CLI tool (built with Go 1.25.4 and Cobra CLI) inspired by Python's cookiecutter. It scaffolds new projects from **blueprints** — project templates stored in a Git-based **registry**. Every blueprint automatically inherits a set of **default files** from the registry's `_defaults/` directory (CI configs, linting, standard scripts, tool declarations) unless explicitly overridden. It supports any language or framework, and includes a sync mechanism to keep scaffolded projects aligned with evolving blueprints — including versioned remote tools that are downloaded on demand rather than stored as binaries.

---

## 1. Core Concepts & Terminology

| Term | Definition |
|------|-----------|
| **Blueprint** | A project skeleton with templated files and a `blueprint.yaml` config. Lives as a directory within a registry. Inherits from `_defaults/` automatically. |
| **Registry** | A Git repository containing one or more blueprints, organized by path, with a top-level `registry.yaml` index and a `_defaults/` directory. |
| **`_defaults/`** | A directory at the registry root containing files that every blueprint inherits automatically. Blueprints override defaults by providing their own version of the same file. |
| **blueprint.yaml** | The config file in each blueprint that declares variables, defaults, prompts, hooks, sync-trackable files, and default overrides/exclusions. |
| **registry.yaml** | The index file at the root of a registry repo that catalogs all available blueprints and declares registry-wide defaults and tool manifests. |
| **Tool Manifest** | A declaration of remote CLI tools/binaries with version pins and download URLs. Tools are fetched at scaffold time — never stored in the registry. |
| **Scaffolded Project** | The local project generated from a blueprint. Contains a `.forge-lock.yaml` tracking its origin blueprint, tool versions, and state. |
| **Managed Files** | Files declared in a blueprint's sync manifest that can be kept up-to-date with the source blueprint after scaffolding. |

---

## 2. Architecture Overview

```
┌──────────────────────────────────────────────────────────────┐
│                         forge CLI                            │
│                      (cobra commands)                         │
├────────┬───────┬───────┬────────┬───────┬────────┬───────────┤
│ create │  init │  sync │  check │  list │ search │  tools    │
├────────┴───────┴───────┴────────┴───────┴────────┴───────────┤
│                       Core Engine                            │
├─────────┬───────────┬─────────┬────────┬──────────┬──────────┤
│   Git   │ Template  │ Config  │  Sync  │ Registry │  Tools   │
│  Client │ Renderer  │ Parser  │ Engine │  Index   │ Resolver │
└─────────┴───────────┴─────────┴────────┴──────────┴──────────┘
```

### Package Layout

```
forge/
├── cmd/                        # Cobra command definitions
│   ├── root.go
│   ├── create.go               # Scaffold a project from a blueprint
│   ├── init.go                 # Initialize a new blueprint
│   ├── sync.go                 # Sync managed files from blueprint
│   ├── check.go                # Check for available blueprint updates
│   ├── list.go                 # List blueprints in a registry
│   ├── search.go               # Search across registries
│   └── tools.go                # Manage remote tools (install, update, list)
├── internal/
│   ├── config/                 # blueprint.yaml parsing and validation
│   │   ├── schema.go
│   │   └── loader.go
│   ├── registry/               # Registry index management
│   │   ├── index.go            # registry.yaml parsing
│   │   ├── resolver.go         # Blueprint path + version resolution
│   │   └── cache.go            # Local registry cache
│   ├── defaults/               # _defaults/ inheritance and merge logic
│   │   ├── resolver.go         # File inheritance resolution
│   │   └── merge.go            # Defaults + blueprint overlay
│   ├── git/                    # Git clone, fetch, sparse checkout
│   │   └── client.go
│   ├── template/               # Template rendering engine
│   │   ├── renderer.go
│   │   └── funcs.go            # Custom template functions
│   ├── prompt/                 # Interactive user prompts
│   │   └── prompt.go
│   ├── sync/                   # Sync and check engine
│   │   ├── engine.go
│   │   └── diff.go
│   ├── tools/                  # Remote tool resolution and download
│   │   ├── manifest.go         # Tool manifest parsing
│   │   ├── resolver.go         # Platform/arch detection, URL resolution
│   │   ├── downloader.go       # Download, verify checksum, extract
│   │   └── cache.go            # Local tool cache (~/.cache/forge/tools/)
│   └── lockfile/               # .forge-lock.yaml management
│       └── lock.go
├── testdata/                   # Test registries and blueprints
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 3. Registry Design

A registry is a Git repo with a well-known structure. The `_defaults/` directory provides a base layer that every blueprint inherits automatically.

### Registry Layout

```
forge-blueprints/                        # The registry repo
├── registry.yaml                        # Index + registry-wide config
│
├── _defaults/                           # Inherited by ALL blueprints
│   ├── .editorconfig
│   ├── .gitignore.tmpl                  # Can be templated
│   ├── .github/
│   │   ├── dependabot.yml
│   │   ├── renovate.json
│   │   └── workflows/
│   │       └── forge-check.yml          # Blueprint drift check CI
│   ├── scripts/
│   │   ├── setup.sh                     # Standard dev setup script
│   │   └── lint.sh
│   ├── .pre-commit-config.yaml
│   ├── CODEOWNERS.tmpl
│   └── LICENSE.tmpl
│
├── go/
│   ├── _defaults/                       # Go-specific defaults (layered on top)
│   │   ├── .golangci.yml
│   │   ├── .github/
│   │   │   └── workflows/
│   │   │       └── ci.yml               # Go-specific CI
│   │   └── scripts/
│   │       └── lint.sh                  # Overrides root _defaults/scripts/lint.sh
│   ├── api/
│   │   ├── blueprint.yaml
│   │   ├── {{project_name}}/
│   │   │   ├── cmd/
│   │   │   ├── internal/
│   │   │   └── go.mod.tmpl
│   │   └── .github/
│   │       └── workflows/
│   │           └── ci.yml               # Overrides go/_defaults/ CI if needed
│   ├── cli/
│   │   ├── blueprint.yaml
│   │   └── ...
│   └── operator/
│       ├── blueprint.yaml
│       └── ...
│
├── rust/
│   ├── _defaults/                       # Rust-specific defaults
│   │   ├── rustfmt.toml
│   │   ├── clippy.toml
│   │   └── .github/
│   │       └── workflows/
│   │           └── ci.yml
│   ├── cli/
│   │   ├── blueprint.yaml
│   │   └── ...
│   └── library/
│       ├── blueprint.yaml
│       └── ...
│
├── typescript/
│   ├── _defaults/
│   │   ├── .eslintrc.json
│   │   ├── .prettierrc
│   │   └── tsconfig.base.json
│   ├── api/
│   │   ├── blueprint.yaml
│   │   └── ...
│   └── react-app/
│       ├── blueprint.yaml
│       └── ...
│
├── terraform/
│   └── module/
│       ├── blueprint.yaml
│       └── ...
│
└── kubernetes/
    ├── helm-app/
    │   ├── blueprint.yaml
    │   └── ...
    └── kustomize-app/
        ├── blueprint.yaml
        └── ...
```

### `_defaults/` Inheritance Model

Defaults use a **layered inheritance** model, resolved bottom-up. More specific layers override less specific ones:

```
Resolution order (last wins):

  1. /_defaults/                    ← Registry-wide defaults (every blueprint gets these)
  2. /go/_defaults/                 ← Language-category defaults (all Go blueprints)
  3. /go/api/                       ← Blueprint's own files (highest priority)
```

**Rules:**

- Every file in `_defaults/` is included in the scaffolded project unless the blueprint provides its own version of the same file at the same relative path.
- Category-level `_defaults/` (e.g., `go/_defaults/`) layer on top of the root `_defaults/`, overriding matching paths.
- A blueprint can explicitly **exclude** inherited defaults it doesn't want via `blueprint.yaml`.
- Default files can use `.tmpl` extensions and are rendered with the same template engine and variables as blueprint files.
- All inherited default files are automatically managed files for sync purposes (strategy: `overwrite` unless configured otherwise).

**Example resolution for `go/api`:**

| File | Source | Reason |
|------|--------|--------|
| `.editorconfig` | `/_defaults/.editorconfig` | No override anywhere |
| `renovate.json` | `/_defaults/.github/renovate.json` | No override |
| `.golangci.yml` | `/go/_defaults/.golangci.yml` | Go category override |
| `scripts/lint.sh` | `/go/_defaults/scripts/lint.sh` | Go category overrides root default |
| `.github/workflows/ci.yml` | `/go/api/.github/workflows/ci.yml` | Blueprint-level override |
| `cmd/main.go` | `/go/api/cmd/main.go` | Blueprint-only file |

### Default Exclusions in `blueprint.yaml`

Blueprints can opt out of specific inherited defaults:

```yaml
# blueprint.yaml
defaults:
  exclude:
    - ".pre-commit-config.yaml"       # Don't want pre-commit for this blueprint
    - "scripts/setup.sh"              # Has its own setup process
  override_strategy:
    "renovate.json": merge            # Merge instead of overwrite on sync
```

---

## 4. Remote Tools Manifest

Tools are CLI binaries, linters, formatters, or dev utilities that a blueprint needs but should never be committed to the registry. Instead, they're declared with version pins and download sources, then resolved at scaffold time.

### Tool Declaration

Tools can be declared at three levels, following the same inheritance model:

**Registry-wide** (in `registry.yaml`):

```yaml
# registry.yaml (excerpt)
tools:
  - name: pre-commit
    version: "3.7.0"
    description: "Git pre-commit hook framework"
    source:
      type: github-release
      repo: "pre-commit/pre-commit"
      asset_pattern: "pre-commit-{{version}}-{{os}}-{{arch}}"
    install_path: ".forge/tools/pre-commit"

  - name: actionlint
    version: "1.7.7"
    description: "GitHub Actions workflow linter"
    source:
      type: github-release
      repo: "rhysd/actionlint"
      asset_pattern: "actionlint_{{version}}_{{os}}_{{arch}}.tar.gz"
    install_path: ".forge/tools/actionlint"
```

**Category-level** (in a category `_defaults/tools.yaml`):

```yaml
# go/_defaults/tools.yaml
tools:
  - name: golangci-lint
    version: "1.62.2"
    description: "Go linters aggregator"
    source:
      type: github-release
      repo: "golangci/golangci-lint"
      asset_pattern: "golangci-lint-{{version}}-{{os}}-{{arch}}.tar.gz"
    install_path: ".forge/tools/golangci-lint"

  - name: goreleaser
    version: "2.6.1"
    source:
      type: github-release
      repo: "goreleaser/goreleaser"
      asset_pattern: "goreleaser_{{goos}}_{{goarch}}.tar.gz"
    install_path: ".forge/tools/goreleaser"
```

**Blueprint-level** (in `blueprint.yaml`):

```yaml
# blueprint.yaml (excerpt)
tools:
  - name: buf
    version: "1.50.0"
    description: "Protobuf tooling"
    source:
      type: github-release
      repo: "bufbuild/buf"
      asset_pattern: "buf-{{os}}-{{arch}}"
    install_path: ".forge/tools/buf"
    # Only included when gRPC is enabled
    condition: "{{ .use_grpc }}"

  # Override the registry-wide version of a tool
  - name: golangci-lint
    version: "1.63.0"             # Pin a newer version for this blueprint
```

### Tool Source Types

```yaml
# GitHub Release — most common
source:
  type: github-release
  repo: "owner/repo"
  asset_pattern: "tool-{{version}}-{{os}}-{{arch}}.tar.gz"

# Direct URL
source:
  type: url
  url: "https://example.com/tools/mytool-{{version}}-{{os}}-{{arch}}.tar.gz"

# Go install
source:
  type: go-install
  module: "github.com/owner/tool"        # Uses `go install module@version`

# npm global
source:
  type: npm
  package: "@owner/tool"                  # Uses `npm install -g package@version`

# Cargo install
source:
  type: cargo-install
  crate: "tool-name"                      # Uses `cargo install crate@version`

# Script — run an install script
source:
  type: script
  url: "https://example.com/install.sh"   # Piped to sh with VERSION env var
```

### Platform Resolution Variables

Tool asset patterns support these variables for cross-platform resolution:

| Variable | macOS (ARM) | macOS (Intel) | Linux (x86_64) | Linux (ARM) |
|----------|-------------|---------------|-----------------|-------------|
| `{{os}}` | `darwin` | `darwin` | `linux` | `linux` |
| `{{arch}}` | `arm64` | `amd64` | `amd64` | `arm64` |
| `{{goos}}` | `Darwin` | `Darwin` | `Linux` | `Linux` |
| `{{goarch}}` | `arm64` | `x86_64` | `x86_64` | `arm64` |
| `{{version}}` | (value from `version` field) | | | |

### Tool Lifecycle

```
forge create go/api
  │
  ├─ 1. Resolve inherited + blueprint-specific tools
  ├─ 2. Check local cache (~/.cache/forge/tools/<name>/<version>/)
  ├─ 3. Download missing tools (verify checksums if provided)
  ├─ 4. Place tools in .forge/tools/ within the scaffolded project
  ├─ 5. Add .forge/tools/ to .gitignore
  └─ 6. Record tool versions in .forge-lock.yaml

forge sync (or forge tools update)
  │
  ├─ 1. Read .forge-lock.yaml for current tool versions
  ├─ 2. Fetch latest blueprint tool declarations
  ├─ 3. Compare versions → report updates available
  └─ 4. Download and replace updated tools

forge tools install
  │
  └─ Re-download all declared tools (e.g., after clone, new machine)
```

### `forge tools` Command

```bash
# Install/reinstall all tools declared for this project
forge tools install

# Check for tool version updates from the source blueprint
forge tools check

# Update all tools to blueprint-declared versions
forge tools update

# Update a specific tool
forge tools update golangci-lint

# List tools and their status
forge tools list

# Output:
#   TOOL              VERSION   STATUS      SOURCE
#   pre-commit        3.7.0     installed   github:pre-commit/pre-commit
#   golangci-lint     1.62.2    update →    github:golangci/golangci-lint (1.63.0 available)
#   actionlint        1.7.7     installed   github:rhysd/actionlint
#   buf               1.50.0    installed   github:bufbuild/buf
```

### Tool References in Scripts and Configs

Blueprints can reference tool paths in their templates:

```yaml
# In a Makefile.tmpl
lint:
 .forge/tools/golangci-lint run ./...

# In a CI workflow template
- run: .forge/tools/actionlint
```

Or via a generated wrapper that `forge create` places in the project:

```bash
# .forge/bin/golangci-lint (auto-generated, added to PATH in scripts)
#!/bin/sh
exec "$(dirname "$0")/../tools/golangci-lint" "$@"
```

---

## 5. `registry.yaml` — Full Schema

```yaml
# registry.yaml — Blueprint registry index
apiVersion: v1
name: "acme-blueprints"
description: "ACME Corp standard project blueprints"
maintainers:
  - name: "Platform Engineering"
    email: "platform@acme.com"

# Registry-wide defaults config
defaults:
  sync_strategy: overwrite          # Default sync strategy for inherited files
  managed: true                     # All default files are managed by default

# Registry-wide tool declarations
tools:
  - name: pre-commit
    version: "3.7.0"
    source:
      type: github-release
      repo: "pre-commit/pre-commit"
      asset_pattern: "pre-commit-{{version}}-{{os}}-{{arch}}"
    install_path: ".forge/tools/pre-commit"
    checksum:
      sha256:
        darwin-arm64: "abc123..."
        darwin-amd64: "def456..."
        linux-amd64: "789ghi..."

  - name: actionlint
    version: "1.7.7"
    source:
      type: github-release
      repo: "rhysd/actionlint"
      asset_pattern: "actionlint_{{version}}_{{os}}_{{arch}}.tar.gz"
    install_path: ".forge/tools/actionlint"

# Blueprint catalog
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

  - name: go/operator
    path: go/operator
    description: "Kubernetes operator with controller-runtime"
    version: "1.0.0"
    tags: ["go", "kubernetes", "operator"]
    latest_commit: "def789abc012"

  - name: rust/cli
    path: rust/cli
    description: "Rust CLI with clap, CI, and cross-compilation"
    version: "1.2.0"
    tags: ["rust", "cli", "clap"]
    latest_commit: "789abc012def"

  - name: rust/library
    path: rust/library
    description: "Rust library crate with benchmarks and docs"
    version: "1.0.0"
    tags: ["rust", "library", "crate"]
    latest_commit: "789abc012def"

  - name: typescript/api
    path: typescript/api
    description: "TypeScript REST API with Express/Fastify and Prisma"
    version: "1.1.0"
    tags: ["typescript", "api", "node"]
    latest_commit: "345ghi678jkl"

  - name: typescript/react-app
    path: typescript/react-app
    description: "React application with Vite, TailwindCSS, and testing"
    version: "1.0.0"
    tags: ["typescript", "react", "frontend"]
    latest_commit: "345ghi678jkl"

  - name: terraform/module
    path: terraform/module
    description: "Terraform module with docs generation and examples"
    version: "1.3.0"
    tags: ["terraform", "iac", "module"]
    latest_commit: "901mno234pqr"

  - name: kubernetes/helm-app
    path: kubernetes/helm-app
    description: "Helm chart with values schema, tests, and RBAC"
    version: "1.0.0"
    tags: ["kubernetes", "helm", "chart"]
    latest_commit: "567stu890vwx"

  - name: kubernetes/kustomize-app
    path: kubernetes/kustomize-app
    description: "Kustomize-based Kubernetes deployment with overlays"
    version: "1.0.0"
    tags: ["kubernetes", "kustomize"]
    latest_commit: "567stu890vwx"
```

---

## 6. `blueprint.yaml` — Full Schema

```yaml
# blueprint.yaml — Blueprint configuration
apiVersion: v1
name: "go-api"
description: "Production Go API service with HTTP/gRPC, observability, and Docker"
version: "2.1.0"
tags: ["go", "api", "grpc", "docker"]

# Control which defaults are inherited
defaults:
  exclude:                              # Opt out of specific inherited defaults
    - ".pre-commit-config.yaml"
    - "scripts/setup.sh"
  override_strategy:                    # Change sync strategy for specific defaults
    "renovate.json": merge

# Variables prompted during scaffolding
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
    description: "Include gRPC support?"
    type: bool
    default: false

  - name: ci_provider
    description: "CI/CD provider"
    type: choice
    choices: ["github-actions", "gitlab-ci", "none"]
    default: "github-actions"

  - name: license
    description: "License type"
    type: choice
    choices: ["MIT", "Apache-2.0", "BSD-3-Clause", "none"]
    default: "Apache-2.0"

# Conditional file inclusion
conditions:
  - when: "{{ not .use_grpc }}"
    exclude:
      - "proto/"
      - "internal/grpc/"
  - when: "{{ eq .ci_provider \"none\" }}"
    exclude:
      - ".github/"
      - ".gitlab-ci.yml"

# Blueprint-specific tools (adds to or overrides inherited tools)
tools:
  - name: buf
    version: "1.50.0"
    source:
      type: github-release
      repo: "bufbuild/buf"
      asset_pattern: "buf-{{os}}-{{arch}}"
    install_path: ".forge/tools/buf"
    condition: "{{ .use_grpc }}"

# Hooks — run commands before/after scaffolding
hooks:
  post_create:
    - "git init"
    - "go mod tidy"
    - "forge tools install"             # Install declared tools

# Sync-managed files (beyond auto-managed defaults)
sync:
  managed_files:
    - path: "Makefile"
      strategy: merge
  ignore:
    - "*.generated.go"
    - "vendor/"

# Directory/file renaming
rename:
  "{{project_name}}/": "."
```

---

## 7. `_defaults/` Deep Dive — What Ships by Default

Here's what a well-stocked `_defaults/` directory looks like for an enterprise registry:

```
_defaults/
├── .editorconfig                       # Universal editor config
├── .gitignore.tmpl                     # Base gitignore (rendered per-language)
├── .gitattributes                      # LF normalization, binary detection
├── .pre-commit-config.yaml             # Pre-commit hooks
├── .github/
│   ├── dependabot.yml                  # Dependabot config
│   ├── renovate.json                   # Renovate config (alternative to dependabot)
│   ├── CODEOWNERS.tmpl                 # Templated with team/org variables
│   ├── PULL_REQUEST_TEMPLATE.md
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.md
│   │   └── feature_request.md
│   └── workflows/
│       ├── forge-check.yml             # Weekly blueprint drift detection
│       └── pr-lint.yml                 # PR title/commit message linting
├── scripts/
│   ├── setup.sh                        # Dev environment setup
│   ├── lint.sh                         # Language-agnostic lint wrapper
│   └── ci-common.sh                    # Shared CI helper functions
├── LICENSE.tmpl                        # Rendered with license type variable
├── SECURITY.md                         # Security policy
└── .forge/
    └── .gitignore                      # Ensures .forge/tools/ is git-ignored
```

**Language category `_defaults/` examples:**

```
go/_defaults/
├── .golangci.yml
├── .github/workflows/ci.yml
├── .goreleaser.yml.tmpl
├── Dockerfile.tmpl
└── scripts/
    └── lint.sh                         # Go-specific: runs golangci-lint

rust/_defaults/
├── rustfmt.toml
├── clippy.toml
├── .github/workflows/ci.yml
├── Dockerfile.tmpl
├── deny.toml                           # cargo-deny config
└── scripts/
    └── lint.sh                         # Rust-specific: runs clippy + fmt

typescript/_defaults/
├── .eslintrc.json
├── .prettierrc
├── tsconfig.base.json
├── .github/workflows/ci.yml
└── scripts/
    └── lint.sh                         # TS-specific: runs eslint + prettier
```

---

## 8. CLI Commands — Detailed Design

### 8.1 `forge create`

Scaffold a new project from a blueprint.

```bash
# From default registry
forge create go/api

# With overrides
forge create go/api \
  --set project_name=my-api \
  --set use_grpc=true \
  --output-dir ./my-api

# Specific version
forge create go/api@v2.1.0

# From explicit registry URL
forge create https://github.com/acme/forge-blueprints//go/api

# Non-interactive (all defaults)
forge create go/api --defaults

# Skip tool installation
forge create go/api --no-tools

# Direct standalone repo (no registry)
forge create git@github.com:someone/standalone-blueprint.git
```

**Flow:**

1. Resolve the blueprint: parse registry + path, fetch `registry.yaml` (cached), validate blueprint exists.
2. Sparse-checkout the blueprint directory + applicable `_defaults/` directories from the registry.
3. Parse and validate `blueprint.yaml`.
4. Prompt the user for each variable.
5. **Resolve defaults inheritance:** merge `/_defaults/` → `/go/_defaults/` → `/go/api/` (blueprint wins).
6. Apply exclusions from `blueprint.yaml` `defaults.exclude`.
7. Evaluate conditions → build final file inclusion/exclusion list.
8. Render all `.tmpl` files through Go's `text/template` engine.
9. Write rendered files to the output directory.
10. **Resolve tools:** merge inherited + blueprint tools, evaluate conditions, download and install.
11. Write `.forge-lock.yaml` (including tool versions and default file provenance).
12. Execute `post_create` hooks.

### 8.2 `forge init`

Initialize a new blueprint within a registry or standalone.

```bash
forge init                                  # New blueprint in current dir
forge init go/grpc-gateway --registry .     # Within a registry repo
forge init --from ./existing-project        # Reverse-engineer from existing
```

### 8.3 `forge sync`

Pull updated managed files and defaults from the source blueprint.

```bash
forge sync                          # Sync all managed files + defaults
forge sync --file .golangci.yml     # Sync a specific file
forge sync --dry-run                # Preview changes
forge sync --force                  # Skip confirmation
forge sync --include-tools          # Also update tools
```

### 8.4 `forge check`

Check if the source blueprint has updates for managed files, defaults, or tools.

```bash
forge check
forge check --output json          # For CI
```

**Output Example:**

```
Blueprint:  acme://go/api
Current:    abc1234 (v2.1.0 — 2025-01-15)
Latest:     def5678 (v2.2.0 — 2025-02-01)

Default files with updates:
  ✱ .github/renovate.json           (modified 2025-01-20) [from: _defaults]
  ✱ .golangci.yml                   (modified 2025-02-01) [from: go/_defaults]
  ✓ .editorconfig                   (up to date)          [from: _defaults]

Managed files with updates:
  ✱ .github/workflows/ci.yml        (modified 2025-01-28)
  ✓ Makefile                        (up to date)

Tools with updates:
  ✱ golangci-lint                   1.62.2 → 1.63.0
  ✓ pre-commit                     3.7.0 (up to date)
  ✓ buf                            1.50.0 (up to date)

Run `forge sync` to apply file updates.
Run `forge tools update` to update tools.
```

### 8.5 `forge tools`

Manage remote tools for the current project.

```bash
forge tools install                 # Install all declared tools
forge tools update                  # Update to blueprint-declared versions
forge tools update golangci-lint    # Update specific tool
forge tools check                   # Check for updates
forge tools list                    # List tools and status
```

### 8.6 `forge list` / `forge search` / `forge info`

```bash
forge list                          # List all blueprints
forge list --tag go                 # Filter by tag
forge search "api"                  # Search across registries
forge info go/api                   # Show blueprint details + inherited defaults + tools
```

---

## 9. `.forge-lock.yaml` — Updated Schema

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
  use_grpc: true
  ci_provider: "github-actions"
  license: "Apache-2.0"

# Tracks where each default file came from
defaults:
  - path: ".editorconfig"
    source: "_defaults/.editorconfig"
    strategy: overwrite
    synced_commit: "abc123def456"
  - path: ".github/renovate.json"
    source: "_defaults/.github/renovate.json"
    strategy: overwrite
    synced_commit: "abc123def456"
  - path: ".golangci.yml"
    source: "go/_defaults/.golangci.yml"
    strategy: overwrite
    synced_commit: "abc123def456"
  - path: ".github/workflows/ci.yml"
    source: "go/api/.github/workflows/ci.yml"
    strategy: overwrite
    synced_commit: "abc123def456"

managed_files:
  - path: "Makefile"
    strategy: merge
    synced_commit: "abc123def456"

tools:
  - name: pre-commit
    version: "3.7.0"
    source: "registry"
  - name: golangci-lint
    version: "1.62.2"
    source: "go/_defaults"
  - name: buf
    version: "1.50.0"
    source: "blueprint"
```

---

## 10. Template Rendering Engine

### Template Syntax

Uses Go's `text/template` with a custom function map:

```
{{ .project_name }}              # Variable substitution
{{ .project_name | snakeCase }}  # Filter/pipe
{{ if .use_grpc }}...{{ end }}   # Conditionals
{{ range .services }}...{{ end}} # Iteration
```

### Built-in Template Functions

| Function | Example | Output |
|----------|---------|--------|
| `snakeCase` | `{{ "MyProject" \| snakeCase }}` | `my_project` |
| `camelCase` | `{{ "my-project" \| camelCase }}` | `myProject` |
| `pascalCase` | `{{ "my-project" \| pascalCase }}` | `MyProject` |
| `kebabCase` | `{{ "MyProject" \| kebabCase }}` | `my-project` |
| `upper` | `{{ "hello" \| upper }}` | `HELLO` |
| `lower` | `{{ "HELLO" \| lower }}` | `hello` |
| `title` | `{{ "hello world" \| title }}` | `Hello World` |
| `replace` | `{{ "foo-bar" \| replace "-" "_" }}` | `foo_bar` |
| `trimPrefix` | `{{ "v1.2.3" \| trimPrefix "v" }}` | `1.2.3` |
| `now` | `{{ now "2006" }}` | `2025` |
| `env` | `{{ env "USER" }}` | `donald` |

### File and Directory Name Templating

Files ending in `.tmpl` are rendered and have the extension stripped. Files without `.tmpl` are copied verbatim.

---

## 11. Sync Engine — Detailed Design

### Sync Strategies

**`overwrite`** — Fully replace the local file with the blueprint/default version. Used for files that should always match the source.

**`merge`** — Three-way merge using the last-synced version as the common ancestor. Conflicts are written as standard merge conflict markers.

### What Gets Synced

1. **Inherited default files** — automatically managed, synced from their source layer (`_defaults/`, category `_defaults/`, or blueprint).
2. **Blueprint-declared managed files** — explicitly declared in `blueprint.yaml` `sync.managed_files`.
3. **Tools** — version-pinned remote tools (via `forge tools update` or `forge sync --include-tools`).

### CI Integration

```yaml
# .github/workflows/forge-check.yml (shipped in _defaults)
name: Blueprint Drift Check
on:
  schedule:
    - cron: '0 9 * * 1'
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: go install github.com/acme/forge@latest
      - run: forge check --output json
```

---

## 12. Development Phases

### Phase 1: Foundation (Weeks 1–3)

**Goal:** Core scaffolding with registry and defaults inheritance working end-to-end.

| Task | Est. | Priority |
|------|------|----------|
| Project setup (Go 1.25.4, Cobra, CI, linting) | 2d | P0 |
| `blueprint.yaml` schema definition and parser | 3d | P0 |
| `registry.yaml` schema and index parser | 2d | P0 |
| Git client with sparse checkout support | 3d | P0 |
| Blueprint resolution — registry + path + version parsing | 2d | P0 |
| `_defaults/` inheritance resolver (layered merge) | 3d | P0 |
| Template rendering engine with function map | 3d | P0 |
| Interactive prompting for variables | 2d | P0 |
| `forge create` command — full flow with defaults inheritance | 3d | P0 |
| `.forge-lock.yaml` generation with default provenance tracking | 1d | P0 |

**Milestone:** `forge create go/api` resolves from a registry, inherits defaults, and produces a working project.

### Phase 2: Registry, Authoring & Tools (Weeks 4–6)

**Goal:** Full registry workflow, blueprint authoring, and remote tool management.

| Task | Est. | Priority |
|------|------|----------|
| `forge list` — list blueprints from registry index | 2d | P0 |
| `forge search` — search by name and tags | 1d | P0 |
| `forge info` — show blueprint details + inherited defaults + tools | 1d | P1 |
| Global config and multi-registry support | 2d | P0 |
| Registry caching with configurable TTL | 2d | P0 |
| `forge init` — generate blueprint + update registry index | 2d | P0 |
| `forge validate` — lint blueprint config | 2d | P1 |
| Conditional file inclusion/exclusion | 2d | P0 |
| Post-create hooks | 1d | P1 |
| Tool manifest parser and inheritance resolution | 2d | P0 |
| Tool downloader — GitHub releases, go install, npm, cargo, URL, script | 3d | P0 |
| Platform/arch detection and asset pattern resolution | 1d | P0 |
| Tool caching (`~/.cache/forge/tools/`) | 1d | P0 |
| `forge tools install/list/check/update` commands | 2d | P0 |
| Checksum verification for downloaded tools | 1d | P1 |

**Milestone:** Full blueprint authoring, registry browsing, and tool management working.

### Phase 3: Sync Engine (Weeks 7–9)

**Goal:** Managed file sync, defaults sync, and drift detection.

| Task | Est. | Priority |
|------|------|----------|
| Sync manifest handling (defaults auto-managed + explicit managed files) | 2d | P0 |
| `forge check` — diff defaults, managed files, and tools against source | 3d | P0 |
| `forge sync` with overwrite strategy | 2d | P0 |
| `forge sync` with 3-way merge strategy | 4d | P0 |
| Defaults sync — track provenance layer, detect upstream changes | 2d | P0 |
| `--dry-run` and `--force` flags | 1d | P0 |
| Conflict resolution UX (markers + interactive prompts) | 3d | P1 |
| `.forge-lock.yaml` update on sync (defaults + tools + managed files) | 1d | P0 |

**Milestone:** Full sync lifecycle — check → sync → resolve, including defaults and tools.

### Phase 4: Polish & Release (Weeks 10–11)

**Goal:** Production-ready v0.1.0.

| Task | Est. | Priority |
|------|------|----------|
| Comprehensive test suite (unit + integration with test registry) | 4d | P0 |
| Error handling, UX messaging, colored output | 2d | P1 |
| Docs — README, blueprint authoring guide, registry setup guide, tools guide | 3d | P0 |
| Homebrew formula / goreleaser setup | 1d | P1 |
| `forge cache clean` command | 1d | P1 |
| Reference registry repo with all starter blueprints and defaults | 3d | P0 |

**Milestone:** Tagged v0.1.0 release with binaries, docs, and a reference registry.

---

## 13. Key Dependencies

| Dependency | Purpose | Version |
|------------|---------|---------|
| Go | Language runtime | 1.25.4 |
| `github.com/spf13/cobra` | CLI framework | latest |
| `github.com/spf13/viper` | Config file handling | latest |
| `gopkg.in/yaml.v3` | YAML parsing | latest |
| `github.com/go-git/go-git/v5` | Pure Go git client | latest |
| `github.com/charmbracelet/huh` | Interactive prompts | latest |
| `github.com/charmbracelet/lipgloss` | Terminal styling | latest |
| `github.com/olekukonez/tablewriter` | CLI table output | latest |
| `github.com/sergi/go-diff` | Diff computation for sync | latest |
| `github.com/stretchr/testify` | Test assertions | latest |

---

## 14. Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|-----------|
| Defaults inheritance complexity (3 layers) | High | Strict "last wins" rule. `forge info` shows full resolution for debugging. Extensive tests for edge cases. |
| 3-way merge complexity for diverse file types | High | Start with overwrite-only; add merge incrementally. |
| Tool download reliability across platforms | Medium | Cache aggressively. Support multiple source types. Checksum verification. Graceful fallback with clear error messages. |
| Sparse checkout performance on large registries | Medium | Cache index and tool manifests. Only checkout blueprint path + relevant `_defaults/`. |
| Private repo authentication | Medium | Lean on system git credential helpers. Don't reinvent auth. |
| Template rendering breaking non-text files | Medium | Only render `.tmpl` files; copy everything else verbatim. |
| Registry index drift | Low | Provide `forge registry update` or CI action that regenerates the index. |

---

## 15. Success Criteria

**v0.1.0 is successful when:**

1. `forge create go/api` resolves from a registry, inherits all default files, and produces a working project.
2. Default files from `_defaults/` and `go/_defaults/` are correctly merged with blueprint-level overrides winning.
3. Blueprints can exclude specific defaults via `defaults.exclude`.
4. `forge tools install` downloads the correct platform-specific binaries for all declared tools.
5. `forge check` detects updates to defaults, managed files, and tool versions.
6. `forge sync` updates managed files and defaults with overwrite and merge strategies.
7. `forge tools update` upgrades tools to blueprint-declared versions.
8. End-to-end test suite passes with the reference registry.

---

## 16. Future Considerations (Post-MVP)

- **Blueprint composition** — layering multiple blueprints (base + language + CI).
- **Interactive TUI** — terminal UI for browsing registries and configuring variables.
- **`forge upgrade`** — re-scaffold entire project against a new blueprint version.
- **Plugin/extension system** — custom renderers, validators, post-processors.
- **Semver constraints** — `forge create go/api@^2.0.0`.
- **Registry CI automation** — GitHub Action to auto-generate `registry.yaml` and validate all blueprints on PR.
- **`forge diff`** — show what would change if re-scaffolded from scratch.
- **Tool auto-update bot** — PR automation that bumps tool versions in the registry when new releases are detected.
