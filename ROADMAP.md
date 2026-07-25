# Roadmap

Where Kernl is, what is missing, and what it deliberately will not become. It is the public counterpart of the working backlog, which is a maintainer note and lives outside this repo.

Kernl is pre-1.0 and shaped around a single-user, local-first workflow. Read the sections below as a statement of direction, not a schedule: there are no dates here on purpose.

## The bet

Notes, bookmarks, captures, tasks, projects, memory and agent execution are usually five tools that share almost nothing. Kernl's claim is that they are one substrate, and that the value appears where they meet: what you captured last month lands in the planner's context automatically, without you remembering it exists.

The loop the product is organized around is **capture, connect, execute, write back**. Every item below is judged by whether it makes that loop shorter or more reliable.

## What exists

These are implemented, covered by hermetic tests, and usable today.

- **The graph substrate.** One SQLite database holding notes, captures, bookmarks, tasks, projects, memory claims, chat sessions and workflow runs as typed nodes with edges, revisions, tags and full-text search. The API, the vault watcher and `kernl capture` all read and write the same database.
- **The markdown vault.** Plain `.md` files stay human-owned. Kernl indexes them, injects stable UUIDs into frontmatter, watches for changes and keeps a revision history. A file edited in any external editor reconciles back into the graph.
- **Substrate-aware planning.** `kernl plan "topic"` and the planner API retrieve relevant vault notes before work starts. This is the part of the product with the least prior art and the most reason to exist.
- **Inbox and capture.** Quick captures enter the graph as pending items, get classified, and are processed into notes, bookmarks or tasks with provenance preserved. A capture body is a primary source: it is never rewritten or merged without a human decision.
- **Bookmarks.** Add or import a URL, archive readable HTML, connect it to the rest of the graph.
- **The orchestrator.** Bead DAGs execute in isolated git worktrees with agent pools, review stages, integration and PR shipment. The epic-to-PR path is implemented end to end.
- **One binary.** The Nuxt UI is compiled into the Go binary; `kernl serve` provides the web UI, the REST/SSE API and nothing else to install.
- **A CLI that matches the GUI.** Every surface the web UI exposes is drivable from the command line, which is what makes Kernl scriptable by an agent rather than only clickable.

## What is missing

The honest gaps, roughly in the order they hurt.

- **The approval gate is a stub.** The orchestrator is described around a human touching only judgment gates, but the approvals API returns empty responses. The UI and the CLI in front of it are real; the gate behind them is not. Until this lands, "unattended execution with human checkpoints" is a design, not a feature.
- **Orchestrator mileage.** The epic-to-PR path passes hermetic tests but has limited runtime against live agent CLIs. Failure modes under real dispatch are the open question.
- **Task lifecycle holes.** A task cannot be deleted or retitled through the API or the UI.
- **Memory is thin.** Claims can be stored, but the curation, editing and versioning surface around them is minimal, and nothing yet guarantees a claim is read back where it matters.
- **A release that counts.** The install path assumes a published release archive. Cutting the first real one is gated on the on-disk data layout settling, because moving it after publication means carrying a migration forever.
- **Ingest and inbox rough edges.** Batch import of a chat export handles text but not attachments; several review actions are less explicit than the Fail Loud rule requires.
- **Documentation.** There is a README, a glossary and this file. There is no documentation site, and there will not be one before there is a release worth documenting.

## Deliberately out of scope

Not "later". These are decisions, and reopening one needs a reason that does not exist yet.

- **Multi-user, multi-tenant, teams, sharing, permissions.** Kernl has one user. Every abstraction added for a second one before that user exists is cost with no payer. This is the single most likely direction to drift in, so it is written down here.
- **A hosted service.** Local-first is the product, not a stage before a SaaS. Self-hosting the server in Docker is supported; running it for someone else is not the plan.
- **A mobile or tablet client.** The tool is built for a desktop, a keyboard and a large monitor. Capture from a phone is worth solving, a full mobile client is not.
- **A plugin ecosystem.** Extension points get extracted from a pattern that proved itself, never designed ahead of one. There is no third-party surface today and no need for one.
- **Windows as a release target.** The watchdog is Unix-only. Windows users run the Docker image.
- **A WYSIWYG editor.** The vault is markdown that a human owns. A rich-text layer between the user and the file is the wrong side of that bet.
- **Replacing the issue tracker or the agent CLIs.** Kernl orchestrates tools that already exist and shells out to them. It is not going to grow its own.

## How this file changes

An item moves from "missing" to "exists" when it is implemented, tested and usable, not when it is planned. An item enters "out of scope" only with the reason attached, because a constraint without its reason is the thing a future contributor undoes by accident.
