# Vibe Engineering Mastery Plugin

## Plugin Identity

- **Name:** vibe-engineering-mastery
- **Purpose:** Turn important ideas into thoroughly-discussed, implementation-ready plans through a structured, multi-stakeholder chain
- **Chain:** vc-strategy → vc-plan-{ceo,eng,design,devex}-review (or vc-autoplan) → vc-writing-plans + Yegge Loop + GLM-5.1 task breakdown → Kimi K2.6 task review → vc-convert-plan-to-beads

## Architecture

This plugin is a **7-stage pipeline**. Each stage produces a durable artifact. The user can invoke any stage independently or run the full chain via the orchestrator.

```
/vc-orchestrator
    ├── /vc-strategy                    → docs/STRATEGY.md
    │       └── AskUserQuestion: autoplan or individual?
    ├── Option A: /vc-autoplan          → docs/reviews/vc-autoplan-YYYY-MM-DD.md
    │       (runs CEO→Design→Eng→DX autonomously)
    └── Option B: /vc-plan-ceo-review   → docs/reviews/vc-plan-ceo-review-YYYY-MM-DD.md
            → /vc-plan-eng-review       → docs/reviews/vc-plan-eng-review-YYYY-MM-DD.md
            → /vc-plan-design-review    → docs/reviews/vc-plan-design-review-YYYY-MM-DD.md
            → /vc-plan-devex-review     → docs/reviews/vc-plan-devex-review-YYYY-MM-DD.md
                    │
                    ↓
            /vc-writing-plans           → docs/plans/YYYY-MM-DD-<feature>-plan.md
                    │
                    ↓
            Yegge Loop (5 iterations, Kimi K2.6) → docs/PLAN.md approved
                    │
                    ↓
            GLM-5.1 task breakdown      → docs/plans/YYYY-MM-DD-<feature>-tasks.md
                    │
                    ↓
            Kimi K2.6 task review       → User approves
                    │
                    ↓
            /vc-convert-plan-to-beads   → beads/YYYY-MM-DD-<feature>-plan.json
                    │
                    ↓
            Bead graph validated        → execution ready
```

### Stage 1: vc-strategy

**Trigger:** `/vc-strategy` or orchestrator auto-route when no `docs/STRATEGY.md` exists

**What it does:**
- Runs an interactive interview (target problem, approach, users, metrics, tracks)
- Pushes back on weak answers with anti-pattern detection
- Writes a short, durable `docs/STRATEGY.md`
- Rerunnable for updates

**Artifact:** `docs/STRATEGY.md`

**Next step:** Orchestrator auto-advances to routing decision

### Stage 2A: vc-autoplan (the autonomous path)

**Trigger:** User selects "Run autoplan" at the routing gate

**What it does:**
- Reads the 4 review skills from disk and runs them sequentially
- Applies 6 auto-decision principles (boil the lake, boring by default, etc.)
- Auto-decides close calls; surfaces taste decisions at a final approval gate
- One command, fully-reviewed plan out

**Artifact:** `docs/reviews/vc-autoplan-YYYY-MM-DD.md`

**Next step:** User approves → proceed to vc-writing-plans

### Stage 2B: Individual Reviews (the interactive path)

**Trigger:** User selects "Go one by one" at the routing gate

**What it does:**
- **vc-plan-ceo-review:** CEO-level scope challenge, 10-star product thinking, 4 modes (expansion/hold/reduction/selective)
- **vc-plan-eng-review:** Architecture lock, data flow, edge cases, test coverage
- **vc-plan-design-review:** UX rating 0-10 per dimension, mockups, interaction states
- **vc-plan-devex-review:** Developer persona, TTHW benchmarks, magical moments, friction points
- Each review is stepped through **inline** with AskUserQuestion gates
- Orchestrator saves each review output before moving to the next

**Artifacts:** `docs/reviews/vc-plan-{ceo,eng,design,devex}-review-YYYY-MM-DD.md`

**Next step:** All selected reviews complete → proceed to vc-writing-plans

### Stage 3: vc-writing-plans + Yegge Loop + Task Breakdown

**Trigger:** Orchestrator or `/vc-writing-plans`

**What it does:**
- Reads `docs/STRATEGY.md` and any review outputs as grounding
- Synthesizes a unified implementation plan
- **Plan creation (Kimi K2.6 persona):** Adopts thorough, structured, TDD-focused approach. Reasons holistically about dependencies.
- **Yegge Loop:** Up to 5 iterative refinement cycles
  - Iteration 1: Kimi K2.6 produces best first draft
  - Iterations 2+: collect feedback, contradiction check, 7-dimension self-critique (also Kimi K2.6)
  - Gate after each: "Approve or continue refining?"
- **Task breakdown (GLM-5.1 persona):** After plan approval, breaks into bite-sized tasks (2-5 minutes each), preserving all bead-mapping metadata
- **Task review (Kimi K2.6 persona):** Reviews task breakdown for dependency ordering, completeness, granularity, test coverage, and bead graph validity

**Artifacts:**
- `docs/plans/YYYY-MM-DD-<feature>-plan.md` (draft)
- `docs/PLAN.md` (final approved plan)
- `docs/plans/YYYY-MM-DD-<feature>-tasks.md` (GLM-5.1 task breakdown)
- `beads/YYYY-MM-DD-<feature>-plan.json` (bead graph, auto-converted)

