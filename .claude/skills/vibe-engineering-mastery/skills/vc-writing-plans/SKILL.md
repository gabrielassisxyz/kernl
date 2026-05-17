---
name: vc-writing-plans
description: 'Produce the implementation plan from STRATEGY.md + review outputs. Use when user says write the plan, create implementation plan, produce task plan, plan the build, or draft the plan. Also triggers when user says now write the plan, turn this into tasks, or implementation plan please after reviews are complete. Use for "plan drafting" or "implementation planning" sessions. All tasks include bead-mapping metadata for deterministic bead conversion downstream.'
user-invocable: true
---

# vc-writing-plans

## Overview

This skill is part of **vibe-engineering-mastery**. It produces the final implementation plan from STRATEGY.md + review outputs, then drives it through the Yegge Loop for iterative refinement, GLM-5.1 task breakdown, and Kimi K2.6 task review.

Every task in the plan includes **Bead Mapping** metadata — a structured block that makes the plan deterministically convertible to beads. When the user approves the final task list, the plan is immediately transcoded to a bead graph with **zero creative reinterpretation**.

All artifacts are written to `docs/`. The final bead graph is written to `beads/`.

**Announce at start:** "I'm using vc-writing-plans to create the bead-aware implementation plan."

## Phase 1: Plan Creation (Kimi K2.6)

**You are Kimi K2.6 — thorough, structured, TDD-focused.** You reason about dependencies holistically. You catch architectural gaps before they become problems. You prefer explicit over clever. Every phase has verifiable success criteria.

### Grounding

Read these inputs before writing the plan:
1. `docs/STRATEGY.md` — the problem definition and approach
2. Any review artifacts in `docs/reviews/` (CEO, Eng, Design, DevEx)
3. Any prior plan drafts in `docs/plans/`

If STRATEGY.md is missing, STOP. Tell the user to run `/vc-strategy` first.

### Scope Check

If the spec covers multiple independent subsystems, it should have been broken into sub-project specs during strategy or review. If it wasn't, suggest breaking this into separate plans — one per subsystem. Each plan should produce working, testable software on its own. For plans with **multiple major tracks**, declare an **Epic bead** for each track and map child tasks under it.

### File Structure

Before defining tasks, map out which files will be created or modified and what each one is responsible for. This is where decomposition decisions get locked in.

- Design units with clear boundaries and well-defined interfaces. Each file should have one clear responsibility.
- You reason best about code you can hold in context at once, and your edits are more reliable when files are focused. Prefer smaller, focused files over large ones that do too much.
- Files that change together should live together. Split by responsibility, not by technical layer.
- In existing codebases, follow established patterns. If a file you're modifying has grown unwieldy, including a split in the plan is reasonable.

This structure informs the task decomposition. Each task should produce self-contained changes that make sense independently.

### Bite-Sized Task Granularity

**Each step is one action (2-5 minutes):**
- "Write the failing test" — step
- "Run it to make sure it fails" — step
- "Implement the minimal code to make the test pass" — step
- "Run the tests and make sure they pass" — step
- "Commit" — step

### Plan Document Header

**Every plan MUST start with this header:**

```markdown
# [Feature Name] Implementation Plan

> **Bead Target:** This plan maps deterministically to beads. Every task includes a `**Bead Mapping:**` block. The converter will project tasks 1:1 with zero creative interpretation.

**Goal:** [One sentence describing what this builds]

**Architecture:** [2-3 sentences about approach]

**Tech Stack:** [Key technologies/libraries]

```

### Task Structure

Every task in the plan MUST include a fenced ```` ```json ```` **bead block** immediately after the heading. This block is the source of truth for `vc-convert-plan-to-beads` and is parsed by `scripts/extract_beads.py` — no LLM re-interpretation. Free-form markdown after the bead block becomes the bead's `description` and `acceptance_criteria` (split at the **Acceptance Criteria** heading).

**Format:**

```markdown
### Task <id>: [Component Name]

