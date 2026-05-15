# Transport Behavioral Contracts for Foolery Go Port
> Extracted from TypeScript Foolery test sources. Citations reference the canonical regression suite.

## Cross-Cutting Principles (per AGENTS.md)

- **Fail Loudly, Never Silently**: Every lookup failure for a configured transport, agent, or backend MUST halt the operation, log a red stderr banner with the greppable marker `FOOLERY DISPATCH FAILURE`, name the missing resource, and surface the failure to any visible session buffer. No silent fallback to the first registered item or a hard-coded default string.
- **Hermetic Tests**: All default tests MUST avoid touching the host environment — no real `os.Getenv`, real filesystem, real network/ports, real process clocks, or real subprocess binaries. Push resolution up the stack and inject dependencies so tests target pure deterministic logic.
- **YAGNI & Flat**: Do not generate preventive abstractions. Do not create single-use interfaces, mappers, or shims. Keep the file structure flat.
- **Functions 4–40 lines, Files under 500 lines**: When porting logic, split by responsibility.
- **Explicit Types**: No `interface{}` overuse. Use Go interfaces for boundaries (e.g., `BackendPort`, `Transport`), but not for single-use indirection.
- **Comments: WHY, not WHAT**: Docstrings on public functions must state intent plus one usage example.

---

## 1. NDJSON Stream Consumption

The system MUST parse newline-delimited JSON from an arbitrary byte stream, independent of chunk boundaries.

**Contracts**
- **Complete lines**: Each line terminated by `\n` yields exactly one parsed JSON value delivered to the consumer.
  [source: foolery/src/lib/__tests__/ndjson-stream.test.ts:17]
- **Chunked boundaries**: A single logical line MAY be split across multiple read chunks; the parser MUST buffer and reassemble before decoding.
  [source: foolery/src/lib/__tests__/ndjson-stream.test.ts:31]
- **Trailing partial flush**: If the stream closes without a trailing newline, the parser MUST flush any non-empty buffered bytes as a final JSON value.
  [source: foolery/src/lib/__tests__/ndjson-stream.test.ts:46]
- **Skip empties**: Empty lines (including consecutive `\n\n`) MUST be ignored and MUST NOT produce empty objects or errors.
  [source: foolery/src/lib/__tests__/ndjson-stream.test.ts:58]
- **Abort respect**: If an abort signal is already triggered before parsing begins, the loop MUST produce no values and exit immediately.
  [source: foolery/src/lib/__tests__/ndjson-stream.test.ts:71]

**Failure modes**
- Invalid JSON in a partial trailing line after stream close MUST surface as a parse error (fail loud), not be silently swallowed.

---

## 2. Backend HTTP Error Mapping

The system MUST translate domain-specific backend error codes into HTTP status codes for API responses.

**Contracts**
- `INVALID_INPUT` → `400`
- `NOT_FOUND` → `404`
- `UNAVAILABLE` → `503`
- Unknown or unlisted codes → `500`
  [source: foolery/src/lib/__tests__/backend-http.test.ts:13]

**Invariants**
- No code is allowed to silently suppress an unmapped error or return a success status.

---

## 3. Transport-Agnostic `onTurnEnded` Firing

`onTurnEnded` is the definitive signal that an agent turn has concluded. It MUST fire for **every** supported transport, and it MUST NOT be gated on a single transport-specific payload shape such as `{type: "result"}` in the generic runtime core.

**Contracts**
- **Claude stdio**: Fires when a line with `type: "result"` arrives.
  [source: foolery/src/lib/__tests__/on-turn-ended-transports.test.ts:106]
- **Codex JSON-RPC**: Fires on the JSON-RPC notification `method: "turn/completed"` (including the `failed` variant with an error).
  [source: foolery/src/lib/__tests__/on-turn-ended-transports.test.ts:139]
  [source: foolery/src/lib/__tests__/on-turn-ended-transports.test.ts:174]
- **Gemini ACP**: Fires when the prompt request result includes a `stopReason: "end_turn"`.
  [source: foolery/src/lib/__tests__/on-turn-ended-transports.test.ts:212]
- **OpenCode HTTP**: Fires on `type: "session_idle"` (the translated form of the SSE `session.idle` event), NOT on per-message `step_finish` with `reason: "stop"`.
  [source: foolery/src/lib/__tests__/on-turn-ended-transports.test.ts:248]

