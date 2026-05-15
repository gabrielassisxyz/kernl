# Foolery Go Port — System Architecture & Cross-Domain Contracts

This document synthesizes the system-wide behavioral contracts and component interactions across all domains (backend, session, dispatch, prompt, transport, orchestration). It is derived from the TypeScript Foolery test suite and serves as the authoritative blueprint for the Go port.

> **Principle:** Every contract below is what the Go port MUST preserve. Implementation choices (goroutines vs. event emitters, channels vs. callbacks) are secondary to the observable behavior.

---

## 1. System Overview

Foolery is a keyboard-first orchestration app for agent-driven software work. The Go backend manages beat state, dispatches agents, streams results, and maintains a session registry. The Vue 3 frontend consumes SSE, queries beats, and renders terminal sessions.

### Core Components

| Component | Ownership | Key Contracts |
|---|---|---|
| Backend Port | `internal/backend` | CRUD, search, deps, error taxonomy, capabilities, CLI delegation |
| Dispatch | `internal/dispatch` | Pool resolution, agent selection, identity, swap, loud failures |
| Session Runtime | `internal/session` | Stdio wiring, JSON line parsing, result detection, lifecycle |
| Transport | `internal/transport` | Dialect adapters (Claude, Codex, Copilot, Gemini, OpenCode) |
| Terminal Manager | `internal/terminal` | Take loop, retry, rollback, watchdog, approvals, forensics |
| Orchestration | `internal/orchestration` | Workflow state machine, hierarchy, sorting, plans, grooming |
| Prompt Engine | `internal/prompt` | Guardrails, scope refinement, skill prompts, boundaries |

### Communication Model
- **Goroutine-per-session** — each terminal session owns a goroutine. No shared mutable state across sessions.
- **Channels over EventEmitters** — replace TS `EventEmitter` with Go channels (`chan Event`) for stdout/stderr/lifecycle streams.
- **SSE broadcast** — a central broker fans out session events to connected HTTP clients.

---

## 2. Cross-Cutting Invariants

### 2.1 Fail Loudly, Never Silently
Every lookup failure for a configured resource MUST:
1. Return an error that halts the current operation.
2. Log a structured error containing the greppable marker `FOOLERY DISPATCH FAILURE` (or subsystem marker).
3. Surface the failure to any visible session buffer as a stderr banner event.
4. Name the missing thing (beat id, state, pool key, workflow id, action name) and the exact config that fixes it.
5. NEVER return `Object.values(x)[0]`, `?? "default"`, or any silent fallback.

### 2.2 Hermetic Tests
- Default tests MUST NOT touch the host environment (no real `os.Getenv`, `os.Open`, `exec.Command`, network, or wall-clock timers).
- Mock at boundaries via named fake structs implementing interfaces.
- Integration tests that exercise real CLIs MUST be tagged `//go:build integration` and excluded from `go test ./...`.

### 2.3 kno / Loom Workflows Are Authoritative
- Valid next states MUST come exclusively from loom-defined transitions.
- No synthetic transitions. No fabricated rollback targets.
- Correction actions that skip gates MUST invoke the backend with `force: true`.

### 2.4 CLI-First Backend
- ALL storage mutations MUST delegate to `bd` or `kno` CLI.
- NEVER write `.beads/issues.jsonl` directly.
- Parse `bd` output as NDJSON (`bufio.Scanner` + `json.Decoder`).

### 2.5 Registry Permissions
- Registry file MUST be created/chmod-ed to `0o600`.
- `inspectRegistryPermissions` MUST detect looser modes and report `needsFix: true`.

---

## 3. Backend → Dispatch → Session Integration

