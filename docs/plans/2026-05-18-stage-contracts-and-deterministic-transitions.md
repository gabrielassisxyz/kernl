---
name: Stage Contracts & Deterministic Transitions
last_updated: 2026-05-18
status: proposed
related:
  - docs/VISION.md (§8 — Orchestrator)
  - docs/STRATEGY.md
  - orchestrator/specs/prompt/prompt.md
  - internal/app/prompt.go
  - internal/app/drive_bead.go
  - internal/dispatch/dispatch.go
  - internal/backend/port.go (WorkflowDescriptor)
  - internal/backend/state_machine.go (ForwardTransitionTarget, DeriveWorkflowRuntimeState)
---

# Stage Contracts & Deterministic Transitions

> Move "what each stage produces" and "when a stage is done" out of the agent's
> prompt-time discretion and into the workflow descriptor + orchestrator.
> Eliminates the two top operational failures: agents drift out of role, and
> agents forget to advance `bd` status.

## 1. Context

### 1.1 What's broken today

Two failure modes consume most of the operational pain in the current
orchestrator (observed in `kernl-npp` MVP run, 2026-05-17, and structurally
visible in code):

**A. Role drift.** A planner agent often jumps straight to implementation. The
acceptance criteria of `BuildBeadStagePrompt`
(`internal/app/prompt.go:21-95`) treat the bead description as the agent's
instructions, regardless of which stage is running. The only stage-aware text
is the line *"Current workflow state: \`planning\`"* and the bd-update command
at exit. There is no role contract, no defined output artifact, no list of
forbidden actions — so the LLM falls back to "do what the bead description
implies", which is the implementation work for the feature.

**B. Agent-owned transitions.** `drive_bead.go:131-147` already claims
`ready_for_X → X` deterministically. But the second half (`X →
ready_for_X_review`) is the agent's responsibility, via `bd update --status`
embedded in the prompt. When the agent forgets — or exits before reaching
that step, or hits a tool rejection that derails it — the bead is marked
`blocked` and the entire session's work is discarded. The compensation code
for this single failure mode (`retryStuckStage`,
`drive_bead.go:202-265`, plus the 🚨 END-OF-STAGE PROTOCOL section of every
prompt) is the largest piece of orchestrator complexity that exists purely to
work around LLM non-determinism.

### 1.2 What's already correct

These pieces are sound and the plan builds on top of them, not around them:

- The workflow descriptor (`internal/backend/port.go:47-67`) already carries
  `Transitions`, `QueueActions`, `ReviewQueueStates`, and per-state owners. The
  data model has room for stage contracts without schema-breaking changes.
- The orchestrator already owns the `ready_for_X → X` claim
  (`drive_bead.go:131-147`).
- `ForwardTransitionTarget` and `DeriveWorkflowRuntimeState` already exist as
  single sources of truth for state progression
  (`internal/backend/state_machine.go:402, 471`).
- The foolery-era prompt spec (`orchestrator/specs/prompt/prompt.md` §1.3)
  already named the right primitives — *Authority Boundary*, *Skill Prompt
  Contracts*, *exhaustive transition commands*. It just stopped at "tell the
  agent" instead of "make the system enforce".

### 1.3 Imported philosophy (from gastown's mail protocol)

Three principles from `docs/design/mail-protocol.md` in the gastown clone that
shape decisions here, even though no mail/messaging is being introduced now:

1. **Ephemeral vs persistent.** Gastown distinguishes `gt nudge` (zero-cost,
   tmux-injected, dies with the session) from `gt mail` (Dolt-committed,
   survives session death). The rule: if the recipient's session dying makes
   the message irrelevant, the message is ephemeral. We apply this here as:
   stage-internal context and prompts are ephemeral (rebuilt each spawn);
   stage outputs (plan, review verdict) are persistent (committed artifacts
   in the worktree). We do not introduce a messaging layer to carry stage
   outputs — the filesystem + git already satisfy "survives session death"
   without commit-graph pollution.

2. **Roles, not stages, talk over wire.** Gastown's mail is between
   long-running roles (Polecat, Witness, Refinery, Deacon, Mayor) — not
   between stages of a single bead's lifecycle. This reinforces: stage
   handoff is mechanism (filesystem + state machine), role coordination is
   protocol (later, when we add Witness/Sweeper/Releaser as long-running
   services). This plan deliberately stays inside the mechanism layer.

3. **Budget persistent communication.** Every persistent artifact has a real
   cost (commit, audit-log entry, future cleanup). Stage artifacts must be
   limited and shaped — not freeform LLM output dumped to disk. The contract
   defines exactly what gets written, where, and in what schema.

