---
name: vc-orchestrator
description: 'Orchestrate the full vibe-engineering-mastery pipeline: strategy → reviews → plan → Yegge Loop → tasks. Use when you want to run the full planning pipeline, start the orchestrator, do the complete review chain, or when multiple stakeholders need to weigh in. Also triggers when users say let us plan this properly or run reviews before we build. Use for "strategic planning" or "product review" sessions.'
user-invocable: true
---

# VC Orchestrator

This skill is the **entry point** for `vibe-engineering-mastery`. It chains all 8 skills into a single interactive pipeline, producing durable artifacts at every stage. You act as the moderator — the user makes all decisions. You dispatch, gate, and preserve.

## Rules (apply to every phase)

1. **Detect before dispatching** — scan `docs/` first. Never blindly invoke.
2. **Never skip human gates** — every phase transition is an `AskUserQuestion`.
3. **Preserve artifacts** — never overwrite without asking. If an artifact exists, offer: resume, rerun, or skip.
4. **Respect chain order** — strategy before reviews, reviews before plans, plans before tasks.
5. **End cleanly** — confirm all artifact locations when the pipeline finishes.

## Phase 0: Artifact Detection

When invoked, scan `docs/` for existing artifacts. Match against these patterns:

```
docs/PLAN.md                          → Yegge Loop in progress? Resume?
docs/plans/YYYY-MM-DD-*-plan.md       → Plan draft exists. Resume refinement?
docs/plans/YYYY-MM-DD-*-tasks.md      → Tasks already generated. Resume task review?
docs/reviews/vc-plan-*-review-*.md    → Some reviews complete. Continue or skip to planning?
docs/reviews/vc-autoplan-*.md         → Autoplan complete. Continue to plan writing?
docs/STRATEGY.md                      → Strategy captured. Proceed to reviews?
```

**Routing logic:**

- If `docs/PLAN.md` exists and was last modified recently → "A plan exists at `docs/PLAN.md`. Would you like to resume the Yegge Loop, start fresh, or review the existing plan?"
- If `docs/plans/YYYY-MM-DD-*-plan.md` exists (plan draft) → read it, summarize the feature and current phase, then: "A plan draft exists for [feature]. Resume plan refinement, start Yegge Loop, or begin task breakdown?"
- If `docs/STRATEGY.md` exists plus some reviews → read STRATEGY.md, summarize the target problem and completed reviews, then: "Strategy and [N] reviews complete. Proceed to plan writing, run remaining reviews, or re-run a review?"
- If `docs/STRATEGY.md` exists, no reviews → read STRATEGY.md, summarize, then: "Strategy captured for [problem]. Proceed to plan reviews? (autoplan / individual reviews / skip to plan writing)"
- If nothing found → "No artifacts detected. Let's start with strategy. Proceed to `/vc-strategy`?"

Always present the user with clear options. Never auto-advance.

## Phase 1: Strategy (`/vc-strategy`)

**When:** No `docs/STRATEGY.md` exists, or user requests re-run.

**Action:**
Invoke the `/vc-strategy` skill. This runs an interactive interview and writes `docs/STRATEGY.md`.

**Gate after completion:**
"Strategy captured at `docs/STRATEGY.md`. Proceed to plan reviews? (yes / edit strategy / stop)"

If user says "edit", re-invoke `/vc-strategy` for updates. If "stop", confirm artifact is saved and exit.

## Phase 2: Routing Decision

After strategy is confirmed, present the routing choice:

**AskUserQuestion:** "How would you like to proceed with plan reviews?"
- A) **Run autoplan** (fast) — all 4 reviews run autonomously, taste decisions surfaced at a final approval gate
- B) **Go one by one** (thorough) — step through CEO, Eng, Design, DevEx reviews interactively
- C) **Skip reviews** — proceed directly to plan writing

If A → invoke `/vc-autoplan` (Phase 2A)
If B → proceed to Phase 2B
If C → proceed to Phase 3

## Phase 2A: Autoplan (`/vc-autoplan`)

**When:** User selects option A.

**Action:**
Invoke the `/vc-autoplan` skill. This reads the 4 review skills from disk, runs them sequentially with auto-decision principles, and surfaces taste decisions at a final gate.

**Output:** `docs/reviews/vc-autoplan-YYYY-MM-DD.md`

**Gate after completion:**
"Autoplan complete. Report saved to `docs/reviews/vc-autoplan-YYYY-MM-DD.md`. Proceed to plan writing? (yes / run individual reviews instead / edit a specific finding)"

## Phase 2B: Individual Reviews (Interactive)

**When:** User selects option B.

Present the review checklist:

```
Plan Reviews — Select which to run:
☐ CEO Review     (scope, product thinking, 4 modes)
☐ Eng Review     (architecture, data flow, edge cases, tests)
☐ Design Review  (UX 0-10, mockups, interaction states)
☐ DevEx Review   (developer persona, TTHW, magical moments)
```

**For each selected review:**

