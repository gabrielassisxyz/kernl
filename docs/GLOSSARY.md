# Kernl — Glossary

> Ubiquitous Language for the Kernl orchestrator. Anchored to `orchestrator/specs/00-architecture.md` where relevant.

## bead

The atomic unit of work in Kernl. A bead is a single task with a defined state, dependencies, and an assigned agent pool. It lives in the Dolt-backed issue tracker (`bd`) and moves through a workflow state machine (queue → active → review → terminal).

**Why not "beat"?** The upstream codebase (`foolery-go`, which is Go — not TypeScript) uses "beat" as a domain term. Kernl renamed to "bead" to avoid collision with the common English word and to evoke a bead in a dependency-graph necklace: each bead is discrete but connected. The `orchestrator/specs/00-architecture.md` may retain "beat" in older sections inherited from foolery-go.

## epic

A collection of beads organized as a directed acyclic graph (DAG). Beads within an epic declare dependencies on each other; Kernl schedules independent beads in parallel waves. Epics are the unit of human ambition — "turn this idea into an epic" is the entry point. See `orchestrator/specs/00-architecture.md` §8.3 for hierarchy and §8.4 for cycle safety.

## take loop

The core execution loop that drives a single bead from claim to terminal state. The orchestrator claims a bead, resolves its dispatch pool, spawns an agent session, monitors stdout/stderr, and on child exit classifies the outcome (success/error/stuck) and decides the next step: advance, retry with a different agent, rollback, or terminate. Capped at 5 follow-up turns per state. See `orchestrator/specs/00-architecture.md` §5.1 and §10.1–10.2 for the full state machine.

## dispatch

The resolution chain that maps a bead to a concrete agent process. Given a workflow + state, the system derives a pool key, looks up a weighted pool of agents, and selects one (respecting exclusions from prior failures). Failures produce a `KERNL DISPATCH FAILURE` marker naming the missing pool key and the config that fixes it. See `orchestrator/specs/00-architecture.md` §3.2.

## fail-fast

When any bead in a wave returns a non-success state or error, the executor immediately sets the epic to `blocked`, drains all in-flight goroutines, and returns without dispatching the next wave. Beads that depend (directly or transitively) on the failed bead are never dispatched. A separate deadlock detector catches the case where no beads are ready but work remains — that becomes `EpicFailed` instead. See `orchestrator/specs/00-architecture.md` §5.1 and the wave loop in `internal/epic/wave.go`.

## cross-agent review

A safety mechanism that ensures a bead's review step is performed by a *different* agent than the one that executed the action. The prior action agent is excluded from the review pool. If exclusion empties the pool, Kernl falls back to the same agent and emits a stderr banner `"Cross-agent review fallback"`. See `orchestrator/specs/00-architecture.md` §5.3.

## harness / agent

**Agent:** An AI worker configured in `kernl.yaml` (command, model, type, vendor). Kernl dispatches agents as child processes via stdio (or JSON-RPC/HTTP for certain transports).

**Harness:** The sandbox environment each agent runs in. In practice, this means a git worktree isolated per bead (default: `~/.kernl/worktrees/<bead-id>`), so agents on different beads never collide.

See `orchestrator/specs/00-architecture.md` §3.2–3.3 for dispatch and session spawning.

## epic-SSE

Server-Sent Events stream scoped to a single epic at `GET /api/epics/{id}/events`. The stream replays buffered events on connect, then pushes live events (state transitions, session starts/errors) as they occur. The monitoring GUI consumes this stream to show live bead state. See `orchestrator/specs/00-architecture.md` §12.1.

## realized parallelism

The ratio of peak concurrent agent sessions to the maximum the epic's dependency graph would allow: `peak / graphMax`. Reported on epic completion as `paralelismo realizado`. A value of `2.0x` on a DAG with max parallelism 2 means both allowed concurrent beads ran simultaneously — the scheduler achieved the theoretical maximum. A value near `1.0x` means the graph was effectively sequential despite having parallelizable branches. This is one of Kernl's key metrics from `docs/STRATEGY.md`.

