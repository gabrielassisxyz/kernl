# Kernl

[![CI](https://github.com/gabrielassisxyz/kernl/actions/workflows/ci.yml/badge.svg)](https://github.com/gabrielassisxyz/kernl/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)](https://go.dev/)

Kernl is a local-first workspace for solo developers: notes, captures, bookmarks,
projects, tasks, memory, and multi-agent execution all living in one typed graph.

It is built for the problem of having one tool for notes, another for tasks,
another for bookmarks, another for planning context, and another for agent
orchestration with almost no shared state between them. Kernl's bet is that these
are not separate products. They are one substrate.

Kernl is opinionated out of the box and deeply configurable when you need it. The
default path should stay boring: point it at a vault and a repo, run the server,
capture ideas, ask for planning context, and watch work move. The advanced path is
there for technical users: custom workflow shapes, agent pools, repository
registries, and self-hosted runtime state without turning first-run setup into a
configuration project.

## What It Does

| Area | What Kernl provides |
| --- | --- |
| Knowledge graph | Notes, captures, bookmarks, tasks, projects, memory claims, sessions, and workflow runs in one SQLite-backed graph. |
| Markdown vault | Plain `.md` files remain human-owned; Kernl indexes them, injects stable UUIDs, and preserves revision history. |
| Substrate-aware planning | `kernl plan "topic"` and the planner API retrieve relevant vault notes automatically before work starts. |
| Inbox and capture | Quick captures enter the graph as pending items and can be converted into durable notes with provenance. |
| Bookmarks | Add or import bookmarks, archive readable HTML, and connect them to the rest of the graph. |
| Multi-agent orchestration | Execute bead DAGs with isolated git worktrees, agent pools, review stages, integration, and PR shipment. |
| Web UI | Serve the embedded Nuxt UI and REST/SSE API from the same Go binary. |

## Current Status

Kernl is pre-1.0 and actively shaped around a solo-developer workflow. The graph,
vault watcher, capture path, bookmarks, memory, planning context, web shell, and
orchestrator core exist. The orchestrator's epic-to-PR path is implemented and
covered by hermetic tests, but still needs more real-world runtime mileage against
live agent CLIs.

Use it if you are comfortable with local-first developer tooling and want one
system that joins personal knowledge with agentic execution. Do not use it yet if
you need a polished team SaaS, mobile-first PKM app, or hosted task manager.

## Installation

### Released Binary

The installer downloads a checksummed release archive for Linux or macOS and
installs `kernl` into `~/.local/bin` by default:

```bash
curl -fsSL https://raw.githubusercontent.com/gabrielassisxyz/kernl/master/install.sh | bash
```

Install a specific version:

```bash
KERNL_VERSION=v0.1.0 curl -fsSL https://raw.githubusercontent.com/gabrielassisxyz/kernl/master/install.sh | bash
```

The installer expects a published GitHub Release. Until the first `v*` release
exists, build from source.

### From Source

Kernl embeds the generated Nuxt site into the Go binary. On a fresh checkout, build
the web assets before any Go build:

```bash
git clone https://github.com/gabrielassisxyz/kernl.git
cd kernl

cd web
npm ci
npm run generate
cd ..

go build -o ./kernl ./cmd/kernl
./kernl version
```

For local development, `run.sh` builds the web and binary together.

### Docker

Docker is the quickest way to run the API and web UI:

```bash
cp kernl.yaml.example kernl.yaml
docker compose up --build
```

Open `http://localhost:8080`.

Docker is best for the graph, notes, API, and UI experience. Full orchestration
still needs host tools and credentials such as `git`, `gh`, `bd`, and agent CLIs.

## Prerequisites

Basic graph and UI usage:

- Go 1.26+ when building from source
- Node.js 24 and npm when regenerating the embedded web UI
- A local `kernl.yaml`
- An optional markdown vault directory

Full orchestration:

- `bd` from [gastownhall/beads](https://github.com/gastownhall/beads), version 1.0.4 or newer
- An agent CLI configured for non-interactive runs
- `gh` authenticated for PR operations
- Git with worktree support
- A repository entry in `kernl.yaml`

Run the doctor before serious use:

```bash
kernl doctor
```

## Quick Start

```bash
# 1. Create local config.
cp kernl.yaml.example kernl.yaml

# 2. Edit registry.repos and, optionally, vault.root.
$EDITOR kernl.yaml

# 3. Check local dependencies and config.
kernl doctor

# 4. Start the API and embedded web UI.
kernl serve

# 5. Capture an idea into the inbox.
kernl capture "Investigate semantic relevance for converted captures"

# 6. Ask Kernl which vault notes are relevant before planning work.
kernl plan "semantic relevance"
```

The server defaults to `http://localhost:8080`. Override it with `--port` or the
`server.port` value in `kernl.yaml`.

## Commands

```bash
kernl serve
```

Start the REST/SSE API and embedded web UI.

```bash
kernl doctor
```

Validate local prerequisites and config.

```bash
kernl capture "text to save"
printf "text from stdin" | kernl capture
```

Create a pending capture in the graph-backed inbox.

```bash
kernl plan "topic"
```

Show the vault notes Kernl would bring into scope for planning that topic.

```bash
kernl bookmark add https://example.com
kernl bookmark import pocket ./pocket-export.html
kernl bookmark import pinboard ./pinboard-export.json
```

Create or import bookmarks.

```bash
kernl epic list
kernl epic run <epic-id>
kernl epic run --workflow ./workflow.yaml <epic-id>
kernl epic run --autonomous <epic-id>
kernl epic merge <epic-id>
kernl epic abort <epic-id>
```

Manage and execute epic bead graphs.

```bash
kernl bead run <bead-id>
```

Run one bead through the configured agent dispatch path.

```bash
kernl sweep
```

Close epics whose PRs have already merged.

```bash
kernl version
kernl --version
```

Print build metadata.

## Configuration

Start from `kernl.yaml.example`:

```yaml
settings:
  agents:
    agent-cli:
      command: your-agent-cli
      args: ["run", "--format", "json"]
      type: generic
      vendor: local
      model: default
      label: worker
      approvalMode: auto

  pools:
    implementation:
      agents:
        - agentId: agent-cli
          weight: 1
    implementation_review:
      agents:
        - agentId: agent-cli
          weight: 1
    integration:
      agents:
        - agentId: agent-cli
          weight: 1
    integration_review:
      agents:
        - agentId: agent-cli
          weight: 1
    shipment:
      agents:
        - agentId: agent-cli
          weight: 1

registry:
  repos:
    - path: /home/me/projects/example
      memoryManager: beads

server:
  port: 8080

vault:
  root: /home/me/vault
```

Key ideas:

- `settings.agents` defines the agent CLI commands Kernl can dispatch.
- `settings.pools` maps workflow stages to weighted agent pools.
- `registry.repos` tells Kernl which repositories it can operate on.
- `server.port` controls the API and UI port.
- `vault.root` enables markdown-vault indexing and graph integration.

## Architecture

```text
                 +---------------------+
                 |     kernl binary    |
                 |  Go CLI + REST/SSE  |
                 +----------+----------+
                            |
          +-----------------+-----------------+
          |                                   |
          v                                   v
+-------------------+              +-------------------+
| Embedded Nuxt UI  |              | CLI commands      |
| graph, tasks      |              | capture, plan,    |
| orchestrator      |              | epic, bead        |
+---------+---------+              +---------+---------+
          |                                  |
          +----------------+-----------------+
                           |
                           v
              +-------------------------+
              | Typed knowledge graph   |
              | SQLite + FTS5           |
              +-----------+-------------+
                          |
       +------------------+------------------+
       |                                     |
       v                                     v
+---------------+                  +-------------------+
| Markdown vault|                  | Orchestrator state |
| user-owned md |                  | bd + worktrees     |
+---------------+                  +-------------------+
```

The graph is the unifier. User notes stay as markdown files. Operational objects
such as captures, bookmarks, tasks, memory claims, sessions, and workflow state are
stored as graph nodes and edges.

## Development

Run the local CI script before pushing:

```bash
bin/ci
```

Install the pre-commit hook once after cloning:

```bash
bin/install-hooks
```

Useful checks:

```bash
gofmt -w ./cmd ./internal
go vet ./...
go test ./...
```

Integration tests are manual-only:

```bash
go test -tags=integration ./...
```

They require real local tools such as `bd` and an agent CLI, and may spend paid
provider tokens. They are not part of default CI.

## Troubleshooting

### `pattern all:.output/public: no matching files found`

The embedded web UI has not been generated yet.

```bash
cd web
npm ci
npm run generate
cd ..
go test ./...
```

### `KERNL DISPATCH FAILURE: no repos registered`

Add a repository to `registry.repos` in `kernl.yaml`.

### `kernl serve` cannot bind to port 8080

Another process is using the default port.

```bash
kernl --port 8081 serve
```

### Docker starts, but orchestration does not work

The Docker setup is for the API and web UI. Full orchestration needs host
credentials and tools: `git`, `gh`, `bd`, and your configured agent CLIs.

### `npm ci` reports a lockfile mismatch in `web/`

Use Node.js 24, then regenerate the lockfile deliberately:

```bash
cd web
npm install --package-lock-only --ignore-scripts --no-audit --no-fund
npm ci
```

## Limitations

- Kernl is single-user and local-first, not a hosted collaboration product.
- Windows release binaries are not currently shipped; Windows users should use Docker.
- The orchestrator's live epic-to-PR loop needs more runtime validation with real agent CLIs.
- Semantic relevance is still evolving. Current graph relevance is strongest when notes, captures, and entities already share links, tags, or source relationships.
- The Docker path does not include the full host orchestration toolchain.

## Contributing

Issues and PRs are welcome. A full contributor guide and issue/PR templates are
planned after the first tagged release, when there is a stable binary and version
output to reference.

For now:

- Open issues with a clear reproduction, expected behavior, actual behavior, OS/arch, and `kernl version` output.
- Keep PRs focused and small.
- Run `bin/ci` before opening a PR.
- Update docs when changing user-facing behavior.

## License

MIT. See [LICENSE](LICENSE).
