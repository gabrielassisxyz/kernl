# Kernl — Vision

> A living document. The "state of the art" of Kernl when complete — not a roadmap,
> not a spec. It defines what Kernl **is**, not when.

## Tagline

**The solo dev's cognitive substrate: your vault, your tasks, and your agents all
living in the same graph — where capture becomes plan, plan becomes parallel epic,
and execution writes back into your knowledge.**

## 1. What Kernl is

Kernl is a single, opinionated, local-first platform that unifies four kinds of
"brain stuff" a solo developer juggles every day — notes, tasks, agentic execution,
and personal memory — into one shared substrate (a typed knowledge graph) backed by
a plain markdown vault on the filesystem.

It is one piece of software, not a bundle of seven. The unifier is the substrate:
every module reads and writes the same graph, so a captured idea, a wiki note, a
project, a task, a bead, a session log, a bookmark, and a fact the DA learned about
you are all first-class nodes in the same world — queryable, linkable, time-travelable.

## 2. Job to be done

A solo developer building their own tools can **ideate** more than they can
**execute**. The bottleneck isn't ideas and it isn't planning — it is that executing
ideas requires juggling many parallel agent sessions, each at a different step, and
human bandwidth doesn't scale to that. Worse, the context required to direct, review,
and decide lives scattered across five tools (Obsidian, Jira/Linear, ChatGPT, an
orchestrator, a notes app) that don't share state.

Kernl exists to close that gap by making **judgment** the only thing the human does
and **everything else** (decomposition, execution in parallel, observability, memory)
something a unified substrate carries — so the cost of having one more idea is the
cost of writing it down, not the cost of managing the toolchain that turns it into
shipped work.

## 3. Target user

**Primary:** solo developers who build their own tools, use agents to accelerate
execution, and feel the daily pain of context fragmentation across five+ tools.
This is a thousands-strong audience, not a millions-strong one — but with an acute
problem and high tolerance for technical tooling. Kernl is designed for them first.

**Welcomed:** AI-native PKM pioneers and knowledge workers (researchers, writers,
founders) who want the substrate-unified experience and are willing to live in a
developer-shaped product. Their use cases inform the design but do not dictate it.

**Not chasing:** the broad Notion/Obsidian audience that demands ten years of UX
polish. Kernl can't and won't out-polish those tools; it wins on substrate
integration, not on chrome.

## 4. What Kernl deliberately does not do

These exclusions define the shape of Kernl as much as any feature does.

- **Not mobile-first.** Desktop Linux is the primary citizen. Inbox capture has a
  mobile path (webhooks, bots), but the full experience assumes a keyboard, two
  monitors, and a real terminal.
- **Not CI/CD platform.** Kernl invokes GitHub Actions, observes builds, reads PR
  state. It does not implement pipelines, builders, runners, or deployment systems.
- **Not RAG or internet-scale semantic search.** Kernl indexes **your** vault,
  **your** notes, **your** conversations. It does not ingest the web. When external
  information is needed, it delegates to external search/research tools
  (Tavily, SerpApi, ARIS-style) and ingests the result back.
- **Not a generic AgentOS.** Kernl orchestrates agents on top of a specific domain
  shape (beads + DAG + judgment gates). It is not a framework for arbitrary
  multi-agent patterns. It is not a runtime for arbitrary swarms.
- **Not multi-user, not collaborative, not SaaS.** Single-user, local-first,
  self-hosted. The schema carries optional `owner_id` and `visibility` fields
  invisibly so a future "Kernl-Cloud" product could exist as an alternate-world
  evolution — but multi-user is **not** a mode of the current Kernl and is not a
  design constraint on current decisions.

## 5. Differentiators (the five non-negotiables)

