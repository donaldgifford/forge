# MVP Gap Implementation Plan

The original implementation plan marked all tasks as [DONE], but manual CLI
testing revealed that several commands don't work end-to-end from the CLI.
The core logic is tested through the Go API (unit/integration tests pass
`RegistryDir` directly), but the CLI-to-core wiring was never completed.

This plan fixes those gaps in dependency order: create (local) -> create
(remote) -> sync -> check.

## Design Decisions

These were resolved during review and are binding for implementation:

1. **Existing directory behavior**: `forge create` requires `--force` to write
   into a non-empty directory. Without `--force`, it refuses and prints a clear
   error.

2. **`--registry-dir` is unified**: Accepts both local filesystem paths AND
   remote go-getter URLs (e.g., `github.com/user/registry`). Go-getter handles
   both natively — this follows the same pattern as Terraform source syntax.

3. **Check uses both approaches**: Hash-based detection (lockfile stores SHA256
   hashes) for quick local checks without registry access. Registry comparison
   (via `--registry-dir`) for detecting upstream changes. Both are implemented.

4. **Sync supports `--ref`**: `forge sync --ref v2.0.0` syncs against a
   specific registry version. When `--ref` is not set, uses the ref from the
   lockfile's blueprint config. The command outputs which ref it is syncing
   against as it runs.

---

## Gap 1: `forge create` with Local Registry Directory

**Problem**: `create.go:144` returns `"registry fetching not yet implemented"`
because `RegistryDir` is never populated from the CLI. There is no
`--registry-dir` flag, and go-getter is not called to fetch.

**Goal**: `forge create go/api --registry-dir ./path/to/registry` works with a
local filesystem directory, similar to how Terraform's go-getter handles local
paths.

### 1.1 — Add `--registry-dir` Flag and `--force` Guard to `cmd/create.go` [DONE]

Add a `--registry-dir` flag that accepts a local filesystem path (absolute or
relative). When provided, pass the path directly to `create.Opts.RegistryDir`.

Also add a `--force` flag and non-empty directory guard to the create command.

**Files to change:**

- `cmd/create.go` — add `--registry-dir` and `--force` flags, pass to opts,
  add non-empty directory check before calling `create.Run()`

**Details:**

- Flag: `--registry-dir` (string) — local path to a registry directory
- Flag: `--force` (bool) — allow writing into non-empty directories
- When `--registry-dir` is set, resolve it to an absolute path and set
  `opts.RegistryDir`
- Before calling `create.Run()`, check if the output directory exists and is
  non-empty. If so, require `--force` or return an error:
  `"output directory %s is not empty — use --force to overwrite"`
- At this stage, `--registry-dir` is local-only. Remote go-getter support is
  added in Gap 2.

### 1.2 — Remove Stub Error in `internal/create/create.go` [DONE]

Remove the `"registry fetching not yet implemented"` error at line 145 and
replace it with proper behavior: when `RegistryDir` is empty, return a clear
error asking the user to provide `--registry-dir` or configure a default
registry.

**Files to change:**

- `internal/create/create.go` — update `resolveAndLoad()` error message

### 1.3 — Verify `forge create` End-to-End with Local Registry [DONE]

Run the CLI binary against `testdata/registry/` and validate the full flow.

**Verification command:**

```bash
./build/bin/forge create go/api \
  --registry-dir ./testdata/registry \
  --defaults \
  --set project_name=my-test-api \
  --set go_module=github.com/example/my-test-api \
  --set use_grpc=false \
  --set license=MIT \
  --no-hooks \
  -o /tmp/forge-test-create
```

**Success criteria:**

- [x] Exit code 0
- [x] `/tmp/forge-test-create/` contains rendered files:
  - `cmd/main.go` with `package main` and `github.com/example/my-test-api`
  - `go.mod` with correct module path
  - `README.md` with project name
  - `.editorconfig` from root `_defaults/`
  - `.golangci.yml` from `go/_defaults/`
  - `scripts/lint.sh` from `go/_defaults/` (overrides root)
  - `.gitignore` rendered from template
  - `LICENSE` rendered from template
- [x] `.forge-lock.yaml` exists with correct `blueprint.name`, `variables`,
  `defaults`, and `managed_files`
- [x] No `.tmpl` extensions in output files
- [x] `.pre-commit-config.yaml` is NOT present (excluded in blueprint.yaml)
- [x] Running the command again without `--force` produces a clear error about
  the existing non-empty directory
- [x] Running the command again WITH `--force` succeeds

### 1.4 — Write a CLI Integration Test [DONE]

Add a test that calls `create.Run()` with the same opts the CLI would pass,
validating the full wiring from flag parsing through file output.

**Files to create:**

- `internal/create/cli_integration_test.go`

**Success criteria:**