1. **Read the review skill** from `vibe-engineering-mastery/skills/vc-plan-<type>-review/SKILL.md`.
2. **Step through inline** — the review skill will issue its own `AskUserQuestion` gates. Follow them. Do not race ahead.
3. **Save output** to `docs/reviews/vc-plan-<type>-review-YYYY-MM-DD.md` when the review completes.
4. **Gate after each review:** "`vc-plan-<type>-review` complete. Saved to `docs/reviews/vc-plan-<type>-review-YYYY-MM-DD.md`. Run next review, re-run this one, or skip to planning?"

Continue through all selected reviews. Track which are done and which are skipped.

## Phase 3: Plan Synthesis

**When:** All selected reviews are complete (or user chose skip), and no unified draft exists.

**Action:**
1. Read `docs/STRATEGY.md`.
2. Read all available review outputs from `docs/reviews/`.
3. Synthesize a unified draft merging: strategy context, review findings (by stakeholder), key decisions, scope boundaries, and architectural direction.
4. Write the unified draft to `docs/PLAN.md` (DRAFT status).

The draft should follow this structure:
```
# [Feature Name]: DRAFT Plan

> Status: DRAFT — pending Yegge Loop refinement.

## Strategy Summary
(brief from STRATEGY.md)

## Review Findings
### CEO Review
(key findings, mode selected, scope decisions)
### Eng Review
(key findings, architectural decisions, risk areas)
### Design Review
(key findings, UX scores, unresolved design decisions)
### DevEx Review
(key findings, persona, magical moment)

## Unified Direction
(synthesized from all reviews)

## Open Questions
(from all reviews)
```

**Gate:** "Unified draft written to `docs/PLAN.md`. Proceed to detailed plan writing? (yes / edit draft first)"

## Phase 4: Detailed Plan Writing (`/vc-writing-plans`)

**When:** Unified draft is ready.

**Action:**
Invoke the `/vc-writing-plans` skill. Pass the unified `docs/PLAN.md` as the grounding document.

**Output:** `docs/plans/YYYY-MM-DD-<feature>-plan.md` (Kimi K2.6 persona creates the detailed plan).

**Gate:** "Detailed plan written to `docs/plans/YYYY-MM-DD-<feature>-plan.md`. Begin Yegge Loop refinement? (yes / review plan first / edit)"

## Phase 5: Yegge Loop

**When:** Detailed plan exists and user confirms.

**Reference:** Read `references/yegge-loop.md` for the full protocol.

**Summary of the loop:**
- **Creator persona:** Kimi K2.6 — thorough, structured, TDD-focused, reasons holistically about dependencies, catches architectural gaps early, explicit over clever.
- **Reviewer persona:** Kimi K2.6 (self-critique or subagent) — meticulous, contrarian, hunts edge cases, challenges assumptions.
- **Iteration 1:** WRITE best first draft. STOP. Present. Ask for feedback.
- **Iterations 2+:** REFINE — COLLECT feedback → CONTRADICTION CHECK → SELF-CRITIQUE (7 dimensions) → SYNTHESIZE → GATE.
- **7 dimensions:** TDD coverage, Quality gates, Dependency gaps, Missing test scenarios, Scope creep, Token density, Bead structural validity.
- **Minimum 5 iterations.** Continue beyond 5 if user has feedback.
- **Subagent reviewer pattern:** If platform supports subagent dispatch, spawn Kimi K2.6 reviewer. Else, perform inline 7-dimension self-critique.
- **Gate after each iteration:** "Iteration [N] complete. Approve plan or continue refining? If refining, what specific dimension needs work?"

**Output:** Final approved `docs/PLAN.md`.

**Skip conditions:**
- User explicitly says "this plan is good, proceed"
- Plan has 2-3 phases AND user confirms clarity in one sentence
- Invoked from automated pipeline

**Gate:** "Plan approved and saved to `docs/PLAN.md`. Proceed to task breakdown? (yes / edit plan / stop)"

## Phase 6: GLM-5.1 Task Breakdown

**When:** Plan is approved.

**Action:**
1. Read the approved `docs/PLAN.md`.
2. Spawn a subagent with GLM-5.1 persona:
   > "You are GLM-5.1 — mechanically precise, exhaustive, granular. Break this plan into bite-sized tasks where each TASK is one action (2-5 minutes). You are NOT creatively reinterpreting the plan. You are mechanically decomposing it while preserving ALL bead-mapping metadata.
   
   Rules:
   1. Every output task MUST include a complete `**Bead Mapping:**` block.
   2. If a task is atomic (2-5 minutes), keep it EXACTLY as-is. Do not split atomic tasks. Preserve Type, Priority, Dependencies, and Parent verbatim.
   3. If a task is too large to be atomic (e.g., 'Build the entire authentication system'), split it into multiple atomic children. The parent becomes an `epic`. Children reference the epic as their Parent and inherit its priority as their base priority. Children do NOT inherit the parent's Dependencies — each child declares its own explicit Dependencies.
   4. When splitting an epic, distribute the parent's Estimated Minutes across children (sum of children = parent's original estimate). Set the epic's Estimated Minutes to 0 (it is a container).
   5. Acceptance criteria MUST remain verifiable. Do not soften or vague-ify them.
   6. The dependency graph MUST remain acyclic. Verify this before outputting.
   7. Do not add dependencies that are not explicitly declared. Do not remove declared dependencies.
   
   For each task: exact file path, specific command, expected output, commit message. DRY. YAGNI. TDD. Preserve dependency ordering from the plan."
