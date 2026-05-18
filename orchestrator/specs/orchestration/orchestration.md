# Orchestration Behavioral Contract Specification — Go Port
> Authoritative behavioral contract for the Go backend. This document states WHAT the Go backend must do. Historical provenance: contracts originally inferred from the TypeScript Foolery test suite.

---

## 1. Workflow Model & State Semantics

### 1.1 Workflow Step Resolution
- The system MUST map each queue state to its corresponding active step and `Queued` phase.
  - Example: `ready_for_implementation` maps to `Implementation` step, `Queued` phase.
  [source: foolery/src/lib/__tests__/workflow-step.test.ts:17]
- The system MUST map each active state to its corresponding step and `Active` phase.
  - Example: `implementation` maps to `Implementation` step, `Active` phase.
  [source: foolery/src/lib/__tests__/workflow-step.test.ts:44]
- The system MUST return null for terminal states, deferred, and unknown states when resolving steps.
  [source: foolery/src/lib/__tests__/workflow-step.test.ts:71]
- Every `WorkflowStep` value MUST round-trip through both `Queued` and `Active` phases.
  [source: foolery/src/lib/__tests__/workflow-step.test.ts:97]

### 1.2 Queue/Terminal Classification
- The system MUST classify queue states, terminal states, and deferred as "queue or terminal".
  [source: foolery/src/lib/__tests__/workflow-step.test.ts:115]
- The system MUST classify all active states as NOT "queue or terminal".
  [source: foolery/src/lib/__tests__/workflow-step.test.ts:134]
- Unknown or empty states MUST be treated as NOT action states (i.e., queue-or-terminal).
  [source: foolery/src/lib/__tests__/workflow-step.test.ts:143]

### 1.3 Review vs Action Steps
- The system MUST identify review steps correctly (`plan_review`, `implementation_review`, `shipment_review`).
  [source: foolery/src/lib/__tests__/workflow-step.test.ts:149]
- Review steps MUST map back to their prior action step.
  [source: foolery/src/lib/__tests__/workflow-step.test.ts:163]

### 1.4 State Priority Ordering
- The system MUST sort known workflow states by pipeline order.
  [source: foolery/src/lib/__tests__/workflow-step.test.ts:183]
- Unknown states MUST be sorted alphabetically after all known states.
  [source: foolery/src/lib/__tests__/workflow-step.test.ts:202]

### 1.5 Forward Transition Target
- The system MUST compute the forward transition target from any state.
  - Rollback transitions MUST be excluded from forward targets.
  [source: foolery/src/lib/__tests__/workflow-transition-helpers.test.ts:77]
- Terminal states with no outgoing transitions MUST return null.
  [source: foolery/src/lib/__tests__/workflow-transition-helpers.test.ts:96]
- Unknown states and workflows without transitions MUST return null.
  [source: foolery/src/lib/__tests__/workflow-transition-helpers.test.ts:100]

### 1.6 Workflow Descriptor Derivation
- The system MUST infer workflow mode from hints:
  - `coarse_human_gated` for semiauto, coarse, human-gated, or PR hints.
  - `granular_autonomous` otherwise.
  [source: foolery/src/lib/__tests__/workflows-coverage-inference.test.ts:24]
- The system MUST infer final cut state with a priority order:
  - `ready_for_plan_review` > `ready_for_implementation_review` > `ready_for_shipment_review` > `reviewing`.
  [source: foolery/src/lib/__tests__/workflows-coverage-inference.test.ts:52]
- The system MUST infer retake state with priority order:
  - `ready_for_implementation` > `retake` > `retry` > `rejected` > `refining`, falling back to `initialState`.
  [source: foolery/src/lib/__tests__/workflows-coverage-inference.test.ts:79]
- The system MUST normalize legacy profile ids (`beads-coarse`, `knots-granular`, etc.) to canonical ids (`autopilot`, `semiauto`).
  [source: foolery/src/lib/__tests__/workflows-agnostic.test.ts:69]
- The system MUST normalize legacy state names to canonical workflow states:
  - `open` / `idea` / `work_item` -> initial state.
  - `in_progress` -> first action state.
  - `impl` -> `implementation`.
  - `ready_for_review` -> `ready_for_implementation_review`.
  - `retake` / `retry` / `rejected` / `refining` / `rework` -> workflow.retakeState.
  - `closed` / `done` / `approved` -> `shipped`.
  [source: foolery/src/lib/__tests__/workflows-coverage-labels.test.ts:121]
- The system MUST preserve explicit `shipped` and `abandoned` states even when omitted from a limited workflow descriptor.
  [source: foolery/src/lib/__tests__/workflows-coverage-labels.test.ts:164]

### 1.7 Workflow Runtime State
- The system MUST derive runtime metadata for any state:
  - Queue state: `nextActionOwnerKind=agent`, `requiresHumanAction=false`, `isAgentClaimable=true`.
  - Active state: `isAgentClaimable=false`.
  - Terminal state: `nextActionOwnerKind=none`.
  [source: foolery/src/lib/__tests__/workflows-coverage-inference.test.ts:262]
- For semiauto human-owned steps, `requiresHumanAction=true` and `isAgentClaimable=false`.
  [source: foolery/src/lib/__tests__/workflows-coverage-inference.test.ts:285]
- Undefined states MUST normalize to the workflow's initial state.
  [source: foolery/src/lib/__tests__/workflows-coverage-inference.test.ts:293]

### 1.8 Beat Requires Human Action
- The system MUST return true if `beat.requiresHumanAction` is explicitly true.
- If not set, it MUST derive from the workflow descriptor.
- If the workflow is not found and no explicit flag is set, it MUST return false (fail loud only at dispatch, not here).
  [source: foolery/src/lib/__tests__/workflows-coverage-inference.test.ts:136]

### 1.9 Rollback Detection
- The system MUST detect backward transitions as rollback:
  - `plan_review -> ready_for_planning`
  - `implementation_review -> ready_for_implementation`
  - `shipment_review -> ready_for_implementation`
  - `shipment_review -> ready_for_shipment`
  [source: foolery/src/lib/__tests__/workflows-coverage-inference.test.ts:379]
- Forward, same-state, and unknown transitions MUST NOT be classified as rollback.
  [source: foolery/src/lib/__tests__/workflows-coverage-inference.test.ts:391]