### 3.1 Beat Lifecycle (Backend Port)
```
Backend.listWorkflows() → []WorkflowDescriptor
Backend.list() → []Beat
Backend.get(id) → Beat | NOT_FOUND
Backend.create(BeatInput) → Beat (ok + id)
Backend.update(id, fields) → ok | error
Backend.close(id, reason) → TerminalState
Backend.delete(id) → ok | error
Backend.search(query) → []Beat
Backend.query(expr) → []Beat
Backend.addDependency(src, tgt) → ok
Backend.removeDependency(src, tgt) → ok
```

### 3.2 Dispatch Pool Resolution
```
WorkflowDescriptor.queueActions[state] → poolKey
  → DispatchPoolSettings[poolKey] → []WeightedAgentEntry
    → selectFromPool(entries, excludeAgentId?) → AgentConfig
```

**Invariant:** If any lookup in this chain fails, the system MUST throw a `DispatchFailureError` with marker `FOOLERY DISPATCH FAILURE`, naming the missing `poolKey` and the exact config path (`settings.pools.<poolKey>`).

### 3.3 Session Spawning
```
TerminalManager.createSession(beatId, repoPath)
  → resolveDispatchAgent(beat, poolKey, exclude)
  → buildTakePrompt(beat, agent)
  → spawnChild(agent.command, agent.args...)
  → sessionRuntime.start()
```

**Invariant:** One-shot agents MUST NOT be dispatched for take/scene operations. Attempting to do so MUST throw with `TERMINAL_DISPATCH_FAILURE_MARKER`.

---

## 4. Session → Transport → Event Stream Integration

### 4.1 Stdio Wiring Model
```
Child stdout ──▶ bufio.Scanner ──▶ JSON line parser ──▶ Event normalizer ──▶ Session event channel
Child stderr ──▶ verbatim ──▶ Session event channel (stderr event)
```

**Contracts:**
- Lines without trailing newline at stream close MUST be flushed as a final event.
- Empty lines MUST be ignored.
- Abort signals (context cancellation) MUST exit the read loop immediately.

### 4.2 Event Normalization Matrix

| Transport | Turn-End Signal | `resultObserved` | `exitReason` | Auto-Reply AskUser |
|---|---|---|---|---|
| Claude stdio | `type: "result"` | true | `"turn_ended"` | Yes (confirmation to stdin) |
| Codex JSON-RPC | `turn/completed` | true | `"turn_ended"` | No |
| Copilot interactive | `session.task_complete` | true | `"turn_ended""` | Yes (`"auto-response"` to stdin) |
| Gemini ACP | `stopReason: "end_turn"` | true | `"turn_ended"` | Yes (`allow_once` to stdin) |
| OpenCode HTTP | `session_idle` | true | `"turn_ended"` | No (UI routing via `onApprovalRequest`) |

**Invariant:** `onTurnEnded` MUST deduplicate (fire exactly once per turn). Per-message boundaries (`step_finish`, `item/completed`) MUST NOT fire `onTurnEnded`.

### 4.3 Watchdog
- Default timeout: 10 minutes for interactive sessions.
- Resets on ANY stdout event.
- Fires even after `resultObserved == true` (result does not guarantee child exit).
- Before SIGTERM, warn via log containing `[terminal-manager] [watchdog]`, `timeout_fired`, `pid`, `timeoutMs`.
- SIGTERM at t=0; SIGKILL after delay if still alive.

### 4.4 Lifecycle Callbacks
`sendUserTurn` MUST emit:
- `prompt_delivery_attempted` (transport = `"stdio"`)
- `prompt_delivery_succeeded` (transport = `"stdio"`)

---

## 5. Terminal Manager → Backend → Orchestration Integration

### 5.1 Take Loop State Machine
```
Claim beat → Spawn agent → Stdio session
  → Child exits
    → Post-exit state fetch
      → [Success] Beat advanced to next state → End
      → [Failure] Non-zero exit
        → Rollback beat to prior queue state
        → If alternative agent exists → Retry with different agent
        → Else → End (log outcome)
      → [Stuck] Active state after exit
        → Follow-up prompt (up to 4 more times)
        → If cap reached (5 stuck turns), emit banner `"follow-up cap reached"`
```

