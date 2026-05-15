---
name: vc-ideate
description: "Generate and critically evaluate grounded ideas about a topic. Use when asking what to improve, requesting idea generation, exploring surprising directions, or wanting the AI to proactively suggest strong options before brainstorming one in depth. Triggers on phrases like 'what should I improve', 'give me ideas', 'ideate on X', 'surprise me', 'what would you change', or any request for AI-generated suggestions rather than refining the user's own idea."
argument-hint: "[feature, focus area, or constraint]"

---

# Generate Improvement Ideas

**Note: The current year is 2026.** Use this when dating ideation documents and checking recent ideation artifacts.

`vc-ideate` precedes `vc-brainstorm`.

- `vc-ideate` answers: "What are the strongest ideas worth exploring?"
- `vc-brainstorm` answers: "What exactly should one chosen idea mean?"
- `vc-plan` answers: "How should it be built?"

This workflow produces a ranked ideation artifact in `docs/`. It does **not** produce requirements, plans, or code.

## Interaction Method

Use the platform's blocking question tool: `AskUserQuestion` in Claude Code (call `ToolSearch` with `select:AskUserQuestion` first if its schema isn't loaded), `request_user_input` in Codex, `ask_user` in Gemini, `ask_user` in Pi (requires the `pi-ask-user` extension). Fall back to numbered options in chat only when no blocking tool exists in the harness or the call errors (e.g., Codex edit modes) — not because a schema load is required. Never silently skip the question.

Ask one question at a time. Prefer concise single-select choices when natural options exist.

## Focus Hint

<focus_hint> #$ARGUMENTS </focus_hint>

Interpret any provided argument as optional context. It may be:

- a concept such as `DX improvements`
- a path such as `plugins/my-project/skills/`
- a constraint such as `low-complexity quick wins`
- a volume hint such as `top 3`, `100 ideas`, or `raise the bar`

If no argument is provided, proceed with open-ended ideation.

## Core Principles

1. **Ground before ideating** - Scan the actual codebase first. Do not generate abstract product advice detached from the repository.
2. **Generate many -> critique all -> explain survivors only** - The quality mechanism is explicit rejection with reasons, not optimistic ranking. Do not let extra process obscure this pattern.
3. **Route action into vc-brainstorm** - Ideation identifies promising directions; `vc-brainstorm` defines the selected one precisely enough for planning. Do not skip to planning - vc-brainstorm is the next step.

## Execution Flow

### Phase 0: Resume and Scope

#### 0.1 Check for Recent Ideation Work

Look in `docs/` for ideation documents created within the last 30 days.

Treat a prior ideation doc as relevant when:

- the topic matches the requested focus
- the path or subsystem overlaps the requested focus
- the request is open-ended and there is an obvious recent open ideation doc
- the issue-grounded status matches: do not offer to resume a non-issue ideation when the current argument indicates issue-tracker intent, or vice versa — treat these as distinct topics

If a relevant doc exists, ask whether to:

1. continue from it
2. start fresh

If continuing:

- read the document
- summarize what has already been explored
- preserve previous idea statuses
- update the existing file instead of creating a duplicate

#### 0.2 Subject-Identification Gate

Before classifying mode or dispatching any grounding, check whether the subject of ideation is identifiable. Every downstream agent — grounding and ideation — needs to know what it's working on. If the subject is ambiguous enough that reasonable sub-agents would diverge on what the topic even is (bare words like `improvements`, `ideas`, `birthday cakes`, `vacation destinations`), the output will be scattered.

**Questioning principles (apply in this phase and in 0.4):**

- Questions exist only to supply what sub-agents need to operate: an identifiable subject (this phase) and enough context for the agent to say something specific about it (0.4, elsewhere modes only). Nothing else.
- Never ask about solution direction, constraints, audience, tone, success criteria, or anything that characterizes the subject — those belong to `vc-brainstorm`.
- Always keep "Surprise me" (letting the agent decide the focus) as a real option, not a fallback for when the user can't name a subject. Ideation is allowed to be greenfield by design.
- Stop as soon as the subject is identifiable or the user has delegated to "Surprise me." More than 3 total questions across 0.2 and 0.4 is a smell that ideation is not the right workflow — consider suggesting `vc-brainstorm`.

