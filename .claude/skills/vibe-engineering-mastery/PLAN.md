# vibe-engineering-mastery: Design & Implementation Plan

> **Status:** COMPLETE — all skills built. Verification passed (zero gstack leaks, 4,458 total lines). All pending fixes applied.
> **Output directory:** `vibe-engineering-mastery/`

---

## CRITICAL CORRECTION — Model Assignments (2026-05-13)

**Previous (WRONG):** GLM-5.1 creates plan, Kimi K2.6 reviews.

**Corrected:**
- **Kimi K2.6** creates the plan (via `vc-writing-plans` + Yegge Loop)
- **Kimi K2.6** reviews through Yegge Loop iterations (self-critique or subagent)
- **GLM-5.1** breaks the approved plan into smaller tasks (granular implementation steps)
- **Kimi K2.6** reviews those broken-down tasks

**Same pattern applies to `vibe-chaos-to-concept`** — `vc-plan` should use:
- Kimi K2.6 for plan creation + Yegge Loop refinement
- GLM-5.1 for task breakdown (after plan is approved)

**Rationale:** Kimi K2.6 excels at holistic planning, reasoning about dependencies, and maintaining architectural coherence across iterations. GLM-5.1 excels at granular decomposition — taking a high-level plan and producing bite-sized, executable tasks with exact file paths and commands. The separation ensures the plan stays architecturally sound (Kimi) while the tasks are mechanically precise (GLM).

---

## 0. Post-Build Refinement (2026-05-13)

After initial construction, a full skill review was run using `skill-forge/scripts/validate_skill.py`. The review identified two structural issues: oversized SKILL.md files (review skills exceeded 500-line / 5000-token guidelines) and duplicated `references/askuser-format.md` across 5 sub-skills. The following refinement was completed.

### 0.1 Centralized Shared Reference

- **Created:** `vibe-engineering-mastery/references/askuser-format.md` (root-level shared reference)
- **Removed:** 5 identical copies from `skills/*/references/askuser-format.md`
- **Updated:** All sub-skill `SKILL.md` files now point to `../../references/askuser-format.md`

### 0.2 Extracted Reference Material

Reference files were extracted from the 4 review skills and `vc-autoplan` to reduce inline bulk. The core workflow instructions (phases, sections, STOP gates) remain in each `SKILL.md`.

| Skill | Extracted References | Lines Removed |
|-------|-------------------|---------------|
| `vc-plan-ceo-review` | cognitive-patterns.md, prime-directives.md, engineering-preferences.md, mode-reference.md, outside-voice.md, completion-summary-template.md | ~202 |
| `vc-plan-design-review` | design-philosophy.md, ai-slop-rules.md | ~199 |
| `vc-plan-eng-review` | cognitive-patterns.md, outside-voice.md | ~89 |
| `vc-plan-devex-review` | dx-reference-tables.md, cognitive-patterns.md, outside-voice.md, mode-reference.md, required-outputs.md | ~281 |

### 0.3 Post-Refinement Validation

| Skill | Score | Line Count | Token Estimate | Status |
|-------|-------|------------|--------------|--------|
| `vc-strategy` | 95 | ~87 | ~500 | Pass |
| `vc-writing-plans` | 95 | ~244 | ~1500 | Pass |
| `vc-orchestrator` | 85 | ~266 | ~1700 | Pass |
| `vc-plan-eng-review` | 80 | ~471 | ~7851 | Pass |
| `vc-plan-design-review` | 85 | ~372 | ~5500 | Pass |
| `vc-autoplan` | 75 | ~734 | ~8442 | Fail (token size) |
| `vc-plan-ceo-review` | 75 | ~774 | ~12509 | Fail (token size) |
| `vc-plan-devex-review` | 75 | ~729 | ~9062 | Fail (token size) |

**Note:** The 3 remaining failing skills fail only on body size (line/token count). Their bulk is the **core sequential workflow** — review sections, phases, and STOP gates. Extracting these would require `Read`-ing a reference before executing each section, which adds friction for a manually-invoked skill. The author declined further compression. All other validation rules pass cleanly.

### 0.4 Final Fixes Completed

