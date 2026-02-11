# Registry Commands — Implementation Guide

Detailed implementation tasks for the features described in
[REGISTRY_COMMANDS_PLAN.md](./REGISTRY_COMMANDS_PLAN.md). Each phase
produces a working, tested feature. Tasks are ordered by dependency — work
through them top-to-bottom.

---

## Phase 1: `forge registry blueprint`

Scaffold a full blueprint directory inside a registry with category
awareness, a rich starter `blueprint.yaml`, template files, and automatic
`registry.yaml` updates.

### Tasks

- [x] **1.1 Define `BlueprintOpts` and `BlueprintResult` types**
  - File: `internal/registrycmd/blueprint.go`
  - Create a new `BlueprintOpts` struct:
    ```go
    type BlueprintOpts struct {
        RegistryDir string   // Registry root (must contain registry.yaml)
        Category    string   // Category directory (e.g., "go")
        Name        string   // Blueprint name within category (e.g., "grpc-service")
        Description string   // Blueprint description (optional, defaults to TODO placeholder)
        Tags        []string // Tags for registry index (optional, defaults to [category])
    }
    ```
  - Create a `BlueprintResult` struct:
    ```go
    type BlueprintResult struct {
        BlueprintDir string // Absolute path to created blueprint directory
        BlueprintYAML string // Absolute path to created blueprint.yaml
        RegistryYAML string // Absolute path to updated registry.yaml
    }
    ```
  - Add a `RunBlueprint(opts *BlueprintOpts) (*BlueprintResult, error)`
    function stub that returns `nil, nil`.

- [x] **1.2 Implement path parsing and validation**
  - File: `internal/registrycmd/blueprint.go`
  - In `RunBlueprint()`, implement:
    1. Validate `RegistryDir` is non-empty.
    2. Resolve `RegistryDir` to absolute path via `filepath.Abs()`.
    3. Verify `registry.yaml` exists at `RegistryDir`. If not, return
       error: `"registry.yaml not found at %s; run forge registry init first"`.
    4. Validate that both `Category` and `Name` are non-empty. Return
       error: `"both category and name are required"` if either is missing.
    5. Construct blueprint path: `<category>/<name>` (e.g., `go/grpc-service`).
    6. Construct absolute blueprint dir:
       `filepath.Join(registryDir, category, name)`.
    7. Check if `blueprint.yaml` already exists at that path. If so,
       return error: `"blueprint.yaml already exists at %s"`.
  - Add a `parseBlueprintPath(arg string) (category, name string, err error)`
    helper that splits a `category/name` positional arg. Expects exactly
    one `/` separator. Returns error if the format is invalid.

- [x] **1.3 Implement blueprint.yaml generation**
  - File: `internal/registrycmd/blueprint.go`
  - Define a `blueprintScaffoldTemplate` const with a rich YAML template.
    Use `fmt.Sprintf` with the following placeholders: name (hyphenated,
    e.g., `go-grpc-service`), description, tags formatted as YAML array.
  - Template content to generate:
    ```yaml
    apiVersion: v1
    name: "<category>-<name>"
    description: "<description>"
    version: "0.1.0"
    tags: [<tags>]

    variables:
      - name: project_name
        description: "Name of the project"
        type: string
        required: true
        validate: "^[a-z][a-z0-9-]*$"

      - name: license
        description: "License type"
        type: choice
        choices: ["MIT", "Apache-2.0", "BSD-3-Clause", "none"]
        default: "Apache-2.0"

    # conditions:
    #   - when: "{{ .some_variable }}"
    #     exclude:
    #       - "optional-dir/"

    hooks:
      post_create:
        - "git init"

    sync:
      managed_files: []
      ignore: []

    rename:
      "{{project_name}}/": "."
    ```
  - Implement a `writeBlueprintYAML(path, name, description string, tags []string) error`
    function that:
    1. Formats the template with provided values.
    2. Validates via `yaml.Unmarshal` + `config.ValidateBlueprint()` round-trip
       (matches the pattern in `registrycmd.writeRegistryYAML()`).
    3. Writes to disk with `0o644` permissions.
  - Implement a `formatTags(tags []string) string` helper that produces a
    YAML inline array string like `"go", "grpc", "api"` for embedding in
    the template.
  - Default `tags` to `[]string{category}` if opts.Tags is empty.
  - Default `description` to `"TODO: Add a description for this blueprint"`
    if opts.Description is empty.

