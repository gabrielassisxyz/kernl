# Yegge Loop — Iterative Plan Refinement

> This reference defines the iterative refinement loop used by `vc-writing-plans` and `vc-orchestrator`.
> Creator persona: **Kimi K2.6**
> Reviewer persona: **Kimi K2.6** (self-critique or subagent)

## Model Assignment

- **Plan creation:** Kimi K2.6 — thorough, structured, TDD-focused. Reasons holistically about dependencies. Catches architectural gaps early. Prefers explicit over clever. Every phase has verifiable success criteria.
- **Plan review (Yegge Loop):** Kimi K2.6 — meticulous, contrarian, hunts edge cases, challenges assumptions. Uses 7-dimension rubric.
- **Task breakdown (post-approval):** GLM-5.1 — mechanically precise, exhaustive, granular. Produces bite-sized executable tasks.
- **Task review:** Kimi K2.6 — checks dependency ordering, completeness, granularity, test coverage.

## Loop Protocol

### Iteration 1 — WRITE
1. Read the original brainstorm spec and all review outputs (STRATEGY.md, review artifacts).
2. Produce the best first draft you can.
3. STOP. Do not keep writing.
4. Present the draft to the user.
5. Ask for feedback: "What should change? What's missing? What's unclear?"

### Iterations 2+ — REFINE

```
COLLECT → CONTRADICTION CHECK → SELF-CRITIQUE → SYNTHESIZE → GATE
```

1. **COLLECT** — Gather user feedback from the previous iteration.
2. **CONTRADICTION CHECK** — Compare proposed changes against the original brainstorm spec. Flag any contradictions.
   - If contradiction found: STOP. Explain the conflict. Ask user which direction to take.
   - If no contradiction: proceed.
3. **SELF-CRITIQUE** — Run the 7-dimension review on the current draft:
   - **TDD coverage** — Does every phase have verifiable success criteria?
   - **Quality gates** — Are phase boundaries sharp and unambiguous?
   - **Dependency gaps** — Are hidden dependencies surfaced?
   - **Missing test scenarios** — Are error paths, edge cases, and rollback covered?
   - **Scope creep** — Has the plan grown beyond the brainstorm spec?
   - **Token density** — Is the plan lean with no filler?
   - **Bead structural validity** — Does every task have a complete Bead Mapping block? Is the dependency graph acyclic? Are priorities consistent with dependencies? Are acceptance criteria verifiable?
4. **SYNTHESIZE** — Produce revised draft incorporating feedback + self-critique findings.
5. **GATE** — Present revised draft. Ask: "Approve or continue refining?"

### Iteration Rules
- **Minimum 5 iterations.**
- Continue beyond 5 if the user provides feedback.
- Stop immediately on contradiction — explain the conflict and ask the user.
- Preserve document structure:
  - Goal
  - Current Phase
  - Phases
  - Key Questions
  - Decisions Made
  - Errors Encountered
  - Notes
- Track deltas in the **Notes** section (what changed this iteration).

## Reviewer Modes

### Inline Self-Critique (default)
Perform the 7-dimension rubric yourself. Be ruthless. Name specific gaps, not vague concerns.

### Subagent Reviewer (if platform supports it)
Spawn a subagent with this prompt:

```
You are Kimi K2.6 reviewing a plan. Be meticulous, contrarian, and hunt for edge cases.
Review this plan against the original spec. Check:
1. TDD coverage — verifiable success criteria per phase?
2. Quality gates — sharp phase boundaries?
3. Dependency gaps — hidden dependencies surfaced?
4. Missing test scenarios — error paths, edge cases, rollback?
5. Scope creep — grown beyond brainstorm spec?
6. Token density — lean, no filler?
7. Bead structural validity — every task has Bead Mapping? dependency graph acyclic? priorities consistent? acceptance criteria verifiable?

Return findings as a numbered list. Be ruthless. No compliments.
```

Incorporate subagent findings into the next iteration.

## Skip Conditions

Skip the Yegge Loop early ONLY if:
- The user explicitly says "this plan is good, proceed"
- The plan has 2-3 phases AND the user confirms clarity in one sentence
- Invoked from an automated pipeline (non-interactive)

## GLM-5.1 Task Breakdown (post-approval)

After the user approves the plan:

1. Read the approved `docs/PLAN.md`.
2. Spawn a subagent with GLM-5.1 persona:
   ```
   You are GLM-5.1 — mechanically precise, exhaustive, granular.
   Break this approved plan into bite-sized tasks where each task is one action (2-5 minutes).
   For each task: exact file path, specific command, expected output, commit message.
   Principles: DRY. YAGNI. TDD.
   ```
3. Subagent outputs: `docs/plans/YYYY-MM-DD-<feature>-tasks.md`
4. Kimi K2.6 reviews the task breakdown for:
   - Dependency ordering (are tasks in the right sequence?)
   - Completeness (did GLM-5.1 miss any plan phases?)
   - Granularity (are tasks truly bite-sized?)
   - Test coverage (is TDD preserved in task form?)
   - Bead graph validity (complete Bead Mapping blocks? acyclic graph? valid epic relationships?)
5. User approves final task list.
