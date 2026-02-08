# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Forge is a Go CLI tool (Go 1.25.4, Cobra CLI) that scaffolds projects from **blueprints** — project templates in a Git-based **registry**. Inspired by Python's cookiecutter but with layered defaults inheritance, managed file sync, and remote tool resolution. The full specification lives in `docs/PROJECT_PLAN.md`.

## Build & Development Commands

```bash
make build            # Build binary to build/bin/forge
make test             # Run all tests with race detector
make test-pkg PKG=./internal/config  # Test a single package
make test-coverage    # Tests with coverage report
make lint             # Run golangci-lint
make lint-fix         # Auto-fix lint issues
make fmt              # Format with gofmt + goimports
make ci               # Full CI: lint + test + build
make check            # Quick pre-commit: lint + test
make run-local        # Build and run the CLI
make release-local    # Test goreleaser locally
```

Tool versions are managed via `mise.toml`. Run `mise install` to set up the development environment.

## Architecture

The CLI uses Cobra for commands (`cmd/`) with core logic in `internal/` packages:

- **cmd/forge/** — Entry point and Cobra command definitions (create, init, sync, check, list, search, tools)
- **internal/config/** — `blueprint.yaml` parsing and validation
- **internal/registry/** — Registry index (`registry.yaml`), blueprint resolution, local cache
- **internal/defaults/** — `_defaults/` layered inheritance resolution (registry-wide → category → blueprint, last wins)
- **internal/getter/** — Source fetching via `hashicorp/go-getter` (registry cloning, tool downloads, archive extraction, checksum verification)
- **internal/template/** — Go `text/template` rendering with custom functions
- **internal/prompt/** — Interactive variable collection via charmbracelet/huh
- **internal/sync/** — Three-way merge sync engine for managed files (overwrite/merge strategies)
- **internal/tools/** — Remote tool manifest parsing, platform-aware download, local cache (`~/.cache/forge/tools/`)
- **internal/lockfile/** — `.forge-lock.yaml` state tracking for scaffolded projects

### Key Concepts

- **Registry**: Git repo containing blueprints, a `registry.yaml` index, and `_defaults/` directories
- **Blueprint**: Project template with `blueprint.yaml` config, templated files, and variable prompts
- **Layered Defaults**: Files inherit through `/_defaults/` → `/go/_defaults/` → `/go/api/` (last wins). Blueprints can exclude inherited defaults in `blueprint.yaml`
- **Managed Files**: Declared in sync manifest; kept aligned with blueprint via overwrite or three-way merge
- **Tool Manifest**: Remote CLI tools declared with version pins and platform-specific download sources (github-release, go-install, npm, cargo, url, script)

## Code Style

- Follows **Uber's Go Style Guide** — enforced by `.golangci.yml` with 30+ linters
- Import ordering: stdlib → third-party → `github.com/donaldgifford` (enforced by gci)
- goimports local prefix: `github.com/donaldgifford`
- Function complexity limits: cyclomatic 15, cognitive 30, max 100 lines / 50 statements
- Structs > 80 bytes should be passed by pointer
- Tests use `testify` for assertions; test helpers must call `t.Helper()`
- Mocks generated with `mockery`
- `nolint` directives require both an explanation and a specific linter name

## CI/CD

- GitHub Actions runs lint, test (with Codecov), and multi-platform goreleaser build on every push/PR to main
- Releases use semantic versioning via PR labels (major/minor/patch/dont-release)
- Binaries built for linux/darwin on amd64/arm64 with GPG signing