**Invariants**
- Deduplication: Repeated `session_idle` signals in the same session MUST trigger `onTurnEnded` exactly once.
  [source: foolery/src/lib/__tests__/on-turn-ended-transports.test.ts:280]
- Synthesized error step finish: A synthesized `step_finish` with `reason: "error"` from the HTTP transport MUST still fire `onTurnEnded`, because the turn is effectively over.
  [source: foolery/src/lib/__tests__/on-turn-ended-transports.test.ts:302]

**Canary — must NOT fire**
- `item/completed` notifications (Codex).
  [source: foolery/src/lib/__tests__/on-turn-ended-transports.test.ts:335]
- Non-result stdio events such as `type: "assistant"` (Claude).
  [source: foolery/src/lib/__tests__/on-turn-ended-transports.test.ts:368]
- OpenCode text deltas (`type: "text"`).
  [source: foolery/src/lib/__tests__/on-turn-ended-transports.test.ts:386]

**Failure mode**
- If the transport layer detects a malformed or unexpected turn-end signal, it should surface it as a runtime error rather than guessing.

---

## 4. OpenCode Event Translation

OpenCode uses SSE envelopes and response parts. The system MUST normalize these into a uniform internal event stream.

### Part Translation
- `step-start` → `step_start`
- `text` → `text` with `part: { text }`
- `step-finish` → `step_finish` with `part: { reason }`; if the field is missing, default `reason` to `"stop"`.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:14]
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:31]
- Unknown part types return an empty array (no event).
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:50]

### Tool State Translation
- **Running tool**: Emits a single `tool_use` event with `status: "running"` and the provided input.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:58]
- **Completed tool**: Emits `tool_use` followed by `tool_result` with `status: "completed"` and the output content.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:77]
- **Pending tool**: Emits only `tool_use` with `status: "pending"` and empty `input`; no `tool_result`.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:103]

### SSE Event Translation
- `permission.asked` forwarded verbatim at top level.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:157]
- `permission.updated` inside nested `event` field: inject `type: "permission.updated"` into the normalized event.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:166]
- `message.part.updated` wrapping a tool part: delegates to tool translation (emits `tool_use` + optionally `tool_result`).
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:180]
- Falls back to `data.part` if `properties.part` is absent.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:209]
- `message.updated` → `message_updated` with `info`.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:220]
- `step.updated` → `step_updated`.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:235]
- `session.idle` → `session_idle`.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:246]
- `session.status` with `status: { type: "idle" }` → `session_idle`.
  `session.status` with `status: { type: "busy" }` MUST be dropped entirely.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:257]
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:277]
- `session.error` → `session_error` with the error message copied to a top-level `message` field.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:293]
- Unknown SSE envelopes return an empty array.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:307]
- Non-objects (e.g., strings, null) return an empty array.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:312]

### Response Aggregation
- When translating a full response, mixed parts MUST be emitted in original order.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:320]
- Events from a separate `events` collection on the response MUST be appended after the parts-derived events.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:347]

### Payload Detection
- `hasOpenCodeMessagePayload` returns true when `parts` is present, when `events` is present, or for direct permission events. It returns false for an empty record.
  [source: foolery/src/lib/__tests__/opencode-event-translate.test.ts:362]

---

## 5. OpenCode Event Formatting

Formatting MUST render domain-specific lifecycle events for user-visible output; unsupported or empty events return `null` so the terminal layer can skip them.

**Contracts**
- Reasoning text rendered in magenta and marked as `isDetail = true`. Empty reasoning returns `null`.
  [source: foolery/src/lib/__tests__/opencode-event-format.test.ts:19]
- `step_updated` rendered with status label; empty step object returns `null`.
  [source: foolery/src/lib/__tests__/opencode-event-format.test.ts:36]
- `session_idle` rendered with the session ID.
  [source: foolery/src/lib/__tests__/opencode-event-format.test.ts:52]
- `session_error` rendered with high-visibility (`isDetail = false`) and contains the error message.
  [source: foolery/src/lib/__tests__/opencode-event-format.test.ts:60]
- File events rendered with filename and MIME type; missing filename returns `null`.
  [source: foolery/src/lib/__tests__/opencode-event-format.test.ts:69]
- Snapshot content truncated before rendering.
  [source: foolery/src/lib/__tests__/opencode-event-format.test.ts:87]
