Este é o máximo que colocamos no AGENTS.md (estilo + arquitetura high-level). Interfaces detalhadas vão em specs/.
# Foolery (Go Port) — Master Agent Briefing
> **Context:** Foolery is a keyboard-first orchestration app for agent-driven software work. This is the Go backend + Vue 3 frontend port from the TypeScript/Next.js implementation.
> **Core Value:** Execute work, break it down, dispatch agents, review outcomes — keep it legible and fast across repos.
## 1. Stack & Commands
- **Backend:** Go 1.24+
- **Frontend:** Vue 3 (Composition API) + Vite
- **UI (Future):** TUI via Bubble Tea (deferred)
- **API:** REST JSON + SSE (not gRPC/WebSocket)
- **Config:** YAML (`foolery.yaml` — settings + registry in one file)
- **Storage:** `bd` CLI (gastownhall/beads) with Dolt. NEVER write JSONL directly.
- **Run (backend):** `go run ./cmd/foolery` or `air` for dev
- **Run (frontend):** `cd web && npm run dev` (Vite)
- **Test:** `go test ./...` — Run before every commit. Hermetic by default.
- **Test (integration):** `go test -tags=integration ./...` — opt-in, manual only.
- **Lint/Format:** `golangci-lint run` + `go vet ./...` + `go fmt ./...`
## 2. Architectural Principles
- **CLI-First Backend:** The Go backend delegates ALL storage mutations to `bd` or `kno` CLI. Never writes `.beads/issues.jsonl` directly.
- **Goroutine-per-Session:** Each terminal session lives in its own goroutine. No shared EventEmitter; use channels.
- **No Shared Mutable State:** `sync.Map` or `map + RWMutex` for registries. Never hold unguarded pointers across goroutines.
- **YAGNI & Flat:** Do NOT generate preventive abstractions. No single-use interfaces, no mappers. Keep file structure flat.
- **Fail Loud, Never Silent:** When a lookup for a configured resource (agent, pool, backend, workflow) fails, the code MUST:
  1. Return an error that halts the current operation.
  2. Log a red ANSI banner via `log.Fatalf` or structured error.
  3. Surface the failure to any visible session buffer as a stderr banner event.
  4. Include the greppable marker `FOOLERY DISPATCH FAILURE` (or subsystem marker).
  5. Name the missing thing (beat id, state, pool key, workflow id, action name) and the exact config that fixes it.
  6. NEVER return a fallback like `Object.values(x)[0]` or `?? "default"`.
- **Comprehension Debt (ADR):** NEVER make silent architectural decisions. If adding a dependency/pattern, update `docs/architecture.md`.
- **Blast Radius (Async Review):** If an edit affects multiple domains, isolate work in a branch. Create a `bd` issue flagged for `human` review. State "BLAST RADIUS WARNING" at the top of any PR. NEVER merge autonomously.
## 3. Code Style & Clean Code
- **Functions:** 4-40 lines. Split if longer. One thing per function (SRP).
- **Files:** Under 500 lines. Split by responsibility.
- **Names:** Specific and unique. Avoid `data`, `handler`, `Manager`.
- **Types:** Explicit. No `any`, no untyped functions. Use Go interfaces for boundaries (BackendPort, Transport), but not for single-use indirection.
- **Control Flow:** Early returns over nested ifs. Max 2 levels of indentation.
- **Dependencies:** Inject dependencies. Wrap third-party libs behind thin interfaces owned by this project.
- **Errors:** Return errors, don't panic. Use `fmt.Errorf("FOOLERY DISPATCH FAILURE: %s not found in pool %s", agentID, poolKey)` for loud failures.
## 4. Tests & TDD (Action via TDD)
- **Hermetic by Default:** Tests in `**/__tests__/` (or `*_test.go` standard) MUST NOT touch the host environment. No `os.Getenv`, no real `os.Open`, no real `exec.Command`, no real network or ports. Mock at boundaries via interfaces.
- **TDD Discipline:**
  1. Write a failing test that documents the expected behavior.
  2. Write minimal production code to pass.
  3. Refactor.
- **Mocks:** Use named fakes / stub structs implementing interfaces. No inline anonymous mocks in tests.
- **Integration Tests (Tagged):** Use `//go:build integration`. These may touch `bd` CLI and real processes, but run ONLY manually via `go test -tags=integration ./...`. Never in the default `go test ./...`.
- **Race Detector:** Integration tests SHOULD use `go test -race ./...` in CI when running tagged suites.
## 5. Comments & Logging
- **Comments:** WHY, not WHAT. Docstrings on public functions: intent + 1 usage example.
- **Logs:** Structured JSON for debugging (`log/slog`). Plain text ONLY for user-facing output.
## 6. Living Documentation & Memory Bank
Update documentation on every relevant change. Never invent terminology.
- **Memory Bank:** Maintain `docs/activeContext.md` and `docs/progress.md` for session state.
- **`docs/glossary.md`:** Read/update for domain terms to maintain Ubiquitous Language.
- **`docs/architecture.md` & `docs/features.md`:** Update for structural/capability changes.
- **Specs (`specs/*.md`):** Authoritative for behavior. Ralph loop uses these as PRDs. Include citations to the TS source when describing behavior.
## 7. Git & Agent Behavior
- **Branch per task:** `feat/[short-name]` or `fix/[short-name]`. Never work on `main`.
- **Atomic Commits:** One commit per feature/fix. Message: `type: what changed`.
- **Push:** Run `git push` after every commit. Unpushed work is lost work.
- **Anti-Overwrite:** NEVER overwrite a file without first verifying its current state (via `git diff` or reading). Beware of silent truncation on large files.
- **No Thinking Loops:** If you make >=2 tool calls without executing a change, STOP deliberating. Implement your best-effort solution immediately and iterate based on the result.
- **Anti-Regression:** Run `go vet ./...` and `go test ./...` before declaring success.
## 8. Known Hurdles & Evolution
- **Stack Quirks:**
  - `bd` CLI output is NDJSON (newline-delimited JSON). Parse with `bufio.Scanner` + `json.Decoder`.
  - Dolt transactions via `bd` are ACID. No manual file locking needed.
  - SSE in Go: use `net/http` with `w.Header().Set("Content-Type", "text/event-stream")` and `fmt.Fprintf(w, "data: %s\n\n", jsonStr)`.
  - Vue 3 reactivity uses proxies. Mutating state outside Vue's lifecycle can miss updates. Always mutate reactive refs.
- **Resolved Bugs:** [Add tricky bugs solved so they aren't reintroduced]
## 9. Issue Tracking (bd / beads)
- **MANDATORY:** Use the `bd` CLI for ALL task tracking. Run `bd prime` if unsure of commands.
- **FORBIDDEN:** Do NOT create `todo.md` or use external trackers.
- **Rules:**
  - Append `--json` to `bd` commands for parsing.
  - Run `bd ready --json` before asking for work.
  - Claim tasks atomically: `bd update <id> --claim --json`.
  - Link discovered work: `bd create "Found bug" -p 1 --deps discovered-from:<parent-id> --json`.
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
