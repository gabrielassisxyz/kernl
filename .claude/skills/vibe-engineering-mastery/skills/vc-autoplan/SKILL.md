---
name: vc-autoplan
description: 'Run all four reviews autonomously with auto-decisions and produce a consolidated review report. Use when user says run autoplan, auto review, run all reviews, quick review everything, or autoplan this. Also triggers on one-command review, batch review, or review all dimensions. Use for "batch review" or "automated review" sessions.'
user-invocable: true
---

# VC Autoplan

This skill is part of vibe-engineering-mastery. All artifacts are written to `docs/`.

Read `../../references/askuser-format.md` for the AskUserQuestion decision brief format.

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

---

# /vc-autoplan — Auto-Review Pipeline

One command. Rough plan in, fully reviewed plan out.

/vc-autoplan reads the full CEO, design, eng, and DX review skill files from disk and follows
them at full depth — same rigor, same sections, same methodology as running each skill
manually. The only difference: intermediate AskUserQuestion calls are auto-decided using
the 6 principles below. Taste decisions (where reasonable people could disagree) are
surfaced at a final approval gate.

---

## The 6 Decision Principles

These rules auto-answer every intermediate question:

1. **Choose completeness** — Ship the whole thing. Pick the approach that covers more edge cases.
2. **Boil lakes** — Fix everything in the blast radius (files modified by this plan + direct importers). Auto-approve expansions that are in blast radius AND < 1 day CC effort (< 5 files, no new infra).
3. **Pragmatic** — If two options fix the same thing, pick the cleaner one. 5 seconds choosing, not 5 minutes.
4. **DRY** — Duplicates existing functionality? Reject. Reuse what exists.
5. **Explicit over clever** — 10-line obvious fix > 200-line abstraction. Pick what a new contributor reads in 30 seconds.
6. **Bias toward action** — Merge > review cycles > stale deliberation. Flag concerns but don't block.

**Conflict resolution (context-dependent tiebreakers):**
- **CEO phase:** P1 (completeness) + P2 (boil lakes) dominate.
- **Eng phase:** P5 (explicit) + P3 (pragmatic) dominate.
- **Design phase:** P5 (explicit) + P1 (completeness) dominate.

---

## Decision Classification

Every auto-decision is classified:

**Mechanical** — one clearly right answer. Auto-decide silently.
Examples: run evals (always yes), reduce scope on a complete plan (always no).

**Taste** — reasonable people could disagree. Auto-decide with recommendation, but surface at the final gate. Three natural sources:
1. **Close approaches** — top two are both viable with different tradeoffs.
2. **Borderline scope** — in blast radius but 3-5 files, or ambiguous radius.
3. **Outside voice disagreements** — outside subagent recommends differently and has a valid point.

**User Challenge** — when the review and outside voice both agree the user's stated direction should change.
This is qualitatively different from taste decisions. When both the primary review and the outside
subagent recommend merging, splitting, adding, or removing features/skills/workflows that
the user specified, this is a User Challenge. It is NEVER auto-decided.

User Challenges go to the final approval gate with richer context than taste
decisions:
- **What the user said:** (their original direction)
- **What both reviews recommend:** (the change)
- **Why:** (the reasoning)
- **What context we might be missing:** (explicit acknowledgment of blind spots)
- **If we're wrong, the cost is:** (what happens if the user's original direction
  was right and we changed it)

The user's original direction is the default. The review must make the case for
change, not the other way around.

**Exception:** If both the primary review and outside subagent flag the change as a security vulnerability or
feasibility blocker (not a preference), the AskUserQuestion framing explicitly
warns: "Both reviews believe this is a security/feasibility risk, not just a
preference." The user still decides, but the framing is appropriately urgent.

---

## Sequential Execution — MANDATORY

Phases MUST execute in strict order: CEO → Design → Eng → DX.
Each phase MUST complete fully before the next begins.
NEVER run phases in parallel — each builds on the previous.

Between each phase, emit a phase-transition summary and verify that all required
outputs from the prior phase are written before starting the next.

---

## What "Auto-Decide" Means

