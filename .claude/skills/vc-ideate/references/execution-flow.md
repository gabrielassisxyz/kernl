# Execution Flow: Phases 1, 1.5, and 2

Load this file after Phase 0 completes. It contains the grounding dispatch, topic decomposition, and divergent ideation phases.

## Phase 1: Mode-Aware Grounding

Before generating ideas, gather grounding. The dispatch set depends on the mode chosen in Phase 0.3. Web research runs in all modes (skip phrases honored). Learnings runs in repo mode and elsewhere-software, and is **skipped by default in elsewhere-non-software** — the CWD repo's `docs/solutions/` almost always contains engineering patterns that do not transfer to naming, narrative, personal, or non-digital business topics.

**Surprise-me grounding depth.** When Phase 0.2 routed to surprise-me mode, Phase 1 must produce richer material than specified mode — Phase 2 sub-agents will discover their own subjects from what Phase 1 returns, so texture matters:

- **Repo mode surprise-me:** the codebase-scan sub-agent samples a few representative files per top-level area (not just reads the top-level layout + AGENTS.md), surfaces recent PR/commit activity as signal about what's actively being worked on, and — when issue intelligence runs — passes issue themes as first-class input rather than footnote. Keep the scan bounded: representative, not exhaustive.
- **Elsewhere mode surprise-me:** user-context synthesis extracts themes, recurring language, tensions, and omissions from whatever the user supplied, rather than just restating it. Web research broadens beyond narrow prior-art for a single subject toward the domain's landscape.
- Specified mode keeps the current shallower scan — the user's named subject anchors what's relevant, so broader exploration is unnecessary.

Generate a `<run-id>` once at the start of Phase 1 (8 hex chars). Reuse it for the V15 cache file (this phase) and the V17 checkpoints (Phases 2 and 4) so they share one per-run scratch directory.

**Pre-resolve the scratch directory path.** Scratch lives directly under `/tmp` (not under `$TMPDIR` and not under `.context/`). `$TMPDIR` on macOS resolves to an obscure per-user path like `/var/folders/64/.../T/` that is hostile for users who want to inspect checkpoints, copy them elsewhere, or reference them later — `/tmp` is universally accessible on macOS, Linux, and WSL, and the per-user isolation `$TMPDIR` provides is not valuable for ephemeral ideation scratch. Run one bash command to create the directory and capture its absolute path for downstream use.

```bash
SCRATCH_DIR="/tmp/vibe-chaos-to-concept/vc-ideate/<run-id>"
mkdir -p "$SCRATCH_DIR"
echo "$SCRATCH_DIR"
```

Use the echoed absolute path (`/tmp/vibe-chaos-to-concept/vc-ideate/<run-id>`) as `<scratch-dir>` for every subsequent checkpoint write and cache read in this run. The run directory is not deleted on Phase 6 completion — the V15 cache is session-scoped and reused across run-ids, and the checkpoints follow the cross-invocation-reusable convention of leaving session-scoped artifacts for later invocations to find.

Run grounding agents in parallel in the **foreground** (do not background — results are needed before Phase 2):

**Repo mode dispatch:**

1. **Quick context scan** — dispatch a general-purpose sub-agent using the platform's cheapest capable model (e.g., `model: "haiku"` in Claude Code) with this prompt:

   > Read the project's AGENTS.md (or CLAUDE.md only as compatibility fallback, then README.md if neither exists), then discover the top-level directory layout using the native file-search/glob tool (e.g., `Glob` with pattern `*` or `*/*` in Claude Code). Also read `STRATEGY.md` if it exists — it captures the product's target problem, approach, persona, metrics, and tracks.
   >
   > **Two paths for other root-level `*.md` files**, depending on whether the focus hint names them:
   >
   > - **User-named references** — if the focus hint names a specific root-level `*.md` file (e.g., focus is "ideate based on FEEDBACK.md", "use NOTES.md as input", "review the gaps in TODO.md"), fully read that file and include its content under a heading `User-named references`. Phase 2 treats these as *constraint*, so sub-agents need actual content, not a gist. Quote or summarize substantive sections; keep one-line gists for files that are mentioned but not the actual subject.
   > - **Additional context** — for any other root-level `*.md` files (not named in the focus), read briefly and include a one-line gist under a heading `Additional context`. Phase 2 treats these as *background*, so a gist is sufficient.
   >
   > Return a concise summary (under 40 lines, longer if user-named references include substantive content) covering:
   >
   > - project shape (language, framework, top-level directory layout)
   > - notable patterns or conventions
   > - obvious pain points or gaps
   > - likely leverage points for improvement
   > - product strategy summary, if `STRATEGY.md` was present — include the approach and active tracks verbatim so ideation can weight toward strategy-aligned directions
   > - `User-named references` section (when the focus hint named root-level `*.md` files)
   > - `Additional context` section (when other root-level `*.md` files exist that the focus did not name)
   >
   > Keep the scan shallow otherwise — read only top-level documentation and directory structure. Do not analyze GitHub issues, templates, or contribution guidelines. Do not do deep code search.
   >
   > Focus hint: {focus_hint}

