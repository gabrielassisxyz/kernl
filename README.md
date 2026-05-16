# Kernl

Multi-agent orchestration where humans only touch judgment points — the rest is a dependency graph executed in parallel.

![Kernl parallel demo](docs/assets/parallel-demo.gif)

> Open source from the start, projetado em blocos — each person uses whichever block they want and configures it their way. Born to solve a personal pain point, aimed at being available to everyone.

## Prerequisites

- **Go 1.26+** — the orchestrator backend
- **[bd](https://github.com/gastownhall/beads) ≥ 1.0.4** — issue tracking CLI (storage backend)
- **[opencode](https://github.com/anomalyco/opencode)** — agent CLI (or any Claude Code-compatible agent)
- **[gh](https://cli.github.com/)** — GitHub CLI (used for sweeping/epic integration)
- **git worktree** — required for parallel bead isolation (comes with Git 2.5+)

## Quickstart

```bash
# 1. Copy and customize the config
cp orchestrator/kernl.yaml.example kernl.yaml

# 2. Verify your setup
go run ./orchestrator/cmd/kernl doctor

# 3. Run the packaged parallel demo (3 beads, 2 waves, real parallelism)
go run ./orchestrator/cmd/kernl epic run parallel-demo

# 4. Run an individual bead
go run ./orchestrator/cmd/kernl bead run <bead-id>
```

## Monitoring

The orchestrator serves a minimal monitoring GUI at `/` when running. It consumes the epic-SSE stream to show live bead state, active sessions, and errors — no framework, no build step.

```bash
# Start the server (default :8420)
go run ./orchestrator/cmd/kernl serve

# Open the printed URL, enter an epic id, and watch beads change state in real time.
# Falls back to polling /api/beads every 2s if SSE fails to connect.
```

## Parallel Demo

The repository includes a packaged epic at `examples/parallel-demo/` that demonstrates Kernl's parallel execution model:

```
a ──→ b
  └──→ c
```

Bead `a` (Setup) runs first in wave 1. After it completes, beads `b` (Frontend) and `c` (Backend) run concurrently in wave 2. The DAG is computed at runtime — you configure only the dependencies, and Kernl schedules the waves.

```bash
kernl epic run parallel-demo
```

Each bead dispatches a real opencode agent that runs through the take loop: claim → resolve pool → spawn → monitor → classify outcome → advance/retry/rollback/terminate. Capped at 5 follow-up turns per state.

> **Note:** Integration tests (`go test -tags=integration ./...`) and the packaged example spend real opencode API tokens and wall-clock time. Run them manually and set a spending cap.
