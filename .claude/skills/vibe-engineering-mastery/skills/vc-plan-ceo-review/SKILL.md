---
name: vc-plan-ceo-review
description: 'Run a CEO-level strategic review on a plan. Challenges scope, premises, and ambition. Use when user says CEO review, challenge the scope, is this the right thing to build, run strategic review, or check the business case. Also triggers on review my plan from a product perspective or what is missing from the strategy. Use for "strategic review" or "scope challenge" sessions.'
user-invocable: true
---

# VC Plan CEO Review

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

# Mega Plan Review Mode

## Philosophy
You are not here to rubber-stamp this plan. You are here to make it extraordinary, catch every landmine before it explodes, and ensure that when this ships, it ships at the highest possible standard.
But your posture depends on what the user needs:
* SCOPE EXPANSION: You are building a cathedral. Envision the platonic ideal. Push scope UP. Ask "what would make this 10x better for 2x the effort?" You have permission to dream — and to recommend enthusiastically. But every expansion is the user's decision. Present each scope-expanding idea as an AskUserQuestion. The user opts in or out.
* SELECTIVE EXPANSION: You are a rigorous reviewer who also has taste. Hold the current scope as your baseline — make it bulletproof. But separately, surface every expansion opportunity you see and present each one individually as an AskUserQuestion so the user can cherry-pick. Neutral recommendation posture — present the opportunity, state effort and risk, let the user decide. Accepted expansions become part of the plan's scope for the remaining sections. Rejected ones go to "NOT in scope."
* HOLD SCOPE: You are a rigorous reviewer. The plan's scope is accepted. Your job is to make it bulletproof — catch every failure mode, test every edge case, ensure observability, map every error path. Do not silently reduce OR expand.
* SCOPE REDUCTION: You are a surgeon. Find the minimum viable version that achieves the core outcome. Cut everything else. Be ruthless.
* COMPLETENESS IS CHEAP: AI coding compresses implementation time 10-100x. When evaluating "approach A (full, ~150 LOC) vs approach B (90%, ~80 LOC)" — always prefer A. The 70-line delta costs seconds with CC. "Ship the shortcut" is legacy thinking from when human engineering time was the bottleneck. Boil the lake.
Critical rule: In ALL modes, the user is 100% in control. Every scope change is an explicit opt-in via AskUserQuestion — never silently add or remove scope. Once the user selects a mode, COMMIT to it. Do not silently drift toward a different mode. If EXPANSION is selected, do not argue for less work during later sections. If SELECTIVE EXPANSION is selected, surface expansions as individual decisions — do not silently include or exclude them. If REDUCTION is selected, do not sneak scope back in. Raise concerns once in Step 0 — after that, execute the chosen mode faithfully.
Do NOT make any code changes. Do NOT start implementation. Your only job right now is to review the plan with maximum rigor and the appropriate level of ambition.

Read `references/prime-directives.md` for the 9 non-negotiable rules that govern this review.

## Engineering Preferences

Read `references/engineering-preferences.md` for the preferences that guide every recommendation.

## Cognitive Patterns

Read `references/cognitive-patterns.md` and internalize the CEO thinking instincts. Let them shape your perspective throughout the review. Don't enumerate them; internalize them.

## Priority Hierarchy Under Context Pressure
Step 0 > System audit > Error/rescue map > Test diagram > Failure modes > Opinionated recommendations > Everything else.
Never skip Step 0, the system audit, the error/rescue map, or the failure modes section. These are the highest-leverage outputs.

## PRE-REVIEW SYSTEM AUDIT (before Step 0)
Before doing anything else, run a system audit. This is not the plan review — it is the context you need to review the plan intelligently.
Run the following commands:
```
git log --oneline -30                          # Recent history
git diff <base> --stat                           # What's already changed
git stash list                                 # Any stashed work
grep -r "TODO\|FIXME\|HACK\|XXX" -l --exclude-dir=node_modules --exclude-dir=vendor --exclude-dir=.git . | head -30
git log --since=30.days --name-only --format="" | sort | uniq -c | sort -rn | head -20  # Recently touched files
```
Then read CLAUDE.md, TODOS.md, and any existing architecture docs.

**Design doc check:**
```bash
DESIGN=$(ls -t docs/designs/*.md 2>/dev/null | head -1)
[ -z "$DESIGN" ] && DESIGN=$(ls -t docs/*-design-*.md 2>/dev/null | head -1)
[ -n "$DESIGN" ] && echo "Design doc found: $DESIGN" || echo "No design doc found"
```
If a design doc exists, read it. Use it as the source of truth for the problem statement, constraints, and chosen approach.

**Handoff note check:**
```bash
HANDOFF=$(ls -t docs/reviews/*-ceo-handoff-*.md 2>/dev/null | head -1)
[ -n "$HANDOFF" ] && echo "HANDOFF_FOUND: $HANDOFF" || echo "NO_HANDOFF"
```
If a handoff note is found: read it. This contains system audit findings and discussion
from a prior CEO review session. Use it as additional context alongside the design doc.
The handoff note helps you avoid re-asking questions the user already answered.
Do NOT skip any steps — run the full review, but use the handoff note to inform your
analysis and avoid redundant questions.

Tell the user: "Found a handoff note from your prior CEO review session. I'll use that
context to pick up where we left off."

## Prerequisite Skill Offer

When the design doc check above prints "No design doc found," offer the prerequisite
skill before proceeding.

Say to the user via AskUserQuestion:

> "No design doc found for this branch. A strategy session produces a structured problem
> statement, premise challenge, and explored alternatives — it gives this review much
> sharper input to work with. Takes about 10 minutes."

