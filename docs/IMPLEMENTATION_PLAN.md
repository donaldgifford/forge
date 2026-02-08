# Implementation Plan: `forge`

This document breaks the PROJECT_PLAN.md into concrete, ordered implementation steps. Each step produces working, testable code. Dependencies between steps are explicit — nothing starts before its prerequisites are done.

---

## Phase 1: Foundation

**Goal:** `forge create go/api` resolves from a local or remote registry, inherits defaults, prompts for variables, renders templates, and writes a working project.

### 1.1 — Project Skeleton and Entry Point [DONE]

Set up the Cobra CLI scaffold with a root command, version flag, and global config structure.

**Files:**

- `cmd/forge/main.go` — entry point, calls `cmd.Execute()`
- `cmd/root.go` — root command with `--verbose`, `--no-color`, `--config` flags
- `cmd/version.go` — `forge version` subcommand, prints build-time ldflags

**Details:**

- Wire ldflags in the Makefile (`-X main.version=...`, `-X main.commit=...`)
- Use `cobra-cli` to generate initial command stubs, then clean up to match project conventions
- Root command initializes a `*slog.Logger` based on `--verbose` flag and passes it via context or a top-level app struct

**Verification:** `make build && ./build/bin/forge version` prints version info. `make lint` passes.

---

### 1.2 — YAML Schema Types and Config Loader [DONE]

Define Go structs for `blueprint.yaml` and `registry.yaml`, plus a loader that reads, unmarshals, and validates them.

**Files:**

- `internal/config/blueprint.go` — `Blueprint` struct matching the full `blueprint.yaml` schema (Section 6 of PROJECT_PLAN)
- `internal/config/registry.go` — `Registry` struct matching the full `registry.yaml` schema (Section 5)
- `internal/config/loader.go` — `LoadBlueprint(path) (*Blueprint, error)` and `LoadRegistry(path) (*Registry, error)`
- `internal/config/validate.go` — validation functions: required fields, regex on `apiVersion`, variable type checking
- `internal/config/loader_test.go`
- `internal/config/validate_test.go`

**Details:**

- Use `gopkg.in/yaml.v3` for parsing
- Struct field tags: `yaml:"field_name"`
- Validate `apiVersion: v1`, required `name` field, variable type must be one of `string | bool | choice | int`
- Variable `validate` field holds a regex pattern string — compile and test it during validation
- `defaults.exclude` is `[]string`, `defaults.override_strategy` is `map[string]string`
- Tool entries include `condition` field (raw template string, evaluated later)

**Verification:** Unit tests load sample YAML fixtures from `testdata/` and assert parsed struct fields. Invalid YAML returns structured errors.

---

### 1.3 — Test Fixtures [DONE]

Create a minimal test registry on disk that all subsequent tests can use.

**Files:**

- `testdata/registry/registry.yaml`
- `testdata/registry/_defaults/.editorconfig`
- `testdata/registry/_defaults/.gitignore.tmpl`
- `testdata/registry/_defaults/LICENSE.tmpl`
- `testdata/registry/go/_defaults/.golangci.yml`
- `testdata/registry/go/_defaults/scripts/lint.sh`
- `testdata/registry/go/api/blueprint.yaml` — declares variables (`project_name`, `go_module`, `use_grpc`, `license`), excludes `.pre-commit-config.yaml`, has one managed file
- `testdata/registry/go/api/{{project_name}}/cmd/main.go.tmpl`
- `testdata/registry/go/api/{{project_name}}/go.mod.tmpl`
- `testdata/registry/go/api/{{project_name}}/README.md.tmpl`

**Details:**

- Keep fixtures minimal — just enough to exercise all three inheritance layers, template rendering, and conditional inclusion
- Include at least one `.tmpl` file at each layer to verify rendering works on defaults too
- Include a `scripts/lint.sh` in both `_defaults/` and `go/_defaults/` to test override behavior

**Verification:** Files exist and are valid YAML/templates. Config loader tests from 1.2 use these fixtures.

---

### 1.4 — Source Fetcher (go-getter) [DONE]

Wrap `hashicorp/go-getter` to fetch registries and (later) tool binaries from any supported source.

go-getter replaces both a dedicated git client and a custom HTTP downloader. It handles git clone/checkout, HTTP downloads, archive extraction, and checksum verification through a unified URL scheme. It shells out to the system `git` binary for git operations.

**Files:**

- `internal/getter/getter.go` — `Getter` struct with methods:
  - `Fetch(src, dest string, opts FetchOpts) error` — download source to dest directory using go-getter
  - `FetchFile(src, dest string, opts FetchOpts) error` — download a single file (for tool binaries)