2. **Learnings search** — dispatch the `vc-learnings-researcher` agent with a brief summary of the ideation focus.

3. **Web research** (always-on; see "Web research" subsection below for skip-phrase and V15 cache handling).

4. **Issue intelligence** (conditional) — if issue-tracker intent was detected in Phase 0.3, dispatch the `vc-issue-intelligence-analyst` agent with the focus hint. Run in parallel with the other agents.

   If the agent returns an error (gh not installed, no remote, auth failure), log a warning to the user ("Issue analysis unavailable: {reason}. Proceeding with standard ideation.") and continue with the remaining grounding.

   If the agent reports fewer than 5 total issues, note "Insufficient issue signal for theme analysis" and proceed with default ideation frames in Phase 2.

**Elsewhere mode dispatch (skip the codebase scan; user-supplied context is the primary grounding):**

1. **User-context synthesis** — dispatch a general-purpose sub-agent (cheapest capable model) to read the user-supplied context from Phase 0.4 intake plus any rich-prompt material, and return a structured grounding summary that mirrors the codebase-context shape (project shape → topic shape; notable patterns → stated constraints; pain points → user-named pain points; leverage points → opportunity hooks the context implies). This keeps Phase 2 sub-agents agnostic to grounding source.

2. **Learnings search** *(elsewhere-software only; skipped by default in elsewhere-non-software)* — dispatch the `vc-learnings-researcher` agent with the topic summary in case relevant institutional knowledge exists (skill-design patterns, prior solutions in similar shape). Skip for elsewhere-non-software: the CWD's `docs/solutions/` is unlikely to be topically relevant for non-digital topics, and running it risks polluting generation with unrelated engineering patterns.

3. **Web research** — same as repo mode (see subsection below).

Issue intelligence does not apply in elsewhere mode. Slack research is opt-in for both modes (see "Slack context" below).

### Web Research (V5, V15)

Always-on for both modes. Skip when the user said "no external research", "skip web research", or equivalent in their prompt or earlier answers; in that case, omit the `vc-web-researcher` agent from dispatch and note the skip in the consolidated grounding summary.

Reuse prior web research within a session via a sidecar cache — see `references/web-research-cache.md` for the cache file shape, reuse check, append behavior, and platform-degradation rules. Read it the first time the `vc-web-researcher` agent would be dispatched in this run (and on every subsequent dispatch where the cache might apply).

When dispatching the `vc-web-researcher` agent, pass: the focus hint, a brief planning context summary (one or two sentences), and the mode. Do not pass codebase content — the agent operates externally.

### Consolidated Grounding Summary

Consolidate all dispatched results into a short grounding summary using these sections (omit any section that produced nothing). Phase 1.5 will append a `Topic axes` section to this same summary after consolidation completes:

- **Codebase context** *(repo mode)* — project shape, notable patterns, pain points, leverage points (project-defining files: AGENTS.md/CLAUDE.md/README.md/STRATEGY.md) OR **Topic context** *(elsewhere mode)* — topic shape, stated constraints, user-named pain points, opportunity hooks
- **User-named references** *(repo mode, when the focus hint named root-level `*.md` files)* — full content from files the user explicitly named in their prompt or focus. Phase 2 treats these as constraint
- **Additional context** *(repo mode, when other root-level markdown was discovered but not named)* — one-line gists per file. Phase 2 treats these as background, not direction
- **Past learnings** — relevant institutional knowledge from `docs/solutions/`
- **Issue intelligence** *(when present, repo mode only)* — theme summaries with titles, descriptions, issue counts, and trend directions
- **External context** *(when web research ran)* — prior art, adjacent solutions, market signals, cross-domain analogies. Note "(reused from earlier dispatch)" when V15 reuse fired
- **Slack context** *(when present)* — organizational context