## 2. The three changes

These are ordered as they should ship: each lands independently, each is
testable in isolation, and each one delivers value without the next.

### Change 1 — Orchestrator owns the full transition (ship first)

**Why first:** it is the smallest change, it eliminates the most pain
immediately, and it unblocks the other two by establishing that the
orchestrator is the canonical mover of bead state.

**What changes:**

- Add a new field to `WorkflowDescriptor.Transitions`: an optional `ExitGate`
  per active-state. Default value `agent_exit_zero` preserves current
  behavior. New value `artifact_exists` requires a path-relative-to-worktree
  to exist when the agent exits.
- After `deps.Driver.RunBead` returns `res.Success == true`
  (`drive_bead.go:181`), the orchestrator:
  1. Computes the next state via `ForwardTransitionTarget`.
  2. Evaluates the `ExitGate` of the active state.
  3. If gate passes → orchestrator runs `bd update --status <next>` itself.
  4. If gate fails → orchestrator marks the bead `blocked` with a structured
     reason (`gate_failed: artifact_missing path=...` or `tests_failed`).
- Remove from `BuildBeadStagePrompt`:
  - The entire 🚨 END-OF-STAGE PROTOCOL section (`prompt.go:69-81`).
  - Rule 7's *"DO NOT run `bd close`"* (already correct, but the symmetric
    constraint "DO NOT run `bd update`" is added — agents must not touch bd
    state at all).
- Remove from `drive_bead.go`:
  - The `retryStuckStage` branch that retries when the agent exited rc=0 but
    didn't advance. The agent has no responsibility to advance, so this case
    cannot occur. Retries for genuine work failures (tests broke, artifact
    missing) stay, but are renamed and re-scoped.
- Restrict bd-mutating commands in the opencode permission allowlist (the
  config injected by `injectOpencodeConfigEnv`,
  `drive_bead.go:163`). `bd show`, `bd list`, `bd comment` stay allowed.
  `bd update`, `bd close`, `bd open` get rejected.

**Acceptance criteria:**

1. A bead in `planning` whose agent exits rc=0 **without** running
   `bd update`:
   - Today: bead stays at `planning`, `retryStuckStage` kicks in, eventually
     marked `blocked` and work discarded.
   - After this change with `ExitGate=agent_exit_zero` (default): bead
     advances to `ready_for_plan_review` automatically, agent's commits are
     preserved.
2. A bead in `planning` with `ExitGate=artifact_exists path=.kernl/<id>/plan.md`
   whose agent exits rc=0 but the artifact does not exist:
   - Bead marked `blocked` with reason
     `gate_failed: artifact_missing path=.kernl/<id>/plan.md`.
   - No bd-status advancement.
   - Agent's session ID and worktree state preserved for inspection.
3. An agent that *attempts* `bd update --status …` against the canonical
   repo is rejected by the opencode permission layer, and the rejection is
   logged as a non-fatal warning (so we observe agents trying it during the
   transition period, but they cannot succeed).
4. `retryStuckStage` is removed; no test references it.
5. The 🚨 END-OF-STAGE PROTOCOL section is absent from prompts generated by
   `BuildBeadStagePrompt`. The retry-stuck heuristic is absent from
   `drive_bead.go`.

**Tests (all hermetic, `*_test.go`):**

- `internal/app/drive_bead_test.go`:
  - `TestDriveBead_OrchestratorAdvancesAfterAgentSuccess` — agent exits
    rc=0, no bd update from the agent → bead transitions
    `planning → ready_for_plan_review`.
  - `TestDriveBead_GateFailsBlocksWithReason` — `ExitGate=artifact_exists`,
    artifact missing → bead state is `blocked`, comment contains
    `gate_failed:`.
  - `TestDriveBead_GateDefaultIsAgentExitZero` — descriptor with no
    `ExitGate` set behaves identically to today's "agent exited cleanly,
    advance" logic.
  - `TestDriveBead_AgentBdUpdateAttemptDoesNotDoubleAdvance` — even if a
    test fake invokes `bd update` directly, the orchestrator's advancement
    is idempotent (already on the target state → no-op, no error).
- `internal/app/prompt_test.go`:
  - `TestBuildBeadStagePrompt_OmitsEndOfStageProtocol` — output does not
    contain "END-OF-STAGE PROTOCOL" or the `bd -C … update … --status`
    pattern.
  - `TestBuildBeadStagePrompt_ForbidsBdMutation` — output contains an
    explicit "Do not run `bd update`, `bd close`, or `bd open`" instruction
    (defense-in-depth alongside the permission layer).
