# Foolery Dispatch & Approval Behavioral Contracts (Go Port Spec)

Authoritative behavioral contract for the Go backend. Each section states the contract (WHAT), followed by invariants, edge cases, failure modes, and historical citations.

---

## 1. Dispatch Pool Resolution

### 1.1 Pool-key derivation
- The system MUST derive the dispatch pool key from the workflow descriptor's `queueActions` mapping for the current beat state.
  - Example: state `ready_to_evaluate` with `queueActions: { ready_to_evaluate: "evaluating" }` resolves pool key `"evaluating"`.
  - [source: foolery/src/lib/__tests__/dispatch-pool-resolver.test.ts:121]

### 1.2 Agent selection from pool
- The system MUST select **only** from agents listed in the resolved pool.
- Agents outside the configured pool MUST NEVER be silently dispatched, even if they appear first in the global agent registry.
  - [source: foolery/src/lib/__tests__/dispatch-pool-resolver.test.ts:131]

### 1.3 Weighted random selection
- Selection MUST respect per-entry `weight`. Over many iterations, every agent in the pool with positive weight MUST be eligible for selection.
  - [source: foolery/src/lib/__tests__/agent-pool.test.ts:58]

### 1.4 Exclusion support
- The dispatcher MUST support excluding a specific `agentId` from selection (e.g., to avoid re-dispatching a recently-failed agent).
- If exclusion leaves no eligible alternatives, selection MUST return null.
  - [source: foolery/src/lib/__tests__/agent-pool.test.ts:113]

### 1.5 Fail Loudly — unconfigured / empty pool
- If the resolved pool is empty or unconfigured, the system MUST:
  1. Throw a loud dispatch failure error.
  2. Include the greppable marker `FOOLERY DISPATCH FAILURE`.
  3. Name the missing `poolKey` and the exact config path that fixes it (`settings.pools.<poolKey>`).
  4. Emit an ANSI-red stderr banner containing the marker and the missing pool key.
- The operation MUST NOT return a fallback agent.
  - [source: foolery/src/lib/__tests__/dispatch-pool-resolver.test.ts:151]
  - [source: foolery/src/lib/__tests__/dispatch-pool-resolver.test.ts:237]
  - [source: AGENTS.md "Fail Loudly, Never Silently"]

### 1.6 Fail Loudly — broken workflow descriptor
- If the workflow descriptor lacks `queueActions` for the current state, the system MUST throw a dispatch failure with the `FOOLERY DISPATCH FAILURE` marker.
  - [source: foolery/src/lib/__tests__/dispatch-pool-resolver.test.ts:183]

### 1.7 Fail Loudly — dangling pool entry
- If a pool entry references an `agentId` that is not registered in the global agent config, the system MUST throw a dispatch failure with the marker.
  - [source: foolery/src/lib/__tests__/dispatch-pool-resolver.test.ts:198]

### 1.8 Unified path for all workflows
- Both custom workflows (e.g., gate workflows) and built-in workflows (e.g., SDLC autopilot) MUST route through the same pool-resolution path.
  - [source: foolery/src/lib/__tests__/dispatch-pool-resolver.test.ts:216]

---

## 2. Agent Pool Utilities

### 2.1 Pool selection invariants
- `selectFromPool` with an empty pool MUST return null.
- `selectFromPool` with no matching registered agents MUST return null.
- `selectFromPool` where all weights are zero MUST return null.
- `selectFromPool` MUST skip entries referencing nonexistent agents and select from the remaining eligible entries.
  - [source: foolery/src/lib/__tests__/agent-pool.test.ts:28]

### 2.2 Strict alternative selection
- `selectFromPoolStrict` MUST return an alternative agent that is **not** the excluded one.
- If no alternative exists (single-entry pool, or all other entries have zero weight), it MUST return null.
  - [source: foolery/src/lib/__tests__/agent-pool-strict.test.ts:25]

### 2.3 Canonical Identity Extraction

**Single sanctioned extractor rule:** `toCanonicalAgentConfig()` (from `agent-identity-canonical.ts`) is the ONLY function outside `agent-identity*.ts` files that may call `normalizeAgentIdentity()`. All other callers (settings, dispatch, leases) MUST route through this single canonical extractor. [source: `foolery/src/lib/agent-identity-canonical.ts:142-159`]

**Extraction fields:** `agentType`, `vendor`, `provider`, `agentName`, `leaseModel` (for Knots leases), `model` (runtime ID for transport), `flavor`, `version`. Parsed provider-specific values always win over caller-supplied fallbacks (`combineProviderResult` pattern).

### 2.4 OpenCode Path Parsing (`parseOpenCodePath`)