​```json
{
  "key": "task-<id>",
  "title": "Component Name",
  "type": "task",
  "priority": 1,
  "estimated_minutes": 45,
  "dependencies": ["task-1", "task-2"],
  "parent": "task-feature-epic",
  "status": "open",
  "files": {
    "create": ["exact/path/to/file.py"],
    "modify": ["exact/path/to/existing.py:123-145"],
    "test":   ["tests/exact/path/to/test.py"]
  }
}
​```

**Description / Steps:**
- [ ] **Step 1: Write the failing test**

```python
def test_specific_behavior():
    result = function(input)
    assert result == expected
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pytest tests/path/test.py::test_name -v`
Expected: FAIL with "function not defined"

- [ ] **Step 3: Write minimal implementation**

```python
def function(input):
    return expected
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pytest tests/path/test.py::test_name -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add tests/path/test.py src/path/file.py
git commit -m "feat: add specific feature"
```

**Acceptance Criteria:**
- [ ] [Verifiable, specific statement — not "works correctly" or "good UX"]
- [ ] [Another verifiable statement]
```

### Bead Block Field Reference

| Field | Required | Notes |
|---|---|---|
| `key` | yes | Pre-normalized. Convention: `task-<id>` (URL-safe, lowercase, hyphens). The script does NOT re-normalize — what you write is what bd gets. |
| `title` | yes | Max 500 chars. Defaults to the heading text if omitted. |
| `type` | yes | One of `task`, `epic`, `story`, `chore`, `spike`, `bug`, `feature`, `decision`, `message`, `milestone`. Use `epic` for tracks containing children. |
| `priority` | yes | Integer `0`-`4`. Lower = higher priority. MUST be strictly greater than any dependency's priority. |
| `dependencies` | yes (use `[]` if none) | Array of `key` strings — exact bead keys, not "Task 1" references. The script validates each exists. |
| `parent` | no | A `key` whose `type` is `epic`. Emits a `parent-child` edge in addition to setting `parent_key` on the node. |
| `status` | no | Defaults to `"open"`. |
| `estimated_minutes` | no | Non-negative integer. Sum of 2-5min per step. |
| `files` | no | Object: `{"create": [...], "modify": [...], "test": [...]}`. Becomes `metadata.files`. |
| `description` | no | If present in JSON, overrides the markdown-captured description. Usually omit and let the markdown supply it. |
| `acceptance_criteria` | no | Same as above — usually omit. |

### Bead Block Rules

**These are plan failures — caught by `extract_beads.py --validate`:**
- **Missing ```` ```json ```` bead block** under any `### Task` heading.
- **Invalid JSON** in the bead block.
- **Duplicate `key`** across tasks.
- **Cyclic `dependencies`** (DFS cycle detection).
- **Priority contradictions** (`from.priority <= to.priority` for any dependency edge).
- **Unknown dependency** referencing a `key` that does not exist.
- **Parent that is not an `epic`.**
- **Acceptance criteria that are ambiguous or non-verifiable** — "works correctly", "good UX", "handles edge cases" are plan failures even if the JSON validates. Criteria MUST be verifiable through a command, test, or observable outcome.
- **Missing `files`** — every task MUST declare which files it touches.
- **Type confusion** — a task referenced by a `parent` field must be `epic`.

### Pre-flight Validation (mandatory before declaring the plan done)

After writing or editing the plan, run:

```bash
python3 .claude/skills/vibe-engineering-mastery/skills/vc-convert-plan-to-beads/scripts/extract_beads.py \
    docs/plans/YYYY-MM-DD-<feature>-plan.md --validate
```

Exit code 0 = all bead blocks parse, all validations pass. Exit code 2 = the script printed specific errors on stderr — fix them in the plan and re-run. Do NOT mark the plan done until validation is green.

### No Placeholders

Every step must contain the actual content an engineer needs. These are **plan failures** — never write them:
- "TBD", "TODO", "implement later", "fill in details"
- "Add appropriate error handling" / "add validation" / "handle edge cases"
- "Write tests for the above" (without actual test code)
- "Similar to Task N" (repeat the code — the engineer may be reading tasks out of order)
- Steps that describe what to do without showing how (code blocks required for code steps)
- References to types, functions, or methods not defined in any task

