# Behavioral Contracts: Prompt, Scope, History, Plan, and Beat Display Domains

## 1. Prompt Guardrails and Execution Boundaries

### 1.1 Single-Step Authority
- The system MUST inject authority lines that restrict the agent to exactly one workflow action per session.
- Authority text MUST name the current workflow mode (e.g., `Planning`) and the allowed exit state(s).
- When only one exit state exists, the phrasing MUST be singular: `Allowed exit state for this session: <state>`.
- When multiple exit states exist, the phrasing MUST be plural and join states with `or`: `Allowed exit states for this session: <state1> or <state2>`.
- Authority lines MUST forbid the agent from claiming, inspecting, reviewing, or advancing later workflow states.
  [source: foolery/src/lib/__tests__/agent-prompt-guardrails.test.ts:10]

### 1.2 Execution Boundary Wrappers
- The system MUST wrap every execution prompt inside a `FOOLERY EXECUTION BOUNDARY` block.
- The boundary MUST instruct the agent to execute only the currently assigned workflow action or explicitly listed child beats.
- For `take` prompts, the boundary MUST contain a hard stop instruction (`stop immediately`) after the single authorized step.
- For `scene` prompts, the boundary MUST treat each child claim as a single-step authorization and mention merge/push confirmation context.
  [source: foolery/src/lib/__tests__/agent-prompt-guardrails.test.ts:28]

### 1.3 Skill Prompt Contracts
- Each workflow step MUST generate a skill prompt containing:
  - A step-specific heading (e.g., `# Planning`).
  - The `bd show <beatId>` command for context retrieval.
  - A `bd sync` reminder.
  - The current workflow state name.
  - An `## Authority Boundary` section with the text `Complete exactly one workflow action, then stop.`
- Skill prompts MUST NEVER contain `kno claim` instructions.
- The prompt MUST include a `bd update <beatId> --state <state> --json` (or equivalent `bd` transition command) for every valid next workflow state, plus the state name in backticks.
- Transition command presence MUST be exhaustive for the step's legal loom transitions.
  [source: foolery/src/lib/__tests__/beats-skill-prompts.test.ts:50]

---

## 2. Scope Refinement

### 2.1 Beat Creation Trigger
- On successful beat creation, the system MUST enqueue a scope-refinement job for the new beat ID.
- If the creation request includes a repo path, that path MUST be passed to the refinement job.
- If beat creation fails, the system MUST NOT enqueue a refinement job.
- If enqueueing itself fails (e.g., queue throws), the beat creation endpoint MUST still return `201 Created` with the created beat; the refinement failure MUST be non-blocking.
  [source: foolery/src/lib/__tests__/beat-creation-scope-refinement.test.ts:43]

### 2.2 Manual Refine-Scope Route
- The `POST /beats/:id/refine-scope` endpoint MUST resolve the canonical beat ID via the backend before enqueuing.
- If canonical lookup fails, the endpoint MUST fall back to the raw provided ID.
- If no refinement agent is configured, the endpoint MUST return `503 Service Unavailable` with a clear `not configured` error.
- On success, the response MUST include `jobId` and `beatId`.
  [source: foolery/src/lib/__tests__/refine-scope-route.test.ts:54]

### 2.3 Refinement Worker Success
- The worker MUST read the beat, spawn the configured scope-refinement agent, and update the beat with the parsed `title`, `description`, and `acceptance` from the agent response.
- The agent output MUST be parsed from a tagged block: `<scope_refinement_json>{...}</scope_refinement_json>`.
- On successful update, the worker MUST record a completion event containing `beatId`, `beatTitle`, and `repoPath`.
- If no agent is configured, the worker MUST skip work silently without spawning or updating.
  [source: foolery/src/lib/__tests__/scope-refinement-worker.test.ts:97]

### 2.4 Refinement Worker Failure and Retry
- On agent non-zero exit, the worker MUST re-enqueue the job.
- On agent outputs flagged with `is_error: true` (normalized error events), the worker MUST treat them as failures and re-enqueue.
- On unparseable agent output, backend read failure, or backend update failure, the worker MUST re-enqueue.
- Failed jobs MUST carry an `excludeAgentIds` list containing the IDs of agents that already failed for this beat.
- On retry, the resolver MUST pass `excludeAgentIds` to agent selection so a different agent is chosen.
- If all agents are exhausted, the worker MUST fail gracefully: record the failure in worker health, do not re-enqueue, and do not update the beat.
  [source: foolery/src/lib/__tests__/scope-refinement-worker.test.ts:152]

### 2.5 Refinement Timeout
- The worker MUST kill the refinement child process with `SIGKILL` after exactly `600s`.
- The old `180s` timeout MUST NOT trigger a kill.
- On timeout, the job MUST be re-enqueued and a log line emitted containing both `scope refinement agent timed out after 600s` and the subsystem marker `[scope-refinement]`.
  [source: foolery/src/lib/__tests__/scope-refinement-worker.test.ts:266]