| Versus | Kernl's non-negotiable advantage |
|---|---|
| **Notion / Roam / Obsidian** (PKM) | Shared substrate with agentic execution. Notion's AI summarizes pages; Kernl's agents create beads/epics from notes and execute real code. Notion is summary; Kernl is a work machine. |
| **Jira / Linear** (project mgmt) | Personal tasks and agentic work live in the same graph. Linear has no concept of "run agents in parallel against the issues." Kernl is Linear plus a real executor of agents. |
| **ChatGPT / Claude desktop** (chat) | The DA carries a persistent substrate about you — versioned additive memory, vault, projects, decisions. ChatGPT forgets; Claude desktop starts from zero. Kernl brings substrate that enriches with use. |
| **PAI / Personal_AI Infrastructure** (life OS) | First-class agentic orchestration plus a rich GUI. PAI is text-first plus Claude Code; Kernl has dedicated UI to see/control epics, swarms, inbox, metrics. PAI is a skill collection; Kernl is an integrated product. |
| **foolery-go / gastown / NTM** (orchestrators) | A cognitive substrate underneath. Existing orchestrators execute beads well; none know your memory, notes, inbox, or decisions. Kernl is orchestrator + brain joined. |

## 6. The substrate

### 6.1 Form

The substrate is a typed knowledge graph. There is one graph, not one-per-module.

- **Nodes** are typed: `Bead`, `Project`, `Task`, `Note`, `Session`, `Decision`,
  `MemoryClaim`, `MemoryRefutation`, `Bookmark`, `BookmarkList`, `Capture`,
  `WorkflowRun`, etc. The set is **closed** — defined by Kernl, not expanded by users.
- **Edges** are typed: `depends_on`, `parent_of`, `inspired_by`, `mentions`,
  `generated_from`, `processed_into`, `refutes`, `relates_to`, etc.
- **Tags** are open. Users invent tags freely. The vault has no folders — only tags.
  UI may render tags as folder trees (Bear / tagfolder style).
- **Type custom by user:** the user can type their own note manually
  (via frontmatter or tag). If typed, the node carries that type. If not, it is a
  generic `user_note`. The DA **reuses** user-given types but **does not invent
  new types** to classify user notes.

### 6.2 Storage and source-of-truth split

| Surface | Truth | Cache |
|---|---|---|
| **User notes** (`.md` files) | Filesystem | SQLite |
| **All other nodes** (Bead, Task, Bookmark, Memory, etc.) | SQLite | — |

This is a deliberate split. Humans write in markdown; the filesystem owns those
bytes. Operational state (beads, tasks, sessions, decisions) lives only in SQLite.
**You can delete `~/.kernl/graph.db` and Kernl rebuilds the user-note half by
re-scanning the vault.** The operational half is gone — its backup is a contract,
not derivable.

### 6.3 Identity

Every user note carries a UUID in its frontmatter (auto-injected on first save). The
graph references notes by UUID, **not by path**. Path is a mutable cache. Therefore:

- `mv`, `git mv`, rename outside the Kernl UI — **links do not break.**
- Watcher detects the move and updates the path cache; the node identity is preserved.
- The vault is portable: zip it, send it elsewhere, Kernl can rebuild from it.

The frontmatter UUID can be visually hidden in the UI, but it is the truth.

### 6.4 Where the graph lives

SQLite (modernc.org/sqlite, pure Go) in `~/.kernl/graph.db`. WAL mode, ACID, FTS5
for full-text search, JSON1 for type-specific fields. No external dependencies.
SQLite is chosen on the merits — its tooling, FTS, JSON support, and operational
maturity beat embedded property graph DBs (KuzuDB, etc.) for a single-user system.
Triple stores (Oxigraph) and embedded graph DBs are noted alternatives if Kernl ever
needs native graph traversal at scale; today, SQL + recursive CTEs are sufficient.

## 7. The Digital Assistant (DA)

The DA is Kernl's persistent agent identity — the one that knows you, holds your
memory, and brokers requests across modules. Distinct from the **execution agents**
of the orchestrator (which are ephemeral, pool-fungible).

### 7.1 Scope and permissions

The DA's access to the graph is derived from where the user invokes it.

- Invoked inside the Projects view? Scope is `project + related`.
- Invoked in the global chat? Scope is `general`.
- Wants to read a node outside scope? **Permission prompt** in the UI:
  *"Can I read `<node>` to answer this? [accept / reject / always in this project]"*.

The pattern is borrowed from agent-CLI directory-permission prompts (Claude Code,
opencode, etc.) applied to personal memory. Solo-user, where operator and user are
the same person, makes this viable.