Auto-decide replaces the USER'S judgment with the 6 principles. It does NOT replace
the ANALYSIS. Every section in the loaded skill files must still be executed at the
same depth as the interactive version. The only thing that changes is who answers the
AskUserQuestion: you do, using the 6 principles, instead of the user.

**Two exceptions — never auto-decided:**
1. Premises (Phase 1) — require human judgment about what problem to solve.
2. User Challenges — when both reviews agree the user's stated direction should change
   (merge, split, add, remove features/workflows). The user always has context the
   review lacks. See Decision Classification above.

**You MUST still:**
- READ the actual code, diffs, and files each section references
- PRODUCE every output the section requires (diagrams, tables, registries, artifacts)
- IDENTIFY every issue the section is designed to catch
- DECIDE each issue using the 6 principles (instead of asking the user)
- LOG each decision in the audit trail
- WRITE all required artifacts to disk

**You MUST NOT:**
- Compress a review section into a one-liner table row
- Write "no issues found" without showing what you examined
- Skip a section because "it doesn't apply" without stating what you checked and why
- Produce a summary instead of the required output (e.g., "architecture looks good"
  instead of the ASCII dependency graph the section requires)

"No issues found" is a valid output for a section — but only after doing the analysis.
State what you examined and why nothing was flagged (1-2 sentences minimum).
"Skipped" is never valid for a non-skip-listed section.

---

## Phase 0: Intake + Restore Point

### Step 1: Capture restore point

Before doing anything, save the plan file's current state:

```bash
mkdir -p docs/reviews
BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null | tr '/' '-')
DATETIME=$(date +%Y%m%d-%H%M%S)
RESTORE_PATH="docs/reviews/${BRANCH}-autoplan-restore-${DATETIME}.md"
echo "Restore point: ${RESTORE_PATH}"
```

Write the plan file's full contents to the restore path with this header:
```
# /vc-autoplan Restore Point
Captured: [timestamp] | Branch: [branch] | Commit: [short hash]

## Re-run Instructions
1. Copy "Original Plan State" below back to your plan file
2. Invoke /vc-autoplan

## Original Plan State
[verbatim plan file contents]
```

Then prepend a one-line HTML comment to the plan file:
`<!-- /vc-autoplan restore point: [RESTORE_PATH] -->`

### Step 2: Read context

- Read CLAUDE.md, TODOS.md, git log -30, git diff against the base branch --stat
- Discover design docs: `ls -t docs/*design*.md 2>/dev/null | head -1`
- Detect UI scope: grep the plan for view/rendering terms (component, screen, form,
  button, modal, layout, dashboard, sidebar, nav, dialog). Require 2+ matches. Exclude
  false positives ("page" alone, "UI" in acronyms).
- Detect DX scope: grep the plan for developer-facing terms (API, endpoint, REST,
  GraphQL, gRPC, webhook, CLI, command, flag, argument, terminal, shell, SDK, library,
  package, npm, pip, import, require, SKILL.md, skill template, Claude Code, MCP, agent,
  action, developer docs, getting started, onboarding, integration, debug,
  implement, error message). Require 2+ matches. Also trigger DX scope if the product IS
  a developer tool (the plan describes something developers install, integrate, or build
  on top of) or if an AI agent is the primary user (Claude Code skills, MCP servers).

### Step 3: Load skill files from disk

Resolve sibling skill paths. If `CLAUDE_SKILL_DIR` environment variable is set:
- `$CLAUDE_SKILL_DIR/vc-plan-ceo-review/SKILL.md`
- `$CLAUDE_SKILL_DIR/vc-plan-design-review/SKILL.md` (only if UI scope detected)
- `$CLAUDE_SKILL_DIR/vc-plan-eng-review/SKILL.md`
- `$CLAUDE_SKILL_DIR/vc-plan-devex-review/SKILL.md` (only if DX scope detected)

If `CLAUDE_SKILL_DIR` is NOT set, search relative to the current skill's directory:
use Glob to find `vc-plan-ceo-review/SKILL.md`, `vc-plan-eng-review/SKILL.md`, etc.
in sibling directories under `skills/`.

Read each file using the Read tool.