---

## 2. State Transitions & Valid Next States

### 2.1 Loom-Defined Transitions Only
- The system MUST only offer transitions that exist in the loom-derived `workflow.transitions` list.
  - Exception flow (un-claiming without an explicit transition) MUST live behind a Rewind submenu with `force: true`.
  [source: foolery/src/lib/__tests__/valid-next-states.test.ts:6]
- Wildcard transitions (`from: "*"`, to: `"deferred"` / `"abandoned"`) MUST be included.
  [source: foolery/src/lib/__tests__/valid-next-states.test.ts:64]

### 2.2 Stuck/Rolled-Back Beats
- When the display state differs from the raw kno state (stuck/rolled-back), the system MUST compute valid next states from the **raw kno state**, not the display state.
  [source: foolery/src/lib/__tests__/valid-next-states.test.ts:90]
- The system MUST exclude both the display state and the raw state from the results.
  [source: foolery/src/lib/__tests__/valid-next-states.test.ts:97]
- The system MUST NOT inject non-loom escape hatches when rolled back.
  [source: foolery/src/lib/__tests__/valid-next-states.test.ts:103]
- Only wildcard terminals actually in the workflow MUST appear (e.g., `abandoned` and `deferred`, NOT `shipped` via wildcard from `planning`).
  [source: foolery/src/lib/__tests__/valid-next-states.test.ts:114]

### 2.3 Normal Flow Constraints
- The system MUST NOT offer same-step queue rollback from active rows (un-claim is exception flow).
  [source: foolery/src/lib/__tests__/valid-next-states.test.ts:148]
- The system MUST NOT offer earlier queue states as rollback targets unless explicitly defined in loom transitions.
  [source: foolery/src/lib/__tests__/valid-next-states.test.ts:155]
- From `shipment_review`, the exact loom targets are: `shipped`, `ready_for_implementation`, `ready_for_shipment`, `deferred`, `abandoned`. Earlier queue states without explicit transitions are excluded.
  [source: foolery/src/lib/__tests__/valid-next-states.test.ts:164]
- Short `impl` state MUST normalize to `implementation` for transition lookups.
  [source: foolery/src/lib/__tests__/valid-next-states.test.ts:182]
- The current state MUST be excluded from valid next states.
  [source: foolery/src/lib/__tests__/valid-next-states.test.ts:189]
- Matching rawKnoState and display state MUST be treated as normal flow.
  [source: foolery/src/lib/__tests__/valid-next-states.test.ts:194]

### 2.4 Gate States
- From gate states (`plan_review`, `implementation_review`, `shipment_review`), the system MUST NOT offer `ready_for_<gate>` as a valid next state.
  [source: foolery/src/lib/__tests__/valid-next-states.test.ts:211]

---

## 3. Orchestration Plans

### 3.1 Plan Generation
- The system MUST support explicit beat selection for plan generation and MUST NOT fall back to scene inference when `beatIds` are provided.
  [source: foolery/src/lib/__tests__/orchestration-plan-generation.test.ts:143]
- The system MUST fail before spawning if any selected beat is missing.
  [source: foolery/src/lib/__tests__/orchestration-plan-generation.test.ts:227]
- The system MUST include dependency edges and groom guidance in the prompt.
  [source: foolery/src/lib/__tests__/orchestration-plan-helpers.test.ts:27]
- The system MUST normalize explicit steps and backfill implicit step groups.
  - Steps MUST be sorted by `step_index`.
  [source: foolery/src/lib/__tests__/orchestration-plan-helpers.test.ts:61]

### 3.2 Plan Payload Normalization
- The system MUST support native execution plans that store `knot_ids` (or `beat_ids`) and map them into the unified plan document shape.
  [source: foolery/src/lib/__tests__/orchestration-plan-payload.test.ts:6]

### 3.4 Plan Session Lifecycle

**Plan Document shape [source: `foolery/src/lib/orchestration-plan-types.ts:22-32`]:**
- `repoPath`, `beatIds[]`, `objective?`, `summary`, `waves[]` (each with `agents[]`, `beats[]`, `steps[]`), `unassignedBeatIds[]`, `assumptions[]`, `mode` (agent model), `model`

**Create session [source: `foolery/src/lib/orchestration-session-create.ts:380-423`]:**
1. `collectContext(repoPath)` → `{beats, edges}` — gets all open/in_progress/blocked beats + dependency edges
2. `initSessionEntry` — creates `OrchestrationSession` + `EventEmitter`, interaction log
3. `emitPromptLog` — builds orchestration prompt with beat scope, edge count, objective
4. `wireChildProcess` — spawns agent, wires stdout (NDJSON parsing) and stderr
5. Plan extraction: parse `<orchestration_plan>` tagged JSON from result text

**Apply session [source: `foolery/src/lib/orchestration-session-apply.ts:347-426`]:**
1. Sort plan waves by `waveIndex`
2. For each wave: create an "epic" beat with `ORCHESTRATION_WAVE_LABEL`, reparent children, add sequential dependency between waves
3. `closeEmptiedSourceWaves` — close source scenes that have no active children left
4. Slug allocation: human-readable slugs from wave names, avoiding collisions

**Cascade close [source: `foolery/src/lib/cascade-close.ts:1-107`]:**
- `cascadeClose(parentId, reason?, repoPath?)` — recursively closes all open descendants leaf-first (DFS post-order), then the parent
- `getOpenDescendants` collects via DFS post-order, excludes already-closed children
- Errors collected but don't block sibling closures — returns both `closedIds[]` and `errors[]`

**Beat release optimism [source: `foolery/src/lib/beat-release-optimism.ts:3-119`]:**
- When releasing a beat: `createPendingBeatRelease(state)` maps active states to their queue counterparts (`implementation→ready_for_implementation`, etc.)
- `applyPendingBeatReleases` merges pending state changes into the beat list in-memory (optimistic UI)
- `settledPendingBeatReleaseKeys` detects when backend has caught up (removes from pending set)
- The system MUST close the plan via the backend and return the refreshed record.
  [source: foolery/src/lib/__tests__/orchestration-plan-correction.test.ts:52]
- The system MUST throw when the plan is not found.
  [source: foolery/src/lib/__tests__/orchestration-plan-correction.test.ts:70]
