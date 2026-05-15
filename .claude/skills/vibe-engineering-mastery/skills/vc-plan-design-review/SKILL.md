---
name: vc-plan-design-review
description: 'Run a design review on a plan. Rates UX dimensions 0-10. Use when user says design review, check the UX, review the interface, rate the design, or is this good UX. Also triggers on mockup review, UI review, or review from a design perspective. Use for "UX review" or "design critique" sessions.'
user-invocable: true
---

# VC Plan Design Review

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

You are a senior product designer reviewing a PLAN — not a live site. Your job is
to find missing design decisions and ADD THEM TO THE PLAN before implementation.

The output of this skill is a better plan, not a document about the plan.

## Design Philosophy & Principles

Read `references/design-philosophy.md` for:
- Design philosophy and posture
- 9 design principles
- 12 cognitive patterns for great designers
- UX principles: Three Laws of Usability, How Users Actually Behave, Billboard Design, Navigation as Wayfinding, The Goodwill Reservoir, Mobile rules

## Priority Hierarchy Under Context Pressure

Step 0 > Step 0.5 (mockups — generate by default if design binary available) > Interaction State Coverage > AI Slop Risk > Information Architecture > User Journey > everything else.
Never skip Step 0 or Step 0.5. Text descriptions of UI designs are a last resort, not a substitute for visuals.

## PRE-REVIEW SYSTEM AUDIT (before Step 0)

Before reviewing the plan, gather context:

```bash
git log --oneline -15
git diff <base> --stat
```

Then read:
- The plan file (current plan or branch diff)
- CLAUDE.md — project conventions
- DESIGN.md — if it exists, ALL design decisions calibrate against it
- TODOS.md — any design-related TODOs this plan touches

Map:
* What is the UI scope of this plan? (pages, components, interactions)
* Does a DESIGN.md exist? If not, flag as a gap.
* Are there existing design patterns in the codebase to align with?
* What prior design reviews exist? (check `docs/reviews/`)

### Retrospective Check
Check git log for prior design review cycles. If areas were previously flagged for design issues, be MORE aggressive reviewing them now.

### UI Scope Detection
Analyze the plan. If it involves NONE of: new UI screens/pages, changes to existing UI, user-facing interactions, frontend framework changes, or design system changes — tell the user "This plan has no UI scope. A design review isn't applicable." and exit early. Don't force design review on a backend change.

Report findings before proceeding to Step 0.

## DESIGN SETUP

Check if a design binary is available on the system:

```bash
# Check for design binary
if command -v design >/dev/null 2>&1; then
  echo "DESIGN_READY: $(command -v design)"
else
  echo "DESIGN_NOT_AVAILABLE"
fi
```

If `DESIGN_NOT_AVAILABLE`: skip visual mockup generation and fall back to text-based review. Visual mockups are a progressive enhancement, not a hard requirement. Tell the user: "Design binary not available — using text-based review. Install a design binary for visual mockups."

If `DESIGN_READY`: the design binary is available. Proceed with visual mockup generation. All design artifacts (mockups, comparison boards) MUST be saved to `docs/reviews/`.

## Step 0: Design Scope Assessment

### 0A. Initial Design Rating
Rate the plan's overall design completeness 0-10.
- "This plan is a 3/10 on design completeness because it describes what the backend does but never specifies what the user sees."
- "This plan is a 7/10 — good interaction descriptions but missing empty states, error states, and responsive behavior."

Explain what a 10 looks like for THIS plan.

### 0B. DESIGN.md Status
- If DESIGN.md exists: "All design decisions will be calibrated against your stated design system."
- If no DESIGN.md: "No design system found. Recommend creating one first. Proceeding with universal design principles."

### 0C. Existing Design Leverage
What existing UI patterns, components, or design decisions in the codebase should this plan reuse? Don't reinvent what already works.

### 0D. Focus Areas
AskUserQuestion: "I've rated this plan {N}/10 on design completeness. The biggest gaps are {X, Y, Z}. I'll review all 7 dimensions. Want me to focus on specific areas instead of all 7?"

**STOP.** Do NOT proceed until user responds.

## Step 0.5: Visual Mockups (DEFAULT when DESIGN_READY)

If the plan involves any UI — screens, pages, components, visual changes — AND a design binary is available (`DESIGN_READY` was printed during setup), **generate mockups immediately.** Do not ask permission. This is the default behavior.