OpenCode model strings use slash-delimited paths [source: `foolery/src/lib/agent-identity-opencode-format.ts:230-252`]:
- **3-segment:** `router/vendor/model-version` → `{router, vendor, model, version}`
- **2-segment:** `vendor/model-version` → `{vendor, model, version}` (no router)
- **1-segment:** `bare-model` → `{model}` only

**Version splitting:** First segment containing `/(\d+(?:\.\d+)*)$/` is split at the start of the numeric run. Non-numeric prefix before numbers moves to name (e.g., `"kimi-k2.6"` → name=`"Kimi-k"`, version=`"2.6"`). Trailing segments after version are absorbed back into name unless they are pure numeric+dot runs.

**Formatting vocabulary:** `VENDOR_DISPLAY_NAMES` (27 entries, e.g., `openrouter`→`"OpenRouter"`, `moonshotai`→`"MoonshotAI"`, `z-ai`→`"Z-AI"`). `formatOpenCodeSegment` applies title-casing with suffix rules (`AI`, `ML`, `IO`, `JS` stay uppercase). [source: `foolery/src/lib/agent-identity-opencode-format.ts:27-51`]

**Version anti-leak:** For OpenCode agents, the version parsed from the model path MUST override any externally leaked binary version (e.g., `"4.7"` from runtime hints). If the model path has no numeric tail, falling back to an explicit `version` from config is allowed. [source: `foolery/src/lib/agent-identity-opencode.test.ts:178`]

### 2.5 Agent Config Normalization
- `resolvePoolAgent` MUST map a `WorkflowStep` (or target id) to the corresponding pool settings key.
- If no pool is configured for the step, it MUST return null (handled upstream by loud failure).
  - [source: foolery/src/lib/__tests__/agent-pool.test.ts:184]

### 2.4 Workflow-specific pool targets
- The system MUST prefer workflow-specific dispatch pool targets over legacy step pools.
- If a workflow-specific target pool does not exist, it MUST fall back to the legacy step pool.
  - [source: foolery/src/lib/__tests__/agent-pool-workflow-targets.test.ts:28]

### 2.5 Step-agent tracking
- The system MUST persist the last dispatched agent per `(beatId, step)` pair.
- Re-recording the same `(beatId, step)` MUST overwrite the previous value.
- Queries for untracked pairs MUST return undefined.
  - [source: foolery/src/lib/__tests__/agent-pool.test.ts:257]

### 2.6 Pool agent swapping
- `swapPoolAgent(sourceId, replacementId)` MUST produce a new pool where every occurrence of `sourceId` is replaced by `replacementId`.
- When `replacementId` already exists in the pool, weights MUST be merged (sum), and duplicate entries collapsed.
- If `sourceId` is not present, the original pool MUST be returned unchanged (identity).
- If `sourceId === replacementId`, the original pool MUST be returned unchanged.
  - [source: foolery/src/lib/__tests__/agent-pool.test.ts:304]

### 2.7 Global swap utilities
- `swapActionsAgent` MUST replace the source agent across all action mappings and return:
  - `affectedActions`: count of changed mappings.
  - `updates`: changed entries only.
  - `updatedActions`: full updated mapping object.
- If the source agent is absent or equals the replacement, the original mapping MUST be returned unchanged (identity).
  - [source: foolery/src/lib/__tests__/agent-pool.test.ts:401]

- `swapPoolsAgent` MUST do the same across all workflow step pools and return:
  - `affectedEntries`: total count of matching pool entries across all steps.
  - `affectedSteps`: count of distinct steps that contained the source agent.
  - `updates`: only steps that changed.
  - `updatedPools`: full updated pools object.
  - [source: foolery/src/lib/__tests__/agent-pool.test.ts:473]

- `countDispatchAgentOccurrences` MUST count:
  - `affectedActions`: occurrences in action mappings.
  - `affectedEntries`: total pool entries (including duplicates) referencing the agent.
  - `affectedSteps`: number of distinct steps with at least one matching entry.
  - [source: foolery/src/lib/__tests__/agent-pool.test.ts:436]

### 2.8 Swappability check
- `getSwappableSourceAgentIds(usedAgents, availableAgents)` MUST return source ids that have at least one different replacement available.
- If `usedAgents` and `availableAgents` are identical single-element sets, the result MUST be empty.
  - [source: foolery/src/lib/__tests__/agent-pool.test.ts:379]

---

## 3. Agent Identity Normalization

