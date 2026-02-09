# Blueprint Authoring Guide

A blueprint is a project template consisting of a `blueprint.yaml` configuration file and a directory of template files.

## Directory Structure

```
my-blueprint/
  blueprint.yaml       # Blueprint configuration
  go.mod.tmpl          # Templated file (.tmpl extension)
  main.go.tmpl
  Makefile              # Static file (copied as-is)
  README.md.tmpl
```

## blueprint.yaml

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

tools:
  - name: golangci-lint
    version: v1.60.0
    source:
      type: github-release
      repo: golangci/golangci-lint
      asset_pattern: "golangci-lint-{{version}}-{{os}}-{{arch}}.tar.gz"

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

## Variables

Variables are collected from users during `forge create`. Each variable has:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Variable name, used in templates as `{{ .name }}` |
| `type` | yes | One of: `string`, `bool`, `choice`, `int` |
| `description` | no | Shown during interactive prompts |
| `default` | no | Default value if user doesn't provide one |
| `required` | no | If true, user must provide a value |
| `validate` | no | Regex pattern for validation |
| `choices` | no | Available options for `choice` type |

Variables can be set via CLI: `forge create my-bp --set project_name=foo --set use_docker=false`

## Template Files

Files with a `.tmpl` extension are rendered using Go `text/template`. The extension is stripped in the output.

Available in templates:
- All variables: `{{ .project_name }}`, `{{ .go_module }}`
- Standard template functions: `upper`, `lower`, `title`, `replace`, `trimSuffix`, etc.

Example `go.mod.tmpl`:

```
module {{ .go_module }}

go 1.25.4
```

## Conditions

Conditions allow excluding files based on variable values:

```yaml
conditions:
  - when: '{{ eq .use_docker "false" }}'
    exclude:
      - Dockerfile
      - docker-compose.yml
      - .dockerignore
```

The `when` expression is a Go template that evaluates to `"true"` or `"false"`. The `exclude` patterns support globs and directory prefixes.

## Hooks

Post-create hooks run after all files are written:

```yaml
hooks:
  post_create:
    - "go mod tidy"
    - "git init"
    - "git add -A"
```

Hooks run in the project directory. If a hook fails, the project files are still kept.

## Managed Files

Files listed under `sync.managed_files` are tracked for ongoing synchronization:

- **`overwrite`** -- File is replaced entirely on sync
- **`merge`** -- Three-way merge preserves local changes while applying upstream updates

## Defaults Inheritance

Blueprints automatically inherit files from `_defaults/` directories in the registry. Use `defaults.exclude` to skip specific inherited files.

See [Registry Setup Guide](REGISTRY_SETUP.md) for details on the inheritance chain.