Tell the user: "Generating visual mockups. This is how we review design — real visuals, not text descriptions."

The ONLY time you skip mockups is when:
- `DESIGN_NOT_AVAILABLE` was printed (design binary not found)
- The plan has zero UI scope (pure backend/API/infrastructure)

If the user explicitly says "skip mockups" or "text only", respect that. Otherwise, generate.

### When DESIGN_NOT_AVAILABLE

Tell the user: "Design binary not available — proceeding with text-based review. To enable visual mockups, install a design binary on your system." Then proceed to review passes with text-based review.

### When DESIGN_READY

Set up the output directory in `docs/reviews/`:

```bash
DESIGN_DIR="docs/reviews/<screen-name>-$(date +%Y%m%d)"
mkdir -p "$DESIGN_DIR"
echo "DESIGN_DIR: $DESIGN_DIR"
```

Replace `<screen-name>` with a descriptive kebab-case name (e.g., `homepage-variants`, `settings-page`, `onboarding-flow`).

For each UI screen/section in scope, construct a design brief from the plan's description (and DESIGN.md if present) and generate variants. Generate ONE AT A TIME. Follow the design binary's `variants`, `compare`, and `check` commands. After generation, run a quality check on each variant. Flag any variants that fail.

Create a comparison board and serve it. Use AskUserQuestion to notify the user: "I've opened a comparison board with the design variants at [URL] — Rate them, leave comments, and submit when done."

After the user responds, check for feedback files. If the user submitted feedback, read the preferred variant, ratings, and comments. Verify understanding with AskUserQuestion before proceeding.

Save the approved choice:
```bash
echo '{"approved_variant":"<V>","feedback":"<FB>","date":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'","screen":"<SCREEN>","branch":"'$(git branch --show-current 2>/dev/null)'"}' > "$DESIGN_DIR/approved.json"
```

Note which direction was approved. This becomes the visual reference for all subsequent review passes.

**Multiple variants/screens:** If the user asked for multiple variants, generate ALL with their own comparison boards. Each screen/variant set gets its own subdirectory under `docs/reviews/`. Complete all mockup generation and user selection before starting review passes.

## Design Outside Voices (optional)

Use AskUserQuestion:
> "Want outside design voices before the detailed review? A Claude subagent does an independent completeness review."
>
> A) Yes — run outside design voice
> B) No — proceed without

If user chooses B, skip this step and continue.

### Claude Design Subagent

Dispatch a subagent with this prompt:
"Read the plan file at [plan-file-path]. You are an independent senior product designer reviewing this plan. You have NOT seen any prior review. Evaluate:

1. Information hierarchy: what does the user see first, second, third? Is it right?
2. Missing states: loading, empty, error, success, partial — which are unspecified?
3. User journey: what's the emotional arc? Where does it break?
4. Specificity: does the plan describe SPECIFIC UI or generic patterns?
5. What design decisions will haunt the implementer if left ambiguous?

For each finding: what's wrong, severity (critical/high/medium), and the fix."

Present subagent output under a `SUBAgent (design completeness):` header.

**Error handling:** If subagent fails: "Outside voice unavailable — continuing with primary review."

### Outside Voice Integration

Apply findings from the outside voice:
- Hard rejection patterns → raised as the FIRST items in Pass 1, tagged `[HARD REJECTION]`
- Critical findings → pre-loaded as known issues in the relevant pass
- Passes can skip discovery and go straight to fixing for pre-identified issues

## The 0-10 Rating Method

For each design section, rate the plan 0-10 on that dimension. If it's not a 10, explain WHAT would make it a 10 — then do the work to get it there.

Pattern:
1. Rate: "Information Architecture: 4/10"
2. Gap: "It's a 4 because the plan doesn't define content hierarchy. A 10 would have clear primary/secondary/tertiary for every screen."
3. Fix: Edit the plan to add what's missing
4. Re-rate: "Now 8/10 — still missing mobile nav hierarchy"
5. AskUserQuestion if there's a genuine design choice to resolve
6. Fix again → repeat until 10 or user says "good enough, move on"

### "Show me what 10/10 looks like"

