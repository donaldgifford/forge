---
id: IMPL-0002
title: "MVP CLI Gap Closure"
status: Completed
author: Donald Gifford
created: 2026-05-07
---
<!-- markdownlint-disable-file MD025 MD041 -->

# IMPL 0002: MVP CLI Gap Closure

**Status:** Completed
**Author:** Donald Gifford
**Date:** 2026-05-07

<!--toc:start-->
- [Objective](#objective)
- [Scope](#scope)
  - [In Scope](#in-scope)
  - [Out of Scope](#out-of-scope)
- [Design Decisions](#design-decisions)
- [Implementation Phases](#implementation-phases)
  - [Gap 1: forge create with Local Registry Directory](#gap-1-forge-create-with-local-registry-directory)
    - [Tasks](#tasks)
    - [Success Criteria](#success-criteria)
  - [Gap 2: forge create with Remote go-getter Registry](#gap-2-forge-create-with-remote-go-getter-registry)
    - [Tasks](#tasks-1)
    - [Success Criteria](#success-criteria-1)
  - [Gap 3: forge sync with Real Project](#gap-3-forge-sync-with-real-project)
    - [Tasks](#tasks-2)
    - [Success Criteria](#success-criteria-2)
  - [Gap 4: forge check Improvements](#gap-4-forge-check-improvements)
    - [Tasks](#tasks-3)
    - [Success Criteria](#success-criteria-3)
- [File Changes](#file-changes)
- [Implementation Order](#implementation-order)
- [Dependencies](#dependencies)
- [References](#references)
<!--toc:end-->

## Objective

The original implementation plan (IMPL-0001) marked all tasks as DONE,
but manual CLI testing revealed that several commands didn't work
end-to-end. The core logic was tested through the Go API
(unit/integration tests pass `RegistryDir` directly), but the
CLI-to-core wiring was never completed.

This plan fixes those gaps in dependency order: create (local) → create
(remote) → sync → check.

**Implements:** [RFC-0001 — Forge: Project Scaffolding CLI](../rfc/0001-forge-project-scaffolding-cli.md)
(closes CLI wiring gaps left by [IMPL-0001](0001-forge-phased-build-plan.md))

## Scope

### In Scope

- Wire `--registry-dir` and `--force` into `forge create`.
- Make `--registry-dir` accept both local paths and go-getter URLs.
- Add `--registry-dir` and `--ref` to `forge sync`.
- Add hash-based local drift detection and registry comparison to
  `forge check`.

### Out of Scope

- Adding new commands (registry blueprint/update — covered by
  PLAN-0001/IMPL-0003).
- Changes to the lockfile schema beyond adding hash fields.

## Design Decisions

These were resolved during review and are binding for implementation:

1. **Existing directory behavior**: `forge create` requires `--force`
   to write into a non-empty directory. Without `--force`, it refuses
   and prints a clear error.
2. **`--registry-dir` is unified**: Accepts both local filesystem paths
   AND remote go-getter URLs (e.g., `github.com/user/registry`).
   Go-getter handles both natively — same pattern as Terraform source
   syntax.
3. **Check uses both approaches**: Hash-based detection (lockfile
   stores SHA256 hashes) for quick local checks without registry
   access. Registry comparison (via `--registry-dir`) for detecting
   upstream changes.
4. **Sync supports `--ref`**: `forge sync --ref v2.0.0` syncs against a
   specific registry version. When `--ref` is not set, uses the ref
   from the lockfile's blueprint config. The command outputs which ref
   is being synced against as it runs.

## Implementation Phases

### Gap 1: `forge create` with Local Registry Directory

**Problem**: `create.go:144` returned `"registry fetching not yet
implemented"` because `RegistryDir` was never populated from the CLI.
There was no `--registry-dir` flag, and go-getter was not called to
fetch.

**Goal**: `forge create go/api --registry-dir ./path/to/registry` works
with a local filesystem directory, similar to Terraform's go-getter
local-path handling.

#### Tasks

- [x] **1.1 Add `--registry-dir` Flag and `--force` Guard to
  `cmd/create.go`**
  - Flag: `--registry-dir` (string) — local path to a registry
    directory.
  - Flag: `--force` (bool) — allow writing into non-empty directories.
  - Resolve `--registry-dir` to absolute path; set
    `opts.RegistryDir`.
  - Before calling `create.Run()`, check if output dir exists and is
    non-empty. If so, require `--force` or return:
    `"output directory %s is not empty — use --force to overwrite"`.
  - At this stage, `--registry-dir` is local-only. Remote support
    comes in Gap 2.

- [x] **1.2 Remove Stub Error in `internal/create/create.go`**
  - Remove `"registry fetching not yet implemented"` at line 145.
  - Replace with: when `RegistryDir` is empty, return a clear error
    asking the user to provide `--registry-dir` or configure a default
    registry.

- [x] **1.3 Verify `forge create` End-to-End with Local Registry**
  - Run the CLI binary against `testdata/registry/` and validate the
    full flow.
  - Verification command:

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

#### Success Criteria

- [x] Exit code 0
- [x] Output dir contains rendered files (`cmd/main.go`, `go.mod`,
  `README.md`, `.editorconfig` from root `_defaults/`, `.golangci.yml`
  from `go/_defaults/`, `scripts/lint.sh` from `go/_defaults/`,
  `.gitignore` and `LICENSE` rendered from templates)
- [x] `.forge-lock.yaml` has correct `blueprint.name`, `variables`,
  `defaults`, `managed_files`
- [x] No `.tmpl` extensions in output files
- [x] `.pre-commit-config.yaml` is NOT present (excluded in
  blueprint.yaml)
- [x] Re-running without `--force` errors clearly; with `--force`
  succeeds

- [x] **1.4 Write a CLI Integration Test**
  - File: `internal/create/cli_integration_test.go`
  - Asserts all expected files exist with correct content
  - Asserts lockfile is valid and parseable
  - Tests `--force` guard
  - Runs in `make test`

### Gap 2: `forge create` with Remote go-getter Registry

**Problem**: When `--registry-dir` is a remote URL (or not provided
and a default registry is configured), `forge create` needs to fetch
the registry via go-getter, then pass the resulting local directory to
`create.Run()`.

**Depends on:** Gap 1.

#### Tasks

- [x] **2.1 Make `--registry-dir` Accept go-getter URLs**
  - Detection heuristic: if the path exists on the local filesystem,
    use it directly. Otherwise treat as a go-getter URL and fetch.
  - For remote sources: create temp dir → `getter.Fetch(...)` → set
    `opts.RegistryDir = tempDir` → `defer cleanupDir(...)`.
  - The `--registry-dir` value is stored in the lockfile's
    `blueprint.registry_url` so `forge sync` can re-fetch later.

- [x] **2.2 Wire Default Registry Resolution (No `--registry-dir`)**
  - Flow when `--registry-dir` is empty:
    1. `config.LoadGlobal()`
    2. Get default registry URL
    3. `registry.Resolve(blueprintRef, defaultRegistryURL)`
    4. Fetch via go-getter into temp dir
    5. Set `opts.RegistryDir = tempDir`
  - If no default registry configured AND input is a short name like
    `go/api`, return a clear error.
  - Add `--registry` flag to look up a named registry from global
    config: `forge create go/api --registry acme`.

#### Success Criteria

- [x] `forge create go/api --registry-dir ./local/path` works
- [x] `forge create go/api --registry-dir github.com/user/registry`
  fetches via go-getter
- [x] `forge create github.com/user/registry//go/api` fetches via
  go-getter
- [x] `forge create go/api` with configured default registry works
- [x] `forge create go/api` with no configured registry and no
  `--registry-dir` gives a clear error
- [x] Temp directory cleaned up after create
- [x] Lockfile `blueprint.registry_url` set so sync can re-fetch

### Gap 3: `forge sync` with Real Project

**Problem**: `forge sync` reads `.forge-lock.yaml` and uses
`blueprint.registry_url` to fetch the registry via go-getter. This
worked conceptually but had never been tested against a real project
created by `forge create`. Also needed `--registry-dir` for local
workflows and `--ref` for version pinning.

**Depends on:** Gap 2.

#### Tasks

- [x] **3.1 Add `--registry-dir` and `--ref` to `forge sync`**
  - When `--registry-dir` is set: use it instead of the lockfile's
    `registry_url`. Same local-vs-remote detection as create.
  - When `--ref` is set: pass as `FetchOpts{Ref: ref}`. Otherwise use
    `lock.Blueprint.Ref`.
  - Output: `info: syncing against ref "v1.2.0"` or `info: syncing
    against latest`.
  - For three-way merge base: fetch the registry at
    `lock.Blueprint.Commit` (last-synced commit) as the merge base.

- [x] **3.2 Ensure Lockfile `registry_url` is Usable by Sync**
  - `internal/create/create.go` — `registry_url` set to the canonical
    go-getter URL (not a relative path).
  - Local paths stored as absolute paths so sync can find them later.

- [x] **3.3 End-to-End Sync Test**
  - Verification:

    ```bash
    # 1. Create a project
    forge create go/api --registry-dir ./testdata/registry \
      --defaults --set project_name=sync-test --no-hooks \
      -o /tmp/forge-sync-test

    # 2. Modify a default file in the registry
    cp testdata/registry/_defaults/.editorconfig{,.bak}
    echo "# updated by sync test" >> testdata/registry/_defaults/.editorconfig

    # 3. Run sync with registry-dir override
    cd /tmp/forge-sync-test
    forge sync --registry-dir /path/to/forge/testdata/registry

    # 4. Verify the local .editorconfig was updated
    grep "updated by sync test" .editorconfig

    # 5. Restore the original registry file
    mv testdata/registry/_defaults/.editorconfig.bak \
       testdata/registry/_defaults/.editorconfig
    ```

#### Success Criteria

- [x] `forge sync --registry-dir ./path` updates files that changed in
  the registry
- [x] `forge sync --dry-run` shows what would change without writing
- [x] `forge sync --file .editorconfig` syncs only that file
- [x] `forge sync --ref v1.0.0` syncs against a specific ref and
  outputs `info: syncing against ref "v1.0.0"`
- [x] `forge sync` without `--ref` uses lockfile ref and outputs which
  ref it is using
- [x] Three-way merge works for managed files with `strategy: merge`
- [x] Conflicts produce git-style markers and a non-zero exit code
- [x] `.forge-lock.yaml last_synced` updated after sync
- [x] `forge check` after sync shows all files as up-to-date

### Gap 4: `forge check` Improvements

**Problem**: `forge check` only detected missing files. It didn't
compare file content for modifications and didn't compare against the
registry to detect upstream changes.

#### Tasks

- [x] **4.1 Add Hash-Based Local Drift Detection**
  - Files to change:
    - `internal/lockfile/lock.go` — add `Hash string` to
      `DefaultEntry` and `ManagedFileEntry`.
    - `internal/create/create.go` — compute hashes in
      `buildLockfile()`.
    - `internal/sync/engine.go` — update hashes after sync writes.
    - `internal/check/check.go` — compare current vs stored hashes.
  - Hash format: `sha256:<hex>` (matches go-getter checksums).
  - `forge check` no flags: compare local content hashes against
    lockfile. Report `up-to-date | modified | missing`. Wire up the
    `StatusModified` constant.

- [x] **4.2 Add Registry Comparison Mode**
  - `cmd/check.go` — add `--registry-dir` flag.
  - `internal/check/check.go` — accept registry dir, render source
    files, compare against local.
  - New statuses with registry available: `up-to-date`,
    `modified-locally`, `upstream-changed`, `both-changed`,
    `missing`.
  - Without `--registry-dir`: only hash-based.
  - With `--registry-dir`: full three-way comparison.

#### Success Criteria

- [x] `forge check` (no flags) detects local modifications via SHA256
- [x] Distinguishes `up-to-date | modified | missing`
- [x] `forge check --registry-dir ./path` detects upstream changes
- [x] Distinguishes `modified-locally | upstream-changed |
  both-changed`
- [x] JSON output (`--output json`) includes all statuses correctly
- [x] Hashes stored in lockfile during `forge create` and updated
  during `forge sync`

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `cmd/create.go` | Modify | Add `--registry-dir`, `--force`, `--registry` flags |
| `cmd/sync.go` | Modify | Add `--registry-dir`, `--ref` flags |
| `cmd/check.go` | Modify | Add `--registry-dir` flag |
| `internal/create/create.go` | Modify | Remove stub error; compute hashes; set lockfile `registry_url` |
| `internal/sync/engine.go` | Modify | Update hashes after writes |
| `internal/check/check.go` | Modify | Hash-based drift; registry comparison |
| `internal/lockfile/lock.go` | Modify | Add `Hash` field |
| `internal/create/cli_integration_test.go` | Create | CLI integration test |

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

Each gap builds on the previous. Validate locally first (Gap 1), add
remote go-getter support (Gap 2), wire sync with `--ref` output
(Gap 3), and improve check with hashes and registry comparison
(Gap 4).

## Dependencies

- Builds on IMPL-0001 (all original phases marked DONE).

## References

- [RFC-0001 — Forge: Project Scaffolding CLI](../rfc/0001-forge-project-scaffolding-cli.md)
- [IMPL-0001 — Forge Phased Build Plan](0001-forge-phased-build-plan.md)