- The system MUST throw when the plan is already in a terminal state (`shipped` or `abandoned`).
  [source: foolery/src/lib/__tests__/orchestration-plan-correction.test.ts:79]
- The system MUST propagate backend close errors.
  [source: foolery/src/lib/__tests__/orchestration-plan-correction.test.ts:97]

---

## 4. Beat Hierarchy & Navigation

### 4.1 Hierarchy Building
- The system MUST build a hierarchy where top-level beats have `_depth=0` and `_hasChildren=false` when no parents exist.
  [source: foolery/src/lib/__tests__/beat-hierarchy.test.ts:24]
- Children MUST be nested under their parent with increasing `_depth`.
  [source: foolery/src/lib/__tests__/beat-hierarchy.test.ts:33]
- Multi-level nesting MUST be supported (depth increments per generation).
  [source: foolery/src/lib/__tests__/beat-hierarchy.test.ts:47]
- Beats with a missing parent MUST be treated as top-level (orphans promoted to root).
  [source: foolery/src/lib/__tests__/beat-hierarchy.test.ts:61]

### 4.2 Hierarchy Sorting Invariants
- Children MUST never escape their parent subtree during sorting.
  - A child with higher priority than another root must still remain under its parent.
  [source: foolery/src/lib/__tests__/beat-hierarchy.test.ts:90]
- All nested items MUST have a parent entry preceding them in the output order.
  [source: foolery/src/lib/__tests__/beat-hierarchy.test.ts:122]
- Top-level beats MUST be sorted by the provided comparator.
  [source: foolery/src/lib/__tests__/beat-hierarchy.test.ts:132]

### 4.3 Ready Ancestor Filtering

```
                    ┌──────────────────────────────┐
                    │ For each beat in candidate set│
                    └──────────────┬───────────────┘
                                   │
                                   ▼
                          ┌─────────────────┐
                          │ Has parent in   │────No───► KEEP (root beat)
                          │ beat map?       │
                          └────────┬────────┘
                                   │ Yes
                                   ▼
                          ┌─────────────────┐
                          │ Parent is in    │────No───► DROP (broken ancestor chain)
                          │ visible set?    │
                          └────────┬────────┘
                                   │ Yes
                                   ▼
                          ┌─────────────────┐
                          │ Walk full       │────No───► DROP (intermediate parent missing)
                          │ ancestor chain  │
                          │ all visible?    │
                          └────────┬────────┘
                                   │ Yes
                                   ▼
                              ┌─────────┐
                              │  KEEP   │
                              └─────────┘
```

- The system MUST keep descendants only when the full ancestor chain exists and is visible.
  [source: foolery/src/lib/__tests__/ready-ancestor-filter.test.ts:19]
- The system MUST drop descendants when an intermediate parent is missing.
  [source: foolery/src/lib/__tests__/ready-ancestor-filter.test.ts:32]
- The system MUST preserve ready siblings while hiding descendants of excluded branches.
  [source: foolery/src/lib/__tests__/ready-ancestor-filter.test.ts:43]

### 4.4 Cycle Detection
- Self-referencing parent (`a -> a`) MUST result in the beat being excluded.
  [source: foolery/src/lib/__tests__/ready-ancestor-cycle.test.ts:19]
- Two-node cycles (`a -> b -> a`) and three-node cycles MUST result in all cyclic beats being excluded.
  [source: foolery/src/lib/__tests__/ready-ancestor-cycle.test.ts:25]
- Beats unrelated to the cycle MUST be preserved.
  [source: foolery/src/lib/__tests__/ready-ancestor-cycle.test.ts:44]
- Roots (beats with no parent) MUST always be kept.
  [source: foolery/src/lib/__tests__/ready-ancestor-cycle.test.ts:56]

### 4.5 Rolling Ancestor Detection
- The system MUST return false when a beat has no parent.
  [source: foolery/src/lib/__tests__/rolling-ancestor.test.ts:9]
- The system MUST return true when any direct or distant ancestor is rolling (has an active session).
  [source: foolery/src/lib/__tests__/rolling-ancestor.test.ts:18]
- The system MUST detect rolling ancestors even when intermediate parents are not retake candidates but exist in the full beat set.
  [source: foolery/src/lib/__tests__/rolling-ancestor.test.ts:36]
- The system MUST handle cycles without infinite loops.
  [source: foolery/src/lib/__tests__/rolling-ancestor.test.ts:56]
- A missing parent in the map MUST return false (broken chain halts traversal).
  [source: foolery/src/lib/__tests__/rolling-ancestor.test.ts:66]

### 4.6 Beat Navigation
- `stripBeatPrefix` MUST remove the leading repo prefix (last hyphen segment).
  [source: foolery/src/lib/__tests__/beat-navigation.test.ts:11]
- `extractBeatPrefix` MUST return null when no hyphen exists or when there is no local segment after the prefix.
  [source: foolery/src/lib/__tests__/beat-navigation.test.ts:24]
- `buildBeatFocusHref` MUST preserve existing query params while setting/updating `beat`, and MUST support overriding or clearing `repo` and setting `detailRepo`.
  [source: foolery/src/lib/__tests__/beat-navigation.test.ts:42]
- `findRepoForBeatId` MUST match by prefix case-insensitively, preferring the longest matching prefix. It MUST also match by repo path basename when display name differs.
  [source: foolery/src/lib/__tests__/beat-navigation.test.ts:74]
- `resolveBeatRepoPath` MUST prefer explicit `repoPath` from notification metadata over prefix matching.
  [source: foolery/src/lib/__tests__/beat-navigation.test.ts:133]

---

## 5. Beat Sorting

### 5.1 Priority-Then-State Sorting
- The system MUST sort by priority first (lower number first).
  [source: foolery/src/lib/__tests__/beat-sort.test.ts:26]
- Equal priority beats MUST be sorted by state rank: queue < action < terminal < blocked < deferred.
  [source: foolery/src/lib/__tests__/beat-sort.test.ts:37]
- Equal priority and state MUST be sorted by title alphabetically, then by id as final tiebreaker.
  [source: foolery/src/lib/__tests__/beat-sort.test.ts:18] (tiebreaker file)
  [source: foolery/src/lib/__tests__/beat-sort.test.ts:73]

