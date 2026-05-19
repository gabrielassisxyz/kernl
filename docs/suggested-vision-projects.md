# Suggested Vision Projects

> Decomposition of `docs/VISION.md` into brainstormable sub-projects. Each entry is
> sized so it fits a single `vc-brainstorm` → `vc-plan` → implementation cycle —
> meaning each is one focused effort, not the whole product.
>
> **This is suggestion, not commitment.** Order, scope, and dependencies are
> recommendations. Re-evaluate before picking one up.

## How to use this list

1. Pick a project from a wave whose dependencies are satisfied.
2. Open it with `/ce-brainstorm` — that session refines the scope, makes
   architectural choices, and produces a requirements doc in `docs/brainstorms/`.
3. Then `/ce-plan` produces the implementation plan in `docs/plans/`.
4. Then `/beads-workflow` converts the plan into beads with dependencies.
5. Orchestrator executes (`kernl epic run <epic-id> --autopilot`).
6. After PR merge, `/ce-compound` (invoked from `shipment` stage and post-merge hook)
   writes the learning to `docs/solutions/`, which seeds the future graph.

> The `vc-*` skills (`vc-brainstorm`, `vc-plan`, `vc-writing-plans`,
> `vc-convert-plan-to-beads`, `vibe-engineering-mastery`, `vibe-chaos-to-concept`)
> are being replaced by the compound-engineering pipeline above. See
> `docs/spikes/2026-05-19-compound-engineering-pipeline-adoption.md` for the
> transition plan.

**Do not fork prematurely.** Pick one project, finish it, come back to this list.
Parallel brainstorms of related projects make late changes very expensive.

---

## Existing baseline (what already exists, do not redo)

- **Orchestrator MVP** — `orchestrator/` is in flight. Bead execution, parallel
  waves, take loop, SSE monitoring GUI, run-state SQLite, worktree manager, sweep
  package, merge manager. Several decisions (workflow-state migration,
  per-stage backends) are mid-implementation per the recent merge.
- **bd integration** — `bd` CLI as the issue-tracker backend, NDJSON parsing,
  hermetic test patterns.
- **quick-capture (Python/Hyprland)** — already works as a standalone capture
  tool; will be absorbed and re-implemented in Go (see P2.4 below).

Sub-projects below build **on top of** this baseline or **redesign** parts of it
where the vision diverges from the current implementation.

---

## Dependency overview

```
                          P0.1 graph substrate
                         /        |        \
                  P0.2 vault     P1.1 DA     P0.3 graph-tools (read API)
                  watcher         core
                    |              |
       +-----+------+------+-------+-----+--------+
       |     |      |      |       |     |        |
     P2.1  P2.2  P2.3  P2.4   P2.5  P2.6     P2.7
     Notes Memory Bookm. Inbox  Wkfl-eng  GUI-shell  Dispatch
       |     |     |      |       |          |        routing
       |     |     |      |       |          |
       +-----+-----+------+-------+----------+
                           |
                  P3.x composed surfaces, graph insights, etc.
                           |
                  P4.x polish + DevEx
```

The figure is approximate. Actual edges may be tighter than shown.

---

## Wave 0 — Foundation (must come before anything else)

### P0.1 — Graph substrate (schema + SQLite layer)
- **Scope:** SQLite schema for `nodes`, `edges`, `revisions`, `tags`. JSON1 for
  type-specific fields. FTS5 indexes over node bodies + titles. CRUD API in Go.
  Optional `owner_id` / `visibility` columns invisible in UI. Hermetic tests.
- **Why:** every other module reads/writes here.
- **Depends on:** nothing.
- **Source vision sections:** §6.1, §6.2, §6.4.

### P0.2 — Vault watcher + identity injection + path cache + revision log
- **Scope:** filesystem watcher on the configured vault root. Detects new/changed/
  moved/deleted `.md` files. Injects UUID into frontmatter on first save. Maintains
  the `path↔uuid` cache. Indexes body into FTS5. Wikilink resolution by UUID with
  fallback to title. **Auto-saves revision diffs every 5 seconds** for every note
  (human-written and DA-written), with author attribution per revision.
- **Why:** makes the vault a live participant in the graph. Required for any module
  that reads notes (which is most modules). The revision log is here (not per-module)
  because it is cross-cutting infrastructure.
