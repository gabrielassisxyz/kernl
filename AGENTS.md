# Kernl: Master Agent Briefing

> **Read this before every interaction.** It is the living public project contract: short, imperative, and action-oriented. Keep stable repository conventions, user-visible constraints, and reproducible build hazards here. Do not turn it into a session log: one-off progress, scratch plans, and harness-specific learnings belong in `local/` or the harness memory.
>
> This is the single canonical agent-instruction file for all harnesses (Claude Code, Codex, OpenCode, …). `CLAUDE.md` just points here. Both are tracked: a clone that does not carry them starts without the project's conventions, and so does the agent working in it.

> **Context:** Kernl is an open-source, block-based, **single-binary (Go)** platform for a solo dev: a **knowledge-graph substrate** (notes, bookmarks, captures, memory) fused with a **multi-agent orchestrator** that executes epics as bead-graphs in true parallel. **Core value:** the human touches only judgment gates; the rest is a dependency graph executed in parallel without continuous supervision. The validated product soul is **substrate-aware planning**: your notes land in the planner's context automatically.

## 0. Project context

`docs/` is for documentation that ships with the public repo, and `docs/GLOSSARY.md` is tracked: keep Ubiquitous Language consistent when changing public concepts. Markdown that is only useful while developing (handoffs, baselines, scratch plans, tracking docs) never belongs in `docs/` or at the repo root.

It belongs in **`local/`** instead, the one git-ignored place for everything the maintainer keeps outside the published project: notes, plans, research, and machine-generated agent-run output alike. A checkout without it is normal, not a broken clone.

Durable learnings that a future session should inherit (tool drift, recurring gotchas, commands that worked, architectural decisions) go to the harness's memory, not into this file. Routine progress does not go anywhere: promote only a learning that is reproducible and whose evidence you can name.

## 1. Stack & Commands

From a fresh clone, in this order:

```bash
cd web && npm install && npm run generate   # first: go:embed needs web/.output/public
bin/install-hooks                           # once per clone
go build -o kernl ./cmd/kernl && ./kernl serve   # build + serve on :8080
bin/ci                                      # before pushing
```

Or, without a Go toolchain: `docker compose up --build` (see §11).

The order matters and the first line is not optional: `//go:embed` resolves at compile
time, so a build that runs before `npm run generate` fails outright. See §1's embed note.

