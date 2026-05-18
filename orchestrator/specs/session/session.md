# Session Behavioral Contract Specification — Go Port
> Authoritative behavioral contract for the Go backend. This document states WHAT the Go backend must do. Historical provenance: contracts originally inferred from the TypeScript Foolery test suite.

---

## 1. Agent Session Runtime

### 1.1 Capabilities Resolution
- **Dialect capabilities must be explicit and deterministic.** Each dialect resolves to a capability set containing `interactive`, `promptTransport`, `supportsFollowUp`, `supportsAskUserAutoResponse`, `stdinDrainPolicy`, and `resultDetection`.
  - Claude: interactive, `stdin-stream-json`, supports follow-up, supports AskUser auto-response, drain policy `close-after-result`, result detection `type-result`.
  - Codex (one-shot): non-interactive, `cli-arg`, no follow-up, no AskUser, drain `never-opened`, detection `type-result`.
  - Copilot (one-shot): non-interactive, `cli-arg`, no follow-up, supports AskUser, detection `type-result`.
  - OpenCode (one-shot): non-interactive, no AskUser.
  - Gemini (one-shot): non-interactive, `cli-arg`, detection `status-result`, drain `never-opened`.
  - Interactive overrides exist for codex (`jsonrpc-stdio`), copilot (`stdin-stream-json`), opencode (`http-server`), and gemini (`acp-stdio`). All interactive variants support follow-up.
  - `supportsInteractive` returns true only for codex, copilot, opencode, gemini; false for claude.
  - Base capabilities must never include transport-unrelated fields like `watchdogTimeoutMs`.
  [source: foolery/src/lib/__tests__/agent-session-capabilities.test.ts:10]

- **Take/scene dispatch must assert interactive capabilities.** If a one-shot capability object is passed to take/scene dispatch, the system must throw a failure marked with `TERMINAL_DISPATCH_FAILURE_MARKER`.
  [source: foolery/src/lib/__tests__/terminal-dispatch-capabilities.test.ts:50]

### 1.2 Runtime Initial State
- **Interactive dialects start with open stdin; one-shot starts with closed stdin.** The runtime state must reflect `stdinClosed = false` for interactive and `true` for one-shot.
  [source: foolery/src/lib/__tests__/agent-session-runtime.test.ts:97]

- **`sendUserTurn` returns true for interactive sessions and false for one-shot.** When interactive, it writes a JSON message to the child’s stdin. When one-shot, it returns false without writing.
  [source: foolery/src/lib/__tests__/agent-session-runtime.test.ts:114]

- **`closeInput` must end the child’s stdin and mark `stdinClosed`.** It must be idempotent: repeated calls must not call `end()` more than once.
  [source: foolery/src/lib/__tests__/agent-session-runtime.test.ts:146]

### 1.3 Scheduled Input Close
- **Input close may be scheduled with a grace period (default 2s).** A timer must close stdin after the grace period unless `cancelInputClose` is called first.
  [source: foolery/src/lib/__tests__/agent-session-runtime.test.ts:172]

- **`dispose` must cancel any pending input-close timer and mark stdin closed.**
  [source: foolery/src/lib/__tests__/agent-session-runtime.test.ts:201]

### 1.4 Stdout / Stderr Wiring
- **Stdout data must be line-buffered and parsed as JSON.** Partial JSON lines accumulated across multiple `data` chunks must be flushed on newline or via an explicit `flushLineBuffer` call.
  [source: foolery/src/lib/__tests__/agent-session-runtime.test.ts:254]

- **Non-JSON stdout must be pushed as raw `stdout` events.**
  [source: foolery/src/lib/__tests__/agent-session-runtime.test.ts:307]

- **Stderr must be pushed as `stderr` events verbatim.**
  [source: foolery/src/lib/__tests__/agent-session-runtime.test.ts:307]

- **`lastStdoutAt` must be updated on every stdout chunk.** It starts null, becomes an integer timestamp after the first chunk, and advances on subsequent chunks.
  [source: foolery/src/lib/__tests__/agent-session-close-diagnostics.test.ts:75]

### 1.5 Event Normalization & Result Detection
- **Claude:** `type: "result"` events set `resultObserved = true` and `exitReason = "turn_ended"`. `AskUserQuestion` tool_use must be auto-answered by writing a confirmation back to stdin and recording the tool use ID in `autoAnsweredToolUseIds`.
  [source: foolery/src/lib/__tests__/agent-session-runtime.test.ts:219]

- **Codex (one-shot):** `item.completed` with `agent_message` followed by `turn.completed` must set `resultObserved = true`. `turn.completed` alone must not set it without a preceding message. `AskUser` must NOT be auto-answered.
  [source: foolery/src/lib/__tests__/agent-session-runtime.test.ts:282]

- **Interactive Codex (JSON-RPC):** After handshake, `turn/completed` param events must set `resultObserved = true` and `exitReason = "turn_ended"`. MCP noise (e.g., `mcpServer/startupStatus/updated`) must be ignored and must not affect state.
  [source: foolery/src/lib/__tests__/agent-session-runtime.test.ts:338]

- **Gemini:** `type: "result"` with `status: "success"` sets `resultObserved = true`. Non-success `status` sets `resultObserved = true` and `lastNormalizedEvent.is_error = true`.
  [source: foolery/src/lib/__tests__/agent-session-runtime-lifecycle.test.ts:76]

- **Copilot interactive:** `session.task_complete` sets `resultObserved = true` and `exitReason = "turn_ended"`. `session.error` sets `resultObserved = true` and `is_error = true`.
  [source: foolery/src/lib/__tests__/copilot-interactive-session.test.ts:90]

- **OpenCode interactive:** `session_idle` is the authoritative turn boundary and must set `resultObserved = true`. `step_finish` with `reason: "stop"` is a per-message boundary and must NOT set `resultObserved`. `step_finish` with `reason: "error"` must set `resultObserved = true` and `is_error = true`.
  [source: foolery/src/lib/__tests__/opencode-interactive-session.test.ts:209]