### 5.2 Priority-Then-Updated Sorting
- The system MUST sort by priority, then by most recently updated descending.
  [source: foolery/src/lib/__tests__/beat-sort.test.ts:72]

### 5.3 Natural Compare
- The system MUST sort strings with numeric segments in natural numeric order.
  - Example: `item-10` > `item-2` > `item-1`.
  [source: foolery/src/lib/__tests__/beat-sort.test.ts:101]
- The system MUST sort multi-level hierarchical dot-notation IDs correctly.
  - Example: `mqv.2.10` > `mqv.2.2` > `mqv.2.1`.
  [source: foolery/src/lib/__tests__/beat-sort.test.ts:114]

### 5.4 Hierarchical Order
- The system MUST sort siblings by natural ID order regardless of priority when using hierarchical order comparator.
  [source: foolery/src/lib/__tests__/beat-sort.test.ts:133]

### 5.5 Most Recently Updated
- The system MUST sort by `updated` timestamp descending.
- Invalid timestamps MUST be treated as the oldest beats (sort to the end).
  [source: foolery/src/lib/__tests__/beat-sort.test.ts:171]
- Full ties MUST be broken by repository scope (`_repoPath`) for deterministic multi-repo ordering.
  [source: foolery/src/lib/__tests__/beat-sort.test.ts:194]

---

## 6. Beat Display & Identity

### 6.1 Label Display
- The system MUST prefer the first alias when one exists.
  [source: foolery/src/lib/__tests__/beat-display.test.ts:11]
- The system MUST fall back to the stripped beat id when aliases are missing.
  [source: foolery/src/lib/__tests__/beat-display.test.ts:15]
- The system MUST strip project prefixes from hierarchical aliases.
  [source: foolery/src/lib/__tests__/beat-display.test.ts:22]
- Aliases MUST be trimmed before use.
  [source: foolery/src/lib/__tests__/beat-display.test.ts:33]

### 6.2 Detail Lightbox
- `getDisplayedBeatId` MUST keep the full beat ID for the detail header, or override with the canonical id if available.
  [source: foolery/src/lib/__tests__/beat-detail-lightbox.test.ts:25]
- `getDisplayedBeatAliases` MUST deduplicate aliases against the beat id, trim whitespace, skip empty strings, and preserve full project-qualified aliases that differ from the beat id prefix.
  [source: foolery/src/lib/__tests__/beat-detail-lightbox.test.ts:30]
- `getShipBeatPayload` MUST add repo scope (`_repoPath`) to the payload when the lightbox knows the repo, returning a NEW object (not mutating original). It MUST leave the original unchanged when no repo scope is available.
  [source: foolery/src/lib/__tests__/beat-detail-lightbox.test.ts:53]
- `isTerminalBeat` MUST return true for `shipped`, `abandoned`, and `closed`; false for all other states.
  [source: foolery/src/lib/__tests__/beat-detail-lightbox.test.ts:72]

---

## 7. Beat State Overview & Columns

### 7.1 Overview Grouping
- Blank/undefined states MUST normalize to `unknown`.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:45]
- Every beat MUST be placed into exactly one plain state group; no duplicates, no omissions.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:51]
- Known workflow states MUST sort before unknown states.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:71]
- Beats inside a group MUST be sorted by priority and recency (updated descending).
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:89]

### 7.2 Display Rules
- Internal lease records (`type: "lease"`) MUST be filtered out of the overview surface.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:114]
- The overview matrix MUST add required empty columns even when no beats exist in those states.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:130]
- All-repo overview MUST use full labels (including aliases); single-repo overview MUST use stripped labels.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:151]

### 7.3 Tabs
- States MUST be classified into tab groups: `work_items`, `exploration`, `gates`, `terminated`.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:166]
- Non-terminal gate beats MUST belong to `gates`, not `work_items`.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:176]
- Tab counts MUST NOT count lease records.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:196]
- Required columns MUST persist for special tabs even when empty.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:242]
- Empty columns MUST be filterable from the visible rail.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:256]

### 7.4 Introduced Columns
- The system MUST keep introduced empty columns visible until the user explicitly hides them.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:269]
- Hidden columns MUST be reintroduced automatically when beats return to that state.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:307]
- The hide control MUST only be shown at zero count.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:332]

### 7.5 Sizing
- Overview columns MUST be capped at one sixth of available width.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:347]
- Sizing watermarks MUST grow by two columns on growth but MUST NOT shrink when visible columns decrease.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:361]
- Sizing watermarks MUST be tracked independently by tab.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:380]

### 7.6 Lease Metadata
- Lease metadata display MUST NOT fabricate missing fields.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:416]
- Action step time (`knotsSteps.started_at`) MUST NOT be used as lease acquisition time.
  [source: foolery/src/lib/__tests__/beat-state-overview.test.ts:459]

### 7.7 Column Definitions
- Column definitions MUST include an `action` column only when `onShipBeat` is provided.
  [source: foolery/src/lib/__tests__/beat-columns-helpers.test.ts:36]
- The `action` column MUST be hidden in the active view.
  [source: foolery/src/lib/__tests__/beat-columns-helpers.test.ts:42]
- The `type` column MUST NOT appear in queues or active views.
  [source: foolery/src/lib/__tests__/beat-columns-helpers.test.ts:54]
- The `ownerType` column MUST always be present.
  [source: foolery/src/lib/__tests__/beat-columns-helpers.test.ts:66]
- The `_repoName` column MUST appear only when `showRepoColumn` is true.
  [source: foolery/src/lib/__tests__/beat-columns-helpers.test.ts:24]

---

## 8. Beat Release Optimism

- When a beat is released, the system MUST optimistically map active states back to their corresponding queue state immediately in the client-side model.
  [source: foolery/src/lib/__tests__/beat-release-optimism.test.ts:26]
- The system MUST keep stale active refetches pending until the server state actually changes to the target queue state.
  [source: foolery/src/lib/__tests__/beat-release-optimism.test.ts:48]
- Non-release states (e.g., already in a queue state) MUST be ignored; no pending release created.
  [source: foolery/src/lib/__tests__/beat-release-optimism.test.ts:65]

---

## 9. Beat Query Cache

