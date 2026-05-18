Ideate on the full architecture, feature set, and product vision for **Kernl** — a personal AI-native platform that unifies project management, knowledge management, multi-agent orchestration, and developer workflow tooling into a single modular system.

The project exists today as a small dashboard (~20 core files) and is being rewritten from scratch with a new stack. This is the moment to rethink everything before implementation begins.
Previously named `nexus` (repository can be seen at /home/gabriel/repositories/nexus).

## Decided constraints (treat as fixed)

- **Backend:** Go (chosen for determinism, LLM-friendliness, native concurrency via goroutines)
- **Frontend:** Vue 3 + Nuxt (progressive, template-based, Nuxt)
- **Database:** SQLite (local-first, personal tool)
- **CSS:** Tailwind v4 + DaisyUI v5, dark theme only
- **Real-time:** SSE or WebSocket
- **Architecture:** Modular / "lego blocks" — user picks which features to enable
- **Deployment:** Desktop Linux, 2x 4K monitors. Personal tool, single user, no auth
- **Data source of truth:** Open question — *not yet decided*. The original `nexus` design (Obsidian vault as source of truth, SQLite as cache, bidirectional sync with "vault wins") is **discarded**. Kernl will not work this way. The new persistence model is downstream of the orchestration núcleo and will be settled during that block's brainstorm.