**Section skip list — when following a loaded skill file, SKIP these sections
(they are already handled by /vc-autoplan):**
- Preamble (run first)
- AskUserQuestion Format
- Completeness Principle — Boil the Lake
- Search Before Building
- Completion Status Protocol
- Telemetry (run last)
- Step 0: Detect base branch
- Review Readiness Dashboard
- Plan File Review Report
- Prerequisite Skill Offer
- Outside Voice — Independent Plan Challenge
- Design Outside Voices

Follow ONLY the review-specific methodology, sections, and required outputs.

Output: "Here's what I'm working with: [plan summary]. UI scope: [yes/no]. DX scope: [yes/no].
Loaded review skills from disk. Starting full review pipeline with auto-decisions."

---

## Phase 0.5: Subagent Availability Check

Before invoking any outside voice subagent, check if the platform supports it:

Attempt to determine if Agent tool / subagent dispatch is available. If the platform
supports spawning subagents:

```
Outside voice: AVAILABLE — will run independent subagent reviews per phase.
```

If subagent dispatch is NOT available:

```
Outside voice: UNAVAILABLE — proceeding with single-reviewer inline review only.
```

If subagent dispatch is unavailable, all Phase 1-3.5 outside voice subagent calls
are skipped. The review completes with primary inline review only. Consensus tables are
skipped (no second voice to compare).

---

## Phase 1: CEO Review (Strategy & Scope)

Follow vc-plan-ceo-review/SKILL.md — all sections, full depth.
Override: every AskUserQuestion → auto-decide using the 6 principles.

**Override rules:**
- Mode selection: SELECTIVE EXPANSION
- Premises: accept reasonable ones (P6), challenge only clearly wrong ones
- **GATE: Present premises to user for confirmation** — this is the ONE AskUserQuestion
  that is NOT auto-decided. Premises require human judgment.
- Alternatives: pick highest completeness (P1). If tied, pick simplest (P5).
  If top 2 are close → mark TASTE DECISION.
- Scope expansion: in blast radius + <1d CC → approve (P2). Outside → defer to TODOS.md (P3).
  Duplicates → reject (P4). Borderline (3-5 files) → mark TASTE DECISION.
- All review sections: run fully, auto-decide each issue, log every decision.
- **Outside voice:** if subagent available, run an independent subagent review (P6).
  Run the primary review first, then the subagent (foreground — do NOT use background).
  Both must complete before building the consensus table.

  **Outside CEO subagent** (via Agent tool):
  "Read the plan file at <plan_path>. You are an independent CEO/strategist
  reviewing this plan. You have NOT seen any prior review. Evaluate:
  1. Is this the right problem to solve? Could a reframing yield 10x impact?
  2. Are the premises stated or just assumed? Which ones could be wrong?
  3. What's the 6-month regret scenario — what will look foolish?
  4. What alternatives were dismissed without sufficient analysis?
  5. What's the competitive risk — could someone else solve this first/better?
  For each finding: what's wrong, severity (critical/high/medium), and the fix."

  **Error handling:** If subagent fails or is unavailable → proceed with
  primary review only, tagged `[single-reviewer]`.

  **Degradation matrix:**
  - Both available → dual review with consensus table
  - Subagent unavailable → "single-reviewer mode — proceeding with primary review only"

- Strategy choices: if outside subagent disagrees with a premise or scope decision with valid
  strategic reason → TASTE DECISION. If both reviews agree the user's stated structure
  should change (merge, split, add, remove) → USER CHALLENGE (never auto-decided).

**Required execution checklist (CEO):**

Step 0 (0A-0F) — run each sub-step and produce:
- 0A: Premise challenge with specific premises named and evaluated
- 0B: Existing code leverage map (sub-problems → existing code)
- 0C: Dream state diagram (CURRENT → THIS PLAN → 12-MONTH IDEAL)
- 0C-bis: Implementation alternatives table (2-3 approaches with effort/risk/pros/cons)
- 0D: Mode-specific analysis with scope decisions logged
- 0E: Temporal interrogation (HOUR 1 → HOUR 6+)
- 0F: Mode selection confirmation

Step 0.5 (Outside Voice): If subagent available, run primary review first, then
the outside subagent. Present subagent output under OUTSIDE SUBAGENT (CEO — strategic
independence) header. Produce CEO consensus table:

```
CEO REVIEW — CONSENSUS TABLE:
═══════════════════════════════════════════════════════════════
  Dimension                           Primary  Outside  Consensus
  ──────────────────────────────────── ──────── ──────── ─────────
  1. Premises valid?                   —        —        —
  2. Right problem to solve?           —        —        —
  3. Scope calibration correct?        —        —        —
  4. Alternatives sufficiently explored?—       —        —
  5. Competitive/market risks covered? —        —        —
  6. 6-month trajectory sound?         —        —        —
═══════════════════════════════════════════════════════════════
CONFIRMED = both agree. DISAGREE = reviews differ (→ taste decision).
Missing outside voice = N/A (not CONFIRMED). Single critical finding from one voice = flagged regardless.
```

If subagent unavailable, skip consensus table — note "single-reviewer only."

Sections 1-11 — for EACH section, run the evaluation criteria from the loaded skill file:
- Sections WITH findings: full analysis, auto-decide each issue, log to audit trail
- Sections with NO findings: 1-2 sentences stating what was examined and why nothing
  was flagged. NEVER compress a section to just its name in a table row.
- Section 11 (Design): run only if UI scope was detected in Phase 0

**Mandatory outputs from Phase 1:**
- "NOT in scope" section with deferred items and rationale
- "What already exists" section mapping sub-problems to existing code
- Error & Rescue Registry table (from Section 2)
- Failure Modes Registry table (from review sections)
- Dream state delta (where this plan leaves us vs 12-month ideal)
- Completion Summary (the full summary table from the CEO skill)

**PHASE 1 COMPLETE.** Emit phase-transition summary:
> **Phase 1 complete.** Outside subagent: [N concerns] (or "unavailable").
> Consensus: [X/6 confirmed, Y disagreements → surfaced at gate].
> Passing to Phase 2.

Do NOT begin Phase 2 until all Phase 1 outputs are written to the plan file
and the premise gate has been passed.

---

**Pre-Phase 2 checklist (verify before starting):**
- [ ] CEO completion summary written to plan file
- [ ] CEO outside voice ran (if available) or noted unavailable
- [ ] CEO consensus table produced (if dual voice) or noted single-reviewer
- [ ] Premise gate passed (user confirmed)
- [ ] Phase-transition summary emitted

## Phase 2: Design Review (conditional — skip if no UI scope)

Follow vc-plan-design-review/SKILL.md — all 7 dimensions, full depth.
Override: every AskUserQuestion → auto-decide using the 6 principles.

**Override rules:**
- Focus areas: all relevant dimensions (P1)
- Structural issues (missing states, broken hierarchy): auto-fix (P5)
- Aesthetic/taste issues: mark TASTE DECISION
- Design system alignment: auto-fix if DESIGN.md exists and fix is obvious
- Dual voices: run outside subagent if available (P6).

  **Outside design subagent** (via Agent tool):
  "Read the plan file at <plan_path>. You are an independent senior product designer
  reviewing this plan. You have NOT seen any prior review. Evaluate:
  1. Information hierarchy: what does the user see first, second, third? Is it right?
  2. Missing states: loading, empty, error, success, partial — which are unspecified?
  3. User journey: what's the emotional arc? Where does it break?
  4. Specificity: does the plan describe SPECIFIC UI or generic patterns?
  5. What design decisions will haunt the implementer if left ambiguous?
  For each finding: what's wrong, severity (critical/high/medium), and the fix."
  NO prior-phase context — subagent must be truly independent.

  Error handling: same as Phase 1 (foreground/blocking, degradation matrix applies).

- Design choices: if outside subagent disagrees with a design decision with valid UX reasoning
  → TASTE DECISION. Scope changes both reviews agree on → USER CHALLENGE.

**Required execution checklist (Design):**

1. Step 0 (Design Scope): Rate completeness 0-10. Check DESIGN.md. Map existing patterns.

2. Step 0.5 (Outside Voice): If available, run primary review first, then the outside subagent.
   Present under OUTSIDE SUBAGENT (design — independent review) header.
   Produce design litmus scorecard (consensus table). Use the litmus scorecard
   format from vc-plan-design-review. Include CEO phase findings in primary review
   context ONLY (not outside subagent — stays independent).