### 2.6 Queue Semantics
- The queue MUST be FIFO.
- Jobs MUST be dequeued in enqueue order.
- The queue MUST support `clear`, `size`, `peek`, and `dequeue` operations.
- The queue MUST support `onEnqueue` listeners that are called when a job is enqueued.
- Listener unsubscribe MUST be supported and effective.
  [source: foolery/src/lib/__tests__/scope-refinement-queue.test.ts:16]

### 2.7 Status Endpoint
- `GET /scope-refinement/status` MUST start the worker if not already running.
- The response MUST contain `queueSize`, `completions`, and `worker` health.
- Completions MUST include `beatId`, `beatTitle`, `repoPath`, a generated `id`, and a `timestamp` greater than zero.
  [source: foolery/src/lib/__tests__/scope-refinement-status-endpoint.test.ts:36]

### 2.8 Prompt Interpolation
- The default scope-refinement prompt MUST interpolate placeholders `{{title}}`, `{{description}}`, and `{{acceptance}}` with the beat's current values.
- Missing values MUST be replaced with the literal string `(none provided)` rather than left blank.
  [source: foolery/src/lib/__tests__/scope-refinement-defaults.test.ts:8]

---

## 3. History and Debug Prompts

### 3.1 Session Entry Summarization
- The history summarizer MUST handle entry kinds: `session_start`, `prompt`, `response`, `session_end`.
- Prompt entries MUST render with their number, source (e.g., `initial`), and workflow state.
- Response entries MUST render the parsed assistant message text and tool uses.
- Tool uses MUST be summarized as `tool:<name> <input_summary>` where `<input_summary>` is derived from the tool's input fields.
- Session end MUST render status and exit code.
- If a session has no entries, the summary MUST return the literal string `No session entries were recorded.`.
  [source: foolery/src/lib/__tests__/history-debug-prompt.test.ts:26]

### 3.2 Debug Prompt Construction
- `buildDebugPrompt` MUST produce a prompt containing:
  - An investigation preamble.
  - An `Expected Outcome` section.
  - An `Actual Outcome` section.
  - `Session Metadata` with session ID, agent name/model/version, and repo path.
  - A `Session Transcript Summary` with rendered entries.
  - A directive to offer `2-4 concrete next-step options`.
  - A hard restriction: `Do not implement fixes or mutate knots in this response.`
  [source: foolery/src/lib/__tests__/history-debug-prompt.test.ts:85]

### 3.3 History Response Visibility
- With detail mode OFF:
  - `assistant` responses MUST be visible.
  - `result`, `user`, `system`, and `stream_event` responses MUST be hidden.
  - Approval-request responses (`mcpServer/elicitation/request`) MUST be visible regardless of detail setting.
- With detail mode ON, all response types MUST be visible.
  [source: foolery/src/lib/__tests__/history-response-visibility.test.ts:5]

---

## 4. Tool Input Summarization

- The summarizer MUST recognize common tool input keys in priority order:
  1. `command` (bash-style)
  2. `filePath` (camelCase, OpenCode)
  3. `file_path` (snake_case, Claude)
  4. `pattern` (glob/grep)
- For unrecognized shapes, the summarizer MUST fall back to compact JSON that includes the actual argument values, not just the tool name.
- For empty, `null`, or `undefined` input, the summarizer MUST return an empty string.
- Output MUST be clipped to a configurable maximum length, appending `...` when truncated.
  [source: foolery/src/lib/__tests__/tool-input-summary.test.ts:5]

---

## 5. Plans Routes

### 5.1 Create and List
- `POST /plans` MUST accept `repoPath`, `beatIds`, `objective`, `mode`, `model`, and `replacesPlanId`.
- `beatIds` is required; missing it MUST yield `400`.
- On success, the endpoint MUST return `201` with the full persisted plan record, including `artifact`, `plan`, `progress`, `lineage`, and `skillPrompt`.
- `GET /plans?repoPath=...` MUST list plans for that repo; missing `repoPath` MUST yield `400`.
  [source: foolery/src/lib/__tests__/plans-route.test.ts:66]

### 5.2 Read
- `GET /plans/:planId` MUST return the full record for the plan.
- It MUST pass an optional `repoPath` query parameter through to the backend.
- If the plan does not exist, the endpoint MUST return `404`.
  [source: foolery/src/lib/__tests__/plans-route.test.ts:163]

### 5.3 Complete
- `POST /plans/:planId/complete` MUST force the plan to its terminal state.
- It requires `repoPath` in the body; missing it MUST yield `400`.
- If the plan is not found, the endpoint MUST return `404`.
- If the plan is already complete, the endpoint MUST return `409 Conflict`.
- On success, the endpoint MUST return the refreshed record with `artifact.state` set to the terminal state.
  [source: foolery/src/lib/__tests__/plans-route.test.ts:229]

---

## 6. Beat Navigation and Display

