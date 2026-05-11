---
id: PLAN-0001
title: "Registry Blueprint and Update Commands"
status: Completed
author: Donald Gifford
created: 2026-05-07
---
<!-- markdownlint-disable-file MD025 MD041 -->

# PLAN 0001: Registry Blueprint and Update Commands

**Status:** Completed
**Author:** Donald Gifford
**Date:** 2026-05-07

<!--toc:start-->
- [Goal](#goal)
- [Context](#context)
- [Approach](#approach)
  - [Part 1: forge registry blueprint](#part-1-forge-registry-blueprint)
    - [Usage](#usage)
    - [Behavior](#behavior)
    - [Flags](#flags)
    - [Differences from forge init --registry](#differences-from-forge-init---registry)
  - [Part 2: forge registry update](#part-2-forge-registry-update)
    - [Usage](#usage-1)
    - [Behavior](#behavior-1)
    - [Output](#output)
    - [Flags](#flags-1)
    - [Git Dependency](#git-dependency)
- [Components](#components)
- [Design Details](#design-details)
  - [Registry YAML Write Strategy](#registry-yaml-write-strategy)
  - [Version Source of Truth](#version-source-of-truth)
  - [Blueprint Name Convention](#blueprint-name-convention)
  - [Error Handling](#error-handling)
- [File Changes](#file-changes)
- [Implementation Order](#implementation-order)
- [Verification](#verification)
  - [forge registry blueprint](#forge-registry-blueprint)
  - [forge registry update](#forge-registry-update)
- [Dependencies](#dependencies)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Goal

Registry maintainers can scaffold richly-templated blueprints inside an
existing registry and keep `registry.yaml` metadata in sync with
on-disk and git state — both as a manual command and as a CI gate.

```bash
# Inside a registry repo:
forge registry blueprint go/grpc-service \
  --description "gRPC service with protobuf" \
  --tags go,grpc,api

# Later, after editing the blueprint:
forge registry update          # updates registry.yaml versions/commits
forge registry update --check  # CI: exit 1 if metadata stale
```

## Context

Registry authors currently use `forge init <path> --registry .` to add
blueprints, but this produces only a minimal `blueprint.yaml` with no
category-aware scaffolding. There is also no tooling to keep
`registry.yaml` metadata (versions, commit hashes) in sync with actual
blueprint changes — authors must update these fields by hand.

This plan adds three capabilities under `forge registry`:

1. **`forge registry blueprint`** — Scaffold a full blueprint
   directory with category-aware defaults, a richer starter
   `blueprint.yaml`, and automatic `registry.yaml` update.
2. **`forge registry update`** — Walk all blueprints in a registry,
   detect changes via git, and bump `version` + `latest_commit` in
   `registry.yaml`.
3. **`forge registry update --check`** — Dry-run mode that detects
   stale metadata and exits non-zero if any blueprint has
   uncommitted version drift. Designed for CI gating.

## Approach

### Part 1: `forge registry blueprint`

#### Usage

```bash
forge registry blueprint go/grpc-service
forge registry blueprint --category go --name grpc-service
forge registry blueprint go/grpc-service --description "gRPC service with protobuf"
forge registry blueprint go/grpc-service --tags go,grpc,api
```

Both positional (`<category>/<name>`) and flag-based (`--category` +
`--name`) forms are supported. Positional takes precedence when
provided.

#### Behavior

1. **Parse blueprint path** — derive category and name (positional arg
   or flags). Error if neither resolves a `<category>/<name>` pair.
2. **Validate** — ensure registry.yaml exists at the resolved root and
   target path doesn't already exist.
3. **Create directory structure**:

   ```
   <category>/<name>/
   ├── blueprint.yaml
   └── {{project_name}}/
       └── README.md.tmpl
   ```

4. **Generate `blueprint.yaml`** — richer than the minimal `forge
   init` template:
   - `apiVersion: v1`
   - `name: <category>-<name>` (e.g., `go-grpc-service`)
   - `description` from `--description` or TODO placeholder
   - `version: "0.1.0"`
   - `tags` from `--tags` or `[<category>]` default
   - `project_name` variable (required, validation regex)
   - `license` choice variable
   - Empty `conditions`, `hooks.post_create`, `sync.managed_files`,
     `sync.ignore` as commented guidance
   - `rename: "{{project_name}}/": "."`
5. **Create starter template file** — `{{project_name}}/README.md.tmpl`
   with `{{ .project_name }}` and `{{ .description }}` placeholders.
6. **Create `<category>/_defaults/`** if it doesn't exist — with a
   `.gitkeep`. Mirrors `registry init --category`.
7. **Update `registry.yaml`** — append a `BlueprintEntry`. Duplicate
   check by `path`; error if already cataloged.
8. **Print success** with next-steps guidance.

#### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--category` | string | (from positional) | Blueprint category directory |
| `--name` | string | (from positional) | Blueprint name within category |
| `--description` | string | TODO placeholder | Blueprint description |
| `--tags` | []string | `[<category>]` | Tags for registry index |

#### Differences from `forge init --registry`

| Aspect | `forge init --registry` | `forge registry blueprint` |
|--------|------------------------|---------------------------|
| Scope | Minimal `blueprint.yaml` only | Full scaffold with template dir, starter files |
| Category awareness | None | Creates `_defaults/` for category if missing |
| Blueprint template | 7-line minimal YAML | Rich YAML with variables, rename, commented sections |
| Starter files | None | `{{project_name}}/README.md.tmpl` |
| Tags | None | Auto-populated from category + `--tags` |
| Target users | Quick one-off | Registry maintainers building a curated registry |

### Part 2: `forge registry update`

#### Usage

```bash
forge registry update
forge registry update --registry-dir ./path/to/registry
forge registry update --check
```

#### Behavior

1. **Locate registry** — look for `registry.yaml` in the cwd or
   `--registry-dir`. Error if not found.
2. **Load `registry.yaml`** — parse all `BlueprintEntry` items.
3. **For each entry**, detect changes:
   a. Verify `blueprint.yaml` exists at `path`. Warn and skip if
   missing.
   b. Read the blueprint's current `version`.
   c. Compute the latest git commit that touched files under that
      path: `git log -1 --format=%H -- <path>/`.
   d. Compare against the entry's `latest_commit`.
   e. Compare the blueprint.yaml `version` against the entry's
      `version`.
4. **Determine status** for each blueprint:
   - **up-to-date** — commit and version both match
   - **version-changed** — blueprint.yaml version differs (author
     bumped version but hasn't run update)
   - **files-changed** — git commit differs but version unchanged
     (author changed files but forgot to bump version)
   - **both-changed** — both differ
   - **missing** — path exists in registry.yaml but directory not
     found
5. **In normal mode** (`forge registry update`):
   - For **version-changed** and **both-changed**: update `version`
     and `latest_commit`.
   - For **files-changed**: update `latest_commit` but warn that
     version was not bumped (informational).
   - Write updated `registry.yaml`.
   - Print a summary table.
6. **In check mode** (`--check`):
   - Do NOT write any files.
   - Print summary table.
   - Exit 0 if all up-to-date; exit 1 if any are stale, listing them.

#### Output

Normal mode:

```
Updating registry metadata...

  BLUEPRINT          STATUS           VERSION
  go/api             up-to-date       1.0.0
  go/grpc-service    version-changed  0.1.0 → 0.2.0
  go/cli             files-changed    1.0.0 (commit updated, version unchanged)

✓ Updated registry.yaml (2 blueprints updated)
```

Check mode (with drift):

```
Registry metadata check failed:

  BLUEPRINT          STATUS           DETAIL
  go/grpc-service    version-changed  registry has 0.1.0, blueprint has 0.2.0
  go/cli             files-changed    commit abc123 ≠ def456, version unchanged

Run `forge registry update` to fix.
```

#### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--registry-dir` | string | `.` (cwd) | Registry root directory |
| `--check` | bool | false | Check-only mode; exit 1 if stale |

#### Git Dependency

`forge registry update` requires a git repository (or
`--registry-dir` pointing at one) because it uses `git log` per
blueprint path. If git is unavailable or the directory is not a git
repo, return: `"registry update requires a git repository"`.

## Components

| Component | Purpose |
|-----------|---------|
| `cmd/registry_blueprint.go` | Cobra command wiring, flag parsing |
| `cmd/registry_update.go` | Cobra command wiring, flag parsing |
| `internal/registrycmd/blueprint.go` | Core blueprint scaffold logic (`RunBlueprint`) |
| `internal/registrycmd/update.go` | Core update + check logic (`RunUpdate`) |
| `internal/registrycmd/blueprint_test.go` | Unit tests |
| `internal/registrycmd/update_test.go` | Unit tests with git fixtures |

## Design Details

### Registry YAML Write Strategy

Both commands modify `registry.yaml`. To preserve comments and
formatting as much as possible:

- **`registry blueprint`**: Load via `yaml.Unmarshal`, append entry,
  `yaml.Marshal` back. Matches `initcmd.appendToRegistryIndex()`.
- **`registry update`**: Same load-modify-marshal approach. Since
  `registry.yaml` is machine-managed metadata, full re-marshaling is
  acceptable.

### Version Source of Truth

The `version` field in `blueprint.yaml` is the source of truth.
`registry.yaml` mirrors it. `forge registry update` copies the
version from `blueprint.yaml` into the registry entry — it does not
auto-bump versions. Authors bump versions by editing `blueprint.yaml`
directly.

### Blueprint Name Convention

The `name` field in `registry.yaml` uses the path format:
`go/grpc-service`. The `name` field in `blueprint.yaml` uses the
hyphenated format: `go-grpc-service`. This follows the existing
testdata convention (`go/api` in registry → `go-api` in
blueprint.yaml).

### Error Handling

- Missing `registry.yaml` → clear error suggesting `forge registry
  init`.
- Blueprint path already exists → error suggesting `forge init` for
  re-initialization.
- Non-git directory for `update` → error with explanation.
- `git log` failure for a specific path → warn and skip.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `cmd/registry_blueprint.go` | Create | Cobra command for `forge registry blueprint` |
| `cmd/registry_update.go` | Create | Cobra command for `forge registry update` |
| `internal/registrycmd/blueprint.go` | Create | Blueprint scaffold logic |
| `internal/registrycmd/blueprint_test.go` | Create | Tests for blueprint scaffold |
| `internal/registrycmd/update.go` | Create | Registry update/check logic |
| `internal/registrycmd/update_test.go` | Create | Tests for update (with git fixtures) |
| `CLAUDE.md` | Modify | Add `registry blueprint` and `registry update` to architecture notes |
| `README.md` | Modify | Add to Quick Start and Commands table |
| `docs/REGISTRY_SETUP.md` | Modify | Document new workflows |

## Implementation Order

```
Phase 1: registry blueprint
  1. internal/registrycmd/blueprint.go      — core scaffold logic
  2. internal/registrycmd/blueprint_test.go  — unit tests
  3. cmd/registry_blueprint.go              — CLI wiring
  4. Manual verification

Phase 2: registry update
  5. internal/registrycmd/update.go         — update + check logic
  6. internal/registrycmd/update_test.go    — tests with git fixtures
  7. cmd/registry_update.go                 — CLI wiring
  8. Manual verification

Phase 3: docs + polish
  9. Update CLAUDE.md, README.md, docs/REGISTRY_SETUP.md
 10. make check
```

## Verification

### `forge registry blueprint`

```bash
# Set up a test registry
forge registry init /tmp/test-reg --name "Test" --category go --category rust

# Scaffold a blueprint
forge registry blueprint go/grpc-service \
  --description "gRPC service" \
  --tags go,grpc \
  --registry-dir /tmp/test-reg

# Verify structure
ls /tmp/test-reg/go/grpc-service/
# → blueprint.yaml  {{project_name}}/

cat /tmp/test-reg/go/grpc-service/blueprint.yaml
# → apiVersion: v1, name: go-grpc-service, tags: [go, grpc], etc.

cat /tmp/test-reg/registry.yaml
# → blueprints should include go/grpc-service entry

# Verify the blueprint is usable
forge create go/grpc-service \
  --registry-dir /tmp/test-reg \
  --defaults --no-hooks \
  --set project_name=my-svc \
  -o /tmp/test-svc --force
ls /tmp/test-svc/
# → README.md (rendered from template)
```

### `forge registry update`

```bash
# Use testdata registry in a git context
cd /tmp/test-reg && git init && git add -A && git commit -m "init"

# Modify a blueprint version
sed -i '' 's/version: "0.1.0"/version: "0.2.0"/' go/grpc-service/blueprint.yaml
git add -A && git commit -m "bump grpc-service"

# Check mode should detect drift
forge registry update --check --registry-dir /tmp/test-reg
# → exit 1, shows go/grpc-service as version-changed

# Update mode should fix it
forge registry update --registry-dir /tmp/test-reg
# → updates registry.yaml with new version + commit

# Check mode should now pass
forge registry update --check --registry-dir /tmp/test-reg
# → exit 0
```

## Dependencies

- Existing `internal/registrycmd/registrycmd.go` for shared registry
  scaffolding patterns.
- `internal/config/` for `Registry`/`BlueprintEntry`/`Blueprint`
  parsing and validation.
- System `git` binary on PATH for `update`.

## Open Questions

- **Auto-bump behavior**: Should `files-changed` warn-only or
  auto-bump patch version? (Decided: warn-only; authors retain
  control.)
- **Nested categories**: Currently only one category level is
  supported. Worth supporting `cloud/aws/lambda`-style nesting later.

## References

- [DESIGN-0002 — Registry Layout & Defaults Inheritance](../design/0002-registry-layout-and-defaults-inheritance.md)
- [IMPL-0003 — Registry Commands Implementation](../impl/0003-registry-commands.md)
- [RFC-0001 — Forge: Project Scaffolding CLI](../rfc/0001-forge-project-scaffolding-cli.md)