**Failure handling.** Grounding agent failures follow "warn and proceed" — never block on grounding failure. If a general-purpose web research agent fails (network, tool unavailable), log a warning ("External research unavailable: {reason}. Proceeding with internal grounding only.") and continue. If elsewhere-mode intake produced no usable context, note in the grounding summary that context is thin so Phase 2 sub-agents can compensate with broader generation.

**Slack context** (opt-in, both modes) — never auto-dispatch. When the user asks for Slack context and Slack tools are available (look for any `slack-researcher` agent or `slack` MCP tools in the current environment), dispatch a Slack research agent with the focus hint in parallel with other Phase 1 agents. When tools are present but the user did not ask, mention availability in the grounding summary so they can opt in. When the user asked but no Slack tools are reachable, surface the install hint instead.

## Phase 1.5: Topic-Surface Decomposition

Before dispatching frame agents in Phase 2, decompose the topic into 3-5 orthogonal **axes** that name *what aspects of the subject to think about*. Phase 2 frames determine *how to think* (the lens); axes determine *what to think on* (the surface). Without an explicit axis list, parallel frames tend to converge on whichever interpretation of the subject is most salient at first read — other parts of the surface go unexamined regardless of how many frames run. Lens diversity alone does not produce surface coverage.

This step is a single orchestrator-side analysis against the grounding summary already in context. No sub-agent dispatch, no additional grounding read, no user-facing question.

**Axis criteria:**

- **3-5 axes.** Fewer than 3 means the topic is atomic — skip per the rule below. More than 5 fragments dispatch and produces thin coverage on each.
- **Orthogonal.** A single idea should naturally fall on one axis, not span multiple. Merge axes that overlap heavily.
- **Derived from grounding.** The grounding summary contains the substance the axes name; do not pick axes from a generic template (e.g., "discovery / engagement / retention" applied to every topic).
- **At the same level.** Don't mix "the entire pricing page" with "the $9.99 tier copy" in the same list.
- **Named in the topic's language.** "Send mechanics" beats "outbound flow optimization." Use words a reader of the topic would recognize, not meta-language about ideation.

**Worked examples (illustrative, not a template — derive from actual grounding):**

| Topic | Axes |
|---|---|
| Social sharing of crossfire and convergence pages | Send mechanics; discovery (receive side); arrival/dwell experience; compounding over time; actor types (first-party, expert, reader) |
| Improve our authentication system | Sign-in flow; session management; account recovery; permissions; identity providers |
| Dark mode for our app | Visual surfaces; toggle UX; system-preference detection; asset variants; edge cases (third-party content) |
| Cache invalidation in the data layer | Trigger surfaces; coordination across replicas; staleness tolerance per data class; observability of invalidation events |

**Skip condition.** Some subjects are atomic and resist meaningful decomposition — a single string output (a name, a tagline), a narrowly-scoped tactical fix ("the typo on line 47 of README"), or a topic where the candidate axes *are* the deliverable (e.g., "what surface should the API expose?"). When 3+ orthogonal axes that pass the criteria above cannot be generated, skip decomposition. Note `Decomposition skipped — atomic subject` in the grounding summary so the artifact records the choice.

**Surprise-me skip.** In surprise-me mode there is no settled subject to decompose — different frames will surface different subjects in Phase 2, and the cross-cutting synthesis step there serves the analogous coverage role. Skip Phase 1.5 in surprise-me mode and note `Decomposition skipped — surprise-me mode` in the grounding summary.

Append the axis list (or skip-reason) to the consolidated grounding summary under a section labeled `Topic axes`. Phase 2 reads this section to thread axes into sub-agent prompts; Phase 3 uses it for axis-spread scoring; Phase 5's artifact template includes it under Grounding Context.

## Phase 2: Divergent Ideation

Generate the full candidate list before critiquing any idea.