- [x] **1.4 Implement starter template file creation**
  - File: `internal/registrycmd/blueprint.go`
  - Create a `createStarterTemplate(blueprintDir string) error` function
    that:
    1. Creates `{{project_name}}/` directory inside the blueprint dir.
       Note: this is a literal directory name containing `{{` and `}}` —
       use it as-is in `os.MkdirAll()`.
    2. Writes `README.md.tmpl` inside that directory with content:
       ```
       # {{ .project_name }}

       {{ .description }}

       ## Getting Started

       TODO: Add getting started instructions.
       ```
    3. Uses `0o750` for directories, `0o644` for files.

- [x] **1.5 Implement category defaults directory creation**
  - File: `internal/registrycmd/blueprint.go`
  - Create an `ensureCategoryDefaults(registryDir, category string) error`
    function that:
    1. Constructs path: `filepath.Join(registryDir, category, "_defaults")`.
    2. If the directory already exists, return nil (no-op).
    3. Otherwise, create it with `os.MkdirAll()` and write a `.gitkeep`
       file inside it.
    4. This matches the existing `createCategory()` pattern in
       `registrycmd.go`.

- [x] **1.6 Implement registry.yaml update**
  - File: `internal/registrycmd/blueprint.go`
  - Create an `appendBlueprint(registryDir string, opts *BlueprintOpts) error`
    function that:
    1. Loads `registry.yaml` using `config.LoadRegistry()`.
    2. Checks for duplicate entries by comparing `Path` field against
       `category/name`. Return error if already cataloged:
       `"blueprint %s already exists in registry.yaml"`.
    3. Appends a `config.BlueprintEntry`:
       ```go
       config.BlueprintEntry{
           Name:        category + "/" + name,
           Path:        category + "/" + name,
           Description: description,
           Version:     "0.1.0",
           Tags:        tags,
           LatestCommit: "",
       }
       ```
    4. Marshals the full `config.Registry` struct back to YAML via
       `yaml.Marshal()`.
    5. Writes to `registry.yaml` with `0o644` permissions.
  - Note: Use the same load-append-marshal pattern as
    `initcmd.appendToRegistryIndex()`, but load via `config.LoadRegistry()`
    instead of raw YAML unmarshal since we know the file exists (validated
    in step 1.2).

- [x] **1.7 Wire up `RunBlueprint()` orchestration**
  - File: `internal/registrycmd/blueprint.go`
  - Assemble `RunBlueprint()` to call the functions from 1.2–1.6 in order:
    1. Parse/validate inputs.
    2. `os.MkdirAll()` for blueprint directory.
    3. `writeBlueprintYAML()`.
    4. `createStarterTemplate()`.
    5. `ensureCategoryDefaults()`.
    6. `appendBlueprint()`.
    7. Return `BlueprintResult` with absolute paths.