1. **Updated all skill descriptions** with trigger phrases — every skill now scores 90-100. Remaining size-only MEDIUM warnings on 3 instruction-dense skills (autoplan, ceo-review, devex-review) are accepted by design.
2. **Eval sets** — declined (author manually invokes skills, eval sets are cosmetic).
3. **Updated `vibe-chaos-to-concept/AGENTS.md`** with Kimi K2.6 + GLM-5.1 model assignment rationale in the Architecture section.

---

## 1. Executive Summary

This document is the authoritative plan for building `vibe-engineering-mastery`, a Claude Code skill pack that chains 8 skills into a single, interactive planning pipeline for "important ideas that need thorough discussion."

**The chain:**

```
/vc-orchestrator
    → /vc-strategy                        → docs/STRATEGY.md
        → AskUserQuestion: autoplan or individual reviews?
            ├─ /vc-autoplan               → Runs CEO→Design→Eng→DX autonomously
            └─ /vc-plan-ceo-review        → Interactive CEO review
               /vc-plan-eng-review        → Interactive engineering review
               /vc-plan-design-review     → Interactive design review
               /vc-plan-devex-review      → Interactive developer experience review
    → /vc-writing-plans                   → PLAN.md (Kimi K2.6 creates + Yegge Loop refines)
    → Yegge Loop (5 iterations, Kimi K2.6 self-critique)
    → GLM-5.1 breaks plan into tasks     → docs/plans/YYYY-MM-DD-<feature>-tasks.md
    → Kimi K2.6 reviews tasks            → final approval
```

**Key architectural decisions:**

1. **Prefix all skills with `vc-`** (following the `vibe-chaos-to-concept` pattern).
2. **Strip gstack infrastructure:** no telemetry, no `~/.gstack` writes, no binary dependencies.
3. **Everything lands in `docs/`** — no artifacts escape the project directory.
4. **Phase 2B is strictly interactive** — individual reviews run sequentially with user involvement.
5. **No beads generation** in this iteration.
6. **Kimi K2.6** for plan creation + Yegge Loop refinement; **GLM-5.1** for task breakdown.

---

## 2. What Is Already Built

### 2.1 Root scaffolding — COMPLETE
- `vibe-engineering-mastery/SKILL.md` — root orchestrator entry point (artifact detection, human gates, direct invocation)
- `vibe-engineering-mastery/AGENTS.md` — pack conventions, architecture, design philosophy

### 2.2 `vc-strategy` — COMPLETE
- `skills/vc-strategy/SKILL.md` — ported from `ce-strategy`. Clean, no preamble to strip.
- `skills/vc-strategy/references/interview.md` — full 8-section interview with anti-patterns and pushback rules.
- `skills/vc-strategy/references/strategy-template.md` — strategy template with post-write checklist.
- **Changes from source:** `vc-` prefix, outputs to `docs/STRATEGY.md`, handoff suggests `/vc-orchestrator`.

---

## 3. What Remains To Build

### 3.1 `vc-orchestrator` + `references/yegge-loop.md`

**Source:** None (synthesized). Follow `vibe-chaos-to-concept` root SKILL.md pattern.

**Files:**
- `skills/vc-orchestrator/SKILL.md`
- `skills/vc-orchestrator/references/yegge-loop.md`

**Orchestrator SKILL.md must implement:**
- Phase 0: Artifact detection (scan `docs/` for STRATEGY.md, PLAN.md, reviews/)
- Phase 1: Invoke `/vc-strategy` if `docs/STRATEGY.md` missing
- Phase 2: Routing AskUserQuestion (autoplan / individual / skip)
- Phase 2A: Invoke `/vc-autoplan` (reads 4 review skills, auto-decides)
- Phase 2B: Interactive individual reviews (step through each inline with user gates)
- Phase 3: Synthesize STRATEGY.md + reviews → unified draft
- Phase 4: Invoke `/vc-writing-plans` with unified draft
- Phase 5: Yegge Loop (5 iterations, Kimi K2.6 self-critique)
- Phase 6: Invoke GLM-5.1 subagent for task breakdown
- Phase 7: Kimi K2.6 reviews broken-down tasks
- Phase 8: Handoff (announce artifacts, suggest next steps)

