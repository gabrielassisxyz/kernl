---
name: vibe-engineering-mastery
description: 'Orchestrate a multi-stakeholder planning pipeline for important ideas. Use when you need to run the full planning pipeline, start strategic reviews, chain stakeholder reviews, or plan a feature thoroughly. Also triggers on "plan this properly", "run reviews before we build", "strategic planning", or "product review". Chains strategy definition, expert reviews (CEO/eng/design/DevEx), and iterative plan refinement (Yegge Loop) into durable docs/ artifacts. Auto-detects existing work and routes to the right stage.'
user-invocable: true
---

# Vibe Engineering Mastery

## Purpose

You have an important idea that needs thorough discussion before implementation. This skill turns it into a production-ready plan through structured stakeholder reviews and iterative refinement.

## The Pipeline

```
Idea → /vc-orchestrator
    → /vc-strategy                        → docs/STRATEGY.md
        → AskUserQuestion: autoplan or individual reviews?
            ├─ /vc-autoplan               → docs/reviews/vc-autoplan-YYYY-MM-DD.md
            └─ /vc-plan-ceo-review        → docs/reviews/vc-plan-ceo-review-YYYY-MM-DD.md
               /vc-plan-eng-review        → docs/reviews/vc-plan-eng-review-YYYY-MM-DD.md
               /vc-plan-design-review     → docs/reviews/vc-plan-design-review-YYYY-MM-DD.md
               /vc-plan-devex-review      → docs/reviews/vc-plan-devex-review-YYYY-MM-DD.md
                    │
                    ↓
            /vc-writing-plans             → docs/plans/YYYY-MM-DD-<feature>-plan.md
                    │
                    ↓
            Yegge Loop (5 iterations, Kimi K2.6)  → docs/PLAN.md approved
                    │
                    ↓
            GLM-5.1 task breakdown        → docs/plans/YYYY-MM-DD-<feature>-tasks.md
                    │
                    ↓
            Kimi K2.6 task review         → User approves
                    │
                    ↓
            /vc-convert-plan-to-beads     → beads/YYYY-MM-DD-<feature>-plan.json
                    │
                    ↓
            Execution ready (bead graph validated)
```

| Stage | Output | Human Gate |
|-------|--------|------------|
| **vc-strategy** | `docs/STRATEGY.md` | Confirm strategy before reviews |
| **vc-autoplan** | `docs/reviews/vc-autoplan-*.md` | Approve consolidated findings |
| **vc-plan-ceo-review** | `docs/reviews/vc-plan-ceo-review-*.md` | Interact per review step |
| **vc-plan-eng-review** | `docs/reviews/vc-plan-eng-review-*.md` | Interact per review step |
| **vc-plan-design-review** | `docs/reviews/vc-plan-design-review-*.md` | Interact per review step |
| **vc-plan-devex-review** | `docs/reviews/vc-plan-devex-review-*.md` | Interact per review step |
| **vc-writing-plans** | `docs/plans/YYYY-MM-DD-*-plan.md` | Approve design before plan |
| **Yegge Loop** | `docs/PLAN.md` | Approve each iteration |
| **vc-convert-plan-to-beads** | `beads/YYYY-MM-DD-*-plan.json` | Confirm bead graph validity |

## Artifact Detection (Auto-Routing)

When invoked, scan `docs/` for existing pipeline artifacts:

1. **Check for `docs/PLAN.md`**
   - If it exists and is in progress → Resume Yegge Loop (ask user to confirm)
   - If it exists and is approved → Ask: new task or continue?

2. **Check for `docs/plans/YYYY-MM-DD-*-plan.md`**
   - If found (and no `PLAN.md` or PLAN.md is old/stale) → Route to **Yegge Loop** with the plan as input
   - Ask user: "Resume plan refinement?" before proceeding

3. **Check for `docs/STRATEGY.md`**
   - If found → Read it, summarize, and ask whether to proceed to reviews
   - If missing → Route to **vc-strategy**

4. **Check for `docs/reviews/`**
   - If review artifacts exist → Present checklist of completed reviews
   - Ask user which reviews to run next

5. **Nothing found** → Start from **vc-strategy**

## Human Review Gates

You MUST pause for explicit user confirmation between stages. Do NOT auto-advance.

### Gate 1: After vc-strategy

Present:
- One-paragraph summary of the strategy
- Key problem, approach, and tracks

Ask: **"Strategy captured. Proceed to plan reviews? Say 'yes' to choose review mode, 'edit' to revise a section, or 'stop' to save and exit."**