- `internal/getter/url.go` — helpers to construct go-getter URLs:
  - `RegistryURL(baseURL, subpath, ref string) string` — e.g., `"github.com/acme/blueprints//go/api?ref=v2.1.0"`
  - `ToolURL(source ToolSource, version string, platform Platform) string` — construct download URL for a tool binary
- `internal/getter/getter_test.go`
- `internal/getter/url_test.go`

**Details:**

- go-getter URL format: `github.com/owner/repo//subpath?ref=tag` — the double-slash separates the repo from the subpath, which aligns directly with forge's registry resolution syntax
- `FetchOpts` includes `Ref string` (appended as `?ref=`), `Checksum string` (appended as `?checksum=sha256:...`)
- For registry fetches, go-getter clones via system git and extracts the subpath — no sparse checkout needed since go-getter handles subpath extraction natively
- Auth relies on system git credential helpers and SSH agent (go-getter delegates to system git)
- All operations use `slog` for debug logging
- go-getter's built-in detectors auto-detect GitHub, BitBucket, S3, GCS URLs and normalize them

**Verification:** Integration test fetches a subpath from a local bare git repo (created in test setup), verifies only the requested subpath is present in dest. Test URL construction helpers.

---

### 1.5 — Registry Resolver [DONE]

Given a user input like `go/api`, `go/api@v2.1.0`, or a full URL, resolve it to a concrete registry URL + blueprint path + ref.

**Files:**

- `internal/registry/resolver.go` — `Resolve(input string, cfg GlobalConfig) (*ResolvedBlueprint, error)`
  - `ResolvedBlueprint` struct: `RegistryURL`, `BlueprintPath`, `Ref`, `Commit`
- `internal/registry/index.go` — `LoadIndex(registryPath string) (*config.Registry, error)` — loads `registry.yaml` from a cloned registry, validates that the requested blueprint exists
- `internal/registry/resolver_test.go`
- `internal/registry/index_test.go`

**Details:**