- **Turn failure tracking:** A `turn/completed` with `status = "failed"` must record `lastTurnError` containing the event type and error payload.
  [source: foolery/src/lib/__tests__/agent-session-runtime.test.ts:453]

### 1.6 Lifecycle Callbacks
- **`onTurnEnded` callback controls stdin close.** Returning `true` prevents scheduled close; returning `false` allows the scheduled close to proceed after the grace period.
  [source: foolery/src/lib/__tests__/agent-session-runtime-lifecycle.test.ts:139]

- **Prompt delivery hooks must fire.** `sendUserTurn` must emit:
  - `prompt_delivery_attempted` (transport = `"stdio"`)
  - `prompt_delivery_succeeded` (transport = `"stdio"`)
  [source: foolery/src/lib/__tests__/agent-session-runtime-lifecycle.test.ts:185]

- **Observation events must fire on result lines:** `stdout_observed`, `response_logged`, `normalized_event_observed`, `turn_ended`.
  [source: foolery/src/lib/__tests__/agent-session-runtime-lifecycle.test.ts:209]

- **Interactive Codex prompt delivery delegates to jsonrpc `startTurn`.** After handshake, `sendUserTurn` must produce a JSON-RPC message with `method: "turn/start"`.
  [source: foolery/src/lib/__tests__/agent-session-runtime.test.ts:346]

### 1.7 Watchdog
- **Watchdog must terminate the child after a configured inactivity timeout.** Default for interactive sessions is 10 minutes (derived from `interactiveSessionTimeoutMinutesToMs(10)`).
  [source: foolery/src/lib/__tests__/opencode-interactive-session.test.ts:313]

- **Watchdog must reset on any stdout event activity.** After activity, the full timeout window must start again.
  [source: foolery/src/lib/__tests__/agent-session-runtime-lifecycle.test.ts:355]

- **Watchdog must fire even after `resultObserved` is true.** A turn ending does not prove the child process has exited. Silence for the full timeout after a result event must still trigger termination. This is a canonical liveness rule.
  [source: foolery/src/lib/__tests__/agent-session-runtime-lifecycle.test.ts:421]

- **Watchdog must be cleared by `dispose`.** After dispose, advancing timers must not trigger timeout.
  [source: foolery/src/lib/__tests__/agent-session-runtime-lifecycle.test.ts:406]

- **Watchdog must be a no-op when `watchdogTimeoutMs` is null.**
  [source: foolery/src/lib/__tests__/agent-session-runtime-lifecycle.test.ts:395]

- **Before sending SIGTERM, the system must warn via `console.warn` containing `[terminal-manager] [watchdog]`, `timeout_fired`, `pid`, `timeoutMs`, `reason=timeout`, and `lastEventType`.** The warn call must happen before `process.kill`.
  [source: foolery/src/lib/__tests__/agent-session-runtime-watchdog-fired.test.ts:104]

- **`terminateProcessGroup` must send SIGTERM, then SIGKILL after a delay.** If `process.kill(-pid, SIGTERM)` throws (e.g., ESRCH), it must fall back to `child.kill(SIGTERM)`, then SIGKILL. Entry and SIGKILL logs must include the exact tokens `signal=`, `exitReason=`, `msSinceLastStdout=`, `lastEventType=`, `reason=`, `pid=`, `delayMs=`.
  [source: foolery/src/lib/__tests__/agent-session-runtime-lifecycle.test.ts:464]

### 1.8 Close Diagnostics
- **`captureChildCloseDiagnostics` must populate:** `exitReason` (default `"normal"` if null), `msSinceLastStdout`, `lastEventType`. It must be safe when state is null.
  [source: foolery/src/lib/__tests__/agent-session-close-diagnostics.test.ts:120]

- **`formatDiagnosticsForLog` must include the exact tokens:** `signal=`, `exitReason=`, `msSinceLastStdout=`, `lastEventType=`, `turnError=`. Null values must print as `null` cleanly.
  [source: foolery/src/lib/__tests__/agent-session-close-diagnostics.test.ts:179]

- **`shouldTreatTurnEndedSignalAsClean` must return true only when:**
  - Signal is null
  - `exitReason` is `"turn_ended"`
  - `lastEventType` is `"result"`
  - `turnError` is null

  It must return false if the exit code is non-zero, or if `lastEventType` is `"turn.failed"`, or `turnError` is non-null.
  [source: foolery/src/lib/__tests__/agent-session-close-diagnostics.test.ts:272]

- **Enriched child_close lifecycle payload must contain:** `childSignal`, `childExitCode`, `exitReason`, `msSinceLastStdout` (as number), `lastEventType`. The matching console.log line must contain `signal=`, `msSinceLastStdout=`, `exitReason=`, `lastEventType=`.
  [source: foolery/src/lib/__tests__/terminal-manager-watchdog-e2e.test.ts:375]
  [source: foolery/src/lib/__tests__/terminal-manager-child-close-diagnostics.test.ts:323]

---

## 2. Interactive Session Adapters

### 2.1 Codex JSON-RPC Session
- **Handshake sends `initialize` then `thread/start`.** `thread/start` params must include `approvalPolicy` (default `"never"`, overridable to `"untrusted"`) and optional `sandbox` (e.g., `"read-only"`).
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:63]

- **Session becomes ready only after `thread/start` response.** `ready` transitions to true and `threadId` is populated.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:117]

- **`startTurn` sends `turn/start` via JSON-RPC with threadId, approvalPolicy, and input text.** If the session is not yet ready, the turn must be queued and flushed automatically after handshake.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:147]

- **Prompt delivery hooks fire for deferred turns:** `onDeferred`, `onAttempted`, `onSucceeded`. `onFailed` must not fire when delivery succeeds.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:205]

- **`interruptTurn` sends `turn/interrupt`.** Returns false if no turn is in progress.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:236]