**Yegge Loop reference must contain:**
- Exact spec from user's original message
- Kimi K2.6 creator persona instructions
- Kimi K2.6 reviewer persona instructions (for self-critique)
- 6-dimension rubric: TDD coverage, Quality gates, Dependency gaps, Missing test scenarios, Scope creep, Token density
- Subagent reviewer pattern (fallback to inline)
- Skip conditions (user approves, 2-3 phases with one-sentence clarity, automated pipeline)

### 3.2 `vc-plan-ceo-review`

**Source:** `refs/gstack/plan-ceo-review/SKILL.md` (~1787 lines).
**Structure:** ~835-line shared preamble + ~952-line unique content.

**Files:**
- `skills/vc-plan-ceo-review/SKILL.md`
- `skills/vc-plan-ceo-review/references/askuser-format.md` *(shared AskUserQuestion format)*

**Unique content to keep (after stripping preamble):**
- AskUserQuestion format reference → `references/askuser-format.md`
- Voice guidelines (shortened inline)
- Completion status protocol (inline)
- **Mega Plan Review Mode** (line ~835):
  - Philosophy (4 modes: SCOPE EXPANSION, SELECTIVE EXPANSION, HOLD SCOPE, SCOPE REDUCTION)
  - 9 Prime Directives (zero silent failures, every error has name, data flows have shadow paths, etc.)
  - 18 CEO cognitive patterns (Bezos doors, Grove paranoia, Munger inversion, Jobs subtraction, etc.)
  - Engineering preferences (DRY, well-tested, explicit over clever, etc.)
- **PRE-REVIEW SYSTEM AUDIT** (git log, diff, stash, TODO grep, read CLAUDE.md/TODOS.md)
- **Design doc check** — look for design docs, offer prerequisite skill
- **Handoff note check** — check for prior CEO review handoff
- **Step 0: Nuclear Scope Challenge + Mode Selection**
  - 0A Premise Challenge (right problem? actual outcome? do nothing?)
  - 0B Existing Code Leverage (reuse existing code?)
  - 0C Dream State Mapping (current → plan → 12-month ideal)
  - 0C-bis Implementation Alternatives (MANDATORY — 2-3 approaches, one minimal, one ideal)
  - 0D Mode-Specific Analysis (expansion/selective/hold/reduction)
  - 0D-POST Persist CEO Plan (write to docs/reviews/, not ~/.gstack/)
  - 0E Temporal Interrogation (what decisions need resolving NOW?)
  - 0F Mode Selection (4 options with context-dependent defaults)
- **11 Review Sections** (architecture, error/rescue, security, data flow, code quality, tests, performance, observability, deployment, long-term trajectory, design/UX)
- **Outside Voice** (independent plan challenge via subagent or Codex — optional)
- **Required outputs** (NOT in scope, what exists, dream state delta, error registry, failure modes, TODOS)
- **How to ask questions** (one issue = one AskUserQuestion)

**What to REMOVE from unique content:**
- All `~/.claude/skills/gstack/bin/*` calls
- All `~/.gstack/` writes (replace with `docs/reviews/`)
- All `~/.gbrain/` references
- Review Readiness Dashboard (depends on gstack binary)
- Review log persistence (depends on gstack binary)
- Plan File Review Report (GSTACK REVIEW REPORT section)
- Capture Learnings (depends on gstack binary)
- Next Steps chaining (remove gstack-specific skill refs like `/office-hours`, `/ship`)
- Codex CLI invocation for outside voice (keep subagent fallback as primary)
- All telemetry commands

### 3.3 `vc-plan-eng-review`

**Source:** `refs/gstack/plan-eng-review/SKILL.md` (~1655 lines).
**Structure:** ~959-line shared preamble + ~696-line unique content.

**Files:**
- `skills/vc-plan-eng-review/SKILL.md`

**Unique content to keep:**
- Minimal header (same as CEO review)
- AskUserQuestion format reference
- Voice guidelines (shortened)
- Completion status protocol
- **Plan Review Mode** (line ~773):
  - Engineering preferences (DRY, well-tested, boring by default, etc.)
  - 14 Eng cognitive patterns (Larson state diagnosis, blast radius, systems over heroes, etc.)
  - Documentation and diagrams preferences