- `internal/backend/state_machine_test.go`:
  - `TestExitGate_ArtifactExists` — gate evaluator returns the expected
    pass/fail given a worktree state.
- Integration (`-tags=integration`):
  - `internal/integration/epic_run_happy_test.go` — extend to assert that
    with a fake agent that does *not* call `bd update`, the epic still
    completes end-to-end.

**Files touched:**

| File | Change |
|---|---|
| `internal/backend/port.go` | Add `ExitGate` field to `WorkflowTransition` (or a sibling map on `WorkflowDescriptor` if cleaner) |
| `internal/backend/state_machine.go` | New: `EvaluateExitGate(wf, state, worktree) (bool, string)` |
| `internal/app/drive_bead.go` | Insert post-spawn gate evaluation + deterministic `bd update`; delete stuck-update retry branch |
| `internal/app/prompt.go` | Delete END-OF-STAGE PROTOCOL section; add bd-mutation prohibition |
| `internal/app/opencode_permissions.go` (or wherever `injectOpencodeConfigEnv` lives) | Reject `bd update`/`close`/`open` |
| Workflow YAML / `factory.go` builtin descriptors | Default `ExitGate: agent_exit_zero` for all stages |

**Migration & rollback:**

- The default `ExitGate=agent_exit_zero` makes the change behavior-preserving
  for any workflow that hasn't opted into stricter gates yet.
- During a transition window (one release), keep a fallback: if the
  orchestrator's `bd update` returns "already in target state", treat as
  success (covers agents from old workflows that still try to update).
- Rollback: revert the four files above; no schema migration is needed
  (the new `ExitGate` field is additive and optional).

**Out of scope for Change 1:**

- Defining role-specific contracts (Change 2).
- Defining handoff artifact paths or schemas (Change 3).
- Adding gates beyond `agent_exit_zero` and `artifact_exists` (e.g.
  `tests_pass`, `commits_match_pattern` — can come later, additive).

---

### Change 2 — Per-stage contracts in the workflow descriptor

**Prerequisites:** Change 1 landed (orchestrator owns transitions).

**What changes:**

- Extend the workflow YAML schema with a `stages` block. Each stage entry
  defines the agent's role contract:

  ```yaml
  stages:
    planning:
      role: |
        Decompose the bead into an actionable plan. Identify subtasks,
        dependencies, and acceptance criteria. Do not write source code.
      inputs:
        - bead.title
        - bead.description
        - bead.acceptance
        - repo state (read-only)
      output_artifact:
        path: ".kernl/<bead_id>/plan.md"
        schema: plan-v1                    # optional; future
      forbidden_paths:
        - "**/*.go"
        - "**/*.ts"
        - "**/*.py"
        - "**/*.rs"
      exit_gate: artifact_exists           # from Change 1
    implementation:
      role: |
        Implement the plan. Modify code to satisfy the acceptance criteria
        and the plan's subtasks. Do not modify the plan.
      inputs:
        - bead.acceptance
        - ".kernl/<bead_id>/plan.md"
      output_artifact:
        kind: commits
        commit_marker: "stage: implementation"
      forbidden_paths:
        - ".kernl/**/plan.md"
        - ".kernl/**/plan-review.md"
      exit_gate: commits_exist_with_marker
    plan_review:
      role: |
        Review the plan for correctness, completeness, and alignment with
        the bead's acceptance criteria. Produce a verdict.
      inputs:
        - bead
        - ".kernl/<bead_id>/plan.md"
      output_artifact:
        path: ".kernl/<bead_id>/plan-review.md"
        must_end_with: "VERDICT: PASS"     # or FAIL
      forbidden_paths:
        - "**/*.go"
        - ".kernl/**/plan.md"              # reviewer cannot rewrite the plan
      exit_gate: artifact_exists
  ```

- `BuildBeadStagePrompt` is rewritten as a contract renderer. Bead title /
  description / acceptance become **input data** in the prompt, not
  instructions. The prompt structure becomes:

  1. **Role** (verbatim from `stages.<state>.role`)
  2. **Inputs available to you** (rendered values)
  3. **Required output** (artifact path + schema/marker constraints)
  4. **You may NOT** (forbidden paths, bd mutation)
  5. **You SHOULD** (existing operating rules: worktree, AGENTS.md, etc.)
  6. **Bead data** (the actual description/acceptance, framed as data, not
     instruction)