- **Approval replies route by request ID.** `respondToApproval` must write a JSON-RPC response with the exact request id, action `accept`/`decline`/decision based on the action type.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:267]

- **Notification translation:**
  - `turn/started` -> `{ type: "turn.started" }`
  - `turn/completed` with status `completed` -> `{ type: "turn.completed" }`
  - `turn/completed` with status `failed` -> `{ type: "turn.failed", error }`
  - `item/started` commandExecution -> `{ type: "item.started", item: { type: "command_execution", ... } }`
  - `item/completed` agentMessage -> `{ type: "item.completed", item: { type: "agent_message", text } }`
  - `item/completed` commandExecution -> `{ type: "item.completed", item: { type: "command_execution", aggregated_output } }`
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:375]

- **Filtering:** `mcpServer/startupStatus/updated` and `thread/started` must be filtered (return null). JSON-RPC error responses must be logged via `console.error`.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:508]

- **Edge case:** `startTurn` returns false when `stdin` is destroyed.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:549]

### 2.2 OpenCode HTTP Session
- **Server URL discovery:** `processStdoutLine` must extract the HTTP URL from a line matching `opencode server listening on <url>`. Non-URL lines must be ignored.
  [source: foolery/src/lib/__tests__/opencode-interactive-session.test.ts:143]

- **Runtime routes stdout to the httpSession.** Non-URL stdout must still be pushed as terminal events.
  [source: foolery/src/lib/__tests__/opencode-interactive-session.test.ts:169]

- **`startTurn` queues the turn before server URL is discovered.** After discovery, queued turns must be dispatched automatically.
  [source: foolery/src/lib/__tests__/opencode-interactive-session.test.ts:493]

- **Model selection must be sent with message turns.** The message body must include `model.providerID` and `model.modelID` parsed from the configured model string.
  [source: foolery/src/lib/__tests__/opencode-http-session-approvals.test.ts:146]

- **Approval forwarding:** Forward `permission.asked` response parts and stream events to the terminal event stream. Wrapped events (nested under `events[].part`) must also be forwarded.
  [source: foolery/src/lib/__tests__/opencode-http-session-approvals.test.ts:177]

- **Tool_use dedup:** Empty-input `tool_use` events must be skipped; the first populated update must be emitted.
  [source: foolery/src/lib/__tests__/opencode-http-session-approvals.test.ts:295]

- **Retry backoff:** Message delivery failures must retry with delays of 8s, 16s, 32s. Retry warning messages must contain the delay.
  [source: foolery/src/lib/__tests__/opencode-http-session-retry.test.ts:74]

- **Retry safety:** If SSE shows turn activity before the message fetch fails, the system must not retry; it must log that it is waiting for `session.idle` from the SSE stream.
  [source: foolery/src/lib/__tests__/opencode-http-session-retry.test.ts:146]

- **Disposal:**
  - `interruptTurn` must dispose the OpenCode server instance (`POST /instance/dispose`).
  - It must not abort a turn after SSE `session_idle`; the dispose must happen cleanly.
  - `interruptTurn` must clear any pending queued turn before disposal.
  [source: foolery/src/lib/__tests__/opencode-http-session-disposal.test.ts:108]

### 2.3 Gemini ACP Session
- **Handshake sends `initialize` then `session/new`.** `session/new` params must include `cwd`. The session becomes ready only after the `session/new` response, and `sessionId` is populated.
  [source: foolery/src/lib/__tests__/gemini-acp-session.test.ts:35]

- **`startTurn` queues prompt until session ready.** After ready, it sends `session/prompt` with the sessionId and prompt text parts.
  [source: foolery/src/lib/__tests__/gemini-acp-session.test.ts:74]

- **`interruptTurn` sends `session/cancel` as a notification** (no id) with the current `sessionId`. Returns false if there is no session.
  [source: foolery/src/lib/__tests__/gemini-acp-session.test.ts:107]

- **Event translation:**
  - `session/update` with `agent_message_chunk` -> `{ type: "message", role: "assistant", content, delta: true }`
  - `session/update` with `tool_call` -> `{ type: "message", ... }`
  - Prompt response with `stopReason: "end_turn"` -> `{ type: "result", status: "success" }`
  - Error response -> `{ type: "result", status: "error" }`
  - Unknown notifications must return null.
  [source: foolery/src/lib/__tests__/gemini-acp-session.test.ts:147]

- **Interactive Gemini via runtime:** After ACP handshake and `sendUserTurn`, `agent_message_chunk` must not set `resultObserved`; `end_turn` response must. Errors must set `is_error = true`.
  [source: foolery/src/lib/__tests__/gemini-interactive-session.test.ts:106]

### 2.4 Copilot Interactive Session
- **Completion:** `session.task_complete` sets `resultObserved` and `exitReason = "turn_ended"`. `session.error` sets `resultObserved` and `is_error = true`.
  [source: foolery/src/lib/__tests__/copilot-interactive-session.test.ts:90]

- **Follow-up:** `sendUserTurn` must write a JSON message of type `user_message` with the prompt text. `resultObserved` must reset after a new turn starts.
  [source: foolery/src/lib/__tests__/copilot-interactive-session.test.ts:246]

- **AskUser auto-response:** `user_input.requested` must trigger an auto-response written back to stdin as a `user_message` containing `"auto-response"`.
  [source: foolery/src/lib/__tests__/copilot-interactive-session.test.ts:356]

---

## 3. Terminal Manager & Take Loop

### 3.0 SessionEntry — In-Memory Session Struct (Go Port Pattern)
Each session lives in a goroutine-local `SessionEntry` struct with these key fields [source: `foolery/src/lib/terminal-manager-types.ts:18-43`]:

| TS Field | Go Equivalent | Notes |
|---|---|---|
| `session: TerminalSession` | Frozen status struct | Immutable snapshot, copy on update |
| `process: ChildProcess` | `*exec.Cmd` | One active child per iteration, replaced on retry |
| `abort?: () => void` | `context.CancelFunc` | Single goroutine owns the context |
| `releaseKnotsLease?: (reason, outcome?, data?)` | `*sync.Once` + closure | Caller replaces closure on lease rotation |
| `emitter: EventEmitter` | `chan TerminalEvent` | Buffered channel, cap 5000 |
| `buffer: TerminalEvent[]` | `[]TerminalEvent` (append-only) | Replay buffer for SSE reconnection |
| `takeLoopLifecycle: Map<number, Trace>` | `map[int]*TakeLoopIterationTrace` | Goroutine-local, no mutex needed |
| `pendingApprovals: Map<string, PendingRecord>` | `map[string]*PendingApprovalRecord` | Shared with approval goroutine — needs `sync.RWMutex` |
| `approvalResponder?: (record, action) => result` | `func(*PendingApprovalRecord, string) (*Result, error)` | Set once, thread-safe after creation |

**Go architectural note:** `claimsPerQueueType`, `lastAgentPerQueueType`, `failedAgentsPerQueueType`, and `followUpAttempts` live in the `TakeLoopContext` which is owned by a single goroutine — no mutex needed for these fields. Only `pendingApprovals` and `session.status` are shared between the take-loop goroutine and the approval/SSE goroutines.

### 3.1 Session Creation & Dispatch
- **`createSession` must spawn an agent child process and begin a take loop.** It must:
  - Resolve the agent dispatch pool based on workflow state.
  - Build a take prompt via `buildTakePrompt`.
  - Log the prompt with source `"initial"` or `"take_2"`.
  - Include `"FOOLERY EXECUTION BOUNDARY:"` in logged prompts.
  - Wrap backend prompts with execution boundaries during take-loop iterations.
  - Log lifecycle events: `prompt_built`, `prompt_send_attempted`, `prompt_delivery_deferred`, `child_close`.
  [source: foolery/src/lib/__tests__/terminal-manager-knots-next-guard-take-loop.test.ts:241]

- **Agent label must appear in stdout `Claimed` and `TAKE 2` log lines**, e.g., `[agent: Codex]`.
  [source: foolery/src/lib/__tests__/terminal-manager-knots-next-guard-take-loop.test.ts:407]

### 3.2 Knots Lease Integration
- **Single-beat sessions must create a Knots lease with canonical agent metadata** (`agentName`, `agentType`, `provider`). Scene sessions (parent with children) must NOT create a lease.
  [source: foolery/src/lib/__tests__/terminal-manager-knots-lease.test.ts:186]

- **Lease must be terminated when the session ends or when cwd validation fails before launch.**
  [source: foolery/src/lib/__tests__/terminal-manager-knots-lease.test.ts:251]

- **Canonical lease metadata must include:** `agentName` (derived from display command label, not raw label field), `agentType`, `provider`.
  [source: foolery/src/lib/__tests__/terminal-manager-knots-lease.test.ts:310]

### 3.3 Abort & Signal
- **`abortSession` must:**
  - Preserve `"aborted"` status even if the child later exits with code 0.
  - Prevent the take-loop from spawning the next iteration.
  - Be idempotent across repeated calls.
  - Log `"aborted"` as the terminal status in `logEnd`.
  [source: foolery/src/lib/__tests__/terminal-manager-abort.test.ts:206]

- **Abort during an active take-loop iteration must stop the second child and prevent a third child.**
  [source: foolery/src/lib/__tests__/terminal-manager-abort.test.ts:325]

- **`terminateSession` must send `SIGTERM` to the process group (`-pid`) and mark status `"aborted"`.** `killSession` must send `SIGKILL` to the process group and mark `"aborted"`.
  [source: foolery/src/lib/__tests__/terminal-manager-signal.test.ts:244]

- **Signal commands must return `not_found` for unknown sessions and `already_exited` for completed sessions without calling `process.kill`.**
  [source: foolery/src/lib/__tests__/terminal-manager-signal.test.ts:290]

- **Abort must force-kill process group descendants even if the leader exits first:** `SIGTERM` at t=0, then `SIGKILL` after a delay.
  [source: foolery/src/lib/__tests__/terminal-manager-abort.test.ts:404]

### 3.4 Step Failure & Rollback
- **Non-zero child exit must trigger `enforceQueueTerminalInvariant` with `rollbackBeatState`.** The beat must be rolled back from its active state to the prior queue state.
  [source: foolery/src/lib/__tests__/terminal-manager-step-failure-rollback.test.ts:236]

- **Take-loop step failure (zero exit but active state) must also trigger rollback.**
  [source: foolery/src/lib/__tests__/terminal-manager-step-failure-rollback.test.ts:272]

- **Per-queue-type claim limit exceeded must trigger rollback.** After rollback, the beat must be in the queue state, not stuck active.
  [source: foolery/src/lib/__tests__/terminal-manager-step-failure-rollback.test.ts:322]

- **Concurrent abort during rollback must be handled gracefully.** The session must finish with `"aborted"` status and no extra children spawned.
  [source: foolery/src/lib/__tests__/terminal-manager-step-failure-rollback.test.ts:418]

### 3.5 Queue Claims & Agent Rotation
- **Per-queue-type claim limit (`maxClaimsPerQueueType`) must stop the take loop.** After the limit is exceeded, `logEnd` must be called and the loop must not spawn another child.
  [source: foolery/src/lib/__tests__/terminal-manager-queue-claims.test.ts:326]

- **Lease audit events must be emitted:** At least one `claim` event and one `success`/`fail` event per iteration, carrying `beatId`, `queueType`, `agent`, and `durationMs`.
  [source: foolery/src/lib/__tests__/terminal-manager-queue-claims.test.ts:371]

- **Agent rotation must use `lastAgentPerQueueType` as a soft exclusion** to rotate agents between iterations.
  [source: foolery/src/lib/__tests__/terminal-manager-queue-claims.test.ts:444]

- **Error retry in basic mode must rotate to a different pooled agent.** The console log must indicate `selected="agent-b"`.
  [source: foolery/src/lib/__tests__/terminal-manager-queue-claims.test.ts:486]