- **BEFORE YOU START:**
  - Design doc check (read design doc if exists)
  - Prerequisite skill offer (if no design doc)
- **Step 0: Scope Challenge**
  - Complexity check (8+ files or 2+ new classes = smell)
  - Search check (built-in vs custom)
  - TODOS cross-reference
  - Completeness check (shortcut vs complete?)
  - Distribution check
  - STOP gate if complexity check triggers
- **4 Review Sections:**
  1. Architecture review (system design, data flow, state machines, coupling, scaling, security, production failures, rollback)
  2. Code quality review (organization, DRY, naming, error handling, edge cases, over/under-engineering, cyclomatic complexity)
  3. Test review (100% coverage goal, test framework detection, trace every codepath, ASCII coverage diagram, E2E decision matrix, regression rule)
  4. Performance review (N+1, memory, indexes, caching, background jobs, slow paths, connection pools)
- **Confidence Calibration** (1-10 scoring for each finding)
- **Outside Voice** (same pattern as CEO)
- **Required outputs** (NOT in scope, what exists, TODOS, diagrams, failure modes, parallelization strategy, completion summary)
- **How to ask questions**

**What to REMOVE:**
- All gstack binary calls
- All `~/.gstack/` writes
- Review Readiness Dashboard
- Review log persistence
- Plan File Review Report
- Capture Learnings
- Test plan artifact write to `~/.gstack/projects/`
- Next Steps chaining (remove `/ship`, `/codex`, etc.)

### 3.4 `vc-plan-design-review`

**Source:** `refs/gstack/plan-design-review/SKILL.md` (~1848 lines).
**Structure:** ~1026-line shared preamble + ~822-line unique content.

**Files:**
- `skills/vc-plan-design-review/SKILL.md`

**Unique content to keep:**
- Minimal header
- AskUserQuestion format reference
- Voice guidelines (shortened)
- Completion status protocol
- **Design Philosophy** (line ~808):
  - 12 Design cognitive patterns (system thinking, empathy simulation, hierarchy as service, etc.)
  - 3 Laws of Usability (Krug)
  - Billboard Design for Interfaces
  - Navigation as Wayfinding
  - Goodwill Reservoir
  - Mobile rules
- **Step 0: Design Scope Assessment**
  - 0A Initial design rating (0-10)
  - 0B DESIGN.md status check
  - 0C Existing design leverage
  - 0D Focus areas AskUserQuestion
- **Step 0.5: Visual Mockups** (DEFAULT — but gracefully degrade)
  - If design binary not available, skip mockups and use text-based review
  - Keep mockup generation logic but replace `$D` binary calls with notes
  - Keep comparison board pattern but degrade gracefully
- **7 Review Passes:**
  1. Information Architecture (0-10 rating)
  2. Interaction State Coverage (loading/empty/error/success/partial table)
  3. User Journey & Emotional Arc (storyboard)
  4. AI Slop Risk (design hard rules, litmus checks, AI slop blacklist)
  5. Design System Alignment
  6. Responsive & Accessibility
  7. Unresolved Design Decisions
- **Post-Pass: Update Mockups**
- **Design Outside Voices** (optional parallel subagent review)
- **0-10 Rating Method**
- **Required outputs** (NOT in scope, what exists, TODOS, completion summary, approved mockups)

**What to REMOVE:**
- All `$D` (design binary) invocations — replace with graceful degradation notes
- All `$B` (browse binary) invocations
- All `~/.gstack/` writes for design artifacts
- DESIGN SETUP block that checks for gstack binary
- Review Readiness Dashboard
- Review log persistence
- Plan File Review Report
- Capture Learnings
- Next Steps chaining (remove `/design-shotgun`, `/design-html`)

### 3.5 `vc-plan-devex-review`

**Source:** `refs/gstack/plan-devex-review/SKILL.md` (~2049 lines).
**Structure:** ~1058-line shared preamble + ~991-line unique content.

**Files:**
- `skills/vc-plan-devex-review/SKILL.md`
- `skills/vc-plan-devex-review/references/dx-hall-of-fame.md` *(inline key examples)*

