# Backend Behavioral Contract Specification — Go Port
> Extracted from TypeScript Foolery test suite. This document states WHAT the Go backend must do, not HOW the TypeScript implementation does it.

---

## 1. Backend Port Contract

Every backend implementation MUST satisfy a shared behavioral contract. Capabilities are advertised via a `BackendCapabilities` struct and MUST NOT be silently ignored.

### 1.1 Read Operations
- **`listWorkflows()`** MUST return at least one workflow descriptor, each having `id`, `mode`, and `retakeState` fields. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:70`]
- **`list()`** MUST return `ok: true` with an array. Every item MUST contain the fields: `id`, `title`, `type`, `state`, `priority`, `labels`, `created`, `updated`. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:82`]
- **`get(id)`** with a valid ID MUST return `ok: true` and the exact beat. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:106`]
- **`get(id)`** with an invalid ID MUST return `ok: false` with code `NOT_FOUND`. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:117`]

### 1.2 Write Operations (gated by `canCreate`)
- **`create(input)`** MUST return `ok: true` and a non-empty string `id`. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:135`]
- **`create` followed by `get`** MUST round-trip the input fields (e.g., `title`, `type`). [source: `foolery/src/lib/__tests__/backend-contract.test.ts:143`]
- **`update(id, fields)`** MUST mutate the specified fields and persist them. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:154`]
- **`close(id, reason)`** MUST transition the beat to a terminal state (`shipped` or `closed`). [source: `foolery/src/lib/__tests__/backend-contract.test.ts:170`]

### 1.3 Delete Operations (gated by `canDelete`)
- **`delete(id)`** MUST remove the beat. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:194`]
- **`get(id)` after `delete(id)`** MUST return `NOT_FOUND`. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:201`]

### 1.4 Search & Query (gated by `canSearch` / `canQuery`)
- **`search(query)`** MUST return beats whose `title` includes the query substring, or whose `id` matches exactly. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:223`]
- **`query(expression)`** MUST return beats matching the expression (e.g., `type:task`). [source: `foolery/src/lib/__tests__/backend-contract.test.ts:246`]

### 1.5 Dependency Management (gated by `canManageDependencies`)
- **`addDependency(source, target)`** MUST create a dependency edge. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:269`]
- **`listDependencies(id)`** MUST return the added dependencies. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:284`]
- **`removeDependency(source, target)`** MUST remove the edge and subsequent `listDependencies` MUST return an empty array. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:299`]