- **Advanced mode with three agents must cycle through the full pool before stopping after repeated failures.**
  [source: foolery/src/lib/__tests__/terminal-manager-queue-claims.test.ts:534]

### 3.6 Error Retry Behavior
- **Non-zero exit retries with a different agent when alternatives exist.** The first agent is excluded. `createLease` count must reflect new attempts. Outcome record must indicate `success = false`, `exitCode`, `beatId`.
  [source: foolery/src/lib/__tests__/terminal-manager-error-retry-retry.test.ts:268]

- **When no alternative agent exists, retry stops.** `logEnd` must be called with the original exit code. Outcome record must indicate `alternativeAgentAvailable = false`, `rolledBack = false`. `rollbackBeatState` must NOT be called because there is no retry.
  [source: foolery/src/lib/__tests__/terminal-manager-error-retry-retry.test.ts:313]

- **Rollback must happen before retry when the beat is in an active state.** After rollback, a new child with a different agent may be spawned.
  [source: foolery/src/lib/__tests__/terminal-manager-error-retry-retry.test.ts:360]

- **Take-loop child error (non-zero exit during loop) must also retry with a different agent.**
  [source: foolery/src/lib/__tests__/terminal-manager-error-retry-retry.test.ts:443]

### 3.7 Outcome Classification
- **Success = true** when the beat advances to the next queue state after the child exits.
  [source: foolery/src/lib/__tests__/terminal-manager-error-retry-outcome.test.ts:232]

- **Success = true** when the beat moves to a prior queue state (review rejection). This is still considered successful agent work because the state changed.
  [source: foolery/src/lib/__tests__/terminal-manager-error-retry-outcome.test.ts:345]

- **Success = false** when:
  - The beat stays at the same queue state.
  - The beat reaches a terminal state directly (not via the expected next queue state).
  - The beat is stuck in an active state (e.g., `implementation`) after exit.
  [source: foolery/src/lib/__tests__/terminal-manager-error-retry-outcome.test.ts:434]

- **`beat_state_observed` event must be emitted in the session buffer** after post-exit state fetch, containing `beatId`, `state`, `reason: "post_exit_state_observed"`.
  [source: foolery/src/lib/__tests__/terminal-manager-error-retry-outcome.test.ts:288]

- **Outcome record fields must include:** `success`, `exitCode`, `claimedState`, `postExitState`, `beatId`, `rolledBack`, `alternativeAgentAvailable`.
  [source: foolery/src/lib/__tests__/terminal-manager-error-retry-retry.test.ts:306]

### 3.8 Cross-Agent Review Fallback (Dispatch)
- **`runDispatch` must honor cross-agent review invariants.** When a review step is requested, the prior action agent must be excluded from the pool.
- **Fallback rule:** If cross-agent review exclusion empties the pool entirely, and the only remaining candidate is the prior-action agent, the system must fall back to that agent rather than stopping the take. It must emit a stderr banner containing `"Cross-agent review fallback"`.
- **Hard exclusion:** The failed agent must never be re-selected on retry, even if the fallback rule would otherwise allow it. If the pool is a single agent and that agent just failed, `runDispatch` must return `"stop"`.
- **No fallback banner** when an alternative agent exists without needing fallback.
  [source: foolery/src/lib/__tests__/terminal-manager-take-dispatch.test.ts:169]

### 3.9 Take-Loop Follow-Up
- **`handleTakeLoopTurnEnded` must send a follow-up prompt when the beat is still in an active state** (not yet advanced). The prompt must contain the beat id and current state, and instruct the agent about `kno rollback`.
  [source: foolery/src/lib/__tests__/terminal-manager-take-loop-follow-up.test.ts:141]

- **If the beat has already advanced (e.g., to a terminal state), follow-up must NOT be sent.**
  [source: foolery/src/lib/__tests__/terminal-manager-take-loop-follow-up.test.ts:177]

- **If `sendUserTurn` fails, the handler must return false and log a warning containing `"failed to send follow-up prompt"`.**
  [source: foolery/src/lib/__tests__/terminal-manager-take-loop-follow-up.test.ts:201]

- **Follow-up cap:** After 5 stuck turns in the same state without advancement, follow-ups must stop. A stderr banner must be emitted containing `"follow-up cap reached"`, `beatId`, and state. The 6th and subsequent calls must return false without sending.
  [source: foolery/src/lib/__tests__/terminal-manager-take-loop-follow-up.test.ts:253]

- **Cap reset paths:**
  - If the beat state advances to a different active state, the count resets to 1 and `lastState` updates.
  - If the beat reaches a queue or terminal state, the count resets to 0.
  [source: foolery/src/lib/__tests__/terminal-manager-take-loop-follow-up.test.ts:334]

- **Follow-up gated by lease health check BEFORE sending** [source: `foolery/src/lib/terminal-manager-take-follow-up.ts:365-391`]:
  1. Fetch current beat state
  2. If queue/terminal → reset counter, return false
  3. Record progress (reset count if state changed, increment)
  4. If count > `MAX_FOLLOW_UPS_PER_STATE (5)` → emit cap banner, return false
  5. `evaluateLeaseHealth(leaseId, repoPath)`:
     - `"lease_ready"` or `"lease_active"` → healthy, proceed
     - `"lease_missing"`, `"lease_terminated"`, `"lease_state_unknown"` → `refuseFollowUpForDeadLease`, emit stderr banner, log audit event, return false
  6. Build follow-up prompt + `runtime.sendUserTurn`

- **Canary:** The take-loop runtime bundle factory must wire `onTurnEnded`. If removed, tests must fail. The exact fake-fix pattern is extracting `config.onTurnEnded` as a function.
  [source: foolery/src/lib/__tests__/terminal-manager-take-loop-follow-up.test.ts:430]

### 3.12 Take-Iteration Close Decision Engine