- [x] **1.8 Write unit tests for blueprint scaffolding**
  - File: `internal/registrycmd/blueprint_test.go`
  - Tests to write (all `t.Parallel()`):
    1. **`TestRunBlueprint_BasicScaffold`** — scaffold `go/grpc-service`
       into a temp registry created by `registrycmd.Run()`. Assert:
       - `blueprint.yaml` exists and is valid via `config.LoadBlueprint()`.
       - Blueprint name is `"go-grpc-service"`.
       - Version is `"0.1.0"`.
       - Tags contain `"go"`.
       - `{{project_name}}/README.md.tmpl` exists and contains
         `{{ .project_name }}`.
       - `go/_defaults/.gitkeep` exists.
       - `registry.yaml` contains entry with `path: go/grpc-service`.
    2. **`TestRunBlueprint_CustomTagsAndDescription`** — scaffold with
       `--tags` and `--description`. Assert tags and description match in
       both `blueprint.yaml` and `registry.yaml`.
    3. **`TestRunBlueprint_DuplicateGuard`** — scaffold same path twice.
       Assert second call returns error containing `"already exists"`.
    4. **`TestRunBlueprint_MissingRegistry`** — call with a non-existent
       `RegistryDir`. Assert error contains `"registry.yaml not found"`.
    5. **`TestRunBlueprint_MissingCategoryOrName`** — call with empty
       `Category` or `Name`. Assert error.
    6. **`TestRunBlueprint_CategoryDefaultsAlreadyExist`** — pre-create
       `go/_defaults/` before calling. Assert no error and directory is
       unchanged (idempotent).
    7. **`TestParseBlueprintPath`** — table-driven test for the path
       parser: `"go/api"` → `("go","api",nil)`, `"go"` → error,
       `"a/b/c"` → error or `("a","b/c",nil)` depending on design,
       `""` → error.
  - Each test creates a fresh temp dir and uses `registrycmd.Run()` to
    bootstrap a minimal registry first (matches existing test patterns).

- [x] **1.9 Create Cobra command wiring**
  - File: `cmd/registry_blueprint.go`
  - Define package-level flag variables:
    ```go
    var (
        regBlueprintCategory    string
        regBlueprintName        string
        regBlueprintDescription string
        regBlueprintTags        []string
        regBlueprintRegistryDir string
    )
    ```
  - Define `registryBlueprintCmd` as `&cobra.Command{}`:
    - `Use: "blueprint [category/name]"`
    - `Short: "Scaffold a new blueprint in a registry"`
    - `Long:` explains both positional and flag-based usage.
    - `Args: cobra.MaximumNArgs(1)`
    - `RunE: runRegistryBlueprint`
  - In `init()`:
    - Register flags: `--category`, `--name`, `--description`,
      `--tags` (StringSliceVar), `--registry-dir` (default `"."`).
    - `registryCmd.AddCommand(registryBlueprintCmd)`
  - Implement `runRegistryBlueprint(_ *cobra.Command, args []string) error`:
    1. If positional arg provided, call `registrycmd.ParseBlueprintPath()`
       to extract category and name. These override flags.
    2. Otherwise use `--category` and `--name` flags.
    3. Construct `BlueprintOpts` and call `registrycmd.RunBlueprint()`.
    4. Print success messages via `ui.NewWriter(noColor)`:
       - `Successf("Blueprint scaffolded at %s", result.BlueprintDir)`
       - `Infof("Edit %s to customize your blueprint", result.BlueprintYAML)`
       - `Infof("Run: forge registry update --registry-dir %s", registryDir)`

- [x] **1.10 Manual verification**
  - Run the following and verify expected output:
    ```bash
    make build

    # Set up test registry
    build/bin/forge registry init /tmp/test-bp-reg \
      --name "Test" --category go --category rust

    # Scaffold a blueprint (positional form)
    build/bin/forge registry blueprint go/grpc-service \
      --description "gRPC service with protobuf" \
      --tags go,grpc,api \
      --registry-dir /tmp/test-bp-reg

    # Verify structure
    ls /tmp/test-bp-reg/go/grpc-service/
    cat /tmp/test-bp-reg/go/grpc-service/blueprint.yaml
    cat /tmp/test-bp-reg/registry.yaml
    ls "/tmp/test-bp-reg/go/grpc-service/{{project_name}}/"

    # Verify the blueprint is usable end-to-end with forge create
    build/bin/forge create go/grpc-service \
      --registry-dir /tmp/test-bp-reg \
      --defaults --no-hooks \
      --set project_name=my-svc \
      -o /tmp/test-svc --force
    ls /tmp/test-svc/
    cat /tmp/test-svc/README.md

    # Scaffold a second blueprint (flag form)
    build/bin/forge registry blueprint \
      --category rust --name web-service \
      --registry-dir /tmp/test-bp-reg
    cat /tmp/test-bp-reg/registry.yaml
    ```

### Success Criteria — Phase 1