Options:
- A) Run /vc-strategy now (we'll pick up the review right after)
- B) Skip — proceed with standard review

If they choose A:

Say: "Running /vc-strategy inline. Once the strategy is ready, I'll pick up
the review right where we left off."

Read the `/vc-strategy` skill file and follow its instructions from top to bottom,
skipping sections already handled (Voice, Confusion Protocol, Completion Status Protocol).
Execute every other section at full depth. When the loaded skill's instructions are
complete, continue with the next step below.

After /vc-strategy completes, re-run the design doc check above.

If a design doc is now found, read it and continue the review.
If none was produced (user may have cancelled), proceed with standard review.

If they skip: "No worries — standard review. If you ever want sharper input, try
/vc-strategy first next time." Then proceed normally. Do not re-offer later in the session.

**Mid-session detection:** During Step 0A (Premise Challenge), if the user can't
articulate the problem, keeps changing the problem statement, answers with "I'm not
sure," or is clearly exploring rather than reviewing — offer /vc-strategy:

> "It sounds like you're still figuring out what to build — that's totally fine, but
> that's what a strategy session is designed for. Want to run /vc-strategy right now?
> We'll pick up right where we left off."

Options: A) Yes, run /vc-strategy now. B) No, keep going.
If they keep going, proceed normally — no guilt, no re-asking.

If they choose A: Read the /vc-strategy skill file, follow its instructions (skip already-handled sections), then re-run the design doc check and resume the review.

When reading TODOS.md, specifically:
* Note any TODOs this plan touches, blocks, or unlocks
* Check if deferred work from prior reviews relates to this plan
* Flag dependencies: does this plan enable or depend on deferred items?
* Map known pain points (from TODOS) to this plan's scope

Map:
* What is the current system state?
* What is already in flight (other open PRs, branches, stashed changes)?
* What are the existing known pain points most relevant to this plan?
* Are there any FIXME/TODO comments in files this plan touches?

### Retrospective Check
Check the git log for this branch. If there are prior commits suggesting a previous review cycle (review-driven refactors, reverted changes), note what was changed and whether the current plan re-touches those areas. Be MORE aggressive reviewing areas that were previously problematic. Recurring problem areas are architectural smells — surface them as architectural concerns.

### Frontend/UI Scope Detection
Analyze the plan. If it involves ANY of: new UI screens/pages, changes to existing UI components, user-facing interaction flows, frontend framework changes, user-visible state changes, mobile/responsive behavior, or design system changes — note DESIGN_SCOPE for Section 11.

### Taste Calibration (EXPANSION and SELECTIVE EXPANSION modes)
Identify 2-3 files or patterns in the existing codebase that are particularly well-designed. Note them as style references for the review. Also note 1-2 patterns that are frustrating or poorly designed — these are anti-patterns to avoid repeating.
Report findings before proceeding to Step 0.

### Landscape Check

Read ETHOS.md for the Search Before Building framework if it exists. Before challenging scope, understand the landscape. WebSearch for:
- "[product category] landscape {current year}"
- "[key feature] alternatives"
- "why [incumbent/conventional approach] [succeeds/fails]"

If WebSearch is unavailable, skip this check and note: "Search unavailable — proceeding with in-distribution knowledge only."

Run the three-layer synthesis:
- **[Layer 1]** What's the tried-and-true approach in this space?
- **[Layer 2]** What are the search results saying?
- **[Layer 3]** First-principles reasoning — where might the conventional wisdom be wrong?

Feed into the Premise Challenge (0A) and Dream State Mapping (0C). If you find a eureka moment, surface it during the Expansion opt-in ceremony as a differentiation opportunity.

---

## Step 0: Nuclear Scope Challenge + Mode Selection

### 0A. Premise Challenge
1. Is this the right problem to solve? Could a different framing yield a dramatically simpler or more impactful solution?
2. What is the actual user/business outcome? Is the plan the most direct path to that outcome, or is it solving a proxy problem?
3. What would happen if we did nothing? Real pain point or hypothetical one?

### 0B. Existing Code Leverage
1. What existing code already partially or fully solves each sub-problem? Map every sub-problem to existing code. Can we capture outputs from existing flows rather than building parallel ones?
2. Is this plan rebuilding anything that already exists? If yes, explain why rebuilding is better than refactoring.

### 0C. Dream State Mapping
Describe the ideal end state of this system 12 months from now. Does this plan move toward that state or away from it?
```
  CURRENT STATE                  THIS PLAN                  12-MONTH IDEAL
  [describe]          --->       [describe delta]    --->    [describe target]
```

### 0C-bis. Implementation Alternatives (MANDATORY)

Before selecting a mode (0F), produce 2-3 distinct implementation approaches. This is NOT optional — every plan must consider alternatives.

For each approach:
```
APPROACH A: [Name]
  Summary: [1-2 sentences]
  Effort:  [S/M/L/XL]
  Risk:    [Low/Med/High]
  Pros:    [2-3 bullets]
  Cons:    [2-3 bullets]
  Reuses:  [existing code/patterns leveraged]

APPROACH B: [Name]
  ...

APPROACH C: [Name] (optional — include if a meaningfully different path exists)
  ...
```

**RECOMMENDATION:** Choose [X] because [one-line reason mapped to engineering preferences].

Rules:
- At least 2 approaches required. 3 preferred for non-trivial plans.
- One approach must be the "minimal viable" (fewest files, smallest diff).
- One approach must be the "ideal architecture" (best long-term trajectory).
- **These two approaches have equal weight.** Don't default to "minimal viable" just because it's smaller. Recommend whichever best serves the user's goal. If the right answer is a rewrite, say so.
- If only one approach exists, explain concretely why alternatives were eliminated.
- Do NOT proceed to mode selection (0F) without user approval of the chosen approach.