- Parse formats (aligned with go-getter's URL conventions):
  - `go/api` → use default registry URL from global config + path `go/api` + latest
  - `go/api@v2.1.0` → same but pin to ref `v2.1.0`
  - `github.com/acme/blueprints//go/api` → explicit registry URL (double-slash separates repo from subpath — native go-getter syntax)
  - `github.com/acme/blueprints//go/api?ref=v2.1.0` → with ref
  - `git@github.com:someone/standalone-blueprint.git` — standalone, no registry.yaml expected
- The resolver converts internal `@v2.1.0` shorthand to go-getter's `?ref=v2.1.0` query param
- `LoadIndex` reads `registry.yaml`, finds the matching blueprint entry, and returns its metadata (version, tags, description)
- If the blueprint is not found in the index, return a clear error listing available blueprints

**Verification:** Unit tests cover all input formats. Index tests use `testdata/registry/registry.yaml`.

---

### 1.6 — Defaults Inheritance Resolver [DONE]

Walk the registry directory tree and merge the layered `_defaults/` directories with the blueprint's own files, applying exclusions.

**Files:**

- `internal/defaults/resolver.go` — `Resolve(registryRoot, blueprintPath string, exclusions []string) (*FileSet, error)`
  - `FileSet` struct: ordered map of `relativePath → FileEntry{AbsPath, SourceLayer, IsTemplate}`
  - `SourceLayer` enum: `LayerRegistryDefault`, `LayerCategoryDefault`, `LayerBlueprint`
- `internal/defaults/resolver_test.go`

**Details:**

- Walk algorithm:
  1. Collect all files under `/_defaults/` → map them by relative path
  2. Walk category `_defaults/` dirs between root and blueprint (e.g., for `go/api`: check `go/_defaults/`). Override matching paths.
  3. Walk the blueprint dir itself. Override matching paths.
  4. Remove any paths listed in `exclusions`
- Directory name `_defaults` is a reserved name — skip it when collecting blueprint files
- Handle nested `_defaults/` at any intermediate category level (e.g., `go/_defaults/`, `go/api/` would never have `_defaults/` since it's a leaf blueprint)
- Track `SourceLayer` for each file — needed later for lockfile provenance
- A file is a template if its name ends in `.tmpl`

**Verification:** Unit tests against `testdata/registry/`:
- `go/api` gets `.editorconfig` from root defaults, `.golangci.yml` from `go/_defaults/`, and `cmd/main.go.tmpl` from blueprint
- `go/_defaults/scripts/lint.sh` overrides `_defaults/scripts/lint.sh`
- Excluded files are absent from the result

---

### 1.7 — Template Rendering Engine [DONE]

Render `.tmpl` files using Go's `text/template` with a custom function map. Also handle directory name templating.

**Files:**

- `internal/template/funcs.go` — custom `template.FuncMap`:
  - `snakeCase`, `camelCase`, `pascalCase`, `kebabCase` — use a small case-conversion library or implement with `strings` and `unicode`
  - `upper`, `lower`, `title`, `replace`, `trimPrefix`, `trimSuffix`
  - `now` — `func(layout string) string` using `time.Now().Format(layout)`
  - `env` — `func(key string) string` using `os.Getenv(key)`
- `internal/template/renderer.go` — `Renderer` struct with:
  - `RenderFile(tmplPath string, vars map[string]any) ([]byte, error)` — parse and execute a single template file
  - `RenderString(tmpl string, vars map[string]any) (string, error)` — render an inline template string (used for variable defaults, conditions, directory names)
  - `RenderPath(path string, vars map[string]any) (string, error)` — render `{{project_name}}` in file/dir paths
- `internal/template/funcs_test.go`
- `internal/template/renderer_test.go`

**Details:**

- Templates use `text/template` (not `html/template`) — no auto-escaping
- `RenderFile` reads the file, parses as template, executes with vars, returns rendered bytes
- `RenderPath` handles directory/file name segments containing `{{var}}` — e.g., `{{project_name}}/cmd/main.go` → `my-api/cmd/main.go`
- Strip `.tmpl` extension from output file names after rendering
- If a template references an undefined variable, return a clear error (use `template.Option("missingkey=error")`)

**Verification:** Unit tests render sample templates with known vars and assert output. Test all custom functions. Test `missingkey=error` behavior.

---

### 1.8 — Interactive Prompt Engine [DONE]

Prompt the user for blueprint variables, using declared types, defaults, and validation.

**Files:**

- `internal/prompt/prompt.go` — `PromptForVariables(vars []config.Variable, overrides map[string]string) (map[string]any, error)`
- `internal/prompt/prompt_test.go`

**Details:**

- Use `charmbracelet/huh` for the interactive UI
- For each variable in order:
  - If an override was provided via `--set key=value` CLI flag, use it (skip prompting)
  - `string` type → text input with optional regex validation
  - `bool` type → confirm prompt
  - `choice` type → select from `choices` list
  - `int` type → text input with integer validation
- Render `default` field as a template (it can reference earlier variables, e.g., `github.com/{{ .org }}/{{ .project_name }}`)
- `--defaults` flag skips all prompts and uses default values (error if a required variable has no default)
- Return `map[string]any` with proper Go types (string, bool, int) — not everything as string

**Verification:** Unit tests use overrides map to bypass interactive prompts and verify variable resolution, type coercion, default template rendering, and validation.

---

### 1.9 — Lockfile Manager

Generate and read `.forge-lock.yaml` to track blueprint provenance, variable values, default file sources, and tool versions.

**Files:**

- `internal/lockfile/lock.go` — `Lockfile` struct matching the schema in Section 9 of PROJECT_PLAN, plus:
  - `Write(path string, lock *Lockfile) error`
  - `Read(path string) (*Lockfile, error)`
- `internal/lockfile/lock_test.go`

**Details:**

- `Lockfile` struct fields:
  - `Blueprint` — registry URL, name, path, ref, commit
  - `CreatedAt`, `LastSynced` — RFC 3339 timestamps
  - `ForgeVersion` — build version string
  - `Variables` — `map[string]any`
  - `Defaults` — `[]DefaultEntry{Path, Source, Strategy, SyncedCommit}`
  - `ManagedFiles` — `[]ManagedFileEntry{Path, Strategy, SyncedCommit}`
  - `Tools` — `[]ToolEntry{Name, Version, Source}`
- `Write` marshals to YAML with a `# .forge-lock.yaml — DO NOT EDIT MANUALLY` header comment
- `Read` unmarshals and validates

**Verification:** Round-trip test: write a lockfile, read it back, assert equality.

---

### 1.10 — `forge create` Command — Full End-to-End Flow

Wire all the pieces together into the `forge create` command.

**Files:**

- `cmd/create.go` — Cobra command definition with flags
- `internal/create/create.go` — `Run(opts CreateOpts) error` — orchestrates the full create flow

**Details:**

Flags:
- `--set key=value` (repeatable) — variable overrides
- `--output-dir` / `-o` — target directory (default: `./<project_name>`)
- `--defaults` — non-interactive, use all defaults
- `--no-tools` — skip tool installation (tools come in Phase 2, but wire the flag now)

Orchestration flow (in `internal/create/create.go`):
1. Parse input → `registry.Resolve(input)`
2. Fetch registry → `getter.Fetch()` with subpath extraction (via cache)
3. Load `registry.yaml` → `registry.LoadIndex()`
4. Load `blueprint.yaml` → `config.LoadBlueprint()`
5. Validate blueprint config → `config.Validate()`
6. Prompt for variables → `prompt.PromptForVariables()`
7. Resolve defaults inheritance → `defaults.Resolve()`
8. Evaluate conditions → filter files based on condition templates
9. Build output file list from `FileSet`
10. For each file:
    - Render path templates (directory/file names)
    - If `.tmpl`: render content through template engine
    - Else: copy verbatim
    - Write to output directory
11. Apply `rename` rules from `blueprint.yaml`
12. Generate `.forge-lock.yaml`
13. Print summary (files created, defaults inherited, etc.)

Do not implement hooks or tools yet — log a message like `"skipping hooks (not yet implemented)"` and move on.

**Verification:**
- Integration test: run `create.Run()` against `testdata/registry/`, assert:
  - Output directory contains expected files
  - Template variables are substituted correctly
  - Defaults inheritance is correct (root → category → blueprint)
  - Excluded defaults are absent
  - `.forge-lock.yaml` exists with correct provenance
  - `.tmpl` extensions are stripped
  - Directory names with `{{project_name}}` are expanded

---

### 1.11 — Registry Cache

Cache fetched registries locally to avoid re-downloading on every command.

**Files:**

- `internal/registry/cache.go` — `Cache` struct:
  - `GetOrFetch(url, subpath, ref string) (localPath string, error)` — return cached path or fetch fresh via go-getter
  - `Invalidate(url string) error`
  - Cache location: `~/.cache/forge/registries/<url-hash>/`
- `internal/registry/cache_test.go`

**Details:**

- Hash the registry URL (SHA256, truncated) as the cache directory name
- On `GetOrFetch`: if dir exists and ref matches, return cached path. Otherwise, re-fetch via `getter.Fetch()`.
- Store the fetched ref in a `.forge-cache-meta` file alongside the cached content for staleness checks
- No TTL for now — users can run `forge cache clean` (Phase 4) or delete manually
- Respect `XDG_CACHE_HOME` if set, otherwise `~/.cache/forge/`

**Verification:** Test that second call to `GetOrFetch` with the same ref reuses the cached directory. Test that a different ref triggers a re-fetch. Test `Invalidate` removes the directory.

---

## Phase 2: Registry Browsing, Authoring, and Tools

**Goal:** Users can browse registries, author new blueprints, and manage remote tools.

### 2.1 — `forge list` Command

List blueprints from one or more registries.

**Files:**

- `cmd/list.go` — Cobra command with `--tag`, `--registry`, `--output` (`table | json`) flags
- `internal/list/list.go` — `Run(opts ListOpts) error`

**Details:**

- Load registry index → filter by tag if provided → render as table using `tablewriter` or JSON
- Table columns: `NAME`, `VERSION`, `DESCRIPTION`, `TAGS`
- Use `lipgloss` for styled headers

**Verification:** Test against `testdata/registry/`. Assert table output contains expected blueprints. Assert `--tag go` filters correctly.

---

### 2.2 — `forge search` Command

Search across registries by name, description, or tags.

**Files:**

- `cmd/search.go`
- `internal/search/search.go` — `Run(query string, opts SearchOpts) error`

**Details:**

- Case-insensitive substring match against blueprint `name`, `description`, and `tags`
- Search across all configured registries
- Same output format as `forge list`

**Verification:** Test search by name, by tag, and by description substring.

---

### 2.3 — Global Config and Multi-Registry Support

Allow users to configure default registries and preferences.

**Files:**

- `internal/config/global.go` — `GlobalConfig` struct:
  - `Registries []RegistryConfig` — name, URL, default flag
  - `CacheDir string`
  - `DefaultRegistry string`
- Load from `~/.config/forge/config.yaml` (respect `XDG_CONFIG_HOME`)
- `internal/config/global_test.go`

**Details:**

- `forge create go/api` uses the default registry
- `forge create go/api --registry acme` uses the named registry
- If no config exists, use a sensible zero-value (empty registries list, user must provide URL or set up config)

**Verification:** Tests load sample config, verify registry lookup by name, verify default selection.

---

### 2.4 — Conditional File Inclusion

Evaluate `conditions` from `blueprint.yaml` to exclude files based on template expressions.

**Files:**

- `internal/create/conditions.go` — `EvaluateConditions(conditions []config.Condition, vars map[string]any, fileSet *defaults.FileSet) error`
- `internal/create/conditions_test.go`

**Details:**

- Each condition has a `when` template string and an `exclude` list of glob patterns
- Render `when` with the template engine against the variables
- If the rendered result is `"true"`, remove matching files from the FileSet
- Glob matching uses `filepath.Match` against relative paths

**Verification:** Test: with `use_grpc=false`, files matching `proto/` and `internal/grpc/` are excluded.

---

### 2.5 — Post-Create Hooks

Execute shell commands after scaffolding.

**Files:**

- `internal/hooks/hooks.go` — `RunPostCreate(hooks []string, workDir string) error`
- `internal/hooks/hooks_test.go`

**Details:**

- Run each hook as `exec.Command("sh", "-c", hook)` with `Dir` set to the output directory
- Stream stdout/stderr to the user
- If a hook fails, print a warning but continue (don't abort the whole create — the project files are already written)
- Skip hooks in `--no-hooks` mode (add flag to `forge create`)

**Verification:** Test that a hook command runs in the correct directory. Test that a failing hook doesn't cause `Run` to return an error.

---

### 2.6 — `forge init` Command

Scaffold a new blueprint directory with a starter `blueprint.yaml`.

**Files:**

- `cmd/init.go`
- `internal/init/init.go` — `Run(opts InitOpts) error`

**Details:**

- `forge init` in current dir → create `blueprint.yaml` with example structure
- `forge init go/grpc-gateway --registry .` → create `go/grpc-gateway/blueprint.yaml` within a registry repo, and append an entry to `registry.yaml`
- `forge init --from ./existing-project` → scan an existing project, reverse-engineer a `blueprint.yaml` by detecting common files and inferring variables (basic heuristic: find project name in paths, suggest as a variable)

**Verification:** Test that `forge init` creates a valid `blueprint.yaml`. Test registry-mode updates `registry.yaml`.

---

### 2.7 — Tool Manifest Parser and Inheritance

Parse tool declarations from `registry.yaml`, category `_defaults/tools.yaml`, and `blueprint.yaml`, then merge them with the same layered-inheritance logic.

**Files:**

- `internal/tools/manifest.go` — `ResolveTool(registry *config.Registry, categoryToolsPath string, blueprint *config.Blueprint, vars map[string]any) ([]ResolvedTool, error)`
  - `ResolvedTool` struct: `Name`, `Version`, `Source`, `InstallPath`, `Checksum`, `SourceLayer`
- `internal/tools/manifest_test.go`

**Details:**

- Merge order: registry tools → category tools → blueprint tools (last wins, matched by `name`)
- Evaluate `condition` field per tool using the template renderer — skip tools whose condition evaluates to `false`
- Category tools loaded from `<category>/_defaults/tools.yaml` if it exists

**Verification:** Test that blueprint-level tool version overrides category-level. Test conditional tools are excluded when condition is false.

---

### 2.8 — Platform Resolver and Tool Downloader

Download tools from various sources using go-getter for download-based sources and exec for package-manager sources.

**Files:**

- `internal/tools/platform.go` — `DetectPlatform() Platform` and `ResolveAssetURL(tool ResolvedTool, platform Platform) (string, error)`
  - `Platform` struct: `OS`, `Arch`, `GOOS`, `GOARCH` (capitalized variants)
- `internal/tools/downloader.go` — `Download(tool ResolvedTool, platform Platform, destDir string) error`
- `internal/tools/cache.go` — `Cache` struct managing `~/.cache/forge/tools/<name>/<version>/`
- `internal/tools/platform_test.go`
- `internal/tools/downloader_test.go`

**Details:**

- Asset URL pattern variables: `{{os}}`, `{{arch}}`, `{{goos}}`, `{{goarch}}`, `{{version}}`
- Source type dispatch:
  - `github-release` → construct go-getter URL: `github.com/<repo>/releases/download/v<version>/<asset_pattern>` — go-getter handles the HTTP download, archive extraction (tar.gz, zip), and checksum verification natively via `?checksum=sha256:...` query param
  - `url` → pass directly to `getter.FetchFile()` — go-getter handles archive detection and extraction automatically
  - `go-install` → `exec.Command("go", "install", module+"@"+version)`
  - `npm` → `exec.Command("npm", "install", "-g", package+"@"+version)`
  - `cargo-install` → `exec.Command("cargo", "install", crate, "--version", version)`
  - `script` → `getter.FetchFile()` to download script, then execute with `VERSION` env var
- go-getter's archive support auto-extracts `.tar.gz`, `.zip`, `.tar.bz2`, `.tar.xz` — no manual extraction code needed
- go-getter's checksum support appends `?checksum=sha256:<hash>` to the URL — no manual verification code needed
- Cache check: if `~/.cache/forge/tools/<name>/<version>/<binary>` exists, copy from cache instead of downloading

**Verification:** Unit test platform detection. Integration test downloads from a local HTTP test server (use `httptest.NewServer`). Test cache hit path. Test that go-getter extracts archives and verifies checksums.

---

### 2.9 — `forge tools` Commands

Wire tool management into CLI subcommands.

**Files:**

- `cmd/tools.go` — `forge tools` parent command
- `cmd/tools_install.go` — `forge tools install`
- `cmd/tools_list.go` — `forge tools list`
- `cmd/tools_check.go` — `forge tools check`
- `cmd/tools_update.go` — `forge tools update [tool-name]`

**Details:**

- `install` — read `.forge-lock.yaml` for declared tools, download all to `.forge/tools/`
- `list` — table output: `TOOL`, `VERSION`, `STATUS`, `SOURCE`
- `check` — compare lockfile tool versions against current blueprint declarations (requires fetching registry)
- `update` — download newer versions, update `.forge-lock.yaml`
- Integrate tool installation into `forge create` flow (skip if `--no-tools`)

**Verification:** Integration test: `forge create` with tools → verify binaries in `.forge/tools/`. `forge tools list` shows installed tools.

---

### 2.10 — Wire Tools into `forge create`

Update the create flow to resolve, download, and install tools after file generation.

**Files:**

- Update `internal/create/create.go` — add tool resolution and installation between step 12 and lockfile generation

**Details:**

- After file rendering, before lockfile:
  1. `tools.ResolveTools()` → get merged tool list
  2. For each tool: check cache → download if needed → copy to `.forge/tools/<name>`
  3. Ensure `.forge/tools/` is in `.gitignore`
  4. Record tool versions in lockfile
- Skip entirely if `--no-tools`

**Verification:** Integration test verifies tools are installed and recorded in lockfile.

---

## Phase 3: Sync Engine

**Goal:** `forge check` detects drift. `forge sync` updates managed files and defaults using overwrite or three-way merge.

### 3.1 — `forge check` Command

Compare local project state against the source blueprint for defaults, managed files, and tools.

**Files:**

- `cmd/check.go` — flags: `--output` (`text | json`)
- `internal/check/check.go` — `Run(opts CheckOpts) (*CheckResult, error)`
  - `CheckResult` struct: `DefaultsUpdates []FileUpdate`, `ManagedUpdates []FileUpdate`, `ToolUpdates []ToolUpdate`
  - `FileUpdate` struct: `Path`, `Status` (up-to-date, modified, new, deleted), `SourceLayer`
  - `ToolUpdate` struct: `Name`, `CurrentVersion`, `AvailableVersion`

**Details:**

- Read `.forge-lock.yaml` → get blueprint origin, commit, tool versions
- Fetch registry at two refs via go-getter:
  - `synced_commit` ref → temp dir A (base snapshot)
  - latest / HEAD → temp dir B (latest snapshot)
- For each file in lockfile `defaults` and `managed_files`:
  - Compare file content in dir A (base) vs dir B (latest)
  - If base == latest: up to date
  - If base != latest: update available
  - If file doesn't exist locally: deleted locally
- For tools: compare lockfile version against current blueprint/registry declaration
- Render styled output: checkmarks for up-to-date, markers for updates available
- JSON output for CI integration

**Verification:** Integration test: modify a file in the test registry after initial create, run check, assert update is detected.

---

### 3.2 — Sync Engine — Overwrite Strategy

Replace local files with the latest version from the blueprint/defaults.

**Files:**

- `internal/sync/engine.go` — `Sync(opts SyncOpts) (*SyncResult, error)`
  - `SyncOpts`: `DryRun bool`, `Force bool`, `FileFilter string`, `IncludeTools bool`
  - `SyncResult`: `Updated []string`, `Skipped []string`, `Conflicts []string`
- `internal/sync/overwrite.go` — `Overwrite(localPath string, sourceContent []byte) error`
- `internal/sync/engine_test.go`

**Details:**

- For files with `strategy: overwrite`:
  1. Fetch latest content from source registry
  2. Re-render if it's a `.tmpl` file (using variables from lockfile)
  3. Replace local file
  4. Update `synced_commit` in lockfile
- `--dry-run` prints what would change without writing
- `--force` skips confirmation prompts
- `--file .golangci.yml` syncs only that file

**Verification:** Test: create project, modify a default file in registry, run sync, assert local file matches new version.

---

### 3.3 — Sync Engine — Three-Way Merge Strategy

For `strategy: merge` files, compute a three-way merge using the last-synced version as the common ancestor.

**Files:**

- `internal/sync/merge.go` — `ThreeWayMerge(base, local, remote []byte) ([]byte, []Conflict, error)`
- `internal/sync/diff.go` — wrapper around `go-diff` for line-level diffing
- `internal/sync/merge_test.go`

**Details:**

- Three inputs:
  - `base` = file content at `synced_commit` (the last time it was synced)
  - `local` = current local file
  - `remote` = latest file from the registry
- Algorithm:
  1. Diff `base → local` to find local changes
  2. Diff `base → remote` to find upstream changes
  3. Apply non-overlapping hunks from both sides
  4. Where hunks overlap → generate conflict markers (`<<<<<<<`, `=======`, `>>>>>>>`)
- Return merged content and a list of conflicts (if any)
- The sync engine uses merge for files with `strategy: merge`, falls back to overwrite for `strategy: overwrite`

**Verification:** Test cases:
- Both sides change different lines → clean merge
- Both sides change the same line → conflict markers
- Only one side changes → clean merge with that side's changes
- No changes on either side → no-op

---

### 3.4 — `forge sync` Command

Wire the sync engine into a CLI command.

**Files:**

- `cmd/sync.go` — flags: `--dry-run`, `--force`, `--file`, `--include-tools`

**Details:**

- Read `.forge-lock.yaml`
- Fetch registry at `synced_commit` (base) and latest via go-getter into temp dirs
- For each default and managed file:
  - Determine strategy (from lockfile or blueprint config)
  - Apply overwrite or merge
- If `--include-tools`: also run `forge tools update`
- Update `.forge-lock.yaml` with new `synced_commit` and `last_synced` timestamp
- Print summary: files updated, files skipped, conflicts

**Verification:** End-to-end test: create project → modify registry → `forge sync --dry-run` shows changes → `forge sync` applies them → lockfile updated.

---

### 3.5 — Conflict Resolution UX

When three-way merge produces conflicts, provide a usable resolution workflow.

**Files:**

- `internal/sync/conflict.go` — `ResolveConflicts(conflicts []ConflictFile, interactive bool) error`

**Details:**

- Write conflict markers to the file (standard git-style `<<<<<<<`/`=======`/`>>>>>>>`)
- Print list of conflicted files to stderr
- If interactive: prompt user per conflict — "keep local", "accept remote", "keep both (conflict markers)", "open in editor"
- If non-interactive (CI): write markers, exit with non-zero status
- After resolution, update lockfile

**Verification:** Test conflict marker generation. Test interactive resolution with mocked input.

---

## Phase 4: Polish and Release

**Goal:** Production-ready v0.1.0 with comprehensive tests, good error UX, and documentation.

### 4.1 — Error Handling and UX

Audit all error paths and add user-friendly messaging with colored output.

**Files:**

- `internal/ui/output.go` — helpers for consistent styled output:
  - `Success(msg)`, `Warning(msg)`, `Error(msg)`, `Info(msg)`
  - Progress indicators for long operations (clone, download)
  - Summary table formatting
- Update all commands to use `ui` package for output

**Details:**

- Use `lipgloss` for styling
- Respect `--no-color` flag and `NO_COLOR` env var
- Wrap low-level errors with context: `fmt.Errorf("failed to clone registry %s: %w", url, err)`
- For common failures, suggest fixes: "registry not found — run `forge config add-registry` to configure"

---

### 4.2 — `forge info` Command

Show detailed blueprint information including inherited defaults, tools, and variable schema.

**Files:**

- `cmd/info.go`
- `internal/info/info.go`

**Details:**

- Resolve blueprint → load config → resolve defaults inheritance → resolve tools
- Display: description, version, variables with types/defaults, inherited files with source layer, tools with versions
- Useful for debugging inheritance: "this file comes from `go/_defaults/`"

---

### 4.3 — `forge cache clean` Command

Clear cached registries and/or tools.

**Files:**

- `cmd/cache.go` — `forge cache clean [--registries] [--tools] [--all]`

**Details:**

- `--registries` removes `~/.cache/forge/registries/`
- `--tools` removes `~/.cache/forge/tools/`
- `--all` removes both
- Print freed disk space

---

### 4.4 — Comprehensive Test Suite

Fill in coverage gaps with integration tests that exercise the full flow.

**Files:**

- `internal/create/create_integration_test.go` — end-to-end `forge create` against test fixtures
- `internal/sync/sync_integration_test.go` — end-to-end sync cycle
- `internal/tools/tools_integration_test.go` — tool download and cache

**Details:**

- Use `testdata/` fixtures for all tests
- Integration tests create temp dirs, run the full flow, and assert on output files
- Test edge cases: missing `registry.yaml`, invalid `blueprint.yaml`, network errors (mock HTTP), empty variables
- Target 60% coverage (matching `.codecov.yml` threshold)

---

### 4.5 — Documentation

Write user-facing docs.

**Files:**

- `README.md` — project overview, installation, quickstart
- `docs/BLUEPRINT_AUTHORING.md` — how to create blueprints and registries
- `docs/REGISTRY_SETUP.md` — registry structure, `_defaults/`, `registry.yaml`
- `docs/TOOLS_GUIDE.md` — tool manifest format, source types, caching

---

### 4.6 — Reference Registry

Create a companion repository with starter blueprints demonstrating all features.

**Scope:**

- `registry.yaml` with several blueprints
- `_defaults/` with standard CI, linting, and license files
- At least `go/cli` and `go/api` blueprints with working templates
- Tool declarations for `golangci-lint` and `goreleaser`
- This can be a separate repo or a `examples/` directory for initial development

---

## Dependency Graph

```
1.1 (skeleton)
 │
 ├── 1.2 (config types) ──── 1.3 (test fixtures)
 │    │
 │    ├── 1.5 (registry resolver)
 │    │    │
 │    │    └── 1.11 (registry cache)
 │    │
 │    ├── 1.6 (defaults resolver)
 │    │
 │    ├── 1.8 (prompt engine)
 │    │
 │    └── 1.9 (lockfile)
 │
 ├── 1.4 (go-getter) ◄── also used by 2.8 (tool downloader) and 3.1/3.4 (check/sync)
 │
 └── 1.7 (template engine)

All of Phase 1 ──► 1.10 (forge create)

1.10 ──► 2.1 (list)
1.10 ──► 2.2 (search)
1.10 ──► 2.3 (global config)
1.10 ──► 2.4 (conditions) ← can be done during 1.10
1.10 ──► 2.5 (hooks)
1.10 ──► 2.6 (init)
1.10 ──► 2.7 (tool manifest) ── 2.8 (downloader via go-getter) ── 2.9 (tool commands) ── 2.10 (wire into create)

2.10 ──► 3.1 (check) ── 3.2 (sync overwrite) ── 3.3 (sync merge) ── 3.4 (sync command) ── 3.5 (conflict UX)

3.5 ──► 4.1–4.6 (polish) ──► 5.1–5.x (miscellaneous)
```

---

## Phase 5: Miscellaneous

**Goal:** Post-release housekeeping — CI hardening, compliance automation, and operational improvements.

### 5.1 — License Compliance CI Job

Add a GitHub Actions workflow that checks all Go module dependencies for license compatibility with Apache-2.0.

**Files:**

- `.github/workflows/license-check.yml`

**Details:**

- Use `google/go-licenses` to scan all transitive dependencies:
  ```yaml
  name: License Check

  on:
    push:
      branches: [main]
    pull_request:
      branches: [main]

  permissions:
    contents: read

  jobs:
    license-check:
      name: Check Dependency Licenses
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v6

        - name: Set up Go
          uses: actions/setup-go@v6
          with:
            go-version-file: go.mod

        - name: Install go-licenses
          run: go install github.com/google/go-licenses@latest

        - name: Check licenses
          run: go-licenses check ./... --allowed_licenses=Apache-2.0,MIT,BSD-2-Clause,BSD-3-Clause,ISC,MPL-2.0

        - name: Generate license report
          if: always()
          run: go-licenses report ./... --template=csv > licenses.csv

        - name: Upload license report
          if: always()
          uses: actions/upload-artifact@v4
          with:
            name: license-report
            path: licenses.csv
  ```
- `--allowed_licenses` whitelist: `Apache-2.0`, `MIT`, `BSD-2-Clause`, `BSD-3-Clause`, `ISC`, `MPL-2.0` — these are all compatible with Apache-2.0 distribution
- The job fails if any dependency uses a license not in the whitelist (e.g., GPL, AGPL, SSPL, CPAL)
- Generates a CSV report as a build artifact for manual review
- Note: `hashicorp/go-getter` is MPL-2.0, which is compatible with Apache-2.0

**Verification:** Push a branch, verify the workflow runs and passes with current dependencies. Intentionally add a GPL dependency in a test branch to confirm the job fails.

---

### 5.2 — Add `go-licenses` to Makefile and mise.toml

Make license checking available locally, not just in CI.

**Files:**

- Update `mise.toml` — add `go:github.com/google/go-licenses`
- Update `Makefile` — add `license-check` and `license-report` targets

**Details:**

- Makefile targets:
  ```makefile
  license-check: ## Check dependency licenses against allowed list
  	@go-licenses check ./... --allowed_licenses=Apache-2.0,MIT,BSD-2-Clause,BSD-3-Clause,ISC,MPL-2.0

  license-report: ## Generate CSV report of all dependency licenses
  	@go-licenses report ./... --template=csv
  ```
- Add `license-check` to the `ci` target so it runs as part of `make ci`

**Verification:** `make license-check` passes locally. `make license-report` outputs a valid CSV.
