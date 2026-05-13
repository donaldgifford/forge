---
id: DESIGN-0001
title: "Blueprint Authoring"
status: Implemented
author: Donald Gifford
created: 2026-05-07
updated: 2026-05-13
---
<!-- markdownlint-disable-file MD025 MD041 -->

# DESIGN 0001: Blueprint Authoring

**Status:** Implemented
**Author:** Donald Gifford
**Date:** 2026-05-07
**Last revised:** 2026-05-13 — config files moved from YAML to HCL
per [DESIGN-0004](0004-unify-config-file-format-after-hcl2-cutover.md).
The `apiVersion` field is gone; the file extension is now the version
signal. The original Go `text/template`-based contract is preserved
for historical reference in
[DESIGN-0003](0003-migrate-template-engine-to-hcl2.md). The decision
record for the engine swap is
[ADR-0001](../adr/0001-use-hcl2-as-the-template-engine.md). Authors
upgrading from v0.2.x or v0.3.x should follow
[docs/MIGRATION.md](../MIGRATION.md).

<!--toc:start-->
- [Overview](#overview)
- [Goals and Non-Goals](#goals-and-non-goals)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Background](#background)
- [Detailed Design](#detailed-design)
  - [Directory Structure](#directory-structure)
  - [blueprint.hcl Schema](#blueprinthcl-schema)
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
the directory layout, the `blueprint.hcl` schema, variable types,
conditions, hooks, and managed-file/sync behavior. It is the
reference for anyone publishing or maintaining a blueprint inside a
forge registry.

The current contract is **`blueprint.hcl`** — HCL2
(`hashicorp/hcl/v2`) for both config files and templates. Older
formats (`apiVersion: v1` text/template, `apiVersion: v2` YAML
configs) are no longer accepted — load-time validation rejects them
with a pointer to the migration tools.

## Goals and Non-Goals

### Goals

- Define the single accepted schema for `blueprint.hcl`.
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

A blueprint is a project template that consists of a `blueprint.hcl`
configuration file and a directory of template files. Blueprints
live inside a registry (see DESIGN-0002) and inherit shared files
from `_defaults/` directories at the registry-root and category
levels.

## Detailed Design

### Directory Structure

```
my-blueprint/
  blueprint.hcl              # Blueprint configuration
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

### `blueprint.hcl` Schema

```hcl
name        = "my-blueprint"
description = "A starter project"
version     = "1.0.0"
tags        = ["go", "api"]

variable "project_name" {
  type        = "string"
  description = "Name of the project"
  required    = true
  validate    = "^[a-z][a-z0-9-]*$"
}

variable "go_module" {
  type        = "string"
  description = "Go module path"
  default     = "github.com/example/${project_name}"
}

variable "use_docker" {
  type        = "bool"
  default     = "true"
  description = "Include Docker support"
}

variable "license" {
  type    = "choice"
  choices = ["MIT", "Apache-2.0", "BSD-3-Clause"]
  default = "MIT"
}

defaults {
  exclude           = [".github/CODEOWNERS"]
  override_strategy = { ".golangci.yml" = "overwrite" }
}

condition {
  when    = license == "none"
  exclude = ["LICENSE*"]
}

hooks {
  post_create = ["go mod tidy", "git init"]
}

sync {
  ignore = ["*.local"]

  managed_file "Makefile" {
    strategy = "merge"
  }
}

rename {
  entry {
    from = "${project_name}/"
    to   = "."
  }
}
```

There is no `apiVersion` field. The file extension (`.hcl` vs the
legacy `.yaml`) is the version signal — see
[DESIGN-0004](0004-unify-config-file-format-after-hcl2-cutover.md).

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

```hcl
condition {
  when    = !use_docker
  exclude = ["Dockerfile", "docker-compose.yml", ".dockerignore"]
}
```

The `when` attribute is an **HCL expression** that must evaluate to
a `bool`. It's parsed at load time, so syntax errors surface with
file/line/column at `forge create` startup rather than on first
evaluation. Examples:

- `!use_docker`
- `license == "none"`
- `replicas > 1`
- `project_name != ""`

The `exclude` patterns support globs and directory prefixes. Multiple
`condition { ... }` blocks are allowed.

### Hooks

Post-create hooks run after all files are written:

```hcl
hooks {
  post_create = ["go mod tidy", "git init", "git add -A"]
}
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
to the schema require a file-format bump and a migration plan. The
most recent bumps were the v1 → v2 template-engine swap (DESIGN-0003)
and the v2 YAML → HCL config-file move
([DESIGN-0004](0004-unify-config-file-format-after-hcl2-cutover.md)).

## Data Model

The on-disk schema is HCL, parsed by `hashicorp/hcl/v2/hcldec` against
declarative specs in `internal/config/hcldec_spec.go`. Go struct
definitions live in `internal/config/blueprint.go` (`Blueprint`,
`Variable`, `Condition`, `Hooks`, `SyncConfig`, etc.) and carry no
encoding-specific tags.

`Condition.When` is an `hcl.Expression` parsed at load time so syntax
errors surface with source location at `LoadBlueprint` time rather
than on first evaluation. The original source text is retained on
`Condition.WhenSource` for diagnostics and migrate-config round-trip.

In-memory variable values are typed as `cty.Value` (from
`zclconf/go-cty`). Conversion between the lockfile YAML scalars on
disk and the cty representation in memory happens in
`internal/lockfile/cty.go` using the declared variable types as the
source of truth.

## Testing Strategy

- Unit tests over the HCL loader
  (`internal/config/loader_hcl_test.go`) with the hermetic fixture
  in `testdata/hcl-registry/`.
- Integration tests of `forge create` end-to-end against
  `testdata/registry/` and `testdata/v2-registry/` (HCL).
- Schema validation tests cover required fields, allowed variable
  types, regex compilability for `validate`, and the YAML-rejection
  path (`TestLoadBlueprint_RejectsBareYAML`,
  `TestLoadBlueprint_RejectsV1Fixture`).

## Migration / Rollout Plan

The schema is versioned via the file extension. The current accepted
file is `blueprint.hcl`. Migration paths:

- v0.3.x (v2 YAML) → v0.4.x (HCL): `forge migrate config --path <registry>`
- v0.2.x (v1 text/template) → v0.4.x (HCL): two steps —
  `forge migrate templates` then `forge migrate config`.

Both are documented in [docs/MIGRATION.md](../MIGRATION.md).

## References

- [RFC-0001 — Forge: Project Scaffolding CLI](../rfc/0001-forge-project-scaffolding-cli.md)
- [DESIGN-0002 — Registry Layout & Defaults Inheritance](0002-registry-layout-and-defaults-inheritance.md)
- [DESIGN-0003 — Migrate template engine to HCL2](0003-migrate-template-engine-to-hcl2.md)
- [DESIGN-0004 — Unify Config File Format After HCL2 Cutover](0004-unify-config-file-format-after-hcl2-cutover.md)
- [ADR-0001 — Use HCL2 as the template engine](../adr/0001-use-hcl2-as-the-template-engine.md)
- [docs/MIGRATION.md](../MIGRATION.md) — v0.2.x/v0.3.x → v0.4.x migration guides.
- `internal/config/blueprint.go` — Go struct definitions.
- `internal/config/hcldec_spec.go` — HCL decoding schemas.
- `internal/template/` — template rendering.