Present these approach options via AskUserQuestion using the `references/askuser-format.md` format: include RECOMMENDATION and `Completeness: N/10` on every option. These approaches differ in coverage (minimal viable vs ideal architecture), so completeness scoring applies directly.

**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. Do NOT proceed to Step 0D or 0F until the user responds to 0C-bis. A "clearly winning approach" is still an approach decision and still needs explicit user approval before it lands in the plan.
**Reminder: Do NOT make any code changes. Review only.**

### 0D-prelude. Expansion Framing (shared by EXPANSION and SELECTIVE EXPANSION)

Every expansion proposal you generate in SCOPE EXPANSION or SELECTIVE EXPANSION mode follows this framing pattern:

FLAT (avoid): "Add real-time notifications. Users would see workflow results faster — latency drops from ~30s polling to <500ms push. Effort: ~1 hour CC."

EXPANSIVE (aim for): "Imagine the moment a workflow finishes — the user sees the result instantly, no tab-switching, no polling, no 'did it actually work?' anxiety. Real-time feedback turns a tool they check into a tool that talks to them. Concrete shape: WebSocket channel + optimistic UI + desktop notification fallback. Effort: human ~2 days / CC ~1 hour. Makes the product feel 10x more alive."

Both are outcome-framed. Only one makes the user feel the cathedral. Lead with the felt experience, close with concrete effort and impact.

**For SELECTIVE EXPANSION:** neutral recommendation posture ≠ flat prose. Present vivid options, then let the user decide. Do not over-sell — "Makes the product feel 10x more alive" is vivid; "This would 10x your revenue" is over-sell. Evocative, not promotional.

### 0D. Mode-Specific Analysis
**For SCOPE EXPANSION** — run all three, then the opt-in ceremony:
1. 10x check: What's the version that's 10x more ambitious and delivers 10x more value for 2x the effort? Describe it concretely.
2. Platonic ideal: If the best engineer in the world had unlimited time and perfect taste, what would this system look like? What would the user feel when using it? Start from experience, not architecture.
3. Delight opportunities: What adjacent 30-minute improvements would make this feature sing? Things where a user would think "oh nice, they thought of that." List at least 5.
4. **Expansion opt-in ceremony:** Describe the vision first (10x check, platonic ideal). Then distill concrete scope proposals from those visions — individual features, components, or improvements. Present each proposal as its own AskUserQuestion. Recommend enthusiastically — explain why it's worth doing. But the user decides. Options: **A)** Add to this plan's scope **B)** Defer to TODOS.md **C)** Skip. Accepted items become plan scope for all remaining review sections. Rejected items go to "NOT in scope."

**For SELECTIVE EXPANSION** — run the HOLD SCOPE analysis first, then surface expansions:
1. Complexity check: If the plan touches more than 8 files or introduces more than 2 new classes/services, treat that as a smell and challenge whether the same goal can be achieved with fewer moving parts.
2. What is the minimum set of changes that achieves the stated goal? Flag any work that could be deferred without blocking the core objective.
3. Then run the expansion scan (do NOT add these to scope yet — they are candidates):
   - 10x check: What's the version that's 10x more ambitious? Describe it concretely.
   - Delight opportunities: What adjacent 30-minute improvements would make this feature sing? List at least 5.
   - Platform potential: Would any expansion turn this feature into infrastructure other features can build on?
4. **Cherry-pick ceremony:** Present each expansion opportunity as its own individual AskUserQuestion. Neutral recommendation posture — present the opportunity, state effort (S/M/L) and risk, let the user decide without bias. Options: **A)** Add to this plan's scope **B)** Defer to TODOS.md **C)** Skip. If you have more than 8 candidates, present the top 5-6 and note the remainder as lower-priority options the user can request. Accepted items become plan scope for all remaining review sections. Rejected items go to "NOT in scope."

**For HOLD SCOPE** — run this:
1. Complexity check: If the plan touches more than 8 files or introduces more than 2 new classes/services, treat that as a smell and challenge whether the same goal can be achieved with fewer moving parts.
2. What is the minimum set of changes that achieves the stated goal? Flag any work that could be deferred without blocking the core objective.

**For SCOPE REDUCTION** — run this:
1. Ruthless cut: What is the absolute minimum that ships value to a user? Everything else is deferred. No exceptions.
2. What can be a follow-up PR? Separate "must ship together" from "nice to ship together."

### 0D-POST. Persist CEO Plan (EXPANSION and SELECTIVE EXPANSION only)

After the opt-in/cherry-pick ceremony, write the plan to disk so the vision and decisions survive beyond this conversation. Only run this step for EXPANSION and SELECTIVE EXPANSION modes.

```bash
mkdir -p docs/reviews
```

Before writing, check for existing CEO plans in docs/reviews/. If any are >30 days old or their branch has been merged/deleted, offer to archive them.

Write to `docs/reviews/vc-plan-ceo-review-YYYY-MM-DD.md` using this format:

```markdown
---
status: ACTIVE
---
# CEO Plan: {Feature Name}
Generated by /vc-plan-ceo-review on {date}
Branch: {branch} | Mode: {EXPANSION / SELECTIVE EXPANSION}
Repo: {owner/repo}

## Vision

### 10x Check
{10x vision description}

### Platonic Ideal
{platonic ideal description — EXPANSION mode only}

## Scope Decisions

| # | Proposal | Effort | Decision | Reasoning |
|---|----------|--------|----------|-----------|
| 1 | {proposal} | S/M/L | ACCEPTED / DEFERRED / SKIPPED | {why} |

## Accepted Scope (added to this plan)
- {bullet list of what's now in scope}

## Deferred to TODOS.md
- {items with context}
```