### 3.1 Lease identity canonicalization
- `toCanonicalLeaseIdentity` MUST produce fields: `agent_type`, `provider`, `agent_name`, `model`, `version`.
- The `label` field MUST NOT be used as `agent_name` fallback; the fallback MUST be derived from the command name (e.g., `"codex"` -> `"Codex"`).
- An explicit `agent_name` in config MUST take precedence.
  - [source: foolery/src/lib/__tests__/agent-identity.test.ts:9]

### 3.2 Provider-specific normalization
- **Claude**: `model` field equal to provider (`"Claude"`) MUST be dropped from display; flavor (e.g., `"Opus"`) kept.
  - [source: foolery/src/lib/__tests__/agent-identity.test.ts:38]
- **Codex**: model prefix `"gpt"` stays; flavor stays even if it duplicates provider name (e.g., `"Codex Spark"`).
  - [source: foolery/src/lib/__tests__/agent-identity.test.ts:56]
- **Copilot**: provider MUST stay `"Copilot"`; inner family becomes model + flavor (e.g., `"Claude Sonnet"`).
  - [source: foolery/src/lib/__tests__/agent-identity.test.ts:170]
- **Gemini**: provider stays `"Gemini"`; flavor extracted after version (e.g., `"Pro"`, `"Flash"`).
  - [source: foolery/src/lib/__tests__/agent-identity-matrix.test.ts:208]

### 3.3 OpenCode path parsing
- OpenCode model strings use slash-delimited paths. The system MUST support:
  - 3-segment: `router/vendor/model-version` (e.g., `openrouter/moonshotai/kimi-k2.6`)
  - 2-segment: `vendor/model-version`
  - bare: `model-version`
- Each path segment MUST be formatted via a known vocabulary table for canonical casing (e.g., `openrouter` -> `OpenRouter`, `moonshotai` -> `MoonshotAI`, `z-ai` -> `Z-AI`).
- Trailing numeric run MUST be extracted as `version`; non-numeric tail appended to model name.
- Empty/whitespace model MUST yield undefined model/flavor/version.
- Malformed input with stray slashes MUST collapse empty tokens and still produce canonical output.
  - [source: foolery/src/lib/__tests__/agent-identity-opencode.test.ts:13]
  - [source: foolery/src/lib/__tests__/agent-identity-opencode-format.test.ts:8]

### 3.4 Version anti-leak for OpenCode
- For OpenCode agents, the version parsed from the model path MUST override any externally leaked binary version (e.g., `"4.7"` from runtime hints).
- If the model path has no numeric tail, falling back to an explicit `version` from config is allowed.
  - [source: foolery/src/lib/__tests__/agent-identity-opencode.test.ts:178]

### 3.5 Claude suffix handling
- Suffixes like `1m` or `fast` MUST be absorbed into the flavor, not the version.
  - Example: `claude-opus-4-7-1m` -> version `"4.7"`, flavor `"Opus (1M context)"`.
  - [source: foolery/src/lib/__tests__/agent-identity.test.ts:141]

### 3.6 Display label formatting
- `formatAgentDisplayLabel` MUST join provider + model + flavor + version into a single human-readable string.
- If no label is provided, it MUST synthesize from normalized parts.
- Pills (badges) MUST include `"cli"` for CLI agents and provider-specific pills (e.g., `"copilot"`, `"openrouter"`) where applicable.
  - [source: foolery/src/lib/__tests__/agent-identity-display-parts.test.ts:8]

---

## 4. Agent Config Normalization Pipeline

### 4.1 Canonicalization Flow
At every write boundary (registration, scan, detect, auto-migration), the system runs normalization via `normalizeRegisteredAgentConfig()` [source: `foolery/src/lib/agent-config-normalization.ts:119-184`]:
1. Clean command string (trim whitespace)
2. Canonicalize runtime model: Claude dot→dash normalization (`claude-opus-4.6` → `claude-opus-4-6`)
3. Call `toCanonicalAgentConfig()` — the single sanctioned extractor
4. Compute display label via `formatAgentDisplayLabel()` (pure formatter, not re-derivation)
5. Merge result with canonical fields + command + approvalMode + label

### 4.2 Orphan Pruning
After normalization, the system prunes: [source: `foolery/src/lib/agent-config-normalization.ts:249-286`]
- Action refs pointing to unregistered agents
- Pool entries referencing unregistered agents
This prevents dangling references from surviving config writes.

### 4.3 Auto-Migration
On settings load, if any agent is non-canonical (detected by `changedPaths`), the canonical form is written back to disk via `persistMigratedSettings()`. [source: `foolery/src/lib/settings.ts:77-188`]