- `message_updated` rendered only when `time.completed` is present; otherwise `null`.
  [source: foolery/src/lib/__tests__/opencode-event-format.test.ts:98]
- Unrelated types (e.g. `tool_use`) return `null`.
  [source: foolery/src/lib/__tests__/opencode-event-format.test.ts:113]

---

## 6. Codex Event Formatting

Codex events arrive after JSON-RPC translation. Formatting MUST produce terminal-ready output with clear detail vs. main-stream separation.

**Contracts**
- `turn.started` and `turn.completed` are rendered as concise marker lines (`▷ turn …`) with `isDetail = true`.
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:13]
- `turn.failed` is ALWAYS visible (`isDetail = false`). It includes the error message, with a fallback `"no error message"` if absent.
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:32]
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:48]
- `item.completed` with `agent_message` renders the text as plain output. Empty text returns `null`.
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:62]
- `item.started` for `agent_message` is dropped (`null`).
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:75]
- `item.delta` for `agent_message` renders raw streaming text.
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:84]
- `command_execution` `item.started` shows the command as a detail marker (`▶ command`). Long commands are clipped with ellipsis and capped to a reasonable length.
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:109]
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:164]
- `command_execution` `item.completed` shows `aggregated_output`. Non-completed status (e.g. `failed`) MUST be surfaced visibly as `[failed]`.
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:122]
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:136]
- Reasoning `item.completed` rendered dimmed/as detail. Empty reasoning dropped.
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:178]
- Terminal interaction events render a concise diagnostic line including item ID, process ID, and stdin content; empty stdin is marked explicitly.
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:198]
- Non-Codex events and unknown item types return `null`.
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:228]
  [source: foolery/src/lib/__tests__/codex-event-format.test.ts:239]

---

## 7. Codex JSON-RPC Translation

The translator is a pure function layer between Codex wire shapes and the internal event bus.

**Contracts**
- `agentMessage/delta`: prefer `params.delta`; fallback to `params.text`. Return `null` if both absent or empty. Emit `item.delta` with `item: { type: "agent_message", id }`; omit `id` when missing.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-translate.test.ts:13]
- `commandExecution/outputDelta`: same `delta`/`text` fallback. Return `null` if empty.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-translate.test.ts:52]
- Reasoning (`textDelta`, `summaryTextDelta`): identical delta/text fallback.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-translate.test.ts:76]
- Terminal interaction: preserves `stdin`, empty strings allowed. Return `null` if no useful data.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-translate.test.ts:94]
- Item filtering: `userMessage` items MUST be filtered at both `item/started` and `item/completed` (they are prompt echoes).
  [source: foolery/src/lib/__tests__/codex-jsonrpc-translate.test.ts:131]
- Empty reasoning `item/completed` (empty `summary` / `summaryParts`) MUST be dropped.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-translate.test.ts:148]
- Reasoning content: concatenate `summary` array text items with `\n`; support legacy `summaryParts`.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-translate.test.ts:162]
- Command items: preserve `aggregatedOutput`, `command`, and `status` in translation. Non-completed statuses (e.g. `failed`) carried forward.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-translate.test.ts:228]
- Turn lifecycle:
  - Completed turn → `turn.completed`, `turnFailed = false`.
  - Failed turn → `turn.failed` with error message, `turnFailed = true`.
  - Missing error message defaults to `"Turn failed"`.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-translate.test.ts:276]
- Method allowlist: `isTranslatedMethod` returns true only for known methods (e.g. `terminalInteraction`, `outputDelta`); unknown methods are rejected so upstream filtering can drop them.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-translate.test.ts:315]

---

## 8. Codex JSON-RPC Session Lifecycle

The session maintains handshake state, turn queuing, and approval routing over a child process stdio transport.

**Session state model [source: `foolery/src/lib/codex-jsonrpc-session.ts:442-519`]:**
- `nextId = 3` (after init=1, thread=2)
- `ready = false` until both handshake responses return
- `threadId` populated after `thread/start` response
- `turnId` populated after `turn/start` response
- `turnInProgress: boolean` gate for interrupt
- `pendingTurn: {prompt, hooks} | null` — buffered if `startTurn` called before handshake complete
- `approvalRequestIds: Map<string, number>` — tracks request IDs for reply routing

