# Changelog

This is a synthesized, agent-facing changelog for Kernl.

Scope window: project inception on 2026-05-15 through the current pre-release
branch state on 2026-06-16.

This document was built from git history, GitHub PR metadata, and the local
project state docs. It is organized by landed capabilities rather than raw diff
order. GoReleaser will still generate per-release notes for GitHub Releases;
this file is the durable project history.

## Version Timeline

| Version | Kind | Date | Summary |
| --- | --- | --- | --- |
| Unreleased | Branch work | 2026-06-16 | Dev environment, CI, release tooling, installer hardening, version metadata, and README rewrite. |
| No public versions yet | None | 2026-06-16 | No git tags or GitHub Releases are published yet. The first release is expected to start the `v*` timeline. |

## Workstreams

The public workstream spine is currently GitHub PRs. Local bead metadata exists
outside the public repository, so this changelog links PRs for durable
branch-level intent and commit links for implementation evidence.

| Workstream | Merged | Summary |
| --- | --- | --- |
| [PR #34](https://github.com/gabrielassisxyz/kernl/pull/34) | 2026-06-16 | Graph view, companion notes, and wikilink autocomplete. |
| [PR #33](https://github.com/gabrielassisxyz/kernl/pull/33) | 2026-06-15 | Orchestrator, Projects, and Tasks screens. |
| [PR #29](https://github.com/gabrielassisxyz/kernl/pull/29) | 2026-06-03 | Wave 0-2 fixes, graph unification, module surfaces, and substrate-aware planning. |
| [PR #28](https://github.com/gabrielassisxyz/kernl/pull/28) | 2026-06-01 | Wave 2 frontend layer and cross-epic integration. |
| [PR #27](https://github.com/gabrielassisxyz/kernl/pull/27) | 2026-06-01 | Dispatch routing and autonomous mode. |
| [PR #26](https://github.com/gabrielassisxyz/kernl/pull/26) | 2026-06-01 | Inbox capture and triage. |
| [PR #25](https://github.com/gabrielassisxyz/kernl/pull/25) | 2026-06-01 | Ingest engine. |
| [PR #24](https://github.com/gabrielassisxyz/kernl/pull/24) | 2026-06-01 | Bookmarks module. |
| [PR #23](https://github.com/gabrielassisxyz/kernl/pull/23) | 2026-06-01 | Memory module. |
| [PR #22](https://github.com/gabrielassisxyz/kernl/pull/22) | 2026-06-01 | GUI shell. |
| [PR #18](https://github.com/gabrielassisxyz/kernl/pull/18) | 2026-05-24 | Subprocess escape hatch and custom workflow resolution. |
| [PR #17](https://github.com/gabrielassisxyz/kernl/pull/17) | 2026-05-24 | Autonomous epic integration, review, and PR. |
| [PR #16](https://github.com/gabrielassisxyz/kernl/pull/16) | 2026-05-24 | Assistant core. |
| [PR #15](https://github.com/gabrielassisxyz/kernl/pull/15) | 2026-05-23 | Vault watcher. |
| [PR #14](https://github.com/gabrielassisxyz/kernl/pull/14) | 2026-05-23 | Graph traversal and relevance. |

## Unreleased

### Dev environment, CI, and release path

This branch turns Kernl from a local-only prototype repo into something that can
be built, checked, packaged, installed, and released from the public repository.

Delivered capability:

- Replaced the dead CI workflow with a full `ci` workflow for formatting, vet,
  unit tests, web generation/tests, advisory lint, vulnerability checks, and
  secret scanning.
- Added `bin/ci`, `bin/install-hooks`, `.gitleaks.toml`, and `.golangci.yml` so
  local checks mirror the repository gates.
- Switched web dependency installation in CI/local scripts to `npm install` with
  Node 24 while the lockfile reproducibility issue remains open.
- Added GoReleaser config, a tag-triggered release workflow, `install.sh`,
  Dockerfile, Compose file, and Docker ignore rules.
- Hardened workflows with top-level permissions, concurrency, job timeouts,
  SHA-pinned actions, and Dependabot updates for GitHub Actions and Go modules.
- Hardened the installer so checksum verification is mandatory and latest-version
  resolution can fall back when the GitHub API is rate-limited.
- Added `kernl version`, `--version`, and build metadata variables populated by
  release ldflags.
- Rewrote the README around Kernl as an all-in-one local-first graph substrate
  plus orchestrator, removed the stale parallel demo GIF, and documented install,
  Docker, source-build, command, config, troubleshooting, and limitation paths.

Representative commits:

- [`ebacb5c`](https://github.com/gabrielassisxyz/kernl/commit/ebacb5c199e22b8f06e7c4faa5ec4ec7d9bf0b00) replaced the old workflow with full CI plus local scripts.
- [`f4a5419`](https://github.com/gabrielassisxyz/kernl/commit/f4a54199df60e7f7aa526032c89f9ae6d07d06b4) aligned web builds on `npm install` and Node 24.
- [`82dcc9b`](https://github.com/gabrielassisxyz/kernl/commit/82dcc9b1e464036e4227f94909bee60d7b7a5208) added GoReleaser, release workflow, installer, and Docker self-hosting.
- [`cb0dfa6`](https://github.com/gabrielassisxyz/kernl/commit/cb0dfa670a71eaf5fb9f91a37c5e081bda247a95) hardened workflows and added Dependabot.
- [`21ce9cc`](https://github.com/gabrielassisxyz/kernl/commit/21ce9cc5873b03a0f5addd76c1cee5cd8677f076) made installer checksum verification mandatory.
- [`22202d6`](https://github.com/gabrielassisxyz/kernl/commit/22202d6d5416cd1f25b9f69c9c45015aafbe78ac) added the version command and release ldflags.
- [`f69a7b0`](https://github.com/gabrielassisxyz/kernl/commit/f69a7b07ba91340064b4a7fcc11f5c180bc2450b) rewrote the README.

## Pre-release History

### Foundation and orchestrator bootstrap

Kernl began as a Go single-binary orchestration runner, then evolved into a
substrate-centered product. The early history established the CLI, config,
preflight checks, bead DAG execution, worktree isolation, event streams, and the
first monitoring UI.

Delivered capability:

- Imported and renamed the engine into the Kernl module and vocabulary.
- Added `kernl serve`, `kernl doctor`, `kernl bead run`, and `kernl epic run`.
- Added config loading, preflight checks, bead DAG loading, ready-set execution,
  semaphore-limited parallelism, worktree management, run-state storage, and
  SSE-based monitoring.
- Added integration harnesses and a packaged parallel demo, later superseded by
  the broader product README.

Representative commits:

- [`1878e55`](https://github.com/gabrielassisxyz/kernl/commit/1878e55933eb7774badd4487b6bc253435c94c0e) started the planning layer.
- [`2d4d3af`](https://github.com/gabrielassisxyz/kernl/commit/2d4d3af7be965e7cde3ebc99503d5c0037c896e8) imported the engine source.
- [`b8f3496`](https://github.com/gabrielassisxyz/kernl/commit/b8f3496f00807d5eef0937472ce1761dc9004157) introduced the Kernl binary commands.
- [`e0caeba`](https://github.com/gabrielassisxyz/kernl/commit/e0caeba249d07ff1138e6c07a87263cf21095a9b) wired `kernl epic run` to the executor and embedded GUI server.

### Graph substrate and vault watcher

The next wave made the graph the product center: SQLite-backed typed nodes and
edges, tags, FTS, revisions, traversal, relevance, and a markdown vault watcher
that keeps human-authored notes as files while indexing them into the graph.

Delivered capability:

- Added the graph package, migration runner, transaction boundaries, typed node
  CRUD, FTS, tags, revisions, and graph test utilities.
- Added traversal helpers, shortest path, depth-limited neighbors, and a
  structural relatedness scorer.
- Added the Note node type, frontmatter UUID injection, wikilink parsing,
  path-cache based rename handling, revision logging, tombstones, cold-start
  reconciliation, watcher lifecycle wiring, and an e2e vault watcher suite.

Representative commits:

- [`2e1ee35`](https://github.com/gabrielassisxyz/kernl/commit/2e1ee359712cf818fb648848b7ddc6b75b07850d) landed edges, tags, FTS search, and revision reads.
- [`5839753`](https://github.com/gabrielassisxyz/kernl/commit/58397538c9ea23bca7865f577c9af206b7f467a8) added traversal and relevance indexes.
- [`0c2423e`](https://github.com/gabrielassisxyz/kernl/commit/0c2423ee3c49b205b75f31684a3e7e73caa761a4) added the Note node migration.
- [`85b780a`](https://github.com/gabrielassisxyz/kernl/commit/85b780a946ae1da6e3d4dc4de5006f1fab8ddbde) added the vault watcher e2e acceptance suite.

### Digital assistant, workflow engine, and dispatch

Kernl then gained the persistent assistant surface, chat/session protocol, scope
and permissions, custom workflow support, subprocess stages, autonomous dispatch,
auditable decisions, and the epic integration-to-PR path.

Delivered capability:

- Added assistant identity and chat session node types, chat APIs, event flow,
  permissions, and UI surfaces.
- Added custom workflow YAML resolution, embedded canonical workflow parity tests,
  handoff payloads, subprocess runner support, and failure handling.
- Added autonomous workflow inference, hard gates, audit-decision nodes, CLI flags,
  and config-driven dispatch.
- Added epic-level integration, integration review, shipment, manual merge
  recovery, sweep support, and epic abort cleanup.

Representative commits:

- [`f175200`](https://github.com/gabrielassisxyz/kernl/commit/f175200cc9a972138ef4598fdbc385b2ce7ddcf3) added chat session and assistant identity node types.
- [`f9c86e2`](https://github.com/gabrielassisxyz/kernl/commit/f9c86e2ff8187ca154519547c3d60829420bf492) added chat protocol endpoints and the engine skeleton.
- [`a5c6479`](https://github.com/gabrielassisxyz/kernl/commit/a5c64792146a85f1033621adc1c206ba5edccd00) added autonomous epic integration, review, and PR creation.
- [`c2e38ae`](https://github.com/gabrielassisxyz/kernl/commit/c2e38ae4ce6d4566005eeaf65a8781478c29fb5f) added workflow handoff payloads and context storage.

Related PRs:

- [PR #16](https://github.com/gabrielassisxyz/kernl/pull/16) - assistant core.
- [PR #17](https://github.com/gabrielassisxyz/kernl/pull/17) - autonomous epic integration, review, and PR.
- [PR #18](https://github.com/gabrielassisxyz/kernl/pull/18) - subprocess escape hatch and custom workflow resolution.
- [PR #19](https://github.com/gabrielassisxyz/kernl/pull/19) - epic abort.

### Wave 2 modules and the magic loop

The Wave 2 work made Kernl feel like a product instead of isolated backend
pieces. Inbox, notes, bookmarks, memory, ingest, dispatch, and the GUI shell were
wired end-to-end, and the keystone substrate-aware planning slice landed.

Delivered capability:

- Added CLI and UI capture, inbox triage, conversion routing, pending APIs, and
  daily rollups.
- Added source-first note editing, autosave to revisions, conflict detection,
  diff suggestions, tag hierarchy, and visible authorship markers.
- Added bookmark archiving, imports, reader/highlighter UI, memory claims and
  refutations, ingest manifests, review queue, and structured extraction.
- Unified the graph database used by CLI, API, and watcher.
- Added `kernl plan` and the planning-context API so relevant vault notes are
  retrieved automatically for a planning topic.

Representative commits:

- [`2d898f0`](https://github.com/gabrielassisxyz/kernl/commit/2d898f05d9d76387a20e2c1837ea4dba531174d3) added the capture CLI and capture action model.
- [`1187ff5`](https://github.com/gabrielassisxyz/kernl/commit/1187ff55dd11cbb5e859b51cd8100fc67e6605ca) added the source-first notes editor.
- [`956c444`](https://github.com/gabrielassisxyz/kernl/commit/956c444a90cf8cd892815088aea4ba9b94d955ad) unified the graph database and hardened watcher/API routing.
- [`02a4d75`](https://github.com/gabrielassisxyz/kernl/commit/02a4d75d485d908643d983fef8d7e09e09a2bfb6) added substrate-aware planning context.

Related PRs:

- [PR #22](https://github.com/gabrielassisxyz/kernl/pull/22) - GUI shell.
- [PR #23](https://github.com/gabrielassisxyz/kernl/pull/23) - memory module.
- [PR #24](https://github.com/gabrielassisxyz/kernl/pull/24) - bookmarks module.
- [PR #25](https://github.com/gabrielassisxyz/kernl/pull/25) - ingest engine.
- [PR #26](https://github.com/gabrielassisxyz/kernl/pull/26) - inbox module.
- [PR #27](https://github.com/gabrielassisxyz/kernl/pull/27) - dispatch routing and autonomous mode.
- [PR #29](https://github.com/gabrielassisxyz/kernl/pull/29) - Wave 0-2 fixes and substrate-aware planning.

### Web product surfaces

The web app then moved from individual module surfaces toward a cohesive
workspace: live home data, chat, polished inbox/notes screens, Projects and Tasks
as human graph nodes, and a graph view with companion notes and wikilink
autocomplete.

Delivered capability:

- Served the Nuxt Home route at `/` and wired module data to live APIs.
- Added chat search over notes, Home layout improvements, inbox polish, note
  editor fixes, and web test coverage.
- Added Orchestrator, Projects, and Tasks screens.
- Modeled Projects and Tasks as graph nodes rather than orchestrator beads.
- Added graph visualization, node type registry, companion-note creation,
  wikilink autocomplete/editor wiring, node search, and edges APIs.

Representative commits:

- [`a6e27fc`](https://github.com/gabrielassisxyz/kernl/commit/a6e27fc20ddc9b77710348b1d8dce56a9919b9f7) added Orchestrator, Projects, and Tasks screens.
- [`2d64d8a`](https://github.com/gabrielassisxyz/kernl/commit/2d64d8a8d179aaeea6b19de742c60b169b838328) modeled Projects and Tasks as graph nodes.
- [`8d93389`](https://github.com/gabrielassisxyz/kernl/commit/8d93389b609b3ead8d95aabb95dffd3280ee922f) added graph view, companion notes, and wikilink autocomplete.

Related PRs:

- [PR #30](https://github.com/gabrielassisxyz/kernl/pull/30) - live UI data and Home route.
- [PR #31](https://github.com/gabrielassisxyz/kernl/pull/31) - screen review polish.
- [PR #32](https://github.com/gabrielassisxyz/kernl/pull/32) - notes editor improvements.
- [PR #33](https://github.com/gabrielassisxyz/kernl/pull/33) - Orchestrator, Projects, and Tasks screens.
- [PR #34](https://github.com/gabrielassisxyz/kernl/pull/34) - graph view, companion notes, and wikilink autocomplete.

## Notes for Agents

- Start with the Version Timeline to understand whether a public version exists.
- Use the Unreleased section for the current release-prep branch.
- Use the Pre-release History sections for architectural orientation.
- Commit links are implementation evidence; PR links are useful for branch-level
  intent and discussion.
- Do not treat GoReleaser-generated GitHub Release notes as a replacement for
  this file. They are release artifacts; this is project memory.
