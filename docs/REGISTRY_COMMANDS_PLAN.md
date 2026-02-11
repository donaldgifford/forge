# Plan: Registry Blueprint & Registry Update Commands

## Context

Registry authors currently use `forge init <path> --registry .` to add
blueprints, but this produces only a minimal `blueprint.yaml` with no
category-aware scaffolding. There is also no tooling to keep
`registry.yaml` metadata (versions, commit hashes) in sync with actual
blueprint changes — authors must update these fields by hand.

This plan adds three capabilities under `forge registry`:

1. **`forge registry blueprint`** — Scaffold a full blueprint directory
   with category-aware defaults, a richer starter `blueprint.yaml`, and
   automatic `registry.yaml` update.
2. **`forge registry update`** — Walk all blueprints in a registry, detect
   changes via git, and bump `version` + `latest_commit` in `registry.yaml`.
3. **`forge registry update --check`** — Dry-run mode that detects stale
   metadata and exits non-zero if any blueprint has uncommitted version
   drift. Designed for CI gating.

---

## 1. `forge registry blueprint`

### Usage

```bash
# Inside a registry repo:
forge registry blueprint go/grpc-service
forge registry blueprint --category go --name grpc-service
forge registry blueprint go/grpc-service --description "gRPC service with protobuf"
forge registry blueprint go/grpc-service --tags go,grpc,api
```

Both positional (`<category>/<name>`) and flag-based (`--category` +
`--name`) forms are supported. The positional form takes precedence when
provided.

### Behavior

1. **Parse blueprint path** — derive category and name:
   - Positional arg `go/grpc-service` → category=`go`, name=`grpc-service`
   - Flags `--category go --name grpc-service` → same result
   - Error if neither form resolves a `<category>/<name>` pair
2. **Validate** — ensure we're inside a registry (registry.yaml exists at
   the resolved root), and that the target path doesn't already exist.
3. **Create directory structure**:
   ```
   <category>/<name>/
   ├── blueprint.yaml
   └── {{project_name}}/
       └── README.md.tmpl
   ```
4. **Generate `blueprint.yaml`** — richer than the minimal `forge init`
   template. Includes:
   - `apiVersion: v1`
   - `name` derived as `<category>-<name>` (e.g., `go-grpc-service`)
   - `description` from `--description` flag or TODO placeholder
   - `version: "0.1.0"`
   - `tags` from `--tags` flag or `[<category>]` as default
   - A `project_name` variable (required, with validation regex)
   - A `license` choice variable with common options
   - Empty `conditions`, `hooks.post_create`, `sync.managed_files`,
     `sync.ignore` sections as commented guidance
   - `rename` section mapping `"{{project_name}}/"` to `"."`
5. **Create starter template file** — a `{{project_name}}/README.md.tmpl`
   with `{{ .project_name }}` and `{{ .description }}` placeholders so the
   blueprint is immediately usable with `forge create`.
6. **Create `<category>/_defaults/`** if it doesn't exist yet — with a
   `.gitkeep` so git tracks it. This mirrors what `registry init --category`
   does.
7. **Update `registry.yaml`** — append a `BlueprintEntry`:
   ```yaml
   - name: go/grpc-service
     path: go/grpc-service
     description: "gRPC service with protobuf"
     version: "0.1.0"
     tags: ["go", "grpc", "api"]
     latest_commit: ""
   ```
   Duplicate check by `path`; error if already cataloged.
8. **Print success** with next-steps guidance.

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--category` | string | (from positional) | Blueprint category directory |
| `--name` | string | (from positional) | Blueprint name within category |
| `--description` | string | TODO placeholder | Blueprint description |
| `--tags` | []string | `[<category>]` | Tags for registry index |

### Differences from `forge init --registry`

| Aspect | `forge init --registry` | `forge registry blueprint` |
|--------|------------------------|---------------------------|
| Scope | Minimal `blueprint.yaml` only | Full scaffold with template dir, starter files |
| Category awareness | None — just writes to a path | Creates `_defaults/` for category if missing |
| Blueprint template | 7-line minimal YAML | Rich YAML with variables, rename, commented sections |
| Starter files | None | `{{project_name}}/README.md.tmpl` |
| Tags | None | Auto-populated from category + `--tags` flag |
| Target users | Quick one-off | Registry maintainers building a curated registry |

### Package Layout

- **`cmd/registry_blueprint.go`** — Cobra command wiring, flag parsing
- **`internal/registrycmd/blueprint.go`** — Core logic: `RunBlueprint(opts)`
- **`internal/registrycmd/blueprint_test.go`** — Unit tests

---

## 2. `forge registry update`

### Usage

```bash
# Inside a registry repo (auto-detects registry.yaml):
forge registry update

# Explicit registry path:
forge registry update --registry-dir ./path/to/registry