3. Passes 1-7: Run each from loaded skill. Rate 0-10. Auto-decide each issue.
   DISAGREE items from scorecard → raised in the relevant pass with both perspectives.

**PHASE 2 COMPLETE.** Emit phase-transition summary:
> **Phase 2 complete.** Outside subagent: [N concerns] (or "unavailable").
> Consensus: [X/Y confirmed, Z disagreements → surfaced at gate].
> Passing to Phase 3.

Do NOT begin Phase 3 until all Phase 2 outputs (if run) are written to the plan file.

---

**Pre-Phase 3 checklist (verify before starting):**
- [ ] All Phase 1 items above confirmed
- [ ] Design completion summary written (or "skipped, no UI scope")
- [ ] Design outside voice ran (if Phase 2 ran and subagent available)
- [ ] Design consensus table produced (if Phase 2 ran and dual voice)
- [ ] Phase-transition summary emitted

## Phase 3: Eng Review

Follow vc-plan-eng-review/SKILL.md — all sections, full depth.
Override: every AskUserQuestion → auto-decide using the 6 principles.

**Override rules:**
- Scope challenge: never reduce (P2)
- Dual voices: run outside subagent if available (P6).

  **Outside eng subagent** (via Agent tool):
  "Read the plan file at <plan_path>. You are an independent senior engineer
  reviewing this plan. You have NOT seen any prior review. Evaluate:
  1. Architecture: Is the component structure sound? Coupling concerns?
  2. Edge cases: What breaks under 10x load? What's the nil/empty/error path?
  3. Tests: What's missing from the test plan? What would break at 2am Friday?
  4. Security: New attack surface? Auth boundaries? Input validation?
  5. Hidden complexity: What looks simple but isn't?
  For each finding: what's wrong, severity, and the fix."
  NO prior-phase context — subagent must be truly independent.

  Error handling: same as Phase 1 (foreground/blocking, degradation matrix applies).

- Architecture choices: explicit over clever (P5). If outside subagent disagrees with valid
  reason → TASTE DECISION. Scope changes both reviews agree on → USER CHALLENGE.
- Evals: always include all relevant suites (P1)
- Test plan: generate artifact at `docs/reviews/{branch}-test-plan-{datetime}.md`
- TODOS.md: collect all deferred scope expansions from Phase 1, auto-write

**Required execution checklist (Eng):**

1. Step 0 (Scope Challenge): Read actual code referenced by the plan. Map each
   sub-problem to existing code. Run the complexity check. Produce concrete findings.

2. Step 0.5 (Outside Voice): If available, run primary review first, then the outside subagent.
   Present subagent output under OUTSIDE SUBAGENT (eng — independent review) header.
   Produce eng consensus table:

```
ENG REVIEW — CONSENSUS TABLE:
═══════════════════════════════════════════════════════════════
  Dimension                           Primary  Outside  Consensus
  ──────────────────────────────────── ──────── ──────── ─────────
  1. Architecture sound?               —        —        —
  2. Test coverage sufficient?         —        —        —
  3. Performance risks addressed?      —        —        —
  4. Security threats covered?         —        —        —
  5. Error paths handled?              —        —        —
  6. Deployment risk manageable?       —        —        —
═══════════════════════════════════════════════════════════════
CONFIRMED = both agree. DISAGREE = reviews differ (→ taste decision).
Missing outside voice = N/A (not CONFIRMED). Single critical finding from one voice = flagged regardless.
```

If subagent unavailable, skip consensus table — note "single-reviewer only."

3. Section 1 (Architecture): Produce ASCII dependency graph showing new components
   and their relationships to existing ones. Evaluate coupling, scaling, security.

4. Section 2 (Code Quality): Identify DRY violations, naming issues, complexity.
   Reference specific files and patterns. Auto-decide each finding.

