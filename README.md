# forge

A CLI tool that scaffolds projects from **blueprints** -- project templates stored in a Git-based **registry**. Inspired by cookiecutter, with layered defaults inheritance, managed file sync, and remote tool resolution.

## Features

- **Blueprint scaffolding** -- Create projects from templates with variable substitution via Go `text/template`
- **Layered defaults** -- Inherit config files through `_defaults/` directories (registry-wide, category, blueprint)
- **Managed file sync** -- Keep files aligned with upstream blueprints using overwrite or three-way merge strategies
- **Registry browsing** -- List, search, and inspect blueprints from Git-based registries

## Installation

### From source

```bash
go install github.com/donaldgifford/forge/cmd/forge@latest
```

### From releases

Download the binary for your platform from [GitHub Releases](https://github.com/donaldgifford/forge/releases).

### Build from source

```bash
git clone https://github.com/donaldgifford/forge.git
cd forge
make build
./build/bin/forge version
```

## Quick Start

```bash
# Create a project from a blueprint
forge create go/api --set project_name=my-service --set go_module=github.com/me/my-service

# List available blueprints
forge list --registry /path/to/registry

# Search blueprints
forge search api --registry /path/to/registry

# Inspect a blueprint
forge info /path/to/blueprint.yaml

# Check for drift against the source blueprint
forge check

# Sync project files with the latest blueprint
forge sync --dry-run
forge sync

# Initialize a new blueprint registry
forge registry init my-registry --name "My Blueprints" --category go --category python

# Add a blueprint to a registry
forge registry blueprint go/grpc-service --registry-dir ./my-registry

# Update registry metadata after blueprint changes
forge registry update --registry-dir ./my-registry

# Clean cached data
forge cache clean
```

## Commands

| Command | Description |
|---------|-------------|
| `forge create <blueprint>` | Scaffold a new project from a blueprint |
| `forge list` | List available blueprints in a registry |
| `forge search <query>` | Search blueprints by name, description, or tags |
| `forge info <blueprint.yaml>` | Show detailed blueprint information |
| `forge check` | Check project for drift against the source blueprint |
| `forge sync` | Sync project files with the latest blueprint version |
| `forge init` | Initialize a new blueprint |
| `forge registry init <path>` | Scaffold a new blueprint registry |
| `forge registry blueprint` | Scaffold a new blueprint in a registry |
| `forge registry update` | Sync blueprint metadata in registry.yaml |
| `forge cache clean` | Clear cached registries |

## Documentation

- [DESIGN-0001 — Blueprint Authoring](docs/design/0001-blueprint-authoring.md) -- How to create blueprints
- [DESIGN-0002 — Registry Layout & Defaults Inheritance](docs/design/0002-registry-layout-and-defaults-inheritance.md) -- How to set up a blueprint registry
- [RFC-0001 — Forge: Project Scaffolding CLI](docs/rfc/0001-forge-project-scaffolding-cli.md) -- High-level proposal and architecture

## Development

Requires Go 1.25.4+ and tools managed via [mise](https://mise.jdx.dev/).

```bash
mise install        # Set up development tools
make check          # Quick pre-commit: lint + test
make ci             # Full CI: lint + test + build
make test-coverage  # Tests with coverage report
```

## License

Apache-2.0