3. Subagent reads `docs/PLAN.md` and produces the task list.
4. Save output to `docs/plans/YYYY-MM-DD-<feature>-tasks.md`.

**Output format:**
```
# [Feature Name]: Task Breakdown

> Generated by GLM-5.1 from approved plan `docs/PLAN.md`.

## Tasks

### Phase 1: [Phase Name]
- [ ] **Task 1.1:** [One-line description]
  - File: `path/to/file.ext`
  - Command: `exact command`
  - Expected: [what to verify]
  - Commit: `type(scope): brief message`

... (repeated for all tasks)
```

**Gate:** "Task breakdown saved to `docs/plans/YYYY-MM-DD-<feature>-tasks.md`. Proceed to Kimi K2.6 task review? (yes / edit tasks first)"

## Phase 7: Kimi K2.6 Task Review

**When:** Task breakdown exists.

**Action:**
Read `docs/plans/YYYY-MM-DD-<feature>-tasks.md` and review for:

1. **Dependency ordering** — are tasks in the right sequence? Do dependencies match the plan? Is the dependency graph acyclic?
2. **Completeness** — did GLM-5.1 miss any plan phases or decisions?
3. **Granularity** — are tasks truly bite-sized (2-5 min each)? Flag any that are too large.
4. **Test coverage** — is TDD preserved in task form? Are test tasks paired with implementation tasks?
5. **Bead graph validity** — does every task have a complete `**Bead Mapping:**` block? Are priorities consistent with dependencies? Are all dependency references resolvable? Are epics properly declared?

Present findings with specific recommendations. Offer to apply fixes to the task file.

**Gate:** "Task review complete. [N] issues found, [N] recommendations. Apply fixes, approve as-is, or edit manually?"

After any fixes are applied, final gate:

"Task list approved. Proceeding to bead conversion."

## Phase 8: Bead Conversion & Handoff

**When:** Task list is approved.

**Action:**
Present a summary of the approved task list and ask:

**Gate:** **"Convert this plan to a bead graph? (yes / save for later)"**

If save for later → confirm artifact locations and end cleanly.

If yes:
1. Dispatch `/vc-convert-plan-to-beads` with:
   - Input: `docs/plans/YYYY-MM-DD-<feature>-tasks.md`
   - Output: `beads/YYYY-MM-DD-<feature>-plan.json`
2. If the converter reports a validation failure (cycle detected, unresolved dependency, priority contradiction, schema violation):
   - Report the specific failure to the user.
   - Ask: **"Fix the plan and re-run conversion, or save for later?"**
   - If fix → loop back to Phase 7 (Kimi K2.6 task review) with the specific failures as review items.
   - If save for later → confirm artifacts and end cleanly.
3. If the converter reports success, proceed to artifact announcement.

**Announce all final artifact locations:**

```
Pipeline complete. Artifacts:

  docs/STRATEGY.md                              — Strategy
  docs/reviews/                                 — Review outputs
  docs/PLAN.md                                  — Approved plan
  docs/plans/YYYY-MM-DD-<feature>-plan.md       — Detailed plan
  docs/plans/YYYY-MM-DD-<feature>-tasks.md      — Task breakdown
  beads/YYYY-MM-DD-<feature>-plan.json          — Bead graph
```

**Suggest next steps:**
- Load bead graph with `bd create --graph beads/YYYY-MM-DD-<feature>-plan.json` when ready to execute
- Invoke `vc-orchestrator` again for a new feature

**End cleanly.** Confirm all artifacts are saved and the pipeline is complete.

## Error Recovery

- **Skill invocation fails:** Report the error. Ask user: retry, skip that phase, or abort.
- **Artifact conflict:** If output file exists, ask: overwrite, save with suffix, or rename.
- **Subagent unavailable:** Fall back to inline execution with the same persona prompt.
- **User wants to go back:** Restart from the phase they specify. Preserve existing artifacts.

## Invocation Points

Users can invoke this skill at any pipeline stage:

```
/vc-orchestrator              → Full pipeline from Phase 0 detection
/vc-orchestrator --from reviews  → Start from Phase 2 routing decision
/vc-orchestrator --from plan     → Start from Phase 4 (detailed planning)
/vc-orchestrator --from tasks    → Start from Phase 6 (task breakdown)
```

The `--from` flag skips detection and routes directly to the named phase. Always confirm the user's intent before proceeding from a flagged entry point.