5. **Section 3 (Test Review) — NEVER SKIP OR COMPRESS.**
   This section requires reading actual code, not summarizing from memory.
   - Read the diff or the plan's affected files
   - Build the test diagram: list every NEW UX flow, data flow, codepath, and branch
   - For EACH item in the diagram: what type of test covers it? Does one exist? Gaps?
   - For LLM/prompt changes: which eval suites must run?
   - Auto-deciding test gaps means: identify the gap → decide whether to add a test
     or defer (with rationale and principle) → log the decision. It does NOT mean
     skipping the analysis.
   - Write the test plan artifact to disk: `docs/reviews/{branch}-test-plan-{datetime}.md`

6. Section 4 (Performance): Evaluate N+1 queries, memory, caching, slow paths.

**Mandatory outputs from Phase 3:**
- "NOT in scope" section
- "What already exists" section
- Architecture ASCII diagram (Section 1)
- Test diagram mapping codepaths to coverage (Section 3)
- Test plan artifact written to disk (Section 3)
- Failure modes registry with critical gap flags
- Completion Summary (the full summary from the Eng skill)
- TODOS.md updates (collected from all phases)

**PHASE 3 COMPLETE.** Emit phase-transition summary:
> **Phase 3 complete.** Outside subagent: [N concerns] (or "unavailable").
> Consensus: [X/6 confirmed, Y disagreements → surfaced at gate].
> Passing to Phase 3.5 (DX Review) or Phase 4 (Final Gate).

---

## Phase 3.5: DX Review (conditional — skip if no developer-facing scope)

Follow vc-plan-devex-review/SKILL.md — all 8 DX dimensions, full depth.
Override: every AskUserQuestion → auto-decide using the 6 principles.

**Skip condition:** If DX scope was NOT detected in Phase 0, skip this phase entirely.
Log: "Phase 3.5 skipped — no developer-facing scope detected."

**Override rules:**
- Mode selection: DX POLISH
- Persona: infer from README/docs, pick the most common developer type (P6)
- Competitive benchmark: run searches if WebSearch available, use reference benchmarks otherwise (P1)
- Magical moment: pick the lowest-effort delivery vehicle that achieves the competitive tier (P5)
- Getting started friction: always optimize toward fewer steps (P5, simpler over clever)
- Error message quality: always require problem + cause + fix (P1, completeness)
- API/CLI naming: consistency wins over cleverness (P5)
- DX taste decisions (e.g., opinionated defaults vs flexibility): mark TASTE DECISION
- Dual voices: run outside subagent if available (P6).

  **Outside DX subagent** (via Agent tool):
  "Read the plan file at <plan_path>. You are an independent DX engineer
  reviewing this plan. You have NOT seen any prior review. Evaluate:
  1. Getting started: how many steps from zero to hello world? What's the TTHW?
  2. API/CLI ergonomics: naming consistency, sensible defaults, progressive disclosure?
  3. Error handling: does every error path specify problem + cause + fix + docs link?
  4. Documentation: copy-paste examples? Information architecture? Interactive elements?
  5. Escape hatches: can developers override every opinionated default?
  For each finding: what's wrong, severity (critical/high/medium), and the fix."
  NO prior-phase context — subagent must be truly independent.

  Error handling: same as Phase 1 (foreground/blocking, degradation matrix applies).

- DX choices: if outside subagent disagrees with a DX decision with valid developer empathy reasoning
  → TASTE DECISION. Scope changes both reviews agree on → USER CHALLENGE.

**Required execution checklist (DX):**

1. Step 0 (DX Scope Assessment): Auto-detect product type. Map the developer journey.
   Rate initial DX completeness 0-10. Assess TTHW.

2. Step 0.5 (Outside Voice): If available, run primary review first, then the outside subagent.
   Present under OUTSIDE SUBAGENT (DX — independent review) header.
   Produce DX consensus table:

```
DX REVIEW — CONSENSUS TABLE:
═══════════════════════════════════════════════════════════════
  Dimension                           Primary  Outside  Consensus
  ──────────────────────────────────── ──────── ──────── ─────────
  1. Getting started < 5 min?          —        —        —
  2. API/CLI naming guessable?         —        —        —
  3. Error messages actionable?        —        —        —
  4. Docs findable & complete?         —        —        —
  5. Upgrade path safe?                —        —        —
  6. Dev environment friction-free?    —        —        —
═══════════════════════════════════════════════════════════════
CONFIRMED = both agree. DISAGREE = reviews differ (→ taste decision).
Missing outside voice = N/A (not CONFIRMED). Single critical finding from one voice = flagged regardless.
```

