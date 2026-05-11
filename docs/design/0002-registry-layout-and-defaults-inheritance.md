---
id: DESIGN-0002
title: "Registry Layout and Defaults Inheritance"
status: Implemented
author: Donald Gifford
created: 2026-05-07
---
<!-- markdownlint-disable-file MD025 MD041 -->

# DESIGN 0002: Registry Layout and Defaults Inheritance

**Status:** Implemented
**Author:** Donald Gifford
**Date:** 2026-05-07

<!--toc:start-->
- [Overview](#overview)
- [Goals and Non-Goals](#goals-and-non-goals)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Background](#background)
- [Detailed Design](#detailed-design)
  - [Directory Structure](#directory-structure)
  - [registry.yaml Schema](#registryyaml-schema)
  - [Layered Defaults](#layered-defaults)
  - [Example](#example)
  - [Excluding Defaults](#excluding-defaults)
- [API / Interface Changes](#api--interface-changes)
  - [Adding Blueprints](#adding-blueprints)
  - [Keeping Metadata in Sync](#keeping-metadata-in-sync)
  - [CI Integration](#ci-integration)
- [Data Model](#data-model)
- [Testing Strategy](#testing-strategy)
- [Migration / Rollout Plan](#migration--rollout-plan)
- [Hosting](#hosting)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Overview

This document specifies the registry contract: the directory layout, the
`registry.yaml` index schema, the layered `_defaults/` inheritance model,
and the registry maintenance commands (`forge registry init|blueprint|update`).
It is the reference for anyone publishing or maintaining a forge registry.

## Goals and Non-Goals

### Goals

- Define a single registry layout that supports any number of blueprints
  organised by category.
- Specify the `_defaults/` inheritance chain (registry-wide → category →
  blueprint).
- Document the `registry.yaml` index schema and the lifecycle commands
  that keep it in sync with on-disk state.

### Non-Goals

- Per-blueprint authoring concerns (covered by DESIGN-0001).
- Hosting/auth specifics — handled by `hashicorp/go-getter`.

## Background

A registry is a Git repository containing blueprints and shared
configuration. Forge inherits files automatically through `_defaults/`
directories at the registry root and category levels, so that every
scaffolded project gets a baseline of CI configs, lint configs, license
templates, and standard scripts without each blueprint redeclaring them.

## Detailed Design

### Directory Structure

```
my-registry/
  registry.yaml              # Registry index
  _defaults/                  # Registry-wide defaults
    .editorconfig
    .gitignore
    LICENSE.tmpl
  go/                         # Category: Go projects
    _defaults/                # Category-level defaults
      .golangci.yml
      scripts/
        lint.sh
    api/                      # Blueprint: go/api
      blueprint.yaml
      go.mod.tmpl
      main.go.tmpl
      Makefile
    cli/                      # Blueprint: go/cli
      blueprint.yaml
      ...
  python/                     # Category: Python projects
    _defaults/
      pyproject.toml
    fastapi/
      blueprint.yaml
      ...
```

### `registry.yaml` Schema

```yaml
apiVersion: v1
name: my-registry
description: Company blueprint registry
blueprints:
  - name: go-api
    path: go/api
    description: Go API service with standard tooling
    tags:
      - go
      - api
      - grpc
  - name: go-cli
    path: go/cli
    description: Go CLI application
    tags:
      - go
      - cli
  - name: python-fastapi
    path: python/fastapi
    description: Python FastAPI service
    tags:
      - python
      - api
```

### Layered Defaults

Files are inherited through a three-level chain (last wins):

1. **Registry defaults** (`/_defaults/`) — applied to all blueprints.
2. **Category defaults** (`/<category>/_defaults/`) — applied to all
   blueprints in that category.
3. **Blueprint files** (`/<category>/<blueprint>/`) — blueprint-specific
   files (highest priority).

### Example

Given this registry structure:

```
_defaults/
  .editorconfig          # All projects get this
  scripts/lint.sh        # Generic lint script
go/
  _defaults/
    .golangci.yml        # All Go projects get this
    scripts/lint.sh      # Go-specific (overrides registry default)
  api/
    blueprint.yaml
    Makefile             # API-specific Makefile
```

Running `forge create go/api` produces:

```
my-project/
  .editorconfig          # From /_defaults/
  .golangci.yml          # From /go/_defaults/
  scripts/lint.sh        # From /go/_defaults/ (overrides /_defaults/)
  Makefile               # From /go/api/
```

### Excluding Defaults

Blueprints opt out of inherited files via `blueprint.yaml`:

```yaml
defaults:
  exclude:
    - ".github/CODEOWNERS"
    - "scripts/deploy.sh"
```

## API / Interface Changes

### Adding Blueprints

Use `forge registry blueprint` to scaffold a new blueprint inside a
registry:

```bash
# Positional form (category/name)
forge registry blueprint go/grpc-service \
  --description "gRPC service with protobuf" \
  --tags go,grpc,api \
  --registry-dir ./my-registry

# Flag form
forge registry blueprint \
  --category python --name fastapi \
  --registry-dir ./my-registry
```

This creates:

- `<category>/<name>/blueprint.yaml` — rich starter config with variables,
  hooks, sync, and rename sections.
- `<category>/<name>/{{project_name}}/README.md.tmpl` — starter template.
- `<category>/_defaults/.gitkeep` — category defaults directory (if it
  doesn't already exist).
- An entry appended to `registry.yaml`.

### Keeping Metadata in Sync

When a blueprint is modified (version bump, template changes, etc.), the
`registry.yaml` index can become stale. Use `forge registry update`:

```bash
# Update stale entries in registry.yaml
forge registry update --registry-dir ./my-registry

# Check-only mode (for CI): exits non-zero if stale
forge registry update --check --registry-dir ./my-registry
```

The update command compares each blueprint's `version` from
`blueprint.yaml` and the latest git commit hash against the values in
`registry.yaml`. It reports one of five statuses for each entry:

| Status | Meaning |
|--------|---------|
| `up-to-date` | Registry entry matches blueprint and git state |
| `version-changed` | `blueprint.yaml` version differs from `registry.yaml` |
| `files-changed` | Git commit differs but version is unchanged |
| `both-changed` | Both version and git commit differ |
| `missing` | Blueprint path does not exist on disk (skipped) |

### CI Integration

```yaml
# GitHub Actions example
- name: Check registry metadata
  run: forge registry update --check
```

## Data Model

The on-disk schema is YAML, parsed by `gopkg.in/yaml.v3`. Go struct
definitions live in `internal/config/registry.go` (`Registry`,
`BlueprintEntry`).

## Testing Strategy

- Unit tests for the inheritance resolver in
  `internal/defaults/resolver_test.go` against `testdata/registry/`.
- Integration tests for `forge registry init|blueprint|update` in
  `internal/registrycmd/`.
- `--check` mode exit-code semantics validated via CLI integration tests.

## Migration / Rollout Plan

Schema versioned via `apiVersion: v1`. Breaking changes bump to `v2`.

## Hosting

Registries are standard Git repositories. Host them on:

- GitHub
- GitLab
- Bitbucket
- Any Git server

Forge uses [go-getter](https://github.com/hashicorp/go-getter) for
fetching, supporting Git, HTTP, and other protocols.

## Open Questions

- Should `forge registry update` auto-bump versions when only files
  change, or always require the author to bump manually?
- Is there a need for nested categories (e.g. `cloud/aws/lambda`)?
  Currently only one category level is supported.

## References

- [RFC-0001 — Forge: Project Scaffolding CLI](../rfc/0001-forge-project-scaffolding-cli.md)
- [DESIGN-0001 — Blueprint Authoring](0001-blueprint-authoring.md)
- [PLAN-0001 — Registry Blueprint & Update Commands](../plan/0001-registry-blueprint-and-update-commands.md)
- [IMPL-0003 — Registry Commands Implementation](../impl/0003-registry-commands.md)
- [hashicorp/go-getter](https://github.com/hashicorp/go-getter) — source
  fetching
