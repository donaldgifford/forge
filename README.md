# forge

A CLI tool that scaffolds projects from **blueprints** -- project templates stored in a Git-based **registry**. Inspired by cookiecutter, with layered defaults inheritance, managed file sync, and remote tool resolution.

## Features

- **Blueprint scaffolding** -- Create projects from templates with variable substitution via Go `text/template`
- **Layered defaults** -- Inherit config files through `_defaults/` directories (registry-wide, category, blueprint)
- **Managed file sync** -- Keep files aligned with upstream blueprints using overwrite or three-way merge strategies
- **Remote tool resolution** -- Declare CLI tools in blueprints with platform-aware download and caching
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

# Manage tools declared in the blueprint
forge tools list
forge tools install

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
| `forge tools list` | List tools declared in the blueprint |
| `forge tools install` | Install tools declared in the blueprint |
| `forge init` | Initialize a new blueprint |
| `forge cache clean` | Clear cached registries and tools |

## Documentation

- [Blueprint Authoring Guide](docs/BLUEPRINT_AUTHORING.md) -- How to create blueprints
- [Registry Setup Guide](docs/REGISTRY_SETUP.md) -- How to set up a blueprint registry
- [Tools Guide](docs/TOOLS_GUIDE.md) -- Tool manifest format and source types

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