### 6.1 Beat Label Display
- `displayBeatLabel` MUST prefer the first human-friendly alias if present.
- If aliases are missing or empty, it MUST fall back to the stripped beat ID (the segment after the last hyphen).
- Hierarchical aliases with project prefixes and dots MUST have the project prefix stripped (e.g., `knots-562b.1` becomes `562b.1`).
- Aliases MUST be trimmed of surrounding whitespace before use.
  [source: foolery/src/lib/__tests__/beat-display.test.ts:10]

### 6.2 Navigation Utilities
- `stripBeatPrefix` MUST remove everything up to and including the last hyphen; if no hyphen exists, return the original string.
- `extractBeatPrefix` MUST return the segment before the last hyphen, or `null` if there is no hyphen or no local segment after the hyphen.
- `buildBeatFocusHref` MUST preserve existing query parameters while adding/updating `beat`, and MUST support overriding or clearing `repo` and setting `detailRepo`.
- `findRepoForBeatId` MUST match the beat ID prefix against repo names; it MUST prefer the longest matching prefix. If no name matches, it MUST match against the basename of the repo path. Matching MUST be case-insensitive.
- `resolveBeatRepoPath` MUST prefer an explicit `repoPath` parameter, falling back to prefix-based repo resolution.
  [source: foolery/src/lib/__tests__/beat-navigation.test.ts:10]

### 6.3 Hierarchy
- `buildHierarchy` MUST assign `_depth = 0` to top-level beats and increment depth for each nesting level.
- Beats with a missing or nonexistent parent MUST be treated as top-level.
- Children MUST appear immediately after their parent in the flattened output.
- A custom comparator MUST sort siblings within their parent's subtree ONLY; children MUST NEVER escape their parent subtree.
- The output MUST include `_hasChildren` boolean flags for UI rendering.
- Top-level beats can be sorted by a different comparator than nested siblings.
  [source: foolery/src/lib/__tests__/beat-hierarchy.test.ts:23]

---

## 7. Beat Column and State Transition Contracts

### 7.1 Valid Next States
- `validNextStates` MUST derive allowed transitions exclusively from the loom-defined workflow descriptor.
- It MUST NOT fabricate rollback transitions to earlier queue states.
- From a queue state, it MUST offer the loom-defined active transition plus wildcard targets (e.g., `deferred`, `abandoned`).
- From an active state, it MUST offer only the loom-defined next target state plus wildcard targets.
- It MUST NOT offer a transition from an active state back to its own queue state (self-queue rollback is exception flow).
- Gate review states (`plan_review`, `implementation_review`, `shipment_review`) MUST NOT offer a return to their own `ready_for_*` queue state in the default dropdown.
- The current state itself MUST be excluded from the result.
- Short/internal state names MUST be normalized before lookup (e.g., `impl` -> `implementation`).
- Raw upstream state strings MUST be normalized (trimmed, case-insensitive) before transition computation.
  [source: foolery/src/lib/__tests__/beat-columns-valid-next-states.test.ts:72]

### 7.2 Column Definitions
- `getBeatColumns` MUST return an array of column definitions.
- When `showRepoColumn` is enabled, a `_repoName` column MUST be included.
- An `action` column MUST be added only when an `onShipBeat` callback is provided, and it MUST be hidden in the active/agent view.
- The `type` column MUST be hidden in the active view and MUST NOT be present in the default queues view.
- An `ownerType` column MUST always be present.
  [source: foolery/src/lib/__tests__/beat-columns-helpers.test.ts:7]

---

## 8. Beat Take Eligibility

- `canTakeBeat` MUST return `true` when ALL of the following hold:
  1. The beat state is a queue state (e.g., `ready_for_implementation`), OR the beat is a `gate` type that is agent-claimable.
  2. The `nextActionOwnerKind` is NOT `human`.
  3. The beat is explicitly marked as claimable (`isAgentClaimable` is `true`).
- It MUST return `false` for terminal states (`shipped`, `abandoned`, `closed`).
- It MUST return `false` for human-owned next actions and human-owned gate beats.
- It MUST return `false` for beats explicitly marked as not claimable.
  [source: foolery/src/lib/__tests__/beat-take-eligibility.test.ts:14]

---

## 9. Failure Mode Requirements (AGENTS.md Alignment)

- Every lookup for configured resources (agent, pool, backend, workflow descriptor) MUST fail loudly.
- On missing configuration:
  1. Return an error that halts the operation.
  2. Log an error containing the greppable marker `FOOLERY DISPATCH FAILURE`.
  3. Name the missing thing and the exact config that fixes it.
- NEVER return a fallback like `Object.values(x)[0]` or coalesce with `?? "default"`.
- Scope-refinement worker failures (agent crash, timeout, parse error) MUST be retried via re-enqueue rather than silently dropped.
- Backend update failures during refinement MUST NOT record a completion event.
  [source: foolery/AGENTS.md:51]