For the full rationale behind the stack decision, see `docs/architecture-decisions.md` (ADR #9).

## Vision statement

Kernl is a **Jira + AI workflow orchestrator + Obsidian + Todoist + Chat interface + LLM Wiki**, unified in one platform. The user describes it as:

> "An Obsidian (digital garden) + LLM Wiki + autonomous multi-agent LLM orchestrator + task/project manager"

> "Kernl + wiki + orchestration + beads + notes + tasks + bookmark manager; open source and modular, you choose the features you want"

## The núcleo — orchestration (bloco zero)

Of all the feature areas below, **multi-agent orchestration is the núcleo**: the block built first, because it is the block that builds everything else (kernl develops kernl). It is not abstract — the concrete starting point is **`foolery-go`** (`~/repositories/_cloned/foolery-go`), an in-progress Go port of [`foolery`](https://github.com/acartine/foolery) (a keyboard-first control room for multi-agent software work, originally TS/Next.js, backed by Knots/Beads). The plan is to modify and integrate `foolery-go` as Kernl's orchestration core.

Immediate driver: the user currently has **no orchestration setup/workflow defined**, which is a hard blocker on developing anything at all. Unblocking this is the first priority.

**The other feature areas are NOT discarded.** They are blocks in a prioritized backlog, each with its own boundary, built one at a time on top of the núcleo. Sequencing is not abandonment — the modular "lego blocks" architecture exists precisely so nothing has to be dropped.

## Core feature areas to ideate on

### 1. Multi-agent orchestration engine — *the núcleo (see above)*

- Pipeline visualization: parallel and sequential execution of "beads" (atomic agent tasks) and "epics" (grouped beads)
- Visual representation of what's running, what's queued, what's blocked, what finished
- Full visibility and transparency of what's happening under the hood — orchestrator state, subagents, parallelism. This was the biggest pain point with a previous tool ("gastown")
- Agent notifications: tasks to review, input needed, PR opened, errors, completion
- GUI to configure different agent harnesses with presets (OpenCode, Claude Code, Codex, etc.)
- Work on multiple projects simultaneously
- Maybe someday: 2D pixel art visualization of a "gastown" with little workers (aspirational, low priority)

### 2. Project & task management

- Grid of cards by status (Active, Queue, Spark, Backlog, Dropped, Done) — not Kanban, card grid
- Slide-in panel with project details, sessions, tasks, log, backlog
- Drag-and-drop between status sections
- Frictionless project creation with workflow presets based on what you want to create (4 types of ideas for planning and brainstorming)
- Easy problem reporting
- Session tracking (work sessions linked to projects)
- Stalled project alerts (>14 days without activity)
- Task management with status cycling (backlog -> in-progress -> done)

### 3. Knowledge management / LLM Wiki

- Digital garden / Personal vault, like if Obsidian were a feature inside a product. (Something similar was already created and can be reused as the foundation: https://github.com/akitaonrails/FrankMD)
- Wikilinks, callouts, same visual language as Obsidian
- Browse and search across vault content
- Bookmark manager integrated into the knowledge base (Karakeep ref: https://github.com/karakeep-app/karakeep)

### 4. Chat interface

- Integrated chat for interacting with LLMs
- Context-aware — knows about your projects, tasks, vault content
- Streaming token display

### 5. Workflow engine & presets

- Predefined workflow presets: planning loops, brainstorming, idea validation, implementation, etc.
- GUI to help you build custom workflows with an AI "guide" that discusses available options, existing workflows, skills, etc.
- Workflow presets for project creation — pick the type of thing you're building and get a tailored pipeline
- Processes should be well-defined and documented

### 6. Integration & extensibility

- Beads + MCP integration (including MCP mail?)
- Seamless integration without needing to debug 5 separate things
- Possibility of native LiteLLM wrapper/integration inside the platform (instead of forking/patching external tools)
- Or: port/copy external tools with only the features needed — following the growing trend of hyper-specialized, hyper-personalized "in-house" software
- Integration tests with tools that required forks/manual patches (LiteLLM, etc.) — must not break the project when upstream updates
- Sandbox-clean setup: minimal global configs (skills, plugins, hooks assembled like lego blocks as needed)

### 7. Observability & security

- Observability + telemetry across the platform
- "Extensive release validation" — especially security audits
- Transparent pipeline state at all times

## What exists today (v0.2.0)

The current implementation has these working features that should be preserved or evolved:

- Card grid with status sections, drag-and-drop, task progress bars, days since last activity
- Slide-in sidebar with sessions, tasks, log, editable backlog with hierarchy
- Session modal with detail view
- Ctrl+K search overlay
- Stalled project alerts
- Backend REST API (CRUD for projects, sessions, tasks, captures)
- Bidirectional sync: Obsidian vault <-> SQLite <-> Frontend *(NOTE: this sync architecture is **discarded** — see Decided constraints. Listed here only as historical record of what nexus did.)*
- Capture system (ideas/tasks/references/questions in Kanban-style grid)
- LLM enrichment endpoint for captures
- OpenCode session launcher

Current architecture: `docs/architecture.md`
Current features spec: `docs/features.md`
Domain glossary: `docs/glossary.md`

## Reference: Daniel Miessler's PAI

The Personal AI Infrastructure (https://github.com/danielmiessler/Personal_AI_Infrastructure) is a related project worth understanding. PAI is a "Life Operating System" layer on top of Claude Code with:

- 7-phase algorithm: OBSERVE -> THINK -> PLAN -> BUILD -> EXECUTE -> VERIFY -> LEARN
- 45 skills, 171 workflows, 37 lifecycle hooks
- Structured memory (work, knowledge, learning, relationship, observability, state)
- ISA/ISC primitives (Ideal State Artifacts / Criteria) as universal PRDs
- Pulse daemon with 22 routes, voice, dashboard, Telegram/iMessage bridges
- Philosophy: text over opaque storage, filesystem as context (no RAG), context scaffolding > model choice

PAI is the "brain" (orchestration, memory, hooks). Kernl would be the "interface" (dashboard, pipeline visualization, chat, notifications). They can coexist — Kernl consumes what PAI-like systems produce.

## Meta-skills analysis

The user also wants to analyze "meta-skills" from Kim (co-author vibe coding) and understand how they fit into the Kernl ecosystem. This is about understanding what patterns/skills make AI-assisted development effective and potentially integrating those insights.

## Project renaming

The project will be renamed. "Kernl" is a working title. Ideation on naming is welcome as part of the broader vision work.

## Focus hint

Go deep. This is a reimagination of the entire project, not incremental improvement. Think about how all these pieces fit together as a coherent system, what the right module boundaries are, what should be built first vs. later, and what the user experience should feel like when everything works together. The modular "lego blocks" architecture is key — the user wants to pick features like plugins, not get a monolith.