### 5.2 Rollback Invariants
- Rollback MUST transition from active state to its corresponding queue state.
- Rollback MUST happen BEFORE retry spawns a new child.
- If rollback throws (e.g., `"not agent-claimable"`), the session MUST reject without spawning children.
- If beat is already in a claimable queue state, rollback MUST be skipped.

### 5.3 Agent Rotation
- Error retry MUST exclude the previously failed agent.
- Pool exhaustion (no alternative) MUST stop retry.
- `lastAgentPerQueueType` tracks soft exclusion for rotation.
- Cross-agent review: the prior action agent MUST be excluded from review pools. If exclusion empties the pool, fallback to the same agent with a stderr banner `"Cross-agent review fallback"`.

### 5.4 Lease Integration (knots)
- Single-beat sessions MUST create a Knots lease with canonical metadata (`agentName`, `agentType`, `provider`).
- Scene sessions (parent with children) MUST NOT create a lease.
- Lease MUST be terminated when session ends or cwd validation fails.

### 5.5 Outcome Record
Every terminal session iteration MUST produce an outcome record with fields:
- `success`, `exitCode`, `claimedState`, `postExitState`, `beatId`, `rolledBack`, `alternativeAgentAvailable`

---

## 6. Approval System Architecture

### 6.1 Approval Extraction
Transport adapters detect approval requests in their event streams and extract:
- `adapter`, `source`, `requestId`, `supportedActions`, `replyTarget`
- For tool requests: `serverName`, `toolName`, `toolParamsDisplay`, `parameterSummary`, `patterns`, `permissionName`

### 6.2 Approval Registry
- `registerApproval` → DTO with `id`, `notificationKey`, `status`, `createdAt`, `updatedAt`, `repoPath`, `beatId`, `sessionId`, `adapter`, `source`, `toolName`, `replyTarget`, `supportedActions`, `actionable`, `agent`.
- `listApprovals` supports filters: `repoPath`, `activeOnly`, `status`, `updatedSince`. Default order: `updatedAt DESC`, `id ASC`.
- `applyApprovalAction(approvalId, action)` → success updates status; failure returns 502 with upstream reason.
- `detachSession(sessionId, reason)` → flips status to `"manual_required"`, `actionable: false`, nulls responder.
- `attachResponderForSession(sessionId, responder)` → makes orphaned record actionable again.

### 6.3 Approval Escalations
- `notificationKey` and `id` MUST be stable based on logical content (session, beat, tool, patterns), NOT ephemeral ids like `permissionId`.
- `buildApprovalLogicalKey` MUST exclude `permissionId` and `requestId`.
- Two events differing only in `permissionId` MUST collapse to the same escalation identity (coalesced in-place).

### 6.4 Session Buffer Visibility
- Approval banner events MUST be pushed to the session event bus as visible stderr/stdout banners containing `FOOLERY APPROVAL REQUIRED`.
- Claude / Copilot approvals → stdout.
- Codex / Gemini / OpenCode approvals → stderr.

---

## 7. Prompt System Architecture

### 7.1 Execution Boundary Wrappers
Every execution prompt MUST be wrapped in a `FOOLERY EXECUTION BOUNDARY` block that:
- Restricts the agent to exactly one workflow action.
- Names the allowed workflow exit state(s).
- Contains a hard stop instruction after the authorized step (for `take` prompts).

### 7.2 Skill Prompt Contracts
Each workflow step MUST generate a skill prompt containing:
- Step-specific heading (e.g., `# Planning`).
- `bd show <beatId>` command.
- `bd sync` reminder.
- Current workflow state name.
- `## Authority Boundary` section with `"Complete exactly one workflow action, then stop."`.
- Exhaustive transition commands for every legal loom transition.
- MUST NEVER contain `kno claim` instructions.

