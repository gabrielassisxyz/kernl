# Engineering Review — Outside Voice Protocol

> Reference for `vc-plan-eng-review`. Used after all review sections are complete.

## Offer the Outside Voice

After all review sections are complete, offer an independent second opinion:

AskUserQuestion:
> "All review sections are complete. Want an outside voice? A different AI system can
> give a brutally honest, independent challenge of this plan — logical gaps, feasibility
> risks, and blind spots that are hard to catch from inside the review."
>
> RECOMMENDATION: Choose A — an independent second opinion catches structural blind
> spots. Two different AI models agreeing on a plan is stronger signal than one model's
> thorough review. Completeness: A=9/10, B=7/10.

Options:
- A) Get the outside voice (recommended)
- B) Skip — proceed to outputs

**If B:** Print "Skipping outside voice." and continue.

**If A:** Construct the plan review prompt. Read the plan file being reviewed. If a STRATEGY.md or design doc was written, read that too — it contains scope decisions and vision.

Prompt (substitute actual plan content — if >30KB, truncate and note "Plan truncated for size"):

"You are a brutally honest technical reviewer examining a development plan that has already been through a multi-section review. Your job is NOT to repeat that review. Instead, find what it missed. Look for: logical gaps and unstated assumptions that survived the review scrutiny, overcomplexity (is there a fundamentally simpler approach the review was too deep in the weeds to see?), feasibility risks the review took for granted, missing dependencies or sequencing issues, and strategic miscalibration (is this the right thing to build at all?). Be direct. Be terse. No compliments. Just the problems.

THE PLAN:
<plan content>"

Dispatch via the Agent tool. The subagent has fresh context — genuine independence.

Present findings under an `OUTSIDE VOICE (subagent):` header.
If the subagent fails or times out: "Outside voice unavailable. Continuing to outputs."

## Cross-Model Tension

After presenting the outside voice findings, note any points where the outside voice disagrees with the review findings. Flag these as:

```
CROSS-MODEL TENSION:
  [Topic]: Review said X. Outside voice says Y. [Present both perspectives neutrally.
  State what context you might be missing that would change the answer.]
```

## User Sovereignty

Do NOT auto-incorporate outside voice recommendations into the review output. Present each tension point to the user. Cross-model agreement is a strong signal — present it as such — but it is NOT permission to act.

For each substantive tension point, use AskUserQuestion:
> "Cross-model disagreement on [topic]. The review found [X] but the outside voice argues [Y]. [One sentence on what context we might be missing.]"
>
> RECOMMENDATION: Choose [A or B] because [one-line reason]. Completeness: A=X/10, B=Y/10.

Options:
- A) Accept the outside voice's recommendation (I'll apply this change)
- B) Keep the current approach (reject the outside voice)
- C) Investigate further before deciding
- D) Add to TODOS.md for later

Wait for the user's response. Do NOT default to accepting. If the user chooses B, the current approach stands — do not re-argue.

If no tension points exist, note: "No cross-model tension — both reviewers agree."

## Outside Voice Integration Rule

Outside voice findings are INFORMATIONAL until the user explicitly approves each one. Do NOT incorporate outside voice recommendations without presenting each finding via AskUserQuestion. Cross-model consensus is a strong signal — present it as such — but the user makes the decision.