**Unique content to keep:**
- Minimal header
- AskUserQuestion format reference
- Voice guidelines (shortened)
- Completion status protocol
- **DX First Principles** (line ~812):
  - 8 principles (zero friction at T0, incremental steps, learn by doing, etc.)
  - 7 DX Characteristics table (Usable, Credible, Findable, Useful, Valuable, Accessible, Desirable)
  - DX Scoring Rubric (0-10)
  - TTHW Benchmarks table
- **Cognitive Patterns** (10 DX leader patterns)
- **Step 0: DX Investigation**
  - 0A Developer Persona Interrogation (choose persona, produce persona card)
  - 0B Empathy Narrative (150-250 word first-person narrative)
  - 0C Competitive DX Benchmarking (WebSearch 3 queries, benchmark table)
  - 0D Magical Moment Design (choose delivery vehicle)
  - 0E Mode Selection (DX EXPANSION / DX POLISH / DX TRIAGE)
  - 0F Developer Journey Trace (6 stages with friction points)
  - 0G First-Time Developer Roleplay (confusion report)
- **0-10 Rating Method** (evidence-based, mode-specific)
- **8 Review Passes:**
  1. Getting Started Experience (Zero Friction)
  2. API/CLI/SDK Design (Usable + Useful)
  3. Error Messages & Debugging (Fight Uncertainty)
  4. Documentation & Learning (Findable + Learn by Doing)
  5. Upgrade & Migration Path (Credible)
  6. Developer Environment & Tooling (Valuable + Accessible)
  7. Community & Ecosystem (Findable + Desirable)
  8. DX Measurement & Feedback Loops
- **Appendix: Claude Code Skill DX Checklist** (conditional)
- **Outside Voice** (same pattern as CEO/Eng)
- **Required outputs** (persona card, empathy narrative, benchmark, magical moment spec, journey map, confusion report, scorecard, implementation checklist, unresolved decisions)

**What to REMOVE:**
- All gstack binary calls
- All `~/.gstack/` writes
- `dx-hall-of-fame.md` file references — inline the 3-4 most relevant examples per pass instead
- Review Readiness Dashboard
- Review log persistence
- Plan File Review Report
- Capture Learnings
- Next Steps chaining
- DX Trend Check (depends on gstack binary)

### 3.6 `vc-autoplan`

**Source:** `refs/gstack/autoplan/SKILL.md` (~1733 lines, FULLY READ).
**Structure:** ~868-line shared preamble + ~865-line unique content.

**Files:**
- `skills/vc-autoplan/SKILL.md`

**What to keep:**
- Minimal header
- AskUserQuestion format reference
- Voice guidelines (shortened)
- Completion status protocol
- **6 auto-decision principles:**
  1. Choose completeness — ship the whole thing
  2. Boil lakes — fix everything in blast radius (< 1 day CC effort, < 5 files)
  3. Pragmatic — 5 seconds choosing, not 5 minutes
  4. DRY — reject duplicates
  5. Explicit over clever — obvious fix > abstraction
  6. Bias toward action — merge > review cycles > stale deliberation