### 7.3 Scope Refinement Pipeline
```
Beat creation endpoint
  → Enqueue scope-refinement job (beatId, repoPath)
    → Worker dequeues
      → Read beat
      → Spawn refinement agent
      → Parse `<scope_refinement_json>{...}</scope_refinement_json>`
      → Update beat with title, description, acceptance
      → Record completion event
      → [Failure] Re-enqueue with excludeAgentIds
        → Timeout (600s SIGKILL)
```

---

## 8. Orchestration System Architecture

### 8.1 Workflow State Machine

| State Classification | Examples | `isAgentClaimable` | `nextActionOwnerKind` |
|---|---|---|---|
| Queue | `ready_for_implementation` | true | `agent` |
| Active | `implementation`, `planning` | false | `agent` |
| Review | `plan_review`, `shipment_review` | false | `agent` or `human` |
| Terminal | `shipped`, `abandoned` | false | `none` |
| Deferred | `deferred` | false | `none` |

**Invariant:** Semiauto human-owned review steps (`plan_review`) MUST have `requiresHumanAction=true` and `isAgentClaimable=false`.

### 8.2 Valid Next States
- MUST derive exclusively from loom `workflow.transitions`.
- Wildcard transitions (`from: "*"`, to: `deferred`/`abandoned`) MUST be included.
- MUST NOT fabricate rollback transitions to earlier queue states.
- MUST exclude the current state from results.
- For stuck/rolled-back beats, compute from raw kno state, NOT display state.

### 8.3 Beat Hierarchy
- `buildHierarchy` assigns `_depth = 0` to top-level beats, incrementing per generation.
- Children MUST appear immediately after their parent in flattened output.
- Children MUST NEVER escape their parent subtree during sorting.
- Orphans (missing parent) MUST be promoted to top-level.

### 8.4 Cycle Safety
- Self-referencing parent (`a -> a`) MUST result in exclusion.
- Multi-node cycles (`a -> b -> a`) MUST result in all cyclic beats excluded.
- Traversal MUST guard against infinite loops using a visited set.

### 8.5 Sorting Contracts
- Priority first (lower number first).
- Then state rank: queue < action < terminal < blocked < deferred.
- Then title alphabetically, then ID tiebreaker.
- Natural numeric compare for dotted IDs (e.g., `mqv.2.10` > `mqv.2.2`).

### 8.6 Query Cache
- Stable keys: `["beats"]`, `["setlist-plan"]`, `["setlist-plan-beat"]`.
- Invalidation MUST refetch all three with `refetchType: "active"`.

---

## 9. Error Taxonomy & Mapping

### 9.1 Backend Error Codes
| Code | Retryable | HTTP |
|---|---|---|
| `NOT_FOUND` | false | 404 |
| `ALREADY_EXISTS` | false | 409 |
| `INVALID_INPUT` | false | 400 |
| `LOCKED` | true | 503 |
| `TIMEOUT` | true | 503 |
| `UNAVAILABLE` | true | 503 |
| `PERMISSION_DENIED` | false | 403 |
| `INTERNAL` | false | 500 |
| `CONFLICT` | false | 409 |
| `RATE_LIMITED` | true | 429 |

### 9.2 Suppressibility
- `LOCKED`, `TIMEOUT`, `UNAVAILABLE`, `RATE_LIMITED` → suppressible (eligible for degraded-mode caching).
- All others → non-suppressible.

### 9.3 Degraded Mode
- First suppressible error: return cached data, enter failure state.
- Within suppression window (e.g., 2 min): continue returning cached data.
- After window expires: return degraded error message (e.g., `"Backend is temporarily unavailable; displaying cached data."`).
- TTL eviction (e.g., 10 min): next suppressible error returns raw error and clears failure state.

---

## 10. Data Flow Diagrams

