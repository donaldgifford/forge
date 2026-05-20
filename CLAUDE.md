# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Project Overview

Forge is a Go CLI tool (Go 1.26.2, Cobra CLI) that scaffolds projects from
**blueprints** — project templates in a Git-based **registry**. Inspired by
Python's cookiecutter but with layered defaults inheritance, managed file sync,
and registry-based browsing. The full specification lives in
`docs/rfc/0001-forge-project-scaffolding-cli.md`.

Templates use **HCL2** (`hashicorp/hcl/v2`) with `${expr}` interpolation and
`%{ if … ~}` directives. HCL2 was chosen so `{{ }}`-using downstream tools
(Helm, Argo CD, kustomize) can be scaffolded without escape gymnastics — see
[ADR-0001](docs/adr/0001-use-hcl2-as-the-template-engine.md). Authors upgrading
older blueprints follow [docs/MIGRATION.md](docs/MIGRATION.md).

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
  info, migrate, registry init/blueprint/update, cache)
- **internal/config/** — `blueprint.hcl` and `registry.hcl` parsing, validation,
  global config with multi-registry support. HCL is the only accepted format
  (`loader.go` dispatches `.hcl` directly and rejects bare `.yaml` with a
  pointer to `forge migrate config`). The HCL decode spec lives in
  `hcldec_spec.go` and the emitter for registry round-trip in `hclemit.go`.
  `Condition.When` is `hcl.Expression` parsed at load time, with the original
  source kept on `WhenSource` for round-tripping
- **internal/registry/** — Registry index (`registry.hcl`), blueprint
  resolution, local cache with TTL
- **internal/defaults/** — `_defaults/` layered inheritance resolution
  (registry-wide → category → blueprint, last wins)
- **internal/getter/** — Source fetching via `hashicorp/go-getter` (registry
  cloning, archive extraction, checksum verification)
- **internal/template/** — HCL2 (`hashicorp/hcl/v2`) rendering with custom
  functions; values flow as `cty.Value` (`zclconf/go-cty`)
- **internal/prompt/** — Interactive variable collection via charmbracelet/huh;
  default-value templates also render through HCL2
- **internal/create/** — Full create workflow orchestration (resolve, prompt,
  render, conditions, lockfile)
- **internal/sync/** — Three-way merge sync engine for managed files
  (overwrite/merge strategies), conflict detection and resolution
- **internal/lockfile/** — `.forge-lock.hcl` state tracking for scaffolded
  projects (HCL is the only accepted on-disk format from v0.5+; bare
  `.forge-lock.yaml` files trigger a rescaffold-or-pin error per ADR-0002).
  Loader/emitter pair in `loader_hcl.go` + `emit_hcl.go` use `hcldec`
  PartialContent on the eager fields and hand-decode the dynamic `variables`
  block; `cty.Value` in memory with typed coercion via `lockfile.ToCtyValues`
  using declared variable types
- **internal/check/** — Drift detection comparing lockfile vs local files
- **internal/hooks/** — Post-create hook execution with context cancellation
- **internal/list/** — Blueprint listing with tag filtering
- **internal/search/** — Blueprint search across name, description, tags
- **internal/info/** — Blueprint inspection with text/JSON output
- **internal/initcmd/** — Blueprint scaffolding (`init` is Go reserved keyword)
- **internal/migratecmd/** — Two one-shot migrators:
  `forge migrate templates` rewrites v1 (Go `text/template`) syntax to v2
  (HCL2) inside `*.tmpl` files and the expression-bearing fields of legacy
  `blueprint.yaml`/`registry.yaml`. `forge migrate config` then converts
  those YAML files to `blueprint.hcl`/`registry.hcl`. Only place in the
  tree that imports `text/template` (used to parse v1 input) and
  `gopkg.in/yaml.v3` (used to parse legacy YAML configs via shadow types
  in `yaml_types.go`)
- **internal/registrycmd/** — Registry scaffolding (`forge registry init`),
  blueprint scaffolding (`forge registry blueprint`), and registry metadata
  update (`forge registry update`)
- **internal/ui/** — Styled CLI output (Success, Warning, Error, Info) respecting
  NO_COLOR

### Key Concepts

- **Registry**: Git repo containing blueprints, a `registry.hcl` index, and
  `_defaults/` directories
- **Blueprint**: Project template with `blueprint.hcl` config, templated files,
  and variable prompts
- **Layered Defaults**: Files inherit through `/_defaults/` → `/go/_defaults/` →
  `/go/api/` (last wins). Blueprints can exclude inherited defaults in
  `blueprint.hcl`
- **Managed Files**: Declared in sync manifest; kept aligned with blueprint via
  overwrite or three-way merge

### CLI Design Decisions

See `docs/impl/0002-mvp-cli-gap-closure.md` for the full history and rationale.

- **`--registry-dir`** is a unified flag on `create`, `sync`, and `check`:
  accepts local paths AND go-getter URLs (auto-detected via `os.Stat`)
- **`forge create`** requires `--force` to write into a non-empty directory
- **`forge check`** uses SHA256 hashes in lockfile for local drift detection,
  plus `--registry-dir` for three-way upstream comparison (modified-locally,
  upstream-changed, both-changed)
- **`forge sync --ref`** pins to a specific registry version; outputs which ref
  is being synced against
- **`forge migrate templates --path <registry>`** rewrites legacy v1
  `text/template` blueprints to v2 (HCL2) in place. Documented in
  [docs/MIGRATION.md](docs/MIGRATION.md); guarded by a dirty-worktree check
  (override with `--force`).
- **`forge migrate config --path <registry>`** rewrites legacy v2 YAML
  configs (`blueprint.yaml`, `registry.yaml`) as their HCL equivalents
  (`blueprint.hcl`, `registry.hcl`). Drops the `apiVersion` field on emit
  (the file extension is the version signal). Same dirty-worktree guard,
  same `--dry-run` / `--strict` / `--force` flags. v0.2.x users run
  `migrate templates` first, then `migrate config`. Comments in source
  YAML are not preserved (per IMPL-0005 OQ-3).

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
- **gosec baseline:** Inline `//nolint:gosec` directives intentionally annotate correct-by-design CLI behaviour (file reads/writes against user-chosen output and registry dirs, `git` against user registry dirs, hook execution from `blueprint.hcl`, lockfile and template I/O from known project paths). Active sites: `internal/create/create.go`, `internal/sync/overwrite.go`, `internal/hooks/hooks.go`, `internal/registrycmd/registrycmd.go`, `internal/registrycmd/update.go`, `internal/config/loader_hcl.go`, `internal/lockfile/loader_hcl.go`, `internal/lockfile/emit_hcl.go`, `internal/template/renderer.go`, `internal/migratecmd/config_walk.go`, `internal/migratecmd/walk.go`, `internal/migratecmd/git.go`. Don't remove without a real fix — gosec reports them as G304/G703/G204/G306.

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