### 4.4 Step-Agent Tracking
- The system MUST persist the last dispatched agent per `(beatId, step)` pair in process-local memory (`Map<string, string>`, keyed by `${beatId}:${step}`). [source: `foolery/src/lib/agent-pool.ts:449-468`]
- This tracking is used for cross-agent review exclusion — the prior action step agent is excluded from review pools.
- Tracking does NOT survive process restarts. It is a soft hint for rotation, not a persistence contract.

---

## 5. Wave Slugs

### 4.1 Slug normalization
- `normalizeWaveSlugCandidate` MUST:
  - Trim whitespace.
  - Lowercase.
  - Collapse multiple special characters/underscores to single hyphens.
  - Strip leading/trailing hyphens.
  - Preserve purely numeric input as-is.
  - [source: foolery/src/lib/__tests__/wave-slugs-extended.test.ts:19]

### 4.2 Slug extraction from labels
- `extractWaveSlug(labels)` MUST return the slug portion of the first label matching `orchestration:wave:<slug>`.
- If labels is empty, or the slug part is empty/whitespace, it MUST return null.
- It MUST return the first valid slug when multiple exist.
  - [source: foolery/src/lib/__tests__/wave-slugs.test.ts:18]
  - [source: foolery/src/lib/__tests__/wave-slugs-extended.test.ts:109]

### 4.3 Legacy numeric slug detection
- `isLegacyNumericWaveSlug` MUST return true for strings consisting solely of digits (e.g., `"1"`, `"022"`), false otherwise, including for null/undefined/empty.
  - [source: foolery/src/lib/__tests__/wave-slugs.test.ts:23]

### 4.4 Slug allocation
- `allocateWaveSlug(usedSet, preferred?)` MUST:
  - Normalize the preferred candidate and return it if not already in `usedSet`.
  - If preferred is taken, missing, empty, or whitespace-only, generate a unique composed candidate.
  - Generated candidates MUST be unique; if a composed collision occurs, try further candidates.
  - If all composed candidates are exhausted, fall back to a suffixed variant (`base-N` with `N >= 2`).
  - Every returned slug MUST be added to `usedSet`.
  - [source: foolery/src/lib/__tests__/wave-slugs.test.ts:30]
  - [source: foolery/src/lib/__tests__/wave-slugs-allocate.test.ts:4]
  - [source: foolery/src/lib/__tests__/wave-slugs-fallback.test.ts:22]

### 4.5 Wave title construction
- `buildWaveTitle(slug, name)` MUST produce `"Scene <slug>: <name>"`.
- If `name` is empty or whitespace, it MUST produce `"Scene <slug>"`.
- `rewriteWaveTitleSlug(oldTitle, newSlug)` MUST:
  - Replace an existing `"Wave N"` or `"Scene <old-slug>"` prefix with `"Scene <new-slug>"`.
  - If no prefix exists, prepend `"Scene <newSlug>: "`.
  - Be case-insensitive regarding `"Wave"` / `"Scene"`.
  - [source: foolery/src/lib/__tests__/wave-slugs.test.ts:43]
  - [source: foolery/src/lib/__tests__/wave-slugs-extended.test.ts:175]

### 4.6 Label predicates
- `isWaveLabel` MUST match exact `orchestration:wave` and any `orchestration:wave:<slug>`.
- `isInternalLabel` MUST match wave labels and `stage:*` labels; MUST return false for plain user labels.
- `isReadOnlyLabel` MUST match `attempts:<number>` only.
- `isWaveSlugLabel` MUST match `orchestration:wave:<non-empty-slug>` only (not the bare wave label).
- `getWaveSlugLabels` MUST filter to only slug labels and return them in order.
  - [source: foolery/src/lib/__tests__/wave-slugs-extended.test.ts:37]

---

## 6. Waves Route Aliases

### 5.1 Blocker alias propagation
- The waves API MUST include blocker beat `aliases` on blocked beats so the UI can report `readinessReason: "Waiting on <alias>"`.
  - [source: foolery/src/lib/__tests__/waves-route-aliases.test.ts:98]

### 5.2 Alias passthrough
- Alias arrays from the backend MUST be forwarded unchanged on wave beat DTOs.
  - [source: foolery/src/lib/__tests__/waves-route-aliases.test.ts:117]

### 5.3 Gate readiness classification
- Agent-owned gate beats (`nextActionOwnerKind: "agent"`, `isAgentClaimable: true`) MUST have `readiness: "runnable"`.
- Human-owned gate beats (`nextActionOwnerKind: "human"`, `requiresHumanAction: true`) MUST have `readiness: "humanAction"` with reason `"Awaiting human approval for this gate."`.
- Agent-owned gates MUST NOT appear as the wave-level `gate` property.
  - [source: foolery/src/lib/__tests__/waves-route-aliases.test.ts:127]
  - [source: foolery/src/lib/__tests__/waves-route-aliases.test.ts:152]