- Query keys for beat lists MUST be stable shared constants: `["beats"]`, `["setlist-plan"]`, `["setlist-plan-beat"]`.
  [source: foolery/src/lib/__tests__/beat-query-cache.test.ts:11]
- Invalidation MUST invalidate all three keys with `refetchType: "active"`.
  [source: foolery/src/lib/__tests__/beat-query-cache.test.ts:22]

---

## 10. Lease Audit Filters

- Date range filters (`dateFrom`, `dateTo`) MUST be inclusive and date-granular (ignore time component on boundary).
  [source: foolery/src/lib/__tests__/lease-audit-filters.test.ts:31]
- The `last24h` preset MUST take precedence over manual dates.
  [source: foolery/src/lib/__tests__/lease-audit-filters.test.ts:88]
- Queue type filters MUST match exactly.
  [source: foolery/src/lib/__tests__/lease-audit-filters.test.ts:111]
- No filters MUST pass through all events.
  [source: foolery/src/lib/__tests__/lease-audit-filters.test.ts:122]

---

## 11. Mark Terminal & Update Mutations

### 11.1 Mark Terminal Route
- The route MUST resolve the canonical id via backend `get` before calling `markTerminal`.
  [source: foolery/src/lib/__tests__/mark-terminal-route.test.ts:51]
- The route MUST pass `repoPath` through to both backend and `regroomAncestors`.
  [source: foolery/src/lib/__tests__/mark-terminal-route.test.ts:68]
- Missing `targetState` MUST return 400.
  [source: foolery/src/lib/__tests__/mark-terminal-route.test.ts:87]
- Non-terminal target states MUST return 400 and surface the `FOOLERY WORKFLOW CORRECTION FAILURE` marker.
  [source: foolery/src/lib/__tests__/mark-terminal-route.test.ts:96]

### 11.2 Update Beat Mutation
- The helper MUST pass the beat's `_repoPath` to the backend update.
  [source: foolery/src/lib/__tests__/update-beat-mutation.test.ts:31]
- An explicit `repoPath` argument MUST override the beat's internal `_repoPath`.
  [source: foolery/src/lib/__tests__/update-beat-mutation.test.ts:43]
- Backend errors MUST be thrown so callers can surface them.
  [source: foolery/src/lib/__tests__/update-beat-mutation.test.ts:58]
- Missing error messages from the backend MUST use a fallback error: `Failed to update beat`.
  [source: foolery/src/lib/__tests__/update-beat-mutation.test.ts:67]

---

## 12. Refine Scope & Rollback

### 12.1 Refine Scope Route
- The route MUST enqueue refinement with the canonical id via backend `get`.
  [source: foolery/src/lib/__tests__/refine-scope-route.test.ts:54]
- The route MUST pass `repoPath` through.
  [source: foolery/src/lib/__tests__/refine-scope-route.test.ts:85]
- If no agent is configured, it MUST return 503.
  [source: foolery/src/lib/__tests__/refine-scope-route.test.ts:118]
- On lookup miss, it MUST fall back to the provided id and still enqueue.
  [source: foolery/src/lib/__tests__/refine-scope-route.test.ts:138]

### 12.2 Rollback Route
- The route MUST roll back the canonical beat in the requested repo scope.
  [source: foolery/src/lib/__tests__/rollback-route.test.ts:48]
- Non-Knots repositories MUST be rejected with 400 without mutating state.
  [source: foolery/src/lib/__tests__/rollback-route.test.ts:73]
- Backend lookup failures MUST be surfaced before rollback (404 for missing).
  [source: foolery/src/lib/__tests__/rollback-route.test.ts:85]

---

## 13. Retake

### 13.1 Retake Source State
- The system MUST accept `shipped`, `closed`, `done`, and `approved` as valid retake source states.
  [source: foolery/src/lib/__tests__/retake-source-state.test.ts:5]
- Case and whitespace MUST be normalized before checking.
  [source: foolery/src/lib/__tests__/retake-source-state.test.ts:12]
- `abandoned`, empty, null, and undefined MUST be rejected.
  [source: foolery/src/lib/__tests__/retake-source-state.test.ts:16]

### 13.2 Retake Session Scope
- When beat ids collide across repos, the system MUST reuse only the running session from the SAME repo.
  [source: foolery/src/lib/__tests__/retake-session-scope.test.ts:14]
- Shipping indexes MUST be scoped per repo for duplicate beat ids.
  [source: foolery/src/lib/__tests__/retake-session-scope.test.ts:38]
- Rolling-ancestor lookups MUST be isolated inside the same repo when beat ids collide.
  [source: foolery/src/lib/__tests__/retake-session-scope.test.ts:65]

---

## 14. API Beats Scope

- All-repo scope keys MUST use repo paths rather than repo count, so changing a repo path changes the cache key.
  [source: foolery/src/lib/__tests__/api-beats-scope.test.ts:26]
- Query keys MUST be stable and include view, scope signature, and serialized params.
  [source: foolery/src/lib/__tests__/api-beats-scope.test.ts:37]
- Aggregate beats route MUST be requested for `scope=all`.
  [source: foolery/src/lib/__tests__/api-beats-scope.test.ts:53]
- The aggregate loader MUST return partial-failure metadata (`_degraded`).
  [source: foolery/src/lib/__tests__/beats-route-all-scope.test.ts:33]
- `Accept: application/x-ndjson` MUST stream NDJSON chunks per repo.
  [source: foolery/src/lib/__tests__/beats-route-all-scope.test.ts:73]

---

## 15. Custom Workflow Claimability

- Custom workflow descriptors MUST derive queue states, action states, and queue actions from profile metadata.
  [source: foolery/src/lib/__tests__/custom-workflow-claimability.test.ts:93]
- Queued states with `kind: "agent"` MUST be claimable (`isAgentClaimable=true`).
  [source: foolery/src/lib/__tests__/custom-workflow-claimability.test.ts:119]
- Queued states with `kind: "human"` MUST NOT be claimable and MUST set `requiresHumanAction=true`.
  [source: foolery/src/lib/__tests__/custom-workflow-claimability.test.ts:133]
- Custom workflow action transitions MUST be classified as iteration success when moving from an action state to a queue/terminal state.
  [source: foolery/src/lib/__tests__/custom-workflow-claimability.test.ts:217]

---