- **Depends on:** P0.1.
- **Source vision sections:** §6.3, §7.2 (revision log), §7.4 (passive discovery), §10.

### P0.3 — Graph traversal/query layer + relevance algorithms
- **Scope:** read-side query helpers — by type, by tag, by edge, by FTS. Implement
  the 4-signal relevance algorithm (direct link, source overlap, Adamic-Adar, type
  affinity). Recursive CTE helpers for path/depth queries. Performance benchmarks
  on synthetic graphs.
- **Why:** every "find related", "show backlinks", "neighbors of X" query lives here.
  Centralizing it prevents N module-specific reimplementations.
- **Depends on:** P0.1.
- **Source vision sections:** §13.

---

## Wave 1 — The actors (DA + workflow engine core)

### P1.1 — DA core (chat layer, scope, permissions)
- **Scope:** persistent DA service. Chat interface (REST + SSE). Scope derivation
  from invocation surface (project / global / specific). Permission-prompt flow
  when DA wants to read a node outside scope. System-prompt construction by scope.
  Conversation history persistence as `ChatMessage` nodes.
- **Why:** the DA is the persistent agent identity of Kernl; almost every other
  module touches it.
- **Depends on:** P0.1, P0.3.
- **Source vision sections:** §7.1.
- **Skill references:** `cass-memory` (procedural memory pattern — compare with the
  DA's persistent memory model); `operationalizing-expertise` (mine session history
  into executable rules — load-bearing for the DA learning-on-user behavior).

### P1.2 — Workflow engine core (shapes, runtime, canonical pipeline)
- **Scope:** declarative YAML workflow shape loader. Stage interface (input/output/
  agent/handoff). Runtime that walks a workflow shape, invokes stages, persists
  `WorkflowRun` nodes. **Implement the vibe-coding canonical pipeline** with all 6
  hot-path stages (Planner, Implementer, Per-bead Reviewer, Merger, Integration
  Reviewer, Releaser) — this absorbs and refactors the current orchestrator code.
- **Why:** the orchestrator's heart. The current `orchestrator/internal/` packages
  are the starting point; this project reshapes them around the vision's stage
  model.
- **Depends on:** P0.1 (writes Bead/Task/WorkflowRun nodes).
- **Source vision sections:** §8.1, §8.2, §8.5.
- **Note:** this is the largest single project on the list. Plan to decompose
  further during its brainstorm.
- **Compounding (embedded, not a new stage):** the `shipment` stage invokes
  `/ce-compound mode:headless` to write `docs/solutions/<categoria>/*.md` with
  frontmatter compatible with the future `Solution`/`Learning` node schema. A
  post-merge hook (GitHub webhook or manual user marker) appends PR review
  comments to the same compound doc — this is the **primary capture channel**
  for the human's judgment on what the swarm produced. When P0.1 lands, a
  migrator transforms each compound doc into a graph node.
- **Skill references:** `ce-code-review` (multi-persona reviewer pattern for
  `implementation_review` and `shipment_review` stages — fully autonomous,
  subagent-parallel, deterministic merge/dedup); `code-review-gemini-swarm-with-ntm`
  (NTM-based alternative pattern); `ubs` (pre-PR quality scan); `multi-pass-bug-hunting`;
  `multi-model-triangulation`; `testing-fuzzing`, `testing-metamorphic` (the
  orchestrator is exactly the "oracle problem" — correct output is unknown but
  I/O relations are predictable), `testing-golden-artifacts`,
  `testing-real-service-e2e-no-mocks`; `release-preparations`, `gh-actions`, `gh-cli`
  (for `shipment`/`shipment_review`); `beads-workflow` (plan→beads converter,
  replaces `vc-convert-plan-to-beads`); `agent-fungibility-philosophy` (design
  constraint — workers fungible, DA non-fungible); `cc-hooks` (event hooks for
  post-stage triggers like "after shipment → /ce-compound").

---

## Wave 2 — The modules (parallel after Wave 1)

