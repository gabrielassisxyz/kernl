# DevEx Review — Outside Voice Protocol

> Reference for `vc-plan-devex-review`. Used after all review sections are complete.

## When to Use

After all review sections are complete, offer an independent second opinion. Two models agreeing on a plan is stronger signal than one model's thorough review.

## AskUserQuestion

> "All review sections are complete. Want an outside design voices before the detailed review? A Claude subagent does an independent completeness review."
>
> A) Yes — run outside design voice
> B) No — proceed without

If user chooses B, skip this step and continue.

## Claude DX Subagent

Dispatch a subagent with this prompt:
"Read the plan file at [plan-file-path]. You are an independent DX engineer reviewing this plan. You have NOT seen any prior review. Evaluate:

1. Getting started: how many steps from zero to hello world? What's the TTHW?
2. API/CLI ergonomics: naming consistency, sensible defaults, progressive disclosure?
3. Error handling: does every error path specify problem + cause + fix + docs link?
4. Documentation: copy-paste examples? Information architecture? Interactive elements?
5. Escape hatches: can developers override every opinionated default?

For each finding: what's wrong, severity (critical/high/medium), and the fix."

Present subagent output under a `SUBAgent (DX completeness):` header.

**Error handling:** If subagent fails: "Outside voice unavailable — continuing with primary review."

## Outside Voice Integration

Apply findings from the outside voice:
- Hard rejection patterns → raised as the FIRST items in Pass 1, tagged `[HARD REJECTION]`
- Critical findings → pre-loaded as known issues in the relevant pass
- Passes can skip discovery and go straight to fixing for pre-identified issues

## User Sovereignty

Do NOT auto-incorporate outside voice recommendations into the plan. Present each finding via AskUserQuestion and get explicit approval.
