# Kernl

Multi-agent orchestration where humans only touch judgment points — the rest is a dependency graph executed in parallel.

<!-- GIF: examples/parallel-demo -->

## Prerequisites

- **Go 1.26+** — the orchestrator backend
- **[bd](https://github.com/gastownhall/beads)** — issue tracking CLI (storage backend)
- **[opencode](https://github.com/anomalyco/opencode)** — agent CLI (or any Claude Code-compatible agent)

## Quickstart

```bash
# 1. Copy and customize the config
cp orchestrator/kernl.yaml.example kernl.yaml

# 2. Verify your setup
go run ./orchestrator/cmd/kernl doctor

# 3. List epics
go run ./orchestrator/cmd/kernl epic list

# 4. Run a bead
go run ./orchestrator/cmd/kernl bead run <bead-id>
```

> **Note:** Integration tests (`go test -tags=integration ./...`) and the packaged example spend real opencode API tokens and wall-clock time. Run them manually and set a spending cap.