**Detection — issue-tracker intent (repo mode only; subject-identifying).**

Issue-tracker intent requires an explicit reference to the tracker or to reports filed in it. Trigger only when the prompt uses phrases like `github issues`, `open issues`, `issue patterns`, `issue themes`, `what users are reporting`, or `bug reports` — the subject is "issues in the tracker." Proceed to 0.3 with issue-tracker intent flagged.

Do NOT trigger on arguments that merely mention bugs as a focus: `bug in auth`, `fix the login issue`, `the signup bug`, `top 3 bugs in authentication` — these are focus hints on regular ideation, not requests to analyze the issue tracker. A bare `bugs` with no tracker phrasing is handled by the vagueness check below, not here.

When combined (e.g., `top 3 issue themes in authentication`, `biggest bug reports about checkout`): detect issue-tracker intent first, volume override in 0.5, remainder is the focus hint. The focus narrows which issues matter; the volume override controls survivor count.

**Detection — subject identifiability.**

The test: would a reader, seeing only this prompt, know what subject the agent should ideate on? Apply judgment to what the words *refer to*, not to their length or surface form.

- **Vague — ask the scope question.** The prompt refers to a quality, category, or placeholder without naming a specific thing. Reasonable readers would pick different subjects. Illustrative cases: `improvements`, `ideas`, `things to fix`, `quick wins`, `what to build`, `bugs` (as the whole prompt, not as a topic like "bugs in auth"), an empty prompt. These are examples of the pattern, not a lookup table — recognize vagueness by what the words point to (a catch-all quality), not by matching specific words.

- **Identifiable — proceed to 0.3.** The prompt names or plausibly names a specific subject: a feature, concept, document, subsystem, page, flow, or concrete topic. A reader would know where to direct thought even without knowing the domain. Illustrative cases: `authentication system`, `our sign-up page`, `browser sniff`, `dark mode`, `cache invalidation`, `a unicorn cake for my 7-year-old`, `plot ideas for a short story`.

**Key distinction:** vagueness is about what the words *refer to*, not phrase length. `browser sniff` is two words but plausibly names a feature, so it is identifiable. `quick wins` is two words but refers only to a quality, so it is vague. Do not treat short phrases as vague by default.

**Being inside a repo does not settle vagueness.** `improvements` in any repo is still scattered across DX, reliability, features, docs, tests, architecture. The repo provides material for grounding *after* a subject is settled, not the subject itself. Do not silently interpret a vague prompt as "about this repo" and proceed.

**Genuine ambiguity (repo mode).** When judgment leaves real doubt on a short phrase — it could be a named feature or a vague concept — a single cheap check settles it: Glob for the phrase in filenames, or Grep for it in README/docs. If it appears anywhere, treat as identifiable and proceed. If it has no repo footprint and still reads vaguely, ask the scope question.

When in doubt otherwise, err toward asking — one question is trivial compared to dispatching ~9 agents on a scattered interpretation.

**The scope question.**

Use the platform's blocking question tool: `AskUserQuestion` in Claude Code (call `ToolSearch` with `select:AskUserQuestion` first if its schema isn't loaded), `request_user_input` in Codex, `ask_user` in Gemini, `ask_user` in Pi (requires the `pi-ask-user` extension). Fall back to numbered options in chat only when no blocking tool exists or the call errors — not because a schema load is required. Never silently skip.

- **Stem:** "What should the agent ideate about?"
- **Options:**
  - "Specify a subject the agent should ideate on"
  - "Surprise me — let the agent decide what to focus on"
  - "Cancel — let me rephrase"

Routing:

- **Specify** → accept the user's follow-up as the subject. Re-apply the identifiability check once. If still ambiguous, ask once more with "Surprise me" still on the menu. Do not cascade toward specificity about *how* to solve — only about *what* the subject is.
- **Surprise me** → mark the run as **surprise-me mode**. The agent will discover subjects from Phase 1 material rather than carry a user-specified subject. This is a first-class mode — it changes how Phase 1 scans and how Phase 2 sub-agents operate (see those phases). **Dispatch routing for surprise-me is deterministic:** if CWD is inside a git repo, route to repo-grounded (the codebase supplies substance); otherwise route to elsewhere-software and require Phase 0.4 to collect at least one piece of substance (URL, description, draft, or paste) before dispatching — "surprise me" outside a repo is only viable once the user has supplied something to surprise them about. Skip Decision 1/2 in Phase 0.3: with no user subject there is no prompt content to weigh, and surprise-me never routes to elsewhere-non-software (no way to infer naming/narrative/personal intent without a subject). The user can correct by interrupting and re-invoking with a named subject.
- **Cancel** → exit cleanly. Narrate that the user can rephrase and re-invoke.

#### 0.3 Mode Classification

Classify the **subject of ideation** (settled in 0.2) into one of three modes for dispatch routing. A user inside any repo can ideate about something unrelated to that repo; a user in `/tmp` can ideate about code they hold in their head.

**Surprise-me short-circuit.** When Phase 0.2 routed to surprise-me mode, skip the two-decision classification below and use the deterministic rule stated in 0.2: repo-grounded when CWD is inside a git repo, elsewhere-software otherwise. The ambiguity-confirmation step at the end of this section also does not fire for surprise-me — there is no user subject to be ambiguous about. State the chosen mode in one sentence and proceed to 0.4.

For specified subjects, make two sequential binary decisions, enumerating negative signals at each:

**Decision 1 — repo-grounded vs elsewhere.** Weigh prompt content first, topic-repo coherence second, and CWD repo presence as supporting evidence only.

- Positive signals for **repo-grounded**: prompt references repo files, code, architecture, modules, tests, or workflows; topic is clearly bounded by the current codebase. Issue-tracker intent from 0.2 is always repo-grounded.
- Negative signals (push toward **elsewhere**): prompt names things absent from the repo (pricing, naming, narrative, business model, personal decisions, brand, content, market positioning); topic is creative, business, or personal with no code surface.

**Decision 2 (only fires if Decision 1 = elsewhere) — software vs non-software.** Classify by whether the *subject* of ideation is a software artifact or system, not by where the individual ideas will eventually land. If the topic concerns a product, app, SaaS, web/mobile UI, feature, page, or service, it is **elsewhere-software** — even when the ideas themselves are about copy, UX, CRO, pricing, onboarding, visual design, or positioning *for that software product*. **Elsewhere-non-software** is reserved for topics with no software surface at all: company or brand naming (independent of product), narrative and creative writing, personal decisions, non-digital business strategy, physical-product design.

Sample classifications:

- "Improve conversion on our sign-up page" → elsewhere-software (the subject is a page)
- "Redesign the onboarding flow" → elsewhere-software (the subject is a flow)
- "Pricing page A/B test ideas" → elsewhere-software (the subject is a page)
- "Features to add to our note-taking app" → elsewhere-software
- "Name my new coffee shop" → elsewhere-non-software (the subject is a brand)
- "Plot ideas for a short story" → elsewhere-non-software (the subject is a narrative)
- "Options for my next career move" → elsewhere-non-software (the subject is a personal decision)

State the inferred approach in one sentence at the top, using plain language the user will recognize. Never print the internal taxonomy label (`repo-grounded`, `elsewhere-software`, `elsewhere-non-software`) to the user — those names are for routing only. Adapt the template below to the actual topic; pick a domain word from the topic itself (e.g., "landing page", "onboarding flow", "naming", "career decision") instead of a mode label.