### 10.1 Take Flow (Full State Machine)
```
TakeLoopContext (per-goroutine mutable state):
  beatId, beat, repoPath, agent, agentInfo
  claimsPerQueueType: map[queueType]int
  lastAgentPerQueueType: map[queueType]string
  failedAgentsPerQueueType: map[queueType]set[string]
  followUpAttempts: {count int, lastState string}
  sessionAborted: bool

User triggers take on beat
  → prepareSessionTargets(beatId, repoPath)
    → Backend.get(beatId) → resolve workflow → resolve children
    → rollbackAgentOwnedActionStateToQueue on each target
  → resolveDispatchAgent(ctx, pool settings)
    → derivePoolKey(workflow, state) → settings.pools[poolKey]
    → selectFromPool(pool, agents, excludeAgentIds)
    → [FAIL] DispatchFailureError("FOOLERY DISPATCH FAILURE")
  → createSession entry (chan TerminalEvent, cap 5000)
  → create Knots lease (single-beat sessions only)
  → validateCwd → [FAIL] mark error, release lease, delay cleanup 5min
  → buildNextTakePrompt(ctx, excludeAgentId?)
    → Backend.get(beatId) → resolve workflow
    → [terminal] → handleTerminalState, markBeatShipped, return null
    → [active && agent-owned] → rollbackStepFailure → find queue state → rollbackBeatState
    → [NOT queue || NOT agent-owned] → handleNotAgentOwned, return null
    → derivePoolKey → selectStepAgent (with computeExclusions)
    → claimCount > maxClaims → handleMaxClaims, return null
    → buildTakePrompt via BackendPort → finalizeClaim
  → spawnTakeChild(ctx, takePrompt)
    → applyEffectiveAgent → reset followUp counter
    → resolveDialect(command) → resolveTakeSceneCapabilities
    → createTakeRuntimeBundle (runtime, httpRefs, jsonrpcSession, acpSession)
    → spawn(agent.command, args, {env: approvalBridgeEnv, detached:true})
    → wire stdout → NDJSON scanner → event normalizer → chan TerminalEvent
    → wire stderr → verbatim → chan TerminalEvent
    → JSON-RPC handshake (if codex interactive) or ACP handshake (if gemini)
    → logAndSendTakePrompt → runtime.sendUserTurn(stdin, prompt)
    → Watchdog starts (10 min default, resets on any stdout)
  → Child exits → wireTakeChildClose
    → captureChildCloseDiagnostics → shouldTreatTurnEndedSignalAsClean → effectiveCode
    → handleTakeIterationClose(ctx, effectiveCode, iterationAgent, claimedState)
      → [Aborted] finish immediately
      → captureBeatSnapshot (fire-and-forget, never throws)
      → Backend.get(beatId) → post-exit state
      → classifyIterationSuccess(exitCode, claimedState, postExitState, workflow)
      → build outcome record
      → [exitCode != 0] → handleErrorExit:
          → record failed agent → enforceQueueTerminalInvariant (rollback)
          → write outcome + audit
          → buildNextTakePrompt with error agent exclusion
          → [next agent available] spawn new child, increment iteration
          → [no alternative] finish session
      → [exitCode == 0] → handleSuccessExit:
          → write outcome + audit
          → buildNextTakePrompt (no error exclusions)
          → [next take available] continue
          → [done] enforce invariant, finish session
  → recordSessionFinishLifecycle
  → recordTakeLoopLifecycle (stdout_observed, turn_ended, child_close, loop_stop)
  → releaseKnotsLease (sync.Once guarded)
  → cleanupTerminalSessionResources (mark approvals manual_required)
```

### 10.2 Follow-Up Loop (post-turn-end continuation)
```
handleTakeLoopTurnEnded(ctx, runtime, child)
  → Backend.get(beatId) → fetch current state
  → [queue/terminal] reset followUp counter, return false
  → recordFollowUpProgress (reset count if state changed, increment)
  → [count > MAX_FOLLOW_UPS_PER_STATE (5)] emit cap banner ("follow-up cap reached"), return false
  → evaluateLeaseHealth(leaseId, repoPath)
    → [lease_ready || lease_active] → healthy
    → [lease_terminated] → refuseFollowUpForDeadLease, return false
  → sendFollowUpPrompt:
    → buildTakeLoopFollowUpPrompt(beatId, state)
    → runtime.sendUserTurn → return true/false
```