All of the following must be true:

1. `make check` passes (lint + all existing tests + new tests).
2. `forge registry blueprint go/grpc-service --registry-dir <reg>` creates:
   - `go/grpc-service/blueprint.yaml` that passes
     `config.LoadBlueprint()` validation.
   - `go/grpc-service/{{project_name}}/README.md.tmpl` with template
     placeholders.
   - `go/_defaults/.gitkeep` (or leaves existing `_defaults/` untouched).
   - A new entry in `registry.yaml` with matching path, name, version,
     description, and tags.
3. The scaffolded blueprint is usable: `forge create go/grpc-service
   --registry-dir <reg> --defaults --set project_name=test -o /tmp/out
   --force` succeeds and `/tmp/out/README.md` contains rendered content.
4. Duplicate blueprint path returns a clear error.
5. Missing `registry.yaml` returns a clear error with guidance.
6. Both positional (`go/grpc-service`) and flag-based (`--category go
   --name grpc-service`) forms work identically.

---

## Phase 2: `forge registry update`

Walk all blueprints in a registry, compare `blueprint.yaml` versions and
git commits against `registry.yaml` entries, and update stale metadata.

### Tasks

- [x] **2.1 Define `UpdateOpts`, `UpdateResult`, and status types**
  - File: `internal/registrycmd/update.go`
  - Define status constants:
    ```go
    type BlueprintStatus string

    const (
        StatusUpToDate      BlueprintStatus = "up-to-date"
        StatusVersionChanged BlueprintStatus = "version-changed"
        StatusFilesChanged  BlueprintStatus = "files-changed"
        StatusBothChanged   BlueprintStatus = "both-changed"
        StatusMissing       BlueprintStatus = "missing"
    )
    ```
  - Define `UpdateOpts`:
    ```go
    type UpdateOpts struct {
        RegistryDir string // Registry root (must contain registry.yaml)
        Check       bool   // Check-only mode — don't write, exit 1 if stale
    }
    ```
  - Define per-blueprint status report:
    ```go
    type BlueprintReport struct {
        Path           string
        Status         BlueprintStatus
        RegistryVersion string // Version currently in registry.yaml
        BlueprintVersion string // Version currently in blueprint.yaml
        RegistryCommit  string // Commit currently in registry.yaml
        LatestCommit    string // Actual latest commit from git
    }
    ```
  - Define `UpdateResult`:
    ```go
    type UpdateResult struct {
        Reports []BlueprintReport
        Updated int  // Count of entries updated (0 in check mode)
        Stale   int  // Count of entries that are out of date
    }
    ```

- [x] **2.2 Implement git commit resolution**
  - File: `internal/registrycmd/update.go`
  - Create `latestCommitForPath(registryDir, bpPath string) (string, error)`:
    1. Run `git -C <registryDir> log -1 --format=%H -- <bpPath>/`.
    2. Parse stdout, trim whitespace.
    3. If the command fails (not a git repo), return
       `("", fmt.Errorf("registry update requires a git repository: %w", err))`.
    4. If no commits touch that path (empty output), return `("", nil)` —
       this means the path exists but has no git history yet (uncommitted
       files).
  - Create `isGitRepo(dir string) bool`:
    1. Run `git -C <dir> rev-parse --git-dir`.
    2. Return `true` if exit code is 0.
  - Use `os/exec.CommandContext` with `context.Background()` matching the
    existing pattern in `registrycmd.gitInit()`.

- [x] **2.3 Implement blueprint status detection**
  - File: `internal/registrycmd/update.go`
  - Create `detectStatus(registryDir string, entry config.BlueprintEntry) BlueprintReport`:
    1. Construct `blueprint.yaml` path:
       `filepath.Join(registryDir, entry.Path, "blueprint.yaml")`.
    2. If the file doesn't exist, return report with `StatusMissing`.
    3. Load blueprint via `config.LoadBlueprint()`. If load fails, log
       warning and return `StatusMissing`.
    4. Call `latestCommitForPath()` to get the actual latest commit.
    5. Compare:
       - `entry.Version` vs `bp.Version`
       - `entry.LatestCommit` vs `latestCommit`
    6. Determine status:
       - Both match → `StatusUpToDate`
       - Version differs, commit matches → `StatusVersionChanged`
       - Version matches, commit differs → `StatusFilesChanged`
       - Both differ → `StatusBothChanged`
    7. Populate and return `BlueprintReport`.