### P2.1 — Notes module (markdown editor, wikilinks, tags-as-folders, DA diff-suggest)
- **Scope:** in-app markdown editor (rich or source-first, decide in brainstorm).
  Wikilink autocomplete using P0.2's resolver. Tag pane that can render as
  folder-like hierarchy (Bear / tagfolder pattern). Frontmatter UI (hides UUID,
  shows user-meaningful fields). **DA diff-suggest flow**: when the user asks the
  DA to format/summarize/extract a note, the editor surfaces a diff panel for
  accept/reject/edit; only the user's action commits. Discreet "DA wrote here"
  ribbon in regions authored by the DA.
- **Why:** notes are the user's primary writing surface. The diff-suggest is the
  load-bearing UX that operationalizes the "LLM never silently writes user notes"
  rule from VISION §7.2.
- **Depends on:** P0.2, P1.1.
- **Source vision sections:** §7.2, §10.

### P2.2 — Memory module (additive MemoryClaim/MemoryRefutation)
- **Scope:** schema for `MemoryClaim` and `MemoryRefutation`. DA-side helpers to
  write additively (never overwrite). On-demand summarization that synthesizes a
  view without persisting it. UI to browse memory by topic, with provenance.
- **Why:** the DA's persistent knowledge about the user, deliberately differentiated
  from PAI's lossy-rewrite model.
- **Depends on:** P0.1, P1.1.
- **Source vision sections:** §7.3.
- **Skill references:** `cass-memory` (compare with `MemoryClaim`/`MemoryRefutation`
  — different philosophies on how procedural memory accumulates; may inform schema
  edges and the additive-vs-rewrite decision).

### P2.3 — Bookmarks module (schema, capture, defuddle agent, lists, highlights)
- **Scope:** schema for `Bookmark` and `BookmarkList`. Capture paths: CLI, browser
  extension API (extension can be a separate sub-project), Pocket/Pinboard import.
  Always archive HTML; screenshot for type=link. DA-driven defuddle flow that
  identifies relevant HTML elements and calls a deterministic `defuddler` script.
  Highlights with per-highlight notes.
- **Why:** bookmarks are first-class citizens of the substrate, not an add-on.
- **Depends on:** P0.1, P1.1.
- **Source vision sections:** §11.

### P2.4 — Inbox module (Quick Capture absorbed, Go-native)
- **Scope:** re-implementation in Go of the `quick-capture` project. CLI `kernl
  capture`. Hyprland keybind script + example for other window managers. Inbox UI
  in Kernl. Processing flow: DA classifies, proposes predefined actions, user
  approves. Lifecycle preserves the original `Capture` node and edges to the
  processed result. Daily/weekly rollups by the DA.
- **Why:** capture friction is the user's stated biggest pain. This is the module
  where Kernl first "earns its keep" for the personal-tool half of the product.
- **Depends on:** P0.1, P1.1. Best after P2.1 (so processed-to-Note has a real
  surface) and P2.3 (so processed-to-Bookmark works).
- **Source vision sections:** §9.

### P2.5 — Ingest engine (passive + active + manifest + async review queue)
- **Scope:** active extraction service that runs structured ingest on demand. Uses
  manifest with content hashes to avoid re-processing (claude-obsidian pattern).
  Async review queue with predefined actions (Create Page, Deep Research, Skip,
  Update, Add Contradiction Callout). Review queue UI.
- **Why:** turns the passive watcher (P0.2) into a substrate enrichment machine
  without falling into the silent-continuous-ingest trap.
- **Depends on:** P0.2, P1.1.
- **Source vision sections:** §7.4 (active ingest), §7.5.

### P2.6 — GUI shell (Vue 3 + Nuxt, top-level navigation, sidebar + right sidebar)
- **Scope:** the Vue 3 + Nuxt shell. Module sidebar (left). Main area. Right
  contextual sidebar with multi-mode (DA chat / panel / diff queue / related items).
  Routing and state for the top-level surfaces. Dark theme, Tailwind v4, DaisyUI v5.
  Home surface (friendly overview). Dashboard surface placeholder (P3 fills metrics).
- **Why:** the shell is the place every module renders into. Without it, the rest
  is headless.
- **Depends on:** P0.1, P1.1 (chat needs DA).
- **Source vision sections:** §12.
- **Skill references:** `tui-glamorous` + `tui-inspector` (deferred TUI variant);
  `ui-polish` (post-functional iterative polish loop); `e2e-testing-for-webapps`
  (Playwright pattern for GUI regression tests).