**Next step:** User approves task list → user-confirmed dispatch of `/vc-convert-plan-to-beads` (with explicit "yes / save for later" gate) → bead graph validated → ready for execution

## Conventions

### Artifact Locations

All durable artifacts live in the **project's `docs/` directory**, except the bead graph which lives in the project's `beads/` directory.

| Artifact | Pattern | Produced By |
|----------|---------|-------------|
| Strategy | `docs/STRATEGY.md` | vc-strategy |
| Autoplan report | `docs/reviews/vc-autoplan-YYYY-MM-DD.md` | vc-autoplan |
| CEO Review | `docs/reviews/vc-plan-ceo-review-YYYY-MM-DD.md` | vc-plan-ceo-review |
| Eng Review | `docs/reviews/vc-plan-eng-review-YYYY-MM-DD.md` | vc-plan-eng-review |
| Design Review | `docs/reviews/vc-plan-design-review-YYYY-MM-DD.md` | vc-plan-design-review |
| DevEx Review | `docs/reviews/vc-plan-devex-review-YYYY-MM-DD.md` | vc-plan-devex-review |
| Plan draft | `docs/plans/YYYY-MM-DD-<feature>-plan.md` | vc-writing-plans |
| Final plan | `docs/PLAN.md` | Yegge Loop (Kimi K2.6) |
| Tasks | `docs/plans/YYYY-MM-DD-<feature>-tasks.md` | GLM-5.1 task breakdown |
| Bead graph | `beads/YYYY-MM-DD-<feature>-plan.json` | vc-convert-plan-to-beads |
| Findings | `docs/findings.md` | vc-writing-plans (if used) |

### Scratch Space

Use OS temp (`/tmp/`) for throwaway files. Use `docs/` for durable outputs only.

### No External Dependencies

This plugin does **not** depend on gstack binaries, gbrain, Proof HITL, or any home-directory state. It is fully self-contained and runs on any Claude Code installation with no extra setup.

## Design Philosophy

### Why `vc-` prefix?

Following the `vibe-chaos-to-concept` precedent (`vc-ideate`, `vc-brainstorm`, `vc-plan`), the `vc-` prefix groups all skills under the "Vibe Chaos" family so users can discover them by typing `/vc-`.

### Why strip gstack preambles?

The gstack preambles are ~80% infrastructure (telemetry, update checks, config, vendoring warnings) and ~20% behavioral shapers (AskUserQuestion format, voice, completion status). Since this pack does not ship gstack binaries, the infrastructure sections are dead weight. We keep the behavioral shapers inline.

### Why `docs/` for everything?

No artifact escapes the project directory. This makes the pack portable (no home-directory assumptions), version-controllable, and repo-scoped. The `vibe-chaos-to-concept` plugin proved this pattern works.

### Why Phase 2B is interactive

The user explicitly asked for "thorough discussion first." Each review is a conversation, not a report. The orchestrator acts as a moderator, not a dispatcher. The user can skip, re-run, or deep-dive any review.

### Why Kimi K2.6 + GLM-5.1?

The user specified model personas for different pipeline phases:

- **Kimi K2.6 (plan creation + review):** "Thorough, structured, TDD-focused. Reasons holistically about dependencies. Catches architectural gaps early. Prefers explicit over clever."
  - Used for: initial plan creation, Yegge Loop iterations, self-critique (7 dimensions), task breakdown review.

- **GLM-5.1 (task breakdown):** "Mechanically precise, exhaustive, granular. Breaks high-level plans into bite-sized executable tasks."
  - Used for: converting approved plan into 2-5 minute tasks with exact file paths, commands, expected outputs.

The separation ensures the plan stays architecturally sound (Kimi) while the tasks are mechanically precise (GLM). If the platform does not support subagent dispatch with model hints, the personas are applied as behavioral prompts in the skill instructions.

## File Structure

```
vibe-engineering-mastery/
├── SKILL.md                          # Orchestrator entry point
├── AGENTS.md                         # This file
└── skills/
    ├── vc-strategy/
    │   ├── SKILL.md
    │   └── references/
    │       ├── interview.md
    │       └── strategy-template.md
    ├── vc-plan-ceo-review/
    │   ├── SKILL.md
    │   └── references/
    │       └── askuser-format.md
    ├── vc-plan-eng-review/
    │   ├── SKILL.md
    │   └── references/
    │       └── askuser-format.md
    ├── vc-plan-design-review/
    │   ├── SKILL.md
    │   └── references/
    │       └── askuser-format.md
    ├── vc-plan-devex-review/
    │   ├── SKILL.md
    │   └── references/
    │       └── askuser-format.md
    ├── vc-autoplan/
    │   ├── SKILL.md
    │   └── references/
    │       └── askuser-format.md
    ├── vc-writing-plans/
    │   ├── SKILL.md
    │   └── references/
    │       └── plan-document-reviewer-prompt.md
    ├── vc-convert-plan-to-beads/
    │   ├── SKILL.md
    │   └── references/
    │       └── graph-schema.md
    └── vc-orchestrator/
        ├── SKILL.md
        └── references/
            └── yegge-loop.md
```

## Notes

- **Beads conversion:** Active via `/vc-convert-plan-to-beads`. Every approved plan is auto-converted to a bead graph.
- **No Proof HITL:** Review is inline in chat.
- **No progress.md:** Not needed — the pipeline is linear.
- **Attestation:** Not implemented in this iteration.