### 1.6 Error Contract
- Every error result MUST have shape `{ ok: false, error: { code, message, retryable } }`. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:329`]
- `code` MUST be a valid `BackendErrorCode`. [source: `foolery/src/lib/__tests__/backend-contract.test.ts:338`]

---

## 2. Backend Error Taxonomy

### 2.1 Error Codes & Retryability
| Code | Retryable by default |
|---|---|
| `NOT_FOUND` | false |
| `ALREADY_EXISTS` | false |
| `INVALID_INPUT` | false |
| `LOCKED` | true |
| `TIMEOUT` | true |
| `UNAVAILABLE` | true |
| `PERMISSION_DENIED` | false |
| `INTERNAL` | false |
| `CONFLICT` | false |
| `RATE_LIMITED` | true |

[source: `foolery/src/lib/__tests__/backend-errors.test.ts:68`]

### 2.2 Error Construction
- `BackendError` MUST store `code`, `message`, and a boolean `retryable`. [source: `foolery/src/lib/__tests__/backend-errors.test.ts:24`]
- It MUST allow optional `details` (map) and optional `cause` (wrapped error). [source: `foolery/src/lib/__tests__/backend-errors.test.ts:37`]
- When `retryable` is not explicitly provided, the default MUST follow the table above. [source: `foolery/src/lib/__tests__/backend-errors.test.ts:32`]

### 2.3 Message Classification
- A `classifyErrorMessage` function MUST map raw backend strings to canonical codes:
  - Substrings containing `not found`, `no such file`, `does not exist` -> `NOT_FOUND`
  - Substrings containing `already exists`, `duplicate key` -> `ALREADY_EXISTS`
  - Substrings containing `database is locked`, `could not obtain lock` -> `LOCKED`
  - Substrings containing `timed out`, `timeout` -> `TIMEOUT`
  - Substrings containing `permission denied`, `unauthorized`, `EACCES` -> `PERMISSION_DENIED`
  - Substrings containing `server busy`, `service unavailable`, `unable to open database` -> `UNAVAILABLE`
  - Unrecognized messages MUST fall back to `INTERNAL`. [source: `foolery/src/lib/__tests__/backend-errors.test.ts:210`]
- Classification MUST be case-insensitive. [source: `foolery/src/lib/__tests__/backend-errors.test.ts:242`]

### 2.4 Suppressibility
- Errors with codes `LOCKED`, `TIMEOUT`, `UNAVAILABLE`, `RATE_LIMITED` are suppressible (eligible for degraded-mode caching). All other codes are non-suppressible. [source: `foolery/src/lib/__tests__/backend-errors.test.ts:251`]

### 2.5 HTTP Status Mapping
- `INVALID_INPUT` -> 400
- `NOT_FOUND` -> 404
- `UNAVAILABLE` -> 503
- Unknown codes -> 500 [source: `foolery/src/lib/__tests__/backend-http.test.ts:13`]

---

## 3. Auto-Routing & Dispatch Failures (Fail Loudly)

The system MUST implement an auto-routing backend that selects a concrete backend based on a repo path. It MUST NEVER silently fall back to a default backend.

### 3.1 No-Repo-Path Failure
- Any backend operation invoked without a `repoPath` MUST throw a `DispatchFailureError` with:
  - Marker phrase: `FOOLERY DISPATCH FAILURE`
  - `kind`: `backend`
  - `reason`: `repo_path_missing`
  - `method`: the name of the invoked operation
  - No backend constructor MUST be called. [source: `foolery/src/lib/__tests__/backend-factory-no-fallback.test.ts:85`]

### 3.2 Unknown Repo Type Failure
- Any operation against a repo whose memory-manager type cannot be detected MUST throw a `DispatchFailureError` with:
  - Marker phrase: `FOOLERY DISPATCH FAILURE`
  - `reason`: `repo_type_unknown`
  - `repoPath`: the supplied path
  - `method`: the invoked operation name
  - A banner string MUST contain the marker, method name, and repo path. [source: `foolery/src/lib/__tests__/backend-factory-no-fallback.test.ts:103`]

### 3.3 Routing Rules
- `knots` memory manager -> `KnotsBackend`
- `beads` memory manager -> `BdCliBackend` (or equivalent CLI adapter)
- `listWorkflows()` with no `repoPath` MAY return built-in descriptors without delegating. [source: `foolery/src/lib/__tests__/backend-factory-no-fallback.test.ts:139`]

### 3.4 Capability Model
- `FULL_CAPABILITIES`: all boolean flags true, `maxConcurrency: 0` (unlimited). [source: `foolery/src/lib/__tests__/backend-capabilities.test.ts:84`]
- `READ_ONLY_CAPABILITIES`: write flags false, read/search/query/listReady true, `maxConcurrency: 0`. [source: `foolery/src/lib/__tests__/backend-capabilities.test.ts:109`]
- `assertCapability` MUST throw an error naming the missing capability and the operation when a required flag is false or `maxConcurrency == 0`. [source: `foolery/src/lib/__tests__/backend-capabilities.test.ts:12`]
- Capability presets MUST be immutable (frozen). [source: `foolery/src/lib/__tests__/backend-capabilities.test.ts:141`]

---

## 4. CLI Backend (`bd`) Behaviors

### 4.1 Execution Model
- The CLI adapter MUST delegate to a `bd` executable. It MUST NOT write JSONL directly. [source: `AGENTS.md` CLI-First Backend principle]
- Commands MUST be serialized per repo path: concurrent calls for the same repo MUST execute serially; concurrent calls for different repos MAY execute in parallel. [source: `foolery/src/lib/__tests__/bd-serialization.test.ts:38`]

### 4.2 Read-Mode Environment
- Read commands (list, ready, search, query, show) MUST set `BD_NO_DB=true` by default. [source: `foolery/src/lib/__tests__/bd-read-no-db.test.ts:59`]
- Write commands MUST NOT force `BD_NO_DB`. [source: `foolery/src/lib/__tests__/bd-read-no-db.test.ts:70`]
- If a read command panics with a nil-pointer dereference when `BD_NO_DB` is disabled, the adapter MUST retry the same command with `BD_NO_DB=true`. [source: `foolery/src/lib/__tests__/bd-read-no-db.test.ts:81`]

### 4.3 Execution Pipeline (3-layer)

The CLI adapter uses a 3-layer execution pipeline [source: `foolery/src/lib/bd-internal.ts:401-548`]:

```
exec() → execSerializedAttempt() → execOnce()
```

**Layer 3 — `execOnce()`**: Raw `child_process.execFile(bd_bin, args, {timeout, killSignal})`. Returns `{stdout, stderr, exitCode, timedOut}`. No retry logic.

**Layer 2 — `execSerializedAttempt()`**: Wrapped in per-repo process lock + promise-chain serialization. On failure:
1. If read command + Dolt nil-pointer panic detected (stderr contains `DOLT_PANIC_STACK_SIGNATURE`) → retry once with `BD_NO_DB=true` (bypasses Dolt, uses JSONL directly). [source: `foolery/src/lib/bd-internal.ts:448-495`]
2. If stderr contains `OUT_OF_SYNC_SIGNATURE` (`"Database out of sync with JSONL"`) → auto-heal: run `bd sync --import-only` → retry original command once. If sync itself fails, return original out-of-sync error. [source: `foolery/src/lib/bd-internal.ts:448-495`]

**Layer 1 — `exec()`**: Up to 2 attempts for retryable commands (read-only or idempotent writes: `update`, `label add/remove`, `sync`, `dep remove`). Non-idempotent writes (`create`, `close`, `dep add`) get 1 attempt. Returns `BdResult<T>`.

### 4.4 Repo Serialization & File Locks
- Per-repo file-based locking in `$TMPDIR/foolery-bd-locks/` named by SHA1 of resolved repo path. [source: `foolery/src/lib/bd-internal.ts:242-348`]
- `owner.json` inside lock dir: `{pid, repoPath, acquiredAt}`. Lock via `mkdir` (atomic), poll on `EEXIST` → check mtime stale (>10 min) → evict → retry.
- Promise-chain serialization per repo key: `Map<string, {tail: Promise<void>, pending: int}>`. All `bd` commands for the same repo execute sequentially; different repos run in parallel.
- `--no-daemon` flag: tried first on label operations and `bd sync`. If bd CLI returns "unknown flag" error, retried without the flag (backward compat). [source: `foolery/src/lib/bd-internal.ts:535-548`]

### 4.5 Timeout & Retry
- Read commands that time out (process killed by signal) MUST be retried once. If the retry also times out, the result MUST be a `TIMEOUT` error containing `bd command timed out after`. [source: `foolery/src/lib/__tests__/bd-timeout.test.ts:61`]
- Idempotent write commands (update) that time out MUST be retried once and may succeed. [source: `foolery/src/lib/__tests__/bd-timeout.test.ts:76`]
- Non-idempotent write commands (create) MUST NOT be retried after timeout; they MUST return the timeout error immediately. [source: `foolery/src/lib/__tests__/bd-timeout.test.ts:111`]

### 4.4 Out-of-Sync Recovery
- If a read command fails with stderr containing `Database out of sync with JSONL`, the adapter MUST:
  1. Run `bd sync --import-only`
  2. Retry the original command once.
  3. If the retry succeeds, return its result; otherwise return the original error. [source: `foolery/src/lib/__tests__/bd-write.test.ts:395`]
  4. If `sync --import-only` itself fails, return the original out-of-sync error. [source: `foolery/src/lib/__tests__/bd-write.test.ts:418`]
- Non-out-of-sync errors MUST NOT trigger sync. [source: `foolery/src/lib/__tests__/bd-auto-import-sync.test.ts:69`]

### 4.5 CRUD Delegation
- `listBeats`: passes `--all` when no status filter is provided; omits `--all` when a status filter is given. [source: `foolery/src/lib/__tests__/bd-read.test.ts:88`]
- `searchBeats`: maps `priority` filter to `--priority-min` and `--priority-max`. [source: `foolery/src/lib/__tests__/bd-read.test.ts:194`]
- `queryBeats`: passes `--limit` and `--sort` when provided. [source: `foolery/src/lib/__tests__/bd-read.test.ts:234`]
- `showBeat`: handles both object and array responses from the CLI, returning the first element if an array is received. [source: `foolery/src/lib/__tests__/bd-read.test.ts:268`]
- `createBeat`: returns parsed JSON `id` on success. If JSON parsing fails, it MUST fall back to using the raw stdout trimmed string as the ID. If stdout is empty and parsing fails, return an error. [source: `foolery/src/lib/__tests__/bd-write.test.ts:72`]
- `deleteBeat`: MUST pass `--force`. [source: `foolery/src/lib/__tests__/bd-write.test.ts:134`]
- `closeBeat`: MUST pass `--reason` when a reason is provided; MUST omit it otherwise. [source: `foolery/src/lib/__tests__/bd-write.test.ts:159`]
- `updateBeat`: MUST accept only kernl-native `IssueStatus` values in `input.State` (`"open"`, `"in_progress"`, `"awaiting_integration"`, `"awaiting_pr_review"`, `"blocked"`, `"closed"`). MUST reject foolery-era legacy status strings with a descriptive error. See §4.12 for the full validation contract and legacy-to-kernl mapping. [source: `docs/plans/2026-05-15-kernl-workflow-plan.md:1342-1343`]

### 4.6 Error Suppression Cache Details
- Cache key construction: `${fn_name}:${query}:${sorted_filters_JSON}:${repoPath}` — filters sorted alphabetically before serialization for deterministic keys. [source: `foolery/src/lib/bd-error-suppression.ts:50`]
- `SUPPRESSIBLE_PATTERNS` match lowercase stderr: `["lock","locked","timed out waiting for bd repo lock","bd command timed out","database is locked","unable to open database","could not obtain lock","busy","eacces","permission denied"]`. [source: `foolery/src/lib/bd-error-suppression.ts:34`]
- Degraded error returned with code `UNAVAILABLE`, retryable=true. The message: `"Unable to interact with beats store right now; please try again shortly."`. [source: `foolery/src/lib/bd-error-suppression.ts:68`]

### 4.7 Label Reconciliation
- `updateBeat` with stage labels MUST:
  1. Load the existing beat via `show`.
  2. Remove any existing `stage:*` label before adding the new one.
  3. Run `bd sync` (or `bd sync --no-daemon`) after label mutations. [source: `foolery/src/lib/__tests__/bd-update-labels.test.ts:47`]
- If the `bd` CLI does not support `--no-daemon`, the adapter MUST retry the label/sync operation without that flag. [source: `foolery/src/lib/__tests__/bd-update-labels.test.ts:124`]
- Adding custom (non-stage) labels MUST NOT trigger the stage-label reconciliation logic. [source: `foolery/src/lib/__tests__/bd-update-labels.test.ts:121`]

### 4.7 Dependency Operations
- `addDep`: MUST use `--blocks` semantics (blocker -> blocked). [source: `foolery/src/lib/__tests__/bd-write.test.ts:233`]
- `removeDep`: MUST invoke `bd dep remove`. [source: `foolery/src/lib/__tests__/bd-write.test.ts:258`]
- `listDeps`: MUST support an optional `type` filter passed as `--type`. [source: `foolery/src/lib/__tests__/bd-write.test.ts:201`]

### 4.8 Beat Normalization (CLI -> Domain)
- `issue_type` maps to `type`; defaults to `task` if missing or invalid. [source: `foolery/src/lib/__tests__/bd-write.test.ts:284`]
- `status` maps to `state`; defaults to workflow initial state (`ready_for_implementation`) if missing or invalid. [source: `foolery/src/lib/__tests__/bd-write.test.ts:319`]
- `created_at` / `updated_at` map to `created` / `updated`. [source: `foolery/src/lib/__tests__/bd-write.test.ts:291`]
- `acceptance_criteria` maps to `acceptance`. [source: `foolery/src/lib/__tests__/bd-write.test.ts:303`]
- `estimated_minutes` maps to `estimate`. [source: `foolery/src/lib/__tests__/bd-write.test.ts:312`]
- Empty or whitespace-only labels MUST be filtered out. [source: `foolery/src/lib/__tests__/bd-write.test.ts:355`]
- Parent inference:
  - If `dependencies` contains a `parent-child` entry, `parent` MUST be set to `depends_on_id`. [source: `foolery/src/lib/__tests__/bd-write.test.ts:364`]
  - If the ID contains dots (e.g., `proj.1.2`), `parent` MUST be the prefix before the last dot (`proj.1`). [source: `foolery/src/lib/__tests__/bd-write.test.ts:377`]
- Unlabeled open beats map to `ready_for_implementation`; unlabeled in-progress beats map to `implementation`. [source: `foolery/src/lib/__tests__/bd-write.test.ts:328`]

### 4.9 Filters
- Filter values that are `null`, `undefined`, or empty strings MUST be omitted from CLI args. [source: `foolery/src/lib/__tests__/bd-cli-backend.test.ts:308`]
- If all filters are empty, the function MUST pass `undefined` (no filters) to the CLI. [source: `foolery/src/lib/__tests__/bd-cli-backend.test.ts:315`]

### 4.10 Custom Statuses

The `BdCliBackend` MUST register kernl custom statuses with `bd` idempotently via `EnsureCustomStatuses(beadsDir string, r BdRunner) error`.

#### 4.10.1 Contract

- **Idempotent:** Calling twice with the same `beadsDir` is a no-op (in-memory cache hit).
- **Cache + sentinel:** An in-memory `map[string]bool` keyed by absolute `beadsDir` path avoids repeated `bd` invocations. A sentinel file `.beads/.kernl-custom-statuses-installed` is written as an advisory marker, but the hot path is the in-memory cache.
- **Merges foreign customs:** Reads current customs first via `BdRunner.GetCustomStatuses()`, appends only kernl customs that are missing, sorts the merged list, and writes the **union** back via `SetCustomStatuses`. Never overwrites foreign customs registered by other consumers.
- **Registered customs:** `"awaiting_integration"` and `"awaiting_pr_review"` (from `workflow.KernlCustomStatuses`).
- **Called on every backend op:** `NewBdCliBackend(repoPath)` calls `EnsureCustomStatuses(filepath.Join(repoPath, ".beads"), b)` once at backend creation. Returns `(*BdCliBackend, error)` — fails loud if registration fails. [source: `docs/plans/2026-05-15-kernl-workflow-plan.md:960-1058`]
- **Test hook:** `ResetEnsureCache()` clears the in-memory cache for hermetic tests. [source: `docs/plans/2026-05-15-kernl-workflow-plan.md:1038-1043`]

#### 4.10.2 `BdRunner` Interface

```go
type BdRunner interface {
    GetCustomStatuses() ([]string, error)
    SetCustomStatuses(list []string) error
}
```

`BdCliBackend` implements `BdRunner` by delegating to `bd config get/set status.custom`. [source: `docs/plans/2026-05-15-kernl-workflow-plan.md:972-977,1317-1328`]

### 4.11 Description-Field Contracts

Stable metadata fields are stored as `"key: value"` lines in the bead's `description` text. Parsing/writing mirrors `gastown/internal/beads/integration.go:69-128`. [source: `docs/2026-05-15-kernl-workflow-brainstorm-spec.md §3.3, §4.5`]

#### 4.11.1 Primitives

| Function | Signature | Behavior |
|---|---|---|
| `GetMetadataField` | `(desc, key string) string` | Extracts first `key: value` line; case-insensitive key match; returns `""` if absent |
| `AddMetadataField` | `(desc, key, value string) string` | Inserts or updates first occurrence; removes duplicate keys |

#### 4.11.2 Stable Fields & Typed Accessors

| Stable field | Bead type | Set when | Getter | Setter |
|---|---|---|---|---|
| `worktree_path` | child | Once at worktree spawn | `GetWorktreePath(d) string` | `SetWorktreePath(d, v) string` |
| `worktree_branch` | child | Once at worktree spawn | `GetWorktreeBranch(d) string` | `SetWorktreeBranch(d, v) string` |
| `epic_branch` | epic | Once at epic creation | `GetEpicBranch(d) string` | `SetEpicBranch(d, v) string` |
| `pr_url` | epic | Once after `gh pr create` | `GetPRURL(d) string` | `SetPRURL(d, v) string` |
| `merge_conflict_at` | epic | Once if merge detects conflict | `GetMergeConflictAt(d) string` | `SetMergeConflictAt(d, v) string` |
| `merge_outcome` | epic | Once when merger finishes | `GetMergeOutcome(d) string` | `SetMergeOutcome(d, v) string` |

`merge_outcome` enum values: `"success"`, `"merge_conflict"`, `"push_failed"`, `"pr_create_failed"`, `"pr_already_exists"`. [source: `docs/2026-05-15-kernl-workflow-brainstorm-spec.md §5.3`]

**Design note:** `AgentState` (high-frequency runtime: heartbeat, follow_up_count, session_id) is NOT stored in description. It lives in `~/.kernl/state/<bead-id>.json` to avoid lost-update races between concurrent worker heartbeats and merger description writes. This is the one conscious divergence from the gastown model. [source: `docs/2026-05-15-kernl-workflow-brainstorm-spec.md §3.3`]

### 4.12 Status Validation on Write

`bdcli.Update(input.State)` MUST accept only kernl-native `IssueStatus` values:

| Accepted | Rejected |
|---|---|
| `"open"`, `"in_progress"`, `"awaiting_integration"`, `"awaiting_pr_review"`, `"blocked"`, `"closed"` | `"ready_for_implementation"`, `"implementation"`, `"implementation_review"`, `"ready_for_shipment"`, `"shipment"`, `"shipment_review"`, `"shipped"`, `"deferred"`, `"abandoned"`, and any other foolery-era legacy string |

Passing a legacy status string MUST return a descriptive error naming the invalid value and listing the accepted constants. [source: `docs/plans/2026-05-15-kernl-workflow-plan.md:1342-1343`, `docs/2026-05-15-kernl-workflow-brainstorm-spec.md §2`]

Legacy-to-kernl mapping (for migration reference only — NOT performed automatically by the backend):

| Legacy | Kernl native |
|---|---|
| `"ready_for_implementation"` | `"open"` |
| `"implementation"` | `"in_progress"` |
| `"implementation_review"`, `"ready_for_shipment"` | `"awaiting_integration"` |
| `"shipment"`, `"shipment_review"`, `"shipped"` | `"closed"` |

---

## 5. Registry Behaviors

### 5.1 Data Model
- Registry is a JSON file containing a `repos` array. Each entry has `path`, `name`, `addedAt`, and optionally `memoryManagerType`. [source: `foolery/src/lib/__tests__/registry.test.ts:41`]

### 5.2 Memory Manager Inference
- Legacy entries missing `memoryManagerType` MUST be inferred at load time:
  - If detection finds `.knots`, type is `knots`.
  - Otherwise default to `beads`. [source: `foolery/src/lib/__tests__/registry.test.ts:41`]
- Unknown `memoryManagerType` strings MUST be normalized via detection fallback. [source: `foolery/src/lib/__tests__/registry-coverage.test.ts:41`]

### 5.3 Backfill
- `backfillMissingRepoMemoryManagerTypes` MUST write the registry only when at least one repo is missing the field. [source: `foolery/src/lib/__tests__/registry.test.ts:138`]
- On missing file (`ENOENT`), it MUST return `fileMissing: true` and not write. [source: `foolery/src/lib/__tests__/registry.test.ts:158`]
- On non-ENOENT read errors (e.g., `EACCES`), it MUST return an error string and `fileMissing: false`. [source: `foolery/src/lib/__tests__/registry-coverage.test.ts:63`]
- Invalid JSON or non-object parsed values MUST be treated as no-ops (`changed: false`). [source: `foolery/src/lib/__tests__/registry-coverage.test.ts:74`]

### 5.4 Permissions
- Registry file MUST be created/chmod-ed to `0o600`. [source: `foolery/src/lib/__tests__/registry.test.ts:180`]
- `inspectRegistryPermissions` MUST detect when the mode is looser than `0o600` and report `needsFix: true`. [source: `foolery/src/lib/__tests__/registry.test.ts:171`]

### 5.5 Repo Lifecycle
- `addRepo(path)` MUST throw if no memory manager is detected. [source: `foolery/src/lib/__tests__/registry-coverage.test.ts:136`]
- `addRepo(path)` MUST throw if the repo is already registered. [source: `foolery/src/lib/__tests__/registry-coverage.test.ts:143`]
- `removeRepo(path)` MUST filter out the repo and persist. [source: `foolery/src/lib/__tests__/registry-coverage.test.ts:167`]
- `loadRegistry` MUST skip entries with no `path` or empty `path`. [source: `foolery/src/lib/__tests__/registry-coverage.test.ts:206`]

---

## 6. Local Worker Behaviors

### 6.1 Tool Execution Guardrails
- `shell_exec` MUST block commands that invoke memory-manager binaries (`kno`, `bd`). [source: `foolery/src/lib/__tests__/local-worker.test.ts:60`]

### 6.2 Poll & Take Preparation
- `preparePoll(repoPath)` MUST return a lease containing the claimed beat ID and a prompt that references it. [source: `foolery/src/lib/__tests__/local-worker.test.ts:72`]
- `prepareTake` for a parent beat MUST include child beat IDs in the prompt and instruct parallel execution when practical. [source: `foolery/src/lib/__tests__/local-worker.test.ts:121`]
- For `knots`-backed repos, `preparePoll` MUST create a lease with a populated `knotsLeaseId`. [source: `foolery/src/lib/__tests__/local-worker.test.ts:154`]

### 6.3 Canonical Metadata
- Lease creation MUST propagate canonical agent metadata: `agentName`, `model`, `modelVersion`, `provider`, `agentType`. [source: `foolery/src/lib/__tests__/local-worker.test.ts:202`]

---

## 7. Server Timing

- A middleware/handler wrapper MUST emit a `Server-Timing` header containing measured segments (e.g., `read;dur=...`) and a `total` duration. [source: `foolery/src/lib/__tests__/server-timing.test.ts:11`]
- If a request exceeds a configured `slowMs` threshold, it MUST log a structured warning with the route and repo context. [source: `foolery/src/lib/__tests__/server-timing.test.ts:24`]

---

## 8. Beads JSONL DTO & State Mapping

### 8.1 Normalization (RawBead -> Beat)
- Field mappings MUST be:
  - `issue_type` -> `type`
  - `status` -> `state`
  - `acceptance_criteria` -> `acceptance` (preferred over raw `acceptance`)
  - `estimated_minutes` -> `estimate` (preferred over raw `estimate`)
  - `created_at` / `updated_at` / `closed_at` -> `created` / `updated` / `closed`
  - `metadata` MUST include `close_reason` when present. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:70`]