### P2.7 — Dispatch routing + autonomous mode
- **Scope:** `kernl epic create` accepts `--workflow=<shape>`. When absent, DA
  infers from epic content and proposes; user confirms (unless autonomous mode).
  Autonomous-mode flag and global config setting; permission-prompt silencing
  across the system. Log-everything behavior in autonomous mode.
- **Why:** turns "we have multiple workflow shapes" from theory into UX.
- **Depends on:** P1.1, P1.2.
- **Source vision sections:** §7.6, §8.3.
- **Skill references:** `agent-fungibility-philosophy` (dispatch must treat workers
  as fungible); `caam` (account-level switching for Pro/Max subs — **complementary**
  to `litellm`: litellm = gateway/model picker at the API layer, caam = OS account
  switcher when an account hits its rate limit; both are useful, different layers);
  `multi-model-triangulation` (when dispatch escalates to cross-model decisions).

---

## Wave 3 — The polish (composed surfaces, graph features, additional shapes)

### P3.1 — Composed module surfaces (Nexus-style)
- **Scope:** for each module surface (Project view, Task view, Bookmark view,
  Bead view, Note view), implement the panels of related content (related notes,
  sessions, beads, bookmarks). Uses P0.3's relevance queries.
- **Depends on:** P0.3, P2.1, P2.3, P2.4, P1.2, P2.6.
- **Source vision sections:** §12 (composed surfaces row).

### P3.2 — Graph view (knowledge graph visualization)
- **Scope:** dedicated tab. Sigma.js + graphology + ForceAtlas2 (or alternative).
  Color by type or by Louvain community. Hover-highlight neighbors. Performance
  target: 5000+ nodes interactive.
- **Depends on:** P0.3, P2.6.
- **Source vision sections:** §12 (Graph view row), §13.

### P3.3 — Graph insights (Louvain + insights surface)
- **Scope:** Louvain community detection over the graph. Surface for "isolated
  nodes", "sparse communities", "bridge nodes", "surprising connections".
  Clickable to highlight in the graph view. Deep Research button on insight cards
  that triggers `research-shape`.
- **Depends on:** P0.3, P3.2, P2.5 (for Deep Research integration).
- **Source vision sections:** §13.

### P3.4 — Dashboard (metrics + charts)
- **Scope:** the dedicated Dashboard tab. Realized parallelism, epics completed,
  intervention counts, ingest activity, graph growth. Instrument the strategy
  metrics that are currently uninstrumented (intervention-out-of-gate,
  epic-without-rescue, idea-to-epic-per-month).
- **Depends on:** P1.2 (orchestrator emits the metrics).
- **Source vision sections:** §12 (Dashboard row), VISION §18 (parked metric work).

### P3.5 — Additional canonical workflow shapes (brainstorm, research, content)
- **Scope:** author the canonical workflow shape YAMLs for `brainstorm-shape`,
  `research-shape`, `content-writing-shape`. Implement the stage primitives each
  needs (Explorer, Adversarial-pass, Synthesizer, Spec-writer, etc.).
  (`inbox-processing-shape` is delivered as part of P2.4 — Inbox.)
- **Depends on:** P1.2 (engine), P1.1 (DA-driven stages).
- **Source vision sections:** §8.2 (paralleled shapes list).
- **Skill references:** `idea-wizard` (brainstorm shape primitive — also enters the
  external dev pipeline as an alt to `/ce-ideate`); `dueling-idea-wizards`
  (adversarial ideation shape); `modes-of-reasoning-project-analysis` (multi-
  perspective analysis shape); `multi-pass-bug-hunting` (bug-hunt shape).

### P3.6 — Auditor stage implementation (multi-mode full-codebase analysis)
- **Scope:** the Auditor continuous-stage from VISION §8.1. Multiple modes: code
  quality, security, performance, test coverage, docs completeness. On-demand
  invocation; scheduled invocation (Sweeper triggers when threshold met). Output
  goes into the graph as `AuditReport` nodes edged to the project/epic/codebase
  analyzed.