**Contracts**
- **Handshake** sends two requests in order:
  1. `initialize` (id = 1) with `{clientInfo: {name: "foolery", version: "1.0.0"}}`
  2. `thread/start` (id = 2)
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:63]
- Handshake params:
  - `approvalPolicy` defaults to `"never"`, but MUST be overridable (e.g. `"untrusted"`).
  - `sandboxMode` is passed as `sandbox` when set (e.g. `"read-only"`).
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:82]
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:98]
- Ready gate: `ready` is false until both handshake responses return. Then `ready = true` and `threadId` is populated.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:117]
- **Turn queuing**: `startTurn` before handshake returns true but defers the wire write. After handshake completion, the pending turn MUST be flushed automatically via `flushPendingTurn`.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:171]
- **`flushPendingTurn`** (L193-218): If thread ready and turn not in progress, sends `turn/start` with `{threadId, input: [{type:"text", text: prompt}], approvalPolicy}`. Sets `turnInProgress = true`.
- **Delivery hooks**: deferred/attempted/succeeded/failed hooks MUST fire in the correct order; failed MUST NOT fire on success.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:205]
- **Interrupt**: `interruptTurn` sends `turn/interrupt` as JSON-RPC notification. Returns `false` if no active turn exists (`!turnInProgress`).
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:236]
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:258]
- **Approval replies**:
  - Elicitation requests responded to by JSON-RPC `id`.
  - `"approve"` → `action: "accept"`.
  - `"reject"` → `action: "decline"`.
  - Command execution approval → `decision: "acceptForSession"` for `"always_approve"`.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:267]
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:312]
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:337]
- **Notification translation**:
  - `turn/started` → `turn.started`.
  - `turn/completed` with `status: "completed"` → `turn.completed`.
  - `turn/completed` with `status: "failed"` → `turn.failed`.
  - `item/started` command → `item.started` with zero-initialized `aggregated_output`.
  - `item/completed` agentMessage → `item.completed` with joined fragment text.
  - `item/completed` command → `item.completed` with `aggregated_output`.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:378]
- **Filtering**: `mcpServer/startupStatus/updated` and `thread/started` notifications MUST be dropped (return `null`).
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:508]
- **JSON-RPC error responses**: MUST log the error (loudly) and not crash the session.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:530]
- **Edge cases**: Unknown message shapes return `null`. `startTurn` with a destroyed stdin returns `false`.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:549]
  [source: foolery/src/lib/__tests__/codex-jsonrpc-session.test.ts:558]

**Go port considerations:**
- JSON-RPC line protocol: `json.NewEncoder(pipe)` + `"\n"` delimiter matches NDJSON
- Session state: goroutine-per-session, single owner — no mutex needed for session state fields
- `writeJsonLine` → check `cmd.StdinPipe()` not closed, `enc.Encode(payload)` then call `Flush()` on the writer
- `Map<string, number>` for `approvalRequestIds` → Go: `map[string]int64`
- `processLine` is the main dispatch — switch on JSON-RPC method + id to route to response handler or notification translator

---

## 9. Codex JSON-RPC Delta Regression Contracts

Deltas are incremental text chunks. The system MUST tolerate both current (`delta`) and legacy (`text`) payload keys.

**Contracts**
- `agentMessage/delta` and `commandExecution/outputDelta` accept both `params.delta` and `params.text`; empty payloads return `null`.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-deltas.test.ts:15]
  [source: foolery/src/lib/__tests__/codex-jsonrpc-deltas.test.ts:74]
- Reasoning `textDelta` and `summaryTextDelta` likewise accept both keys.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-deltas.test.ts:140]
- Empty reasoning `item/completed` MUST be dropped even when `summaryParts` contains entries with empty text.
  [source: foolery/src/lib/__tests__/codex-jsonrpc-deltas.test.ts:166]

---

## 10. Agent Token Usage

Token accounting MUST be extracted from transport-specific completion events and attributed exactly once.

**Contracts**
- Codex `turn.completed` with `input_tokens` and `output_tokens` yields `{ inputTokens, outputTokens, totalTokens }`.
  [source: foolery/src/lib/__tests__/agent-token-usage.test.ts:8]
- Incomplete payloads (e.g. only `total_tokens`) MUST be ignored (`null`) rather than guessed.
  [source: foolery/src/lib/__tests__/agent-token-usage.test.ts:24]