`handleTakeIterationClose(ctx, exitCode, iterationAgent, claimedState)` is the post-child-exit decision engine [source: `foolery/src/lib/terminal-manager-take-iteration.ts:43-119`]:

1. If `sessionAborted()` → finish immediately (don't spawn more children)
2. `captureBeatSnapshot(post_turn_failure)` — fire-and-forget goroutine, must never re-throw into the take loop
3. `Backend.get(beatId)` → post-exit state
4. Log `beat_state_observed` to interaction log with `reason: "post_exit_state_observed"`
5. Resolve workflow, step, claimed action state
6. `classifyIterationSuccess(code, claimedState, postExitState, workflow)`:
   - Success (`true`): beat advanced to next expected queue/terminal state, or review returned to prior queue (rejection is still successful work)
   - Failure (`false`): beat stayed at same state, jumped to unrelated terminal, or stuck in active state
7. Check alternative agent availability (`hasAlternativeAgent`)
8. Build outcome record: `{success, exitCode, claimedState, postExitState, beatId, rolledBack, alternativeAgentAvailable}`
9. **Branch on exit code:**
   - `code !== 0` → `handleErrorExit`:
     - Record failed agent in `failedAgentsPerQueueType`
     - `enforceQueueTerminalInvariant(ctx)` → rollback if stuck in active state
     - Write outcome + audit events
     - `buildNextTakePrompt(ctx, errorAgentId)` with error agent exclusion
     - [alternative exists] increment iteration, emit retry events, spawn new child with different agent
     - [exhausted] log stop reason, finish session
   - `code === 0` → `handleSuccessExit`:
     - Write outcome + audit events
     - `buildNextTakePrompt(ctx)` (no error exclusions)
     - [next take available] continue loop
     - [done] enforce invariant, finish session

`buildNextTakePrompt(ctx, lastErrorAgentId?)` algorithm [source: `foolery/src/lib/terminal-manager-take-prompt.ts:54-142`]:
1. `Backend.get(beatId)` → current beat
2. Resolve workflow
3. `[terminalStates.includes(state)]` → `handleTerminalState`: log stop, `markBeatShipped` (fire-and-forget), return null
4. `[phase === Active && owner === "agent"]` → `rollbackStepFailure`: find corresponding queue state, execute `rollbackBeatState`, reload beat
5. `[phase !== Queued || owner !== "agent"]` → `handleNotAgentOwned`: log stop reason, return null
6. Derive `queueType` from pool key, increment `claimsPerQueueType[queueType]`
7. `selectStepAgent(ctx, workflow, state, queueType, lastErrorAgentId)`:
   - `computeExclusions`: `failedAgentsPerQueueType[queueType]` ∪ `lastErrorAgentId` ∪ (`isReview` ? current agent + prior action step agent : `lastAgentPerQueueType[queueType]`)
   - `runDispatch(DispatchArgs)` → `"stop"` or `{stepAgentOverride, maxClaims}`
8. If `claimCount > maxClaims` → `handleMaxClaims`: announce per-queue cap, log stop, return null
9. `buildClaimPromptResult`: rotate Knos lease → `BackendPort.buildTakePrompt(beatId, repoPath)` → `finalizeClaim` → return `{prompt, beatState, agentOverride}`

**`computeExclusions` rule [source: `foolery/src/lib/terminal-manager-take-agent.ts:106-133`]:**
- Start with `failedAgentsPerQueueType[queueType]` (set of agents that already failed for this queue type)
- Add `lastErrorAgentId` if present (the agent that just errored)
- If `isReview`: add current iteration agent + prior action step agent (cross-agent review exclusion)
- If NOT review: add `lastAgentPerQueueType[queueType]` (soft rotation exclusion)
- The exclusion set is passed to `runDispatch` → `resolveDispatchAgent(ctx, {strictExclusion})`
- **`shouldContinueShipFollowUp` returns true on a clean close (`exitCode: 0`, `exitReason: "raw_close"`) when the execution prompt was sent but ship completion prompt was not.**
- **It returns false for fatal runtime exits** such as `"timeout"` or `"error"`, even if the exit code is 0.
  [source: foolery/src/lib/__tests__/terminal-manager-follow-up.test.ts:6]

### 3.11 Next Knot Guard & Edge Cases
- **`rollbackBeatState` throwing must not crash the terminal manager.** The session must reject with `"not agent-claimable"` and must not spawn children.
  [source: foolery/src/lib/__tests__/terminal-manager-knots-next-guard-take-loop.test.ts:468]

- **If a beat remains non-claimable after rollback, `createSession` must reject** and must not spawn children.
  [source: foolery/src/lib/__tests__/terminal-manager-knots-next-guard-take-loop.test.ts:504]

- **If the beat is already in a claimable queue state, rollback must be skipped.**
  [source: foolery/src/lib/__tests__/terminal-manager-knots-next-guard-take-loop.test.ts:539]

- **`assertClaimable` must throw when the beat is in an active non-claimable state.**
  [source: foolery/src/lib/__tests__/terminal-manager-knots-next-guard-take-loop.test.ts:468]

---

## 4. Session Connection Manager

### 4.1 Connection Lifecycle
- **`connect()` must be idempotent.** Repeated calls for the same sessionId must not create duplicate SSE connections.
  [source: foolery/src/lib/__tests__/session-connection-manager.test.ts:142]

- **`disconnect()` must close the EventSource** and remove the session from connected IDs.
  [source: foolery/src/lib/__tests__/session-connection-manager.test.ts:245]

- **`onError` must remove the connection entry** so that a later `startSync` can reconnect.
  [source: foolery/src/lib/__tests__/session-connection-manager.test.ts:272]

### 4.2 Event Buffering & Forwarding
- **`subscribe()` must receive forwarded events.** `unsubscribe()` must stop forwarding.
  [source: foolery/src/lib/__tests__/session-connection-manager.test.ts:203]

- **`getBuffer()` must return buffered events for replay** and an empty array for unknown sessions.
  [source: foolery/src/lib/__tests__/session-connection-manager.test.ts:226]

- **`hasExited()` must return true after an `exit` event.** `getExitCode()` must return the parsed integer code after exit, and null before.
  [source: foolery/src/lib/__tests__/session-connection-manager.test.ts:254]

### 4.3 Status & Notification Side Effects
- **`exit` with code 0 must update terminal store status to `"completed"`** and fire an in-app notification with beat title and repo.
- **`exit` with non-zero code must update status to `"error"`** and fire an error notification including the exit code.
- **Duplicate exit events must notify only once.**
- **`beat_state_observed` must invalidate queries** for beats, setlist-plan, and setlist-plan-beat, but must NOT update terminal status or fire notifications.
  [source: foolery/src/lib/__tests__/session-connection-manager.test.ts:151]

- **Preserving aborted status:** If the terminal status is already `"aborted"`, the `exit` event must NOT overwrite it to `"completed"`. The notification must say `"terminated"` instead of `"completed"`.
  [source: foolery/src/lib/__tests__/session-connection-manager.test.ts:367]

- **`startSync` must connect only to running sessions**, must be idempotent (no duplicate subscribers), and `stopSync` must disconnect all and remove subscribers.
  [source: foolery/src/lib/__tests__/session-connection-manager.test.ts:282]

### 4.4 Approval Notifications
- **Approval banner events in stderr matching `FOOLERY APPROVAL REQUIRED` must:**
  - Upsert the pending approval exactly once.
  - Fire a notification with `kind: "approval"`, message, `beatId`, `repoPath`, and `href` pointing to approvals tab.
  - Show a toast warning exactly once.
  - Duplicate approval events must not emit a second notification.
  [source: foolery/src/lib/__tests__/session-connection-manager.test.ts:408]

---

## 5. Terminal Approval Session

### 5.1 Approval Recording
- **`recordPendingApproval` must store approvals with reply metadata** including `supportedActions`, `nativeSessionId`, `replyTarget`, and map them by `approvalId` within the session entry.
  [source: foolery/src/lib/__tests__/terminal-approval-session.test.ts:73]

### 5.2 Approval Action Routing
- **`performApprovalAction` must:**
  - Return `ok: true` and mark status `"approved"` when the responder succeeds.
  - Route Codex approvals through the JSON-RPC session’s `respondToApproval`.
  - Mark Claude bridge approvals as `"approved"` after UI action (no external responder needed).
  [source: foolery/src/lib/__tests__/terminal-approval-session.test.ts:107]

### 5.3 Failure Paths
- **Unsupported action:** Return `ok: false`, status `"unsupported"`, `failureReason: "approval_action_not_supported"`.
- **Failed reply:** Return `ok: false`, status `"reply_failed"`, `failureReason` from responder. The failure reason must be appended to the session buffer.
- **Retry success must clear `failureReason`.**
  [source: foolery/src/lib/__tests__/terminal-approval-session.test.ts:251]

### 5.4 Registry Cleanup
- **`cleanupTerminalSessionResources` must mark pending approvals as `manual_required` with `actionable: false` and `actionableReason: "approval_responder_unavailable"`.** The global approval registry must survive terminal session removal.
  [source: foolery/src/lib/__tests__/terminal-approval-session.test.ts:212]

### 5.5 API Route
- **POST `/api/terminal/{sessionId}/approvals/{approvalId}` must:**
  - Return 404 if the session does not exist.
  - Delegate to the canonical approval action.
  - Return 200 with `approvalId`, `action`, `status`.
  [source: foolery/src/lib/__tests__/terminal-approval-compat-route.test.ts:83]

---

## 6. Stores

### 6.1 Terminal Store
- **`updateStatus` must be referentially transparent when the status is unchanged.** It must mutate the terminal reference only on actual changes.
  [source: foolery/src/stores/__tests__/terminal-store.test.ts:33]

- **`upsertTerminal` must add or replace by `sessionId`.** Adding a terminal must set it as active and open the panel. Removing a terminal must fall back to the last remaining terminal as active; if none remain, `activeSessionId` becomes null and the panel closes.
  [source: foolery/src/stores/__tests__/terminal-store-extended.test.ts:111]

- **Panel controls:** `openPanel`, `closePanel`, `togglePanel`. Height clamped to [15, 80].
  [source: foolery/src/stores/__tests__/terminal-store-extended.test.ts:54]

- **`rehydrateFromBackend` must:**
  - Mark stale running terminals as `"disconnected"` if absent from the backend list.
  - Not change non-running terminals absent from backend.
  - Sync `status` and `startedAt` for known terminals.
  - Adopt orphaned running backend sessions into the store and open the panel/unminimize.
  - Not adopt completed orphans.
  - Fix `activeSessionId` if it points to a non-existent terminal.
  [source: foolery/src/stores/__tests__/terminal-store-extended.test.ts:294]

### 6.2 Notification Store
- **`addNotification` must prepend, assign generated `id` and `timestamp`, default `read: false`.**
- **`markAllRead` must mark all unread as read without mutating state if there are no unread.**
- **`clearAll` must empty the list.**
- **`selectUnreadCount` must return the count of unread notifications.**
- **Deduplication:** If a `dedupeKey` already exists, `addNotification` must return `false` and not add a duplicate.
- **Optional fields:** `beatId`, `repoPath`, `href`, `kind` must be preserved.
  [source: foolery/src/stores/__tests__/notification-store.test.ts:7]

### 6.3 App Store
- **Initial state:** `filters: { state: "queued" }`, `commandPaletteOpen: false`, `viewMode: "table"`, `activeRepo: null`, `registeredRepos: []`, `pageSize: 50`.
- **`setFilter` must overwrite the specific key while preserving others.**
- **`setFiltersFromUrl` must replace the entire filters object.**
- **`resetFilters` must revert to `{ state: "queued" }`.**
- **`setActiveRepo` must persist to `localStorage`.** Setting `null` must persist the all-repositories sentinel and `getPersistedRepo()` must return null.
- **`setRegisteredRepos` and `setPageSize` mutations.**
  [source: foolery/src/stores/__tests__/app-store.test.ts:22]

---

## 7. Cross-Cutting Contracts (Go Port Alignment)

### 7.1 Fail Loudly, Never Silently
- All tests enforce that missing resources, invalid configurations, dispatch pool exhaustion, and approval routing failures must surface explicit error markers. The system must:
  1. Return an error that halts the operation.
  2. Log an ANSI-red banner or structured warning.
  3. Surface the failure to the session buffer as a stderr banner event.
  4. Include a greppable marker (`FOOLERY DISPATCH FAILURE`, `TERMINAL_DISPATCH_FAILURE_MARKER`, `follow-up cap reached`).
  5. Name the missing thing and the config that fixes it.
  6. Never return the first registered item, a `?? "default"` literal, or a downgraded warning.
  [source: foolery/AGENTS.md:51]

- **Dispatch failure example:** `resolveTakeSceneCapabilities` must throw when one-shot capabilities are used for take/scene dispatch, carrying `TERMINAL_DISPATCH_FAILURE_MARKER`.
  [source: foolery/src/lib/__tests__/terminal-dispatch-capabilities.test.ts:50]

- **Approval failure example:** Unsupported approval actions must set `status: "unsupported"`, `failureReason: "approval_action_not_supported"`.
  [source: foolery/src/lib/__tests__/terminal-approval-session.test.ts:251]

### 7.2 Hermetic Test Policy
- Default tests must not touch `process.env`, real filesystem, real child processes, real network, real timers, or host binaries (`git`, `kno`, `node`, `bun`).
- All external dependencies are mocked (`backend-instance`, `child_process`, `knots`, `settings`, `localStorage`, etc.).
- Tests that need the real environment belong in `__manual_tests__` and run via `bun run test:manual`.
  [source: foolery/AGENTS.md:100]

### 7.3 Session Lease Info Sync
- `syncSessionLeaseInfo` must mirror `knotsLeaseId` and `knotsLeaseAgentInfo` onto the session object. It must drop non-canonical fields like `agentType` from the surfaced `knotsAgentInfo`. When the lease is undefined, it must clear the session lease fields.
  [source: foolery/src/lib/__tests__/sync-session-lease-info.test.ts:40]

### 7.4 Retake Session Scope
- **Repo-scoped session lookup:** `findRunningTerminalForBeat` must return the running session from the same repo when beat ids collide across repos.
  [source: foolery/src/lib/__tests__/retake-session-scope.test.ts:14]

- **`isRetakeSourceState` must accept:** `shipped`, `closed`, `done`, `approved` (case-insensitive, whitespace-tolerant). It must reject non-terminal states.
  [source: foolery/src/lib/__tests__/retake-source-state.test.ts:4]

### 7.5 Cascade Close
- **`getOpenDescendants` must return leaf-first (post-order) list** of open descendants. It must exclude already-closed children.
  [source: foolery/src/lib/__tests__/cascade-close.test.ts:38]

- **`cascadeClose` must close leaf children before the parent.** It must collect errors without blocking siblings, returning both the list of closed ids and any errors.
  [source: foolery/src/lib/__tests__/cascade-close.test.ts:101]

### 7.6 Draft Persistence
- **`saveDraft` / `loadDraft` must round-trip JSON.** Invalid JSON or non-object JSON must return null. Overwrites must replace previous drafts.
- **`hasDraft` / `clearDraft` behavior.**
- **`mergeDraftDefaults` must merge draft fields over defaults without overriding with empty arrays or empty strings unless explicitly set.**
  [source: foolery/src/lib/__tests__/create-draft-persistence.test.ts:23]

---

## 8. Dialect Normalizers (Summary)

- **Claude normalizer:** Passes through valid objects. Returns null for non-objects.
  [source: foolery/src/lib/__tests__/agent-adapter.test.ts:6]

- **Codex normalizer:** Skips `thread.started` and `turn.started`. `item.completed` with `agent_message` -> assistant. `item.completed` with `reasoning` -> `stream_event` delta. `item.started` with `command_execution` -> `assistant` `tool_use`. `item.completed` with `command_execution` -> `user` `tool_result`. `turn.completed` -> `result` with accumulated text. `turn.failed` / `error` -> `result` with `is_error: true`. Unknown types return null.
  [source: foolery/src/lib/__tests__/agent-adapter-codex.test.ts:6]

- **Copilot normalizer:** `assistant.message_delta` -> `stream_event` delta. `assistant.message` with `toolRequests` -> `assistant` `tool_use`. `user_input.requested` -> `assistant` `AskUserQuestion`. `session.task_complete` -> `result` with accumulated text. `session.error` -> `result` `is_error: true`.
  [source: foolery/src/lib/__tests__/agent-adapter-copilot-gemini.test.ts:6]

- **Gemini normalizer:** Skips `init` and user messages. `message` assistant -> `assistant`. `result` success -> `result` with accumulated text. `result` error -> `result` `is_error: true`.
  [source: foolery/src/lib/__tests__/agent-adapter-copilot-gemini.test.ts:123]

- **OpenCode normalizer:** `text` -> `assistant`. `step_finish` reason `"stop"` -> null (per-message boundary). `step_finish` reason `"error"` -> `result` `is_error`. `session_idle` -> `result` with accumulated text and resets accumulator. `tool_use` / `tool_result` map to Claude-shaped blocks. `reasoning` -> `stream_event` delta. `session_error` -> `result` `is_error`.
  [source: foolery/src/lib/__tests__/agent-adapter-opencode.test.ts:12]

- **Edge cases:** `item.completed` with unknown item type returns null. `item.started` without item returns null. Non-string `aggregated_output` must be coerced. `turn.failed` without error object still returns `is_error: true` with a generic message.
  [source: foolery/src/lib/__tests__/agent-adapter-edge.test.ts:46]