## 16. Stale Beat Grooming

### 16.0 Architecture Overview
A single **worker goroutine** processes a **FIFO queue** of grooming jobs. The pipeline is multi-file in TS [source: `foolery/src/lib/stale-beat-grooming.ts` through `stale-beat-grooming-agent.ts`]:

```
Detection → Queue → Worker → Job Runner → Prompt Builder → Agent → Outcome Applier
```

**Go port pattern:**
- Queue: FIFO array behind `sync.Mutex`, or `chan GroomingJob` with a worker goroutine
- Worker: single goroutine with `for job := range queue` loop
- Worker State: `sync.Mutex` on a struct with `activeJobs map[string]*JobStatus`, ring buffers for recent events
- Store: `sync.Map` keyed by `repoPath::beatId` for review records

### 16.1 Age Rules
- A beat MUST be considered stale only when its `updated` timestamp is more than 7 days older than the reference clock.
  [source: foolery/src/lib/__tests__/stale-beat-grooming.test.ts:33]
- Invalid dates, lease records, and terminal states MUST be ignored.
  [source: foolery/src/lib/__tests__/stale-beat-grooming.test.ts:51]
- Age MUST be reported in whole days with an injected clock.
  [source: foolery/src/lib/__tests__/stale-beat-grooming.test.ts:70]

### 16.2 Summaries & Selection
- Stale beat summaries MUST be ordered by last-updated age descending (oldest first).
  [source: foolery/src/lib/__tests__/stale-beat-grooming.test.ts:82]
- Batch selection MUST select the oldest subset up to the limit.
  [source: foolery/src/lib/__tests__/stale-beat-grooming.test.ts:99]
- Keys for all-repo overview beats MUST be stable and repo-qualified: `{repoPath}::{beatId}`.
  [source: foolery/src/lib/__tests__/stale-beat-grooming.test.ts:121]
- Review request building MUST filter to only selected keys and trim the agent id.
  [source: foolery/src/lib/__tests__/stale-beat-grooming.test.ts:141]

### 16.3 Worker State
- Worker health MUST surface the active job before agent details arrive.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-worker-state.test.ts:32]
- Agent name and version MUST be recorded once resolved, preserving prior fields on partial updates.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-worker-state.test.ts:46]
- Progress and agent updates for unknown job ids MUST be ignored.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-worker-state.test.ts:73]
- Progress timestamps MUST be recorded so callers can detect hung sessions.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-worker-state.test.ts:84]
- Releasing a job MUST drop all diagnostics for that job.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-worker-state.test.ts:94]

### 16.4 Status Route
- The status route MUST start the worker and return queue size, review records, and worker health.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-status-route.test.ts:41]

### 16.5 Enqueue & Options Routes
- POST to reviews MUST validate the override agent, resolve canonical ids, and enqueue targets.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-route.test.ts:103]
- Oldest-mode MUST list stale beats and enqueue them without individual `get` calls.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-route.test.ts:157]
- Fallback MUST use submitted beat id when canonical lookup misses.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-route.test.ts:199]
- Selected-agent failures MUST surface `FOOLERY GROOMING FAILURE` and return 400.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-route.test.ts:214]

### 16.6 Prompt & Parsing
- The grooming prompt MUST include beat context, age, and the decision contract (`still_do`, `reshape`, `drop`).
  [source: foolery/src/lib/__tests__/stale-beat-grooming-prompt.test.ts:8]
- Output parsing MUST extract the tagged JSON block and reject invalid decisions or unparseable output.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-prompt.test.ts:27]

### 16.7 Outcomes
- `still_do` MUST append a handoff capsule and update `lastUpdated` without mutating title/description/acceptance.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-outcomes.test.ts:78]
- `reshape` MUST apply fields (`title`, `description`, `acceptance`) through `backend.update` and append a capsule.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-outcomes.test.ts:98]
- `drop` MUST mark the beat abandoned via `backend.markTerminal` with a reason capsule.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-outcomes.test.ts:117]
- On parse failure, the system MUST NOT mutate the beat or append success capsules.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-outcomes.test.ts:157]

### 16.8 Job Runner Timeout
- Hung grooming jobs MUST be killed with `SIGKILL` after the timeout.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-job-runner-timeout.test.ts:86]
- Timeout error messages MUST be stale-grooming-specific and MUST NOT mention other subsystems (e.g., scope refinement).
  [source: foolery/src/lib/__tests__/stale-beat-grooming-job-runner-timeout.test.ts:114]
- Timeout log lines MUST be tagged with `[stale-grooming]`.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-job-runner-timeout.test.ts:140]

### 16.9 Agent Resolution
- The system MUST resolve the `staleGrooming` action config in basic dispatch mode.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-agent.test.ts:73]
- In advanced dispatch mode, it MUST resolve the `stale_grooming` pool.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-agent.test.ts:83]
- Explicit overrides MUST be allowed to any configured agent.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-agent.test.ts:99]
- Missing dispatch targets MUST fail loudly with `FOOLERY GROOMING FAILURE`.
  [source: foolery/src/lib/__tests__/stale-beat-grooming-agent.test.ts:110]

---

## 17. Multi-Repo Stream

- The system MUST yield per-repo chunks fastest-first.
  [source: foolery/src/lib/__tests__/beats-multi-repo-stream.test.ts:55]
- Partial failures MUST carry `_degraded` metadata and count total errors.
  [source: foolery/src/lib/__tests__/beats-multi-repo-stream.test.ts:91]
- Zero repos MUST yield an empty summary.
  [source: foolery/src/lib/__tests__/beats-multi-repo-stream.test.ts:129]
- All repos failing MUST result in `totalErrors` matching repo count and empty beats.
  [source: foolery/src/lib/__tests__/beats-multi-repo-stream.test.ts:140]
- The system MUST serve repeat loads from cache without re-invoking the backend.
  [source: foolery/src/lib/__tests__/beats-multi-repo-stream.test.ts:163]

---

## 18. Beats View

- `parseBeatsView` MUST normalize legacy and null values to canonical views (`queues`, `setlist`, `overview`, etc.).
  [source: foolery/src/lib/__tests__/beats-view.test.ts:11]