---

## 7. Terminal Launch Args (Claude Approval Bridge)

### 6.1 Claude bypass default
- For Claude agents in default mode (`approvalMode` absent or `"bypass"`), both interactive and prompt-mode launch args MUST include `--dangerously-skip-permissions`.
  - [source: foolery/src/lib/__tests__/claude-approval-launch-args.test.ts:33]

### 6.2 Claude prompt mode with approval enabled
- When `approvalMode: "prompt"`, prompt-mode args MUST:
  - Omit `--dangerously-skip-permissions`.
  - Include `--permission-mode default`, `--setting-sources project`, `--strict-mcp-config --mcp-config`, and `--permission-prompt-tool <tool-id>`.
  - [source: foolery/src/lib/__tests__/claude-approval-launch-args.test.ts:49]

### 6.3 Claude interactive with approval enabled
- When `approvalMode: "prompt"`, interactive args MUST:
  - Omit `--dangerously-skip-permissions`.
  - Include `--permission-prompt-tool <tool-id>`.
  - Retain model flag if provided.
  - [source: foolery/src/lib/__tests__/claude-approval-launch-args.test.ts:86]

### 6.4 One-shot take CLI args forbidden
- Building one-shot CLI argument lists for take loops MUST throw a dispatch failure containing the greppable `TERMINAL_DISPATCH_FAILURE_MARKER` if the agent does not support one-shot args (e.g., OpenCode, Codex app-server).
  - [source: foolery/src/lib/__tests__/claude-approval-launch-args.test.ts:224]
  - [source: foolery/src/lib/__tests__/claude-approval-launch-args.test.ts:238]

### 6.5 Agent-specific server commands
- OpenCode take sessions MUST use `serve --port 0 --print-logs`.
- Codex take sessions MUST use `app-server --listen stdio:// -c model="..."`.
  - [source: foolery/src/lib/__tests__/claude-approval-launch-args.test.ts:187]

---

## 8. Approval Visibility & Extraction

### 7.1 Multi-adapter extraction
- `extractApprovalRequest` MUST recognize approval payloads from:
  - **Claude**: `AskUserQuestion` tool_use with `questions[].question` and `options`.
  - **Codex**: `mcpServer/elicitation/request` and `item/commandExecution/requestApproval`.
  - **Copilot**: `user_input.requested` with `question` and `choices`.
  - **Gemini**: `session/request_permission` with `message` and `options`.
  - **OpenCode**: `permission.asked` and `permission.updated` (including nested `part` wrappers).
  - [source: foolery/src/lib/__tests__/approval-request-visibility.test.ts:12]

### 7.2 Extracted fields
- For every adapter, the system MUST extract at minimum:
  - `adapter`, `source`, `requestId`, `supportedActions`, `replyTarget`.
- For tool/server permission requests, it MUST also extract `serverName`, `toolName`, `toolParamsDisplay`, `parameterSummary`, `patterns`, `permissionName`, `nativeSessionId`, `sessionId`.
  - [source: foolery/src/lib/__tests__/approval-request-visibility.test.ts:153]

### 7.3 Banner formatting
- `formatApprovalRequestBanner` MUST:
  - Include the stable marker `FOOLERY APPROVAL REQUIRED`.
  - Include greppable key=value pairs for all extraction fields.
  - Preserve adapter-specific routing fields (e.g., `sessionId`, `requestId`, `toolUseId`, `patterns`).
  - [source: foolery/src/lib/__tests__/approval-request-visibility.test.ts:283]

### 7.4 Runtime visibility
- Approval requests detected in the agent stdout/stderr stream MUST be pushed to the session event bus as visible stderr/stdout banners containing the `FOOLERY APPROVAL REQUIRED` marker.
  - Claude / Copilot appearances go to stdout.
  - Codex / Gemini / OpenCode appearances go to stderr.
  - [source: foolery/src/lib/__tests__/approval-request-runtime.test.ts:106]

### 7.5 OpenCode auto-reply (Gemini ACP)
- For Gemini adapter, detecting a permission request MUST trigger an automatic `allow_once` reply written to the child stdin.
  - [source: foolery/src/lib/__tests__/approval-request-runtime.test.ts:219]

### 7.6 OpenCode `onApprovalRequest` callback
- For OpenCode adapter, the runtime MUST invoke `onApprovalRequest` with the extracted request object so the system can route it to the approvals UI.
  - [source: foolery/src/lib/__tests__/approval-request-runtime.test.ts:262]

---

## 9. Approval Registry