### 10.3 Approval Flow
```
Agent stdout/stderr stream
  → Approval extractor (per dialect):
      Claude: AskUserQuestion tool_use → auto-answer confirmation
      Codex: mcpServer/elicitation/request, item/commandExecution/requestApproval
      Copilot: user_input.requested → auto-response
      Gemini: session/request_permission → auto-reply allow_once
      OpenCode: permission.asked, permission.updated (BFS nested search)
    → Extracted fields: adapter, source, requestId, nativeSessionId, replyTarget
    → Banner: formatApprovalRequestBanner (FOOLERY APPROVAL REQUIRED marker)
    → Push banner to session event bus (Claude/Copilot→stdout, others→stderr)
    → terminalApprovalSession.recordPendingApproval(entry, request)
    → approvalRegistry.registerApproval(dto)
      → notificationKey = buildApprovalLogicalKey (excludes ephemeral permissionId/requestId)
      → Store upsert (coalesce by notificationKey)
        → UI notification (approval bell, dedupe by notificationKey)
          → User selects action
            → approvalRegistry.applyApprovalAction(id, action)
              → [approve/always_approve/reject] Responder route:
                  Codex JSON-RPC: respondToCodexApproval → accept/decline/acceptForSession
                  OpenCode HTTP: POST /session/:id/permissions/:pid {response:"once"|"always"|"reject"}
                  Claude bridge: always ok (UI-only action)
                  Copilot bridge: not connected (direct auto-response on stream)
                → update store status, clear failureReason
              → [failure] markApprovalFailed(id, reason), explainApprovalFailureReason
```

### 10.4 Scope Refinement / Stale Grooming Flow
```
Beat creation endpoint
  → Enqueue scope refinement job (beatId, repoPath)
    → Worker polls FIFO queue
      → Read beat → spawn refinement agent (600s timeout)
      → Parse <scope_refinement_json> tagged JSON
      → Backend.update(beatId, {title, description, acceptance})
      → Record completion event
      → [Failure] Re-enqueue with excludeAgentIds
        → [All agents exhausted] Fail gracefully, no re-enqueue

Stale beat grooming:
  → GET /api/beats/stale-grooming → filter beats updated > 7 days ago
  → POST /api/beats/stale-grooming/reviews → enqueue selected beat IDs
  → Worker goroutine drains FIFO queue
    → resolveStaleBeatGroomingAgent (dispatchMode: advanced→pool, basic→action)
    → buildGroomingPrompt → spawn agent
    → Parse <stale_beat_grooming_json> tagged output
    → [still_do] append handoff capsule, update lastUpdated
    → [reshape] apply title/description/acceptance via backend.update
    → [drop] markTerminal(abandoned) with reason capsule
    → [parse failure] no mutation, record failure
    → Update worker health (activeJobs, completions, failures)
```

---

## 11. File Organization (Go Port)