### 7.2 Writing to human notes

The DA **never** writes directly to a user's `.md` file without explicit approval.
When the user asks the DA to "format this note", "summarize this", or "extract key
points", the DA produces a **diff/patch** that appears in the UI for review.
Accept/reject/edit; only the user's action commits the change.

All notes — human-written and DA-written — carry a **revisions** log that auto-saves
diffs every 5 seconds. Revision author is recorded (`human`, `DA`, `agent:<id>`).
Regions written by the DA show a discreet ribbon in the UI (Google-Docs style).

### 7.3 Memory model

The DA writes to its own subgraph of `MemoryClaim` nodes, additively.

- Every fact the DA learns about the user is a node:
  `MemoryClaim { about, claim, source_node, ts, confidence }`.
- The DA does **not** rewrite past memory. If a claim is superseded, the DA creates
  a `MemoryRefutation` node edged to the old one. Nothing is deleted or overwritten.
- Re-summarization ("what do I know about GA?") is done **on demand** when answering;
  the result is not persisted over the originals.
- This is intentionally the opposite of PAI's `MEMORY/` reorganization model.
  Lossy rewrites are silent bugs; additive logs are auditable.

### 7.4 Ingest model

Two passes, never one continuous-and-invisible one.

- **Passive discovery (always on):** filesystem watcher sees a new `.md`, creates a
  `Note` node (default type: `user_note`), injects UUID if missing, indexes the body
  in FTS5. Zero LLM involvement. The DA always knows the note exists and can find it
  by keyword.