- **Repo-grounded:** "Treating this as a topic in this codebase — about X."
- **Elsewhere-software:** "Treating this as a product/software topic outside this repo — about X."
- **Elsewhere-non-software:** "Treating this as a [naming | narrative | business | personal] topic — about X."

Do not prescribe correction phrases ("say X to switch"). State the inferred mode plainly and proceed. If the user disagrees, they will correct in their own words or interrupt to re-invoke — reclassify and re-run any affected routing when that happens.

**Active confirmation on mode ambiguity.** Only fire when mode classification is genuinely ambiguous *after* 0.2 settled the subject — e.g., "our docs" could mean repo docs (repo-grounded) or public marketing docs (elsewhere-software). Most subjects settled in 0.2 classify cleanly here. When ambiguous, ask one confirmation question via the blocking tool with two self-contained labels naming the two candidate interpretations in plain language (e.g., "Treat as repo docs in this codebase" vs "Treat as public marketing docs") — never leak internal mode names. Otherwise the one-sentence inferred-mode statement is sufficient; do not ask.

**Routing rule (non-software mode).** When Decision 2 = non-software, still run Phase 1 Elsewhere-mode grounding (user-context synthesis + web-research by default; skip phrases honored). Learnings-researcher is skipped by default in this mode — the CWD's `docs/solutions/` rarely transfers to naming, narrative, personal, or non-digital business topics; see Phase 1 for the full rationale. Then load `references/universal-ideation.md` and follow it in place of Phase 2's software frame dispatch and the Phase 6 menu narrative. This load is non-optional — the file contains the domain-agnostic generation frames, critique rubric, and wrap-up menu that replace Phase 2 and the post-ideation menu for this mode, and none of those details live in this main body. Improvising from memory produces the wrong facilitation for non-software topics. Do not run the repo-specific codebase scan at any point. The §6.5 Review Failure Recovery in `references/post-ideation-workflow.md` still applies — load and follow it whenever a file save (the elsewhere-mode default for Save and end) fails, so the local-save fallback path stays reachable in non-software elsewhere runs.

#### 0.4 Context-Substance Gate (Elsewhere Modes Only)

Skip in repo mode — the repo provides the substance Phase 1 agents work from. In elsewhere modes (both software and non-software), Phase 1 agents depend on user-supplied context for substance. A bare prompt with no description, URL, or artifact leaves the user-context-synthesis agent with nothing to synthesize and weakens web research's relevance.

Apply the discrimination test: would swapping one piece of the user's stated context for a contrasting alternative materially change which ideas survive? If yes, context is load-bearing — proceed. If no, ask 1-3 narrowly chosen questions focused on **supplying substance, not characterizing the subject**:

- A URL or file to read
- A brief description of the current state
- A paste of an existing draft or brief

Build on what the user already provided rather than starting from a template. Default to free-form questions; use single-select only when the answer space is small and discrete. After each answer, re-apply the test before asking another. Stop on dismissive responses ("idk just go") — treat genuine "no context" answers as real answers and note context is thin in the summary so Phase 2 can compensate with broader generation.

