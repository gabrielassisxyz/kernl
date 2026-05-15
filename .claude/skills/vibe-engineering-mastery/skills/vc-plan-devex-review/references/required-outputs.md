# DevEx Review — Required Outputs

> Reference for `vc-plan-devex-review`. Produce all of these at the end of the review.

## Developer Persona Card
The persona card from Step 0A. This goes at the top of the plan's DX section.

## Developer Empathy Narrative
The first-person narrative from Step 0B, updated with user corrections.

## Competitive DX Benchmark
The benchmark table from Step 0C, updated with the product's post-review scores.

## Magical Moment Specification
The chosen delivery vehicle from Step 0D with implementation requirements.

## Developer Journey Map
The journey map from Step 0F, updated with all friction point resolutions.

## First-Time Developer Confusion Report
The roleplay report from Step 0G, annotated with which items were addressed.

## "NOT in scope" section
DX improvements considered and explicitly deferred, with one-line rationale each.

## "What already exists" section
Existing docs, examples, error handling, and DX patterns that the plan should reuse.

## TODOS.md updates
After all review passes are complete, present each potential TODO as its own individual AskUserQuestion. Never batch. For DX debt: missing error messages, unspecified upgrade paths, documentation gaps, missing SDK languages. Each TODO gets:
- **What:** One-line description
- **Why:** The concrete developer pain it causes
- **Pros:** What you gain (adoption, retention, satisfaction)
- **Cons:** Cost, complexity, or risks
- **Context:** Enough detail for someone to pick this up in 3 months
- **Depends on / blocked by:** Prerequisites

Options: **A)** Add to TODOS.md **B)** Skip **C)** Build it now

## DX Scorecard

Write to `docs/reviews/vc-plan-devex-review-YYYY-MM-DD.md`:

```
+====================================================================+
|              DX PLAN REVIEW — SCORECARD                             |
+====================================================================+
| Dimension            | Score  | Prior  | Trend  |
|----------------------|--------|--------|--------|
| Getting Started      | __/10  | __/10  | __ ↑↓  |
| API/CLI/SDK          | __/10  | __/10  | __ ↑↓  |
| Error Messages       | __/10  | __/10  | __ ↑↓  |
| Documentation        | __/10  | __/10  | __ ↑↓  |
| Upgrade Path         | __/10  | __/10  | __ ↑↓  |
| Dev Environment      | __/10  | __/10  | __ ↑↓  |
| Community            | __/10  | __/10  | __ ↑↓  |
| DX Measurement       | __/10  | __/10  | __ ↑↓  |
+--------------------------------------------------------------------+
| TTHW                 | __ min | __ min | __ ↑↓  |
| Competitive Rank     | [Champion/Competitive/Needs Work/Red Flag]   |
| Magical Moment       | [designed/missing] via [delivery vehicle]    |
| Product Type         | [type]                                      |
| Mode                 | [EXPANSION/POLISH/TRIAGE]                    |
| Overall DX           | __/10  | __/10  | __ ↑↓  |
+====================================================================+
| DX PRINCIPLE COVERAGE                                               |
| Zero Friction      | [covered/gap]                                  |
| Learn by Doing     | [covered/gap]                                  |
| Fight Uncertainty  | [covered/gap]                                  |
| Opinionated + Escape Hatches | [covered/gap]                       |
| Code in Context    | [covered/gap]                                  |
| Magical Moments    | [covered/gap]                                  |
+====================================================================+
```

If all passes 8+: "DX plan is solid. Developers will have a good experience."
If any below 6: Flag as critical DX debt with specific impact on adoption.
If TTHW > 10 min: Flag as blocking issue.

## DX Implementation Checklist

Include in the review output:

```
DX IMPLEMENTATION CHECKLIST
============================
[ ] Time to hello world < [target from 0C]
[ ] Installation is one command
[ ] First run produces meaningful output
[ ] Magical moment delivered via [vehicle from 0D]
[ ] Every error message has: problem + cause + fix + docs link
[ ] API/CLI naming is guessable without docs
[ ] Every parameter has a sensible default
[ ] Docs have copy-paste examples that actually work
[ ] Examples show real use cases, not just hello world
[ ] Upgrade path documented with migration guide
[ ] Breaking changes have deprecation warnings + codemods
[ ] TypeScript types included (if applicable)
[ ] Works in CI/CD without special configuration
[ ] Free tier available, no credit card required
[ ] Changelog exists and is maintained
[ ] Search works in documentation
[ ] Community channel exists and is monitored
```

## Unresolved Decisions
If any AskUserQuestion goes unanswered, note here. Never silently default.