### 8.1 Registration DTO shape
- `registerApproval` MUST expose a canonical DTO with fields:
  - `id`, `notificationKey`, `status`, `createdAt`, `updatedAt`, `repoPath`, `beatId`, `sessionId`, `adapter`, `source`, `toolName`, `replyTarget`, `supportedActions`, `actionable`, `agent` (name, provider, model, version).
  - [source: foolery/src/lib/__tests__/approval-registry.test.ts:79]

### 8.2 Listing filters
- `listApprovals` MUST support:
  - `repoPath`: exact match.
  - `activeOnly`: include only actionable pending records.
  - `status`: array of statuses (including CSV/repeated style).
  - `updatedSince`: cursor filter on `updatedAt`.
- Default ordering MUST be `updatedAt DESC`, then `id ASC`.
  - [source: foolery/src/lib/__tests__/approval-registry.test.ts:124]
  - [source: foolery/src/lib/__tests__/approvals-route.test.ts:90]

### 8.3 Session detach invariants
- `detachSession(sessionId, reason)` MUST:
  - Keep the approval record visible (do NOT delete it).
  - Flip status to `"manual_required"`.
  - Set `actionable` to `false`.
  - Set `actionableReason` to `"approval_responder_unavailable"`.
  - Null out the responder so subsequent actions cannot be applied automatically.
  - [source: foolery/src/lib/__tests__/approval-registry.test.ts:190]

### 8.4 Re-attach responder
- `attachResponderForSession(sessionId, responder)` MUST make an orphaned record actionable again, allowing future `applyApprovalAction` calls to succeed.
  - [source: foolery/src/lib/__tests__/approval-registry.test.ts:262]

### 8.5 Action application results
- `applyApprovalAction(approvalId, action)` MUST:
  - Return success (`ok: true`) and update the record status when the responder succeeds.
  - Return `404` for unknown approval.
  - Return `409` with code `approval_responder_unavailable` after detach.
  - Return `502` when the responder fails non-fatally (e.g., network error) and include the upstream reason in the error.
  - Reject invalid actions with `400`.
  - [source: foolery/src/lib/__tests__/approval-registry.test.ts:211]
  - [source: foolery/src/lib/__tests__/approvals-actions-route.test.ts:106]

---

## 10. Approval Repo Scope

### 9.1 Cross-repo annotation
- `annotateApprovalsForRepo(approvals, activeRepoPath)` MUST:
  - Set `isCrossRepo: true` when `approval.repoPath != activeRepoPath`.
  - Treat approvals with no `repoPath` as local (`isCrossRepo: false`).
  - Sort active-repo approvals before cross-repo ones; for ties, `updatedAt DESC` or equivalent recency ordering.
  - [source: foolery/src/lib/__tests__/approval-repo-scope.test.ts:29]

### 9.2 Active-repo filtering
- `selectActiveRepoApprovals` MUST hide cross-repo approvals when `activeRepo` is set.
- When `activeRepo` is empty/null, it MUST return all approvals.
  - [source: foolery/src/lib/__tests__/approval-repo-scope.test.ts:67]

---

## 11. Approval Escalations

### 10.1 Banner parsing
- `parseApprovalBanner` MUST parse the multi-line banner back into an ApprovalRequest-like object, restoring all key=value fields.
  - [source: foolery/src/lib/__tests__/approval-escalations.test.ts:26]

### 10.2 Escalation identity stability
- `approvalEscalationFromBanner` and `approvalEscalationFromRequest` MUST generate stable `id` and `notificationKey` values based on the logical content of the request (session, beat, tool, patterns), NOT on ephemeral ids like `permissionId`.
- Two events that differ only in `permissionId` MUST collapse to the same escalation identity.
  - [source: foolery/src/lib/__tests__/approval-escalations.test.ts:42]
  - [source: foolery/src/lib/__tests__/approval-escalations-detail.test.ts:78]

### 10.3 Logical key builder
- `buildApprovalLogicalKey` MUST exclude ephemeral identifiers (`permissionId`, `requestId`) from the key so that OpenCode permission rotations coalesce.
  - [source: foolery/src/lib/__tests__/approval-escalations-detail.test.ts:78]

### 10.4 Escalation hydration
- `approvalEscalationFromPendingRecord` MUST reconstitute an `ApprovalEscalation` from a persisted `PendingApprovalRecord`, preserving all fields including `failureReason`.
  - [source: foolery/src/lib/__tests__/approval-escalations.test.ts:93]

### 10.5 Href construction
- `buildApprovalsHref(repoPath)` MUST return a URL with `view=finalcut&tab=approvals&repo=<encoded>`.
- `buildApprovalConsoleHref({beatId, repoPath})` MUST return a URL with `view=history&beat=<id>&repo=<encoded>&detailRepo=<encoded>`.
  - [source: foolery/src/lib/__tests__/approval-escalations.test.ts:61]