# Dry-run / CI check mode:
forge registry update --check
```

### Behavior

1. **Locate registry** — look for `registry.yaml` in the current directory
   or `--registry-dir`. Error if not found.
2. **Load `registry.yaml`** — parse all `BlueprintEntry` items.
3. **For each blueprint entry**, detect changes:
   a. Verify `blueprint.yaml` exists at the declared `path`. Warn and skip
      if missing.
   b. Load the blueprint's `blueprint.yaml` to read its current `version`.
   c. Compute the latest git commit that touched files under that path:
      ```
      git log -1 --format=%H -- <path>/
      ```
   d. Compare against the entry's `latest_commit` in `registry.yaml`.
   e. Compare the blueprint.yaml `version` against the entry's `version`.
4. **Determine status** for each blueprint:
   - **up-to-date** — commit and version both match
   - **version-changed** — blueprint.yaml version differs from
     registry.yaml version (author bumped version in blueprint.yaml but
     hasn't run update yet)
   - **files-changed** — git commit differs but version in blueprint.yaml
     is unchanged (author changed files but forgot to bump version)
   - **both-changed** — both version and commit differ
   - **missing** — path exists in registry.yaml but directory not found
5. **In normal mode** (`forge registry update` without `--check`):
   - For **version-changed** and **both-changed** blueprints: update the
     entry's `version` and `latest_commit` in registry.yaml.
   - For **files-changed** blueprints: update `latest_commit` but print a
     warning that the version in `blueprint.yaml` was not bumped. The
     registry entry's `version` is synced from `blueprint.yaml`'s version
     regardless — the warning is informational.
   - Write updated `registry.yaml`.
   - Print a summary table of what changed.
6. **In check mode** (`forge registry update --check`):
   - Do NOT write any files.
   - Print the same summary table.
   - Exit 0 if all blueprints are up-to-date.
   - Exit 1 if any blueprint has stale metadata, printing which entries
     are out of date.

### Output

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

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--registry-dir` | string | `.` (cwd) | Registry root directory |
| `--check` | bool | false | Check-only mode; exit 1 if stale |

### Git Dependency

`forge registry update` requires being run inside a git repository (or
pointing `--registry-dir` at one) because it uses `git log` to determine
the latest commit per blueprint path. If git is not available or the
directory is not a git repo, return a clear error:
`"registry update requires a git repository"`.

### Package Layout

- **`cmd/registry_update.go`** — Cobra command wiring, flag parsing
- **`internal/registrycmd/update.go`** — Core logic: `RunUpdate(opts)`
- **`internal/registrycmd/update_test.go`** — Unit tests (using test git
  repos via `git init` + `git commit` in temp dirs)

---

## 3. Design Details

### Registry YAML Write Strategy

Both commands modify `registry.yaml`. To preserve comments and formatting
as much as possible:

- **`registry blueprint`**: Load via `yaml.Unmarshal`, append entry,
  `yaml.Marshal` back. This matches the existing pattern in
  `initcmd.appendToRegistryIndex()`.
- **`registry update`**: Same load-modify-marshal approach. Since
  `registry.yaml` is machine-managed metadata, full re-marshaling is
  acceptable.

### Version Source of Truth

The `version` field in `blueprint.yaml` is the source of truth.
`registry.yaml` mirrors it. `forge registry update` copies the version
from `blueprint.yaml` into the registry entry — it does not auto-bump
versions. Authors bump versions by editing `blueprint.yaml` directly.

### Blueprint Name Convention

The `name` field in `registry.yaml` uses the path format: `go/grpc-service`.
The `name` field in `blueprint.yaml` uses the hyphenated format:
`go-grpc-service`. This follows the existing convention in the testdata
(`go/api` in registry → `go-api` in blueprint.yaml).

### Error Handling

- Missing `registry.yaml` → clear error with suggestion to run
  `forge registry init`
- Blueprint path already exists → error, suggest using `forge init` for
  re-initialization
- Non-git directory for `update` → error with explanation
- `git log` failure for a specific path → warn and skip that blueprint

---

## 4. Files to Create

| File | Purpose |
|------|---------|
| `cmd/registry_blueprint.go` | Cobra command for `forge registry blueprint` |
| `cmd/registry_update.go` | Cobra command for `forge registry update` |
| `internal/registrycmd/blueprint.go` | Blueprint scaffold logic |
| `internal/registrycmd/blueprint_test.go` | Tests for blueprint scaffold |
| `internal/registrycmd/update.go` | Registry update/check logic |
| `internal/registrycmd/update_test.go` | Tests for registry update (with git fixtures) |

## 5. Files to Modify

| File | Change |
|------|--------|
| `CLAUDE.md` | Add `registry blueprint` and `registry update` to architecture notes |
| `README.md` | Add commands to Quick Start and Commands table |
| `docs/REGISTRY_SETUP.md` | Document new workflows |

---

## 6. Implementation Order

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

---

## 7. Verification

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