- [x] **2.4 Implement registry.yaml update logic**
  - File: `internal/registrycmd/update.go`
  - Create `updateRegistryEntries(registryDir string, reg *config.Registry, reports []BlueprintReport) int`:
    1. Iterate over `reports`.
    2. For each non-up-to-date and non-missing report, find the matching
       entry in `reg.Blueprints` by `Path`.
    3. Set `entry.Version = report.BlueprintVersion`.
    4. Set `entry.LatestCommit = report.LatestCommit`.
    5. Count and return number of entries updated.
  - Create `writeRegistry(registryDir string, reg *config.Registry) error`:
    1. Marshal `reg` via `yaml.Marshal()`.
    2. Write to `filepath.Join(registryDir, "registry.yaml")` with `0o644`.

- [x] **2.5 Wire up `RunUpdate()` orchestration**
  - File: `internal/registrycmd/update.go`
  - Implement `RunUpdate(opts *UpdateOpts) (*UpdateResult, error)`:
    1. Validate `RegistryDir` is non-empty, resolve to absolute path.
    2. Verify `registry.yaml` exists. If not, return error:
       `"registry.yaml not found at %s; run forge registry init first"`.
    3. Verify it's a git repo via `isGitRepo()`. If not, return error:
       `"registry update requires a git repository"`.
    4. Load registry via `config.LoadRegistry()`.
    5. For each `BlueprintEntry`, call `detectStatus()` and collect
       reports.
    6. Count stale entries (status != `StatusUpToDate`).
    7. If `opts.Check` is false and stale > 0:
       - Call `updateRegistryEntries()`.
       - Call `writeRegistry()`.
    8. Return `UpdateResult` with reports, updated count, and stale count.

- [x] **2.6 Write unit tests for update logic**
  - File: `internal/registrycmd/update_test.go`
  - **Test helper**: `setupGitRegistry(t *testing.T) string` — creates a
    temp dir, runs `registrycmd.Run()` to scaffold a registry, runs
    `registrycmd.RunBlueprint()` to add a blueprint, then `git init`,
    `git add -A`, `git commit -m "init"`. Returns the registry dir path.
    Must call `t.Helper()`.
  - Tests to write (all `t.Parallel()` where safe — note git operations
    may need sequential execution within a test):
    1. **`TestRunUpdate_AllUpToDate`** — setup git registry, run update.
       Assert all reports are `StatusUpToDate`, `Updated == 0`,
       `Stale == 0`.
    2. **`TestRunUpdate_VersionChanged`** — setup git registry, modify
       `blueprint.yaml` version field, commit. Run update. Assert status
       is `StatusVersionChanged`. Assert `registry.yaml` entry now has new
       version and new commit hash.
    3. **`TestRunUpdate_FilesChanged`** — setup git registry, modify a
       template file (not version), commit. Run update. Assert status is
       `StatusFilesChanged`. Assert `registry.yaml` commit updated but
       version unchanged.
    4. **`TestRunUpdate_BothChanged`** — modify version AND template file,
       commit. Assert `StatusBothChanged`. Assert both fields updated.
    5. **`TestRunUpdate_MissingBlueprint`** — manually add a bogus entry
       to `registry.yaml` for a path that doesn't exist. Assert status is
       `StatusMissing`, no error returned (graceful skip).
    6. **`TestRunUpdate_CheckMode_Clean`** — all up-to-date. Check mode
       returns `Stale == 0`.
    7. **`TestRunUpdate_CheckMode_Stale`** — modify version, don't run
       update. Check mode returns `Stale > 0`, `Updated == 0`, and
       `registry.yaml` is NOT modified on disk.
    8. **`TestRunUpdate_NotGitRepo`** — point at a registry dir that is
       not a git repo. Assert error contains
       `"requires a git repository"`.
    9. **`TestRunUpdate_MissingRegistryYAML`** — point at empty dir.
       Assert error contains `"registry.yaml not found"`.