### 10.6 Logging marker
- `logApprovalEscalation` MUST emit a JSON log line containing the marker `FOOLERY APPROVAL ESCALATION`, plus `eventName`, `approvalId`, and `sessionId`.
  - [source: foolery/src/lib/__tests__/approval-escalations.test.ts:73]

### 10.7 Failure reason explanation
- `explainApprovalFailureReason` MUST map known error strings to human-friendly hints:
  - `missing_reply_target` / `missing_opencode_reply_target` -> "no longer connected"
  - `opencode_http_404` / `opencode_http_410` -> "no longer recognises"
  - `opencode_http_502` -> "server error"
  - `opencode_returned_false` -> "rejected the reply"
  - `"The user aborted a request."` -> "did not respond"
  - `"fetch failed"` / `ECONNREFUSED ...` -> "Could not reach"
  - `unsupported_adapter:*` -> includes the adapter name
- Unknown or empty reasons MUST return null.
  - [source: foolery/src/lib/__tests__/approval-escalations.test.ts:132]

### 10.8 Detail text formatting
- `formatApprovalDetailText` MUST prefer the most specific human-readable detail in this priority:
  1. `message`
  2. `question`
  3. `toolParamsDisplay`
  4. `parameterSummary` (treat raw `{}` as absent)
  5. `patterns[0]`
  6. `permissionName` + `toolUseId`
- It MUST NOT emit generic placeholders like `"Manual approval is required."` when more specific data exists.
  - [source: foolery/src/lib/__tests__/approval-escalations-detail.test.ts:11]

---

## 12. Approval Escalation Store

### 11.1 Upsert semantics
- `upsertPendingApproval` MUST return `true` when the approval is newly inserted, and `false` when it matches an existing `notificationKey` / logical identity and is therefore coalesced (updated in place).
- After coalescence, the store MUST contain only one record, and its mutable fields (e.g., `replyTarget.permissionId`) MUST reflect the latest event.
  - [source: foolery/src/stores/__tests__/approval-escalation-store.test.ts:40]
  - [source: foolery/src/stores/__tests__/approval-escalation-store-coalesce.test.ts:37]

### 11.2 Lifecycle states
- The store MUST support these explicit state transitions:
  - `markManualAction(id)` -> status `"manual_required"` (still visible in pending selector).
  - `markApprovalResponding(id, action)` -> status `"responding"`, clear `failureReason`.
  - `markApprovalResolved(id, action)` -> status `"approved"` / `"always_approved"` / `"rejected"`, remove from pending count, clear `failureReason`.
  - `markApprovalFailed(id, reason)` -> status `"reply_failed"`, set `failureReason`.
  - `markApprovalUnsupported(id, reason)` -> set `failureReason` to the reason code.
  - `dismissApproval(id)` -> remove from pending selectors entirely.
  - [source: foolery/src/stores/__tests__/approval-escalation-store.test.ts:67]

### 11.3 Persistence across terminal lifecycle
- Removing a terminal session MUST NOT cascade-delete its approvals from this store.
- Approvals MUST survive until the user explicitly resolves, dismisses, or marks them manual.
  - [source: foolery/src/stores/__tests__/approval-escalation-store-persistence.test.ts:29]

### 11.4 Pending selectors
- `selectPendingApprovals` MUST include statuses that still need user attention (e.g., `pending`, `manual_required`, `reply_failed`), excluding `approved` / `rejected` / `dismissed`.
- `selectPendingApprovalCount` MUST reflect the same filter.
  - [source: foolery/src/stores/__tests__/approval-escalation-store.test.ts:40]

---

## 13. Scope Refinement Pending Store

### 12.1 Beat tracking
- `markPending(beatId)` MUST add the beat id to a pending set.
- It MUST be idempotent: calling twice with the same id MUST not change state.
- `markComplete(beatId)` MUST remove the beat id.
- `markComplete` on an unknown id MUST be a no-op.
- `selectIsPending(beatId)` MUST return true only if the beat id is in the set.
  - [source: foolery/src/stores/__tests__/scope-refinement-pending-store.test.ts:7]

---

## 14. App Update Route

### 13.1 GET status
- `GET /api/app-update` MUST return the persisted update status (`phase`, `message`, `error`, `startedAt`, `endedAt`, `workerPid`, `launcherPath`, `fallbackCommand`).
  - [source: foolery/src/lib/__tests__/app-update-route.test.ts:41]