- Logging MUST bind usage to a single consuming beat. Even with multiple beats in scope, the event logs exactly once under the target beat ID.
  [source: foolery/src/lib/__tests__/agent-token-usage.test.ts:33]
  [source: foolery/src/lib/__tests__/agent-token-usage.test.ts:62]

---

## 11. OpenCode Model Selection

Model identifiers include a provider prefix and a slash-delimited model ID, possibly containing additional slashes.

**Contracts**
- `"openrouter/z-ai/glm-5.1"` splits into `providerID = "openrouter"` and `modelID = "z-ai/glm-5.1"`.
  [source: foolery/src/lib/__tests__/opencode-model-selection.test.ts:11]
- Empty, whitespace-only, or undefined input returns undefined.
  [source: foolery/src/lib/__tests__/opencode-model-selection.test.ts:22]
- Invalid references lacking a provider slash MUST throw a loud error with the message `"expected \u003cproviderID\u003e/\u003cmodelID\u003e"`.
  [source: foolery/src/lib/__tests__/opencode-model-selection.test.ts:29]

---

## 12. OpenCode Rendering Pipeline (Smoke)

The end-to-end pipeline MUST translate and format a complete OpenCode turn so that the user-visible stream contains reasoning, tool calls, tool output, session errors, and permission banners.

**Contracts**
- A complete turn with reasoning, a completed bash tool, assistant text, and step finish MUST result in rendered output containing all four elements in order.
  [source: foolery/src/lib/__tests__/opencode-pipeline-smoke.test.ts:71]
- Tool call rendering MUST recognize both snake_case (`file_path`) and camelCase (`filePath`) input fields so file paths are visible.
  [source: foolery/src/lib/__tests__/opencode-pipeline-smoke.test.ts:108]
- `session.error` MUST render as an error banner containing the message.
  [source: foolery/src/lib/__tests__/opencode-pipeline-smoke.test.ts:129]
- `permission.asked` MUST render an approval banner containing the word "approval".
  [source: foolery/src/lib/__tests__/opencode-pipeline-smoke.test.ts:138]

---

## 13. Codex Plan Parsing & Accumulation

The Codex normalizer MUST preserve plan-structured NDJSON and tagged JSON inside `agent_message` text, and it MUST accumulate multi-part assistant output into the final turn result.

**Contracts**
- NDJSON `plan_final` and `wave_draft` objects inside `agent_message` text MUST be preserved line-by-line.
  [source: foolery/src/lib/__tests__/codex-plan-parsing.test.ts:5]
  [source: foolery/src/lib/__tests__/codex-plan-parsing.test.ts:154]
- Tagged wrappers such as `<plan_json>`…`</plan_json>` MUST survive normalization intact.
  [source: foolery/src/lib/__tests__/codex-plan-parsing.test.ts:76]
- Multiple `agent_message` items accumulate in order; they MUST NOT replace each other.
  [source: foolery/src/lib/__tests__/codex-plan-parsing.test.ts:110]
- The accumulated text MUST be surfaced in the `turn.completed` result event.
  [source: foolery/src/lib/__tests__/codex-plan-parsing.test.ts:35]
- Claude normalizer MUST be idempotent pass-through.
  [source: foolery/src/lib/__tests__/codex-plan-parsing.test.ts:135]

---

## 14. Performance Events & Timing

### Server Timing
- `withServerTiming` MUST inject a `Server-Timing` response header containing every named measure with its duration, plus a `total` entry.
  [source: foolery/src/lib/__tests__/server-timing.test.ts:11]
- Requests exceeding the configured `slowMs` threshold MUST log a structured warning including the route label and any provided context.
  [source: foolery/src/lib/__tests__/server-timing.test.ts:24]

### Client Perf Events
- `buildPerfMeasureName` / `parsePerfMeasureName` round-trip a structured name of `{category}:{label}:{spanId}`.
  [source: foolery/src/lib/__tests__/perf-events.test.ts:11]
- `summarizeClientPerfEvents` MUST compute:
  - `totalEvents`
  - per-kind counts
  - `totalLongTaskMs`
  - `totalRenderCommitMs`
  [source: foolery/src/lib/__tests__/perf-events.test.ts:20]

### Client Diagnostics Initialization
- Diagnostics MUST be activatable via URL query parameter (`?diagnostics=1`) and the setting MUST persist.
  [source: foolery/src/lib/__tests__/client-perf.test.ts:9]
