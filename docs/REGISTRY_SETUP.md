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

## Hosting

Registries are standard Git repositories. Host them on:

- GitHub
- GitLab
- Bitbucket
- Any Git server

Forge uses [go-getter](https://github.com/hashicorp/go-getter) for fetching, supporting Git, HTTP, and other protocols.

## Tool Declarations

Registries can declare tools at the registry level using a `_tools.yaml` file or per-category in category `_defaults/`. These tools are inherited by blueprints in the same way as default files.

See [Tools Guide](TOOLS_GUIDE.md) for details.