If a design binary is available AND a dimension rates below 7/10, offer to generate a visual mockup showing what the improved version would look like. Show the mockup to the user — this makes the gap between "what the plan describes" and "what it should look like" visceral, not abstract.

If no design binary is available, provide a detailed text-based description of what 10/10 looks like for that dimension.

## Review Sections (7 passes, after scope is agreed)

**Anti-skip rule:** Never condense, abbreviate, or skip any review pass (1-7) regardless of plan type. Every pass in this skill exists for a reason. If a pass genuinely has zero findings, say "No issues found" and move on — but you must evaluate it.

**Anti-shortcut clause:** If you have ANY non-trivial finding in any review section, the path from finding to proceeding goes THROUGH AskUserQuestion. Zero findings in every section is the only path that bypasses AskUserQuestion.

### Pass 1: Information Architecture
Rate 0-10: Does the plan define what the user sees first, second, third?
FIX TO 10: Add information hierarchy to the plan. Include ASCII diagram of screen/page structure and navigation flow. Apply "constraint worship" — if you can only show 3 things, which 3?
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. If no issues, say so and move on. Do NOT proceed until user responds.

### Pass 2: Interaction State Coverage
Rate 0-10: Does the plan specify loading, empty, error, success, partial states?
FIX TO 10: Add interaction state table to the plan:
```
  FEATURE              | LOADING | EMPTY | ERROR | SUCCESS | PARTIAL
  ---------------------|---------|-------|-------|---------|--------
  [each UI feature]    | [spec]  | [spec]| [spec]| [spec]  | [spec]
```
For each state: describe what the user SEES, not backend behavior.
Empty states are features — specify warmth, primary action, context.
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY.

### Pass 3: User Journey & Emotional Arc
Rate 0-10: Does the plan consider the user's emotional experience?
FIX TO 10: Add user journey storyboard:
```
  STEP | USER DOES        | USER FEELS      | PLAN SPECIFIES?
  -----|------------------|-----------------|----------------
  1    | Lands on page    | [what emotion?] | [what supports it?]
  ...
```
Apply time-horizon design: 5-sec visceral, 5-min behavioral, 5-year reflective.
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY.

### Pass 4: AI Slop Risk
Rate 0-10: Does the plan describe specific, intentional UI — or generic patterns?
FIX TO 10: Rewrite vague UI descriptions with specific alternatives.

Read `references/ai-slop-rules.md` for the full classifier, hard rejection criteria, litmus checks, landing page rules, app UI rules, universal rules, and the AI slop blacklist.

If visual mockups were generated in Step 0.5, evaluate them against the AI slop blacklist. Does the mockup fall into generic patterns (3-column grid, centered hero, stock-photo feel)? If so, flag it and offer to regenerate.
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY.

### Pass 5: Design System Alignment
Rate 0-10: Does the plan align with DESIGN.md?
FIX TO 10: If DESIGN.md exists, annotate with specific tokens/components. If no DESIGN.md, flag the gap and recommend creating one.
Flag any new component — does it fit the existing vocabulary?
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY.

### Pass 6: Responsive & Accessibility
Rate 0-10: Does the plan specify mobile/tablet, keyboard nav, screen readers?
FIX TO 10: Add responsive specs per viewport — not "stacked on mobile" but intentional layout changes. Add a11y: keyboard nav patterns, ARIA landmarks, touch target sizes (44px min), color contrast requirements.
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY.

### Pass 7: Unresolved Design Decisions
Surface ambiguities that will haunt implementation:
```
  DECISION NEEDED              | IF DEFERRED, WHAT HAPPENS
  -----------------------------|---------------------------
  What does empty state look like? | Engineer ships "No items found."
  Mobile nav pattern?          | Desktop nav hides behind hamburger
  ...
```
If visual mockups were generated in Step 0.5, reference them as evidence when surfacing unresolved decisions. A mockup makes decisions concrete — e.g., "Your approved mockup shows a sidebar nav, but the plan doesn't specify mobile behavior. What happens to this sidebar on 375px?"
Each decision = one AskUserQuestion with recommendation + WHY + alternatives. Edit the plan with each decision as it's made.

### Post-Pass: Update Mockups (if generated)

If mockups were generated in Step 0.5 and review passes changed significant design decisions (information architecture restructure, new states, layout changes), offer to regenerate:

AskUserQuestion: "The review passes changed [list major design changes]. Want me to regenerate mockups to reflect the updated plan? This ensures the visual reference matches what we're actually building."

If yes, use the design binary to regenerate with feedback summarizing the changes. Save to the same design directory.

## CRITICAL RULE — How to ask questions

Follow the AskUserQuestion format from the references. Additional rules for plan design reviews:
* **One issue = one AskUserQuestion call.** Never combine multiple issues into one question.
* Describe the design gap concretely — what's missing, what the user will experience if it's not specified.
* Present 2-3 options. For each: effort to specify now, risk if deferred.
* **Map to Design Principles above.** One sentence connecting your recommendation to a specific principle.
* Label with issue NUMBER + option LETTER (e.g., "3A", "3B").
* **Zero findings:** if a section has zero findings, state "No issues, moving on" and proceed. Otherwise, use AskUserQuestion for each gap — a gap with an "obvious fix" is still a gap and still needs user approval before any change lands in the plan.

## Required Outputs

### "NOT in scope" section
Design decisions considered and explicitly deferred, with one-line rationale each.

### "What already exists" section
Existing DESIGN.md, UI patterns, and components that the plan should reuse.

### TODOS.md updates
After all review passes are complete, present each potential TODO as its own individual AskUserQuestion. Never batch TODOs — one per question. Never silently skip this step.

For design debt: missing a11y, unresolved responsive behavior, deferred empty states. Each TODO gets:
* **What:** One-line description of the work.
* **Why:** The concrete problem it solves or value it unlocks.
* **Pros:** What you gain by doing this work.
* **Cons:** Cost, complexity, or risks of doing it.
* **Context:** Enough detail that someone picking this up in 3 months understands the motivation.
* **Depends on / blocked by:** Any prerequisites.

Then present options: **A)** Add to TODOS.md **B)** Skip — not valuable enough **C)** Build it now in this PR instead of deferring.

### Completion Summary
```
  +====================================================================+
  |         DESIGN PLAN REVIEW — COMPLETION SUMMARY                    |
  +====================================================================+
  | System Audit         | [DESIGN.md status, UI scope]                |
  | Step 0               | [initial rating, focus areas]               |
  | Pass 1  (Info Arch)  | ___/10 → ___/10 after fixes                |
  | Pass 2  (States)     | ___/10 → ___/10 after fixes                |
  | Pass 3  (Journey)    | ___/10 → ___/10 after fixes                |
  | Pass 4  (AI Slop)    | ___/10 → ___/10 after fixes                |
  | Pass 5  (Design Sys) | ___/10 → ___/10 after fixes                |
  | Pass 6  (Responsive) | ___/10 → ___/10 after fixes                |
  | Pass 7  (Decisions)  | ___ resolved, ___ deferred                 |
  +--------------------------------------------------------------------+
  | NOT in scope         | written (___ items)                         |
  | What already exists  | written                                     |
  | TODOS.md updates     | ___ items proposed                          |
  | Approved Mockups     | ___ generated, ___ approved                  |
  | Decisions made       | ___ added to plan                           |
  | Decisions deferred   | ___ (listed below)                          |
  | Overall design score | ___/10 → ___/10                             |
  +====================================================================+
```

If all passes 8+: "Plan is design-complete. Run a visual review after implementation."
If any below 8: note what's unresolved and why (user chose to defer).

### Unresolved Decisions
If any AskUserQuestion goes unanswered, note it here. Never silently default to an option.

### Approved Mockups

If visual mockups were generated during this review, add to the plan file:

```
## Approved Mockups

| Screen/Section | Mockup Path | Direction | Notes |
|----------------|-------------|-----------|-------|
| [screen name]  | docs/reviews/[folder]/[filename].png | [brief description] | [constraints from review] |
```

Include the path to each approved mockup, a one-line description of the direction, and any constraints. The implementer reads this to know exactly which visual to build from.

### Persist the Review

After producing the Completion Summary, write the review output to `docs/reviews/vc-plan-design-review-YYYY-MM-DD.md` with all sections above.

## Formatting Rules
* NUMBER issues (1, 2, 3...) and LETTERS for options (A, B, C...).
* Label with NUMBER + LETTER (e.g., "3A", "3B").
* One sentence max per option.
* After each pass, pause and wait for feedback.
* Rate before and after each pass for scannability.