### 13.2 POST restrictions
- POST MUST be rejected with `403` if the request origin is not in an allowed local set.
- Allowed origins MUST include `http://localhost` (and loopback equivalents depending on config).
- On rejection, it MUST log `Rejected update request from origin <origin>.`
  - [source: foolery/src/lib/__tests__/app-update-route.test.ts:53]

### 13.3 Update start
- If allowed and no update is in progress, POST MUST return `202` with the status payload.
- If an update is already in progress (`phase != idle/completed/failed`), POST MUST return `409`.
  - [source: foolery/src/lib/__tests__/app-update-route.test.ts:69]
  - [source: foolery/src/lib/__tests__/app-update-route.test.ts:88]

### 13.4 Failure handling
- If `startAppUpdate` throws, the endpoint MUST return `500` with the error message, but the JSON body MUST still include the persisted failure status (e.g., `phase: "failed"`).
  - [source: foolery/src/lib/__tests__/app-update-route.test.ts:105]

---

## 15. Dispatch Forensics

### 14.1 Snapshot capture
- `captureBeatSnapshot(boundary, ctx, deps)` MUST:
  - Persist a structured snapshot containing: boundary name, timestamp, beat state, active leases, session/beat/lease identifiers.
  - Emit an audit event (`beat_snapshot_<boundary>`) with the snapshot path.
  - Record `captureErrors` (e.g., `showKnot` failure) rather than aborting the write.
  - Never throw even if the snapshot writer throws; swallow the error and continue so the turn does not hang.
  - [source: foolery/src/lib/__tests__/dispatch-forensics.test.ts:100]

### 14.2 Snapshot path format
- `snapshotPath` MUST include:
  - Root log directory.
  - `_dispatch_forensics` slug.
  - Date (`YYYY-MM-DD`).
  - `sessionId` segment.
  - Filename containing boundary, beat id, captured-at timestamp, and `.json` extension.
  - [source: foolery/src/lib/__tests__/dispatch-forensics.test.ts:176]

### 14.3 Failure classification
- `classifyTurnFailure(preSnapshot, postSnapshot, signals)` MUST detect categories in this priority:
  1. **concurrent_claim_detected**: pre state is queue-ready, post state moved to action, and a lease OTHER than ours claimed it (new step with different `lease_id`).
  2. **our_agent_double_claim_suspected**: post state shows 2+ new action steps all with our `lease_id`.
  3. **kno_half_transition_suspected**: post state moved to action with exactly 1 new step with our `lease_id`, BUT `agentClaimExitedNonZero` is true (claim command exited non-zero despite state change).
  4. **lease_terminated_unexpectedly**: our lease went to `lease_terminated` and `foolerInitiatedLeaseTerminate` is false.
  5. **unknown_state_change**: something changed but no specific rule matched.
- If nothing changed between pre and post, it MUST return null.
- If the lease termination was initiated by Foolery (`foolerInitiatedLeaseTerminate: true`), it MUST return null (not an anomaly).
  - [source: foolery/src/lib/__tests__/dispatch-forensics.test.ts:229]

### 14.4 Forensic banner
- `buildForensicBannerBody` MUST include:
  - Greppable marker `FOOLERY DISPATCH FORENSIC`.
  - Category name, beat id, session id, lease id, iteration number.
  - Paths to pre and post snapshot files.
  - Human-readable reasoning.
  - [source: foolery/src/lib/__tests__/dispatch-forensics.test.ts:356]

### 14.5 Post-turn forensics orchestration
- `runPostTurnForensics` MUST:
  - If a failure is classified, emit the banner to the session buffer AND log an audit event (`dispatch_forensic_classified`) with category and conflicting lease id.
  - If nothing changed, return `classified: false` and emit no banner.
  - [source: foolery/src/lib/__tests__/dispatch-forensics.test.ts:381]

---

## 16. Hermetic Test Rules (Applies to All Go Port Tests)

- Default tests MUST NOT touch the host environment:
  - No real `os.Getenv` / `process.env`.
  - No real filesystem writes (`tmpdir`, `mkdtemp`, real cwd reads).
  - No real process execution (`exec.Command`, `spawn`, `bash -c`).
  - No real network or ports.
  - No host binaries (`git`, `bd`, `kno`, `node`, `go` toolchain calls in unit tests).
  - No wall-clock timers; inject `func() time.Time` or `Clock` interfaces.
- If a function depends on any of these, push the dependency up the stack and accept an interface so tests target the deep, deterministic logic.
- Integration tests that genuinely exercise the environment MUST be tagged (e.g., `//go:build integration`) and run only manually, excluded from default `go test ./...`.
  - [source: AGENTS.md "Hermetic Test Policy"]