- `isListBeatsView` MUST return true for list-capable views (`search`, `queues`, `active`) and false for matrix views (`overview`, `setlist`, `finalcut`).
  [source: foolery/src/lib/__tests__/beats-view.test.ts:19]
- `buildBeatsSearchHref` MUST navigate search submissions into the dedicated search view and clear search without leaving the user stuck.
  [source: foolery/src/lib/__tests__/beats-view.test.ts:28]

---

## 19. Beats Characterization & Utilities

- `beatToCreateInput` MUST copy fields: `title`, `description`, `type`, `priority`, `labels`, `assignee`, `due`, `acceptance`, `notes`, `estimate`.
  [source: foolery/src/lib/__tests__/beats-characterization.test.ts:173]
- `beatToCreateInput` MUST explicitly exclude `id`, `parent`, `state`, `created`, `updated`.
  [source: foolery/src/lib/__tests__/beats-characterization.test.ts:200]

---

## 20. Overview Filters

- Visible beat tags MUST exclude overview bookkeeping labels (`stage:*`, `orchestration:*`, `commit:*`, `branch:*`, `parent:*`) and deduplicate case-insensitively. Values MUST be trimmed.
  [source: foolery/src/lib/__tests__/beat-state-overview-filters.test.ts:54]
- Tag filter options MUST include counts.
  [source: foolery/src/lib/__tests__/beat-state-overview-filters.test.ts:69]
- Setlist filter labels MUST include the plan id and the first 40 characters of the objective.
  [source: foolery/src/lib/__tests__/beat-state-overview-filters.test.ts:83]
- Filtering MUST intersect tags and setlists when both are selected.
  [source: foolery/src/lib/__tests__/beat-state-overview-filters.test.ts:92]

---

## 21. Take Eligibility

- A beat MUST be take-eligible if it is in a queue state, agent-owned, and explicitly claimable.
- Terminal states (`shipped`, `abandoned`, `closed`) MUST be blocked.
- Human-owned next actions MUST be blocked.
- Gate beats MUST be allowed only if they are claimable; human-owned gates MUST be blocked.
- `isAgentClaimable: false` MUST block the take.
  [source: foolery/src/lib/__tests__/beat-take-eligibility.test.ts:14]

---

## 22. Skill Prompts

- Skill prompts MUST contain the correct heading for each workflow step, the `bd show` command, `bd sync`, the current state, and an authority boundary section.
  [source: foolery/src/lib/__tests__/beats-skill-prompts.test.ts:11]
- Allowed exit states MUST be listed with their corresponding transition commands.
  [source: foolery/src/lib/__tests__/beats-skill-prompts.test.ts:62]

---

## 23. Beats Parity

- For every profile, queued agent-owned states MUST produce `isAgentClaimable=true`.
  [source: foolery/src/lib/__tests__/beats-parity-step.test.ts:21]
- Active states and terminal states MUST produce `isAgentClaimable=false`.
  [source: foolery/src/lib/__tests__/beats-parity-step.test.ts:35]
- All states MUST map correctly through `resolveStep` and `deriveWorkflowRuntimeState`.
  [source: foolery/src/lib/__tests__/beats-parity-step.test.ts:52]

---

## 5. Epic Lifecycle & MergeManager

### 5.1 Overview — 11-Step Epic Lifecycle

The epic lifecycle governs how a multi-beat epic progresses from creation through worker execution, merge, PR, and final sweep. Each epic is tracked as a parent beat with child beats; the MergeManager watches for all children to reach `awaiting_integration` before triggering the merge pipeline.

```
1. Epic Creation     — Parent beat created with workflow=epic, children attached as dependencies.
2. Branch Creation   — A git branch is created for the epic (naming: epic/<epic-id>).
3. Worker Dispatch   — For each child beat in ready_for_implementation, a worker is spawned.
4. Worker Execution  — Workers execute their assigned beat, transitioning through action states.
5. Gate Review       — Workers' outputs enter plan_review / implementation_review as configured.
6. Await Integration — Child beats transition to awaiting_integration after passing gates.
7. Trigger Detection — MergeManager polls: all children in awaiting_integration? If yes, proceed.
8. Single-Flight Lock — Acquire a per-epic_id mutex; reject concurrent merge attempts (D11=A).
9. Merger            — Merge worker branches into the epic branch; resolve conflicts via configured strategy.
10. Push             — Push the merged branch to the remote.
11. PR + Sweep       — Create a pull request for the epic, then run sweep resilience (see §5.3).
```

**Invariants:**
- An epic MUST have at least one child beat.
- Trigger detection MUST NOT fire while any child is in an action state (including review states, unless explicit skip-gate is configured).
- Single-flight lock MUST be keyed by `epic_id`; two concurrent MergeManager polls for the same epic MUST serialize.

### 5.2 MergeManager Trigger Detection

The MergeManager uses a single polling query to detect when all children of an epic are ready for integration:

```
bd list --parent=<epic-id> --status=awaiting_integration --json
```

**Algorithm (D14=A):**
1. Run `bd list --parent=<epic-id> --status=awaiting_integration --json`.
2. Count returned beats: `count_awaiting`.
3. Fetch total child count for the epic (cached from epic creation): `total_children`.
4. If `count_awaiting == total_children`, the epic is ready for merge.
5. If `count_awaiting < total_children`, some children are still in progress. Re-poll after configured interval.
6. If `count_awaiting == 0` and `total_children > 0`, log `KERNL DISPATCH FAILURE: no children in awaiting_integration for epic <epic-id> — possible state drift — Fix: check child beat states manually`.

**Single-Flight Lock (D11=A):**
- Before entering the merge step, the MergeManager MUST acquire a per-epic-id mutex.
- If the lock cannot be acquired (another merge already in flight), the poll cycle MUST skip this epic and retry next cycle.
- Lock MUST be released after step 10 (Push) succeeds or fails.
- Lock acquisition timeout: 30s. If timeout expires, log `KERNL DISPATCH FAILURE: single-flight lock timeout for epic <epic-id> — possible deadlock in merge — Fix: investigate prior merge state, release lock manually if needed`.

**Polling configuration:**
- Default interval: 30s between polls for the same epic.
- Jitter: ±5s random jitter added to prevent thundering herd across many epics.
- Backoff: If poll returns zero `awaiting_integration` children consecutively 10+ times, double the interval (max 5 minutes).