Derive the feature slug from the plan being reviewed (e.g., "user-dashboard", "auth-refactor"). Use the date in YYYY-MM-DD format.

After writing the CEO plan, run the spec review loop on it:

## Spec Review Loop

Before presenting the document to the user for approval, run an adversarial review.

**Step 1: Dispatch reviewer subagent**

Use the Agent tool to dispatch an independent reviewer. The reviewer has fresh context
and cannot see the brainstorming conversation — only the document. This ensures genuine
adversarial independence.

Prompt the subagent with:
- The file path of the document just written
- "Read this document and review it on 5 dimensions. For each dimension, note PASS or
  list specific issues with suggested fixes. At the end, output a quality score (1-10)
  across all dimensions."

**Dimensions:**
1. **Completeness** — Are all requirements addressed? Missing edge cases?
2. **Consistency** — Do parts of the document agree with each other? Contradictions?
3. **Clarity** — Could an engineer implement this without asking questions? Ambiguous language?
4. **Scope** — Does the document creep beyond the original problem? YAGNI violations?
5. **Feasibility** — Can this actually be built with the stated approach? Hidden complexity?

The subagent should return:
- A quality score (1-10)
- PASS if no issues, or a numbered list of issues with dimension, description, and fix

**Step 2: Fix and re-dispatch**

If the reviewer returns issues:
1. Fix each issue in the document on disk (use Edit tool)
2. Re-dispatch the reviewer subagent with the updated document
3. Maximum 3 iterations total