Dispatch parallel ideation sub-agents on the inherited model (do not tier down -- creative ideation needs the orchestrator's reasoning level). Omit the `mode` parameter so the user's configured permission settings apply. Dispatch count is mode-conditional: **4 sub-agents only when issue-tracker intent was detected in Phase 0.2 AND the issue intelligence agent returned usable themes** (see override below — cluster-derived frames capped at 4); **6 sub-agents otherwise**, including the insufficient-issue-signal fallback from Phase 1 where intent triggered but themes were not returned. Each targets ~6-8 ideas (yielding ~36-48 raw ideas across 6 frames or ~24-32 across 4 frames, roughly 25-30 survivors after dedupe in the 6-frame path and fewer in the 4-frame path). Adjust per-agent targets when volume overrides apply (e.g., "100 ideas" raises it, "top 3" may lower the survivor count instead).

Give each sub-agent: the grounding summary, the focus hint, the per-agent volume target, the **topic axis list from Phase 1.5** (when decomposition produced one), and an instruction to generate raw candidates only (not critique). Each agent's first few ideas tend to be obvious -- push past them. Ground every idea in the Phase 1 grounding summary.

**Axis spread instruction.** When an axis list is present, instruct each sub-agent to distribute its ideas across multiple axes — the frame's lens applies to every axis, but ideas should not all cluster on one. Each idea must be tagged with the axis it targets. The frame is a lens; the axis list is the surface map. A frame that plausibly reaches an axis should produce at least one idea there before doubling up on a different axis. When decomposition was skipped (atomic subject or surprise-me), omit the axis instruction entirely — do not invent axes at dispatch time.

**Constraint vs background.** In the dispatch prompt, mark the user's prompt, focus hint, and any *User-named references* (root-level files the user named in their focus and the codebase-scan fully read) as *constraints* — ideas that violate them are out regardless of basis. Mark the rest of the grounding summary (codebase context, additional context, learnings, external context) as *background* — informative, not directive. Background can support an idea's basis and inform direction; it must not pull ideation toward whatever was loudest in the corpus when the user named a different focus. This is the primary defense against grounding noise (an unrelated `FEEDBACK.md` the user did not name, a tangentially-cited prior-art result) shaping survivors against user intent.

Assign each sub-agent a different ideation frame as a **starting bias, not a constraint**. Prompt each to begin from its assigned perspective but follow any promising thread -- cross-cutting ideas that span multiple frames are valuable.

**Frame selection (mode-symmetric — same six frames in repo and elsewhere modes):**

1. **Pain and friction** — user, operator, or topic-level pain points; what is consistently slow, broken, or annoying.
2. **Inversion, removal, or automation** — invert a painful step, remove it entirely, or automate it away.
3. **Assumption-breaking and reframing** — what is being treated as fixed that is actually a choice; reframe one level up or sideways.
4. **Leverage and compounding** — choices that, once made, make many future moves cheaper or stronger; second-order effects.
5. **Cross-domain analogy** — generate ideas by asking how completely different fields solve a structurally analogous problem. The grounding domain is the user's topic; the analogy domain is anywhere else (other industries, biology, games, infrastructure, history). Push past the obvious analogy to non-obvious ones.
6. **Constraint-flipping** — invert the obvious constraint to its opposite or extreme. What if the budget were 10x or 0? What if the team were 100 people or 1? What if there were no users, or 1M? Use the resulting design as a candidate even if the constraint flip itself is not realistic.

**Issue-tracker mode override (repo mode only).** When issue-tracker intent is active and themes were returned by the issue intelligence agent: each high/medium-confidence theme becomes a frame. Pad with frames from the 6-frame default pool (in the order listed above) if fewer than 3 cluster-derived frames. Cap at 4 total — issue-tracker mode keeps its tighter dispatch by design.

**Per-idea output contract (uniform across all frames, all modes):**

Each sub-agent returns this structure per idea:

- **title**
- **summary** (2-4 sentences)
- **axis** — required when Phase 1.5 produced an axis list. Pick the one axis this idea most centrally targets; do not span. Omit entirely when decomposition was skipped.
- **basis** (required, tagged) — one of:
  - `direct:` quoted line / specific file / named issue / explicit user-supplied context
  - `external:` named prior art, domain research, adjacent pattern, with source
  - `reasoned:` explicit first-principles argument for why this move likely applies — not a gesture; the argument is written out
- **why_it_matters** — connects the basis to the move's significance
- **meeting_test** — one line confirming this would warrant team discussion (waived when Phase 0.5 detected tactical focus signals)

Basis is required, not optional. If a sub-agent cannot articulate a basis of at least one type, the idea does not surface. The failure mode to prevent is generic "AI-slop" ideas that sound plausible but lack a basis the user can verify.

**Generation rules (uniform across frames, all modes):**

- Every idea carries an articulated basis. Unjustified speculation does not surface, regardless of how plausible it sounds.
- Bias toward the basis type your frame naturally produces — pain/inversion/leverage tend toward `direct:`; analogy and constraint-flipping tend toward `reasoned:`; assumption-breaking is mixed — but don't exclude other basis types.
- Apply the meeting-test as a default floor: would this idea warrant team discussion? If not, it's below the floor and does not surface. The floor is relaxed only when Phase 0.5 detected tactical focus signals.
- Stay within the subject's identity. Product expansions, new surfaces, new markets, retirements, and architectural pivots are fair game when the basis supports them. Subject-replacement moves (abandoning the project, pivoting to unrelated domains, becoming a different organization) are out regardless of basis.
- **Honor the asked scope.** When the focus hint names a part of the subject (a flow, a stage, a section, a feature within a larger product — e.g., "account settings", "onboarding flow", "pricing page copy", "gameplay rules"), ideate at full ambition *within that scope*. Expanding the surface to the whole subject — proposing fundamental changes to the broader product when the user named one slice — is a scope mismatch even when no subject-replacement occurred. Big-picture thinking still applies; it just operates inside the bounded surface the user named, not by widening the surface.

**Surprise-me mode addendum.** When Phase 0.2 routed to surprise-me, include this additional instruction in each sub-agent's dispatch prompt:

> No user-specified subject. Through your frame's lens, explore the Phase 1 material and identify the subject(s) you find most interesting for this frame. Different frames finding different subjects is the feature — cross-subject divergence is what makes surprise-me valuable. Each idea still carries a basis; the basis may include identification of the subject itself (why *this* subject is worth ideating on through your lens, citing what in the Phase 1 material signals it).

After all sub-agents return:

1. Merge and dedupe into one master candidate list.
2. Synthesize cross-cutting combinations -- scan for ideas from different frames that combine into something stronger. In specified mode, expect 3-5 additions at most. **In surprise-me mode, cross-cutting is the magic layer** — frames often converge on overlapping subjects or find complementary angles; expect 5-8 additions and give this step more attention. Surface combinations that span multiple frame-chosen subjects as a distinctive surprise-me output pattern.
3. **Axis-coverage check (when Phase 1.5 produced an axis list; skipped otherwise).** Count ideas per axis after dedupe. For any axis with zero ideas, dispatch one recovery sub-agent (any unused frame, or the frame whose lens fits the missing axis best — e.g., Pain & friction for usability axes, Cross-domain analogy for distribution or compounding axes) targeting that axis specifically. The recovery dispatch carries the same per-idea output contract and ~3-5 ideas as its target. **Cap recovery at 2 axes total** — if more than 2 axes are empty after the first round, accept thin coverage rather than fanning out further. After recovery returns, merge into the master list and dedupe again. Note empty axes that were not recovered in the rejection summary as "axis: <name> — recovery skipped (cap reached)" so the gap is visible to the user.
4. If a focus was provided, weight the merged list toward it without excluding stronger adjacent ideas.
5. Spread ideas across multiple dimensions when justified: workflow/DX, reliability, extensibility, missing capabilities, docs/knowledge compounding, quality/maintenance, leverage on future work.

**Checkpoint A (V17).** Immediately after the cross-cutting synthesis step completes and the raw candidate list is consolidated, write `<scratch-dir>/raw-candidates.md` (using the absolute path captured in Phase 1) containing the full candidate list with sub-agent attribution. This protects the most expensive output (6 parallel sub-agent dispatches + dedupe) before Phase 3 critique potentially compacts context. Best-effort: if the write fails (disk full, permissions), log a warning and proceed; the checkpoint is not load-bearing. Not cleaned up at the end of the run (the run directory is preserved so the V15 cache remains reusable across run-ids in the same session — see Phase 6).

After merging and synthesis — and before presenting survivors — load `references/post-ideation-workflow.md`. This load is non-optional. The file contains the adversarial filtering rubric, artifact template, quality bar, and the canonical Phase 6 handoff menu (Refine, Review and iterate in chat, Brainstorm, Save and end) — these options do not appear anywhere in the main SKILL.md body. Skipping the load silently degrades every subsequent step; the agent improvises the menu from memory instead of presenting the documented options. "Quickly" means fewer Phase 2 sub-agents, not skipping references. Do not load this file before Phase 2 agent dispatch completes.