- [x] Test creates a project from `testdata/registry/go/api`
- [x] Asserts all expected files exist with correct content
- [x] Asserts lockfile is valid and parseable
- [x] Tests the `--force` guard (non-empty directory rejected, accepted with
  force)
- [x] Runs in `make test`

---

## Gap 2: `forge create` with Remote go-getter Registry

**Problem**: When `--registry-dir` is a remote URL (or when it's not provided
and a default registry is configured), `forge create` needs to fetch the
registry using go-getter, then pass the resulting local directory to
`create.Run()`.

**Depends on**: Gap 1 (local create must work first)

### 2.1 — Make `--registry-dir` Accept go-getter URLs [DONE]

Extend `--registry-dir` to detect whether the value is a local path or a
remote URL. If remote, use `getter.Fetch()` to clone into a temp directory.

**Files to change:**

- `cmd/create.go` — add detection logic, call go-getter for remote sources

**Details:**

- Detection heuristic: if the path exists on the local filesystem, use it
  directly. Otherwise, treat it as a go-getter URL and fetch.
- For remote sources:
  1. Create temp directory
  2. `getter.Fetch(ctx, registryDir, tempDir, FetchOpts{})` to clone
  3. Set `opts.RegistryDir = tempDir`
  4. `defer cleanupDir(logger, tempDir)` for cleanup
- The `--registry-dir` value is stored in the lockfile's
  `blueprint.registry_url` so that `forge sync` can re-fetch later.

### 2.2 — Wire Default Registry Resolution (No `--registry-dir`) [DONE]

When `--registry-dir` is not set, use `registry.Resolve()` to parse the
blueprint reference and the default registry from global config.

**Files to change:**

- `cmd/create.go` — add fallback to global config when `--registry-dir` is
  empty
- `internal/registry/resolver.go` — verify it handles all input formats
  correctly

**Details:**

- Flow when `--registry-dir` is empty:
  1. Load global config via `config.LoadGlobal()`
  2. Get default registry URL
  3. `registry.Resolve(blueprintRef, defaultRegistryURL)` to get
     `RegistryURL`, `BlueprintPath`, `Ref`
  4. Fetch via go-getter into temp dir
  5. Set `opts.RegistryDir = tempDir`
- If no default registry is configured and the input is a short name like
  `go/api`, return a clear error: `"no default registry configured — use
  --registry-dir or configure a default registry in
  ~/.config/forge/config.yaml"`
- Add `--registry` flag to look up a named registry from global config:
  `forge create go/api --registry acme`

**Success criteria:**

- [x] `forge create go/api --registry-dir ./local/path` works (from Gap 1)
- [x] `forge create go/api --registry-dir github.com/user/registry` fetches
  via go-getter and creates a project
- [x] `forge create github.com/user/registry//go/api` fetches via go-getter
  (full URL in blueprint ref)
- [x] `forge create go/api` with a configured default registry works
- [x] `forge create go/api` with NO configured registry and NO `--registry-dir`
  gives a clear error message
- [x] Temp directory is cleaned up after create (no leaked dirs in `/tmp`)
- [x] Lockfile `blueprint.registry_url` is set to the go-getter URL (so sync
  can re-fetch)

---

## Gap 3: `forge sync` with Real Project

**Problem**: `forge sync` reads `.forge-lock.yaml` and uses the
`blueprint.registry_url` to fetch the registry via go-getter. This works
conceptually but has never been tested against a real project created by
`forge create`. It also needs `--registry-dir` for local workflows and `--ref`
for version pinning.

**Depends on**: Gap 2 (need a project with a valid `registry_url` in lockfile)

### 3.1 — Add `--registry-dir` and `--ref` to `forge sync` [DONE]

Add `--registry-dir` to override the lockfile's `registry_url` (same semantics
as create: accepts local path or go-getter URL). Add `--ref` to sync against a
specific registry version.

**Files to change:**

- `cmd/sync.go` — add `--registry-dir` and `--ref` flags, output which ref is
  being used

**Details:**

- When `--registry-dir` is set: use it instead of the lockfile's
  `registry_url` for fetching. Same local-vs-remote detection as create.
- When `--ref` is set: pass as `FetchOpts{Ref: ref}` to go-getter. When not
  set, use `lock.Blueprint.Ref` from lockfile.
- Output the ref being synced against: `info: syncing against ref "v1.2.0"`
  or `info: syncing against latest` when no ref is set.
- For three-way merge base: fetch the registry at `lock.Blueprint.Commit`
  (the last-synced commit) as the merge base.

### 3.2 — Ensure Lockfile `registry_url` is Usable by Sync [DONE]

Verify that `forge create` stores a `registry_url` in the lockfile that
`forge sync` can pass directly to go-getter.

**Files to change (if needed):**

- `internal/create/create.go` — ensure `registry_url` is set to the
  canonical go-getter URL (not a relative path)
- When `--registry-dir` is a local path, store the absolute path in the
  lockfile so sync can find it later