**Convergence guard:** If the reviewer returns the same issues on consecutive iterations
(the fix didn't resolve them or the reviewer disagrees with the fix), stop the loop
and persist those issues as "Reviewer Concerns" in the document rather than looping
further.

If the subagent fails, times out, or is unavailable — skip the review loop entirely.
Tell the user: "Spec review unavailable — presenting unreviewed doc." The document is
already written to disk; the review is a quality bonus, not a gate.

**Step 3: Report**

After the loop completes (PASS, max iterations, or convergence guard):

1. Tell the user the result — summary by default:
   "Your doc survived N rounds of adversarial review. M issues caught and fixed.
   Quality score: X/10."
   If they ask "what did the reviewer find?", show the full reviewer output.

2. If issues remain after max iterations or convergence, add a "## Reviewer Concerns"
   section to the document listing each unresolved issue. Downstream skills will see this.

### 0E. Temporal Interrogation (EXPANSION, SELECTIVE EXPANSION, and HOLD modes)
Think ahead to implementation: What decisions will need to be made during implementation that should be resolved NOW in the plan?
```
  HOUR 1 (foundations):     What does the implementer need to know?
  HOUR 2-3 (core logic):   What ambiguities will they hit?
  HOUR 4-5 (integration):  What will surprise them?
  HOUR 6+ (polish/tests):  What will they wish they'd planned for?
```
NOTE: These represent human-team implementation hours. With AI coding,
6 hours of human implementation compresses to ~30-60 minutes. The decisions
are identical — the implementation speed is 10-20x faster. Always present
both scales when discussing effort.

Surface these as questions for the user NOW, not as "figure it out later."

### 0F. Mode Selection
In every mode, you are 100% in control. No scope is added without your explicit approval.

Present four options:
1. **SCOPE EXPANSION:** The plan is good but could be great. Dream big — propose the ambitious version. Every expansion is presented individually for your approval. You opt in to each one.
2. **SELECTIVE EXPANSION:** The plan's scope is the baseline, but you want to see what else is possible. Every expansion opportunity presented individually — you cherry-pick the ones worth doing. Neutral recommendations.
3. **HOLD SCOPE:** The plan's scope is right. Review it with maximum rigor — architecture, security, edge cases, observability, deployment. Make it bulletproof. No expansions surfaced.
4. **SCOPE REDUCTION:** The plan is overbuilt or wrong-headed. Propose a minimal version that achieves the core goal, then review that.

Context-dependent defaults:
* Greenfield feature → default EXPANSION
* Feature enhancement or iteration on existing system → default SELECTIVE EXPANSION
* Bug fix or hotfix → default HOLD SCOPE
* Refactor → default HOLD SCOPE
* Plan touching >15 files → suggest REDUCTION unless user pushes back
* User says "go big" / "ambitious" / "cathedral" → EXPANSION, no question
* User says "hold scope but tempt me" / "show me options" / "cherry-pick" → SELECTIVE EXPANSION, no question

After mode is selected, confirm which implementation approach (from 0C-bis) applies under the chosen mode. EXPANSION may favor the ideal architecture approach; REDUCTION may favor the minimal viable approach.

Once selected, commit fully. Do not silently drift.

Present these mode options via AskUserQuestion using the `references/askuser-format.md` format: include RECOMMENDATION. These options differ in kind (review posture), not coverage — do NOT emit `Completeness: N/10` per option. Include the one-line note: `Note: options differ in kind, not coverage — no completeness score.`

**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. If this section turned up zero findings, state "No issues, moving on" and proceed. If the section has findings, you MUST call AskUserQuestion as a tool_use — a finding with an "obvious fix" is still a finding and still needs user approval before any change lands in the plan. Do NOT proceed until the user responds.
**Reminder: Do NOT make any code changes. Review only.**

## Review Sections (11 sections, after scope and mode are agreed)

**Anti-skip rule:** Never condense, abbreviate, or skip any review section (1-11) regardless of plan type (strategy, spec, code, infra). Every section in this skill exists for a reason. "This is a strategy doc so implementation sections don't apply" is always wrong — implementation details are where strategy breaks down. If a section genuinely has zero findings, say "No issues found" and move on — but you must evaluate it.

**Anti-shortcut clause:** The plan file is the OUTPUT of the interactive review, not a substitute for it. Writing every finding into one plan write without firing AskUserQuestion defeats the review's purpose. If you have ANY non-trivial finding in any review section, the path from finding to completion goes THROUGH AskUserQuestion. Zero findings in every section is the only path that bypasses AskUserQuestion. If you find yourself wanting to write a plan with findings before asking, stop and call AskUserQuestion now.

### Section 1: Architecture Review
Evaluate and diagram:
* Overall system design and component boundaries. Draw the dependency graph.
* Data flow — all four paths. For every new data flow, ASCII diagram the:
    * Happy path (data flows correctly)
    * Nil path (input is nil/missing — what happens?)
    * Empty path (input is present but empty/zero-length — what happens?)
    * Error path (upstream call fails — what happens?)
* State machines. ASCII diagram for every new stateful object. Include impossible/invalid transitions and what prevents them.
* Coupling concerns. Which components are now coupled that weren't before? Is that coupling justified? Draw the before/after dependency graph.
* Scaling characteristics. What breaks first under 10x load? Under 100x?
* Single points of failure. Map them.
* Security architecture. Auth boundaries, data access patterns, API surfaces. For each new endpoint or data mutation: who can call it, what do they get, what can they change?
* Production failure scenarios. For each new integration point, describe one realistic production failure (timeout, cascade, data corruption, auth failure) and whether the plan accounts for it.
* Rollback posture. If this ships and immediately breaks, what's the rollback procedure? Git revert? Feature flag? DB migration rollback? How long?

**EXPANSION and SELECTIVE EXPANSION additions:**
* What would make this architecture beautiful? Not just correct — elegant. Is there a design that would make a new engineer joining in 6 months say "oh, that's clever and obvious at the same time"?
* What infrastructure would make this feature a platform that other features can build on?

**SELECTIVE EXPANSION:** If any accepted cherry-picks from Step 0D affect the architecture, evaluate their architectural fit here. Flag any that create coupling concerns or don't integrate cleanly — this is a chance to revisit the decision with new information.

Required ASCII diagram: full system architecture showing new components and their relationships to existing ones.
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. If this section turned up zero findings, state "No issues, moving on" and proceed. If the section has findings, you MUST call AskUserQuestion as a tool_use — a finding with an "obvious fix" is still a finding and still needs user approval before any change lands in the plan. Do NOT proceed until the user responds.
**Reminder: Do NOT make any code changes. Review only.**

### Section 2: Error & Rescue Map
This is the section that catches silent failures. It is not optional.
For every new method, service, or codepath that can fail, fill in this table:
```
  METHOD/CODEPATH          | WHAT CAN GO WRONG           | EXCEPTION CLASS
  -------------------------|-----------------------------|-----------------
  ExampleService#call      | API timeout                 | TimeoutError
                           | API returns 429             | RateLimitError
                           | API returns malformed JSON  | JSONParseError
                           | DB connection pool exhausted| ConnectionPoolExhausted
                           | Record not found            | RecordNotFound
  -------------------------|-----------------------------|-----------------

  EXCEPTION CLASS              | RESCUED?  | RESCUE ACTION          | USER SEES
  -----------------------------|-----------|------------------------|------------------
  TimeoutError                 | Y         | Retry 2x, then raise   | "Service temporarily unavailable"
  RateLimitError               | Y         | Backoff + retry         | Nothing (transparent)
  JSONParseError               | N ← GAP   | —                      | 500 error ← BAD
  ConnectionPoolExhausted      | N ← GAP   | —                      | 500 error ← BAD
  RecordNotFound               | Y         | Return nil, log warning | "Not found" message
```
Rules for this section:
* Catch-all error handling (`rescue StandardError`, `catch (Exception e)`, `except Exception`) is ALWAYS a smell. Name the specific exceptions.
* Catching an error with only a generic log message is insufficient. Log the full context: what was being attempted, with what arguments, for what user/request.
* Every rescued error must either: retry with backoff, degrade gracefully with a user-visible message, or re-raise with added context. "Swallow and continue" is almost never acceptable.
* For each GAP (unrescued error that should be rescued): specify the rescue action and what the user should see.
* For LLM/AI service calls specifically: what happens when the response is malformed? When it's empty? When it hallucinates invalid JSON? When the model returns a refusal? Each of these is a distinct failure mode.
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. If this section turned up zero findings, state "No issues, moving on" and proceed. If the section has findings, you MUST call AskUserQuestion as a tool_use — a finding with an "obvious fix" is still a finding and still needs user approval before any change lands in the plan. Do NOT proceed until the user responds.
**Reminder: Do NOT make any code changes. Review only.**

### Section 3: Security & Threat Model
Security is not a sub-bullet of architecture. It gets its own section.
Evaluate:
* Attack surface expansion. What new attack vectors does this plan introduce? New endpoints, new params, new file paths, new background jobs?
* Input validation. For every new user input: is it validated, sanitized, and rejected loudly on failure? What happens with: nil, empty string, string when integer expected, string exceeding max length, unicode edge cases, HTML/script injection attempts?
* Authorization. For every new data access: is it scoped to the right user/role? Is there a direct object reference vulnerability? Can user A access user B's data by manipulating IDs?
* Secrets and credentials. New secrets? In env vars, not hardcoded? Rotatable?
* Dependency risk. New gems/npm packages? Security track record?
* Data classification. PII, payment data, credentials? Handling consistent with existing patterns?
* Injection vectors. SQL, command, template, LLM prompt injection — check all.
* Audit logging. For sensitive operations: is there an audit trail?

For each finding: threat, likelihood (High/Med/Low), impact (High/Med/Low), and whether the plan mitigates it.
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. If this section turned up zero findings, state "No issues, moving on" and proceed. If the section has findings, you MUST call AskUserQuestion as a tool_use — a finding with an "obvious fix" is still a finding and still needs user approval before any change lands in the plan. Do NOT proceed until the user responds.
**Reminder: Do NOT make any code changes. Review only.**

### Section 4: Data Flow & Interaction Edge Cases
This section traces data through the system and interactions through the UI with adversarial thoroughness.

**Data Flow Tracing:** For every new data flow, produce an ASCII diagram showing:
```
  INPUT ──▶ VALIDATION ──▶ TRANSFORM ──▶ PERSIST ──▶ OUTPUT
    │            │              │            │           │
    ▼            ▼              ▼            ▼           ▼
  [nil?]    [invalid?]    [exception?]  [conflict?]  [stale?]
  [empty?]  [too long?]   [timeout?]    [dup key?]   [partial?]
  [wrong    [wrong type?] [OOM?]        [locked?]    [encoding?]
   type?]
```
For each node: what happens on each shadow path? Is it tested?

**Interaction Edge Cases:** For every new user-visible interaction, evaluate:
```
  INTERACTION          | EDGE CASE              | HANDLED? | HOW?
  ---------------------|------------------------|----------|--------
  Form submission      | Double-click submit    | ?        |
                       | Submit with stale CSRF | ?        |
                       | Submit during deploy   | ?        |
  Async operation      | User navigates away    | ?        |
                       | Operation times out    | ?        |
                       | Retry while in-flight  | ?        |
  List/table view      | Zero results           | ?        |
                       | 10,000 results         | ?        |
                       | Results change mid-page| ?        |
  Background job       | Job fails after 3 of   | ?        |
                       | 10 items processed     |          |
                       | Job runs twice (dup)   | ?        |
                       | Queue backs up 2 hours | ?        |
```
Flag any unhandled edge case as a gap. For each gap, specify the fix.
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. If this section turned up zero findings, state "No issues, moving on" and proceed. If the section has findings, you MUST call AskUserQuestion as a tool_use — a finding with an "obvious fix" is still a finding and still needs user approval before any change lands in the plan. Do NOT proceed until the user responds.
**Reminder: Do NOT make any code changes. Review only.**

### Section 5: Code Quality Review
Evaluate:
* Code organization and module structure. Does new code fit existing patterns? If it deviates, is there a reason?
* DRY violations. Be aggressive. If the same logic exists elsewhere, flag it and reference the file and line.
* Naming quality. Are new classes, methods, and variables named for what they do, not how they do it?
* Error handling patterns. (Cross-reference with Section 2 — this section reviews the patterns; Section 2 maps the specifics.)
* Missing edge cases. List explicitly: "What happens when X is nil?" "When the API returns 429?" etc.
* Over-engineering check. Any new abstraction solving a problem that doesn't exist yet?
* Under-engineering check. Anything fragile, assuming happy path only, or missing obvious defensive checks?
* Cyclomatic complexity. Flag any new method that branches more than 5 times. Propose a refactor.
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. If this section turned up zero findings, state "No issues, moving on" and proceed. If the section has findings, you MUST call AskUserQuestion as a tool_use — a finding with an "obvious fix" is still a finding and still needs user approval before any change lands in the plan. Do NOT proceed until the user responds.
**Reminder: Do NOT make any code changes. Review only.**

### Section 6: Test Review
Make a complete diagram of every new thing this plan introduces:
```
  NEW UX FLOWS:
    [list each new user-visible interaction]

  NEW DATA FLOWS:
    [list each new path data takes through the system]

  NEW CODEPATHS:
    [list each new branch, condition, or execution path]

  NEW BACKGROUND JOBS / ASYNC WORK:
    [list each]

  NEW INTEGRATIONS / EXTERNAL CALLS:
    [list each]

  NEW ERROR/RESCUE PATHS:
    [list each — cross-reference Section 2]
```
For each item in the diagram:
* What type of test covers it? (Unit / Integration / System / E2E)
* Does a test for it exist in the plan? If not, write the test spec header.
* What is the happy path test?
* What is the failure path test? (Be specific — which failure?)
* What is the edge case test? (nil, empty, boundary values, concurrent access)

Test ambition check (all modes): For each new feature, answer:
* What's the test that would make you confident shipping at 2am on a Friday?
* What's the test a hostile QA engineer would write to break this?
* What's the chaos test?

Test pyramid check: Many unit, fewer integration, few E2E? Or inverted?
Flakiness risk: Flag any test depending on time, randomness, external services, or ordering.
Load/stress test requirements: For any new codepath called frequently or processing significant data.

For LLM/prompt changes: Check CLAUDE.md for the "Prompt/LLM changes" file patterns. If this plan touches ANY of those patterns, state which eval suites must be run, which cases should be added, and what baselines to compare against.
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. If this section turned up zero findings, state "No issues, moving on" and proceed. If the section has findings, you MUST call AskUserQuestion as a tool_use — a finding with an "obvious fix" is still a finding and still needs user approval before any change lands in the plan. Do NOT proceed until the user responds.
**Reminder: Do NOT make any code changes. Review only.**

### Section 7: Performance Review
Evaluate:
* N+1 queries. For every new ActiveRecord association traversal: is there an includes/preload?
* Memory usage. For every new data structure: what's the maximum size in production?
* Database indexes. For every new query: is there an index?
* Caching opportunities. For every expensive computation or external call: should it be cached?
* Background job sizing. For every new job: worst-case payload, runtime, retry behavior?
* Slow paths. Top 3 slowest new codepaths and estimated p99 latency.
* Connection pool pressure. New DB connections, Redis connections, HTTP connections?
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. If this section turned up zero findings, state "No issues, moving on" and proceed. If the section has findings, you MUST call AskUserQuestion as a tool_use — a finding with an "obvious fix" is still a finding and still needs user approval before any change lands in the plan. Do NOT proceed until the user responds.
**Reminder: Do NOT make any code changes. Review only.**

### Section 8: Observability & Debuggability Review
New systems break. This section ensures you can see why.
Evaluate:
* Logging. For every new codepath: structured log lines at entry, exit, and each significant branch?
* Metrics. For every new feature: what metric tells you it's working? What tells you it's broken?
* Tracing. For new cross-service or cross-job flows: trace IDs propagated?
* Alerting. What new alerts should exist?
* Dashboards. What new dashboard panels do you want on day 1?
* Debuggability. If a bug is reported 3 weeks post-ship, can you reconstruct what happened from logs alone?
* Admin tooling. New operational tasks that need admin UI or rake tasks?
* Runbooks. For each new failure mode: what's the operational response?

**EXPANSION and SELECTIVE EXPANSION addition:**
* What observability would make this feature a joy to operate? (For SELECTIVE EXPANSION, include observability for any accepted cherry-picks.)
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. If this section turned up zero findings, state "No issues, moving on" and proceed. If the section has findings, you MUST call AskUserQuestion as a tool_use — a finding with an "obvious fix" is still a finding and still needs user approval before any change lands in the plan. Do NOT proceed until the user responds.
**Reminder: Do NOT make any code changes. Review only.**

### Section 9: Deployment & Rollout Review
Evaluate:
* Migration safety. For every new DB migration: backward-compatible? Zero-downtime? Table locks?
* Feature flags. Should any part be behind a feature flag?
* Rollout order. Correct sequence: migrate first, deploy second?
* Rollback plan. Explicit step-by-step.
* Deploy-time risk window. Old code and new code running simultaneously — what breaks?
* Environment parity. Tested in staging?
* Post-deploy verification checklist. First 5 minutes? First hour?
* Smoke tests. What automated checks should run immediately post-deploy?

**EXPANSION and SELECTIVE EXPANSION addition:**
* What deploy infrastructure would make shipping this feature routine? (For SELECTIVE EXPANSION, assess whether accepted cherry-picks change the deployment risk profile.)
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. If this section turned up zero findings, state "No issues, moving on" and proceed. If the section has findings, you MUST call AskUserQuestion as a tool_use — a finding with an "obvious fix" is still a finding and still needs user approval before any change lands in the plan. Do NOT proceed until the user responds.
**Reminder: Do NOT make any code changes. Review only.**

### Section 10: Long-Term Trajectory Review
Evaluate:
* Technical debt introduced. Code debt, operational debt, testing debt, documentation debt.
* Path dependency. Does this make future changes harder?
* Knowledge concentration. Documentation sufficient for a new engineer?
* Reversibility. Rate 1-5: 1 = one-way door, 5 = easily reversible.
* Ecosystem fit. Aligns with ecosystem direction?
* The 1-year question. Read this plan as a new engineer in 12 months — obvious?

**EXPANSION and SELECTIVE EXPANSION additions:**
* What comes after this ships? Phase 2? Phase 3? Does the architecture support that trajectory?
* Platform potential. Does this create capabilities other features can leverage?
* (SELECTIVE EXPANSION only) Retrospective: Were the right cherry-picks accepted? Did any rejected expansions turn out to be load-bearing for the accepted ones?
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. If this section turned up zero findings, state "No issues, moving on" and proceed. If the section has findings, you MUST call AskUserQuestion as a tool_use — a finding with an "obvious fix" is still a finding and still needs user approval before any change lands in the plan. Do NOT proceed until the user responds.
**Reminder: Do NOT make any code changes. Review only.**

### Section 11: Design & UX Review (skip if no UI scope detected)
The CEO calling in the designer. Not a pixel-level audit — that's for dedicated design review skills. This is ensuring the plan has design intentionality.

Evaluate:
* Information architecture — what does the user see first, second, third?
* Interaction state coverage map:
  FEATURE | LOADING | EMPTY | ERROR | SUCCESS | PARTIAL
* User journey coherence — storyboard the emotional arc
* AI slop risk — does the plan describe generic UI patterns?
* DESIGN.md alignment — does the plan match the stated design system?
* Responsive intention — is mobile mentioned or afterthought?
* Accessibility basics — keyboard nav, screen readers, contrast, touch targets

**EXPANSION and SELECTIVE EXPANSION additions:**
* What would make this UI feel *inevitable*?
* What 30-minute UI touches would make users think "oh nice, they thought of that"?

Required ASCII diagram: user flow showing screens/states and transitions.

If this plan has significant UI scope, recommend: "Consider running /vc-plan-design-review for a deep design review of this plan before implementation."
**STOP.** AskUserQuestion once per issue. Do NOT batch. Recommend + WHY. If this section turned up zero findings, state "No issues, moving on" and proceed. If the section has findings, you MUST call AskUserQuestion as a tool_use — a finding with an "obvious fix" is still a finding and still needs user approval before any change lands in the plan. Do NOT proceed until the user responds.
**Reminder: Do NOT make any code changes. Review only.**

## Outside Voice

Read `references/outside-voice.md` for the independent plan challenge protocol, cross-model tension handling, and user sovereignty rules.

## CRITICAL RULE — How to ask questions
Follow the AskUserQuestion format from `references/askuser-format.md`. Additional rules for plan reviews:
* **One issue = one AskUserQuestion call.** Never combine multiple issues into one question.
* Describe the problem concretely, with file and line references.
* Present 2-3 options, including "do nothing" where reasonable.
* For each option: effort, risk, and maintenance burden in one line.
* **Map the reasoning to my engineering preferences above.** One sentence connecting your recommendation to a specific preference.
* Label with issue NUMBER + option LETTER (e.g., "3A", "3B").
* **Zero findings:** if a section has zero findings, state "No issues, moving on" and proceed. Otherwise, use AskUserQuestion for each finding — a finding with an "obvious fix" is still a finding and still needs user approval before any change lands in the plan.

## Required Outputs

### "NOT in scope" section
List work considered and explicitly deferred, with one-line rationale each.

### "What already exists" section
List existing code/flows that partially solve sub-problems and whether the plan reuses them.

### "Dream state delta" section
Where this plan leaves us relative to the 12-month ideal.

### Error & Rescue Registry (from Section 2)
Complete table of every method that can fail, every exception class, rescued status, rescue action, user impact.

### Failure Modes Registry
```
  CODEPATH | FAILURE MODE   | RESCUED? | TEST? | USER SEES?     | LOGGED?
  ---------|----------------|----------|-------|----------------|--------
```
Any row with RESCUED=N, TEST=N, USER SEES=Silent → **CRITICAL GAP**.

### TODOS.md updates
Present each potential TODO as its own individual AskUserQuestion. Never batch TODOs — one per question. Never silently skip this step.

For each TODO, describe:
* **What:** One-line description of the work.
* **Why:** The concrete problem it solves or value it unlocks.
* **Pros:** What you gain by doing this work.
* **Cons:** Cost, complexity, or risks of doing it.
* **Context:** Enough detail that someone picking this up in 3 months understands the motivation, the current state, and where to start.
* **Effort estimate:** S/M/L/XL (human team) → with AI coding: S→S, M→S, L→M, XL→L
* **Priority:** P1/P2/P3
* **Depends on / blocked by:** Any prerequisites or ordering constraints.

Then present options: **A)** Add to TODOS.md **B)** Skip — not valuable enough **C)** Build it now in this PR instead of deferring.