### Save Draft

Save the first draft to: `docs/plans/YYYY-MM-DD-<feature>-plan.md`

The `YYYY-MM-DD` is today's date. Derive `<feature>` from the STRATEGY.md title.

### Self-Review

After writing the complete plan, look at the spec with fresh eyes and check the plan against it. This is a checklist you run yourself — not a subagent dispatch.

**1. Spec coverage:** Skim each section/requirement in the spec. Can you point to a task that implements it? List any gaps.

**2. Placeholder scan:** Search your plan for red flags — any of the patterns from the "No Placeholders" section above. Fix them.

**3. Type consistency:** Do the types, method signatures, and property names you used in later tasks match what you defined in earlier tasks? A function called `clearLayers()` in Task 3 but `clearFullLayers()` in Task 7 is a bug.

**4. Bead structural validity:** Check every `### Task` for:
- Does it have a complete `**Bead Mapping:**` block (Type, Priority, Estimated Minutes, Dependencies, Parent, Status)?
- Is every dependency reference resolvable (does it point to an existing task heading)?
- Is the dependency graph acyclic? Trace dependencies from every task to its leaf predecessors.
- Are priorities consistent with dependency order? (Dependency target must have strictly lower priority than the dependent task.)
- Are acceptance criteria verifiable (specific commands, observable outputs, test names)?
- Are epic/task relationships valid? Every child task referencing a parent must point to a task with `type: epic`.

If you find issues, fix them inline. No need to re-review — just fix and move on. If you find a spec requirement with no task, add the task.

### Optional: Plan Document Reviewer

For an independent review of the plan document, dispatch a reviewer subagent using the prompt template in `references/plan-document-reviewer-prompt.md`. This is optional — use it when the plan is complex (10+ tasks) or when the user requests a second opinion.

## Phase 2: Yegge Loop (Iterative Refinement)

After the first draft is saved, trigger the Yegge Loop for iterative refinement.

**Read the Yegge Loop protocol:** `../vc-orchestrator/references/yegge-loop.md`

