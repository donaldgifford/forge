# Registry Setup Guide

A registry is a Git repository containing blueprints and shared configuration files.

## Directory Structure

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

## registry.yaml

The registry index lists all available blueprints:

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

## Layered Defaults

Files are inherited through a three-level chain:

1. **Registry defaults** (`/_defaults/`) -- Applied to all blueprints
2. **Category defaults** (`/go/_defaults/`) -- Applied to all blueprints in the `go/` category
3. **Blueprint files** (`/go/api/`) -- Blueprint-specific files

When the same file exists at multiple levels, the most specific version wins (blueprint > category > registry).

### Example

Given this registry structure:

```
_defaults/
  .editorconfig          # All projects get this
  scripts/lint.sh        # Generic lint script
go/
  _defaults/
    .golangci.yml        # All Go projects get this
    scripts/lint.sh      # Go-specific lint (overrides registry default)
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

## Excluding Defaults

Blueprints can opt out of inherited files in `blueprint.yaml`:

```yaml
defaults:
  exclude:
    - ".github/CODEOWNERS"
    - "scripts/deploy.sh"
```

## Adding Blueprints

Use `forge registry blueprint` to scaffold a new blueprint inside a registry:

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

- `<category>/<name>/blueprint.yaml` -- A rich starter config with variables,
  hooks, sync, and rename sections.
- `<category>/<name>/{{project_name}}/README.md.tmpl` -- A starter template.
- `<category>/_defaults/.gitkeep` -- Category defaults directory (if it doesn't
  already exist).
- An entry appended to `registry.yaml`.

Edit `blueprint.yaml` to customize variables, add template files, and configure
sync behavior.

## Keeping Metadata in Sync

When you modify a blueprint (change its version, update template files, etc.),
the `registry.yaml` index can become stale. Use `forge registry update` to
reconcile it:

```bash
# Update stale entries in registry.yaml
forge registry update --registry-dir ./my-registry

# Check-only mode (for CI): exits non-zero if stale
forge registry update --check --registry-dir ./my-registry
```

The update command compares each blueprint's `version` from `blueprint.yaml`
and the latest git commit hash against the values in `registry.yaml`. It
reports one of five statuses for each entry:

| Status | Meaning |
|--------|---------|
| `up-to-date` | Registry entry matches blueprint and git state |
| `version-changed` | `blueprint.yaml` version differs from `registry.yaml` |
| `files-changed` | Git commit differs but version is unchanged |
| `both-changed` | Both version and git commit differ |
| `missing` | Blueprint path does not exist on disk (skipped) |

### CI Integration

Add a check step to your CI pipeline to catch stale metadata:

```yaml
# GitHub Actions example
- name: Check registry metadata
  run: forge registry update --check
```

## Hosting

Registries are standard Git repositories. Host them on:

- GitHub
- GitLab
- Bitbucket
- Any Git server

Forge uses [go-getter](https://github.com/hashicorp/go-getter) for fetching, supporting Git, HTTP, and other protocols.
