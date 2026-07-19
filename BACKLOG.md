# Backlog

Two sections, deliberately separate:

- **`## Tasks`** — planned, drainable dev work: things decided and ready to pull and implement. This is the ready-queue.
- **`## Deferred`** — work we consciously decided NOT to do now, kept so it is not forgotten. Each entry records what it is, why it was deferred, and any dependency that must land first.

Parked *ideas* (maybe-someday, not committed) live in [`IDEAS.md`](IDEAS.md), not here. The full capture → classify → plan → drain flow is documented in `llm-workflow/planning-pipeline.md`. **Backend for this project: markdown** (this file + `IDEAS.md`) — a test round of the pipeline; may switch to `br` beads later via a mechanical `beads-workflow` conversion. (The product's `bd`/orchestrator store is a separate, untouched concern.)

## Tasks

Planned dev work, ready to pull. (Mirrors the kernl-tagged items in the orchestrator inbox; this file is the markdown source of truth during the test.)

### Public-readiness pass: bootstrap top-up, file tree, and what the repo exposes

The repo is public but was never prepared for someone who is not the author. This is one unit of work covering four things that are all the same defect — the repo assumes the machine and the person that built it.

- **The spec is not in the clone.** `AGENTS.md` and `CLAUDE.md` are git-ignored (`.gitignore:58,60`). A contributor gets a repo with no conventions and no "Common hurdles" — and so does *any* agent working in a worktree, since `git worktree add` materializes only tracked files. Verified on 2026-07-18: a freshly created worktree has no `AGENTS.md`, and two other worktrees open at the time had none either. Every trap documented in those 260 lines is invisible to the agent standing in the worktree, which makes a documented hurdle behave exactly like an undocumented one.
- **Committing it requires depersonalising it first.** The current file is written in the first person to a specific person: *"a solo-dev tool — one user, me, now"*, *"call me out"*, *"I want to be contested, not obeyed"*, *"if I forget to commit closed work, remind me"*, plus `~/.claude/projects/-home-gabriel-repositories-kernl/memory/` as an absolute path. Two different fixes: identity and absolute paths get removed; the maintainer-workflow parts move to a git-ignored `local/` rather than being reworded.
- **Gates that are advertised but absent or silent.** `bin/install-hooks` exists while the repo has no `.githooks/` — verify what that script installs, or it is a hook nobody has. `bin/ci` skips `golangci-lint` silently when the binary is missing, so a run can be green with the linter never having executed.
- **File-tree and exposure review.** What sits at the root, what is generated, what should never have been tracked. The `public-repo` skill covers this; it has not been run here.

**Why now:** a 2026-07-18 audit across 15 repos found this repo has the heaviest personal content and one of only two untracked specs. Full findings and the proposed fix per repo: `llm-workflow/improve/reports/2026-07-18-hurdles-audit.md`.

**Not in scope:** promoting the repo's `Common hurdles` bullets into `bin/ci` gates or tripwire hooks. That work is driven centrally from the audit above.

### Add a delete-task button + API
`PATCH /api/tasks` accepts only `status`/`tags`/`dueDate` and there is no `DELETE` at all — so a task can be neither deleted nor retitled, by API or UI. For a task manager this is a bug, not backlog. (Moved from `llm-workflow/BACKLOG.md` P1.)

**Planned 2026-07-17 (Gabriel: include both delete AND retitle, mirror how `project` does it).** Most plumbing already exists — `nodes.DeleteTask` is written and `project` is a near-identical sibling that already ships full delete + retitle. This is wiring, not new infrastructure. **Parallel-safe** with the inbox auto-classify task below (disjoint file sets).

- **Goal:** a task can be deleted and retitled via API + UI, mirroring `project`.
- **Anti-goal:** no cascade-delete of what a task points to; no generic node-edit framework; no undo/soft-delete UI — the store already writes a tombstone revision on delete, so history is preserved and that is enough.

**Backend**
- `internal/graph/nodes/task.go`: add `SetTaskTitle` mirroring `UpdateProjectMeta` (`internal/graph/nodes/project.go:140`), scoped to `type = 'task'`.
- `internal/api/tasks.go` `patchTaskHandler` (`:146`): add `Title *string` to the body struct (`:155`); **include `Title` in the all-nil guard** (`:164`) or a title-only patch is wrongly rejected — mirror `patchProjectHandler` (`internal/api/projects.go:174`); apply via `SetTaskTitle` inside the existing `DoWrite` tx.
- `internal/api/tasks.go`: add `deleteTaskHandler` mirroring `deleteProjectHandler` (`internal/api/projects.go:242`): in one `DoWrite` tx, find the companion note via the companion edge, capture its path from `note_paths`, `nodes.DeleteNote`, delete the `note_paths` row, then `nodes.DeleteTask` (a task has no children to cascade); map `graph.ErrNotFound`→404; after commit, best-effort `os.Remove` the companion file; respond **204**.
- Register `DELETE /api/tasks/{id}` in `RegisterTaskRoutes` (`internal/api/tasks.go:38`).

**Frontend**
- `web/composables/useTasks.ts`: add `update(id, { title })` (PATCH) and `remove(id)` (DELETE), mirroring `web/composables/useProjects.ts:75` / `:88`.
- `web/components/tasks/TaskDetail.vue`: make the header `<h2>{{ task.title }}</h2>` (`:27`) inline-editable, emitting a new `set-title`; add a delete button (header next to close `:40`, or footer `:92`) emitting `delete`.
- `web/pages/tasks.vue`: add `changeTitle` + `removeTask` handlers next to `changeStatus` (`:197`) / `changeDueDate` (`:204`); wire `@set-title` / `@delete` on `<TaskDetail>` (`:96`). `removeTask` clears `selected.value` and calls `reload()`.

**Tests** (`internal/api/tasks_crud_test.go`)
- Add a `deleteTaskViaAPI` helper mirroring `patchTaskViaAPI` (`:27`).
- `TestDeleteTaskRemovesCompanion` mirroring `TestDeleteProjectRemovesCompanion` (`internal/api/projects_crud_test.go:183`): create via API, assert the companion file exists, `DELETE`, assert 204 + empty list + companion file gone + zero live note nodes, then a second `DELETE` → 404.
- A retitle round-trip test, and an assertion that a title-only `PATCH` is **not** rejected.

**Verify:** `go test ./internal/api/... ./internal/graph/nodes/...` green with the new tests; then `run.sh` and drive the UI — create a task, rename it, delete it; confirm it leaves the board **and** the companion note file is gone from the vault.

### Build the approval gate the orchestrator is described around

`internal/api/approvals.go` is 27 lines that ignore every request and return `[]` / `{}`. All three routes are stubs: `GET /api/approvals`, `POST /api/approvals/{id}/actions`, and the terminal-scoped `POST /api/terminal/{sessionId}/approvals/{approvalId}`. So the gate the project is described around — the human touches only judgment gates, the rest runs unattended — has a UI and a CLI in front of nothing.

`kernl approval list` and `approval resolve` exist, are tested, and will keep returning empty until this is built.

**Decisions to settle before implementing, not during:**
- **The action vocabulary is currently two vocabularies.** The web UI posts `{"action":"approve"|"reject"}`; `internal/terminal` defines `accept`, `always_approve` and `decline`. One of them has to win, or the split has to become deliberate and documented.
- **Where approval state lives.** `ApprovalRegistry` holds it in the process today, which loses every pending gate on restart — acceptable or not is a product call.
- **What a gate carries.** No comment/reason field exists anywhere in the contract, so a rejection cannot currently say why.

**Also wrong, and cheap to fix alongside:** `ApprovalRegistry.ApplyAction` returns nil for an unknown id — a silent no-op, against the Fail Loud rule.

**Verify:** a pending gate raised by a real bead run appears in `GET /api/approvals`, in the UI, and in `kernl approval list`; resolving it from any of the three unblocks the run; the decision survives a server restart if persistence is chosen.

### Add a field that lets a task be automatically developed by the orchestrator
A per-task flag marking it as auto-developable, so the orchestrator can pick it up and drive it — the first concrete step toward developing kernl inside kernl.

### Review/redo kernl's UI — sidebar and palette
The sidebar (icons + logo) and the color palette.

### Batch override of auto-classify in the inbox
A checkbox to toggle whether auto-classify runs, plus a button to trigger the classifier on the currently-unclassified inbox items.

**Planned 2026-07-17 (Gabriel: runtime flag, default ON, no persistence across restart; the checkbox lives in the batch "Review batch" modal).** Auto-classify is a server-side background loop (started once at boot in `cmd/kernl/serve.go:150`, gated only on `cfg.LLM.IsSet()`), so a UI checkbox needs a live server-side switch the loop reads each tick — a client-only toggle cannot stop it. **Parallel-safe** with the delete-task task above (disjoint file sets).

- **Goal:** stop the background auto-classify loop from running, and trigger a one-shot classify pass over the unclassified `pending` captures on demand.
- **Anti-goal:** not a boot-only config that needs a restart; no settings-persistence system — the toggle is session-live and resets to the config default on restart; no per-item classify controls.
- **"Unclassified" is:** a `pending`-tagged capture with empty `SuggestedActions` — already how `classifier.processPending` selects the set (`internal/inbox/classifier.go:74`).

**Backend**
- `internal/inbox/classifier.go`: export an on-demand pass — `ClassifyPending` wrapping today's unexported `processPending` (`:62`) — so a handler can run exactly one pass; the `Run` loop (`:45`) gates each tick on a live "enabled" flag instead of running unconditionally.
- `internal/app/app.go` (`type App`, `:22`): add a runtime `autoClassify bool` guarded by an `RWMutex` (No Shared Mutable State) with getter/setter; also hold what the on-demand handler needs to run a pass (the LLM, or the `Classifier` itself — today it lives only inside the `serve.go` goroutine and is **not** on `App`, so this wiring is required).
- `internal/config/config.go` `InboxConfig` (`:111`): add `AutoClassify bool` (`auto_classify` yaml, **default true**); read it at the `cmd/kernl/serve.go:150` gate to seed the runtime flag. The loop still starts; it just gates each tick on the flag.
- `internal/api/inbox.go`: add `GET` + `PUT /api/inbox/auto-classify` (read / set the runtime flag) and `POST /api/inbox/classify` (run one `ClassifyPending` pass over the unclassified set); register in `RegisterInboxRoutes` (`:24`). **Fail loud** if the LLM is unset — a `KERNL DISPATCH FAILURE`-style error naming the missing config, never a silent no-op.

**Frontend**
- `web/components/inbox/InboxBatchDump.vue`: add an **"Auto-classify" checkbox** inside the "Review batch" `UiModal` (`:19`) — `GET /api/inbox/auto-classify` on open to reflect the current state, `PUT` on change. Default renders ON.
- `web/pages/inbox.vue`: add a **"Classify unclassified now"** button in the unprocessed-tab toolbar (the right-side action cluster next to Focus, `:13`), shown when unclassified `pending` items exist; on click `POST /api/inbox/classify` then `refresh()`. Disable + explain when there are no unclassified items or no LLM configured.

**Tests**
- `internal/inbox/classifier_test.go`: a test for the on-demand `ClassifyPending` pass (reuse the `mockLLM` + temp-graph harness of `TestClassifier`, `:23`), and a test that the `Run` loop does nothing while the flag is disabled.
- `internal/api/inbox_test.go`: cover `GET` / `PUT /api/inbox/auto-classify` and `POST /api/inbox/classify`.

**Verify:** `go test ./internal/inbox/... ./internal/api/...` green; then `run.sh` and drive the UI — paste a batch with auto-classify **OFF**, confirm the created captures stay unclassified (no background suggestions appear); click "Classify now", confirm the suggestions appear; toggle back ON.

### Ingest review resolution: refuse an empty action, and stop calling a delete "skip"

Resolving an ingest review with no action permanently destroys it. `ResolveReview` (`internal/ingest/resolve.go:74`) treats `""` and `"Skip"` the same and both run `DeleteIngestReview` — so a `POST /api/ingest/queue/{id}/resolve` with an empty body discards the review with no confirmation, and `kernl ingest queue resolve <id>` with no `--action` inherits that. The CLI default is *deliberate* and carries a comment (`cmd/kernl/ingest.go:366`) explaining that inventing a different default would make the CLI resolve reviews differently from the API it is a client of. That reasoning is sound, which is why the fix belongs in the API, not the CLI.

Two defects, one root: an action that means "discard permanently" is both the **implicit default** and named **`skip`**, a word that reads as "leave it for later".

**Decided 2026-07-19 (both parts together):**
- **Refuse the empty action at the API.** `ResolveReview` stops accepting `""` as `Skip` and returns a validation error; the handler (`internal/api/ingest.go:342`) maps it to **400**. `ingestResolveAction` then requires `--action` and exits **2** with the valid-token list it already builds. One decision, one place, CLI and API back in agreement.
- **Rename the CLI token `skip` → `discard`**, no alias, no deprecation window. The wire value stays `"Skip"` — only the shell-facing token changes, via the translation map that already exists for `create-page` → `"Create Page"` (`ingestResolveActions`, `cmd/kernl/ingest.go:21`). Renaming the wire value is a separate wire-contract change and is **not** in scope here.

**Why breaking the default is safe:** the web UI never takes this path — `resolveAction` (`web/pages/ingest.vue:232`) always sends an explicit action. The empty-action path is reachable only from the CLI or a direct API call, so there is no consumer to deprecate for.

**Anti-goal:** no soft-delete or undo for reviews; no change to what `Create Page` / `Update` do; no rename of the `"Skip"` wire value.

**Tests:** `ResolveReview` with `""` returns the validation error and leaves the review in place (today it deletes it); the handler answers 400; a regression tripwire asserting `kernl ingest queue resolve <id>` with no `--action` exits 2 and writes nothing.

**Verify:** create a review, call resolve with an empty body — the review is still in `kernl ingest queue list`. Then `--action discard` removes it.

### Creation output leads with an opaque UUID instead of the thing you just named

Graph node ids are bare **UUIDv7** — `createNode` calls `ids.New()` (`internal/graph/nodes/chokepoint.go:48`), which is `uuid.NewV7().String()`. There is no short human-readable prefix; that convention belongs to bd bead ids, a different store. So a 36-character opaque string carries no information a reader can check, and every creation message currently opens with one:

- `task create` → `Created task <uuid>` (`cmd/kernl/task.go:151`) — **no title at all**
- `project create` → `Created project <uuid> — <title>` (`cmd/kernl/project.go:177`)
- `memory add-claim` → `Added claim <uuid> under <subject>` (`cmd/kernl/memory.go:225`)
- `inbox batch` → `Created batch <uuid> with N capture(s)` (`cmd/kernl/inbox_batch.go:167`)

`task create` is the worst of the four because it is also the verb that *constructs* its title: unquoted positional args are joined into one string (`taskCreateBody`, `cmd/kernl/task.go:258`). The comment defending that join says the slip is harmless because "the title is echoed back on success anyway" — **it is not**; that verb echoes only the id. The join's stated safety net does not exist.

`note write` already gets this right (`Wrote <path> (N bytes)`, `cmd/kernl/note.go:273`): where the identity is human-readable, the code already leads with it.

**Decided 2026-07-19 — one format across the four:**

    Created task "Fix the login bug" (01912f3e-7c4a-7b3d-9f21-4e8a1c2b5d70)

- **Title first, quoted.** The quotes delimit where the title starts and ends, which is precisely what needs checking when a positional join built it — unquoted, a swallowed word is invisible.
- **Id last, in parentheses.** Still there to copy, out of the reading path.
- **One line**, so `grep`/`awk` keep working. Structured consumers use `--json`, which does not change.

**Also decided, same unit of work:** add `--title` to `task create`, mirroring `project create` (`cmd/kernl/project.go:39`). The join exists because the verb has no unambiguous alternative — `task create` is positional-only today. With `--title` present and the title echoed back, the join costs nothing and stays as interactive forgiveness.

**Not in scope:** `bead create` (`cmd/kernl/bead_api.go:214`) keeps leading with its id — bd ids are short and readable, and beads are the orchestrator's store, not the graph. Making `task create` strict like `project create` was rejected: completing the verb is the fix, and strictness would only be reachable *after* `--title` exists anyway.

**Tests:** `task create` with unquoted multi-word input echoes the joined title in quotes; `--title` and a positional title together are rejected (mirror `projectCreateTitle`, `cmd/kernl/project.go:188`); a golden-output assertion per verb for the new line shape.

### Keep the CLI in step when the ingest screen is reworked

The ingest CLI and the ingest UI are two clients of one contract, and the CLI is the one that gets forgotten — it has no screen to look wrong on. Whenever the ingest flow or its screen is reworked, re-check the CLI surface in the same unit of work: the action vocabulary (`ingestResolveActions`), what `queue list` shows, and the help/`capabilities` text that describes them. A UI-only change that adds or renames a resolution action leaves `kernl ingest queue resolve` silently unable to express it.

Also pending on that rework, from the agent-ergonomics pass: `ingest queue list` still emits PascalCase against the camelCase wire contract (R2-007b, in `## Deferred`) and the third-level `ingest queue *` subcommands are invisible to `capabilities --json`. Both are cheapest to fix while the route is already open.

### Add a way to organize and categorize projects
Some structure for grouping/categorizing projects.

### Add an inbox filter by classification
Filter the inbox by classification.

### A backlog/deferred section in the UI, separate from tasks
Surface a backlog/deferred area distinct from active tasks (the product-side mirror of this file's Tasks-vs-Deferred split).

### Populate kernl's Memory with the telos
There is no `telos`-tagged note in `~/vault`, so the DA knows *what* exists and stays blind to *why* (U7 of the v0.1.0 plan was never populated). (Moved from `llm-workflow/BACKLOG.md`.)

**Update 2026-07-18:** superseded by step 5 of the DA absorption track (below) — about-me lands in the vault as tagged notes (`telos` among them), so populating the tag becomes part of that migration rather than a standalone chore.

### Give the DA the context this repo's agents already have
Carry over context *for the DA inside kernl* — logs, lessons, backlog, about-me — as two problems that must NOT share one solution:
- **Constitutional (always-on):** who Gabriel is and what he aims at. This is what the `telos` tag + `MaxTelosBytes = 4000` cap was built for. The work is **curation** (distil `about-me/` to earn its 4 KB), not raising the cap.
- **Situational (on demand):** lessons, backlog, ops logs, session history — **retrieval, not injection**. Kernl already has `Classifier.relatedContext` (`internal/inbox/classifier.go`); the DA needs the equivalent tool.

Decided (2026-07-14): **llm-workflow stays the source of truth**. A symlink into the vault is dead (`WalkDir`/`fsnotify` don't follow links; `LoadTelos` filters on a `tags: [telos]` frontmatter that `about-me/*.md` lack). So build a **one-way syncer (llm-workflow → kernl vault)** that stamps the frontmatter and curates to the 4 KB cap — same shape as `sync-machine-rule.sh`. Do not let it grow into a "sync everything" script. Code refs: `internal/chat/engine.go`, `internal/planning/telos.go`. (Moved from `llm-workflow/BACKLOG.md` S10.)

**Revised 2026-07-18 (supersedes the 2026-07-14 decision):** the one-way syncer is dead; **about-me moves into the vault** as tagged notes — `telos`, `identity`, `working-style`, `environment`, one tag per constitutional file — because the consumers differ by nature: `kernl plan` wants the why-chain (telos proper), a DA conversation wants identity + working-style, machine work wants environment. Injection becomes per-tag, per-consumer, and **distillation becomes a tuning knob, not a rule**: the 4 KB `MaxTelosBytes` cap was a fence for the current raw chat-completions engine (system prompt re-sent every turn, small proxy models), not a property of the problem — a surface backed by an agent harness takes the full notes. The two premises that forced the 2026-07-14 decision are being removed on purpose: (a) a **first-class injectable field** replaces the symlink/frontmatter hack, and (b) a **versioned vault** (track step 3) preserves the git history that made llm-workflow the safer home. Prerequisite: track step 3; executed as track step 5. When executed, llm-workflow's `AGENTS.md` settled decision #1 (about-me storage) must be updated in the same session.

### Switch the model / harness from inside the DA chat

**Captured 2026-07-17 (Gabriel).** Add a model/harness picker to the DA chat — the same affordance the web UIs of ChatGPT, Claude, and Gemini have: a control in the chat that swaps which model (and, more broadly, which **harness**) answers, without leaving the conversation.

- **Goal:** pick the backing model/harness per DA conversation from the chat surface.
- **Two axes, don't conflate them:** *model* (e.g. which LLM behind the same in-app DA engine) vs *harness* (the DA answered by the in-app engine vs. by an external agent harness like Claude Code — see the linking task below). Decide whether v1 does just model-swap or the full harness-swap; the harness axis is what unlocks the next task.
- **Groundwork already present:** the DA panel has a static `v2.4-stable` version badge and a `scope · global` chip today (mocked — see "DA panel — version badge, scope, and context chips" under *UI chrome currently mocked*). The model/harness picker is the real control those chips were gesturing at; wire them together rather than adding a parallel widget.

**Update 2026-07-18:** the harness axis is decided — it lands as wiring the chat surface to the existing `internal/adapter` (which already builds non-interactive `claude -p` stream-json invocations, permission bridge included); see step 6 of the DA absorption track. The `kernl da` CLI path does **not** depend on this item.

### A dedicated, full-screen page for the DA

**Captured 2026-07-17 (Gabriel).** Today the DA lives in a side panel (`web/components/DaChatSurface.vue`). Add a **dedicated full-screen DA page** — a route where the DA is the whole surface, not a docked panel — for longer, focused sessions.

- **Goal:** a full-screen DA route (its own page), reusing the existing chat surface component so panel and page stay in sync.
- **Open:** whether the panel becomes an entry point that "expands" into the page, or the page is a peer route; and how the model/harness picker and context chips render with the extra room. Not designed yet — capture only.

### When the DA's harness is Claude, link the session to llm-workflow's context

**Captured 2026-07-17 (Gabriel). Decided 2026-07-18: lands as the `kernl da` CLI subcommand — no longer depends on the model/harness switcher.** When the harness picked for the DA is **Claude** (i.e. an actual Claude Code / agent session, not the in-app LLM engine), that session should run **as if launched from inside the `llm-workflow` repo** — carrying all the context that repo's harness already injects (`AGENTS.md`, `about-me/`, ops logs, lessons, backlog) — **but** also told the things that make the DA the DA: that it is running **inside kernl**, and **which kernl tools/APIs are available to it**.

- **The intent:** stop the two worlds being separate. The rich agent context Gabriel gets from a terminal session in `llm-workflow` and the DA's situational awareness of kernl should be the *same* session when he drives the DA with the Claude harness.
- **Relation to the existing syncer task ("Give the DA the context this repo's agents already have"):** that task is about the **in-app DA engine** — curating `about-me/` → a `telos`-tagged vault note (constitutional) plus a retrieval tool (situational). *This* task is about the **external Claude harness** being pointed at llm-workflow's context directly. Related, not the same — do not collapse them; one feeds the built-in DA, the other bridges to a real agent CLI session.
- **Decided mechanism (2026-07-18) — `kernl da` v1:** an interactive wrapper around `claude`, launchable from **any directory**, that reproduces the llm-workflow session experience **exactly** — that is the requirement, not an approximation of it. It anchors `cwd` at `~/repositories/llm-workflow` regardless of the launch dir (cwd drives hooks, CLAUDE.md, skills, and ai-memory scoping, so anchoring guarantees byte-identical behavior where injected settings would only approximate it), passes the launch dir via `--add-dir`, and appends a kernl preamble: "you are the DA; you run inside kernl; drive it through the kernl CLI". The preamble **never hand-describes the CLI surface** (hand-written copies drift) — it defers to `kernl --help` and per-subcommand help, which is why the agent-ergonomics pass on the CLI (track step 1) comes first. Reminders keep working unchanged: llm-workflow's SessionStart hook fires because cwd is anchored there — they stay a hook until a kernl-native mechanism reproduces "arrive in the environment where the action happens". Kernl's APIs reach the session through the CLI (direct DB writes, shared `internal/*` packages); `kernl da` must **not** require `kernl serve` to be up.

### A complete, agent-oriented CLI for kernl

**Captured 2026-07-17 (Gabriel).** Grow kernl's CLI (`cmd/kernl/` today has `serve`, `bookmark`, `task`, …) into a **complete CLI aimed at agent use** — the surface an agent (or Gabriel from a terminal) drives kernl through, covering the graph: captures/inbox, notes, tasks, projects, bookmarks, memory, and the DA.

- **The reference points he named:** **`gt`** (Graphite / Gastown), the **Obsidian CLI**, and Notion's newer CLI — agent-ergonomic CLIs with robot/JSON output, scriptable subcommands, and clean help/errors. The `llm-workflow` skill **`agent-ergonomics-and-intuitiveness-maximization-for-cli-tools`** is the rubric to build against.
- **Why it matters:** an agent-first CLI is how the orchestrator and external agents operate kernl headlessly; it also underpins the Claude-harness linking task above (that session needs kernl tools, and a good CLI is the cheapest way to expose them).
- **Open (capture only):** scope of v1 (which subcommands first — inbox/capture and tasks are the daily surface), and the JSON/robot-mode contract. Greenfield-ish and large — this is a committed direction, not a scoped plan yet; it wants a planning round before draining.

**Update 2026-07-18:** sequenced as **step 1 of the DA absorption track** (below) — the ergonomics pass happens *before* `kernl da` is built, so the DA's preamble points at a stable, agent-grade surface from day one instead of chasing CLI changes. Method: run the `agent-ergonomics-and-intuitiveness-maximization-for- cli-tools` skill against the CLI. Scope additions: keep the CLI **direct-to-DB** (no `serve` dependency — logic is already shared with the server via `internal/*` packages, so there is no duplicated-logic risk), and verify how a running server notices direct-DB writes from the CLI (staleness) — fix with notification/poll if it doesn't.

### The DA absorption track — `kernl da` + migrating llm-workflow into kernl (decided 2026-07-18)

**The frame:** llm-workflow's role as "the mayor" — a context-rich general assistant session (constitutional about-me + state + memory) — is validated by daily use: it is the most-used repo since it exists, and a session there already does the job the DA is meant to do. The decided end state: **kernl absorbs llm-workflow piece by piece** — knowledge first, harness machinery later, each piece moving when the kernl organ for it exists. llm-workflow dissolves into kernl over time, which was always its charter ("internalize into kernl what proves its value" — it proved it).

**What migrates vs what stays (by category, not by proof):** knowledge and state (backlog, lessons, about-me, eventually ops logs) belong in the graph. Harness machinery (hooks, skills, injection scripts) migrates only by **reimplementation as kernl features**, never by copying scripts — and Claude-specific glue stays with the harness until kernl wraps the harness entirely (closer than it sounds: `internal/adapter` already drives five agent CLIs non-interactively, permission bridge included).

**The decided sequence** (details live in the items referenced):

1. **Agent-ergonomics pass on the CLI** — see *A complete, agent-oriented CLI*.
2. **`kernl da` v1** — see the decided mechanism under *When the DA's harness is Claude…*: exact-same-experience wrapper, cwd anchored at llm-workflow, launchable anywhere, preamble deferring to `kernl --help`.
3. **Version the vault** — private git repo for `~/vault`, `.gitignore` for `.kernl-graph.db`. Cheap, and the unblock for steps 4–5: history/blame is what makes moving about-me and ops logs acceptable (see the 2026-07-11 incident that founded the machine-log rule).
4. **Knowledge absorption** — llm-workflow `BACKLOG.md` → kernl tasks/projects; `lessons.md` → kernl memory claims. This **flips the source of truth**: "keep BACKLOG.md true" becomes "keep kernl true" for every session. Scope: llm-workflow's backlog only, not the per-project planning pipelines. Reminders explicitly stay behind as an llm-workflow hook (their value is arriving in the environment where the action happens; `kernl da`'s anchored cwd keeps delivering exactly that).
5. **about-me → vault as tagged notes**, ops logs → vault notes afterwards — see the 2026-07-18 revision under *Give the DA the context…*.
6. **In-app DA chat over the Claude harness** — wire `DaChatSurface` → `internal/adapter`; absorbs the harness axis of *Switch the model / harness…* and dissolves the 4 KB cap as a necessity.
7. **Phase 2 — the DA session joins the graph:** a `kernl da` terminal session becomes a `chat-session` node. Registered as a goal, not designed.
8. **Horizon (registered, not designed):** remaining harness machinery organ by organ — context injection → injectables, session capture → chat-session nodes, lessons/improve-system → memory consolidation.

**Already done (2026-07-18):** the orchestrator harness in `kernl.local.yaml` (gitignored — recorded here because git can't) switched from `opencode run --format json` + claude-sonnet-4-6 to the `claude` CLI with model `claude-opus-4-8`. The adapter builds the non-interactive invocation itself (`-p`, stream-json, permission bridge, `--model` from config), so the entry needs no `args`. `kernl doctor` green; the server was not running, so nothing needed a restart.

## Deferred — agent-ergonomics pass 2 (decided 2026-07-19)

The pass-2 audit scored 401 CLI surfaces and applied 9 of 10 recommendations on PR #109. These are the pieces consciously left out — kept here (not only in the gitignored audit workspace) because two of them need a decision from the maintainer and the rest are real follow-up work.

Both "needs a decision" items were decided on 2026-07-19 and moved to `## Tasks`: R2-006 c (the `ingest queue resolve` empty-action default) and R2-006 d (`inbox prep`, whose finding did not survive review — see below).

### Corrected finding — `inbox prep` does not have an expensive default (R2-006 d)
The pass-2 audit recorded this as "mutates by default; the safe path is opt-in, the expensive LLM path is the default". **That characterisation is wrong**, and it is kept here so a later pass does not reopen the question from the same false premise.

`inbox.Prep` short-circuits on an existing prep edge *before* constructing any prompt (`internal/inbox/prep.go:52-62`), and the handler documents the property (`internal/api/inbox.go:671`). So bare `prep` and `prep --show` differ **only** for a never-prepped capture: bare `prep` generates once, `--show` answers "no prep yet" at exit 0. On an already-prepped capture the two are identical and neither costs an LLM call.

The pair is therefore coherent — `prep` is **get-or-create**, `--show` is **get-if-exists**. What is actually defective is that nothing says so: the `Details` text (`cmd/kernl/inbox.go:99-103`) documents `--show` alone and never states that the bare form generates, or that generation costs an LLM call the first time.

**Decided:** keep the default; fix the description. Rewrite the `prep` `Details` to state the get-or-create semantics and the first-call LLM cost, so it reaches `capabilities --json` too (which exposes `Details` since R2-003). Inverting the default (`--generate` to write) and splitting into `prep make` / `prep show` were both rejected: they trade one surprise for another and break a fence the code justifies. A `--yes` gate was rejected as well — the operation destroys nothing, and using the gate here would dilute the signal it now carries on `inbox reopen` and `inbox batch apply`.

(R2-009 part 1 — the `task create` positional join — was decided on 2026-07-19 and moved to `## Tasks`. It was filed as a taste call; it turned out to rest on a comment the code contradicts.)

### Backlog work — a feature, not a CLI tweak (R2-009 part 2)
`task set` has no `--description` / `--project`, so a task can never be re-described or re-parented from the CLI. Adding the flags requires backend work first: the PATCH handler (`internal/api/tasks.go`) only accepts title/status/tags/dueDate, and there is no `nodes.SetTaskDescription` or task re-parent (re-parenting is a graph-edge operation, not a field write). Wire the node-layer setters + the PATCH handler, *then* the CLI flags — otherwise the flags are a silent no-op.

### Backlog work — mechanical, no decision needed
- **R2-004(b)** — a dangling flag value (`--project` with nothing after it) points the recovery hint at root `kernl --help` instead of the owning verb's help (`takeFlag`, `verbio.go`). The clean fix threads the verb through `takeFlag`'s ~35 call sites. Deferred for churn, not risk.
- **R2-007(b)(c)** — `ingest queue list` and `memory claims --json` leak PascalCase (`ID`/`Title`) against the camelCase wire contract. Fixing it is a wire-shape change: add json tags (or a DTO) *and* flip the CLI decoder atomically, and check the web GUI does not consume those routes first, or the listing goes silently empty.
- **capabilities third-level subcommands** — `ingest queue *` and `inbox batch *` live in plain string slices, not `commandMeta.Subs`, so they are invisible to `capabilities --json` and their `--help` renders the parent topic. Promoting them to `commandMeta.Subs` fixes both. (Structured per-flag data in capabilities is the same class of change: needs a `Flags` field on `commandMeta`; the command `Details` string is exposed today as the interim.)

The `kernl triage` mega-command (the pass-3 headline) sits on top of these plus the approvals decision — see the API contract audit's approvals note / PR #110.

## Deferred — findings from the API contract audit (decided 2026-07-19)

Building a CLI client against all ~88 REST routes exercised surfaces nothing else had. What follows was found, judged, and consciously not acted on. The defects that were real — path traversal, the CORS wildcard, Go field names on the wire, settings replacing whole sections, an update check asserting an answer nobody computed — are fixed; these are the remainder.

### API authentication
The API has none. A caller that reaches the port can read and write the vault, the graph and the orchestrator. Two mitigations landed — the server binds loopback by default, and CORS is restricted to local development origins — so the reachable surface is now local processes and pages rather than the network. Adding real authentication is a design decision that has not been made: what kind (token, local socket, OS user), how a CLI on the same machine presents it, and what a container deployment does about it.

### `bead rollback` on the bd backend — not a defect
`BdCliBackend.Rewind` fails with `bd backend does not support rewind; use knots backend for workflow corrections`, so `kernl bead rollback` can never succeed against bd. This is correct behaviour, recorded here so it is not "fixed" by a later pass: the bd CLI has no rewind (only `reopen`, also refused), and rewind plausibly means discarding recorded progress rather than setting a status. A half-rewound bead is worse than a loud refusal. Implementing it as a `bd update --status <target>` would look like a fix and be a trap.

### Remaining snake_case on kernl's own wire
The wire/storage split fixed the shapes that had to move. Three smaller surfaces still speak snake_case and were left alone to keep that change reviewable: `scope_node_id` in the `POST /api/chat/sessions/{id}/messages` request body, and `file_path` / `node_id` in `web/pages/ingest.vue`.

### Zero is indistinguishable from unset for runtime integers
`config.Load` backfills a default for every zero-valued runtime integer (`maxConcurrentBeads`, `stageRetryAttempts`, the sweep counters), so an explicit zero is written to `kernl.yaml` and read back as the default. No change in the API layer can fix this; the field has to become a pointer in `config.Config`, the way `InboxConfig.AutoClassify` already is. A test pins the current behaviour so the surprise is documented rather than rediscovered.

### `ChatSession` wire types and the empty-list response
`newBookmarkResponses` returns `nil` for an empty list, so `GET /api/bookmarks` answers `null` rather than `[]`. Changing it is an improvement and a wire change, so it belongs with whatever pass revisits list responses generally.

### A dead permission surface in the web client
`useChatSession` exports `pendingPermission` and `resolvePermission`, and no component renders either. Either the UI for resolving a tool-call permission was never built or it was removed; the composable and its backing route are live regardless.

## Deferred — Human judgment gates (approvals): the capture flow is unbuilt (decided 2026-07-19)

**This is the product's core promise — "the human touches only judgment gates" — and it does not work end-to-end.** The approval subsystem is fully-typed scaffolding with no runtime wiring. Confirmed across three layers:

- Neither registry is ever instantiated in runtime: `approvals.NewApprovalRegistry` has no caller, and `terminal` `RecordPendingApproval` / `SetApprovalResponder` have no caller. `internal/approvals` is imported by no runtime package.
- The execution helper `terminal.PerformApprovalAction` has no runtime caller.
- The agent adapters *can* be configured with an MCP approval bridge (`kernl_approval`, `--permission-prompt-tool`, `KERNL_APPROVAL_BRIDGE_BASE_URL`), but the `Build*WithBridge` arg builders that would activate it have no runtime caller, and the bridge base URL is read nowhere.

So no live agent permission prompt is ever captured into anything the API could serve. The three `/api/approvals*` routes were empty stubs; the web client's `pendingPermission`/`resolvePermission` (see the entry above) is the GUI half of the same unbuilt feature.

**What was done instead (the honest-facade fix, shipped):** the routes now answer `501 Not Implemented` with a clear message rather than faking success. Before this, `POST /api/approvals/{id}/actions` reported success for an id that never existed — `kernl approval resolve apr-999 --action approve --yes` printed "Resolved" at exit 0 — and `GET /api/approvals` reported "nothing pending" when it could report nothing at all. 501 lets a caller tell "unbuilt" from "idle".

**What building it for real requires (the future project):**
1. **Activate the approval bridge in agent dispatch** — call the `*WithBridge` arg builders so a live agent routes permission prompts through the `kernl_approval` MCP server (or the per-adapter equivalent: Codex `approval_policy`, ACP permission requests).
2. **Receive prompts at an endpoint** and extract them (`approvals.ExtractApprovalRequest` already exists) into a `PendingApprovalRecord`; call `RecordPendingApproval` on the owning session.
3. **Wire the responder** (`SetApprovalResponder`) so applying an action replies to the agent over its native transport (ACP / stdio / JSON-RPC) to unblock it. `PerformApprovalAction` already encapsulates the status transitions and returns an `HTTPStatus`, ready to be called from the API.
4. **Instantiate the registry in `app.App`** and thread it into `RegisterApprovalRoutes`, then replace the 501 handlers with real ones.
5. Design decisions still open: which adapters/transports are in scope first, how the bridge authenticates (`KERNL_APPROVAL_BRIDGE_TOKEN`), how a reply injects per transport, and how the terminal-session route relates to the orchestrator-gate route (two vocabularies today: accept/always_approve/decline vs the gate's).

**Dependency:** the `kernl triage` mega-command (a pass-2 ergonomics candidate) wants an "approvals waiting on you" slice. Until this project lands, `triage` must show approvals as *unavailable*, not *0 pending* — the 501 makes that distinction detectable.

## Deferred — CLI ⇄ GUI parity track (decided 2026-07-18, implemented 2026-07-19)

### `kernl` CLI parity with the web GUI — IMPLEMENTED, in review

Shipped as 22 verbs / 65 subcommands. New verbs are thin clients of the running `kernl serve` REST API; `capture`, `bookmark add|import`, `plan`, `bead run` and `epic run|merge|abort` stay direct, because they execute locally and have no thin-client form. Still out of scope by decision: the DA chat and the SSE event streams, whose non-interactive CLI shape is an open design question.

The original inventory that scoped the work is kept below for reference.

Every GUI capability should eventually be reachable from the `kernl` CLI — full CRUD for every tab/feature. Today the CLI covers roughly 10% of the GUI surface: `capture`, `plan`, `bookmark add/import`, `epic list/run/merge/abort`, `bead run`, `sweep`, `doctor`, `serve`, `version`. The web GUI is backed by ~88 REST routes; the gap by area:

- **Home / dashboard:** health, approvals (list + act), app-update check.
- **Inbox:** list pending/processed, process (keep/convert/discard), reopen, batch analyze/apply, batch-log, auto-classify get/set, prep briefings, rollups.
- **Notes / vault:** list notes, read/save/delete a vault file, suggest, apply-hunks, tags.
- **Bookmarks:** list (only add/import exist), highlights.
- **Memory:** claims list/add/refute, topics, telos.
- **Projects:** full CRUD (list/create/patch/delete).
- **Tasks:** full CRUD (plus the delete/retitle fix tracked in `## Tasks`).
- **Orchestrator:** beads list/get/create/patch/close/rollback/refine-scope/ mark-terminal, epic events/sessions, session nudge, approvals resolve.
- **DA chat:** session create/list, send message, read events (CLI shape TBD — possibly out of scope for a non-interactive CLI).
- **Ingest:** paste, upload, source, trigger, queue list/resolve/merge-plan.
- **Settings:** get; set inbox/llm/runtime/vault.
- **Graph:** nodes list/search/related/briefing, edges.

**Why deferred:** it is feature work (~30–50 new verbs), deliberately kept out of the agent-ergonomics pass, which measures and fixes only existing surfaces.
- **Depends on:** the ergonomics-pass foundation — command metadata table, exit-code dictionary, camelCase `--json` conventions, `capabilities --json` — so new verbs are born onto those rails instead of repeating the audited flaws.
- **Design decision to settle first:** new verbs as thin clients of the running `kernl serve` REST API (no second SQLite writer, but requires the server up) vs. direct use of internal services (works offline, duplicates wiring, and contends for the graph-DB lock with a running server). Decide once, apply to the whole track.
- **Execution:** run as its own feature track (kernl-dev pipeline); after it ships, re-run the agent-ergonomics audit as pass 2 to score the new surfaces against the pass-1 baseline and catch drift.

## Deferred — from v0.1.0 (decided 2026-06-26)

> Source of the v0.1.0 scope decisions these defer from: the v0.1.0 roadmap brainstorm (2026-06-26). In-scope work is tracked separately as the v0.1.0 plan.

### Ingest — "Deep Research" action
When ingested content makes a claim needing verification, this action would dispatch a research task (an agent investigates external sources and folds findings back into the graph).
- **Why deferred:** needs a research-agent pipeline that does not exist; overlaps with the Orchestrator / DA tools, both deferred.
- **Depends on:** agent research pipeline (Orchestrator or a DA research tool).
- **Note:** the extractor prompt is being narrowed to stop emitting this action until it is built; re-enable in `internal/ingest/llm_extractor.go` when ready.

### Ingest — "Add Contradiction Callout" action
When ingested content conflicts with itself or with established knowledge, this action would flag the conflict (attach a contradiction callout) for reconciliation.
- **Why deferred:** needs contradiction detection/marking infrastructure, which is conceptually the same machinery as Memory's `refutes` edges, plus a callout UI.
- **Depends on:** Memory rewiring (v0.1.0). Candidate **stretch** once Memory is wired and time allows; otherwise a follow-up.

### Notes — Undo delete note
A feature to undo the deletion of a note (move the note back from the system trash to the vault and reconnect it in the graph).
- **Why deferred:** the initial implementation simply moves the file to the system trash. Building a full undo flow (tracking the original location, moving it back, and ensuring graph reconnection) is a follow-up scope.

### Notes — WYSIWYG / ProseMirror editor (Tiptap / Milkdown)
A true rich-text editor beyond the v0.1.0 CodeMirror 6 live-preview approach.
- **Why deferred:** CodeMirror live-preview is the closest-to-Obsidian path and reuses the existing editor; a ProseMirror swap is a larger rewrite with markdown round-trip risk. Only pursue if live-preview proves insufficient in daily use.

### DA — automated eval harness (golden transcripts, scored)
Replace/augment the manual UAT plan with scored, repeatable evals.
- **Why deferred:** premature carrying cost before prompts stabilize. Build after the manual UAT (v0.1.0) settles the prompts.

### ~~Bookmarks — full reformulation~~ → **UN-DEFERRED 2026-07-14. This is active work now.**
~~The bookmarks visualization is poor and needs a redesign.~~
- ~~**Why deferred:** off the magic loop; low priority. Ships rough in v0.1.0.~~
- **Why it came back (Gabriel, 2026-07-14):** he started processing his real WhatsApp inbox, and **the captures that are links have nowhere correct to land.** Saving them into today's bookmarks would mean re-doing all of it after the reformulation — so the reformulation is now *cheaper than the workaround*. **The deferral said "off the magic loop". Processing the inbox put it ON the magic loop.** That is the condition that expired, and it expired for the right reason.
- **Also, "ships rough" understated it:** the backend does not do what he wants yet — *"there are maaaany features to implement"*. What exists: `internal/graph/nodes/bookmark.go` (+ `bookmark_list.go`, tested), `internal/api/bookmarks.go` (create, list, highlights, archive), `cmd/kernl/bookmark.go` (`add`, `import`), and a UI of ~300 lines (`web/pages/bookmarks.vue`, `BookmarkItem.vue`, `BookmarkReader.vue`). So this is a **redesign on top of a working skeleton**, not a greenfield — and not a polish job either.
- **Blocked on a decision, not on code:** the development process itself. See `llm-workflow/BACKLOG.md` **S12** — there is no "loop" skill between bootstrap and review, and the sequencing questions this feature raises (backend or UI first, harden before or after) are exactly the gap. **This feature is the observation vehicle for S12: do the work watched, and write down what was actually done.**

### Inbox — the batch merge misses messages it should catch, AND it cannot tell you when it failed

**Gabriel, 2026-07-14:** the WhatsApp batch does not propose merging messages sent in sequence about the same subject. **Investigated against the running server (`localhost:8080`), not by reading code — and the result contradicted the first two theories, so the evidence is written down here in full.**

**What was tested.** Four real messages of his, posted to `/api/inbox/batch/analyze`:

```
6/13/26, 14:16 - Gabriel: coisa pra eu lembrar: … criar forma de gerenciar vagas:
                          - step (entrevista com IA, teste de código, …)
                          - detalhes da vaga (link, salário, …)
6/13/26, 14:16 - Gabriel: - itens que bianca colocou no curriculo heh   ← a forgotten bullet
4/6/26,  09:35 - Gabriel: https://claude.ai/chat/3d0b7342-…
4/6/26,  09:35 - Gabriel: PROCESSAR ^                                    ← acts on the previous msg
```

**Result:** `"enrichmentStatus": "applied"` — **the enricher runs, it is not broken** — and it proposed exactly one merge:

```json
{"sourceSequences":[0,1], "reason":"second message adds a bullet to the list started in the first"}
```

**So three things are now known, and two of them killed a theory:**

1. **The forgotten-bullet case already works today.** It is an explicitly permitted merge in the current prompt (`internal/inbox/batch_enrichment.go:148` — *"an item added to a list an earlier message started"*). **No prompt change is needed for it.** The first theory (*"the prompt is too strict, loosen it"*) is **wrong for this case**.
2. **The `link` + `PROCESSAR ^` case is genuinely NOT covered.** A message that *refers to or acts on* its neighbour ("^", "isso", a link followed by an instruction) is **one thought split across two sends**, and it matches none of the three permitted shapes (restated / finished in the next message / bullet added to a list). This is a real prompt-coverage gap.
3. **A second, unrelated asymmetry:** the merge prompt renders messages as `[seq] Sender: body` — **with no timestamp** — while the classifier that runs later over the same captures *does* get one (`internal/inbox/classifier.go:327`: `[%d] (written %s)`). The merge proposer is strictly blinder than the classifier. Both of Gabriel's pairs were sent in the **same minute**, and the merge proposer cannot see that.

**THE TEST THAT STILL HAS TO BE RUN — this is the whole point of this entry.** If the bullet case merges in a 4-message paste but did **not** merge in Gabriel's real paste, then **the prompt is not the bug** and patching it would be treating a symptom. Two live hypotheses:

- **Scale.** In a paste of 100+ messages the model's attention degrades: a merge it finds trivially in 4 messages it loses in 100. **If this is it, no prompt edit helps** — the fix is windowing/chunking the enrichment call.
- **The UI swallowed it.** See the Fail-Loud bug below.

**How to run it (Gabriel deferred this on 2026-07-14 — it is slow to reproduce by hand, so do it from the API):** recover the original paste — **`batch_log` persists `RawText`** (`internal/inbox/batch_log.go`), so any past `batchId` gives the exact text back — and POST it to `/api/inbox/batch/analyze`. Read `enrichmentStatus` and `mergeProposals`. **Do not touch the prompt before this run answers scale-vs-UI.**

**Then, and only then:** the fix for (2) and (3) is narrow — send the timestamp, and add the "refers to / acts on the neighbour" shape to the permitted list. **Keep** *"same topic is NOT enough"*: that half of the fence is still right, and the structural guarantees behind it (bodies rebuilt from source; the human accepts every merge; the capture count is the parser's, not the model's — all landed together in `dc2d888`) are what make a *proposal* safe in the first place.

### Inbox — the batch review UI hides its own failures (violates Fail Loud, Never Silent)

**Found 2026-07-14 while investigating the merge complaint above. This is a bug on its own, and it is the reason the merge complaint was hard to diagnose at all.**

The backend computes an honest `EnrichmentStatus` — `applied | unavailable | failed | none` — plus an `enrichmentError`. **The UI throws both away:**

- **`web/components/inbox/InboxBatchDump.vue:291-294` catches every `/analyze` failure and sets `proposals.value = []`.** An empty `catch`. A timeout, a 500, a model error, unparseable JSON — all render identically to *"the model looked and found nothing worth merging"*.
- **`enrichmentStatus` is never rendered anywhere in `web/`** (grepped: zero hits). If the LLM is not configured, `buildOptionalBatchLLM` returns `nil` → status `unavailable` → **zero merge proposals, silently.**

**So there is no way for the user to distinguish "no merges suggested" from "enrichment never ran".** That is precisely what `AGENTS.md:90` forbids: *"**Fail Loud, Never Silent** … NEVER return a fallback"*.

**Fix:** surface the status in the review modal. `failed`/`unavailable` must *say so* ("merge suggestions unavailable — the model call failed"), distinctly from `applied` with an empty list ("no merges suggested"). The batch itself stays usable either way — enrichment genuinely is optional — but the user has to be told which world they are in.

**Not started (Gabriel deferred it 2026-07-14: document, don't fix now).**

### Inbox — import a WhatsApp export *with* its media (zip), not just the text

**Gabriel, 2026-07-14.** Today the batch importer takes pasted text only. A real WhatsApp export is a **directory**: the messages plus every attachment. Two things follow, and the first is nearly free while the second is a genuine feature.

- **The cheap half — stop creating garbage captures.** There is currently **no handling of media lines at all** (grepped `internal/` and `web/` for `omitted|attached|Media`: zero hits). Every media message becomes a capture whose body is a filename or "Media omitted". Dropping those is a filter in `cleanSegments` (`internal/inbox/batch.go:695`).
- **The real half — the attachments are *pointers*, and they resolve.** Verified against Gabriel's own export at **`~/Downloads/whatsapp-export/`** (98 files + `messages-export.txt`). The Android format names the file **inside the message line**:

  ```
  4/5/26, 19:26 - Gabriel Assis: IMG-20260405-WA0009.jpg (file attached)
  ```

  The filename is the join key — **matching by timestamp is the fragile solution to a problem that does not exist.** (A caveat that will bite: WhatsApp prefixes these lines with an invisible **U+200E LRM** in some exports/locales, so a naive `HasPrefix` fails silently. Normalize first. The iOS/pt-BR variants use `<attached: …>` / `<anexado: …>` — do not hardcode one shape without checking against this dump.)

**The feature, as Gabriel scoped it:** upload the zip → match text to attachments → call an **image-interpretation** model on the images → understand when several images were sent **together** (the same dump has five images at `19:26`, which is exactly that case: they are one thought, not five).

**Not started. The example dump to build against is `~/Downloads/whatsapp-export/` — read the real format, do not write a regex against a remembered one.**

### Orchestrator — the decision artifact (autonomy without comprehension debt)

**Gabriel, 2026-07-14:** *"How do I use autonomous orchestration without comprehension debt?"*

**The rules already answer it — what is missing is a place to put the answer.** `AGENTS.md:110` already says *"**Comprehension Debt:** never make a silent architectural decision… record the rationale"*, and `AGENTS.md:113` already says a multi-domain edit is flagged **BLAST RADIUS** and **never merged autonomously**. So the posture is settled: **execution may be autonomous; decision may not.**

**The gap is that "record the rationale" has no destination.** It is an instruction with no artifact, and an instruction with no artifact does not survive a fan-out of parallel agents — the rationale ends up in a session that dies, and the reviewer is left reconstructing *why* from a diff. That is comprehension debt arriving in the worst possible form: after the fact.

**The shape of the fix (not designed yet, deliberately):** every autonomously-executed unit (bead/epic) must deposit, next to its output, what it **decided**, **why**, and **what it rejected**. Comprehension debt is not paid by supervising more — it is paid by making the *why* a required output, not a courtesy.

**Not started.** Related: the wider "what is the development loop" question is `llm-workflow/BACKLOG.md` **S12**.

### Orchestrator — polish / real-world mileage
Hardening the epic-to-PR path against live agent CLIs.
- **Why deferred:** low priority for v0.1.0; the loop does not depend on it for daily solo use.

### Settings + Profile — dedicated UI
No UI exists for settings or profile today.
- **Why deferred:** not on the magic loop; not a v0.1.0 priority.

### Graph — full rework (clustering, hover edge labels)
Richer graph interactions beyond the v0.1.0 visibility canary.
- **Why deferred:** the canary (edge visibility + verifying connections actually work) covers v0.1.0; clustering/labels are over-scope.

### Memory — claim editing / versioning UI
Anything beyond Keep / Edit / Discard on the `DA · learned` card plus the existing refute flow.
- **Why deferred:** YAGNI until a real need shows up.

## UI chrome currently mocked (decided 2026-06-28)

These ship as hardcoded placeholders because no real data source exists yet. Kept intentionally (they read as "real" chrome) until the backing state is built; replace the mock when wiring each.

### DA panel — version badge, scope, and context chips
`web/components/DaChatSurface.vue`: the `v2.4-stable` version badge (rendered per assistant message — so it only shows once the DA replies, which is why an empty chat shows nothing), the `scope · global` chip, and the `Context: All files` footer chip are all static.
- **Why deferred:** there is no versioning, scope, or active-context concept wired to the DA yet. Define those concepts, then bind the chips.
- **Note:** the greeting name (`Morning, Gabriel.`) is also hardcoded; source it from user/session config when that exists.

### Shell footer — `system_online` and `synced` status
`web/layouts/default.vue`: the green `system_online` dot/label and the `synced` indicator are static.
- **What to build:** a real status signal for each.
  - `system_online`: define the possible states (e.g. `online` / `degraded` / `offline`), derived from a health/heartbeat check (the `/api/health` endpoint already exists and now returns vault info — extend it with a real status, or add a dedicated probe).
  - `synced`: define the possible states (e.g. `synced` / `syncing` / `dirty` / `error`), derived from vault-watch / write-back state.
- **Why deferred:** no health-state or sync-state source exists yet; wiring a label to a nonexistent source would be a "live lie".
- **Done already (not mocked):** the `~/vault` path now comes from real config via `/api/health` → `vaultLabel`; the `UTF-8` and `system_ready` chips were removed.

## Captured from inbox triage — 2026-07-15

Feature ideas routed here from Gabriel's WhatsApp capture inbox during a manual triage (in `llm-workflow`). Kept in one section on purpose — **distribute each into the thematic sections above when it is picked up**, don't let this become a parallel backlog. Every entry carries its capture provenance (WhatsApp, 2026-07-14) and Gabriel's triage note; none is scoped or committed to yet.

### DA / intelligence

- **Upload PDFs of favourite books so the DA knows they exist** — as *references*, not absolute truth. The intent: give the DA the philosophy of the books most important to Gabriel, the ones he wants to internalize.
- **A recurring-items note/tag.** When he writes something down, loses it, and writes it *again*, that re-noting is the **strongest signal the thing matters to him**. Could be a dedicated note, a tag, or any way to surface the recurrence.
- **A new capture type: `spontaneous`** — distinct from idea / spark / backlog / thought. The insight was the *naming*: these are the ideas that arrive while walking, showering, lying idle. One can become a task, a self-insight, or a reflection.
- **DA should hold back new connections** — not surface a new connection it found; only *hint* at it, until Gabriel notices it himself.
- **Sycophancy + critical thinking** (Maggie Appleton) — fold into the DA's customizable prompt, and it is worth a **benchmark** of its own.
- **Compound-engineering-style learning** — learn patterns over time, iterate, learn from mistakes. Connects to `llm-workflow` `PLAN.md`'s CE stance (adopt selectively, *not* wholesale).
- **Scan notes → extract actionables** — the ideas he wrote and lost in his notes. This is the feature that would automate the very triage that produced this section.
- **A `graph-lint` / `graph-cleanup` pass** to tidy the graph periodically — possibly an internal feature wrapped by a UI, not necessarily a skill.

### Orchestrator / agents

- **A "mayor"-style orchestrator tab**, more intuitive: ask for one or more things and it suggests the depth/rigor the task needs. (The depth mechanism came from an old skill-flows repo and would look different today; the *tab itself* he still wants.)
- **Revisit the fungible-vs-role-based agents discussion** *(task: understand/recall)*. Raw capture: his argument that in kernl it is all scalable *workers*, workflow-based rather than role-based ("quem implementa o código não tem role específica"; questions the merge-agent bottleneck, premature role taxonomy, and the context-window limit of one agent "following the whole thread"). **Re-read it and decide what still makes sense to keep/apply.**

### UI

- **Mini-browser tab UI** — tabs per chat/session (like tmux session tabs) + a notification dot (waiting on you / finished / failed-or-interrupted).
- **Accent-colour customization (light/dark)** — nice-to-have, *not* priority. DA *personality* is already done (the customizable prompt); the DA **name is hardcoded** across kernl and needs an edit field. **Reference: Amazing Marvin** — infinitely customizable but takes 1+ weeks to learn; the anti-example that justifies kernl's "works out of the box".
- **Two designs:** an "enterprise/normie" one (to start visualizing how to sell to mid-size companies) + the OSS build lets the user pick.
- **A real fullscreen "setup"-style onboarding** — like the config you do at signup, teaching the basic features. Not the usual guided tour.
- **Token-consumption dashboard.**
- **Automatic filters + lists** — filters for bookmarks / tasks / projects / tags (a tag groups all node types carrying it); auto-lists (optionally driven by a prompt specifying what may be created) for bookmarks.

### Mobile

- **A kernl mobile app (Android/iOS)** — start with only the phone-useful features (capture/inbox, bookmark, notes) + DA integration from the phone. Ship a **`/app` path first** = an app-like (PWA) view to use before the native app exists (especially for iOS, to avoid the store fee). *(The Telegram-integration idea from the same capture is personal use, not kernl.)*

### Build / dev

- **`Vite+` (voidzero-dev/vite-plus) — a spike/recon task.** First understand what it actually is, then assess whether it makes sense for kernl. Not adopted, just investigated.

### Product philosophy — reinforce in `PRODUCT.md` / `VISION.md`

- **"Works perfectly out of the box; endlessly customizable for the technical folks without overwhelming the out-of-the-box user."** Already stated in `README.md` — reinforce it in `PRODUCT.md`/`VISION.md`. Amazing Marvin (above) is the concrete anti-example it exists to avoid.
