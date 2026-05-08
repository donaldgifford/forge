---
id: DESIGN-0001
title: "Blueprint Authoring"
status: Implemented
author: Donald Gifford
created: 2026-05-07
---
<!-- markdownlint-disable-file MD025 MD041 -->

# DESIGN 0001: Blueprint Authoring

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
  - [blueprint.yaml Schema](#blueprintyaml-schema)
  - [Variables](#variables)
  - [Template Files](#template-files)
  - [Conditions](#conditions)
  - [Hooks](#hooks)
  - [Managed Files](#managed-files)
  - [Defaults Inheritance](#defaults-inheritance)
- [API / Interface Changes](#api--interface-changes)
- [Data Model](#data-model)
- [Testing Strategy](#testing-strategy)
- [Migration / Rollout Plan](#migration--rollout-plan)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Overview

This document specifies the contract for authoring a forge blueprint: the
directory layout, the `blueprint.yaml` schema, variable types, conditions,
hooks, and managed-file/sync behavior. It is the reference for anyone
publishing or maintaining a blueprint inside a forge registry.

## Goals and Non-Goals

### Goals

- Define a single, versioned schema (`apiVersion: v1`) for `blueprint.yaml`.
- Specify the runtime semantics of variables, conditions, hooks, and sync
  strategies.
- Document the file layout convention (`.tmpl` extension, templated
  directory names, `_defaults/` inheritance hooks).

### Non-Goals

- Registry-wide layout (covered by DESIGN-0002).
- Template engine internals (covered by `internal/template/`).
- The `forge create` orchestration flow (covered by RFC-0001 and the
  internal create package).

## Background

A blueprint is a project template that consists of a `blueprint.yaml`
configuration file and a directory of template files. Blueprints live
inside a registry (see DESIGN-0002) and inherit shared files from
`_defaults/` directories at registry-root and category levels.

## Detailed Design

### Directory Structure

```
my-blueprint/
  blueprint.yaml       # Blueprint configuration
  go.mod.tmpl          # Templated file (.tmpl extension)
  main.go.tmpl
  Makefile             # Static file (copied as-is)
  README.md.tmpl
```

Files with a `.tmpl` extension are rendered using Go `text/template`. The
extension is stripped in the output. Files without `.tmpl` are copied
verbatim.

Directory and file names support `{{project_name}}`-style substitution —
e.g., `{{project_name}}/cmd/main.go.tmpl` renders to
`my-api/cmd/main.go` once `project_name=my-api` is bound.

### `blueprint.yaml` Schema

```yaml
apiVersion: v1
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
  - name: go_module
    type: string
    description: Go module path
    required: true
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
  - when: '{{ eq .license "none" }}'
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
```

### Variables

Variables are collected from users during `forge create`. Each variable
has the following fields:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Variable name, used in templates as `{{ .name }}` |
| `type` | yes | One of: `string`, `bool`, `choice`, `int` |
| `description` | no | Shown during interactive prompts |
| `default` | no | Default value if user doesn't provide one |
| `required` | no | If true, user must provide a value |
| `validate` | no | Regex pattern for validation |
| `choices` | no | Available options for `choice` type |

Variables can be set via CLI: `forge create my-bp --set project_name=foo
--set use_docker=false`.

The `default` field is itself rendered as a template, so it can reference
earlier variables — e.g., `default: "github.com/{{ .org }}/{{ .project_name }}"`.

### Template Files

Files with `.tmpl` are rendered using Go `text/template`. The renderer
exposes:

- All variables as `{{ .project_name }}`, `{{ .go_module }}`, etc.
- Custom functions: `snakeCase`, `camelCase`, `pascalCase`, `kebabCase`,
  `upper`, `lower`, `title`, `replace`, `trimPrefix`, `trimSuffix`, `now`,
  `env`, `default`.

Example `go.mod.tmpl`:

```
module {{ .go_module }}

go 1.25.4
```

### Conditions

Conditions allow excluding files based on variable values:

```yaml
conditions:
  - when: '{{ eq .use_docker "false" }}'
    exclude:
      - Dockerfile
      - docker-compose.yml
      - .dockerignore
```

The `when` expression is a Go template that evaluates to `"true"` or
`"false"`. The `exclude` patterns support globs and directory prefixes.

### Hooks

Post-create hooks run after all files are written:

```yaml
hooks:
  post_create:
    - "go mod tidy"
    - "git init"
    - "git add -A"
```

Hooks run in the project directory. If a hook fails, the project files
are still kept.

### Managed Files

Files listed under `sync.managed_files` are tracked for ongoing
synchronization:

- **`overwrite`** — File is replaced entirely on sync.
- **`merge`** — Three-way merge preserves local changes while applying
  upstream updates.

### Defaults Inheritance

Blueprints automatically inherit files from `_defaults/` directories in
the registry. Use `defaults.exclude` to skip specific inherited files.
See [DESIGN-0002 — Registry Layout & Defaults
Inheritance](0002-registry-layout-and-defaults-inheritance.md) for the
full inheritance chain.

## API / Interface Changes

This document specifies the user-facing authoring contract. Changes to
the schema require an `apiVersion` bump and a migration plan.

## Data Model

The on-disk schema is YAML, parsed by `gopkg.in/yaml.v3`. Go struct
definitions live in `internal/config/blueprint.go` (`Blueprint`,
`Variable`, `Condition`, `Hook`, `SyncManifest`, etc.).

## Testing Strategy

- Unit tests over the YAML loader (`internal/config/loader_test.go`) with
  fixtures in `testdata/registry/go/api/blueprint.yaml`.
- Integration tests of `forge create` end-to-end (see
  `internal/create/cli_integration_test.go`).
- Schema validation tests cover required fields, allowed variable types,
  and regex compilability for `validate`.

## Migration / Rollout Plan

The schema is versioned via `apiVersion: v1`. Breaking changes will bump
to `v2` and ship alongside a migration helper.

## Open Questions

- Should `default` field rendering be opt-in to avoid surprising
  behavior when users include `{{` in literal defaults?
- Is `int` type pulling its weight, or should we standardise on `string`
  with `validate: "^\\d+$"`?

## References

- [RFC-0001 — Forge: Project Scaffolding CLI](../rfc/0001-forge-project-scaffolding-cli.md)
- [DESIGN-0002 — Registry Layout & Defaults Inheritance](0002-registry-layout-and-defaults-inheritance.md)
- `internal/config/blueprint.go` — Go struct definitions
- `internal/template/` — template rendering