If subagent unavailable, skip consensus table — note "single-reviewer only."

3. Passes 1-8: Run each from loaded skill. Rate 0-10. Auto-decide each issue.
   DISAGREE items from consensus table → raised in the relevant pass with both perspectives.

4. DX Scorecard: Produce the full scorecard with all 8 dimensions scored.

**Mandatory outputs from Phase 3.5:**
- Developer journey map (9-stage table)
- Developer empathy narrative (first-person perspective)
- DX Scorecard with all 8 dimension scores
- DX Implementation Checklist
- TTHW assessment with target

**PHASE 3.5 COMPLETE.** Emit phase-transition summary:
> **Phase 3.5 complete.** DX overall: [N]/10. TTHW: [N] min → [target] min.
> Outside subagent: [N concerns] (or "unavailable").
> Consensus: [X/6 confirmed, Y disagreements → surfaced at gate].
> Passing to Phase 4 (Final Gate).

---

## Decision Audit Trail

After each auto-decision, append a row to the plan file using Edit:

```markdown
<!-- AUTONOMOUS DECISION LOG -->
## Decision Audit Trail

| # | Phase | Decision | Classification | Principle | Rationale | Rejected |
|---|-------|----------|-----------|-----------|----------|
```

Write one row per decision incrementally (via Edit). This keeps the audit on disk,
not accumulated in conversation context.

---

## Pre-Gate Verification

Before presenting the Final Approval Gate, verify that required outputs were actually
produced. Check the plan file and conversation for each item.

**Phase 1 (CEO) outputs:**
- [ ] Premise challenge with specific premises named (not just "premises accepted")
- [ ] All applicable review sections have findings OR explicit "examined X, nothing flagged"
- [ ] Error & Rescue Registry table produced (or noted N/A with reason)
- [ ] Failure Modes Registry table produced (or noted N/A with reason)
- [ ] "NOT in scope" section written
- [ ] "What already exists" section written
- [ ] Dream state delta written
- [ ] Completion Summary produced
- [ ] Outside voice ran (if available) or noted unavailable
- [ ] CEO consensus table produced (if dual voice) or noted single-reviewer

**Phase 2 (Design) outputs — only if UI scope detected:**
- [ ] All 7 dimensions evaluated with scores
- [ ] Issues identified and auto-decided
- [ ] Outside voice ran (if available) or noted unavailable
- [ ] Design litmus scorecard produced

**Phase 3 (Eng) outputs:**
- [ ] Scope challenge with actual code analysis (not just "scope is fine")
- [ ] Architecture ASCII diagram produced
- [ ] Test diagram mapping codepaths to test coverage
- [ ] Test plan artifact written to disk at `docs/reviews/`
- [ ] "NOT in scope" section written
- [ ] "What already exists" section written
- [ ] Failure modes registry with critical gap assessment
- [ ] Completion Summary produced
- [ ] Outside voice ran (if available) or noted unavailable
- [ ] Eng consensus table produced (if dual voice) or noted single-reviewer

**Phase 3.5 (DX) outputs — only if DX scope detected:**
- [ ] All 8 DX dimensions evaluated with scores
- [ ] Developer journey map produced
- [ ] Developer empathy narrative written
- [ ] TTHW assessment with target
- [ ] DX Implementation Checklist produced
- [ ] Outside voice ran (if available) or noted unavailable
- [ ] DX consensus table produced (if dual voice) or noted single-reviewer

**Cross-phase:**
- [ ] Cross-phase themes section written

**Audit trail:**
- [ ] Decision Audit Trail has at least one row per auto-decision (not empty)

If ANY checkbox above is missing, go back and produce the missing output. Max 2
attempts — if still missing after retrying twice, proceed to the gate with a warning
noting which items are incomplete. Do not loop indefinitely.

---

## Phase 4: Final Approval Gate

**STOP here and present the final state to the user.**