- **Decision classification:**
  - Mechanical — auto-decide silently
  - Taste — auto-decide, surface at final gate (close approaches, borderline scope, codex disagreements)
  - User Challenge — NEVER auto-decide (both models agree user's direction should change)
- **Conflict resolution tiebreakers:**
  - CEO phase: P1 (completeness) + P2 (boil lakes) dominate
  - Eng phase: P5 (explicit) + P3 (pragmatic) dominate
  - Design phase: P5 (explicit) + P1 (completeness) dominate
- **Sequential execution logic:**
  - Phase 0: Intake + Restore Point (read context, detect UI/DX scope, load skill files)
  - Phase 0.5: Codex auth preflight (degrade gracefully if unavailable)
  - Phase 1: CEO Review (SELECTIVE EXPANSION, dual voices, consensus table)
  - Phase 2: Design Review (conditional on UI scope)
  - Phase 3: Eng Review
  - Phase 3.5: DevEx Review (conditional on DX scope)
  - Phase 4: Consolidation + Final Approval Gate
- **Section skip list** when following loaded skill files (shared preamble sections already handled)
- **Dual voices pattern:**
  - Claude subagent ( foreground Agent tool) + Codex (Bash, with timeout)
  - Degradation matrix: both fail → single-reviewer; codex only → [codex-only]; subagent only → [subagent-only]
  - Consensus table per phase (dimensions × 2 voices)
- **Taste decisions to surface at final gate:**
  - Close approaches (two valid ways, no clear winner)
  - Borderline scope (in blast radius but 3-5 files, or ambiguous radius)
  - Codex disagreements (if outside voice used)
- **User Challenges at final gate:**
  - What the user said / what both models recommend / why / blind spots / cost of being wrong
- **Final approval gate** with one AskUserQuestion: "Approve or run individual reviews?"

**What to REMOVE:**
- All gstack binary calls (`gstack-config`, `gstack-slug`, `gstack-codex-probe`, etc.)
- All `~/.gstack/` writes (replace with `docs/reviews/`)
- All `~/.gbrain/` references
- Codex CLI invocations (replace with Claude subagent only, or note as optional)
- Filesystem boundary prefix for Codex prompts (not needed without Codex CLI)
- Phase 0.5 Codex auth preflight (simplify: check if subagent available, else inline)
- Review Readiness Dashboard
- Review log persistence
- Plan File Review Report
- Capture Learnings
- Next Steps chaining (remove `/ship`, `/codex`, `/review`, etc.)
- **Path changes:**
  - Replace `~/.claude/skills/gstack/plan-*/SKILL.md` with relative pack paths via `CLAUDE_SKILL_DIR`
  - Restore point: write to `docs/reviews/` instead of `~/.gstack/projects/`
  - Design doc discovery: search `docs/` instead of `~/.gstack/projects/`

### 3.7 `vc-writing-plans`

**Source:** `refs/superpowers/skills/writing-plans/SKILL.md` (~152 lines).

**Files:**
- `skills/vc-writing-plans/SKILL.md`
- `skills/vc-writing-plans/references/plan-document-reviewer-prompt.md`

**What to keep (almost everything — very clean source):**
- Bite-sized task granularity
- TDD guidance
- Self-review checklist
- Execution handoff
- **Changes from source:**
  - `name: vc-writing-plans`
  - Default path: `docs/plans/YYYY-MM-DD-<feature>-plan.md`
  - Add Kimi K2.6 creator persona at start
  - Add Yegge Loop at end (inline or reference)
  - Add GLM-5.1 task-breakdown phase after Yegge Loop approval

**Kimi K2.6 Creator Persona to inject:**
"You are Kimi K2.6 — thorough, structured, TDD-focused. You reason about dependencies holistically. You catch architectural gaps before they become problems. You prefer explicit over clever. Every phase has verifiable success criteria."

**GLM-5.1 Task Breakdown to inject:**
"After the plan is approved, invoke a subagent with GLM-5.1 persona to break the plan into bite-sized tasks (2-5 minutes each). The subagent produces: exact file paths, specific commands, expected outputs, and commit messages."

---

## 4. Preamble Removal Strategy

All gstack review skills share an identical ~800-line preamble. The following is the **complete list** of preamble sections to strip, with notes on which are load-bearing:

| # | Preamble Section | Load-Bearing? | Action |
|---|-----------------|---------------|--------|
| 1 | Update check | No | Strip |
| 2 | Session bookkeeping (`~/.gstack/sessions/`) | No | Strip |
| 3 | Proactive/skill-prefix config | No | Strip |
| 4 | Telemetry | No | Strip |
| 5 | Writing style prompt | Partial | Inline a 3-line version |
| 6 | Lake intro | No | Strip |
| 7 | Feature discovery | No | Strip |
| 8 | Routing injection (CLAUDE.md) | No | Strip |
| 9 | Vendoring warning | No | Strip |
| 10 | Plan mode safe operations | No | Strip |
| 11 | AskUserQuestion format | **YES** | Extract to `references/askuser-format.md` |
| 12 | Artifacts sync (gbrain) | No | Strip |
| 13 | Model-specific behavioral patch | Partial | Inline key nudges |
| 14 | Voice guidelines | **YES** | Shorten to 20 lines inline |
| 15 | Context recovery (`~/.gstack/projects/`) | No | Strip |
| 16 | Writing style (jargon list) | Partial | Keep jargon list inline |
| 17 | Completeness principle | Partial | Keep 3-line version |
| 18 | Confusion protocol | **YES** | Keep inline |
| 19 | Continuous checkpoint mode | No | Strip |
| 20 | Context health | No | Strip |
| 21 | Question tuning | No | Strip |
| 22 | Repo ownership | No | Strip |
| 23 | Search before building | Partial | Keep 2-line version |
| 24 | Completion status protocol | **YES** | Keep inline |
| 25 | Operational self-improvement | No | Strip |
| 26 | Telemetry (run last) | No | Strip |
| 27 | Plan status footer | No | Strip |
| 28 | Step 0: Detect platform and base branch | Partial | Keep only if needed for review |

**Minimal header for each review skill (~50 lines):**
```yaml
---
name: vc-plan-xxx-review
description: "..."
---

# Skill Title

This skill is part of vibe-engineering-mastery. All artifacts are written to `docs/`.

Read `references/askuser-format.md` for the AskUserQuestion decision brief format.

## Voice

- Lead with the point. Concrete, not abstract.
- Name files, functions, line numbers.
- Bias toward explicit over clever.
- No AI vocabulary: robust, comprehensive, nuanced, etc.
- The user decides.

## Confusion Protocol

For high-stakes ambiguity, STOP. Name it, present 2-3 options with tradeoffs, ask.

## Completion Status Protocol

Report: DONE, DONE_WITH_CONCERNS, BLOCKED, NEEDS_CONTEXT.
Escalate after 3 failed attempts.
```

Then proceed directly to the unique review content.

---

## 5. Yegge Loop Reference (for `references/yegge-loop.md`)

**Creator:** Kimi K2.6 — "thorough, structured, TDD-focused, reasons holistically about dependencies, catches architectural gaps early, explicit over clever."

**Reviewer:** Kimi K2.6 (self-critique) or subagent with same persona — "meticulous, contrarian, hunts edge cases, challenges assumptions."

**Loop Protocol:**
- Iteration 1 — WRITE: best first draft, STOP, present, ask for feedback
- Iterations 2+ — REFINE: COLLECT → CONTRADICTION CHECK → SELF-CRITIQUE (6 dimensions) → SYNTHESIZE → GATE
- Minimum 5 iterations
- Continue beyond 5 if user has feedback
- Stop on contradiction (explain conflict, ask user)
- Preserve structure (Goal, Current Phase, Phases, Key Questions, Decisions Made, Errors Encountered, Notes)
- Track deltas in Notes section

**6 Dimensions for Self-Critique:**
1. TDD coverage — verifiable success criteria per phase?
2. Quality gates — sharp phase boundaries?
3. Dependency gaps — hidden dependencies surfaced?
4. Missing test scenarios — error paths, edge cases, rollback?
5. Scope creep — grown beyond brainstorm spec?
6. Token density — lean, no filler?

**Subagent reviewer pattern:**
```
If platform supports subagent dispatch:
  Spawn reviewer subagent with Kimi K2.6 reviewer prompt.
  Pass: current plan + original spec + 6-dimension rubric.
  Incorporate findings into next iteration.
Else:
  Perform inline 6-dimension self-critique.
```

**Skip conditions:**
- User explicitly says "this plan is good, proceed"
- Plan has 2-3 phases AND user confirms clarity in one sentence
- Invoked from automated pipeline

---

## 6. GLM-5.1 Task Breakdown Phase

**When:** After Yegge Loop approves the plan.
**What:** Break the approved plan into bite-sized, executable tasks.
**How:**
1. Read approved `docs/PLAN.md`
2. Spawn subagent with GLM-5.1 persona:
   "You are GLM-5.1 — mechanically precise, exhaustive, granular. Break this plan into tasks where each task is one action (2-5 minutes). For each task: exact file path, specific command, expected output, commit message. DRY. YAGNI. TDD."
3. Subagent outputs: `docs/plans/YYYY-MM-DD-<feature>-tasks.md`
4. Kimi K2.6 reviews task breakdown for:
   - Dependency ordering (are tasks in the right sequence?)
   - Completeness (did GLM-5.1 miss any plan phases?)
   - Granularity (are tasks truly bite-sized?)
   - Test coverage (is TDD preserved in task form?)
5. User approves final task list.

---

## 7. Artifact Locations (Final)

| Artifact | Location |
|----------|----------|
| Strategy | `docs/STRATEGY.md` |
| Autoplan report | `docs/reviews/vc-autoplan-YYYY-MM-DD.md` |
| CEO Review | `docs/reviews/vc-plan-ceo-review-YYYY-MM-DD.md` |
| Eng Review | `docs/reviews/vc-plan-eng-review-YYYY-MM-DD.md` |
| Design Review | `docs/reviews/vc-plan-design-review-YYYY-MM-DD.md` |
| DevEx Review | `docs/reviews/vc-plan-devex-review-YYYY-MM-DD.md` |
| Plan (draft) | `docs/plans/YYYY-MM-DD-<feature>-plan.md` |
| Plan (approved) | `docs/PLAN.md` |
| Tasks (GLM-5.1) | `docs/plans/YYYY-MM-DD-<feature>-tasks.md` |
| Findings | `docs/findings.md` |

---

## 8. Remaining Build Order

1. ~~Root scaffolding~~ — DONE
2. ~~vc-strategy~~ — DONE
3. **vc-orchestrator** + `references/yegge-loop.md`
4. **shared `references/askuser-format.md`** — extract from gstack preamble
5. **vc-plan-ceo-review** — port unique content, strip preamble
6. **vc-plan-eng-review** — port unique content, strip preamble
7. **vc-plan-design-review** — port unique content, strip preamble, degrade mockups
8. **vc-plan-devex-review** — port unique content, strip preamble, inline hall-of-fame
9. **vc-autoplan** — port auto-decision principles + sequential logic
10. **vc-writing-plans** — port with Kimi K2.6 creator + GLM-5.1 task breakdown

---

## 9. Source File Read Status

| Source | Lines Read | Preamble Length | Unique Content Start | Status |
|--------|-----------|-----------------|---------------------|--------|
| `ce-strategy/SKILL.md` | All | None | N/A | Fully read |
| `ce-strategy/references/interview.md` | All | N/A | N/A | Fully read |
| `ce-strategy/references/strategy-template.md` | All | N/A | N/A | Fully read |
| `plan-ceo-review/SKILL.md` | All (~1787) | ~835 | Line 835 | Fully read |
| `plan-eng-review/SKILL.md` | All (~1655) | ~959 | Line 959 | Fully read |
| `plan-design-review/SKILL.md` | All (~1848) | ~1026 | Line 1026 | Fully read |
| `plan-devex-review/SKILL.md` | All (~2049) | ~1058 | Line 1058 | Fully read |
| `autoplan/SKILL.md` | All (~1733) | ~868 | Line 869 | **Fully read** |
| `writing-plans/SKILL.md` | All (~152) | None | N/A | Fully read |
| `writing-plans/plan-document-reviewer-prompt.md` | Not read | N/A | N/A | UNREAD |

**Note:** `autoplan` is now FULLY READ. Key findings captured in section 3.6. The remaining source files still need to be read: `writing-plans/plan-document-reviewer-prompt.md`.

---

## 10. vibe-chaos-to-concept Update Required

The user wants the **same model pattern** applied to `vibe-chaos-to-concept`:
- `vc-plan` (writing-plans equivalent) should use **Kimi K2.6** for plan creation + Yegge Loop refinement
- After plan approval, **GLM-5.1** breaks into tasks
- **Kimi K2.6** reviews the broken-down tasks

This requires updating:
- `vibe-chaos-to-concept/skills/vc-plan/SKILL.md` — add Kimi K2.6 creator persona, add GLM-5.1 task breakdown phase after Yegge Loop
- `vibe-chaos-to-concept/AGENTS.md` — document the model assignment pattern

Do this AFTER completing vibe-engineering-mastery.

---

## 11. Next Step (Immediate)

**Before compaction, build these in order:**
1. ~~Read remaining `autoplan/SKILL.md` content~~ — DONE
2. Write `skills/vc-orchestrator/references/yegge-loop.md`
3. Write `skills/vc-orchestrator/SKILL.md`
4. Write shared `skills/vc-plan-ceo-review/references/askuser-format.md`
5. Write `skills/vc-plan-ceo-review/SKILL.md`
6. Then continue with eng, design, devex, autoplan, writing-plans.
