---
name: vc-strategy
description: 'Create or maintain docs/STRATEGY.md. Use when starting a new product, updating direction, or when prompts like write our strategy, update the roadmap, what are we working on, or set up the strategy doc come up. Also triggers when downstream planning needs upstream grounding and no strategy doc exists yet. Use for "strategy session" or "roadmap planning" work.'
user-invocable: true
---

# Product Strategy

**Note: The current year is 2026.** Use this when dating the strategy document.

`vc-strategy` produces and maintains `docs/STRATEGY.md` - a short, durable anchor document that captures what the product is, who it serves, how it succeeds, and where the team is investing. It lives in `docs/` as a canonical, well-known file. Downstream skills (`vc-plan-ceo-review`, `vc-plan-eng-review`, `vc-plan-design-review`, `vc-plan-devex-review`, `vc-writing-plans`) read it as grounding when it exists.

The document is short and structured on purpose. Good answers to a handful of sharp questions produce a better strategy than any amount of prose. This skill asks those questions, pushes back on weak answers, and writes the doc.

## Interaction Method

Default to the platform's blocking question tool: `AskUserQuestion` in Claude Code (call `ToolSearch` with `select:AskUserQuestion` first if its schema isn't loaded), `request_user_input` in Codex, `ask_user` in Gemini, `ask_user` in Pi. Fall back to numbered options in chat only when no blocking tool exists in the harness or the call errors — not because a schema load is required. Never silently skip the question.

Ask one question at a time. Prefer free-form responses for the substantive sections (problem, approach, persona); reserve single-select for routing decisions (which section to revisit). Each option label must be self-contained.

## Core Principles

1. **Anchor, not plan.** Strategy is what the product is and why. Features belong in downstream brainstorms; schedules belong in the issue tracker. Do not let either creep into the doc.
2. **Rigor in the questions, not the headings.** The section headers are plain English. The interview questions enforce strategy discipline.
3. **Short is a feature.** The template is constrained. Adding sections costs more than it looks like. Push back on expansion.
4. **Durable across runs.** This skill is rerunnable. On a second run it updates in place, preserves what is working, and only challenges sections that look stale or weak.

## Execution Flow

### Phase 0: Route by File State

Read `docs/STRATEGY.md` using the native file-read tool.

- **File does not exist** → First run. Go to Phase 1.
- **File exists and argument names a specific section** → Targeted update. Go to Phase 2.
- **File exists, no argument** → Ask which section(s) to revisit, then Phase 2.

Announce the path in one line: "Strategy doc not found - let's write it." or "Found existing strategy - let's review and update."

### Phase 1: First-Run Interview

Read `references/interview.md`. This load is non-optional — the pushback rules, anti-pattern examples, and quality bar for each section live there. Improvising from memory produces a passive transcription instead of a strategy doc.

Run the interview in the section order of the final document:

1. Target problem
2. Our approach
3. Who it's for
4. Key metrics
5. Tracks
6. Milestones (optional)
7. Not working on (optional)
8. Marketing (optional)

For each section, ask the opening question, apply the pushback rules, and capture the final answer in the user's own language. Do not skip the pushback step — it is the core of the skill. Two rounds of pushback per section maximum; capture what the user has given after that and note the section is worth revisiting on the next run.

When all required sections (1-5) are captured, read `references/strategy-template.md`, fill it in, and present the full draft in chat before writing. Offer one round of edits. Then write to `docs/STRATEGY.md`.

### Phase 2: Update Run

Read the existing `docs/STRATEGY.md` thoroughly. Summarize current state in 3-5 lines so the user sees what is on file.

If the argument named a specific section, jump to that section in `references/interview.md`. Preserve all other sections exactly. Apply pushback as if this were a first run — do not rubber-stamp existing weak content just because it is already written.

If no specific target, ask the user which section to revisit using the blocking question tool. Options:

- "Target problem"
- "Our approach"
- "Who it's for"
- "Metrics, tracks, or other"

For each revisited section, re-interview with full pushback. For sections the user confirms are still accurate, leave them untouched. Update the `last_updated` value in the YAML frontmatter to today's ISO date.

Write the updated doc back to `docs/STRATEGY.md`.

### Phase 3: Downstream Handoff

After writing, note in one line where the file lives (`docs/STRATEGY.md`) and that downstream skills will pick it up as grounding on their next run.

If no downstream skill has run yet on this repo, suggest `/vc-orchestrator` or `/vc-autoplan` as a next step.

## What This Skill Does Not Do

- Does not update the issue tracker or reconcile in-flight work. Strategy is the doc; execution lives elsewhere.
- Does not prioritize the backlog. Prioritization is a separate workflow.
- Does not write product requirements or implementation plans — those are downstream skills.
- Does not compute metric values. It records which metrics matter and where they live, not what they read today.