**Surprise-me exception.** When the run is in surprise-me mode and routed to elsewhere-software (per 0.2's deterministic routing for no-repo CWDs), at least one piece of substance is required — there is no subject AND no repo, so Phase 1 and 2 agents would have nothing to discover subjects from. Dismissive responses are not acceptable here; if the user still has no context after one ask, tell them the run needs a URL, description, or paste to proceed and end cleanly so they can re-invoke with material.

When the user provides rich context up front (a paste, a brief, an existing draft, a URL), confirm understanding in one line and skip this step entirely.

If this step materially changes the topic (not just adds context but shifts the subject), re-run 0.2 and 0.3 against the refined scope before dispatching Phase 1 — classify on what's actually being ideated on, not the scope at first read.

#### 0.5 Interpret Focus and Volume

Infer two things from the argument and any intake so far:

- **Focus context** — concept, path, constraint, or open-ended
- **Volume override** — any hint that changes candidate or survivor counts

Default volume:

- each ideation sub-agent generates about 6-8 ideas (yielding ~36-48 raw ideas across 6 frames in the default path, or ~24-32 across 4 frames in issue-tracker mode; roughly 25-30 survivors after dedupe in the 6-frame path and fewer in the 4-frame path)
- keep the top 5-7 survivors

Honor clear overrides such as:

- `top 3`
- `100 ideas`
- `go deep`
- `raise the bar`

**Tactical scope detection.** Parse the focus hint (and any intake answers from 0.2 specify path) for tactical signals: `polish`, `typo`, `typos`, `quick wins`, `small improvements`, `cleanup`, `small fixes`. When present, lower the Phase 2 ambition floor — the user has explicitly opted into tactical scope. Default otherwise is step-function (see Phase 2 meeting-test floor).

Use reasonable interpretation rather than formal parsing.

#### 0.6 Cost Transparency Notice

Before dispatching Phase 1, surface the agent count for the inferred mode in one short line so multi-agent cost is not invisible. Compute the count from the actual dispatch decision: 1 grounding-context agent (codebase scan in repo mode; user-context synthesis in elsewhere) + 1 learnings (skip in elsewhere-non-software) + 1 web researcher + 6 ideation = baseline 9 in repo mode and elsewhere-software, 8 in elsewhere-non-software. When issue-tracker intent triggers (repo mode only): add 1 for the issue-intelligence agent and drop ideation from 6 to 4, for a net -1 (baseline 8). Add 1 if the user opted into Slack research. Subtract 1 if the user issued a web-research skip phrase or V15 reuse will fire. In **surprise-me mode**, agent count is the same but per-agent exploration is deeper — note "(surprise-me mode: deeper exploration per agent)" when active. Phase 2's axis-coverage check may dispatch up to 2 additional recovery sub-agents when generation leaves any topic axis empty (skipped in surprise-me mode); when not in surprise-me, append "(+up to 2 if axis-coverage requires recovery)" to the count line.

Examples (defaults, no skips, no opt-ins):

- **Repo mode, specified subject:** "Will dispatch ~9 agents: codebase scan + learnings + web research + 6 ideation sub-agents. Skip phrases: 'no external research', 'no slack'."
- **Repo mode, surprise-me:** "Will dispatch ~9 agents (surprise-me mode: deeper exploration per agent): codebase scan + learnings + web research + 6 ideation sub-agents. Skip phrases: 'no external research', 'no slack'."
- **Repo mode, issue-tracker intent:** "Will dispatch ~8 agents: codebase scan + learnings + web research + issue intelligence + 4 ideation sub-agents. Skip phrases: 'no external research', 'no slack'." Reflects the successful-theme path; if issue intelligence returns insufficient signal (see Phase 1), ideation falls back to 6 sub-agents and the total becomes ~9.
- **Elsewhere-software:** "Will dispatch ~9 agents: context synthesis + learnings + web research + 6 ideation sub-agents. Skip phrases: 'no external research'."
- **Elsewhere-non-software:** "Will dispatch ~8 agents: context synthesis + web research + 6 ideation sub-agents. Skip phrases: 'no external research'."

The line is informational; users do not need to acknowledge it.

### Phases 1, 1.5, and 2: Grounding, Decomposition, and Ideation

After Phase 0 completes, load `references/execution-flow.md` and follow it. This load is non-optional — the file contains the grounding dispatch logic (Phase 1), topic-surface decomposition (Phase 1.5), and divergent ideation dispatch with frame selection, per-idea output contracts, and post-merge synthesis (Phase 2). Skipping it and improvising from memory will degrade grounding quality and ideation diversity.

Phase 2 ends by loading `references/post-ideation-workflow.md` for Phases 3-6 (adversarial filtering, presentation, persistence, and handoff menu).