- [x] **2.7 Create Cobra command wiring**
  - File: `cmd/registry_update.go`
  - Define package-level flag variables:
    ```go
    var (
        regUpdateRegistryDir string
        regUpdateCheck       bool
    )
    ```
  - Define `registryUpdateCmd` as `&cobra.Command{}`:
    - `Use: "update"`
    - `Short: "Update blueprint metadata in registry.yaml"`
    - `Long:` explains that it syncs version and commit fields from
      blueprints and git history into registry.yaml. Mention `--check`
      for CI mode.
    - `Args: cobra.NoArgs`
    - `RunE: runRegistryUpdate`
  - In `init()`:
    - Register `--registry-dir` (default `"."`) and `--check` (bool).
    - `registryCmd.AddCommand(registryUpdateCmd)`
  - Implement `runRegistryUpdate(_ *cobra.Command, _ []string) error`:
    1. Construct `UpdateOpts` from flags.
    2. Call `registrycmd.RunUpdate()`.
    3. Print summary table using `fmt.Fprintf` with aligned columns.
       Use `ui.NewWriter(noColor)` for status messages.
    4. In normal mode:
       - Print each blueprint's status.
       - If updated > 0: `Successf("Updated registry.yaml (%d blueprints
         updated)", result.Updated)`.
       - If updated == 0: `Info("All blueprints up to date")`.
    5. In check mode:
       - Print each blueprint's status.
       - If stale > 0: `Errorf("Registry metadata is stale (%d blueprints
         need update)", result.Stale)`, then `return fmt.Errorf("...")`.
         Cobra converts RunE errors to non-zero exit. Alternatively return
         a sentinel error or use `os.Exit(1)`. Prefer returning an error
         from `RunE` — Cobra will print it and exit 1.
       - If stale == 0: `Successf("All blueprints up to date")`.

- [ ] **2.8 Manual verification**
  - Run the following and verify expected output:
    ```bash
    make build

    # Create a test registry with a blueprint and git history
    rm -rf /tmp/test-update-reg
    build/bin/forge registry init /tmp/test-update-reg \
      --name "Update Test" --category go
    build/bin/forge registry blueprint go/api \
      --description "Go API" --tags go,api \
      --registry-dir /tmp/test-update-reg

    cd /tmp/test-update-reg
    git init && git add -A && git commit -m "init"

    # All should be up to date
    build/bin/forge registry update
    echo "Exit: $?"   # Should be 0

    # Check mode should also pass
    build/bin/forge registry update --check
    echo "Exit: $?"   # Should be 0

    # Bump version in blueprint.yaml
    sed -i '' 's/version: "0.1.0"/version: "0.2.0"/' \
      go/api/blueprint.yaml
    git add -A && git commit -m "bump api version"

    # Check mode should now fail
    build/bin/forge registry update --check
    echo "Exit: $?"   # Should be 1

    # Update should fix it
    build/bin/forge registry update
    cat registry.yaml  # Should show version: 0.2.0 and new commit

    # Check should now pass again
    build/bin/forge registry update --check
    echo "Exit: $?"   # Should be 0
    ```

### Success Criteria — Phase 2

All of the following must be true:

1. `make check` passes (lint + all existing tests + new tests).
2. `forge registry update` in a clean registry prints "up to date" and
   makes no changes.
3. After bumping `version` in a `blueprint.yaml` and committing, `forge
   registry update` updates the matching entry in `registry.yaml` with
   the new version and latest commit hash.
4. After modifying a template file (without version bump) and committing,
   `forge registry update` updates the commit hash and prints a warning
   about unchanged version.
5. `forge registry update --check` in a clean registry exits 0.
6. `forge registry update --check` with stale metadata exits 1 and prints
   which blueprints are out of date.