- **Active ingest (on demand):** the user (or the DA, when answering a question)
  triggers structured extraction — entities, concepts, type-specific fields,
  cross-links — using a manifest with content hashes to avoid re-processing (the
  pattern from claude-obsidian's `wiki-ingest`). Extracted output lives in the DA's
  subgraph (`vault-llm/`) and points back to the original via `processed_from` edge.

### 7.5 Async review system

When the LLM hits a decision it cannot make alone during ingest (unknown entity, name
collision, contradiction with existing claim), it does not guess. It emits a review
card in an async queue with **predefined actions** (Create Page, Deep Research, Skip,
Update, Add Contradiction Callout). The user opens the queue when ready. Actions are
constrained to prevent the LLM from inventing arbitrary operations.

### 7.6 Autonomous mode

A first-order setting (`kernl.yaml: interactive: false`, or `--autonomous` flag) that
silences confirmations and permission prompts. Used when running Kernl unattended
(cron jobs, background processing, trusted-inference flows). Modes:

- **Interactive (default):** every inferred decision asks for approval.
- **Autonomous:** infer-and-go; log everything; surface the log for after-the-fact
  review.

## 8. The Orchestrator

### 8.1 Stages of the vibe-coding canonical pipeline

This pipeline is the opinion Kernl ships and maintains. It is one of several workflow
shapes; the orchestrator infrastructure can run others. But this is the canonical one
— documented, instrumented, supported.

**Hot path (per epic):**

1. **Planner** — decomposes the epic into a DAG of beads with acceptance criteria
   (which include tests and docs). Optional `spec-write` operation for ambiguous epics.
2. **Implementer (fungible pool)** — pulls available beads, executes. Acceptance
   criteria include tests and docs; there is no separate Test-writer or Doc-writer
   stage.
3. **Per-bead Reviewer** — cross-agent (the implementer cannot review). Operations
   include code-review-llm, lint, test-run, security-scan (each configurable per
   epic/project).
4. **Merger** — Bors-style single-flight merge into the epic branch.
5. **Integration Reviewer** — reviews the merged whole after all children land.
   On reject, creates **fix-up beads** that re-enter the pipeline; does **not**
   reject the whole epic.
6. **Releaser** — versioning, changelog, final PR to main. **Not optional.**

**Continuous / on-demand:**

7. **Sweeper** — ambient work: stale-issue detection, worktree cleanup, bd↔git sync,
   zombie-agent reaping.
8. **Auditor (multi-mode)** — full-codebase analysis under different lenses: quality,
   security, performance, test coverage, docs completeness. Run on demand or scheduled.

### 8.2 Workflow shapes — first-class canonical vs DIY custom

Workflow shapes are declarative (YAML). Three classes:

- **Canonical (first-class):** maintained by the Kernl project. Ship with the binary.
  Documented, instrumented with the strategy metrics, debuggable by the DA out of the
  box, evolved by community/project PRs. The vibe-coding pipeline is the flagship.
  Other canonical shapes: `brainstorm-shape`, `research-shape`, `inbox-processing-shape`,
  `content-writing-shape`.
- **Custom (DIY):** users author YAML files in `.kernl/workflows/<name>.yaml`, versioned
  in their project's git. The DA can help author them (chat-driven or GUI builder).
  Run on the same runtime as canonical. **Trade-off:** the user is the maintainer.
  No docs, no metrics, no community support, no automatic upgrade when the project
  ships a new canonical version.
- **Community shared:** any DIY workflow can be exported as a package and shared via
  URL or repo. `kernl workflow import <url>` installs. This is the community surface;
  it is not a marketplace.

The substrate does not enforce that a "vibe-coding" workflow be the canonical one —
a user can author a `my-vibe-coding.yaml` without Integration Reviewer. They simply
own that decision and its consequences.

### 8.3 Dispatch routing

How does the orchestrator decide which workflow shape to apply to an epic?

- Epic carries a `workflow` field in metadata. If present, it is respected absolutely.
- If absent, the DA infers from the epic content (description, acceptance criteria,
  type) and proposes a shape. **The user confirms** before execution — unless running
  in **autonomous mode**, where the inference is logged and executed without prompt.
- `kernl epic create` defaults to `workflow: vibe-coding-pipeline` for software work.
  Other shapes are picked explicitly or by inference.

### 8.4 Coordinator: observational dominant + DA chat always accessible

Kernl is **dashboard-first**. The primary surface to interact with the orchestrator
is the GUI: see epics, swarm, metrics, inbox. Primary actions are UI-driven (click
"execute", "abort", "approve gate", "open fix-up bead").

The **DA chat is always accessible** via the contextual right sidebar. The user
**can** say *"DA, execute this epic"*, *"DA, what's the state of bead X?"*, *"DA, retry
this gate"* and the DA performs the action. This is supported and complete — but it
is not the dominant mode. The dashboard is. Implications:

- No "Mayor agent" persona. The DA serves the coordination function via chat.
- UI is the load-bearing surface for action; chat is the conversational one.
- Both reach the same underlying runtime; there is no surface that only one can use.

### 8.5 Per-bead loop and escalation

When a Per-bead Reviewer rejects, the bead returns to the pool with `review_feedback`
populated. Default escalation guards:

- `rejection_count >= N` (default 3) → escalate to human automatically.
- Recurrent pattern (same error class twice consecutively) → escalate before N.
- Time-budget exceeded → escalate.

Configurable per project. Integration Reviewer rejections produce **fix-up beads**,
not epic-level rejection.

## 9. The Inbox

Kernl absorbs the `quick-capture` project as the **Inbox** module, re-implemented in
Go to keep the stack coherent.

- **Capture** (CLI, hotkey, webhook, bot): creates a `Capture` node — atomic,
  sub-second, zero LLM. The user-visible promise is **under 5 seconds from intent
  to saved**.
- **Inbox view** in the GUI: list of unprocessed captures, with filter to see
  processed history.
- **Processing is opt-in**: the user triggers it (or cron polls "process inbox
  idle"). The DA proposes: *"this looks like a task / idea / reference / question"*
  and shows the suggestion in the inbox card with **predefined actions** —
  `Create Task`, `Create Note`, `Save Bookmark`, `Trigger Research`, `Discuss`,
  `Discard`. For questions, the DA can pre-run external research and present the
  results inline for the user's decision.
- **Lifecycle:** the original `Capture` node is preserved (it is the user's authored
  artifact). When processed, a new node is created in the appropriate subgraph
  (Task, Note in the DA's subgraph, Bookmark, etc.) and edged with
  `processed_into` from the original.
- **Rollups:** the DA generates daily/weekly summaries of inbox activity as
  markdown notes in its subgraph.

## 10. Notes and the vault

- Notes are markdown files on the filesystem. UTF-8, real `.md`, copyable, portable.
- The vault has **no folders** — only tags. UI may show tags as a folder-like tree
  (Bear / tagfolder pattern).
- Wikilinks `[[note name]]` are first-class and resolved by UUID-aware lookup. Rename
  does not break links.
- LLM-generated notes live in a separate directory (`vault-llm/`) but are nodes in
  the same graph. They always point back to source nodes via `generated_from` edges.
- Project notes: the project's `.md` file is 100% the user's writing. The GUI shows
  related material (tasks, sessions, related notes, bookmarks, beads) in surrounding
  panels — **not** by auto-populated regions inside the `.md`. The contract "what is
  in the file, the user owns" is sacred.

## 11. Bookmarks

A first-class node type with rich structure.

```yaml
Bookmark:
  id: uuid
  type: link | file | image
  url: string             # path for file/image
  title: string
  description: string
  fetched_at: ts
  archived_html: blob     # always present (link-rot defense, Karakeep style)
  archived_screenshot: blob?   # default ON for type=link
  tags: [string]
  notes: markdown         # user's general notes
  highlights:             # list, each independently noted
    - { id, range, text, color?, created_at,
        notes: markdown   # user's note on THIS highlight
      }
  content_markdown: text  # extracted by defuddle agent (see below)

BookmarkList:
  id: uuid
  name: string
  bookmarks: [bookmark_id]
  tags: [string]
  visibility: private | shareable (export-as-HTML)
```

Capture paths: browser extension, `kernl bookmark <url>` CLI, inbox processing, or
import (Pocket, Pinboard).

**Defuddle agent:** at enrichment time, the DA examines the `archived_html`,
identifies the relevant tags/classes/ids, and passes them as parameters to a
deterministic `defuddler` script (Python or Rust binary). The script extracts clean
markdown into `content_markdown`. The DA is the guide; the defuddler is the
mechanism. Cheaper and more deterministic than asking the LLM to extract directly.

## 12. GUI top-level

Kernl is a Vue 3 + Nuxt SPA served by the Go backend. Dark theme. The metaphor is
**sidebar of modules + main area + contextual right sidebar + composed surfaces**.

| Surface | Content |
|---|---|
| **Home** | A friendly "overview of the day". Today's tasks, active projects, alerts, quick capture. Not a dashboard. No metrics screen on the user's face. |
| **Dashboard** (dedicated tab) | Metrics and charts. Realized parallelism, epics/month, intervention counts, ingest stats, graph growth. For when you want trends. |
| **Sidebar / header** (top-level nav) | Active modules: Projects, Notes, Tasks, Orchestrator, Inbox, Bookmarks, Chat, Graph view, Dashboard. |
| **Right contextual sidebar** | Multi-purpose: DA chat scoped to the current surface, panel of selected project/task/bookmark, diff-review queue, related-items panel. |
| **Composed surfaces per module** | Opening a Project shows related notes, tasks, sessions, beads, bookmarks, wiki-logs in surrounding panels (Nexus-style). Each module surface is composed, not isolated. |
| **Graph view** (dedicated tab) | Visualization of the knowledge graph, Obsidian-shape but with better performance and UX. Used for exploration, not as home. |

## 13. Knowledge graph features (from llm_wiki, incorporated)

The substrate is more than storage. It exposes intelligence over itself:

- **4-signal relevance model**: relatedness between nodes ranked by direct link (×3.0),
  source overlap (×4.0), Adamic-Adar (×1.5), type affinity (×1.0). Powers
  "related notes", "find similar".
- **Louvain community detection**: automatically discovers clusters of knowledge
  without user tagging. Cohesion score per cluster; low-cohesion clusters are flagged.
- **Graph insights**: proactive surfacing of:
  - **Isolated nodes** (degree ≤ 1) — notes adrift.
  - **Sparse communities** (cohesion < 0.15) — areas with weak internal cross-references.
  - **Bridge nodes** (connecting 3+ clusters) — critical junction nodes.
  - **Surprising connections** — cross-community or cross-type links with high relevance.
- **Source folder auto-watch**: external file changes in the vault are detected and
  trigger the same ingest/delete lifecycle as in-app actions.

## 14. DevEx is a pillar, not an afterthought

Kernl is open source from day one. Setup friction kills open source. Therefore:

- **Setup wizard** — first-run guided experience. Sane defaults; minimal manual
  configuration. `kernl doctor` validates the environment.
- **Tour** — in-app guided walkthrough on first login. Explains the substrate, the
  vibe-coding pipeline, the DA, the inbox.
- **Specialized skill for LLM helpers** — a skill (compatible with Claude Code,
  opencode, Gemini CLI, Cursor, and similar agent CLIs) that knows Kernl deeply
  and can help any user install, configure, use, and debug it conversationally.
- **Video tutorials** — short, focused, by-feature. Not optional.
- **`kernl explain <thing>`** — a CLI command that explains any Kernl concept,
  command, or error in plain language, using the user's own state as examples.

This effort is non-negotiable. A solo dev who cannot get Kernl running in 5 minutes
will not become a contributor; an open-source project without contributors stays a
personal tool.

## 15. Modularity

Kernl is a single product, not a bundle. Modularity is delivered through
**view-toggling**, not through fragmented deployment.

- The binary is single. SQLite is single. The graph is single.
- A config (`enabled_modules: [orchestrator, projects, tasks, notes]`) hides
  inactive modules from the sidebar and disables their endpoints/hooks/sweepers.
- Data of disabled modules is preserved; only the surfaces vanish.
- This is **super-optional**: most users enable everything. The toggle exists so a
  user who only wants the orchestrator and project management is not visually
  burdened.

**Exception (deliberate):** the **Orchestrator can be installed and run standalone**,
without the rest of Kernl. This honors the orchestrator's heritage (foolery-go) and
serves users who only want the agentic execution piece — a major DevEx win for
newcomers exploring vibe coding without committing to the full Kernl.

## 16. Multi-user posture

Kernl is single-user. This is deliberate and non-negotiable for the product
identified above.

- The graph schema carries optional `owner_id` and `visibility` fields on nodes and
  edges. Invisible in the single-user UI. Their existence costs nothing now and
  preserves an option later.
- Conflict resolution, multi-master sync, sharing UI, auth, billing — **none** are
  designed for in the current Kernl. They would require redesign of the substrate
  and the entire product surface.
- If a future "Kernl-Cloud" exists, it is an **alternate-world evolution** with its
  own design conversation, not a mode toggle. This is not a promise — it is a
  freedom from carrying impossible trade-offs in current decisions.

## 17. The complete flow you can observe

A walkthrough of Kernl-when-done, in one continuous narrative:

> A thought strikes mid-day. You hit the hotkey, type four words into the floating
> terminal, hit Enter. Capture saved (<1s). You go back to what you were doing.
>
> An hour later, you open Kernl. The Home view shows today's tasks, your active
> epics, and three new inbox cards. You click one. It is your earlier capture. The
> DA's suggestion card is already there: *"this looks like a research question
> about RAFT consensus. I've drafted 4 search queries — want me to run them and
> ingest results into your knowledge base?"* You click *Trigger Research*.
>
> A `WorkflowRun` of `research-shape` starts in the background. The right sidebar
> tells you it is searching. You return to your current work.
>
> Your current work is an epic: *"add per-bead reviewer escalation thresholds to
> the orchestrator."* You open the Orchestrator tab. You see the epic's DAG. The
> Planner already decomposed it into 6 beads. 3 are running in parallel — you watch
> the swarm view as their bead-state changes. One bead's Per-bead Reviewer rejected
> twice; the third rejection just escalated and the bead now shows the escalation
> ribbon. You click it: the reviewer's feedback is in the panel. You read, write a
> directive in the DA chat: *"the test was right; the implementer misread the spec.
> Tell it to read §3.2 of the architecture doc again."* The DA writes the bead's
> follow-up context, releases it back to the pool. Within minutes a different
> implementer in the pool picks it up.
>
> Meanwhile, the Research workflow finished. A review card appeared: *"3 of 4 pages
> ingested; one source has a contradiction with [[raft-leader-election]] note —
> review?"* You open it. The DA's diff suggestion is clear; you accept it. The
> `[[raft-leader-election]]` note now has a `> [!contradiction]` callout with a
> citation to the new source.
>
> You write a quick note in the vault about what you learned. The DA's wiki ingest
> happens in the background — your new note is indexed for full-text immediately;
> structured extraction happens later when you ask. You move on.
>
> When the epic's last bead clears Per-bead Reviewer and Merger, the Integration
> Reviewer fires. It finds no issue. Releaser opens the PR to main, with a
> changelog entry the Releaser drafted. You review the PR — that is the one place
> you do touch — approve, merge. Kernl marks the epic done. The Dashboard tab
> updates: realized parallelism, time-to-ship, intervention count.
>
> At end of day you ask the DA: *"what did we learn about RAFT today and what
> changed in the codebase?"* It synthesizes from the graph — your research, your
> note, the merged PR, the bead history — and produces a one-paragraph summary
> with citations to every source. You save it as a daily rollup.
>
> Tomorrow it remembers, because the substrate did.

That is the product. Every piece in that walk-through is something Kernl explicitly
provides; none of it is delegated to another tool.

## 18. Parked decisions (open questions worth flagging)

These are deliberately not closed; they belong in sub-project brainstorms.

- **Vault file format options beyond markdown** — should `.canvas` files (Obsidian-style)
  be first-class? Other formats?
- **Browser extension scope** — full reading-mode capture, or just URL + title?
- **Webhook/bot transports for Inbox** — telegram, Slack, Discord, generic webhook?
- **Specific community workflow registry/marketplace mechanics** — out of MVP scope,
  but worth discussion when adoption justifies.
- **Strategy metrics instrumentation** — only "realized parallelism" is currently
  measured by the orchestrator core. Others (interventions out-of-gate, epics
  completed without manual rescue, ideas-to-epic-per-month) need definition and
  hooks.
- **Inbox processing scheduler** — purely on-demand, or with an idle-time auto-process
  option configurable per user?
- **Vector search over the substrate** — llm_wiki ships optional vector search via
  LanceDB. Adopting it is a benefit for "find semantically related"; rejecting it
  keeps the dependency surface smaller and forces relevance to come from the
  4-signal model plus FTS5. Decision deferred to the graph-tools sub-project.

## 19. References (the projects whose ideas are in Kernl's DNA)

- **foolery-go** / **gastown** — orchestration heritage; Mayor vs CLI debate;
  Refinery vs Integration Reviewer.
- **NTM** — fungible-generalist pattern; reference for tmux-based swarm.
- **bd (gastownhall/beads)** — issue tracker as the backend for beads; Dolt sync.
- **claude-obsidian** — the LLM Wiki pattern, ingest with manifest, hot cache.
- **llm_wiki (nashsu)** — knowledge graph relevance model (4-signal), Louvain
  community detection, graph insights, async review queue, source folder auto-watch.
- **ARIS** — auto-research-in-sleep pattern; Research Wiki as persistent knowledge
  base with relationship graph; meta-optimize.
- **PAI (Personal_AI Infrastructure)** — life OS framing; DA identity; ISA primitive;
  algorithm phases. Kernl deliberately departs from PAI's lossy memory-rewrite model.
- **Karakeep** — bookmarks with archived HTML, highlights, lists. Reference for
  bookmark UX.
- **Karpathy's LLM Wiki gist** — the foundational pattern.
- **Obsidian (closed source) + obsidian-api** — UUID-vs-path debate resolved by
  honest inspection of how Obsidian actually handles renames.

## 20. What this document is not

- Not a roadmap. There are no dates.
- Not a spec. Implementation details are out of scope here.
- Not a contract with users (Kernl has no users yet). It is a contract with future
  self about what Kernl is when it earns its name.
- Not immutable. It is the current best articulation. As decisions are refined or
  reality teaches, this document evolves.

---

*Living document. Last revised: 2026-05-16.*
