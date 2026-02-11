# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Project Overview

Forge is a Go CLI tool (Go 1.25.4, Cobra CLI) that scaffolds projects from
**blueprints** — project templates in a Git-based **registry**. Inspired by
Python's cookiecutter but with layered defaults inheritance, managed file sync,
and registry-based browsing. The full specification lives in
`docs/PROJECT_PLAN.md`.

## Build & Development Commands

```bash
make build            # Build binary to build/bin/forge
make test             # Run all tests with race detector
make test-pkg PKG=./internal/config  # Test a single package
make test-coverage    # Tests with coverage report
make lint             # Run golangci-lint
make lint-fix         # Auto-fix lint issues
make fmt              # Format with gofmt + goimports
make ci               # Full CI: lint + test + build + license check
make license-check    # Check dependency licenses
make license-report   # Generate CSV license report
make check            # Quick pre-commit: lint + test
make run-local        # Build and run the CLI
make release-local    # Test goreleaser locally
```

Tool versions are managed via `mise.toml`. Run `mise install` to set up the
development environment.

## Architecture

The CLI uses Cobra for commands (`cmd/`) with core logic in `internal/`
packages:

- **cmd/forge/** — Entry point (`main.go`)
- **cmd/** — Cobra command definitions (create, init, sync, check, list, search,
  info, registry init/blueprint/update, cache)
- **internal/config/** — `blueprint.yaml` and `registry.yaml` parsing, validation,
  global config with multi-registry support
- **internal/registry/** — Registry index (`registry.yaml`), blueprint
  resolution, local cache with TTL
- **internal/defaults/** — `_defaults/` layered inheritance resolution
  (registry-wide → category → blueprint, last wins)
- **internal/getter/** — Source fetching via `hashicorp/go-getter` (registry
  cloning, archive extraction, checksum verification)
- **internal/template/** — Go `text/template` rendering with custom functions
- **internal/prompt/** — Interactive variable collection via charmbracelet/huh
- **internal/create/** — Full create workflow orchestration (resolve, prompt,
  render, conditions, lockfile)
- **internal/sync/** — Three-way merge sync engine for managed files
  (overwrite/merge strategies), conflict detection and resolution
- **internal/lockfile/** — `.forge-lock.yaml` state tracking for scaffolded
  projects
- **internal/check/** — Drift detection comparing lockfile vs local files
- **internal/hooks/** — Post-create hook execution with context cancellation
- **internal/list/** — Blueprint listing with tag filtering
- **internal/search/** — Blueprint search across name, description, tags
- **internal/info/** — Blueprint inspection with text/JSON output
- **internal/initcmd/** — Blueprint scaffolding (`init` is Go reserved keyword)
- **internal/registrycmd/** — Registry scaffolding (`forge registry init`),
  blueprint scaffolding (`forge registry blueprint`), and registry metadata
  update (`forge registry update`)
- **internal/ui/** — Styled CLI output (Success, Warning, Error, Info) respecting
  NO_COLOR

### Key Concepts

- **Registry**: Git repo containing blueprints, a `registry.yaml` index, and
  `_defaults/` directories
- **Blueprint**: Project template with `blueprint.yaml` config, templated files,
  and variable prompts
- **Layered Defaults**: Files inherit through `/_defaults/` → `/go/_defaults/` →
  `/go/api/` (last wins). Blueprints can exclude inherited defaults in
  `blueprint.yaml`
- **Managed Files**: Declared in sync manifest; kept aligned with blueprint via
  overwrite or three-way merge

### CLI Design Decisions

See `docs/gaps_implementation.md` for the full history and rationale.

- **`--registry-dir`** is a unified flag on `create`, `sync`, and `check`:
  accepts local paths AND go-getter URLs (auto-detected via `os.Stat`)
- **`forge create`** requires `--force` to write into a non-empty directory
- **`forge check`** uses SHA256 hashes in lockfile for local drift detection,
  plus `--registry-dir` for three-way upstream comparison (modified-locally,
  upstream-changed, both-changed)
- **`forge sync --ref`** pins to a specific registry version; outputs which ref
  is being synced against

## Code Style

- use `make lint` and `make fmt` to enforce our style guide.
- use `/Uber Go Style Guide` skill to help.
- Follows **Uber's Go Style Guide** — enforced by `.golangci.yml` with 30+
  linters
- Import ordering: stdlib → third-party → `github.com/donaldgifford` (enforced
  by gci)
- goimports local prefix: `github.com/donaldgifford`
- Function complexity limits: cyclomatic 15, cognitive 30, max 100 lines / 50
  statements
- Structs > 80 bytes should be passed by pointer
- Tests use `testify` for assertions; test helpers must call `t.Helper()`
- Mocks generated with `mockery`
- `nolint` directives require both an explanation and a specific linter name

## CI/CD

- GitHub Actions runs lint, test (with Codecov), license check, and
  multi-platform goreleaser build on every push/PR to main
- License compliance check using `google/go-licenses` with Apache-2.0
  compatible whitelist
- Releases use semantic versioning via PR labels
  (major/minor/patch/dont-release)
- Binaries built for linux/darwin on amd64/arm64 with GPG signing

## Rules

These rules must always be followed when working in this repository.

1. **Use the `todo-comments` skill for code annotations.** All TODO, FIX, HACK,
   WARN, PERF, NOTE, and TEST comments must follow the todo-comments format.
   Respect and obey `CLAUDE` type directives — these are binding behavioral
   instructions embedded in code.
2. **Never commit directly to `main`.** All changes go through feature branches
   and pull requests. Use the `git-workflow` skill (`/branch`) to create
   branches with the correct type prefix (feat/, fix/, chore/, docs/, bug/).
3. **Always look for enabled skills to use.** Check what skills are enabled for
   the repo and use those as guiding tools for work.
4. **Always check for make target for a command.** Check if there is an existing
   make target for what you are trying to run. This helps with automating your
   ability to run commands within the scope of safety we have defined.