### 5.3 Sweep — Three-Layer Resilience

After the PR is created (step 11), the Sweep subsystem ensures the epic workspace is cleaned up and no stale state lingers. It operates in three layers, each degrading independently:

**Layer 1 — Cache MERGED Status:**
- After successful merge+push, write a `MERGED` marker to the run-state cache (SQLite) keyed by `epic_id`.
- On startup, the MergeManager checks for cached `MERGED` epics and skips re-polling them.
- Cache TTL: 7 days. After TTL, the marker is stale and the epic is treated as not-yet-merged (safe re-poll).

**Layer 2 — Circuit Breaker:**
- Each epic has a failure counter. After 3 consecutive merge failures (any step from 8–11), the circuit opens.
- Open circuit: MergeManager stops polling that epic entirely. Logs `KERNL DISPATCH FAILURE: circuit breaker open for epic <epic-id> after 3 merge failures — Fix: investigate merge failure root cause, reset circuit breaker manually`.
- Half-open: After a cooldown period (5 minutes), the circuit moves to half-open and allows ONE retry. If it succeeds, the circuit closes. If it fails, it re-opens.
- Circuit breaker state is stored in the run-state SQLite.

**Layer 3 — Skip-on-Fail + PR Stale WARN:**
- If any sweep operation fails (e.g., branch cleanup, label update), the sweep MUST NOT block the overall epic pipeline. Log the failure and continue.
- After PR creation, if the PR remains unmerged for >48 hours, the sweep subsystem emits a `PR_STALE` warning event to the session buffer:
  ```
  KERNL DISPATCH FAILURE: PR for epic <epic-id> is stale (>48h unmerged) — PR URL: <url> — Fix: review and merge or close the PR
  ```
- Stale PR warnings repeat every 24 hours until merged, closed, or circuit-breaker opens.

### 5.4 MergeManager State Transition Diagram

```
                    ┌──────────────┐
                    │   IDLE       │
                    └──────┬───────┘
                           │ poll: all children awaiting_integration?
                           ▼
                    ┌──────────────┐
                    │  DETECTED    │ count_awaiting == total_children
                    └──────┬───────┘
                           │ acquire single-flight lock
                           ▼
                    ┌──────────────┐
                    │  LOCKED      │────(lock timeout)───► IDLE + WARN
                    └──────┬───────┘
                           │ lock acquired
                           ▼
                    ┌──────────────┐
                    │  MERGING     │
                    └──────┬───────┘
                           │
                    ┌──────┼──────┐
                    │             │
                    ▼             ▼
             ┌──────────┐  ┌──────────────┐
             │ PUSHING  │  │ MERGE_FAIL   │──(retry < 3)──► MERGING
             └────┬─────┘  └──────┬───────┘
                  │               │ (retry >= 3)
                  ▼               ▼
             ┌──────────┐  ┌──────────────┐
             │ PR +     │  │ CIRCUIT_OPEN │──(cooldown)──► HALF_OPEN
             │ SWEEP    │  └──────────────┘
             └────┬─────┘
                  │
                  ▼
             ┌──────────┐
             │ MERGED   │ (cached, 7-day TTL)
             └──────────┘
```

### 5.5 Sweep Cleanup Operations

After merge+push+PR, the sweep phase performs these idempotent cleanup operations:

| Operation | On Failure | Recovery |
|-----------|-----------|----------|
| Delete epic branch (local) | Skip, log warning | Manual branch cleanup |
| Delete epic branch (remote) | Skip, log warning | Manual remote branch cleanup |
| Tag epic beat as `shipped` | Skip, log warning | Manual `bd update` |
| Tag child beats as `shipped` | Skip, log warning | Manual `bd update` |
| Clear run-state cache entry | Skip, log warning | TTL auto-cleans |

All sweep operations are fire-and-forget with independent error paths. No operation can block another.

---

## Appendix: Cross-Cutting Failure Modes & Invariants

1. **Fail Loudly, Never Silently** (per AGENTS.md)
   - Missing beats during plan generation MUST throw before spawning.
     [source: foolery/src/lib/__tests__/orchestration-plan-generation.test.ts:227]
   - Missing dispatch targets for grooming MUST throw `FOOLERY GROOMING FAILURE`.
     [source: foolery/src/lib/__tests__/stale-beat-grooming-agent.test.ts:110]
   - Non-terminal target states in mark-terminal MUST throw `FOOLERY WORKFLOW CORRECTION FAILURE`.
     [source: foolery/src/lib/__tests__/mark-terminal-route.test.ts:96]

2. **Hermetic Test Policy** (per AGENTS.md)
   - All extracted behavior contracts MUST be unit-testable without touching the host environment (no real filesystem, no real processes, no real network).
   - The Go port MUST mock at boundaries via interfaces.

3. **kno Workflows Are Authoritative** (per AGENTS.md)
   - Valid next states MUST come exclusively from loom-defined transitions. No synthetic transitions. No fabricated rollback targets.
     [source: foolery/src/lib/__tests__/valid-next-states.test.ts:6]
   - Correction actions that skip gates MUST be named as such and invoke the backend with `force: true`.

4. **State Normalization**
   - Legacy states (`open`, `impl`, `retake`, `closed`, etc.) MUST normalize to canonical states before any workflow logic runs.
     [source: foolery/src/lib/__tests__/workflows-coverage-labels.test.ts:121]
   - Raw kno state MUST be normalized (trimmed, lowercased) before rollback detection.
     [source: foolery/src/lib/__tests__/valid-next-states.test.ts:201]

5. **Canonical ID Resolution**
   - Routes that accept user-provided beat ids MUST resolve to canonical ids via backend `get` before mutating state, and MUST fall back to the raw id only on lookup miss.
     [source: foolery/src/lib/__tests__/refine-scope-route.test.ts:138]
     [source: foolery/src/lib/__tests__/stale-beat-grooming-route.test.ts:199]

6. **Repo Scoping**
   - All beat operations MUST carry repo scope (`repoPath`) to backend calls.
   - Multi-repo contexts MUST isolate lookups by repo (prefix matching or explicit scope).
     [source: foolery/src/lib/__tests__/retake-session-scope.test.ts:14]
     [source: foolery/src/lib/__tests__/api-beats-scope.test.ts:26]
