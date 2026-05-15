# Vibe Chaos to Concept Plugin

## Plugin Identity

- **Name:** vibe-chaos-to-concept
- **Purpose:** Turn raw ideas into refined implementation plans through a structured, human-in-the-loop chain
- **Chain:** vc-ideate — vc-brainstorm — vc-plan

## Architecture

This plugin is a **3-stage pipeline** with a multi-model refinement loop. Each stage produces a durable artifact that feeds the next. The user can invoke any stage independently or run the full chain.

```
/vc-ideate            → docs/YYYY-MM-DD-<topic>-ideation.md
    ↓
/vc-brainstorm        → docs/YYYY-MM-DD-<topic>-brainstorm-spec.md
    ↓
/vc-plan              → docs/task_plan.md (+ docs/findings.md)
    → Kimi K2.6 creates plan  —  best first draft
    → Yegge Loop (Kimi K2.6)  —  5+ iterations, self-critique
    → Kimi K2.6 approves plan
    → GLM-5.1 breaks into tasks  —  bite-sized executable work
    → Kimi K2.6 reviews tasks  —  dependency + completeness check
    → User approves → execution ready
```

### Why Kimi K2.6 + GLM-5.1?

The pipeline splits plan creation and task breakdown across two model personas by design:

- **Kimi K2.6 (plan creation + review):** Excels at holistic planning, reasoning about dependencies, and maintaining architectural coherence across iterations. It produces structurally sound plans and catches gaps early through the Yegge Loop.
- **GLM-5.1 (task breakdown):** Excels at granular decomposition — taking a high-level plan and producing bite-sized, executable tasks with exact file paths, specific commands, expected outputs, and commit messages.

**Separation of concerns:** Kimi keeps the plan architecturally sound; GLM keeps the tasks mechanically precise. If the platform does not support subagent dispatch with model hints, apply these personas as behavioral prompts within the skill instructions.

## Conventions

### Artifact Locations

All durable artifacts live in the **project's `docs/` directory**:

| Artifact | Pattern | Produced By |
|----------|---------|-------------|
| Ideation log | `docs/YYYY-MM-DD-<topic>-ideation.md` | vc-ideate |
| Brainstorm spec | `docs/YYYY-MM-DD-<topic>-brainstorm-spec.md` | vc-brainstorm |
| Task plan | `docs/task_plan.md` | vc-plan |
| Research | `docs/findings.md` | vc-plan |

### Skill Invocation

Invoke skills with their slash command name: `/vc-ideate`, `/vc-brainstorm`, `/vc-plan`.

Skills are **user-invocable** (`user-invocable: true`) and may auto-trigger on relevant prompts.

### Scratch Space

`vc-ideate` uses `/tmp/vibe-chaos-to-concept/vc-ideate/<run-id>/` for checkpoints and web-research cache.

`vc-plan` uses `.planning/` for parallel-session isolation when the `init-session.sh` slug mode is used.

### No External Tools

This plugin does **not** use external review tools (e.g., Proof HITL). Review happens inline in conversation.

## Stage Guide

### Stage 1: vc-ideate

**Trigger:** "What should I improve?", "Give me ideas", "Ideate on X"

**What it does:**
- Grounds ideation in the current codebase (repo mode) or user-supplied context (elsewhere mode)
- Dispatches parallel sub-agents to generate ideas across 6 frames (pain, inversion, reframing, leverage, analogy, constraint-flipping)
- Adversarially filters candidates to 5-7 survivors with articulated basis
- Presents survivors for user review

**Artifact:** `docs/YYYY-MM-DD-<topic>-ideation.md`

**Next step:** User selects "Brainstorm a selected idea" or exits

### Stage 2: vc-brainstorm

**Trigger:** `/vc-brainstorm`, auto-triggered from vc-ideate on selection

**What it does:**
- Reads the ideation log (if present) and asks the user which idea to develop
- Collaborative Socratic dialogue: one question at a time, 2-3 approaches, get approval
- Designs architecture, components, data flow, error handling, testing strategy
- Hard gate: no code until the user approves the design

**Artifact:** `docs/YYYY-MM-DD-<topic>-brainstorm-spec.md`

**Next step:** User approves → vc-plan is invoked automatically

### Stage 3: vc-plan

**Trigger:** `/vc-plan`, auto-triggered from vc-brainstorm on approval

**What it does:**
- Reads the brainstorm spec (if present) to pre-populate task phases
- Creates `docs/task_plan.md` + `docs/findings.md` from templates
- **Kimi K2.6** creator persona adopted for all plan creation and review
- **Yegge Loop:** minimum 5 iterative refining cycles with user feedback, no hard cap
- **Self-critique on each iteration:** TDD coverage, quality gates, dependency gaps, missing test scenarios, scope creep, token density
- **GLM-5.1 Phase:** after plan approval, break into bite-sized, executable tasks (2-5 minutes each)
- **Kimi K2.6 Review Phase:** review GLM-5.1 task breakdown for dependency ordering, completeness, granularity, and test coverage
- User approves → plan is ready for execution

**Artifacts:**
- `docs/task_plan.md` (plan + Yegge Loop iterations)
- `docs/findings.md` (research / context)
- Task breakdown embedded in or alongside plan

**Next step:** User approves → plan is ready for execution (beads conversion)

## Yegge Loop

Built into `vc-plan`. After writing `docs/task_plan.md`:

1. Output Iteration 1 (best first draft)
2. Collect user feedback
3. Self-critique and contradiction check against the brainstorm spec
4. Critique own iteration (verbosity, token density, scope)
5. Output next iteration
6. Repeat until user approves or 5 iterations reached

Minimum 5 iterations. Continue if the user has feedback, no hard cap. Stop on contradiction.

## File Structure

```
vibe-chaos-to-concept/
├── SKILL.md                          # Orchestrator entry point
├── AGENTS.md                         # This file
└── skills/
    ├── vc-ideate/
    │   ├── SKILL.md                  # Ideation stage (Phase 0: intake/scoping)
    │   ├── agents/
    │   │   ├── vc-learnings-researcher.agent.md
    │   │   ├── vc-issue-intelligence-analyst.agent.md
    │   │   └── vc-web-researcher.agent.md
    │   └── references/
    │       ├── execution-flow.md         # Phases 1, 1.5, 2: grounding + ideation
    │       ├── post-ideation-workflow.md  # Phases 3-6: filtering + handoff
    │       ├── universal-ideation.md
    │       └── web-research-cache.md
    ├── vc-brainstorm/
    │   └── SKILL.md                  # Brainstorming stage
    └── vc-plan/
        ├── SKILL.md                  # Planning stage
        ├── scripts/
        │   ├── attest-plan.sh
        │   ├── check-complete.sh
        │   ├── init-session.sh
        │   ├── resolve-plan-dir.sh
        │   ├── session-catchup.py
        │   ├── set-active-plan.sh
        │   └── attest-plan.ps1 (and others for Windows)
        └── templates/
            ├── task_plan.md
            └── findings.md
```

## Notes

- **progress.md removed:** Not needed — beads takes the place of execution tracking.
- **No Proof HITL:** Review is inline in chat to minimize external dependencies.
- **progress.md removed from scripts:** `init-session.sh` and `init-session.ps1` no longer create `progress.md`.
- **Parallel plans:** Each skill supports isolated planning directories under `.planning/`.
- **Attestation:** `task_plan.md` can be SHA-256 locked via `/plan-attest` or `scripts/attest-plan.sh`.