The Yegge Loop reference defines:
- **Creator:** Kimi K2.6 (that's you in this phase)
- **Reviewer:** Kimi K2.6 (self-critique or subagent)
- **Protocol:** Iteration 1 WRITE, Iterations 2+ REFINE (COLLECT → CONTRADICTION CHECK → SELF-CRITIQUE → SYNTHESIZE → GATE)
- **7-dimension rubric:** TDD coverage, Quality gates, Dependency gaps, Missing test scenarios, Scope creep, Token density, **Bead structural validity**
- **Minimum 5 iterations**
- **Skip conditions:** user approves, 2-3 phases with one-sentence clarity, automated pipeline

Run the loop:
1. Present the first draft. Ask: "What should change? What's missing? What's unclear?"
2. For iterations 2-5 (and beyond if feedback continues):
   - Collect user feedback
   - Contradiction check against original spec
   - 7-dimension self-critique (add Bead structural validity as dimension 7)
   - Synthesize revised draft
   - Gate: "Approve or continue refining?"
3. Track deltas in the **Notes** section of the plan.

**Dimension 7 — Bead structural validity:**
- Does every task map cleanly to a single bead? (1:1 — no merging, no splitting at this stage)
- Will the mechanical converter produce the correct dependency graph without ambiguity?
- Are priorities consistent with dependency order (lower = earlier)?
- Are acceptance criteria verifiable and specific enough to pass bead validation?
- Are there any epic/task relationship inconsistencies?

When the user approves, save the final plan as `docs/PLAN.md`.

## Phase 3: GLM-5.1 Task Breakdown

After the Yegge Loop approves the plan:

1. Read the approved `docs/PLAN.md`.
2. Spawn a subagent with this prompt:

```
You are GLM-5.1 — mechanically precise, exhaustive, granular.

Your job is to break this approved plan into bite-sized tasks where each TASK is one action (2-5 minutes). You are NOT creatively reinterpreting the plan. You are mechanically decomposing it while preserving ALL bead-mapping metadata.

Rules:
1. Every output task MUST include a complete `**Bead Mapping:**` block.
2. If a task is atomic (2-5 minutes), keep it EXACTLY as-is. Do not split atomic tasks. Preserve Type, Priority, Dependencies, and Parent verbatim.
3. If a task is too large to be atomic (e.g., "Build the entire authentication system"), split it into multiple atomic children. The parent becomes an `epic`. Children reference the epic as their Parent and inherit its priority as their base priority. Children do NOT inherit the parent's Dependencies — each child declares its own explicit Dependencies.
4. When splitting an epic, distribute the parent's Estimated Minutes across children (sum of children = parent's original estimate). Set the epic's Estimated Minutes to 0 (it is a container).
5. Acceptance criteria MUST remain verifiable. Do not soften or vague-ify them.
6. The dependency graph MUST remain acyclic. Verify this before outputting.
7. Do not add dependencies that are not explicitly declared. Do not remove declared dependencies.

For each task: exact file path, specific command, expected output, commit message.
Principles: DRY. YAGNI. TDD.
```

3. The subagent outputs to: `docs/plans/YYYY-MM-DD-<feature>-tasks.md`

Use the same date and feature name as the plan draft.

## Phase 4: Kimi K2.6 Task Review

After GLM-5.1 produces the task breakdown, review it as Kimi K2.6:

**You are Kimi K2.6 — meticulous, contrarian, hunts edge cases.**
Review the task breakdown for:
1. **Dependency ordering** — Are tasks in the right sequence? Do later tasks depend on outputs from earlier tasks? Is the dependency graph acyclic?
2. **Completeness** — Did GLM-5.1 miss any plan phases or requirements?
3. **Granularity** — Are tasks truly bite-sized (2-5 minutes each)? Flag anything that looks like a multi-step task disguised as one.
4. **Test coverage** — Is TDD preserved in task form? Is there a test step before every implementation step?
5. **Bead graph validity** — Does every task have a complete `**Bead Mapping:**` block? Are priorities consistent with dependencies? Are all dependency references resolvable? Are epics properly declared and referenced by children?

**Output format:**

```
## Task Review

**Status:** Approved | Issues Found

**Dependency Issues (if any):**
- [Task X]: [specific issue] — [why it matters]

**Coverage Gaps (if any):**
- [Missing task for phase Y]: [what's missing]

**Granularity Issues (if any):**
- [Task X]: [why it's too large] — [suggested split]

**Bead Graph Issues (if any):**
- [Task X]: [missing field / cyclic dependency / priority contradiction / unresolvable dependency / invalid type-parent relationship]

**Final Recommendation:** [Proceed | Fix issues above and re-run]
```

Present the review to the user. The user has final approval.

## Phase 5: Bead Conversion (Auto-Trigger)

After all phases are complete and the user approves the final task list, **do not offer execution mode options** (subagent-driven or inline execution). The plan is bead-native by construction.

**Gate:** Present the task review summary and ask: **"Convert this plan to a bead graph? (yes / save for later)"**

If yes:
1. Read the approved `docs/plans/YYYY-MM-DD-<feature>-tasks.md`.
2. Dispatch `/vc-convert-plan-to-beads` with the task breakdown file path as the sole input.
3. The converter produces the bead graph at `beads/YYYY-MM-DD-<feature>-plan.json`.
4. The converter runs structural validation (acyclic, resolvable deps, schema compliance). If validation fails, report the specific issue and STOP — do not proceed. The user must fix the plan and re-run.

If save for later: confirm artifact locations and end cleanly.

**"Plan transcoded to beads. Artifacts:**
- **Plan:** `docs/PLAN.md`
- **Tasks:** `docs/plans/YYYY-MM-DD-<feature>-tasks.md`
- **Bead graph:** `beads/YYYY-MM-DD-<feature>-plan.json`

**Next:** The bead graph is ready. Load it with `bd create --graph beads/YYYY-MM-DD-<feature>-plan.json` when you are ready to execute."

(End of file)