Present as a message, then use AskUserQuestion:

```
## /vc-autoplan Review Complete

### Plan Summary
[1-3 sentence summary]

### Decisions Made: [N] total ([M] auto-decided, [K] taste choices, [J] user challenges)

### User Challenges (both reviews disagree with your stated direction)
[For each user challenge:]
**Challenge [N]: [title]** (from [phase])
You said: [user's original direction]
Both reviews recommend: [the change]
Why: [reasoning]
What we might be missing: [blind spots]
If we're wrong, the cost is: [downside of changing]
[If security/feasibility: "⚠️ Both reviews flag this as a security/feasibility risk,
not just a preference."]

Your call — your original direction stands unless you explicitly change it.

### Your Choices (taste decisions)
[For each taste decision:]
**Choice [N]: [title]** (from [phase])
I recommend [X] — [principle]. But [Y] is also viable:
  [1-sentence downstream impact if you pick Y]

### Auto-Decided: [M] decisions [see Decision Audit Trail in plan file]

### Review Scores
- CEO: [summary]
- CEO Outside Voice: [summary or "unavailable"], Consensus [X/6 confirmed]
- Design: [summary or "skipped, no UI scope"]
- Design Outside Voice: [summary or "unavailable"], Consensus [X/7 confirmed] (or "skipped")
- Eng: [summary]
- Eng Outside Voice: [summary or "unavailable"], Consensus [X/6 confirmed]
- DX: [summary or "skipped, no developer-facing scope"]
- DX Outside Voice: [summary or "unavailable"], Consensus [X/6 confirmed] (or "skipped")

### Cross-Phase Themes
[For any concern that appeared in 2+ phases' reviews independently:]
**Theme: [topic]** — flagged in [Phase 1, Phase 3]. High-confidence signal.
[If no themes span phases:] "No cross-phase themes — each phase's concerns were distinct."

### Deferred to TODOS.md
[Items auto-deferred with reasons]
```

**Cognitive load management:**
- 0 user challenges: skip "User Challenges" section
- 0 taste decisions: skip "Your Choices" section
- 1-7 taste decisions: flat list
- 8+: group by phase. Add warning: "This plan had unusually high ambiguity ([N] taste decisions). Review carefully."

AskUserQuestion options:
- A) Approve as-is (accept all recommendations)
- B) Approve with overrides (specify which taste decisions to change)
- B2) Approve with user challenge responses (accept or reject each challenge)
- C) Interrogate (ask about any specific decision)
- D) Revise (the plan itself needs changes)
- E) Reject (start over)

**Option handling:**
- A: mark APPROVED, write the autoplan report to `docs/reviews/vc-autoplan-YYYY-MM-DD.md`
- B: ask which overrides, apply, re-present gate
- C: answer freeform, re-present gate
- D: make changes, re-run affected phases (scope→Phase 1, design→Phase 2, test plan→Phase 3, arch→Phase 3). Max 3 cycles.
- E: start over

On approval, write the consolidated report to `docs/reviews/vc-autoplan-YYYY-MM-DD.md`.

---

## Important Rules

- **Never abort.** The user chose /vc-autoplan. Respect that choice. Surface all taste decisions, never redirect to interactive review.
- **Two gates.** The non-auto-decided AskUserQuestions are: (1) premise confirmation in Phase 1, and (2) User Challenges — when both reviews agree the user's stated direction should change. Everything else is auto-decided using the 6 principles.
- **Log every decision.** No silent auto-decisions. Every choice gets a row in the audit trail.
- **Full depth means full depth.** Do not compress or skip sections from the loaded skill files (except the skip list in Phase 0). "Full depth" means: read the code the section asks you to read, produce the outputs the section requires, identify every issue, and decide each one. A one-sentence summary of a section is not "full depth" — it is a skip. If you catch yourself writing fewer than 3 sentences for any review section, you are likely compressing.
- **Artifacts are deliverables.** Test plan artifact, failure modes registry, error/rescue table, ASCII diagrams — these must exist on disk or in the plan file when the review completes. If they don't exist, the review is incomplete.
- **Sequential order.** CEO → Design → Eng → DX. Each phase builds on the last.
