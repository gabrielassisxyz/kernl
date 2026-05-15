---
name: vibe-chaos-to-concept
description: "Turn raw ideas into implementable plans. Orchestrates a 3-stage pipeline: ideation (vc-ideate) → concept design (vc-brainstorm) → implementation planning (vc-plan). Detects existing artifacts, routes to the right stage, and inserts human review gates between stages. Use when saying 'turn my idea into a plan' or 'concept to implementation'."
user-invocable: true
---

# Vibe Chaos to Concept

## Purpose

You have a rough idea, a feature request, or a problem to solve. This skill turns it into a production-ready implementation plan through three stages of structured thinking, each producing a durable artifact.

## The Pipeline

```
Idea → /vc-ideate → Ideation Artifact → /vc-brainstorm → Brainstorm Spec → /vc-plan → Task Plan
```

| Stage | Output | Human Gate |
|-------|--------|------------|
| **vc-ideate** | `docs/YYYY-MM-DD-<topic>-ideation.md` | Review survivors, select one |
| **vc-brainstorm** | `docs/YYYY-MM-DD-<topic>-brainstorm-spec.md` | Approve design before planning |
| **vc-plan** | `docs/task_plan.md` + `docs/findings.md` | Approve plan before execution |

## Artifact Detection (Auto-Routing)

When invoked, scan `docs/` for existing pipeline artifacts:

1. **Check for `docs/task_plan.md`**
   - If it exists and is in progress → Resume planning (ask user to confirm)
   - If it exists and all phases are complete → Ask: new task or continue?

2. **Check for `docs/YYYY-MM-DD-*-brainstorm-spec.md`**
   - If found (and no `task_plan.md` or task_plan is old/stale) → Route to **vc-plan** with the spec as input
   - Ask user: "Use this brainstorm spec?" before proceeding

3. **Check for `docs/YYYY-MM-DD-*-ideation.md`**
   - If found (and no brainstorm spec or spec is old/stale) → Route to **vc-brainstorm** with the ideation log as input
   - Ask user: "Continue from this ideation?" before proceeding

4. **Nothing found** → Start from **vc-ideate** with the user's prompt

## Human Review Gates

You MUST pause for explicit user confirmation between stages. Do NOT auto-advance.

### Gate 1: After vc-ideate

Present:
- Top 3-5 survivors from the ideation artifact
- One-line summary of each

Ask: **"Which idea should we develop into a concept? Say the number, describe a different idea, or say 'none' to stop."**

If the user selects an idea → invoke `/vc-brainstorm` with that idea
If the user says "none" → end cleanly
If the user describes a new idea → feed that to `/vc-brainstorm` directly

### Gate 2: After vc-brainstorm

Present:
- The brainstorm spec's one-paragraph summary
- Architecture overview (1-2 sentences)
- Key decisions made

Ask: **"Approve this design and move to planning? Say 'yes' to create task_plan.md, 'no' to revise, or 'stop' to end here."**

If yes → invoke `/vc-plan` with the brainstorm spec
If no → collect specific feedback, loop back into vc-brainstorm
If stop → end cleanly. The brainstorm spec is saved for later.

### Gate 3: After vc-plan (Yegge Loop)

The Yegge Loop inside vc-plan handles its own user gates (minimum 5 iterations, continue if the user has feedback, no hard cap). When the user approves the final plan:

Ask: **"Plan approved. Execute now, or save for later?"**

If execute now → proceed to implementation (beads or manual)
If save → confirm the artifact locations and end cleanly

## Direct Stage Invocation

The user can invoke any stage directly:

- `/vc-ideate [focus]` — Skip detection, run ideation directly
- `/vc-brainstorm` — Skip detection, run brainstorm directly
- `/vc-plan` — Skip detection, run planning directly

When invoked directly, still run artifact detection and offer to resume from existing work before starting fresh.

## Integration with Beads

After all three stages complete and the user approves:

- Convert `docs/task_plan.md` into a **bead chain** for execution tracking
- Each bead maps to one phase from the task plan
- Status updates flow back to `docs/task_plan.md` (phase status: pending → in_progress → complete)

Beads conversion is a separate step — it does not happen automatically. Ask the user if they want beads after plan approval.

## File Conventions

| File | Location | Purpose |
|------|----------|---------|
| Ideation log | `docs/YYYY-MM-DD-<topic>-ideation.md` | Raw ideas, ranked |
| Brainstorm spec | `docs/YYYY-MM-DD-<topic>-brainstorm-spec.md` | Approved design |
| Task plan | `docs/task_plan.md` | Implementation roadmap |
| Findings | `docs/findings.md` | Research, errors, decisions |

## Rules

1. **Always detect before dispatching.** Check `docs/` for existing artifacts first.
2. **Never skip human gates.** Auto-advancing between stages destroys the value of the pipeline.
3. **Preserve artifacts.** Never overwrite an existing ideation log or brainstorm spec without asking.
4. **Respect the chain order.** Do not invoke vc-plan without a brainstorm spec or vc-brainstorm without an ideation log unless the user explicitly requests it.
5. **End cleanly.** When the user stops at any gate, confirm what artifacts exist and where they are saved.