### Scope Expansion Decisions (EXPANSION and SELECTIVE EXPANSION only)
For EXPANSION and SELECTIVE EXPANSION modes: expansion opportunities and delight items were surfaced and decided in Step 0D (opt-in/cherry-pick ceremony). The decisions are persisted in the CEO plan document. Reference the CEO plan for the full record. Do not re-surface them here — list the accepted expansions for completeness:
* Accepted: {list items added to scope}
* Deferred: {list items sent to TODOS.md}
* Skipped: {list items rejected}

### Diagrams (mandatory, produce all that apply)
1. System architecture
2. Data flow (including shadow paths)
3. State machine
4. Error flow
5. Deployment sequence
6. Rollback flowchart

### Stale Diagram Audit
List every ASCII diagram in files this plan touches. Still accurate?

### Completion Summary

Read `references/completion-summary-template.md` for the full completion summary template. Fill in all fields.

### Unresolved Decisions
If any AskUserQuestion goes unanswered, note it here. Never silently default.

## Handoff Note Cleanup

After producing the Completion Summary and writing the CEO plan to `docs/reviews/`, clean up any handoff notes for this branch — the review is complete and the context is no longer needed.

```bash
rm -f docs/reviews/*-ceo-handoff-*.md 2>/dev/null || true
```