- Defaults for missing/invalid fields:
  - `type` defaults to `task`. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:112`]
  - `state` defaults to workflow initial state (`ready_for_implementation`). [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:118`]
  - `priority` defaults to `2` for out-of-range or non-numeric values. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:124`]
- Parent inference rules (same as CLI normalization):
  - Explicit `parent` field wins.
  - Dotted ID infers parent from prefix before last dot.
  - `dependencies` array with `parent-child` type infers parent. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:94`]
- Empty/whitespace labels MUST be filtered. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:157`]

### 8.2 Invariant Parsing
- If `notes` contains a section headed `[Invariants]`, each subsequent line matching `Kind: condition` MUST be parsed into an `invariants` array and removed from the visible `notes`. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:230`]
- If no valid invariant lines follow the header, `notes` MUST be left unchanged. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:258`]
- Invariant condition strings MUST be trimmed. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:270`]

### 8.3 Denormalization (Beat -> RawBead)
- All domain fields MUST map back to their raw counterparts. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:288`]
- Optional undefined fields MUST be omitted from the raw output. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:333`]
- Invariants MUST be embedded into `notes` under an `[Invariants]` block. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:361`]
- Blank invariant conditions MUST be skipped when embedding. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:401`]

### 8.4 Round-Trip Fidelity
- A full round-trip (`raw -> domain -> raw -> domain`) MUST preserve all scalar fields. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:426`]
- Workflow labels (`wf:state:*`, `wf:profile:*`) are additive during denormalization; therefore round-tripped labels will include them even if absent originally. [source: `foolery/src/lib/__tests__/beads-jsonl-dto.test.ts:440`]

### 8.5 Compat Status Mapping
- `mapWorkflowStateToCompatStatus`:
  - `deferred` -> `deferred`
  - `blocked`, `rejected` -> `blocked`
  - Terminal states (`shipped`, `abandoned`, `closed`, `done`, `approved`) -> `closed`
  - Queue states (`ready_for_*`) -> `open`
  - Active states (`planning`, `implementation`, `shipment`) -> `in_progress`
  - Unknown/empty -> `open` [source: `foolery/src/lib/backends/__tests__/beads-compat-status.test.ts:18`]
- `mapStatusToDefaultWorkflowState`:
  - `closed` -> terminal state (e.g., `shipped`)
  - `deferred` -> `deferred`
  - `blocked` -> `retakeState`
  - `in_progress` -> first action state (or `implementation` if present, else fallback `in_progress`)
  - `open` -> `initialState`
  - Unknown -> `initialState` [source: `foolery/src/lib/backends/__tests__/beads-compat-status.test.ts:55`]

---

## 9. Beads State Machine

### 9.1 State Transitions
- `nextBeat(id, expectedState)` MUST:
  - Verify the beat's current state matches `expectedState`; if not, throw a state mismatch error containing the substrings `expected state` and `currently`. [source: `foolery/src/lib/__tests__/beads-state-machine.test.ts:48`]
  - Advance the beat to the next forward state in its workflow and persist via `update`. [source: `foolery/src/lib/__tests__/beads-state-machine.test.ts:42`]
  - Throw `not found` if the beat does not exist. [source: `foolery/src/lib/__tests__/beads-state-machine.test.ts:65`]
  - Throw `no forward transition` if the beat is in a terminal state. [source: `foolery/src/lib/__tests__/beads-state-machine.test.ts:69`]

### 9.2 Claim
- `claimBeat(id)` MUST transition a queued state (`ready_for_*`) to its active counterpart (e.g., `ready_for_implementation` -> `implementation`). [source: `foolery/src/lib/__tests__/beads-state-machine.test.ts:99`]
- It MUST throw a state mismatch error if the beat is already active or terminal. [source: `foolery/src/lib/__tests__/beads-state-machine.test.ts:112`]
- It MUST throw a state mismatch error if the beat is queued but human-owned (non-agent-claimable, e.g., `ready_for_plan_review` under `semiauto` profile). [source: `foolery/src/lib/__tests__/beads-state-machine.test.ts:136`]

---

## 10. Error Suppression & Degraded Mode

### 10.1 Cache Behavior
- Successful results MUST be cached per operation signature (method + filters + repo path + query). [source: `foolery/src/lib/__tests__/bd-error-suppression.test.ts:39`]
- Cache keys MUST be order-independent for filter maps. [source: `foolery/src/lib/__tests__/bd-error-suppression.test.ts:178`]
- Cache MUST have a maximum entry limit (e.g., 64); oldest entries MUST be evicted on overflow. [source: `foolery/src/lib/__tests__/bd-error-suppression.test.ts:146`]

### 10.2 Suppression Window
- On the first suppressible error after a successful cache entry, the system MUST return the cached data and enter a failure state. [source: `foolery/src/lib/__tests__/bd-error-suppression.test.ts:53`]
- While inside the suppression window (e.g., 2 minutes), subsequent suppressible errors MUST continue returning the cached data. [source: `foolery/src/lib/__tests__/bd-error-suppression.test.ts:61`]
- After the suppression window expires, suppressible errors MUST return a degraded error message (e.g., `Backend is temporarily unavailable; displaying cached data.`). [source: `foolery/src/lib/__tests__/bd-error-suppression.test.ts:69`]

### 10.3 TTL & Cleanup
- Cache entries older than a TTL (e.g., 10 minutes) MUST be evicted. [source: `foolery/src/lib/__tests__/bd-error-suppression-extended.test.ts:36`]
- If the cache entry expires during an ongoing failure, the next suppressible error MUST return the raw error (not degraded) and clear the failure state. [source: `foolery/src/lib/__tests__/bd-error-suppression-extended.test.ts:56`]
- Recovery (a successful result) MUST clear the failure state for that signature. [source: `foolery/src/lib/__tests__/bd-error-suppression.test.ts:83`]

### 10.4 Non-Suppressible Errors
- Parse failures and generic non-lock errors MUST NOT be suppressed; they MUST be returned as-is even if cached data exists. [source: `foolery/src/lib/__tests__/bd-error-suppression.test.ts:132`]

---

## 11. Testing Principles (Hermetic)

- All default-suite tests MUST be hermetic: no real `exec`, no real filesystem, no real network, no wall-clock timers. Dependencies MUST be injected or mocked at boundaries. [source: `AGENTS.md` Hermetic Test Policy]
- Integration tests that exercise real CLIs or processes MUST live in a separate tagged suite (e.g., `//go:build integration`) and MUST NOT run in the default `go test ./...`. [source: `AGENTS.md` Integration Tests]
- Mocks MUST be named fakes or stub structs implementing interfaces. No inline anonymous mocks. [source: `AGENTS.md` Mocks]
