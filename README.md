# forge

A CLI tool that scaffolds projects from **blueprints** -- project templates stored in a Git-based **registry**. Inspired by cookiecutter, with layered defaults inheritance, managed file sync, and remote tool resolution.

## Features

- **Blueprint scaffolding** -- Create projects from templates with variable substitution via HCL2 (`${expr}` interpolation and `%{ if … ~}` directives — friendly to `{{ }}`-using tools like Helm and Argo CD). Both blueprint configs (`blueprint.hcl`) and template files use the same HCL2 grammar.
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
forge info /path/to/blueprint.hcl

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
| `forge info <blueprint.hcl>` | Show detailed blueprint information |
| `forge check` | Check project for drift against the source blueprint |
| `forge sync` | Sync project files with the latest blueprint version |
| `forge init` | Initialize a new blueprint |
| `forge registry init <path>` | Scaffold a new blueprint registry |
| `forge registry blueprint` | Scaffold a new blueprint in a registry |
| `forge registry update` | Sync blueprint metadata in `registry.hcl` |
| `forge cache clean` | Clear cached registries |

## Documentation

- [DESIGN-0001 — Blueprint Authoring](docs/design/0001-blueprint-authoring.md) -- How to create blueprints
- [DESIGN-0002 — Registry Layout & Defaults Inheritance](docs/design/0002-registry-layout-and-defaults-inheritance.md) -- How to set up a blueprint registry
- [DESIGN-0003 — Migrate template engine to HCL2](docs/design/0003-migrate-template-engine-to-hcl2.md) -- Engine swap rationale
- [DESIGN-0004 — Unify config file format after HCL2 cutover](docs/design/0004-unify-config-file-format-after-hcl2-cutover.md) -- Config-format unification
- [ADR-0001 — Use HCL2 as the template engine](docs/adr/0001-use-hcl2-as-the-template-engine.md) -- Decision record
- [docs/MIGRATION.md](docs/MIGRATION.md) -- v0.2.x → v0.5.x migration guides
- [RFC-0001 — Forge: Project Scaffolding CLI](docs/rfc/0001-forge-project-scaffolding-cli.md) -- High-level proposal and architecture

## Migrating from older releases

Per [ADR-0002](docs/adr/0002-forge-does-not-ship-in-tool-migrators.md),
forge no longer ships in-tool migrators. The `forge migrate` command
was removed in v0.5.x (IMPL-0007). Users on legacy formats follow the
**pin → migrate → upgrade** pattern instead:

```sh
# Install the last release that ships `forge migrate`:
go install github.com/donaldgifford/forge@v0.4.1
```

Then, with that pinned binary:

- **v0.2.x → v0.3.x (templates):** `forge migrate templates --path /path/to/registry`
- **v0.3.x → v0.4.x (configs):** `forge migrate config --path /path/to/registry`

Coming from v0.2.x or earlier? Run **both** in order — templates
first, then config. Once the registry is on the v0.4 format, upgrade
to current forge.

**v0.4.x → v0.5.x (lockfiles)** is per-project, not per-registry, and
has no in-tool path: rescaffold the project from the current
blueprint, or stay pinned to v0.4.1.

See [docs/MIGRATION.md](docs/MIGRATION.md) for the complete
walkthrough.

## Development

Requires Go 1.26.2+ and tools managed via [mise](https://mise.jdx.dev/).

```bash
mise install        # Set up development tools
make check          # Quick pre-commit: lint + test
make ci             # Full CI: lint + test + build
make test-coverage  # Tests with coverage report
```

## License

Apache-2.0