## Next Steps

After completing the review, recommend the next step(s):

- **If engineering review is needed:** "Consider running /vc-plan-eng-review next for architecture, data flow, edge case, and test review."
- **If UI scope was detected:** "Consider running /vc-plan-design-review for a deep design review."
- **If both are needed, recommend eng review first**, then design review.

Use AskUserQuestion to present the next step:
- A) Run /vc-plan-eng-review next
- B) Run /vc-plan-design-review next (only if UI scope detected)
- C) Skip — I'll handle reviews manually

## docs/designs Promotion (EXPANSION and SELECTIVE EXPANSION only)

At the end of the review, if the vision produced a compelling feature direction, offer to promote the CEO plan to the project repo. AskUserQuestion:

"The vision from this review produced {N} accepted scope expansions. Want to promote it to a design doc in the repo?"
- **A)** Promote to `docs/designs/{FEATURE}.md` (committed to repo, visible to the team)
- **B)** Keep in `docs/reviews/` only (local, personal reference)
- **C)** Skip

If promoted, copy the CEO plan content to `docs/designs/{FEATURE}.md` (create the directory if needed) and update the `status` field in the original CEO plan from `ACTIVE` to `PROMOTED`.

## Formatting Rules
* NUMBER issues (1, 2, 3...) and LETTERS for options (A, B, C...).
* Label with NUMBER + LETTER (e.g., "3A", "3B").
* One sentence max per option.
* After each section, pause and wait for feedback.
* Use **CRITICAL GAP** / **WARNING** / **OK** for scannability.

## Mode Quick Reference

Read `references/mode-reference.md` for the mode comparison table.
