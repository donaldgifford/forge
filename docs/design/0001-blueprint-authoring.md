---
id: DESIGN-0001
title: "Blueprint Authoring"
status: Implemented
author: Donald Gifford
created: 2026-05-07
updated: 2026-05-08
---
<!-- markdownlint-disable-file MD025 MD041 -->

# DESIGN 0001: Blueprint Authoring

**Status:** Implemented
**Author:** Donald Gifford
**Date:** 2026-05-07
**Last revised:** 2026-05-08 — rewritten for `apiVersion: v2` (HCL2).
The original Go `text/template`-based contract is preserved for
historical reference in
[DESIGN-0003](0003-migrate-template-engine-to-hcl2.md). The decision
record for the engine swap is
[ADR-0001](../adr/0001-use-hcl2-as-the-template-engine.md). Authors
upgrading from v1 should follow [docs/MIGRATION.md](../MIGRATION.md).

<!--toc:start-->
- [Overview](#overview)
- [Goals and Non-Goals](#goals-and-non-goals)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Background](#background)
- [Detailed Design](#detailed-design)
  - [Directory Structure](#directory-structure)
  - [blueprint.yaml Schema](#blueprintyaml-schema)
  - [Variables](#variables)
  - [Template Files](#template-files)
    - [Why HCL2](#why-hcl2)
  - [Conditions](#conditions)
  - [Hooks](#hooks)
  - [Managed Files](#managed-files)
  - [Defaults Inheritance](#defaults-inheritance)
- [API / Interface Changes](#api--interface-changes)
- [Data Model](#data-model)
- [Testing Strategy](#testing-strategy)
- [Migration / Rollout Plan](#migration--rollout-plan)
- [References](#references)
<!--toc:end-->

## Overview

This document specifies the contract for authoring a forge blueprint:
the directory layout, the `blueprint.yaml` schema, variable types,
conditions, hooks, and managed-file/sync behavior. It is the
reference for anyone publishing or maintaining a blueprint inside a
forge registry.

The current contract is **`apiVersion: v2`**, backed by HCL2
(`hashicorp/hcl/v2`). The v1 (`text/template`) contract is no longer
accepted — load-time validation rejects v1 blueprints with a pointer
to the migration tool.

## Goals and Non-Goals

### Goals

- Define a single, versioned schema (`apiVersion: v2`) for
  `blueprint.yaml`.
- Specify the runtime semantics of variables, conditions, hooks, and
  sync strategies under HCL2 templating.
- Document the file layout convention (`.tmpl` extension, templated
  directory names, `_defaults/` inheritance hooks).

### Non-Goals

- Registry-wide layout (covered by DESIGN-0002).
- Template engine internals (covered by `internal/template/` and
  DESIGN-0003).
- The `forge create` orchestration flow (covered by RFC-0001 and the
  internal create package).

## Background

A blueprint is a project template that consists of a `blueprint.yaml`
configuration file and a directory of template files. Blueprints
live inside a registry (see DESIGN-0002) and inherit shared files
from `_defaults/` directories at the registry-root and category
levels.

## Detailed Design

### Directory Structure

```
my-blueprint/
  blueprint.yaml             # Blueprint configuration
  ${project_name}/           # Templated directory name
    go.mod.tmpl              # Templated file (.tmpl extension)
    cmd/main.go.tmpl
    Makefile                 # Static file (copied as-is)
    README.md.tmpl
```

Files with a `.tmpl` extension are rendered through the HCL2 engine.
The extension is stripped in the output. Files without `.tmpl` are
copied verbatim — they may still contain `{{ }}` content because the
HCL2 engine never inspects them.

Directory and file names support `${project_name}`-style
interpolation — e.g., `${project_name}/cmd/main.go.tmpl` renders to
`my-api/cmd/main.go` once `project_name = "my-api"` is bound.

### `blueprint.yaml` Schema

```yaml
apiVersion: v2
name: my-blueprint
description: A starter project
version: 1.0.0
tags:
  - go
  - api

variables:
  - name: project_name
    type: string
    description: Name of the project
    required: true
    validate: "^[a-z][a-z0-9-]*$"
  - name: go_module
    type: string
    description: Go module path
    default: "github.com/example/${project_name}"
  - name: use_docker
    type: bool
    default: "true"
    description: Include Docker support
  - name: license
    type: choice
    choices:
      - MIT
      - Apache-2.0
      - BSD-3-Clause
    default: MIT

defaults:
  exclude:
    - ".github/CODEOWNERS"
  override_strategy:
    ".golangci.yml": overwrite

conditions:
  - when: 'license == "none"'
    exclude:
      - LICENSE*

hooks:
  post_create:
    - "go mod tidy"
    - "git init"

sync:
  managed_files:
    - path: Makefile
      strategy: merge
  ignore:
    - "*.local"

rename:
  "${project_name}/": "."
```

### Variables

Variables are collected from users during `forge create`. Each
variable has the following fields:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Variable name, used in templates as `${name}`. |
| `type` | yes | One of: `string`, `bool`, `choice`, `int`. |
| `description` | no | Shown during interactive prompts. |
| `default` | no | Default value if user doesn't provide one. |
| `required` | no | If true, user must provide a value. |
| `validate` | no | Regex pattern for validation. |
| `choices` | no | Available options for `choice` type. |

Variables can be set via CLI: `forge create my-bp --set
project_name=foo --set use_docker=false`.

The `default` field is itself rendered through the HCL2 engine, so
it can reference earlier variables — e.g.,
`default: "github.com/${org}/${project_name}"`.

### Template Files

Files with `.tmpl` are rendered through HCL2. The renderer exposes:

- **Variables** as `${project_name}`, `${go_module}`, etc.
- **Custom functions:** `snakeCase`, `camelCase`, `pascalCase`,
  `kebabCase`, `now(layout)`, `env(key)`.
- **Standard library functions** from `cty/function/stdlib`: `upper`,
  `lower`, `title`, `replace(s, old, new)`, `trimPrefix(s, prefix)`,
  `trimSuffix(s, suffix)`, `coalesce(a, b)` (replaces v1 `default`).

Example `go.mod.tmpl`:

```
module ${go_module}

go 1.26.2
```

Conditional content uses HCL's directive syntax:

```
%{ if use_docker ~}
# Docker support enabled
COPY Dockerfile .
%{ endif ~}
```

The `~` after the directive trims surrounding whitespace, mirroring
v1's `{{- … -}}` semantics.

#### Why HCL2

HCL2's `${expr}` and `%{ … ~}` delimiters do not collide with the
`{{ }}` syntax used by Helm, Argo CD, kustomize, or any of the
Hashicorp config DSLs. A blueprint that scaffolds a Helm chart
contains verbatim `{{ .Values.replicas }}` lines that pass through
the renderer untouched. See
[ADR-0001](../adr/0001-use-hcl2-as-the-template-engine.md) for the
full rationale.

### Conditions

Conditions allow excluding files based on variable values:

```yaml
conditions:
  - when: '!use_docker'
    exclude:
      - Dockerfile
      - docker-compose.yml
      - .dockerignore
```

The `when` field is a **bare HCL expression** that must evaluate to
a `bool`. Examples:

- `!use_docker`
- `license == "none"`
- `replicas > 1`
- `project_name != ""`

The `exclude` patterns support globs and directory prefixes.

### Hooks

Post-create hooks run after all files are written:

```yaml
hooks:
  post_create:
    - "go mod tidy"
    - "git init"
    - "git add -A"
```

Hooks run in the project directory. If a hook fails, the project
files are still kept.

### Managed Files

Files listed under `sync.managed_files` are tracked for ongoing
synchronization:

- **`overwrite`** — File is replaced entirely on sync.
- **`merge`** — Three-way merge preserves local changes while
  applying upstream updates.

### Defaults Inheritance

Blueprints automatically inherit files from `_defaults/` directories
in the registry. Use `defaults.exclude` to skip specific inherited
files. See [DESIGN-0002 — Registry Layout & Defaults
Inheritance](0002-registry-layout-and-defaults-inheritance.md) for
the full inheritance chain.

## API / Interface Changes

This document specifies the user-facing authoring contract. Changes
to the schema require an `apiVersion` bump and a migration plan.
The most recent bump (v1 → v2) is documented in
[DESIGN-0003](0003-migrate-template-engine-to-hcl2.md).

## Data Model

The on-disk schema is YAML, parsed by `gopkg.in/yaml.v3`. Go struct
definitions live in `internal/config/blueprint.go` (`Blueprint`,
`Variable`, `Condition`, `Hook`, `SyncManifest`, etc.).

In-memory variable values are typed as `cty.Value` (from
`zclconf/go-cty`). Conversion between the YAML scalars on disk and
the cty representation in memory happens in
`internal/lockfile/cty.go` using the declared variable types as the
source of truth.

## Testing Strategy

- Unit tests over the YAML loader
  (`internal/config/loader_test.go`) with fixtures in
  `testdata/registry/go/api/blueprint.yaml`.
- Integration tests of `forge create` end-to-end (see
  `internal/create/cli_integration_test.go`).
- Schema validation tests cover required fields, allowed variable
  types, regex compilability for `validate`, and the v1-rejection
  path (`TestLoadBlueprint_RejectsV1Fixture`).

## Migration / Rollout Plan

The schema is versioned via `apiVersion`. The current accepted
version is **v2**. Existing v1 blueprints must be migrated using
`forge migrate templates --path <registry>` — see
[docs/MIGRATION.md](../MIGRATION.md).

Future schema bumps will follow the same pattern: a `forge migrate
…` subcommand, a load-time error pointing at the migration command
and the migration guide, and a frozen pre-bump fixture in
`testdata/` so the rejection path stays under test.

## References

- [RFC-0001 — Forge: Project Scaffolding CLI](../rfc/0001-forge-project-scaffolding-cli.md)
- [DESIGN-0002 — Registry Layout & Defaults Inheritance](0002-registry-layout-and-defaults-inheritance.md)
- [DESIGN-0003 — Migrate template engine to HCL2](0003-migrate-template-engine-to-hcl2.md)
- [ADR-0001 — Use HCL2 as the template engine](../adr/0001-use-hcl2-as-the-template-engine.md)
- [docs/MIGRATION.md](../MIGRATION.md) — v1 → v2 migration guide.
- `internal/config/blueprint.go` — Go struct definitions.
- `internal/template/` — template rendering.
