---
id: IMPL-0003
title: "Registry Commands"
status: Completed
author: Donald Gifford
created: 2026-05-07
---
<!-- markdownlint-disable-file MD025 MD041 -->

# IMPL 0003: Registry Commands

**Status:** Completed
**Author:** Donald Gifford
**Date:** 2026-05-07

<!--toc:start-->
- [Objective](#objective)
- [Scope](#scope)
  - [In Scope](#in-scope)
  - [Out of Scope](#out-of-scope)
- [Implementation Phases](#implementation-phases)
  - [Phase 1: forge registry blueprint](#phase-1-forge-registry-blueprint)
    - [Tasks](#tasks)
    - [Success Criteria — Phase 1](#success-criteria--phase-1)
  - [Phase 2: forge registry update](#phase-2-forge-registry-update)
    - [Tasks](#tasks-1)
    - [Success Criteria — Phase 2](#success-criteria--phase-2)
  - [Phase 3: Documentation & Polish](#phase-3-documentation--polish)
    - [Tasks](#tasks-2)
    - [Success Criteria — Phase 3](#success-criteria--phase-3)
- [File Changes](#file-changes)
- [Testing Plan](#testing-plan)
- [Appendix: Key Patterns to Follow](#appendix-key-patterns-to-follow)
  - [Git Operations in Tests](#git-operations-in-tests)
- [References](#references)
<!--toc:end-->

## Objective

Detailed implementation tasks for the features described in PLAN-0001.
Each phase produces a working, tested feature. Tasks are ordered by
dependency — work top-to-bottom.

**Implements:** [PLAN-0001 — Registry Blueprint & Update Commands](../plan/0001-registry-blueprint-and-update-commands.md)

## Scope

### In Scope

- `forge registry blueprint` — full blueprint scaffolding inside a
  registry.
- `forge registry update` — sync `registry.yaml` metadata from
  `blueprint.yaml` versions and git commits.
- `forge registry update --check` — CI gate.
- Documentation updates in `CLAUDE.md`, `README.md`, and
  `docs/REGISTRY_SETUP.md`.

### Out of Scope

- New blueprint variables/conditions/hooks (covered by DESIGN-0001).
- Changes to `forge create`/`sync`/`check` (covered by IMPL-0002).

## Implementation Phases

### Phase 1: `forge registry blueprint`

Scaffold a full blueprint directory inside a registry with category
awareness, a rich starter `blueprint.yaml`, template files, and
automatic `registry.yaml` updates.

#### Tasks

- [x] **1.1 Define `BlueprintOpts` and `BlueprintResult` types**
  - File: `internal/registrycmd/blueprint.go`
  - `BlueprintOpts`:

    ```go
    type BlueprintOpts struct {
        RegistryDir string   // Registry root (must contain registry.yaml)
        Category    string   // Category directory (e.g., "go")
        Name        string   // Blueprint name within category (e.g., "grpc-service")
        Description string   // Defaults to TODO placeholder
        Tags        []string // Defaults to [category]
    }
    ```

  - `BlueprintResult`:

    ```go
    type BlueprintResult struct {
        BlueprintDir  string // Absolute path to created blueprint directory
        BlueprintYAML string // Absolute path to created blueprint.yaml
        RegistryYAML  string // Absolute path to updated registry.yaml
    }
    ```

  - Add `RunBlueprint(opts *BlueprintOpts) (*BlueprintResult, error)`
    stub returning `nil, nil`.

- [x] **1.2 Implement path parsing and validation**
  - In `RunBlueprint()`:
    1. Validate `RegistryDir` non-empty.
    2. Resolve to absolute path via `filepath.Abs()`.
    3. Verify `registry.yaml` exists. If not, error:
       `"registry.yaml not found at %s; run forge registry init first"`.
    4. Validate both `Category` and `Name` non-empty. Error:
       `"both category and name are required"`.
    5. Construct blueprint path: `<category>/<name>`.
    6. Construct absolute blueprint dir.
    7. Check `blueprint.yaml` doesn't already exist. If so, error:
       `"blueprint.yaml already exists at %s"`.
  - Add `parseBlueprintPath(arg) (category, name string, err error)`
    helper that splits a `category/name` arg.

- [x] **1.3 Implement blueprint.yaml generation**
  - Define a `blueprintScaffoldTemplate` const with rich YAML
    template using `fmt.Sprintf` placeholders for name, description,
    tags.
  - Template generates `apiVersion: v1`, name `<category>-<name>`,
    description, version `"0.1.0"`, tags, `project_name`/`license`
    variables, commented `conditions`, `hooks.post_create`,
    `sync.managed_files`, `sync.ignore`, `rename`.
  - `writeBlueprintYAML(path, name, description, tags) error`:
    1. Format template.
    2. Round-trip via `yaml.Unmarshal` + `config.ValidateBlueprint()`
       (matches `registrycmd.writeRegistryYAML()` pattern).
    3. Write with `0o644`.
  - `formatTags([]string) string` produces YAML inline array string.
  - Default `tags` to `[]string{category}` if `opts.Tags` empty.
  - Default `description` to `"TODO: Add a description for this blueprint"`.

- [x] **1.4 Implement starter template file creation**
  - `createStarterTemplate(blueprintDir string) error`:
    1. Create `{{project_name}}/` directory (literal name with `{{}}`).
    2. Write `README.md.tmpl`:

       ```
       # {{ .project_name }}

       {{ .description }}

       ## Getting Started

       TODO: Add getting started instructions.
       ```

    3. Use `0o750` for dirs, `0o644` for files.

- [x] **1.5 Implement category defaults directory creation**
  - `ensureCategoryDefaults(registryDir, category string) error`:
    1. Path: `filepath.Join(registryDir, category, "_defaults")`.
    2. If exists, no-op.
    3. Otherwise `os.MkdirAll()` and write a `.gitkeep`.
    4. Mirrors existing `createCategory()` pattern.

- [x] **1.6 Implement registry.yaml update**
  - `appendBlueprint(registryDir, opts) error`:
    1. Load `registry.yaml` via `config.LoadRegistry()`.
    2. Duplicate check by `Path` (`category/name`). Error if already
       cataloged.
    3. Append `BlueprintEntry` (`Name`, `Path`, `Description`,
       `Version: "0.1.0"`, `Tags`, `LatestCommit: ""`).
    4. Marshal full `Registry` back via `yaml.Marshal()`.
    5. Write with `0o644`.

- [x] **1.7 Wire up `RunBlueprint()` orchestration**
  - Assemble in order:
    1. Parse/validate inputs.
    2. `os.MkdirAll()` for blueprint directory.
    3. `writeBlueprintYAML()`.
    4. `createStarterTemplate()`.
    5. `ensureCategoryDefaults()`.
    6. `appendBlueprint()`.
    7. Return `BlueprintResult`.

- [x] **1.8 Write unit tests for blueprint scaffolding**
  - File: `internal/registrycmd/blueprint_test.go`
  - All `t.Parallel()`:
    1. **`TestRunBlueprint_BasicScaffold`** — scaffold
       `go/grpc-service` into a temp registry. Assert
       `blueprint.yaml` valid via `config.LoadBlueprint()`, name is
       `go-grpc-service`, version `0.1.0`, tags contain `go`,
       `{{project_name}}/README.md.tmpl` exists with template
       placeholders, `go/_defaults/.gitkeep` exists, `registry.yaml`
       contains entry.
    2. **`TestRunBlueprint_CustomTagsAndDescription`** — scaffold
       with `--tags` and `--description`; assert match in both
       `blueprint.yaml` and `registry.yaml`.
    3. **`TestRunBlueprint_DuplicateGuard`** — scaffold same path
       twice; assert second call errors with `"already exists"`.
    4. **`TestRunBlueprint_MissingRegistry`** — non-existent
       `RegistryDir`; assert error contains `"registry.yaml not
       found"`.
    5. **`TestRunBlueprint_MissingCategoryOrName`** — empty
       `Category` or `Name`; assert error.
    6. **`TestRunBlueprint_CategoryDefaultsAlreadyExist`** —
       pre-create `go/_defaults/`; assert no error and idempotent.
    7. **`TestParseBlueprintPath`** — table-driven: `"go/api"` →
       `("go","api",nil)`, `"go"` → error, `""` → error.

- [x] **1.9 Create Cobra command wiring**
  - File: `cmd/registry_blueprint.go`
  - Package-level flag vars: `regBlueprintCategory`,
    `regBlueprintName`, `regBlueprintDescription`,
    `regBlueprintTags`, `regBlueprintRegistryDir`.
  - `registryBlueprintCmd`:
    - `Use: "blueprint [category/name]"`
    - `Short: "Scaffold a new blueprint in a registry"`
    - `Args: cobra.MaximumNArgs(1)`
    - `RunE: runRegistryBlueprint`
  - In `init()`: register flags (`--category`, `--name`,
    `--description`, `--tags` (StringSliceVar), `--registry-dir`
    default `"."`); `registryCmd.AddCommand(registryBlueprintCmd)`.
  - `runRegistryBlueprint`:
    1. If positional arg, `registrycmd.ParseBlueprintPath()`;
       overrides flags.
    2. Otherwise use `--category` and `--name`.
    3. Construct `BlueprintOpts` and call `RunBlueprint()`.
    4. Print success via `ui.NewWriter(noColor)`:
       - `Successf("Blueprint scaffolded at %s", result.BlueprintDir)`
       - `Infof("Edit %s to customize your blueprint", result.BlueprintYAML)`
       - `Infof("Run: forge registry update --registry-dir %s", registryDir)`

- [x] **1.10 Manual verification**
  - Verify:

    ```bash
    make build

    build/bin/forge registry init /tmp/test-bp-reg \
      --name "Test" --category go --category rust

    build/bin/forge registry blueprint go/grpc-service \
      --description "gRPC service with protobuf" \
      --tags go,grpc,api \
      --registry-dir /tmp/test-bp-reg

    ls /tmp/test-bp-reg/go/grpc-service/
    cat /tmp/test-bp-reg/go/grpc-service/blueprint.yaml
    cat /tmp/test-bp-reg/registry.yaml
    ls "/tmp/test-bp-reg/go/grpc-service/{{project_name}}/"

    build/bin/forge create go/grpc-service \
      --registry-dir /tmp/test-bp-reg \
      --defaults --no-hooks \
      --set project_name=my-svc \
      -o /tmp/test-svc --force
    ls /tmp/test-svc/
    cat /tmp/test-svc/README.md

    build/bin/forge registry blueprint \
      --category rust --name web-service \
      --registry-dir /tmp/test-bp-reg
    cat /tmp/test-bp-reg/registry.yaml
    ```

#### Success Criteria — Phase 1

1. `make check` passes.
2. `forge registry blueprint go/grpc-service --registry-dir <reg>`
   creates `blueprint.yaml` (validates), `{{project_name}}/README.md.tmpl`,
   `<category>/_defaults/.gitkeep`, and a new entry in
   `registry.yaml`.
3. The scaffolded blueprint is usable: `forge create
   go/grpc-service --registry-dir <reg> --defaults --set
   project_name=test -o /tmp/out --force` succeeds.
4. Duplicate path returns clear error.
5. Missing `registry.yaml` returns clear error.
6. Both positional and flag-based forms work identically.

### Phase 2: `forge registry update`

Walk all blueprints, compare versions and commits against
`registry.yaml`, update stale metadata.

#### Tasks

- [x] **2.1 Define `UpdateOpts`, `UpdateResult`, status types**
  - File: `internal/registrycmd/update.go`
  - Status constants: `StatusUpToDate`, `StatusVersionChanged`,
    `StatusFilesChanged`, `StatusBothChanged`, `StatusMissing`.
  - `UpdateOpts { RegistryDir string; Check bool }`.
  - `BlueprintReport { Path, Status, RegistryVersion,
    BlueprintVersion, RegistryCommit, LatestCommit }`.
  - `UpdateResult { Reports []BlueprintReport; Updated int; Stale int }`.

- [x] **2.2 Implement git commit resolution**
  - `latestCommitForPath(registryDir, bpPath) (string, error)`:
    1. Run `git -C <registryDir> log -1 --format=%H -- <bpPath>/`.
    2. Parse stdout, trim.
    3. If command fails (not a git repo), error:
       `"registry update requires a git repository: %w"`.
    4. If no commits touch path (empty output), return `("", nil)`.
  - `isGitRepo(dir) bool`:
    1. Run `git -C <dir> rev-parse --git-dir`.
    2. Return `true` on exit 0.
  - Use `os/exec.CommandContext` with `context.Background()` matching
    `registrycmd.gitInit()`.

- [x] **2.3 Implement blueprint status detection**
  - `detectStatus(registryDir, entry) BlueprintReport`:
    1. Construct `blueprint.yaml` path.
    2. If file missing → `StatusMissing`.
    3. Load via `config.LoadBlueprint()`. On error → warn, return
       `StatusMissing`.
    4. `latestCommitForPath()` for actual latest commit.
    5. Compare `entry.Version` vs `bp.Version`, `entry.LatestCommit`
       vs `latestCommit`.
    6. Determine status (both match / version differs / commit
       differs / both differ).
    7. Populate and return `BlueprintReport`.

- [x] **2.4 Implement registry.yaml update logic**
  - `updateRegistryEntries(registryDir, reg, reports) int`:
    1. Iterate reports.
    2. For non-up-to-date and non-missing, find matching entry in
       `reg.Blueprints` by `Path`.
    3. Set `entry.Version = report.BlueprintVersion`.
    4. Set `entry.LatestCommit = report.LatestCommit`.
    5. Return count.
  - `writeRegistry(registryDir, reg) error`:
    1. Marshal via `yaml.Marshal()`.
    2. Write to `registry.yaml` with `0o644`.

- [x] **2.5 Wire up `RunUpdate()` orchestration**
  - `RunUpdate(opts) (*UpdateResult, error)`:
    1. Validate `RegistryDir` non-empty, resolve absolute.
    2. Verify `registry.yaml` exists. If not, error:
       `"registry.yaml not found at %s; run forge registry init first"`.
    3. Verify git repo. If not, error: `"registry update requires a
       git repository"`.
    4. Load via `config.LoadRegistry()`.
    5. For each entry, `detectStatus()`; collect reports.
    6. Count stale (status != `StatusUpToDate`).
    7. If `!opts.Check` and stale > 0: `updateRegistryEntries()` +
       `writeRegistry()`.
    8. Return `UpdateResult`.

- [x] **2.6 Write unit tests for update logic**
  - File: `internal/registrycmd/update_test.go`
  - Helper: `setupGitRegistry(t)` — creates temp dir, scaffolds
    registry, adds blueprint, `git init`/`add`/`commit`. Calls
    `t.Helper()`.
  - Tests:
    1. **`TestRunUpdate_AllUpToDate`** — assert all `StatusUpToDate`,
       `Updated == 0`, `Stale == 0`.
    2. **`TestRunUpdate_VersionChanged`** — modify version, commit,
       run update; assert `StatusVersionChanged` and registry entry
       updated.
    3. **`TestRunUpdate_FilesChanged`** — modify template (not
       version), commit; assert `StatusFilesChanged`, commit
       updated, version unchanged.
    4. **`TestRunUpdate_BothChanged`** — modify version AND template;
       assert `StatusBothChanged`, both fields updated.
    5. **`TestRunUpdate_MissingBlueprint`** — bogus entry in
       `registry.yaml`; assert `StatusMissing`, no error.
    6. **`TestRunUpdate_CheckMode_Clean`** — all up-to-date; check
       returns `Stale == 0`.
    7. **`TestRunUpdate_CheckMode_Stale`** — modify version; check
       returns `Stale > 0`, `Updated == 0`, file unchanged.
    8. **`TestRunUpdate_NotGitRepo`** — non-git dir; error contains
       `"requires a git repository"`.
    9. **`TestRunUpdate_MissingRegistryYAML`** — empty dir; error
       contains `"registry.yaml not found"`.

- [x] **2.7 Create Cobra command wiring**
  - File: `cmd/registry_update.go`
  - Package-level flag vars: `regUpdateRegistryDir`,
    `regUpdateCheck`.
  - `registryUpdateCmd`:
    - `Use: "update"`
    - `Short: "Update blueprint metadata in registry.yaml"`
    - `Args: cobra.NoArgs`
    - `RunE: runRegistryUpdate`
  - In `init()`: register `--registry-dir` (default `"."`),
    `--check`; `registryCmd.AddCommand(registryUpdateCmd)`.
  - `runRegistryUpdate`:
    1. Construct `UpdateOpts`.
    2. Call `RunUpdate()`.
    3. Print summary table with aligned columns.
    4. Normal mode: print each status; if updated > 0:
       `Successf("Updated registry.yaml (%d blueprints updated)", ...)`;
       else `Info("All blueprints up to date")`.
    5. Check mode: if stale > 0:
       `Errorf("Registry metadata is stale (%d blueprints need update)", ...)`,
       return error from `RunE` (Cobra exits 1); else
       `Successf("All blueprints up to date")`.

- [x] **2.8 Manual verification**
  - Verify:

    ```bash
    make build

    rm -rf /tmp/test-update-reg
    build/bin/forge registry init /tmp/test-update-reg \
      --name "Update Test" --category go
    build/bin/forge registry blueprint go/api \
      --description "Go API" --tags go,api \
      --registry-dir /tmp/test-update-reg

    cd /tmp/test-update-reg
    git init && git add -A && git commit -m "init"

    build/bin/forge registry update
    echo "Exit: $?"   # 0

    build/bin/forge registry update --check
    echo "Exit: $?"   # 0

    sed -i '' 's/version: "0.1.0"/version: "0.2.0"/' go/api/blueprint.yaml
    git add -A && git commit -m "bump api version"

    build/bin/forge registry update --check
    echo "Exit: $?"   # 1

    build/bin/forge registry update
    cat registry.yaml  # version: 0.2.0 + new commit

    build/bin/forge registry update --check
    echo "Exit: $?"   # 0
    ```

#### Success Criteria — Phase 2

1. `make check` passes.
2. Clean registry → `forge registry update` prints "up to date", no
   changes.
3. After version bump and commit → update writes new version + commit.
4. After file change without version bump → update writes new commit,
   warns about unchanged version.
5. `--check` clean → exit 0.
6. `--check` stale → exit 1 with list of stale entries.
7. `--check` does NOT modify `registry.yaml`.
8. Non-git directory → clear error.
9. Missing-on-disk paths → `missing` status, graceful skip.

### Phase 3: Documentation & Polish

#### Tasks

- [x] **3.1 Update `CLAUDE.md`**
  - In Architecture, extend `internal/registrycmd/` bullet to mention
    `forge registry blueprint` and `forge registry update`.
  - In `cmd/` bullet, add the two new commands.

- [x] **3.2 Update `README.md`**
  - Add to Quick Start after `registry init` example:

    ```bash
    forge registry blueprint go/grpc-service --registry-dir ./my-registry
    forge registry update --registry-dir ./my-registry
    ```

  - Add Commands table rows.

- [x] **3.3 Update `docs/REGISTRY_SETUP.md`** (now DESIGN-0002)
  - Add "Adding Blueprints" section with the `registry blueprint`
    workflow.
  - Add "Keeping Metadata in Sync" section documenting `registry
    update` and `--check` for CI; include a GitHub Actions snippet:

    ```yaml
    - name: Check registry metadata
      run: forge registry update --check
    ```

- [x] **3.4 Run full CI check** — `make check`, `make build`, `make ci`.

#### Success Criteria — Phase 3

1. `make check` passes.
2. `CLAUDE.md` reflects new `registrycmd` scope.
3. `README.md` Quick Start and Commands table include both commands.
4. `docs/REGISTRY_SETUP.md` (DESIGN-0002) has working examples.
5. No stale references to `forge init --registry` as the sole
   blueprint-add path.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `cmd/registry_blueprint.go` | Create | Cobra command |
| `cmd/registry_update.go` | Create | Cobra command |
| `internal/registrycmd/blueprint.go` | Create | Scaffold logic |
| `internal/registrycmd/blueprint_test.go` | Create | Tests |
| `internal/registrycmd/update.go` | Create | Update + check logic |
| `internal/registrycmd/update_test.go` | Create | Tests with git fixtures |
| `CLAUDE.md` | Modify | Architecture notes |
| `README.md` | Modify | Quick Start, Commands |
| `docs/REGISTRY_SETUP.md` | Modify | New workflows (now DESIGN-0002) |

## Testing Plan

- Unit tests for `RunBlueprint` and `RunUpdate` covering success and
  failure paths (Phase 1 and 2 task lists above).
- Tests use `t.TempDir()`; git operations use a hermetic env
  (`GIT_AUTHOR_NAME`, `GIT_COMMITTER_NAME`, etc.) so they run in CI
  and don't depend on user git config.

## Appendix: Key Patterns to Follow

| Pattern | Example | Location |
|---------|---------|----------|
| Opts/Result structs | `registrycmd.Opts` / `registrycmd.Result` | `internal/registrycmd/registrycmd.go` |
| Cobra command file per subcommand | `cmd/registry_init.go` | `cmd/registry_init.go` |
| Flag vars at package level | `regInitName`, `regInitDescription` | `cmd/registry_init.go` |
| YAML round-trip validation | `yaml.Unmarshal` + `config.Validate*()` | `registrycmd.writeRegistryYAML()` |
| `ui.NewWriter(noColor)` for output | `w.Successf(...)` / `w.Infof(...)` | `cmd/registry_init.go` |
| File permissions | dirs `0o750`, files `0o644` | throughout `registrycmd.go` |
| Error wrapping | `fmt.Errorf("context: %w", err)` | throughout |
| Test structure | `t.Parallel()`, `t.TempDir()`, `testify` | `registrycmd_test.go` |
| `filepath.Abs()` early | Resolve paths at entry point | `registrycmd.Run()` |
| Guard existing files | `os.Stat()` before write | `registrycmd.Run()` |
| Import ordering | stdlib → third-party → `github.com/donaldgifford` | enforced by gci |

### Git Operations in Tests

```go
func initGitRepo(t *testing.T, dir string) {
    t.Helper()
    runGit(t, dir, "init")
    runGit(t, dir, "add", "-A")
    runGit(t, dir, "commit", "-m", "init")
}

func runGit(t *testing.T, dir string, args ...string) string {
    t.Helper()
    cmd := exec.Command("git", args...)
    cmd.Dir = dir
    cmd.Env = append(os.Environ(),
        "GIT_AUTHOR_NAME=test",
        "GIT_AUTHOR_EMAIL=test@test.com",
        "GIT_COMMITTER_NAME=test",
        "GIT_COMMITTER_EMAIL=test@test.com",
    )
    out, err := cmd.CombinedOutput()
    require.NoError(t, err, "git %v failed: %s", args, out)
    return strings.TrimSpace(string(out))
}
```

## References

- [PLAN-0001 — Registry Blueprint & Update Commands](../plan/0001-registry-blueprint-and-update-commands.md)
- [DESIGN-0002 — Registry Layout & Defaults Inheritance](../design/0002-registry-layout-and-defaults-inheritance.md)
- [RFC-0001 — Forge: Project Scaffolding CLI](../rfc/0001-forge-project-scaffolding-cli.md)