- Recorded events MUST be summarized correctly.
  [source: foolery/src/lib/__tests__/client-perf.test.ts:39]

---

## 15. Local Worker Tool Execution

The local worker is responsible for running in-repo tools on behalf of agents. It MUST enforce sandbox boundaries and prepare execution context.

**Contracts**
- `shell_exec` MUST block execution of memory-manager binaries (`kno`, `bd`). It MUST return a loud failure with a message indicating the block.
  [source: foolery/src/lib/__tests__/local-worker.test.ts:60]
- Poll preparation MUST claim the first ready, agent-claimable bead and prepare a lease prompt containing that beat’s ID.
  [source: foolery/src/lib/__tests__/local-worker.test.ts:72]
- Parent-scene preparation MUST wrap prompts with instructions to execute child beats in parallel and MUST include the child beat IDs.
  [source: foolery/src/lib/__tests__/local-worker.test.ts:121]
- Knots-backed polls MUST create a lease and populate `knotsLeaseId` in the result.
  [source: foolery/src/lib/__tests__/local-worker.test.ts:154]
- Canonical metadata (`agentName`, `model`, `modelVersion`, `provider`, `agentType`) MUST be forwarded into lease creation options.
  [source: foolery/src/lib/__tests__/local-worker.test.ts:202]

---

## 14. OpenCode HTTP Session Retry & Disposal

### 14.1 Retry Backoff
Message delivery failures MUST retry with delays of 8s, 16s, 32s. Retry warning messages must contain the delay.
[source: foolery/src/lib/__tests__/opencode-http-session-retry.test.ts:74]

Retry safety: If SSE shows turn activity before the message fetch fails, the system must not retry; it must log that it is waiting for `session.idle` from the SSE stream.
[source: foolery/src/lib/__tests__/opencode-http-session-retry.test.ts:146]

### 14.2 Disposal
- `interruptTurn` must dispose the OpenCode server instance (`POST /instance/dispose`).
- It must not abort a turn after SSE `session_idle`; the dispose must happen cleanly.
- `interruptTurn` must clear any pending queued turn before disposal.
[source: foolery/src/lib/__tests__/opencode-http-session-disposal.test.ts:108]

### 14.3 Approval Reply
`respondToOpenCodeApproval` POSTs to `{baseUrl}/session/{nativeSessionId}/permissions/{permissionId}` with body `{response: "once"|"always"|"reject", remember: bool}`. 1500ms timeout. Returns `{ok, status?, reason?}`. [source: `foolery/src/lib/opencode-approval-actions.ts:29-86`]

Action mapping: `"approve"`→`"once"`, `"always_approve"`→`"always"`, `"reject"`→`"reject"`.

### 14.4 Approval Bridge Environment Variables
When spawning take children, the following env vars are injected for Claude approval bridge support [source: `foolery/src/lib/terminal-manager-take-child.ts:268-279`]:
- `FOOLERY_TERMINAL_SESSION_ID`
- `FOOLERY_APPROVAL_BRIDGE_BASE_URL`
- `FOOLERY_APPROVAL_BRIDGE_TOKEN`

---

## Open Issues / Port Gaps to Track

1. **Abort signal propagation in NDJSON parsing**: The TS test proves pre-aborted signals stop parsing immediately. In Go, this maps to `context.Context` cancellation checked at loop boundaries.
2. **Turn-ended deduplication**: The Go runtime needs a per-session boolean guard or equivalent atomic state so duplicate `session_idle` or synthetic terminators emit only once.
3. **Delta key fallback (`delta` vs `text`)**: Codex JSON-RPC deltas must prefer `delta` and fall back to `text`. Missing both = null event.
4. **Hermetic session tests**: Every transport session (Codex, OpenCode, Gemini, Claude) should use injected `io.Reader`/`io.Writer` fakes rather than real child processes. The Go port must not import `os/exec` in unit tests.
5. **Backend error mapping** should be a pure function with exhaustive `switch`; unknown codes MUST hit `default: 500` and never return zero.
6. **OpenCode model selection** must reject invalid strings with a loud error rather than silently ignoring or returning empty strings.
7. **Token usage logging** must dedupe by beat ID; if multiple beats appear in the session scope, the event logs once under the consuming beat, not under every beat.