- `forbidden_paths` is wired into the opencode permission config: writes to
  those paths are rejected at the sandbox level, not just discouraged in
  text. This is the load-bearing piece — text-only constraints are
  unreliable.

**Acceptance criteria:**

1. A planning agent's prompt:
   - Contains its `role:` text near the top, before any bead content.
   - Lists `.kernl/<bead_id>/plan.md` as the required output.
   - Lists `**/*.go` (and siblings) under "you may NOT modify".
   - Frames the bead description under a "Bead data (input)" heading, not as
     instructions to execute.
2. A planning agent that attempts `Write src/foo.go`:
   - Receives a sandbox rejection (not just a text warning).
   - The rejection is logged with `stage=planning forbidden_path=src/foo.go`.
3. The same bead, when its state advances to `implementation`, gets a prompt
   whose role text is the implementation contract; the planning role text is
   absent.
4. A custom YAML workflow can define a new stage with its own `role:`,
   `inputs:`, `output_artifact:`, `forbidden_paths:`, and have it executed
   without code changes in kernl. This satisfies VISION §8.2's promise that
   workflow shapes are declarative.
5. Removing the `stages:` block from a workflow YAML degrades gracefully:
   the prompt falls back to today's generic format with no role contract.
   No workflow that exists today breaks.

**Tests:**

- `internal/app/prompt_test.go`:
  - `TestBuildBeadStagePrompt_RendersStageContract` — given a descriptor
    with a `planning` stage role, the rendered prompt contains the role
    string, the input list, the output artifact path, and the forbidden
    paths.
  - `TestBuildBeadStagePrompt_BeadIsInputNotInstruction` — bead description
    appears under a heading matching `Bead data` (or `Inputs`), not under
    `Steps` / `Instructions`.
  - `TestBuildBeadStagePrompt_FallbackWhenNoStageBlock` — descriptor with
    no `stages` block produces a non-contract prompt that still
    runs (parity with today minus the deleted END-OF-STAGE section).
- `internal/backend/workflow_test.go`:
  - `TestLoadWorkflow_StagesBlockParses` — YAML round-trip for the stage
    contract block.
  - `TestLoadWorkflow_StagesBlockOptional` — workflow without `stages`
    loads with empty contracts.
- `internal/app/opencode_permissions_test.go`:
  - `TestForbiddenPathsRejectedAtSandbox` — given a stage contract with
    `forbidden_paths: ["**/*.go"]`, the generated opencode permission
    config rejects writes to `src/foo.go`.
- Integration:
  - `internal/integration/stage_contract_planning_test.go` — fake agent
    configured to attempt `Write src/foo.go` during a planning stage; assert
    the write is rejected, the plan.md artifact is still required, and the
    bead ends `blocked` with `artifact_missing` (not silently advanced).

**Files touched:**

| File | Change |
|---|---|
| `internal/backend/port.go` | Add `Stages map[string]StageContract` to `WorkflowDescriptor` |
| `internal/backend/workflow_yaml.go` (or wherever YAML loading lives) | Parse new `stages:` block |
| `internal/app/prompt.go` | Rewrite `BuildBeadStagePrompt` as contract renderer |
| `internal/app/opencode_permissions.go` | Translate `forbidden_paths` into permission rejections |
| `internal/backend/factory.go` | Add stage contracts to the builtin SDLC descriptor |
| Workflow YAML examples / canonical-vibe-coding shape | Author the role text for each stage |

**Migration & rollback:**

- Additive schema; old workflow YAMLs continue to work.
- The canonical `vibe-coding-pipeline` descriptor gets stage contracts
  authored in the same PR. Custom workflows can adopt at their own pace.
- Rollback: revert prompt.go and the YAML parser changes; the descriptor
  field becomes unread but harmless.

**Out of scope for Change 2:**

- Schema validation of artifact content (e.g. validating that `plan.md`
  matches `plan-v1`). The contract reserves the field; enforcement is a
  later addition.
- Multi-artifact outputs from a single stage.
- Per-stage tool allowlists beyond path restrictions.

---

### Change 3 — Artifact-based handoff convention

**Prerequisites:** Changes 1 and 2 landed.

**What changes:**

This is the smallest change of the three — mostly convention codification —
but it formalizes the data path between stages so downstream tooling (UI,
audit, the future DA) can find stage outputs predictably.