If yes → proceed to Gate 2 (routing decision)
If edit → route back to vc-strategy with section focus
If stop → end cleanly

### Gate 2: Routing Decision

Present:
- Completed: strategy
- Upcoming: plan reviews

AskUserQuestion:

> "How do you want to review the plan?"

Options:
- A) **Run autoplan** — full CEO→Design→Eng→DX pipeline, auto-decisions, one approval gate at the end (fast)
- B) **Go one by one** — step through each review interactively, with user involvement at every decision point (thorough)
- C) **Skip reviews** — go straight to plan writing with only the strategy as input (for simple ideas)

If A → invoke `/vc-autoplan`
If B → proceed to Gate 3 (individual reviews)
If C → invoke `/vc-writing-plans`

### Gate 3: Individual Reviews (Phase 2B)

Present a checklist of reviews:
- [ ] CEO Review (scope, ambition, 10-star product)
- [ ] Engineering Review (architecture, tests, edge cases)
- [ ] Design Review (UX, accessibility, interaction states)
- [ ] DevEx Review (developer experience, onboarding, API ergonomics)

**Process:**
1. Ask user which review to run first
2. Read the selected review skill
3. Step through it **inline** with AskUserQuestion at every gate
4. Save review output to `docs/reviews/vc-plan-<type>-review-YYYY-MM-DD.md`
5. Ask: **"Run next review, re-run this one, or skip to planning?"**
6. Repeat until user says "skip to planning"

### Gate 4: After Reviews Complete

Present:
- Summary of all review findings
- Key decisions and concerns

Ask: **"Proceed to plan writing? Say 'yes' to synthesize, 're-run' to redo a review, or 'stop' to save findings."**

If yes → invoke `/vc-writing-plans`
If re-run → return to Gate 3
If stop → end cleanly

### Gate 5: After vc-writing-plans

The Yegge Loop handles its own user gates (up to 5 iterations). When the user approves the final plan:

Ask: **"Plan approved. Proceed to task breakdown, or save for later?"**

If proceed → continue to GLM-5.1 task breakdown
If save → confirm artifact locations and end cleanly

## Direct Stage Invocation

The user can invoke any stage directly:

- `/vc-orchestrator` — Full pipeline from artifact detection
- `/vc-strategy [optional: section to revisit]` — Strategy document
- `/vc-autoplan` — Auto-review pipeline
- `/vc-plan-ceo-review` — CEO review
- `/vc-plan-eng-review` — Engineering review
- `/vc-plan-design-review` — Design review
- `/vc-plan-devex-review` — Developer experience review
- `/vc-writing-plans` — Plan writing + Yegge Loop + task breakdown + bead conversion
- `/vc-convert-plan-to-beads` — Convert approved task breakdown to bead graph (standalone)

When invoked directly, still run artifact detection and offer to resume from existing work before starting fresh.

## File Conventions

| File | Location | Purpose |
|------|----------|---------|
| Strategy | `docs/STRATEGY.md` | Product strategy |
| Autoplan report | `docs/reviews/vc-autoplan-YYYY-MM-DD.md` | Consolidated auto-review |
| CEO Review | `docs/reviews/vc-plan-ceo-review-YYYY-MM-DD.md` | CEO findings |
| Eng Review | `docs/reviews/vc-plan-eng-review-YYYY-MM-DD.md` | Engineering findings |
| Design Review | `docs/reviews/vc-plan-design-review-YYYY-MM-DD.md` | Design findings |
| DevEx Review | `docs/reviews/vc-plan-devex-review-YYYY-MM-DD.md` | DevEx findings |
| Plan draft | `docs/plans/YYYY-MM-DD-<feature>-plan.md` | Implementation plan |
| Final plan | `docs/PLAN.md` | Approved plan |
| Bead graph | `beads/YYYY-MM-DD-<feature>-plan.json` | Bead graph for `bd create --graph` |

## Rules

1. **Always detect before dispatching.** Check `docs/` for existing artifacts first.
2. **Never skip human gates.** Auto-advancing destroys the value of thorough discussion.
3. **Preserve artifacts.** Never overwrite an existing strategy or review without asking.
4. **Respect the chain order.** Do not invoke vc-writing-plans without reviews unless the user explicitly requests it.
5. **End cleanly.** When the user stops at any gate, confirm what artifacts exist and where they are saved.
6. **Save every review.** Each review must produce a durable artifact before the next review begins.