## run-state

An embedded SQLite database (WAL mode, pure Go via `modernc.org/sqlite`) that persists ephemeral execution tracking across orchestrator restarts. Stores two mappings: (1) worktree paths for dispatched beads (`epic_id + bead_id → path`), and (2) agent session records (`bead_id + state → agent_id, session_id, status, updated_at`). Used exclusively by the resume planner (`internal/epic/resume.go`) to decide whether to skip completed beads, reconnect to in-flight sessions, or re-dispatch failed beads. See `internal/runstate/store.go`.

## worktree

A git worktree sandbox created per bead at `<root>/<epicID>/<beadID>` via `git worktree add` on a branch named `kernl/<beadID>`. This gives each agent an isolated working directory while sharing the repository's object store, so agents on different beads never collide on files. The `worktree.Manager` wraps three git subcommands (`add`, `remove`, `prune`) with fail-loud errors and injectable `Run` for hermetic testing. Recorded in the run-state store for resume. See `internal/worktree/worktree.go`.

## wave

A set of beads within an epic that have no remaining unsatisfied dependencies and can execute in parallel. Kernl schedules beads in waves: when one wave completes (all beads terminal), the next wave of dependency-free beads is dispatched. Waves are computed from the epic's dependency graph at runtime. See `orchestrator/specs/00-architecture.md` §4.5.

## knots (dormant)

A lease system that records which agent is working on which bead, with canonical metadata (agent name, type, provider). Single-bead sessions create a Knots lease on spawn and release it on completion. Scene (parent-with-children) sessions do not create leases. Marked **dormant** — the implementation is deferred but the concept is reserved in the domain model. See `orchestrator/specs/00-architecture.md` §5.4.

## tag

A **label, not a node**. A tag is the graph's cross-type matching axis: the one thing that can connect a note, a task, a project and a bookmark because they are about the same subject. Tags nest by convention — the name is a flat string with `/` separators (`homelab/nas`), and querying a parent includes its descendants. A tag page is therefore a *query*, not a destination: it has no description, no content and no edges of its own.

Tag names are **normalised on write**: trimmed, lowercased, and rejected if they break the nesting convention (`/foo`, `foo//bar`). Lowercasing is what makes a tag a matching axis rather than prose — `tags.name` is `UNIQUE`, so `Homelab` and `homelab` would otherwise be two subjects that never meet. The rules live in `internal/graph/tagname/`, a leaf package, because there are two write paths (`tags.Add` and the nodes chokepoint) and they must not drift.

**Why not an Area (PARA)?** An Area is a *drawer* — another hierarchy to file things into, which does not remove the "unfiled" bucket, it renames it. A tag is an *edge*. If a handful of tags later prove to be life anchors that need substance of their own, they get promoted to nodes *then*, with real usage data: tag → Area is easy, Area → tag is a painful migration. See `artifacts/plans/2026-07-11-universal-tags-plan.md`.

## system tag

A machine-authored tag, namespaced under the reserved `sys/` prefix (`sys/pending`, `sys/triaged`, `sys/audit`) and hidden from user-facing tag surfaces by default. The prefix rides the `/` nesting convention, so the same rule that hides a subtree hides system tags — no schema column, no denylist.

Users cannot author one: the API rejects a `sys/` tag with a 400, and the vault reconciler drops one found in a note's YAML frontmatter. Both boundaries are load-bearing, because a note's tags are authored in its file, not through the API — without the vault guard, typing `tags: [sys/pending]` into a markdown file would forge a capture back into the inbox queue.

The inverse also holds: **system tags never land on notes.** A note is file-backed, so the vault owns its tags — and the vault may not author `sys/`. Machine provenance on a note belongs in its `origin` field and its edges. `telos` is *not* a system tag: the user writes it by hand in the vault, and the system merely reads it. See `internal/graph/tags/system.go`.