- **Backend/CLI:** Go 1.26+. Single binary built from `./cmd/kernl`.
- **Orchestrator storage:** per repository, declared by `registry.repos[].memoryManager`. Two trackers are supported: **`br`** (beads_rust ≥ 0.2.10, SQLite) and **`bd`** (gastownhall/beads ≥ 1.0.4, Dolt embedded). `br` is what the repositories this orchestrator serves actually use, so it is the one a new target repository will need. This is the *product's* runtime store for executing epics, **not** this repo's own dev-task tracker; that is §6, and conflating the two is how a tracker ends up proposed as a backlog backend. `bv` is not used and stays out. `.beads/` is gitignored (no task data in the public repo).
- **The two trackers share the `.beads/` directory name and nothing else.** `bd` stores `.beads/embeddeddolt/`, `br` stores `.beads/beads.db` (SQLite). Detection is on those contents, never on the directory name. `br` also has **no `-C`**: it finds its database by walking up from the working directory, which never works from a worktree, so every invocation - kernl's own and the ones written into stage prompts - carries `--db <repo>/.beads/beads.db`.
- **Runtime state:** `~/.kernl/state/<bead-id>.json` per-bead store (heartbeats, follow-up counts, watchdog). Purgeable, reconstructed from bead metadata on restart.
- **Graph substrate:** SQLite at `<vault.root>/.kernl-graph.db` (nodes/edges/revisions/tags + FTS5). API, vault watcher, and `kernl capture` all share this one DB.
- **Frontend:** Vue 3 (Composition API) + **Nuxt**, in `web/`. Built to static (`nuxt generate` → `web/.output/public`) and **embedded into the Go binary** via `//go:embed` (see `web/embed.go`).
- **API:** REST JSON + SSE (not gRPC/WebSocket). REST emits **camelCase** JSON.
- **Config:** YAML (`kernl.yaml`; copy from `kernl.yaml.example`). `kernl doctor` validates.
- **LLM backing:** the DA brain currently points at a local openai-compat proxy; the orchestrator shells out to CLI agents (claude/codex/copilot/opencode/gemini/pi/agy). A `settings.agents.<id>.command` outside that list is rejected when the agent is resolved from its pool, not discovered as a spawned CLI refusing another dialect's flags.
- **The run report's Oracle has its own seam.** By default it goes through `llm.provider`/`llm.endpoint`, the same API the DA chat uses. Setting **`llm.agent: <settings.agents key>`** asks that agent's CLI instead, in one-shot answer mode - useful when the model you want lives behind a coding plan rather than behind the endpoint. Only `claude` and `pi` can be asked this way (measured: their one-shot stdout is the answer with no framing, and both can be run without tools); anything else is refused by name. There is no `kernl settings set llm --agent` flag: like `settings.pools`, it is edited in `kernl.yaml`.
- **A fork an implementer cannot resolve alone is handed to the DA, a separate top-level `da:` block.** `da.agent` (a `settings.agents` key) and `da.workDir` (the operator's own system repository) are both unset by default, which leaves the gate off and an implementer deciding every fork alone, same as before either key existed. The DA is not the Oracle above, and the contrast is the point: the Oracle is tool-less and context-less on purpose, because it only ever writes about a decision already taken, while the DA has tools and a working directory on purpose, because choosing on the operator's behalf is worthless without knowing what the operator has already decided or written down. One is never a precedent for the other.
- **A dispatched agent runs unattended unless `settings.agents.<id>.approvalMode` is `prompt`.** Under `prompt` every tool call becomes a judgment gate: the agent blocks, the request is parked as a pair of JSON files under `~/.kernl/state/approvals/`, and it resumes when a human answers through `kernl approval list`/`resolve`, the REST API, or the GUI. The store is a directory rather than a server because the process that raises a gate (the agent's bridge) is never the one that answers it, and `kernl bead run` stands up no HTTP listener at all. Only **claude** and **pi** can be gated - measured against their real CLIs; any other dialect configured with `prompt` is refused at dispatch rather than run unasked. An unanswered gate is denied with a reason after `orchestrator.approvalTimeout` (default 30m), and until then it holds both the agent process and one of `maxConcurrentBeads`' slots.
- **Run:** regenerate the frontend, *then* build, *then* serve (default :8080) - in that order, because the embed resolves at compile time. `docker compose up --build` does the whole sequence in one step.
- **`go run ./cmd/kernl serve` is a trap whenever `web/` changed.** `//go:embed` bakes `web/.output/public` in at **compile time**, so a bare `go run`/`go build` ships whatever stale build is on disk: **the UI you test is not the UI you wrote, and nothing warns you.** It is only safe when you have not touched `web/`, or after a fresh `cd web && npm run generate`. See §10.
- **Test (unit, hermetic):** `go test ./...`, run before every commit.
- **Test (integration contracts, manual):** `go test -tags=integration ./...`.
- **Test (bd fixture e2e, manual):** `go test -tags='integration bde2e' ./...`.
- **Test (real agent e2e, manual):** `go test -tags='integration realagent' ./...`.
- **Lint/format:** `gofmt`, `go vet ./...`, `golangci-lint run` (required gate, see §4).
- **Local CI (run before pushing):** `bin/ci`, mirrors `.github/workflows/ci.yml`.
- **Install git hooks (run once after clone):** `bin/install-hooks`.

### ⚠️ The embed gotcha (most common hurdle)

`web/embed.go` does `//go:embed all:.output/public`. That directory is the Nuxt build output and is **gitignored**. So on a fresh checkout (or after `git clean`) `go build`/`go test`/`go vet` **fail to compile** with *"no matching files found"*. Before any Go build you must have built the web at least once:

```bash
cd web && npm install && npm run generate   # produces web/.output/public
```

`bin/ci` does this for you. CI writes a placeholder for Go-only jobs and builds the web for real in the `test-web` job.

## 2. Architectural Principles

- **Fail Loud, Never Silent:** when a lookup for a configured resource (agent, pool, backend, workflow) fails, the code MUST: return an error that halts the operation; surface it; include the greppable marker `KERNL DISPATCH FAILURE`; name the missing thing AND the exact config that fixes it. NEVER return a fallback like `?? "default"` or `values(x)[0]`.
- **No Shared Mutable State:** `sync.Map` or `map + RWMutex` for registries. Goroutine-per-session; communicate over channels, never unguarded shared pointers.
- **YAGNI & Flat:** no preventive abstractions, no single-use interfaces, no mappers. Use interfaces for real boundaries (BackendPort, Transport), not indirection.
- **Clay, not Lego. Build for the user who exists:** kernl is open-source, but it is a **solo-dev tool**, and today it has exactly one user. Architecture *emerges from real use* and is extracted once a pattern proves itself; it is never designed up front for users who don't exist yet. Order: **make it work → make it right → make it fast**. KISS isn't "less code", it's "don't solve a problem you don't have yet". **Contest the framing (standing duty):** when a request starts framing kernl as multi-user, multi-tenant, teams, a plugin ecosystem, or "configurable N backends" before a real present need exists, STOP and ask whether that need exists *now*. The future-proofing has to be justified against today's need, not tomorrow's. This guards the project's *posture*, which the YAGNI rule above doesn't reach.
- **Comprehension Debt:** never make a silent architectural decision. Adding a dependency or pattern → say so, and record the rationale in the appropriate docs or memory.
- **Blast Radius:** if an edit spans multiple domains, isolate it on a branch, flag "BLAST RADIUS WARNING" at the top of the PR, and never merge it autonomously.

## 3. Code Style & Clean Code

- **Functions:** 4-40 lines, one thing each (SRP). **Files:** under 500 lines, split by responsibility.
- **Names:** specific and unique. Avoid `data`, `handler`, `Manager`, `util`.
- **Types:** explicit. No `any`, no untyped functions.
- **Control flow:** early returns over nested ifs; max 2 levels of indentation.
- **Dependencies:** inject them; wrap third-party libs behind a thin interface this project owns.
- **Errors:** return, don't panic. `fmt.Errorf("KERNL DISPATCH FAILURE: %s not found in pool %s", id, pool)`.
- **JSON contract:** REST is camelCase; the frontend reads via a `pick()` helper. Never bake Go field names into the wire format.
- **Material Symbols are a subset, not the full font.** `web/public/fonts/material-symbols-outlined.woff2` ships only the glyphs the app uses (~42 KB). Using a `material-symbols-outlined` icon that isn't in the subset renders a blank box: the icon will not appear. Whenever you introduce a NEW icon name, add it (alphabetically) to the `ICONS` list in `web/tools/material-symbols-subset.mjs`, run `node tools/material-symbols-subset.mjs` to regenerate the woff2, and update the glyph-list header comment in `web/assets/css/fonts.css`. Removing the last use of an icon is the reverse.

## 4. Tests & TDD

- **Reasoning is coding:** your "reasoning" is a failing test, not an essay.
  1. Write failing test → 2. minimal code to pass → 3. refactor.
- **Hermetic by default:** unit tests MUST NOT touch the host: no real `os.Getenv`/`os.Open`/`exec.Command`/network/ports. Mock at boundaries via interfaces.
- **Mocks:** named fakes / stub structs, not inline anonymous mocks.
- **Integration tests:** **manual only**, never in default CI. `-tags=integration` exercises local bd contract/smoke tests. Add `bde2e` for full bd fixture workflows and `realagent` for tests that shell out to real agent CLIs. Capability drift must fail loud or log `KERNL BD CAPABILITY DRIFT`.
- **golangci-lint** is a **required** CI gate: the tree runs clean and new findings fail the build. errcheck excludes only genuinely-unactionable calls (Encode/Fprintf to a flushed ResponseWriter, deferred `Close()`); see `.golangci.yml`. Don't reintroduce debt or blanket-`nolint` to get green.

## 5. Comments & Logging

- **Comments:** WHY, not WHAT. Keep existing comments: they carry intent. Docstrings on public funcs: intent + one usage example.
- **Logs:** structured `log/slog` for debug/observability; plain text only for user-facing CLI output.

## 6. Git, Secrets & Agent Behavior

- **Work in your own worktree. Mandatory.** Multiple sessions run in parallel and nothing tells you another is active. Before your first write, run **`bin/worktree new
  <type>/<short-name>`** and do everything in the dir it prints (branched off a fresh
  `origin/master`, under `$WORKTREE_BASE/kernl/<task>`). The main tree stays on `master` as a clean reference, never commit there. `feat/<short-name>` / `fix/<short-name>`, or `feat/<epicID>` for an epic. Read-only exploration needs no worktree. Tidy up merged worktrees with `bin/worktree status` / `rm`.
- **Task tracking: kernl itself, and nothing else.** This project's work lives in the graph, as tasks in the `kernl` project. Start any session with `kernl task list --project 019ecb9a-677b-792a-8b55-af14161ed9e2`, and keep it true in the same session: starting, finishing, blocking or deciding a unit of work all go in, or the next session repeats it. The markdown backlog in `local/` was retired on 2026-07-30 and is a pointer; do not add work there. It went because it and the graph were both live at once, and 8 of its 23 open entries existed in both, under different names in different languages. Two surfaces means an item saved to the wrong one is an item lost. The product's `bd`/orchestrator store is a separate runtime concern and is never a dev-task backend. **There is deliberately no published roadmap.** A statement of direction nobody maintains reads as a promise, and at pre-1.0 with one user there is no direction stable enough to publish. If one earns its place later it comes back; until then, what the project does is what the README and the code say it does.
- **Small releases:** atomic commits, `type: what changed`. Every commit on `master` passes `bin/ci` and is production-ready. Never `git add .` blind: separate unrelated changes. Closed work gets committed before the next task starts; say so when it hasn't been.
- **Before every commit:** show `git status` + `git diff --cached` and confirm no `.env`, token, key, or secret is staged. If one is, **STOP and say so**. The deterministic backstop is the `gitleaks` pre-commit hook (`bin/install-hooks`); this habit is the probabilistic one. If a secret ever lands, rotate the key, because a pushed secret is compromised regardless of history rewrite.
- **Anti-overwrite:** never overwrite a file without first reading/`git diff`-ing it.
- **Anti-regression:** run `bin/ci` (or at least `go vet ./... && go test ./...`) before declaring success.
- **Refactoring is not automatic:** after a large feature, proactively list refactoring candidates (files > ~500 lines, duplicated logic, long functions, hardcoded config) and ask before pruning. The maintainer decides; the tests are the safety net. But don't refactor *too early* either: tolerate some duplication while a pattern is still forming, and extract an abstraction *from* proven, repeated code, never inventing it ahead of the use that justifies it. Consolidate once it works and the seams are obvious.

## 7. Pair programming with an agent

- The human defines the **WHAT**; for consequential **HOW** decisions (architecture, semantics, new dependencies, or a changed public contract), the agent presents options and trade-offs and waits for a decision. Once the direction is settled, the agent owns the implementation details; do not wait for line-by-line dictation.
- **Plan first.** For any non-trivial task, present the full plan + to-do list and wait for approval BEFORE writing code. For destructive operations, gate behind an explicit flag.
- A non-trivial task should arrive with four things: **what is wanted / how / what is explicitly NOT wanted / how it gets validated.** When one is missing (especially the anti-goal), ask instead of assuming the default.
- If a task is impossible under the stated constraints, or information is missing, **say so, don't invent or guess.** Propose a better approach when you have one: this project wants to be contested, not obeyed blindly.

## 8. Security (habit, not a phase)

- When touching user input, network, filesystem, auth, or queries, flag the risk and propose the guard (SSRF, rate limiting, path traversal, injection, encryption).
- Dependency CVEs are caught automatically by `govulncheck` in CI. Secret leaks by `gitleaks`.
- Before any release/tag: run a manual security pass and list findings with severity + file; then a second pass with another model/harness, because neither finds everything.

## 9. Post-implementation checklist (run before saying "done")

1. New tests written and passing (`go test ./...`).
2. `bin/ci` green (gofmt, vet, unit tests, web tests, govulncheck, gitleaks).
3. `git diff --cached` reviewed: zero secrets.
4. Commits small and well-described.
5. Refactoring candidates listed (if the change was large).
6. Security risks flagged (if you touched a sensitive surface).
7. Docs updated when the change affects user-facing behavior, setup, release flow, or architecture.
8. Memory updated when the session produced a learning a future one should inherit (see §0).

## 10. Common hurdles (append as discovered)

- **Embed:** `npm run generate` before any `go build` (see §1). The #1 trip-up.
- **Web changes need a process restart:** `//go:embed` freezes `web/*` in memory; a running `kernl serve` won't see web edits until rebuilt and restarted.
- **bd CLI drift:** `bd close --reason` is supported in bd 1.0.4, but `bd update --reason` is not; terminal updates preserve reasons via `bd update --append-notes`.
- **`br list` has no `--parent` filter, and getting the direction wrong reports success having done nothing.** An epic's children come from `br dep list <epic> --direction up`; asking the other direction returns the epic's own parents, which is nothing, so the epic loads with zero children and the run reports success having done no work. Those children then have to be fetched with `br show <ids...>`, because `br list` reports `dependency_count` but never the dependencies, and the epic's DAG is built from exactly those. `br list --limit` defaults to **50**, not unlimited.
- **Read `--help` for the verb you are calling; br's flags are not uniform across verbs.** Measured against `br 0.2.10`, correcting two claims this file carried until 2026-07-31: `br update` **does** have `--set-labels` (repeatable, replacing), so a label set is replaced in one atomic call rather than remove-then-add; and `br create` **does** have `--parent`, which creates the parent-child dependency directly. What `br create` does **not** have is `--acceptance` or `--notes` - those exist only on `br update`, so a bead needing them takes a second immediate call. Extrapolating one verb's flags onto another is how both wrong claims got here.
- **br's envelopes differ per command.** `show`, `ready`, `dep list` and `comments list` return arrays; `list` returns `{"issues":[…]}`. Failures come back as `{"error":{code,message,hint}}` on **stdout** with a non-zero exit, so a caller reading only the exit code loses the reason and one reading stdout as data decodes an error as an empty result. `br schema commands --format json` is the authority.
- **Dolt embedded:** transactions are ACID; no manual file locking. Validate lock contention if the orchestrator and a manual `bd` run concurrently.
- **SSE in Go:** `Content-Type: text/event-stream`, `fmt.Fprintf(w, "data: %s\n\n", js)`, flush.
- **Vue 3 reactivity:** mutate reactive refs inside Vue's lifecycle, or updates are missed.
- **`npm install` vs `npm ci` (breaks the frontend build):** after adding/bumping a web dependency, always run `cd web && npm install` to fully refresh `package-lock.json`, then verify with `npm ci`. `npm install` is lenient and reconciles a partially-stale lock against the existing `node_modules`, so a broken lock (missing/mismatched transitive deps, e.g. `@emnapi/*`) can pass locally yet make a clean frontend build fail, because `bin/ci` and the Docker build both run the strict `npm ci`. Commit the lock only after `npm ci` succeeds from a clean state.
  - **The npm binary's own version is part of this** (2026-07-18): a plain `npm install` under npm 11.6.2 wrote a lock its *own* `npm ci` then rejected (`@emnapi/*` missing + version-mismatched). `npx -y npm@latest install` produced a lock both accept, with an 11-line diff and no churn. If `npm install` → `npm ci` still disagrees, regenerate with `npm@latest` before suspecting anything else.
- **Kill your dev server before running `bin/ci`** (2026-07-18): `nuxt dev` holds a lock, so `nuxt generate` aborts with `Another Nuxt dev is already running (PID …)`. That failure then cascades: every Go step reports `web/embed.go: pattern all:.output/public: no matching files found`, because the embed prerequisite never produced its output. The output accuses the Go build and the embed directive; the actual cause is a background process you started. `pgrep -af nuxt` before believing any of it.
- **A registered repository can declare a tracker it has no store for, and only the sweep log says so.** `registry.repos[].memoryManager` overrules detection by design, so nothing compares the declaration against `.beads/`. A repository that was never initialized builds its backend happily and fails on the first call: `ERROR sweep tick failed … no beads project found`, once per tick with the server already up, and every `/api/beads` request too when the entry is `repos[0]` (which is the implicit default for the beads API and dispatch). `kernl doctor`'s `tracker-store` check names it up front; it is advisory, because one uninitialized entry must not stop the other repositories from being served.
- **A gitignored tree can be invisible to search.** `local/`, `.beads/` and `web/.output/` are git-ignored, and some search tools skip ignored paths by default, returning zero hits and exit 0, which reads exactly like "it was never written down". Name the directory explicitly before concluding something is undocumented.
- **One `kernl serve` owns a graph database; a second one exits, and running a second instance means giving it its own copy.** `serve` takes an exclusive `flock` on `<resolved graph db>.lock` before it opens the database, so two servers pointed at one `vault.root` (or two with no vault configured, both falling back to `~/.kernl`) can no longer both run. The second exits non-zero, never binds its port, and names the pid its lock file recorded. What it guards is the *database*, not the port: two servers on different databases still coexist, every CLI verb keeps working against a database a server owns (SQLite serializes the writers), and `kernl epic run` still stands up its own API on a fallback port. The invariant is that one process runs the automatic background loops (classifier, vault watcher, sweep tick), not that one process serves the API.
  **To run a second instance anyway** (testing another build against real data), stop the owner, copy, then start both:
  ```bash
  cp -a ~/vault /tmp/vault-copy          # the whole root: notes AND .kernl-graph.db
  cp kernl.yaml /tmp/kernl-copy.yaml     # then edit vault.root: /tmp/vault-copy
  kernl --config /tmp/kernl-copy.yaml --port 8081 serve
  ```
  Two things make this fussier than it looks. **Copy the whole vault root, not just the `.db`:** the reconciler's cold start diffs the tree against the database, so a copied database next to an empty tree tombstones every note in the copy before you can test anything. And **do not `cp` the database out from under a running server:** the graph opens SQLite in WAL mode, so a live `.kernl-graph.db` plus its `-wal`/`-shm` sidecars are not a consistent snapshot when copied one file at a time. If stopping the owner is not an option, take the database with an online backup instead: `cp -a ~/vault/. /tmp/vault-copy/ && rm -f /tmp/vault-copy/.kernl-graph.db*`, then `sqlite3 ~/vault/.kernl-graph.db ".backup /tmp/vault-copy/.kernl-graph.db"`. Either way the destination must be empty or absent beforehand; copying onto a previous copy leaves a database and a note tree from two different moments.

## 11. Release / deploy

Standardized binary distribution:
- **Release:** push a `v*` tag → `.github/workflows/release.yml` runs **goreleaser** (`.goreleaser.yaml`). It cross-compiles `./cmd/kernl` for linux+darwin × amd64+arm64 (`CGO_ENABLED=0`; the `before` hook runs `nuxt generate` so the web UI is embedded), publishes tar.gz archives + `checksums.txt` to a GitHub Release. **Windows is not a target** (syscall.Kill in watchdog.go is Unix-only). Windows users use Docker.
- **Install (end users):** `install.sh` (curl|bash) downloads the right archive into `~/.local/bin/kernl`. AUR / Homebrew / mise are deferred.
- **Docker (optional self-host of `serve`):** `Dockerfile` + `compose.yaml`: `docker compose up --build`, UI on :8080. Orchestration needs the host toolchain.
- **Local dev:** `go build -o kernl ./cmd/kernl` after `cd web && npm run generate`, or `docker compose up --build` for the whole sequence.