- Define the on-disk layout for stage artifacts:

  ```
  .kernl/<bead_id>/
    plan.md                  # planning output (Change 2)
    plan-review.md           # plan_review output, ends with VERDICT: PASS|FAIL
    implementation-notes.md  # optional, implementer to reviewer
    review-implementation.md # implementation_review output
    integration-notes.md     # optional
    ...
  ```

- All artifacts live inside the bead's worktree, get committed by the
  stage's agent, and travel with the merge into the epic branch. No
  separate store, no separate sync. Audit history = git history of
  `.kernl/<bead_id>/`.
- After each successful stage advancement, the orchestrator runs
  `bd comment <bead_id>` with a structured note:

  ```
  stage: planning
  agent: kimi-k2.6
  session_id: <id>
  artifact: .kernl/<bead_id>/plan.md
  commit: <sha>
  duration: 2m41s
  ```

  This gives `bd show <id>` a complete audit of the stage history without
  digging through worktrees.
- The `.gitignore` for the worktree explicitly **un-ignores** `.kernl/` so
  artifacts survive `git add -A`.
- The next stage's prompt references its inputs by reading the artifact
  from disk (Change 2's `inputs:` block already lists them); the
  orchestrator does not need to pass artifact bodies through the prompt —
  the agent reads the file when needed.

**Acceptance criteria:**

1. After a successful planning stage, the bead's worktree contains
   `.kernl/<bead_id>/plan.md`, the file is committed, and `bd show <id>`
   shows a `stage: planning` comment with the artifact path.
2. An implementation agent, with the planning artifact present in its
   worktree, can `Read .kernl/<bead_id>/plan.md` (no permission rejection)
   but cannot `Write` it (forbidden via Change 2).
3. After all stages complete and the bead's worktree is merged into the
   epic branch, the epic branch contains the full `.kernl/<bead_id>/`
   directory with every stage's artifacts.
4. The `bd show` audit trail for a completed bead lists, in order, every
   stage that ran, the agent that ran it, the session ID, the artifact
   produced, and the commit SHA.

**Tests:**

- `internal/app/drive_bead_test.go`:
  - `TestDriveBead_StageCommentRecorded` — after a successful stage
    advancement, the bd backend received a `Comment` call with the
    structured note format.
- `internal/integration/artifact_handoff_test.go` (`-tags=integration`):
  - End-to-end: run a 2-stage workflow (planning → implementation) with
    fake agents; assert the planning artifact exists after stage 1, the
    implementation agent's session has it readable, and the final
    worktree contains both artifacts.
- `internal/worktree/worktree_test.go`:
  - `TestWorktreeGitignoreAllowsKernlDir` — the gitignore template used
    when creating a worktree does not exclude `.kernl/`.

**Files touched:**

| File | Change |
|---|---|
| `internal/app/drive_bead.go` | After advancement, call `Backend.Comment` with structured stage record |
| `internal/worktree/worktree.go` (or wherever gitignore is templated) | Ensure `.kernl/` is not ignored |
| `internal/backend/bdcli.go` | Confirm `bd comment` is exposed; add if missing |
| `docs/VISION.md` §10 or §11 | Optional: document the `.kernl/<bead_id>/` convention as part of the substrate |

**Migration & rollback:**

- Pure addition; no existing behavior changes.
- Rollback: stop calling `Backend.Comment` with the structured record.
  Worktrees keep `.kernl/` directories regardless.

**Out of scope for Change 3:**

- A UI surface for browsing stage artifacts (UI work, separate plan).
- Cross-bead artifact references (e.g., bead B's planning reads bead A's
  artifacts) — wait for a real use case.
- Compressing or pruning old artifacts from merged epics.

---

## 3. What this plan deliberately does NOT do

These are tempting adjacent changes that are **out of scope** for this plan.
They should be evaluated separately:

- **No mail/messaging layer.** The gastown analysis (separate
  conversation) concluded that mail makes sense for long-running roles
  (Witness, Sweeper, Releaser) that don't exist yet in kernl. Adding
  messaging now would be solving a problem we don't have.
- **No Agent Mail (mcp_agent_mail) integration.** Same reason. Revisit
  when parallel-bead coordination becomes a measurable pain (the
  kernl-swarm-parallel-execution-bug class).
- **No new agent roles** (Witness, Releaser, Sweeper). VISION §8.1 names
  them; they belong in their own plan once the core pipeline is reliable.
- **No DA integration** with stage artifacts. The DA reading `.kernl/`
  artifacts to summarize epic progress is a great use of the substrate, but
  is a separate UI/DA story.
- **No workflow shape registry / marketplace** (VISION §8.2 community
  shared). Custom workflows authoring is enabled by Change 2; distribution
  is a later concern.
- **No automatic gate authoring.** The canonical-vibe-coding shape gets its
  contracts hand-authored once; we do not build a tool that derives
  contracts from bead types.

## 4. Order of execution

```
Change 1  ──►  Change 2  ──►  Change 3
(orchestrator    (per-stage     (artifact
 owns            contracts)     handoff)
 transitions)
```

- **Change 1** is independently valuable: deletes `retryStuckStage`,
  eliminates the most common operational failure, requires no descriptor
  changes from users.
- **Change 2** depends on Change 1 only because the new contract assumes
  the orchestrator (not the agent) owns the transition — otherwise the
  contract would need to keep the END-OF-STAGE PROTOCOL section.
- **Change 3** depends on Change 2 because the artifact paths are defined
  in the stage contract.

Each change should be a separate PR / epic in bd. Land Change 1 and live
on it for at least one full epic run before starting Change 2.

## 5. Sequential implementation steps

This plan is executed sequentially by a single implementer (not decomposed
into beads). Complete every step in a change before moving to the next
step; complete every step in a change before starting the next change.

### Change 1 — Orchestrator owns the full transition

1. **Add the `ExitGate` field** to `WorkflowTransition` (or a sibling map
   on `WorkflowDescriptor` if cleaner) in `internal/backend/port.go`.
   Allowed values: `"agent_exit_zero"` (default) and `"artifact_exists"`.
   Unknown values are a load-time error.
2. **Implement `EvaluateExitGate`** in
   `internal/backend/state_machine.go`. Signature:
   `func EvaluateExitGate(wf WorkflowDescriptor, fromState, worktreePath, beadID string) (passed bool, reason string)`.
   For `agent_exit_zero`: always `true`. For `artifact_exists`: requires
   a per-state path template (allowed placeholder `<bead_id>`); returns
   `false, "artifact_missing: <resolved-path>"` if the file does not
   exist relative to `worktreePath`.
3. **Wire post-spawn evaluation** in `internal/app/drive_bead.go`. After
   `deps.Driver.RunBead` returns with `res.Success == true`, call
   `EvaluateExitGate`. If passed → `deps.Backend.Update(beadID,
   UpdateBeadInput{State: nextState, …})`. If failed → `Update` to
   state `blocked` with a `bd comment` containing `gate_failed: <reason>`.
4. **Make the orchestrator's `Update` idempotent for the transition
   window**: if the backend reports "already in target state", treat
   as success and continue. Add a unit test for this.
5. **Delete `retryStuckStage`** and its callers in
   `internal/app/drive_bead.go`. Genuine work-failure retries (gate
   failed, tests broke) are a different code path and stay; only the
   "agent exited rc=0 but didn't call bd update" branch is removed.
6. **Delete the END-OF-STAGE PROTOCOL section** (lines 69–81 today) from
   `internal/app/prompt.go`. Add a single new line under "Operating
   rules": `Do not run `bd update`, `bd close`, or `bd open`. The
   orchestrator advances the bead when your stage completes.`
7. **Restrict bd-mutating commands** in the opencode permission config
   injected by `injectOpencodeConfigEnv` (find current location via
   `grep -rn injectOpencodeConfigEnv internal/`). `bd update`, `bd
   close`, `bd open` get rejected. `bd show`, `bd list`, `bd comment`
   stay allowed.
8. **Write the tests listed in §2.Change 1 Tests**. All hermetic tests
   must pass; integration test must pass under
   `go test -tags=integration ./internal/integration/...`.
9. **Run quality gates**: `go vet ./...`, `go test ./...`,
   `go test -tags=integration ./...`. All must pass.

### Change 2 — Per-stage contracts

10. **Add `StageContract` and `Stages map[string]StageContract`** to
    `WorkflowDescriptor` in `internal/backend/port.go`. `StageContract`
    fields: `Role string`, `Inputs []string`, `OutputArtifact
    StageArtifact`, `ForbiddenPaths []string`. `StageArtifact` fields:
    `Path string` (template, supports `<bead_id>`), `Kind string`
    (`"file"` or `"commits"`), `CommitMarker string` (when
    `Kind=="commits"`), `MustEndWith string` (optional).
11. **Extend the YAML loader** (locate via
    `grep -rn 'yaml.Unmarshal\|yaml.Decoder' internal/`) to parse the
    `stages:` block. Unknown fields are a load-time error.
12. **Rewrite `BuildBeadStagePrompt`** in `internal/app/prompt.go` as a
    contract renderer. New prompt structure (in this exact order):
    1. `# Bead <ID> — <Title>` (unchanged)
    2. `## Role` — verbatim from `stages.<state>.role`. Fallback if
       contract missing: today's generic engineer text.
    3. `## Inputs available to you` — bullet list from `stages.<state>.inputs`,
       with file paths resolved (e.g. `.kernl/<bead_id>/plan.md` →
       resolved path).
    4. `## Required output` — describes the artifact (path or
       commit marker + any `must_end_with` constraint).
    5. `## You may NOT` — forbidden paths + the bd-mutation prohibition
       from step 6.
    6. `## Operating rules` — existing rules 1–6 from today (worktree,
       AGENTS.md, tests, commits); rule 7 (no bd-mutation) moves to §5.
    7. `## Bead data` — the bead's description and acceptance, framed
       as data not instructions.
    8. `## Bead metadata` (unchanged).
13. **Translate `forbidden_paths` into the opencode permission config**
    in the same file as step 7. Writes to those globs get rejected at
    the sandbox layer, not just discouraged in text.
14. **Author the canonical-vibe-coding stage contracts**: add the
    `stages:` block to the builtin SDLC descriptor in
    `internal/backend/factory.go` (find via
    `grep -n BuiltinWorkflowDescriptors`). Stages to author: `planning`,
    `plan_review`, `implementation`, `implementation_review`,
    `integration`, `integration_review`, `shipment`, `shipment_review`.
    Use the schema from §2.Change 2's YAML example as the source of truth
    for `planning` / `implementation` / `plan_review`; write the rest by
    analogy.
15. **Write the tests listed in §2.Change 2 Tests**. All must pass.
16. **Run quality gates** as in step 9.

### Change 3 — Artifact-based handoff

17. **Hook a `Backend.Comment` call** in `internal/app/drive_bead.go`
    after each successful stage advancement (post step 3's success
    branch). Comment body format:
    ```
    stage: <state>
    agent: <agentID>
    session_id: <sessionID>
    artifact: <resolved-artifact-path-or-commit-marker>
    commit: <HEAD-sha-of-worktree>
    duration: <Xs>
    ```
    Find or add a `Comment(beadID, body, repoPath) error` method on the
    backend interface in `internal/backend/port.go`.
18. **Confirm `bd comment` is invoked** by the `BdCliBackend`
    implementation (`internal/backend/bdcli.go`). If the method is
    missing, add it; if present, wire it through.
19. **Ensure `.kernl/` is not gitignored** in the worktree creation
    path. Locate via `grep -rn 'gitignore\|\.gitignore' internal/worktree/`.
    If the worktree starts from a template that ignores hidden dirs,
    explicitly un-ignore `.kernl/` (e.g. `!.kernl/`).
20. **Write the tests listed in §2.Change 3 Tests**. All must pass.
21. **Final quality gates**: `go vet ./...`, `go test ./...`,
    `go test -tags=integration ./...`. All must pass. Run the full
    plan acceptance check from §6.

## 5a. Implementation rules (non-negotiable)

These rules apply to **every** step above. Violations should be reverted
before commit.

1. **Sequential.** Complete every step in a change before moving on.
   Complete every change in full (code + tests passing) before starting
   the next change. Do not interleave.
2. **No scope creep.** If a step does not appear above, do not implement
   it. If you notice unrelated cleanup opportunities, note them in a
   comment in your final report; do not act on them.
3. **No premature abstraction.** If a gate type or stage field is not
   listed in this plan, do not add it "for future use". The plan
   intentionally ships the minimum.
4. **Delete what the plan says to delete.** `retryStuckStage`, the
   END-OF-STAGE PROTOCOL section, the agent-side bd-update language —
   these are not optional removals. The success criterion §6.8 requires
   that lines deleted > lines added; if you keep dead code "just in
   case", you have failed the criterion.
5. **Match existing style.** Read the surrounding 200 lines before
   adding code. Match naming, error wrapping (`KERNL DISPATCH FAILURE:
   <problem> — Fix: <action>`), test naming (`TestFoo_DoesBar`), file
   layout.
6. **Hermetic tests.** New `*_test.go` tests use fakes/stubs, no
   network, no real disk outside `t.TempDir()`. Integration tests
   behind `//go:build integration`.
7. **Files < 500 lines, functions 4–40 lines.** From `AGENTS.md`. If a
   change pushes a file over 500, split before committing.
8. **No comments restating code.** Only WHY a non-obvious decision was
   made (e.g. "idempotent because the agent may still try to update
   during the migration window"). Never WHAT the code does.
9. **If genuinely ambiguous, stop and ask.** Do not invent a behavior
   the plan didn't specify. Examples of legitimate ambiguity: the exact
   YAML key for `ExitGate` (top-level vs nested under transitions); the
   exact format the existing opencode permission config uses. Examples
   of non-ambiguity: the names of new fields, the structure of the new
   prompt — these are specified above.
10. **Commit per step.** One commit per numbered step, with a
    conventional message: `feat(<area>): <step summary>` or
    `refactor(<area>): <step summary>`. This makes the change reviewable
    and revertable.

## 6. Acceptance criteria for the plan as a whole

The plan is successful when all of the following hold simultaneously:

1. `retryStuckStage` does not exist in the codebase.
2. `BuildBeadStagePrompt` does not contain the strings "END-OF-STAGE
   PROTOCOL" or "bd update --status".
3. Running `go run ./cmd/kernl epic run <some-real-epic>` on a workflow
   with fully-authored stage contracts completes end-to-end without any
   agent invoking `bd update`.
4. A planning agent that attempts to write a `.go` file in a worktree is
   rejected at the sandbox layer; the bead does not advance; the bead is
   marked `blocked` with `artifact_missing` if it never wrote
   `.kernl/<id>/plan.md`.
5. `bd show <bead-id>` for a completed bead shows one structured stage
   comment per advancement, with agent, session, and artifact path.
6. A user-authored custom workflow YAML with a new stage role runs
   without modifying any Go code.
7. `go test ./...` and `go test -tags=integration ./...` pass.
8. The total Go lines deleted (from `retryStuckStage`, the
   END-OF-STAGE protocol section, the agent's bd-update logic) is
   greater than the lines added for stage contracts. (Sanity check that
   we shipped simplification, not just addition.)

## 7. Risks & mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| A workflow YAML in the wild has a stage that genuinely needs to mutate bd (e.g. a workflow author's stage explicitly calls `bd comment` and we over-restrict). | Low | The restriction is on `bd update`/`close`/`open` only. `bd comment` and `bd show`/`list` remain allowed. |
| Sandbox path restrictions block legitimate writes (e.g. planner needs to create scaffolding directories under `src/` as part of the plan format). | Medium | The `forbidden_paths` is per-stage and authored; if scaffolding is part of planning's contract, it's allowed by not listing those paths. The canonical pipeline's planner does not write source code by design. |
| An `agent_exit_zero` gate over-advances when the agent actually failed but exited cleanly anyway. | Medium | Same risk exists today (with `retryStuckStage` not catching this case either). Change 2's `artifact_exists` gate is the real fix. Change 1 ships with the weaker default to stay behavior-preserving; aggressive gates are authored in Change 2. |
| Fallback "agent ran `bd update` themselves" during the migration window introduces double-advancement bugs. | Low | Make the orchestrator's `bd update` idempotent (already-at-target → no-op). Test explicitly. |
| Stage contracts grow unwieldy YAML over time. | Medium | The contract has a fixed small shape (role, inputs, output, forbidden, gate). Schema enforcement at load time; reject unknown fields. |

## 8. References

- **Code:**
  - `internal/app/prompt.go:21-95` — current `BuildBeadStagePrompt`
  - `internal/app/drive_bead.go:131-147` — current claim-side advancement
  - `internal/app/drive_bead.go:202-265` — `retryStuckStage` (to be deleted)
  - `internal/dispatch/dispatch.go` — agent selection (unchanged by this plan)
  - `internal/backend/port.go:47-72` — `WorkflowDescriptor` and
    `WorkflowTransition`
  - `internal/backend/state_machine.go:402, 471` — transition helpers
- **Specs:**
  - `orchestrator/specs/prompt/prompt.md` §1.3 — original foolery skill
    prompt contract spec; this plan finishes implementing it
  - `orchestrator/specs/00-architecture.md` — overall orchestrator shape
- **Product:**
  - `docs/VISION.md` §8.1 — orchestrator stages
  - `docs/VISION.md` §8.2 — declarative workflow shapes (canonical vs DIY)
  - `docs/STRATEGY.md` — MVP focus and metrics
- **External (read-only):**
  - `~/repositories/_cloned/gastown/docs/design/mail-protocol.md` — source
    of the ephemeral/persistent and roles-vs-stages principles imported
    into §1.3
  - `~/repositories/_cloned/mcp_agent_mail/SKILL.md` — for future
    parallel-bead coordination work (out of scope here)