- **Depends on:** P1.2 (engine).
- **Source vision sections:** §8.1 (Auditor row).
- **Skill references:** `codebase-audit` (parametric by domain — primary reference);
  `ux-audit` (`ux` mode); `beads-compliance-and-completion-verification` (compliance
  mode — kernl's recurring bd-status-drift makes this load-bearing); `mock-code-finder`
  (placeholder/stub detection mode); `reality-check-for-project` (vision-vs-implementation
  gap mode); `testing-conformance-harnesses` (spec ↔ implementation harness mode —
  `orchestrator/specs/*` defines behavior that needs proving); `ce-code-review`
  (multi-persona reviewer pattern — reuse from P1.2); `simplify-and-refactor-code-isomorphically`;
  `codebase-pattern-extraction`; `codebase-archaeology`; `codebase-report`.

### P3.7 — Custom workflow authoring (DA-assist + GUI builder)
- **Scope:** chat-driven custom workflow creation ("DA, build me a workflow
  for X") that proposes YAML, shows preview DAG, saves to `.kernl/workflows/`.
  GUI canvas builder for visual editing. Import from URL/repo for community
  workflows.
- **Depends on:** P1.2, P1.1, P2.6.
- **Source vision sections:** §8.2 (custom + community), §12 (custom workflow
  creation flow).

### P3.8 — Scheduled maintenance workflows (cron-triggered audit/fix shapes)
- **Scope:** workflow shapes that run on a cron trigger without manual invocation:
  codebase audit (security, performance, code quality, doc drift, devex), library
  updater sweep, beads compliance audit, mock/stub detection, reality-check against
  vision, git/worktree/stash janitors. Each shape produces remediation beads that
  enter the normal orchestrator queue. Cron infra (or external scheduler hook). Per-
  shape configuration: frequency, scope filter, severity threshold for bead creation.
- **Why:** automates the periodic-hygiene work currently listed manually in
  `docs/dev-workflow-skills.md`. Without this, the rotation runs only when
  the user remembers; with it, the substrate self-maintains.
- **Depends on:** P1.2 (workflow engine), P3.6 (Auditor stage — many cron shapes
  invoke it).
- **Source vision sections:** **TBD — VISION § workflow shapes gains a section on
  scheduled shapes when P3.8 is brainstormed. Updating VISION accordingly is a
  required deliverable of the P3.8 brainstorm phase.**
- **Skill references:** `cc-hooks` (cron/event-trigger pattern); `library-updater`;
  `beads-compliance-and-completion-verification`; `mock-code-finder`;
  `reality-check-for-project`; `codebase-audit`; `git-repo-janitor`,
  `git-stash-janitor`, `git-worktree-branch-rationalization` (worktree/repo hygiene);
  `world-class-doctor-mode-for-cli-tools` (self-healing pattern reference). Manual
  versions of these rotines live in `docs/dev-workflow-skills.md` and that
  file is the bridge: each routine there gets a corresponding shape here.

---

## Wave 4 — DevEx + opening to others

### P4.1 — Setup wizard + tour + `kernl explain`
- **Scope:** first-run wizard that detects environment and configures sensibly.
  In-app tour on first login. `kernl explain <thing>` CLI for plain-language
  concept/command/error explanation using user's own state.
- **Depends on:** the modules being explained existing.
- **Source vision sections:** §14.
- **Skill references:** `installer-workmanship` (curl|bash installer devex);
  `world-class-doctor-mode-for-cli-tools` (kernl already has `doctor` — this is the
  gold-standard pattern to converge on, with capabilities reflection, robot-docs,
  per-run scoring artifact).

### P4.2 — LLM-helper skill for Kernl
- **Scope:** a Claude Code / opencode / Gemini / Cursor skill that knows Kernl
  deeply. Helps any user install, configure, use, and debug Kernl conversationally.
  Published to relevant skill registries.
- **Depends on:** stable core (Wave 0-2 substantially done).
- **Source vision sections:** §14.
- **Skill references:** `agent-ergonomics-and-intuitiveness-maximization-for-cli-tools`
  (the kernl CLI is consumed primarily by agents — the 10-dimension rubric and
  robot-mode pattern directly inform this skill's surface).

### P4.3 — Orchestrator standalone packaging
- **Scope:** the Orchestrator can be installed and run **without** the rest of
  Kernl. Separate binary or build flag (`kernl-orchestrator`). Documentation for
  standalone install. Honors the foolery-go heritage; serves the
  exploring-vibe-coding newcomer.
- **Depends on:** P1.2 substantially complete; clean boundary between orchestrator
  package and the rest of Kernl.
- **Source vision sections:** §15 (exception).
- **Skill references:** `installer-workmanship` (standalone install surface);
  `system-performance-remediation` (perf tooling reference for the standalone
  binary); `documentation-website-for-software-project` (Nextra docs site for the
  standalone orchestrator project).

### P4.4 — Modular view-toggle (super-optional)
- **Scope:** `enabled_modules` config that hides inactive modules from sidebar
  and disables their endpoints/sweepers. Data of disabled modules preserved.
- **Depends on:** modules existing.
- **Source vision sections:** §15.

### P4.5 — Video tutorial production
- **Scope:** short, by-feature video tutorials. Recording, editing (see existing
  `video-obs-youtube-music` skill), publishing.
- **Depends on:** stable surfaces to record against.
- **Source vision sections:** §14.

---

## Wave 5 — Optional / aspirational (do not block on these)

### P5.1 — Browser extension (full bookmark capture path)
- **Scope:** Chrome / Firefox extension. One-click bookmark capture into Kernl.
  Posts to the Kernl REST API. Selection-to-highlight flow on the page.
- **Why optional:** CLI capture and import already cover the bookmark capture
  need. Extension is convenience.
- **Depends on:** P2.3.
- **Skill references:** `browser-extension-automation` (extension scaffold and
  automation patterns).

### P5.2 — Inbox transports beyond CLI/hotkey (telegram, webhook, generic bot)
- **Scope:** webhook receiver. Telegram bot integration. Generic bot adapter.
- **Why optional:** CLI/hotkey is the primary path; mobile capture is "open
  question" in vision.
- **Depends on:** P2.4.

### P5.3 — `.canvas` files (Obsidian-style) as first-class
- **Scope:** load and render `.canvas` files from the vault; treat as
  composable visual notes with their own node type.
- **Why optional:** parked decision in VISION §18.
- **Depends on:** P2.1.

### P5.4 — Strategy metrics full instrumentation
- **Scope:** define and implement the three uninstrumented metrics
  (interventions-out-of-gate, epics-without-rescue, ideas-to-epic-per-month).
- **Why optional but valuable:** without these, the strategy thesis is not
  measurable. Could be merged into P3.4 (Dashboard) or done separately.
- **Depends on:** definitions; instrumentation hooks.
- **Source vision sections:** §18.

---

## Ordering recommendation (single-flight, no parallel sub-projects)

If picking one at a time:

1. **P0.1** — Graph substrate. Foundation.
2. **P0.2** — Vault watcher. Unlocks every notes-touching module.
3. **P1.1** — DA core. Unlocks everything DA-touching.
4. **P1.2** — Workflow engine core. This is the largest one; decompose during its
   brainstorm. Reshapes existing orchestrator code.
5. **P2.6** — GUI shell. Once core actors exist, the shell makes them visible.
6. **P2.4** — Inbox. The user's stated biggest pain; satisfying it early gives
   Kernl a "first use" that justifies the substrate.
7. **P2.1** — Notes module.
8. **P0.3** — Graph traversal/relevance. (Could come earlier if P2.1/P2.4 need
   relatedness queries.)
9. **P2.7** — Dispatch routing + autonomous mode.
10. **P2.5** — Ingest engine.
11. **P2.2** — Memory module.
12. **P2.3** — Bookmarks module.
13. **P3.x** — composed surfaces, graph view, insights, dashboard, additional
    shapes, Auditor stage, custom workflow authoring.
14. **P4.x** — DevEx and openness.
15. **P5.x** — optional, as appetite.

This is one recommended path. The actual order should be driven by **value
realized per project finished**, not by topological correctness alone.

---

## Notes on this list

- The "Workflow engine core" project (P1.2) is by far the largest. Its
  brainstorm session will need to decompose further (likely into 4-6 sub-
  projects of its own). Expect to spend two or three planning passes on it.
- Many wave-2 projects can be done in either order; the listed order is
  recommended for value realization, not enforced by dependencies.
- Re-evaluate this list after each project completes. Reality teaches things
  the brainstorm cannot.