### 3.3 — End-to-End Sync Test [DONE]

**Verification steps:**

```bash
# 1. Create a project
forge create go/api --registry-dir ./testdata/registry \
  --defaults --set project_name=sync-test --no-hooks \
  -o /tmp/forge-sync-test

# 2. Modify a default file in the registry
cp testdata/registry/_defaults/.editorconfig testdata/registry/_defaults/.editorconfig.bak
echo "# updated by sync test" >> testdata/registry/_defaults/.editorconfig

# 3. Run sync with registry-dir override
cd /tmp/forge-sync-test
forge sync --registry-dir /path/to/forge/testdata/registry

# 4. Verify the local .editorconfig was updated
grep "updated by sync test" .editorconfig

# 5. Restore the original registry file
mv testdata/registry/_defaults/.editorconfig.bak testdata/registry/_defaults/.editorconfig
```

**Success criteria:**

- [x] `forge sync --registry-dir ./path` updates files that changed in the
  registry
- [x] `forge sync --dry-run` shows what would change without writing
- [x] `forge sync --file .editorconfig` syncs only that file
- [x] `forge sync --ref v1.0.0` syncs against a specific ref and outputs
  `info: syncing against ref "v1.0.0"` as it runs
- [x] `forge sync` without `--ref` uses lockfile ref and outputs which ref
  it is using
- [x] Three-way merge works for managed files with `strategy: merge`
- [x] Conflicts produce git-style markers and a non-zero exit code
- [x] `.forge-lock.yaml` `last_synced` timestamp is updated after sync
- [x] `forge check` after sync shows all files as up-to-date

---

## Gap 4: `forge check` Improvements

**Problem**: `forge check` currently only detects missing files (exists vs
doesn't exist). It doesn't compare file content to detect modifications,
and it doesn't compare against the registry to detect upstream changes.

### 4.1 — Add Hash-Based Local Drift Detection [DONE]

Store SHA256 content hashes in the lockfile at create/sync time. `forge check`
compares current file hashes against stored hashes to detect local
modifications without needing registry access.

**Files to change:**

- `internal/lockfile/lock.go` — add `Hash string` field to `DefaultEntry`
  and `ManagedFileEntry`
- `internal/create/create.go` — compute and store hashes in `buildLockfile()`
- `internal/sync/engine.go` — update hashes after sync writes files
- `internal/check/check.go` — compare current file hash against stored hash

**Details:**

- Hash: `sha256:<hex>` format (same convention as go-getter checksums)
- `forge check` with no flags: compare local file content hashes against
  lockfile hashes. Report "up-to-date", "modified", or "missing".
- New status: `StatusModified` is already defined but never set — wire it up.

### 4.2 — Add Registry Comparison Mode [DONE]

When `--registry-dir` is provided, also compare local files against the
registry source to detect upstream changes.

**Files to change:**

- `cmd/check.go` — add `--registry-dir` flag
- `internal/check/check.go` — accept registry dir, render source files, and
  compare against local

**Details:**

- New statuses when registry is available:
  - `up-to-date` — local matches registry
  - `modified-locally` — local differs from lockfile hash (local changes)
  - `upstream-changed` — registry differs from lockfile hash (upstream changes)
  - `both-changed` — local AND registry both differ from lockfile hash
  - `missing` — file doesn't exist locally
- Without `--registry-dir`: only hash-based (local vs lockfile)
- With `--registry-dir`: full three-way comparison (local vs lockfile vs
  registry)

**Success criteria:**

- [x] `forge check` (no flags) detects when a file has been modified locally
  by comparing SHA256 hashes
- [x] `forge check` distinguishes "up-to-date", "modified", and "missing"
- [x] `forge check --registry-dir ./path` also detects upstream changes in
  the registry
- [x] `forge check --registry-dir ./path` distinguishes "modified-locally",
  "upstream-changed", and "both-changed"
- [x] JSON output (`--output json`) includes all statuses correctly
- [x] Hashes are stored in lockfile during `forge create` and updated during
  `forge sync`

---

## Implementation Order

```
Gap 1.1  Add --registry-dir flag and --force guard to create
Gap 1.2  Remove stub error in create.go
Gap 1.3  Manual verification of local create
Gap 1.4  CLI integration test
    │
Gap 2.1  Make --registry-dir accept go-getter URLs (remote)
Gap 2.2  Wire default registry resolution (no --registry-dir)
    │
Gap 3.1  Add --registry-dir and --ref to sync
Gap 3.2  Ensure lockfile registry_url is usable by sync
Gap 3.3  End-to-end sync test
    │
Gap 4.1  Hash-based local drift detection
Gap 4.2  Registry comparison mode for check
```

Each gap builds on the previous. We validate locally first (Gap 1), then add
remote go-getter support (Gap 2), then wire sync with `--ref` output (Gap 3),
and improve check with hashes and registry comparison (Gap 4).