```
internal/
  backend/
    port.go              # BackendPort interface
    errors.go            # BackendError, error codes, classification
    capabilities.go      # Capability presets, assertCapability
    factory.go           # Auto-routing backend factory
    bdcli.go             # bd CLI adapter, serialization, retries
    knots.go             # KnotsBackend adapter
    registry.go          # Repo registry, memory manager inference
    dto.go               # RawBead <-> Beat normalization
    state_machine.go     # claimBeat, nextBeat
    degraded.go          # Error suppression cache
    worker.go            # Local worker (poll, take prep)
  dispatch/
    pool.go              # selectFromPool, selectFromPoolStrict
    resolver.go          # resolveDispatchAgent, fail loud
    identity.go          # toCanonicalLeaseIdentity, formatAgentDisplayLabel
    swap.go              # swapPoolAgent, swapActionsAgent, swapPoolsAgent
    slugs.go             # wave slug allocation, normalization
  session/
    runtime.go           # Session runtime, stdout/stderr wiring
    lifecycle.go         # onTurnEnded, prompt delivery hooks
    watchdog.go          # Inactivity timeout, SIGTERM/SIGKILL
    diagnostics.go       # captureChildCloseDiagnostics, formatDiagnosticsForLog
    connection.go        # SSE connection manager
    terminal_store.go    # Terminal status, panel, rehydration
    notification.go      # Notification store, deduplication
  transport/
    claude.go            # Claude stdio adapter
    codex.go             # Codex JSON-RPC session
    copilot.go           # Copilot interactive session
    gemini.go            # Gemini ACP session
    opencode.go          # OpenCode HTTP session
    formatter.go         # Event formatting (detail vs. main stream)
    ndjson.go            # NDJSON stream parser
  terminal/
    manager.go           # createSession, abort, terminate, kill
    take_loop.go         # handleTakeLoopTurnEnded, follow-up cap
    retry.go             # Error retry, agent rotation, rollback
    outcome.go           # Outcome classification, record fields
    dispatch.go          # runDispatch, cross-agent review fallback
    forensics.go         # Snapshot capture, failure classification
  orchestration/
    workflow.go          # Step resolution, transition targets
    state.go             # deriveWorkflowRuntimeState, validNextStates
    hierarchy.go         # buildHierarchy, cycle detection
    sorting.go           # Priority, state rank, natural compare
    overview.go          # Beat state overview, column definitions
    plans.go             # Plan generation, completion
    grooming.go          # Stale beat grooming, worker, timeout
  prompt/
    guardrails.go        # Execution boundary wrappers, authority lines
    skills.go            # Skill prompt generation per workflow step
    refinement.go        # Scope refinement worker, queue, status
    history.go           # History debug prompt, response visibility
    tools.go             # Tool input summarization
  approvals/
    extract.go           # Multi-adapter approval extraction
    registry.go          # Approval DTO, list, apply, detach
    escalation.go        # Logical key, coalescence, failure explanation
    banner.go            # formatApprovalRequestBanner
  api/
    routes.go            # HTTP route definitions
    beats.go             # Beat routes, mark terminal, rollback
    plans.go             # Plan routes
    approvals.go         # Approval routes
    app.go               # App update route
    streams.go           # SSE session streaming
  config/
    foolery.go           # YAML config loading
    agent_settings.go    # Agent pool settings
```

---

## 12. Migration Gaps & Open Questions

1. **NDJSON abort semantics** — Map TS `AbortSignal` to Go `context.Context` cancellation at parser loop boundaries.
2. **Turn-ended deduplication** — Per-session atomic boolean or mutex to guard against duplicate `session_idle` triggers.
3. **Delta key fallback** — Codex JSON-RPC deltas prefer `delta` field, fall back to `text`. Empty both → null event.
4. **Hermetic session tests** — All transport sessions MUST use injected `io.Reader`/`io.Writer` fakes, never real `os/exec`.
5. **Backend error mapping** — Pure exhaustive `switch`; unknown codes → 500, never zero.
6. **OpenCode model selection** — Lacks provider slash MUST throw loud error, not return empty string.
7. **Token usage deduplication** — Log event once per consuming beat, not per beat in session scope.
8. **Registry file storage** — TS uses `localStorage` / `~/.config`; Go port MUST use a file path derived from `$HOME` and enforce `0o600`.
9. **Vue 3 reactivity proxy awareness** — Not applicable to Go backend, but any shared mutable state (caches, registries) MUST use `sync.Map` or `map + RWMutex`.

---

*This document is a living specification. Update it whenever a domain spec changes significantly.*