7. `forge registry update --check` does NOT modify `registry.yaml`.
8. Running in a non-git directory returns a clear error.
9. Blueprint paths missing from disk are reported as `missing` and skipped
   gracefully.

---

## Phase 3: Documentation & Polish

Update documentation to cover the new commands and ensure consistency
across all docs.

### Tasks

- [ ] **3.1 Update `CLAUDE.md`**
  - File: `CLAUDE.md`
  - In the Architecture section, add to the `internal/registrycmd/`
    bullet:
    ```
    - **internal/registrycmd/** — Registry scaffolding (`forge registry init`),
      blueprint scaffolding (`forge registry blueprint`), and registry
      metadata update (`forge registry update`)
    ```
  - In the `cmd/` bullet, add `registry blueprint` and `registry update`
    to the command list.

- [ ] **3.2 Update `README.md`**
  - File: `README.md`
  - Add to Quick Start section after the registry init example:
    ```bash
    # Add a blueprint to a registry
    forge registry blueprint go/grpc-service --registry-dir ./my-registry

    # Update registry metadata after blueprint changes
    forge registry update --registry-dir ./my-registry
    ```
  - Add rows to the Commands table:
    ```
    | `forge registry blueprint` | Scaffold a new blueprint in a registry |
    | `forge registry update`    | Sync blueprint metadata in registry.yaml |
    ```

- [ ] **3.3 Update `docs/REGISTRY_SETUP.md`**
  - File: `docs/REGISTRY_SETUP.md`
  - Add a section on "Adding Blueprints" that shows the
    `forge registry blueprint` workflow.
  - Add a section on "Keeping Metadata in Sync" that documents:
    - The `forge registry update` command and its output.
    - The `--check` flag for CI pipelines.
    - Example GitHub Actions snippet:
      ```yaml
      - name: Check registry metadata
        run: forge registry update --check
      ```

- [ ] **3.4 Run full CI check**
  - Run `make check` (lint + test).
  - Run `make build`.
  - Run `make ci` if available for the full pipeline.

### Success Criteria — Phase 3

1. `make check` passes.
2. `CLAUDE.md` accurately reflects the new `registrycmd` package scope.
3. `README.md` Quick Start and Commands table include both new commands.
4. `docs/REGISTRY_SETUP.md` has working examples for
   `forge registry blueprint` and `forge registry update --check`.
5. No stale references to the old `forge init --registry` workflow remain
   as the sole way to add blueprints (it still works, but the new command
   is documented as the recommended path for registry maintainers).

---

## Appendix: Key Patterns to Follow

These patterns are established in the existing codebase. New code should
be consistent with them.

### Existing Conventions

| Pattern | Example | Location |
|---------|---------|----------|
| Opts/Result structs | `registrycmd.Opts` / `registrycmd.Result` | `internal/registrycmd/registrycmd.go:17-36` |
| Cobra command file per subcommand | `cmd/registry_init.go` | `cmd/registry_init.go` |
| Flag vars at package level | `regInitName`, `regInitDescription` | `cmd/registry_init.go:10-15` |
| YAML round-trip validation | `yaml.Unmarshal` + `config.Validate*()` | `registrycmd.writeRegistryYAML()` |
| `ui.NewWriter(noColor)` for output | `w.Successf(...)` / `w.Infof(...)` | `cmd/registry_init.go:35,50-56` |
| File permissions | dirs `0o750`, files `0o644` | throughout `registrycmd.go` |
| Error wrapping | `fmt.Errorf("context: %w", err)` | throughout |
| Test structure | `t.Parallel()`, `t.TempDir()`, `testify` | `registrycmd_test.go` |
| `filepath.Abs()` early | Resolve paths at entry point | `registrycmd.Run()` |
| Guard existing files | `os.Stat()` before write | `registrycmd.Run():104` |
| Import ordering | stdlib → third-party → `github.com/donaldgifford` | enforced by gci linter |

### Git Operations in Tests

Tests for `registry update` need real git repos. Pattern:

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
    // Set minimal git config for commits in test repos.
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

This ensures tests don't depend on the user's global git config and work
in CI environments.
