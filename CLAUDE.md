# Kernl — Master Agent Briefing (AGENTS.md)

> **Context:** Kernl is the multi-agent orchestration core that executes epics as bead-graphs in true parallel. This repo holds the planning layer (docs, Claude Code skills) and the Go `orchestrator/` block (ported from foolery-go).
> **Core Value:** The human touches only judgment gates; the rest is a dependency graph of beads executed in parallel without continuous supervision. Main metric: zero out-of-gate interventions per epic.

## 1. Stack & Commands
- **Backend/CLI:** Go 1.26+. Issue tracker: `br` (beads_rust, https://github.com/Dicklesworthstone/beads_rust) is the agent-facing CLI; `bv` is the TUI viewer (https://github.com/Dicklesworthstone/beads_viewer). The Go orchestrator still shells out to **`bd`** (gastownhall/beads ≥ 1.0.4) — migration of the Go call sites is tracked as a follow-up. Both `br` and `bd` operate on the same `.beads/issues.jsonl`. Dolt, SQLite (run-state), YAML config (`kernl.yaml`).
- **Agent Runtime State:** `~/.kernl/state/<bead-id>.json` is the canonical per-bead runtime store (heartbeats, follow-up counts, watchdog state). Purgeable for reset — the orchestrator reconstructs it from bead metadata on restart.
- **MVP era / Vision era:** today the orchestrator delegates persistence to `bd` (Dolt-backed). Per `docs/VISION.md` §6, the destination is a unified typed knowledge graph in `~/.kernl/graph.db` (SQLite) where `Bead` is just one node type. `bd` is the **MVP backend**; the unified substrate (P0.1 in `docs/suggested-vision-projects.md`) absorbs it later. Do NOT design new persistence around the bd boundary as if it were permanent.
- **Frontend (future):** Vue 3 (Composition API) + Nuxt (per VISION §12).
- **API:** REST JSON + SSE (not gRPC/WebSocket)
- **Run:** `go run ./cmd/kernl` (from repo root)
- **Test:** `go test ./...` — Run before every commit. Hermetic by default.
- **Test (integration):** `go test -tags=integration ./...` — opt-in, manual only.
- **Lint/Format:** `go vet ./...` + `go fmt ./...` + `golangci-lint run`

## 2. Architectural Principles
- **CLI-First Backend (MVP era):** The Go backend delegates ALL storage mutations to the `bd` CLI. Never writes `.beads/issues.jsonl` directly. (Vision era: this collapses into direct SQLite writes against the unified graph — see §1.)
- **Goroutine-per-Session:** Each terminal session lives in its own goroutine. No shared EventEmitter; use channels.
- **No Shared Mutable State:** `sync.Map` or `map + RWMutex` for registries. Never hold unguarded pointers across goroutines.
- **YAGNI & Flat:** Do NOT generate preventive abstractions. No single-use interfaces, no mappers. Keep file structure flat.
- **Fail Loud, Never Silent:** When a lookup for a configured resource (agent, pool, backend, workflow) fails, the code MUST:
  1. Return an error that halts the current operation.
  2. Log a red ANSI banner via `log.Fatalf` or structured error.
  3. Surface the failure to any visible session buffer as a stderr banner event.
  4. Include the greppable marker `KERNL DISPATCH FAILURE` (or subsystem marker).
  5. Name the missing thing (bead id, state, pool key, workflow id, action name) and the exact config that fixes it.
  6. NEVER return a fallback like `Object.values(x)[0]` or `?? "default"`.
- **Comprehension Debt (ADR):** NEVER make silent architectural decisions. If adding a dependency/pattern, update `docs/architecture.md`.
- **Blast Radius (Async Review):** If an edit affects multiple domains, proceed, BUT isolate work in a branch. You MUST:
  1. Create a `bd` issue flagged for `human` code review.
  2. Clearly state "BLAST RADIUS WARNING" at the top of the Pull Request description.
  3. NEVER merge this PR autonomously.

## 3. Code Style & Clean Code
- **Functions:** 4-40 lines. Split if longer. One thing per function (SRP).
- **Files:** Under 500 lines. Split by responsibility.
- **Names:** Specific and unique. Avoid `data`, `handler`, `Manager`.
- **Types:** Explicit. No `any`, no untyped functions. Use Go interfaces for boundaries (BackendPort, Transport), but not for single-use indirection.
- **Control Flow:** Early returns over nested ifs. Max 2 levels of indentation.
- **Dependencies:** Inject dependencies. Wrap third-party libs behind thin interfaces owned by this project.
- **Errors:** Return errors, don't panic. Use `fmt.Errorf("KERNL DISPATCH FAILURE: %s not found in pool %s", agentID, poolKey)` for loud failures.

## 4. Tests & TDD (Action via TDD)
- **Reasoning is Coding:** Do NOT write explanatory essays. Your "reasoning" is writing a failing test.
- **Hermetic by Default:** Tests MUST NOT touch the host environment. No `os.Getenv`, no real `os.Open`, no real `exec.Command`, no real network or ports. Mock at boundaries via interfaces.
- **TDD Discipline:** 1. Write failing test. 2. Write minimal code to pass. 3. Refactor.
- **Mocks:** Use named fakes / stub structs implementing interfaces. No inline anonymous mocks in tests.
- **Integration Tests (Tagged):** Use `//go:build integration`. These may touch `bd` CLI and real processes, but run ONLY manually via `go test -tags=integration ./...`. Never in the default `go test ./...`.
- **Race Detector:** Integration tests SHOULD use `go test -race ./...` in CI when running tagged suites.

## 5. Comments & Logging
- **Comments:** WHY, not WHAT. Docstrings on public functions: intent + 1 usage example.
- **Logs:** Structured JSON for debugging (`log/slog`). Plain text ONLY for user-facing output.

## 6. Living Documentation & Session Notes
Update documentation on every relevant change. Never invent terminology.
- **Session notes (dev-process, NOT product memory):** `docs/activeContext.md` and `docs/progress.md` are agent-handoff scratchpads for the build of Kernl itself. They are NOT the additive `MemoryClaim` model from VISION §7.3 — that one is the product's own memory subsystem (separate concern; see P2.2 in `docs/suggested-vision-projects.md`). For persistent dev knowledge, use **Claude Code auto-memory** at `~/.claude/projects/-home-gabriel-repositories-kernl/memory/` (write a memory file + index it in `MEMORY.md`). The legacy `bd remember` / `bd memories` store still exists but is no longer the recommended path — `br` has no memory subsystem, so all new dev-knowledge entries go to auto-memory.
- **`docs/glossary.md`:** Read/update for domain terms to maintain Ubiquitous Language.
- **`docs/architecture.md` & `docs/features.md`:** Update for structural/capability changes.
- **Specs (`orchestrator/specs/*.md`):** Authoritative for behavior. The orchestrator is Go (ported from foolery-go, also Go) — there is no TS source to cite.

## 7. Git & Agent Behavior
- **Branch per task:** `feat/[short-name]` or `fix/[short-name]`. Never work on `main`.
- **Atomic Commits:** One commit per feature/fix. Message: `type: what changed`.
- **Push:** Run `git push` after every commit. Unpushed work is lost work.
- **Anti-Overwrite:** NEVER overwrite a file without first verifying its current state (via `git diff` or reading). Beware of silent truncation on large files.
- **No Thinking Loops:** If you make ≥2 tool calls without executing a change, STOP deliberating. Implement your best-effort solution immediately and iterate based on the result.
- **Anti-Regression:** Run `go vet ./...` and `go test ./...` before declaring success.

## 8. Known Hurdles & Evolution
- **Stack Quirks:**
  - `bd` CLI output is NDJSON (newline-delimited JSON). Parse with `bufio.Scanner` + `json.Decoder`.
  - Dolt transactions via `bd` are ACID. No manual file locking needed.
  - SSE in Go: use `net/http` with `w.Header().Set("Content-Type", "text/event-stream")` and `fmt.Fprintf(w, "data: %s\n\n", jsonStr)`.
  - Vue 3 reactivity uses proxies. Mutating state outside Vue's lifecycle can miss updates. Always mutate reactive refs.
- **Resolved Bugs:** [Add tricky bugs solved so they aren't reintroduced]

## 9. Issue Tracking (br / bv / beads)
- **MANDATORY:** Use the `br` CLI for ALL agent-facing task tracking. Run `br --help` or `br robot-docs` for the command reference.
- **FORBIDDEN:** Do NOT create `todo.md` or use external trackers.
- **Two binaries, one data:** `br` (beads_rust) is the agent-facing CLI; `bv` is the TUI viewer for human triage. Both operate on `.beads/`. The Go orchestrator still shells out to the legacy `bd` binary — leave that path alone until the Go-code migration bead lands.
- **Rules:**
  - Append `--json` to `br` commands for parsing (most subcommands support it).
  - Run `br ready --json` before asking for work.
  - Claim tasks atomically: `br update <id> --status=in_progress --json`.
  - Link discovered work: `br create --title="Found bug" --priority=1 --type=bug` then `br dep add <new-id> <parent-id> --kind=discovered-from`.
  - Sync to disk before ending session: `br sync --flush-only` then `git add .beads/` and commit.
- **Triage UI:** `bv` for graph-aware browsing, bottleneck identification, and dependency visualization. Read-only by default — won't mutate state.

## 10. The Collaborative Yegge Loop (Planning Tasks)
When generating a plan, ADR, or Beads epics, execute this loop:
1. Output Iteration 1 (your absolute best, production-ready attempt) and STOP. Prompt user for feedback.
2. Cross-reference user feedback against core principles.
3. If feedback contradicts rules, point out the error and ask how to proceed.
4. If aligned, output the next iteration. Repeat until explicit approval (up to 5x).

## 11. Session Completion (Exit Checklist)
Work is NOT complete until tests pass and `git push` succeeds.
1. File issues for remaining work via `bd`.
2. Update the Memory Bank (`activeContext.md`, `progress.md`) before stopping.
3. Run quality gates (`go vet ./...`, `go test ./...`, `golangci-lint run`).
4. `git pull --rebase` -> `git push`.

## 12. Skill Routing Guide

The kernl dev workflow is built on three layers of skills. Pick the right entry point for the user's intent — don't guess; the wrong skill at the wrong layer wastes a cycle.

### Layer 1 — Kernl-specific workflow skills (project-local, in `.claude/skills/` and `.opencode/skills/`)

These are the **canonical entry points** for working on kernl. Each is a router that invokes the underlying jeffrey-skills / compound-engineering skills in the right order with kernl-specific conventions.

| Trigger / intent | Skill | What it does |
|---|---|---|
| "start a kernl feature", "plan a kernl feature", "new pipeline", source-project from `docs/suggested-vision-projects.md` | `/kernl-dev` | Full external dev pipeline: CE-based brainstorm → plan → beads → orchestrator run → PR review → compound. Two human gates: plan approval and PR review. Everything else is autonomous. |
| "audit kernl", "pre-milestone check", "is kernl ready", "where are we", "reality check" | `/kernl-audit` | Pre-milestone rotation: beads compliance, mock-finder, reality-check, ubs, conformance, domain audit. Run BEFORE declaring anything shipped. |
| "kernl is stuck", "kernl epic run hangs", "debug kernl", "stage flake" | `/kernl-debug` | Triage router for kernl-specific failure modes (GDB attach for hangs, deadlock-finder for races, multi-pass for second-look bugs). |
| "clean kernl repo", "tidy worktrees", "kernl hygiene" | `/kernl-hygiene` | Worktree rationalization, stash archaeology, repo junk cleanup, stale per-bead state. Run AFTER `/kernl-audit`, never before. |

### Layer 2 — Compound-engineering pipeline (used internally by `/kernl-dev`)

The CE skills are the **building blocks** that `/kernl-dev` orchestrates. Invoke directly only if you need fine-grained control over one phase.

| Skill | Phase |
|---|---|
| `/ce-strategy` | Maintain `docs/STRATEGY.md` (kernl already has one). |
| `/ce-ideate` (or `/idea-wizard` / `/dueling-idea-wizards` as alternates) | Optional high-level ideation. |
| `/ce-brainstorm` | Requirements doc → `docs/brainstorms/`. |
| `/ce-plan` | Implementation plan → `docs/plans/`. |
| `/beads-workflow` | Plan → bead graph (replaces the legacy `vc-convert-plan-to-beads`). |
| `/ce-code-review` | Multi-persona review, invoked by orchestrator `implementation_review` / `shipment_review` stages. |
| `/ce-compound` | Post-shipment knowledge artifact → `docs/solutions/` (frontmatter compatible with future graph node schema). |

The legacy `vc-*` skills (`vc-brainstorm`, `vc-plan`, `vc-writing-plans`, `vc-convert-plan-to-beads`, `vibe-engineering-mastery`, `vibe-chaos-to-concept`) are **being deprecated** for kernl. Do not start new cycles on them. See `docs/spikes/2026-05-19-compound-engineering-pipeline-adoption.md` for the transition plan.

### Layer 3 — Periodic / ad-hoc skills

Catalog at `docs/dev-workflow-skills.md` (trigger + frequency + command). Highlights:

- **Pre-PR every time:** `/ubs` (bug scan).
- **Before any milestone:** `/beads-compliance-and-completion-verification`, `/mock-code-finder`, `/reality-check-for-project`.
- **When `kernl epic run` hangs:** `/gdb-for-debugging`. When stages flake: `/deadlock-finder-and-fixer`.
- **Monthly:** `/library-updater`.
- **Worktree debris:** `/git-worktree-branch-rationalization` and friends (invoked via `/kernl-hygiene`).
- **Before deciding to add a dep:** `/research-software` (already used informally — promote to mandatory).
- **CLI surface stabilization:** `/agent-ergonomics-and-intuitiveness-maximization-for-cli-tools` (the kernl CLI is consumed primarily by agents).

These all become **automated cron shapes** under suggested project **P3.8 — Scheduled maintenance workflows** (see `docs/suggested-vision-projects.md`). Until then, run manually at the documented triggers.

### Routing decision tree

When the user gives an ambiguous request:

```
Is the user starting new work?
  └─ Yes → /kernl-dev
  └─ No → continue

Is the user checking quality / readiness?
  └─ Yes → /kernl-audit
  └─ No → continue

Is something broken in kernl itself (hang, flake, wrong output)?
  └─ Yes → /kernl-debug
  └─ No → continue

Is the repo cluttered (worktrees, stashes, junk files)?
  └─ Yes → /kernl-hygiene
  └─ No → look at Layer 3 for the specific ad-hoc skill;
          if no match, ask the user to clarify before guessing.
```

### Skill substrate

- Skills live in `.claude/skills/<skill-name>/SKILL.md` for Claude Code and `.opencode/skills/<skill-name>/SKILL.md` for OpenCode. Content is identical between platforms unless platform-specific tool names are in play (e.g., `AskUserQuestion` in Claude Code vs `ask_user` in OpenCode).
- OpenCode hooks live in `.opencode/hooks/hooks.yaml` (requires the `@gabrielassisxyz/opencode-hooks` plugin). Claude Code hooks live in `.claude/settings.json`.
- The `kernl-*` skills are **kernl-internal**: they reference and orchestrate global skills from `~/.claude/skills/` and `~/.codex/skills/`. They are not standalone — if a global skill is missing, the kernl-* skill will tell the user which one to install.

<!-- bd BEADS INTEGRATION block removed during bd→br migration on 2026-05-19;
     br agents --add injects its replacement below. -->


<!-- br-agent-instructions-v1 -->

---

## Beads Workflow Integration

This project uses [beads_rust](https://github.com/Dicklesworthstone/beads_rust) (`br`/`bd`) for issue tracking. Issues are stored in `.beads/` and tracked in git.

### Essential Commands

```bash
# View ready issues (open, unblocked, not deferred)
br ready              # or: bd ready

# List and search
br list --status=open # All open issues
br show <id>          # Full issue details with dependencies
br search "keyword"   # Full-text search

# Create and update
br create --title="..." --description="..." --type=task --priority=2
br update <id> --status=in_progress
br close <id> --reason="Completed"
br close <id1> <id2>  # Close multiple issues at once

# Sync with git
br sync --flush-only  # Export DB to JSONL
br sync --status      # Check sync status
```

### Workflow Pattern

1. **Start**: Run `br ready` to find actionable work
2. **Claim**: Use `br update <id> --status=in_progress`
3. **Work**: Implement the task
4. **Complete**: Use `br close <id>`
5. **Sync**: Always run `br sync --flush-only` at session end

### Key Concepts

- **Dependencies**: Issues can block other issues. `br ready` shows only open, unblocked work.
- **Priority**: P0=critical, P1=high, P2=medium, P3=low, P4=backlog (use numbers 0-4, not words)
- **Types**: task, bug, feature, epic, chore, docs, question
- **Blocking**: `br dep add <issue> <depends-on>` to add dependencies

### Session Protocol

**Before ending any session, run this checklist:**

```bash
git status              # Check what changed
git add <files>         # Stage code changes
br sync --flush-only    # Export beads changes to JSONL
git commit -m "..."     # Commit everything
git push                # Push to remote
```

### Best Practices

- Check `br ready` at session start to find available work
- Update status as you work (in_progress → closed)
- Create new issues with `br create` when you discover tasks
- Use descriptive titles and set appropriate priority/type
- Always sync before ending session

<!-- end-br-agent-instructions -->

---

## bv — TUI Beads Viewer

`bv` (https://github.com/Dicklesworthstone/beads_viewer) is the graph-aware TUI viewer for beads workspaces. It reads the same `.beads/` data that `br` writes and is **read-only by default** — won't mutate state. Use it for triage, dependency exploration, and bottleneck identification.

### When to use bv vs. br

- Use **`br`** for any **mutation** (create/update/close/dep) or for scripted reads (`--json` output).
- Use **`bv`** for **human triage** when you need to see the dependency graph, find bottlenecks, or browse beads visually.

### Essential invocations

```bash
bv                        # Launch interactive TUI on the current .beads/ workspace
bv --db <path>            # Point at a specific database / directory
bv --version              # Version check
bv --update               # Self-update to latest release
```

### Robot mode (structured output for agents)

```bash
bv -f json                # Machine-readable output
bv --robot-by-label <l>   # Filter robot outputs by label
bv --robot-by-assignee <a># Filter by assignee
bv --robot-min-confidence # Filter by min confidence (0.0-1.0)
bv --robot-max-results <n># Limit count
```

`-f toon` is also supported (smaller token footprint than JSON for the same payload). Set `TOON_STATS=1` to compare sizes on stderr.

### Performance

Defaults are tuned for graphs ≤ a few thousand beads. For large graphs, pass `--force-full-analysis` only when needed (slow). The `--profile-startup` flag emits timing diagnostics for debugging cold-start latency.

### Anti-patterns

- Don't use `bv` to mutate state via the TUI key-bindings during agent sessions — keep mutations on `br` so the audit trail in `.beads/interactions.jsonl` is clean.
- Don't rely on `bv` output structure in scripts — use `bv --robot-* -f json` for the stable contract.
