# Kernl Orchestration Core (MVP) Implementation Plan

> **Bead Target:** This plan maps deterministically to beads. Every task includes a `**Bead Mapping:**` block. The converter will project tasks 1:1 with zero creative interpretation.

> **⚠️ Before Phase 0 — one parameter to fill:** the Go module path is `github.com/gabrielassisxyz/kernl`. Replace `<user>` with the real GitHub handle everywhere in this plan before conversion/execution. This is the only unresolved placeholder; flagged for Yegge iteration 1.

**Goal:** Take the `foolery-go` engine library the last mile — assemble it into a runnable `kernl` binary that executes a real epic (a bead graph with dependencies) with true parallelism, isolated per-bead git worktrees, durable run-state, and a minimal monitoring GUI.

**Architecture:** `foolery-go` is imported as the `orchestrator/` block of the `kernl` repo (packages under `orchestrator/internal/...`). The engine's tested pieces — `bd` adapter, take-loop decision functions, dispatch + cross-agent exclusion, `SessionConnectionManager` — are reused untouched. The greenfield slice is: a **SessionDriver** (spawns the agent `exec.Cmd`, wires its pipes to `session.SessionRuntime`, drives the take-loop functions — this glue does not exist today; `exec.Command` appears only in `bdcli.go`/`knots.go`), an **EpicExecutor** (DAG → ready-set → wave dispatch with a concurrency semaphore, fail-fast), a **WorktreeManager** (the only git-aware package), a **SQLite run-state store**, real **API + epic-SSE** handlers, and a `kernl` **CLI** (`serve`, `doctor`, `epic run`, `epic list`, `bead run`). The orchestration core stays git-agnostic as a package invariant; only `internal/worktree` and `internal/epic` know git.

**Tech Stack:** Go 1.26, `gopkg.in/yaml.v3`, `modernc.org/sqlite` (cgo-free), `net/http` (REST + SSE), `bd` CLI (storage), `opencode` CLI (agent harness), `git worktree`. Hermetic unit tests in `*_test.go` (`go test ./...`); integration tests behind `//go:build integration` (`go test -tags=integration ./...`, opt-in/manual). Plain-text operator output by default; structured slog behind `--log-format=json` / `KERNL_LOG_LEVEL=debug`.

**Priority rule:** `Priority` is a coarse bucket. Within the same bucket, execution order is determined solely by the `Dependencies` DAG via topological sort. A task may depend on another task with the same or lower priority, but never on a task with a *higher* priority number.

**Conventions (from `AGENTS.md` + eng/devex reviews — apply to every new package):** files < 500 lines, funcs 4–40 lines, fail-loud with greppable marker `KERNL DISPATCH FAILURE`, named fakes at boundaries, no preventive abstractions. Every fail-loud error in the new packages (`worktree`, `epic`, `app`, `runstate`, `api`, `preflight`) carries **problem + cause + actionable fix + (when applicable) next command** — not just the marker.

**Cross-cutting deliverables (no single owner — every phase updates its slice, like inline unit tests):**
- `README.md` — each phase that adds an operable surface documents it.
- `docs/glossary.md` — each phase that introduces a term (bead/epic/wave, take loop, dispatch, cross-agent review, run-state, knots-dormant) records it.
- Hermetic unit tests — TDD inline in every task that writes code.

---

## File Structure

**Repo layout after Phase 0:**

```
kernl/                                  # the repo (git init here)
  docs/  skills/  ideas.md  TODOS.md     # existing planning layer — moves in, untouched
  orchestrator/                          # the foolery-go block
    cmd/kernl/                           # was cmd/foolery — the binary + subcommands
      main.go  serve.go  doctor.go  epic.go  bead.go
    internal/
      api/        # stubs → real handlers; epic-SSE endpoint
      app/        # NEW — engine assembly + SessionDriver
      backend/    # REUSE untouched (bd adapter, state machine)
      config/     # MODIFY — add Orchestrator block (worktree root, max-concurrent-beads)
      dispatch/   # REUSE untouched (agent rotation, cross-agent exclusion)
      epic/       # NEW — DAG, EpicExecutor, bead-source + translation, epic events
      orchestration/ session/ terminal/ adapter/ transport/ prompt/  # REUSE untouched
      preflight/  # NEW — kernl doctor checks
      runstate/   # NEW — SQLite run-state store
    web/          # NEW — minimal HTML/JS/CSS monitoring GUI
    kernl.yaml.example   # was foolery.yaml.example — rich + commented
    AGENTS.md  specs/    # REUSE (marker renamed)
  examples/parallel-demo/.beads/         # NEW — packaged epic for the magical moment
  LICENSE  README.md                     # NEW
```

**New files and their single responsibility:**

| File | Responsibility |
|---|---|
| `orchestrator/internal/worktree/worktree.go` | `WorktreeManager`: `git worktree add/remove/prune` under a configurable root. The only git-aware package. |
| `orchestrator/internal/epic/dag.go` | Build a DAG from a bead set; topological ready-set computation; cycle detection. Pure, no I/O. |
| `orchestrator/internal/epic/beadsource.go` | Read an epic (parent + children + deps) from `.beads/` via the `bd` adapter; translate `vc-convert-plan-to-beads` output shape if it diverges from what the adapter expects. |
| `orchestrator/internal/epic/events.go` | Epic event types: `bead-state-changed`, `session-started`, `session-error`, `wave-advanced`; parallelism counters. |
| `orchestrator/internal/epic/executor.go` | `EpicExecutor`: wave dispatch loop, `max-concurrent-beads` semaphore, fail-fast on terminal child failure, parallelism metric capture. |
| `orchestrator/internal/app/app.go` | `App`: assemble engine from `config.Config` — backend, `TerminalManager`, `SessionConnectionManager`, `WorktreeManager`, `EpicExecutor`, run-state store. |
| `orchestrator/internal/app/driver.go` | `SessionDriver`: spawn the agent `exec.Cmd` in a worktree, wire stdout/stderr to `session.SessionRuntime`, drive the take-loop decision functions per beat; the nudge-prompt builder. |
| `orchestrator/internal/runstate/store.go` | SQLite store: `bead→worktree` (1:1) + per-`(bead,state)` agent/session records. `bd` remains source of truth for bead state. |
| `orchestrator/internal/preflight/preflight.go` | `kernl doctor` checks: `bd` in PATH, `opencode` configured, Go version, `kernl.yaml` valid. |
| `orchestrator/internal/api/epics.go` | Real epic handlers + `GET /api/epics/{id}/events` SSE endpoint fed by `EpicExecutor`. |
| `orchestrator/cmd/kernl/{main,serve,doctor,epic,bead}.go` | The `kernl` binary and subcommands. |
| `web/{index.html,app.js,style.css}` | Minimal monitoring GUI consuming epic-SSE. |

---

## Phase 0 — Rename & Repo Consolidation

### Task 1: Initialize the kernl repo and import the orchestrator block

**Bead Mapping:**
- type: `epic`
- Priority: `0`
- Estimated Minutes: `0`
- Dependencies: `none`
- Parent: `none`
- Status: `open`

> Epic container for Phase 0. Children: Tasks 2–7.

---

### Task 2: Create the kernl git repo and subtree-import foolery-go

**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: `14`
- Dependencies: `none`
- Parent: `Task 1`
- Status: `open`

**Files:**
- Create: `/home/gabriel/repositories/kernl/.git` (repo init)
- Create: `/home/gabriel/repositories/kernl/.gitignore`
- Modify: `/home/gabriel/repositories/kernl/` (foolery-go contents land under `orchestrator/`)

**Description / Steps:**
- [ ] **Step 1: Capture the foolery-go test baseline**

Run in `~/repositories/_cloned/foolery-go`:
```bash
go test ./... 2>&1 | tail -3
```
Expected: all packages pass. Record the exact summary line — this is the green baseline the Phase 0 gate (Task 7) must match.

- [ ] **Step 2: Initialize the kernl repo and commit the existing planning layer**

```bash
cd /home/gabriel/repositories/kernl
git init
printf 'orchestrator/web/node_modules/\n*.jsonl\n__pycache__/\n.beads/*.db\n' > .gitignore
git add -A && git commit -m "chore: initial commit — planning layer (docs, skills, ideas)"
```

- [ ] **Step 3: Subtree-import foolery-go under `orchestrator/` (preserves its history)**

```bash
git subtree add --prefix=orchestrator /home/gabriel/repositories/_cloned/foolery-go main
```
Expected: foolery-go tree appears at `orchestrator/`, its commit history grafted in.

- [ ] **Step 4: Verify the imported tree still builds and tests green**

```bash
cd /home/gabriel/repositories/kernl/orchestrator && go test ./... 2>&1 | tail -3
```
Expected: identical summary to Step 1.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "chore: import foolery-go engine as orchestrator/ block"
```

**Acceptance Criteria:**
- [ ] `git -C /home/gabriel/repositories/kernl log --oneline | grep -c .` returns ≥ 3 commits, including foolery-go history.
- [ ] `cd orchestrator && go test ./...` exits 0 with the same pass count as the Step 1 baseline.
- [ ] `orchestrator/go.mod` exists; `docs/`, `skills/`, `ideas.md` are unmoved at repo root.

---

### Task 3: Mechanical rename — `foolery` → `kernl` (module path, identifiers, markers)

**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: `16`
- Dependencies: `Task 2`
- Parent: `Task 1`
- Status: `open`

**Files:**
- Modify: `orchestrator/go.mod` (module path)
- Modify: all `orchestrator/**/*.go` (109 files reference `foolery` — import paths, the `FOOLERY DISPATCH FAILURE` marker, env var prefixes)

**Description / Steps:**
- [ ] **Step 1: Rewrite the module path**

Edit `orchestrator/go.mod` line 1:
```
module github.com/gabrielassisxyz/kernl
```

- [ ] **Step 2: Rewrite every import path**

```bash
cd /home/gabriel/repositories/kernl/orchestrator
grep -rl 'github.com/gastownhall/foolery' --include='*.go' . \
  | xargs sed -i 's#github.com/gastownhall/foolery/internal#github.com/gabrielassisxyz/kernl/orchestrator/internal#g'
```

- [ ] **Step 3: Rename the fail-loud marker and env var prefix**

```bash
grep -rl 'FOOLERY DISPATCH FAILURE' --include='*.go' . | xargs sed -i 's/FOOLERY DISPATCH FAILURE/KERNL DISPATCH FAILURE/g'
grep -rl 'FOOLERY WORKFLOW CORRECTION FAILURE' --include='*.go' . | xargs sed -i 's/FOOLERY WORKFLOW CORRECTION FAILURE/KERNL WORKFLOW CORRECTION FAILURE/g'
grep -rl 'FOOLERY_' --include='*.go' . | xargs sed -i 's/FOOLERY_/KERNL_/g'
grep -rl '"foolery-bd-locks"' --include='*.go' . | xargs sed -i 's/foolery-bd-locks/kernl-bd-locks/g'
```
Note: `internal/adapter/adapter.go` const `TerminalDispatchFailureMarker = "FOOLERY DISPATCH FAILURE"` is covered by Step 3's first line.

- [ ] **Step 4: Rename the binary directory**

```bash
git mv cmd/foolery cmd/kernl
```
(The `package main` stays; `cmd/kernl/main.go` is rewritten in Phase 2 — for now just fix its imports and the `foolery starting` / `foolery stopped` log strings to `kernl`.)

- [ ] **Step 5: Build and test**

```bash
go build ./... && go test ./... 2>&1 | tail -3
```
Expected: builds clean; same pass count as Task 2 Step 1 baseline. If anything fails, fixing it is part of this task — do not proceed red.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "refactor: rename foolery -> kernl (module path, markers, env vars)"
```

**Acceptance Criteria:**
- [ ] `grep -rn 'gastownhall/foolery' orchestrator/ --include='*.go'` returns nothing.
- [ ] `grep -rn 'FOOLERY' orchestrator/ --include='*.go'` returns nothing.
- [ ] `cd orchestrator && go build ./... && go test ./...` both exit 0, pass count unchanged from baseline.

---

### Task 4: Mechanical rename — `beat` → `bead` (vocabulary alignment)

**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: `18`
- Dependencies: `Task 3`
- Parent: `Task 1`
- Status: `open`

**Files:**
- Modify: all `orchestrator/**/*.go` using `Beat`/`beat` identifiers (`backend/port.go` `Beat`, `CreateBeatInput`, `BeatDependency`, `BeatListFilters`; `api/beats.go`; `terminal` `BeatID`; `session` `BeatID`; etc.)
- Rename: `orchestrator/internal/api/beats.go` → `orchestrator/internal/api/beads.go`

**Description / Steps:**
- [ ] **Step 1: Rename the exported and local identifiers**

The `bd` adapter calls (`bd show <id>`, `bd list`) are unchanged — `bd` is the CLI name, not the domain word. Only Go-side `Beat`/`beat` identifiers change to `Bead`/`bead`:
```bash
cd /home/gabriel/repositories/kernl/orchestrator
grep -rl --include='*.go' -e 'Beat' -e 'beat' . | xargs sed -i \
  -e 's/Beat/Bead/g' -e 's/beatId/beadId/g' -e 's/beatID/beadID/g' \
  -e 's/\bbeat\b/bead/g' -e 's/beats/beads/g'
```
Caution: review the diff for false hits in comments/strings that intentionally said "beat" (the musical apelido in `AGENTS.md` prose is fine to leave; code identifiers are not).

- [ ] **Step 2: Rename the API file**

```bash
git mv internal/api/beats.go internal/api/beads.go
```

- [ ] **Step 3: Fix JSON tags deliberately**

The `bd` CLI JSON uses its own field names. Inspect `backend/port.go` `Bead` struct tags — keep whatever `bd` actually emits (verify against `bd show --json` output in Task 8's fixture); rename only the Go field names, not necessarily the `json:"..."` tags. Adjust any tag that was Go-side invented (not bd-emitted) to the bead vocabulary.

- [ ] **Step 4: Build and test**

```bash
go build ./... && go test ./... 2>&1 | tail -3
```
Expected: builds clean; same pass count as baseline.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "refactor: rename beat -> bead (vocabulary alignment with bd/.beads domain)"
```

**Acceptance Criteria:**
- [ ] `grep -rn '\bBeat\b\|\bbeat\b' orchestrator/ --include='*.go'` returns nothing (excluding `AGENTS.md` prose, which is not `*.go`).
- [ ] `orchestrator/internal/api/beads.go` exists; `beats.go` does not.
- [ ] `cd orchestrator && go build ./... && go test ./...` both exit 0, pass count unchanged from baseline.

---

### Task 5: Rename config + doc artifacts (`foolery.yaml` → `kernl.yaml`, AGENTS.md marker)

**Bead Mapping:**
- type: `chore`
- Priority: `1`
- Estimated Minutes: `8`
- Dependencies: `Task 3`
- Parent: `Task 1`
- Status: `open`

**Files:**
- Rename: `orchestrator/foolery.yaml.example` → `orchestrator/kernl.yaml.example`
- Modify: `orchestrator/internal/config/foolery.go` → rename file to `config.go`, `config.Load` default path arg
- Modify: `orchestrator/AGENTS.md` (marker `FOOLERY DISPATCH FAILURE` → `KERNL DISPATCH FAILURE`; `foolery.yaml` → `kernl.yaml`; `go run ./cmd/foolery` → `go run ./cmd/kernl`)

**Description / Steps:**
- [ ] **Step 1: Rename the config file and example**

```bash
cd /home/gabriel/repositories/kernl/orchestrator
git mv foolery.yaml.example kernl.yaml.example
git mv internal/config/foolery.go internal/config/config.go
```

- [ ] **Step 2: Update the AGENTS.md references**

In `orchestrator/AGENTS.md` replace: `FOOLERY DISPATCH FAILURE` → `KERNL DISPATCH FAILURE`, `foolery.yaml` → `kernl.yaml`, `./cmd/foolery` → `./cmd/kernl`. Add one line under §1: "`foolery.yaml` → `kernl.yaml` (renamed in the kernl rename commit)."

- [ ] **Step 3: Build and test**

```bash
go build ./... && go test ./... 2>&1 | tail -3
```

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "chore: rename foolery.yaml -> kernl.yaml and AGENTS.md references"
```

**Acceptance Criteria:**
- [ ] `orchestrator/kernl.yaml.example` exists; no file named `foolery*` remains (`find orchestrator -name 'foolery*'` is empty).
- [ ] `grep -c FOOLERY orchestrator/AGENTS.md` returns 0.
- [ ] `cd orchestrator && go build ./...` exits 0.

---

### Task 6: Add `LICENSE` (MIT, preserving the acartine copyright)

**Bead Mapping:**
- type: `chore`
- Priority: `1`
- Estimated Minutes: `6`
- Dependencies: `Task 2`
- Parent: `Task 1`
- Status: `open`

**Files:**
- Create: `/home/gabriel/repositories/kernl/LICENSE`

**Description / Steps:**
- [ ] **Step 1: Write the MIT license preserving provenance**

`foolery-go` derives from `acartine/foolery` (MIT). The MIT license requires the original copyright notice be preserved. Write `LICENSE`:
```
MIT License

Copyright (c) 2024 acartine (original "foolery" — https://github.com/acartine/foolery)
Copyright (c) 2026 gabrielassisxyz (Kernl)

Permission is hereby granted, free of charge, to any person obtaining a copy
... [standard MIT body] ...
```

- [ ] **Step 2: Commit**

```bash
git add LICENSE && git commit -m "chore: add MIT LICENSE preserving acartine/foolery copyright"
```

**Acceptance Criteria:**
- [ ] `/home/gabriel/repositories/kernl/LICENSE` exists, contains the string `acartine` and the full MIT permission body.

---

### Task 7: Phase 0 gate — full suite green, end-to-end

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: `6`
- Dependencies: `Task 3`, `Task 4`, `Task 5`
- Parent: `Task 1`
- Status: `open`

**Files:**
- Test: `orchestrator/...` (whole module)

**Description / Steps:**
- [ ] **Step 1: Run the full hermetic suite**

```bash
cd /home/gabriel/repositories/kernl/orchestrator && go test ./... 2>&1 | tee /tmp/kernl-phase0-gate.txt | tail -5
```

- [ ] **Step 2: Compare pass count to the Task 2 Step 1 baseline**

The number of passing tests MUST equal the foolery-go baseline. A drop = the rename broke something = this task is not done until it is back to green at the baseline count.

- [ ] **Step 3: Vet and build**

```bash
go vet ./... && go build ./...
```

**Acceptance Criteria:**
- [ ] `go test ./...` exits 0; pass count == foolery-go baseline (Task 2 Step 1).
- [ ] `go vet ./...` and `go build ./...` both exit 0.
- [ ] No occurrence of `foolery` or `FOOLERY` in any `orchestrator/**/*.go` or `orchestrator/go.mod`.

---

## Phase 1 — De-risking

### Task 8: De-risking — integration harness + `opencode -s` verification

**Bead Mapping:**
- type: `epic`
- Priority: `1`
- Estimated Minutes: `0`
- Dependencies: `Task 1`
- Parent: `none`
- Status: `open`

> Epic container for Phase 1. Children: Tasks 9–10. The "Passo A" milestone from the eng review is realized by **Task 22** (its passing integration test depends on the SessionDriver + `kernl bead run`, both built in Phase 2).

---

### Task 9: Spike — verify `opencode run -s <session_id>` resume semantics

**Bead Mapping:**
- type: `spike`
- Priority: `1`
- Estimated Minutes: `20`
- Dependencies: `Task 7`
- Parent: `Task 8`
- Status: `open`

**Files:**
- Create: `docs/spikes/2026-05-14-opencode-resume.md`

**Description / Steps:**
- [ ] **Step 1: Run a fresh opencode session non-interactively and capture its session id**

```bash
opencode run --help 2>&1 | grep -A1 -- '-s\|--session'
opencode run "print hello" 2>&1 | tee /tmp/oc-run-1.txt
```
Record: does `opencode run` print/expose a session id? Where?

- [ ] **Step 2: Attempt to resume that session non-interactively**

```bash
opencode run -s <captured-session-id> "continue: now print goodbye" 2>&1 | tee /tmp/oc-run-2.txt
```
Record: does `-s` resume the same conversation? Does it error on an unknown id? Is the flag spelled `-s` or `--session`?

- [ ] **Step 3: Write the findings doc**

`docs/spikes/2026-05-14-opencode-resume.md` records: exact flag name, whether non-interactive resume works, what an invalid/expired session id does, and the verdict — **resume design confirmed** or **resume design needs revision** (and how). This doc is the input that unblocks Task 26 (resume logic).

**Acceptance Criteria:**
- [ ] `docs/spikes/2026-05-14-opencode-resume.md` exists and states a clear verdict on `opencode run -s` with the exact flag spelling.
- [ ] The doc records observed behaviour for: valid id resume, invalid id, and the id-capture mechanism.

---

### Task 10: Integration test harness scaffolding

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: `24`
- Dependencies: `Task 7`
- Parent: `Task 8`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/integration/harness.go` (build tag `//go:build integration`)
- Create: `orchestrator/internal/integration/harness_test.go` (build tag `//go:build integration`)
- Create: `orchestrator/testdata/kernl.yaml` (integration config fixture)
- Create: `orchestrator/testdata/beads-single/.beads/issues.jsonl` (one-bead fixture for Passo A)

**Description / Steps:**
- [ ] **Step 1: Write the failing harness smoke test**

`orchestrator/internal/integration/harness_test.go`:
```go
//go:build integration

package integration

import "testing"

func TestHarnessBootsConfigAndTempRepo(t *testing.T) {
	h := NewHarness(t)
	defer h.Cleanup()

	if h.Config.Settings.Agents["opencode"].Command == "" {
		t.Fatal("expected opencode agent in integration fixture config")
	}
	if h.RepoPath == "" {
		t.Fatal("expected harness to create a temp git repo with .beads/")
	}
}
```

- [ ] **Step 2: Run it — verify it fails to compile**

```bash
cd orchestrator && go test -tags=integration ./internal/integration/ -run TestHarnessBoots -v
```
Expected: FAIL — `NewHarness` undefined.

- [ ] **Step 3: Implement the harness**

`orchestrator/internal/integration/harness.go` — loads `testdata/kernl.yaml`, copies a bead fixture into a `t.TempDir()` git repo (`git init` + `bd` import), exposes `Config`, `RepoPath`, and `Cleanup()`. Skip the whole test (`t.Skip`) with an actionable message if `bd` or `opencode` is not in `PATH` — integration tests must never hard-fail a missing external tool, they skip loudly.

- [ ] **Step 4: Write the fixtures**

`testdata/kernl.yaml`: one `opencode` agent, one pool referencing it, one repo entry, orchestrator block with `worktreeRoot` and `maxConcurrentBeads: 2`. `testdata/beads-single/.beads/issues.jsonl`: one `task` bead in `ready_for_implementation`.

- [ ] **Step 5: Run the test to verify it passes**

```bash
go test -tags=integration ./internal/integration/ -run TestHarnessBoots -v
```
Expected: PASS (or SKIP with the actionable message if tools absent).

- [ ] **Step 6: Confirm the default suite is unaffected**

```bash
go test ./... 2>&1 | tail -2
```
Expected: integration files excluded (no `integration` tag), pass count == Phase 0 gate.

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "test: integration harness scaffolding + fixtures (//go:build integration)"
```

**Acceptance Criteria:**
- [ ] `go test -tags=integration ./internal/integration/ -run TestHarnessBoots` PASSES or SKIPS with a message naming the missing tool.
- [ ] `go test ./...` (no tag) pass count is unchanged from the Phase 0 gate.
- [ ] `testdata/kernl.yaml` and `testdata/beads-single/.beads/issues.jsonl` exist and are valid.

---

## Phase 2 — Product Assembly (engine + driver + API + CLI)

### Task 11: Assemble the product — engine, SessionDriver, API wiring, CLI

**Bead Mapping:**
- type: `epic`
- Priority: `1`
- Estimated Minutes: `0`
- Dependencies: `Task 1`
- Parent: `none`
- Status: `open`

> Epic container for Phase 2. Children: Tasks 12–22. Independent of Phase 3a (`worktree`) — these touch `app`/`api`/`cmd`, not git.

---

### Task 12: Config — add the `Orchestrator` block to `kernl.yaml`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: `14`
- Dependencies: `Task 7`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Modify: `orchestrator/internal/config/config.go`
- Test: `orchestrator/internal/config/config_test.go`
- Modify: `orchestrator/kernl.yaml.example`

**Description / Steps:**
- [ ] **Step 1: Write the failing test**

`orchestrator/internal/config/config_test.go` — add:
```go
func TestLoadAppliesOrchestratorDefaults(t *testing.T) {
	path := writeTempYAML(t, "settings:\n  agents: {}\n  pools: {}\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Orchestrator.MaxConcurrentBeads != 5 {
		t.Errorf("MaxConcurrentBeads default = %d, want 5", cfg.Orchestrator.MaxConcurrentBeads)
	}
	if cfg.Orchestrator.WorktreeRoot == "" {
		t.Error("WorktreeRoot default must be set (~/.kernl/worktrees)")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/config/ -run TestLoadAppliesOrchestrator -v
```
Expected: FAIL — `cfg.Orchestrator` undefined.

- [ ] **Step 3: Add the struct and defaults**

In `config.go`:
```go
type OrchestratorConfig struct {
	WorktreeRoot       string `yaml:"worktreeRoot,omitempty"`
	MaxConcurrentBeads int    `yaml:"maxConcurrentBeads,omitempty"`
	RunStatePath       string `yaml:"runStatePath,omitempty"`
}
```
Add `Orchestrator OrchestratorConfig \`yaml:"orchestrator"\`` to `Config`. In `Load`, after the existing defaults: if `MaxConcurrentBeads == 0` set `5`; if `WorktreeRoot == ""` set `filepath.Join(os.UserHomeDir(), ".kernl", "worktrees")`; if `RunStatePath == ""` set `filepath.Join(os.UserHomeDir(), ".kernl", "runstate.db")`.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/config/ -run TestLoadAppliesOrchestrator -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(config): add orchestrator block (worktree root, max-concurrent-beads, run-state path)"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/config/` exits 0 including the new test.
- [ ] `Config.Orchestrator` has `WorktreeRoot`, `MaxConcurrentBeads` (default 5), `RunStatePath`.

---

### Task 13: Rich, commented `kernl.yaml.example` + actionable empty-config error

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `16`
- Dependencies: `Task 12`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Modify: `orchestrator/kernl.yaml.example`
- Modify: `orchestrator/internal/config/config.go` (validation in `Load`)
- Test: `orchestrator/internal/config/config_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing validation test**

```go
func TestLoadRejectsEmptyAgentsWithActionableError(t *testing.T) {
	path := writeTempYAML(t, "settings:\n  agents: {}\n  pools: {}\n")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for config with zero agents")
	}
	for _, want := range []string{"KERNL DISPATCH FAILURE", "settings.agents", "kernl.yaml.example"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
}
```
Note: this changes `Load`'s behaviour for genuinely empty configs — update `TestLoadAppliesOrchestratorDefaults` (Task 12) to include one stub agent so it still exercises the defaults path.

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/config/ -run TestLoadRejectsEmpty -v
```
Expected: FAIL — no error returned.

- [ ] **Step 3: Implement the validation**

In `Load`, after defaults: if `len(cfg.Settings.Agents) == 0` return
```go
fmt.Errorf("KERNL DISPATCH FAILURE: %s defines zero agents under settings.agents — the orchestrator cannot dispatch. Fix: copy kernl.yaml.example and fill in at least one agent. Next: kernl doctor", path)
```

- [ ] **Step 4: Write the rich example**

`kernl.yaml.example` — one fully-filled `opencode` agent (command, args, type, vendor, model — every field commented with what it does), one pool referencing it, one `registry.repos` entry, the `orchestrator` block with `worktreeRoot` / `maxConcurrentBeads` / `runStatePath` commented. Every map is populated, not `{}`.

- [ ] **Step 5: Run to verify it passes + sanity-load the example**

```bash
go test ./internal/config/ -run TestLoadRejectsEmpty -v
go run ./cmd/kernl doctor --config kernl.yaml.example   # after Task 18; until then: a one-off Load() check
```
Expected: test PASS; the example file parses without error.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat(config): rich commented kernl.yaml.example + actionable empty-config error"
```

**Acceptance Criteria:**
- [ ] Loading a zero-agent config returns an error containing `KERNL DISPATCH FAILURE`, `settings.agents`, and `kernl.yaml.example`.
- [ ] `kernl.yaml.example` has zero empty `{}` maps; every field carries an explanatory comment.
- [ ] `go test ./internal/config/` exits 0.

---

### Task 14: `preflight` package — the `kernl doctor` checks

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `26`
- Dependencies: `Task 12`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/preflight/preflight.go`
- Test: `orchestrator/internal/preflight/preflight_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing test**

```go
package preflight

import "testing"

func TestRunCollectsAllChecks(t *testing.T) {
	fakeLook := func(bin string) (string, error) {
		if bin == "bd" { return "/usr/bin/bd", nil }
		return "", errNotFound
	}
	rep := Run(Deps{LookPath: fakeLook, ConfigPath: "testdata/ok.yaml", GoVersion: "go1.26"})
	if rep.Check("bd").OK != true {
		t.Error("bd check should pass when LookPath finds it")
	}
	if rep.Check("opencode").OK != false {
		t.Error("opencode check should fail when LookPath misses it")
	}
	if rep.Check("opencode").Fix == "" {
		t.Error("a failing check must carry an actionable Fix string")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/preflight/ -v
```
Expected: FAIL — package undefined.

- [ ] **Step 3: Implement `preflight`**

`Deps` struct (injectable `LookPath`, `ConfigPath`, `GoVersion` — keeps it hermetic). `Run(Deps) Report`. Checks: `bd` in PATH, `opencode` in PATH, Go version ≥ 1.26, `kernl.yaml` exists and `config.Load` succeeds. Each `Check` has `Name`, `OK bool`, `Detail string`, `Fix string`. A failing check's `Fix` names the exact remedy (e.g. `"install bd: see https://github.com/gastownhall/beads"`). `Report.Check(name)` and `Report.AllOK()`.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/preflight/ -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(preflight): kernl doctor checks (bd, opencode, go version, config)"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/preflight/` exits 0.
- [ ] Every failing `Check` carries a non-empty actionable `Fix`.
- [ ] `preflight` has no global state and takes all externals via `Deps` (hermetic).

---

### Task 15: SessionDriver — spawn the agent process and drive the take loop

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `48`
- Dependencies: `Task 12`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/app/driver.go`
- Create: `orchestrator/internal/app/nudge.go`
- Test: `orchestrator/internal/app/driver_test.go`
- Test: `orchestrator/internal/app/nudge_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing nudge-builder test**

The single parameterized nudge builder (eng review Issue 8 — DRY). `orchestrator/internal/app/nudge_test.go`:
```go
package app

import (
	"strings"
	"testing"
)

func TestBuildNudgePromptByCause(t *testing.T) {
	turnEnded := BuildNudgePrompt(NudgeInput{BeadID: "kb-1", State: "implementing", Cause: NudgeTurnEnded})
	if !strings.Contains(turnEnded, "kb-1") || !strings.Contains(turnEnded, "implementing") {
		t.Errorf("turn-ended nudge must name bead and state: %q", turnEnded)
	}
	resumed := BuildNudgePrompt(NudgeInput{BeadID: "kb-1", State: "implementing", Cause: NudgeResumedAfterInterruption})
	if resumed == turnEnded {
		t.Error("resumed-after-interruption nudge must differ from turn-ended")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/app/ -run TestBuildNudge -v
```
Expected: FAIL — undefined.

- [ ] **Step 3: Implement the nudge builder**

`orchestrator/internal/app/nudge.go` — `NudgeCause` enum (`NudgeTurnEnded`, `NudgeResumedAfterInterruption`), `NudgeInput{BeadID, State, Cause}`, `BuildNudgePrompt(NudgeInput) string`. The core message body lives here once; cause only varies the framing sentence. This generalizes `terminal.BuildTakeLoopFollowUpPrompt`.

- [ ] **Step 4: Write the failing driver test (with a fake process spawner)**

`driver_test.go`:
```go
func TestDriverRunBeadAdvancesViaTakeLoop(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "ready_for_implementation"}}
	spawn := &fakeSpawner{ // emits a scripted opencode NDJSON stream then exits 0
		script: `{"type":"output","content":"ok"}
{"type":"turn_ended"}
`,
		onExit: func() { be.state["kb-1"] = "done" },
	}
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: newTestSCM()})
	res, err := d.RunBead(context.Background(), RunBeadInput{BeadID: "kb-1", RepoPath: t.TempDir(), AgentID: "opencode"})
	if err != nil {
		t.Fatalf("RunBead: %v", err)
	}
	if res.FinalState != "done" {
		t.Errorf("FinalState = %q, want done", res.FinalState)
	}
	if !spawn.spawned {
		t.Error("driver must spawn the agent process")
	}
}
```

- [ ] **Step 5: Run to verify it fails**

```bash
go test ./internal/app/ -run TestDriverRunBead -v
```
Expected: FAIL — `NewSessionDriver` undefined.

- [ ] **Step 6: Implement the SessionDriver**

`orchestrator/internal/app/driver.go`. `DriverDeps` injects `Backend backend.BackendPort`, `Spawn SpawnFunc` (`func(ctx, cmd, args, cwd, env) (Process, stdout, stderr io.Reader, err error)` — the real impl uses `exec.CommandContext`; tests inject a fake), `SCM *session.SessionConnectionManager`. `RunBead(ctx, RunBeadInput) (RunBeadResult, error)`:
1. Resolve the agent command from config; resolve dialect via `adapter.ResolveDialect`.
2. `Spawn` the process in `RepoPath`; create a `session.SessionRuntime` (via `NewSessionRuntimeWithCapabilities`) and `Start(ctx, stdout, stderr)`.
3. Pump `runtime.Events()` into the `SessionConnectionManager` for that session id.
4. On `turn_ended`: call the take-loop decision path (`terminal.HandleTakeLoopTurnEnded` with `FollowUpDeps` → may send a `BuildNudgePrompt(...NudgeTurnEnded)` via `runtime.SendUserTurn`).
5. On process exit: call `terminal.HandleTakeIterationClose(...)` to classify success/failure and observe the post-exit bead state.
6. Return `RunBeadResult{SessionID, FinalState, Success}`.
This is the missing glue between `adapter` + `session.SessionRuntime` + the `terminal` take-loop functions. Keep `driver.go` < 500 lines; split a `driver_pump.go` if the event-pump grows past ~120 lines.

- [ ] **Step 7: Run to verify both tests pass**

```bash
go test ./internal/app/ -run 'TestDriver|TestBuildNudge' -v
```
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "feat(app): SessionDriver — spawn agent, pump runtime events, drive take loop"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/app/` exits 0 (driver + nudge tests).
- [ ] `RunBead` spawns a process, wires it to `session.SessionRuntime`, pumps events into the `SessionConnectionManager`, and returns the observed post-exit bead state.
- [ ] `BuildNudgePrompt` is the single nudge source, parameterized by `NudgeCause`.
- [ ] The real `SpawnFunc` is the only place in `internal/app` that calls `exec.Command*`; the driver logic is tested with a fake spawner.

---

### Task 16: `App` — engine assembly from config

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `28`
- Dependencies: `Task 15`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/app/app.go`
- Test: `orchestrator/internal/app/app_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing test**

```go
func TestNewAppWiresEngineFromConfig(t *testing.T) {
	cfg := testConfig(t) // 1 opencode agent, 1 pool, 1 repo, orchestrator block
	a, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if a.Backend == nil || a.Terminal == nil || a.SCM == nil || a.Driver == nil {
		t.Fatal("NewApp must wire backend, terminal manager, SCM, and driver")
	}
	if a.Backend.Capabilities().Workflows == false {
		t.Error("expected bd backend capabilities")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/app/ -run TestNewApp -v
```
Expected: FAIL — `NewApp` undefined.

- [ ] **Step 3: Implement `NewApp`**

`orchestrator/internal/app/app.go`. `App` struct holds `Backend backend.BackendPort`, `Terminal *terminal.TerminalManager`, `SCM *session.SessionConnectionManager`, `Driver *SessionDriver`, `Config *config.Config`. `NewApp(*config.Config) (*App, error)`:
- Build the `bd` backend via `backend.NewBdCliBackend(repoPath)` for the registered repo (fail-loud if `registry.repos` is empty).
- `terminal.NewTerminalManager(terminal.WithMaxSessions(cfg.Orchestrator.MaxConcurrentBeads))`.
- `session.NewSessionConnectionManager(provider, notify)` — the `provider` is a thin adapter over `TerminalManager` satisfying `session.SessionProvider`.
- `NewSessionDriver(DriverDeps{...})` with the real `exec`-based `SpawnFunc`.
This is the entrypoint that "monta o motor" — today nothing does this.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/app/ -run TestNewApp -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(app): NewApp — assemble engine (backend, terminal, SCM, driver) from config"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/app/` exits 0.
- [ ] `NewApp` returns an `*App` with non-nil `Backend`, `Terminal`, `SCM`, `Driver`.
- [ ] `NewApp` fails loud (`KERNL DISPATCH FAILURE`, naming `registry.repos`) when no repo is registered.

---

### Task 17: Real API handlers — beads + sessions wired to the engine

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `30`
- Dependencies: `Task 16`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Modify: `orchestrator/internal/api/beads.go` (stubs → real)
- Modify: `orchestrator/internal/api/streams.go` (route to `SCM.ServeSSE`)
- Modify: `orchestrator/internal/api/routes.go` (`NewRouter` takes `*app.App`)
- Test: `orchestrator/internal/api/beads_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing handler test**

```go
func TestListBeadsHandlerReturnsBackendBeads(t *testing.T) {
	a := &app.App{Backend: &fakeBackend{beads: []backend.Bead{{ID: "kb-1", Title: "first"}}}, Config: testCfg()}
	r := NewRouter(a)
	req := httptest.NewRequest("GET", "/api/beads", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	var got []backend.Bead
	json.Unmarshal(w.Body.Bytes(), &got)
	if len(got) != 1 || got[0].ID != "kb-1" {
		t.Errorf("body = %s", w.Body.String())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/api/ -run TestListBeadsHandler -v
```
Expected: FAIL — `NewRouter` signature mismatch / handler returns `[]struct{}{}`.

- [ ] **Step 3: Implement the real handlers**

Change `NewRouter(cfg)` → `NewRouter(a *app.App)`. `RegisterBeadRoutes(mux, a)` etc. `listBeadsHandler` calls `a.Backend.List(nil, repoPath)`; `getBeadHandler` → `a.Backend.Get(id, repoPath)`; `createBeadHandler` → `a.Backend.Create(...)`. `streams.go` `sessionEventsHandler` → `a.SCM.ServeSSE(w, r, r.PathValue("id"))` (the real fan-out exists; only the route was a broken stub). Every handler that hits a missing resource fails loud with `KERNL DISPATCH FAILURE` + the fix.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/api/ -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(api): wire bead + session-SSE handlers to the engine (drop stubs)"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/api/` exits 0.
- [ ] No handler in `beads.go` returns `struct{}{}` / `[]struct{}{}`.
- [ ] `GET /api/sessions/{id}/events` routes to `SCM.ServeSSE` (not the old broken stub).

---

### Task 18: `kernl` CLI — `main` + `serve` + `doctor`

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `30`
- Dependencies: `Task 14`, `Task 17`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Modify: `orchestrator/cmd/kernl/main.go` (rewrite — subcommand dispatch)
- Create: `orchestrator/cmd/kernl/serve.go`
- Create: `orchestrator/cmd/kernl/doctor.go`
- Test: `orchestrator/cmd/kernl/cli_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing subcommand-dispatch test**

```go
func TestDispatchUnknownSubcommandFailsLoud(t *testing.T) {
	err := Dispatch([]string{"frobnicate"})
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("expected loud failure for unknown subcommand, got %v", err)
	}
}

func TestDispatchDoctorRunsPreflight(t *testing.T) {
	var ran bool
	doctorFn = func(args []string) error { ran = true; return nil }
	t.Cleanup(func() { doctorFn = runDoctor })
	if err := Dispatch([]string{"doctor"}); err != nil || !ran {
		t.Fatalf("doctor not dispatched: ran=%v err=%v", ran, err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./cmd/kernl/ -v
```
Expected: FAIL — `Dispatch` undefined.

- [ ] **Step 3: Implement the CLI skeleton**

`main.go`: `main()` calls `Dispatch(os.Args[1:])`; `Dispatch` routes `serve` / `doctor` / `epic` / `bead` / `--help` to per-command funcs (indirected through package vars like `doctorFn` so they're test-swappable), unknown → `KERNL DISPATCH FAILURE: unknown subcommand %q. Run: kernl --help`. `serve.go`: `runServe` = the current `main.go` HTTP server, but built from `app.NewApp(cfg)` + `api.NewRouter(a)`, printing `kernl serving — API http://localhost:PORT` on startup. `doctor.go`: `runDoctor` runs `preflight.Run` and prints each check plain-text (`✓ bd` / `✗ opencode — <Detail>  Fix: <Fix>`), exits non-zero if `!AllOK()`. Also: `serve` and `epic run` call `preflight.Run` first and abort early with the doctor output if a hard check fails (F1).

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./cmd/kernl/ -v && go build ./cmd/kernl/
```
Expected: PASS; binary builds.

- [ ] **Step 5: Manual smoke**

```bash
go run ./cmd/kernl doctor --config kernl.yaml.example
go run ./cmd/kernl --help
```
Expected: doctor prints check lines; `--help` lists subcommands.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat(cli): kernl binary — subcommand dispatch + serve + doctor"
```

**Acceptance Criteria:**
- [ ] `go test ./cmd/kernl/` exits 0; `go build ./cmd/kernl/` produces a binary.
- [ ] `kernl doctor` prints one plain-text line per check; failing checks show `Fix:`.
- [ ] Unknown subcommand fails loud with `KERNL DISPATCH FAILURE` and points to `kernl --help`.
- [ ] `kernl serve` runs preflight first and aborts early on a hard-failed check.

---

### Task 19: `kernl bead run <id>` — single-bead dispatch via the driver

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `22`
- Dependencies: `Task 18`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Create: `orchestrator/cmd/kernl/bead.go`
- Test: `orchestrator/cmd/kernl/bead_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing test**

```go
func TestRunBeadCmdInvokesDriver(t *testing.T) {
	var gotID string
	fakeApp := &app.App{ /* driver with a fake spawner */ }
	err := runBeadWithApp(fakeApp, []string{"kb-1"}, func(id string) { gotID = id })
	if err != nil || gotID != "kb-1" {
		t.Fatalf("bead run did not drive kb-1: got=%q err=%v", gotID, err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./cmd/kernl/ -run TestRunBeadCmd -v
```
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `bead run`**

`bead.go`: `runBead(args)` parses the bead id, builds the `App`, resolves the repo path + agent, calls `app.Driver.RunBead(ctx, ...)`, prints plain-text progress (`bead kb-1 → implementing`, `agent opencode spawned`, `bead kb-1 → done`). This is the operable command behind the "Passo A" milestone.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./cmd/kernl/ -run TestRunBeadCmd -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(cli): kernl bead run <id> — single-bead dispatch"
```

**Acceptance Criteria:**
- [ ] `go test ./cmd/kernl/` exits 0 including `bead run`.
- [ ] `kernl bead run <id>` drives one bead through the `SessionDriver` and prints plain-text progress.

---

### Task 20: `kernl epic list` — epic discovery from `.beads/`

**Bead Mapping:**
- type: `task`
- Priority: `3`
- Estimated Minutes: `18`
- Dependencies: `Task 19`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Create: `orchestrator/cmd/kernl/epic.go` (the `epic list` half; `epic run` added in Task 23)
- Test: `orchestrator/cmd/kernl/epic_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing test**

```go
func TestEpicListShowsEpicsWithChildCounts(t *testing.T) {
	be := &fakeBackend{beads: []backend.Bead{
		{ID: "kb-0", Type: "epic", Title: "demo epic"},
		{ID: "kb-1", Type: "task", ParentID: "kb-0"},
		{ID: "kb-2", Type: "task", ParentID: "kb-0"},
	}}
	out := captureEpicList(t, be)
	if !strings.Contains(out, "kb-0") || !strings.Contains(out, "demo epic") || !strings.Contains(out, "2") {
		t.Errorf("epic list output missing id/title/child-count: %q", out)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./cmd/kernl/ -run TestEpicList -v
```
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `epic list`**

`epic.go`: `runEpicList(args)` builds the `App`, `a.Backend.List(&backend.BeadListFilters{Type: "epic"}, repoPath)`, counts children per epic via a second `List` (or `ListDependencies`), prints a plain-text table: `ID  TITLE  CHILDREN  STATE`.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./cmd/kernl/ -run TestEpicList -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(cli): kernl epic list — discover epics in .beads/"
```

**Acceptance Criteria:**
- [ ] `go test ./cmd/kernl/` exits 0 including `epic list`.
- [ ] `kernl epic list` prints id, title, child count, and state per epic.

---

### Task 21: README + glossary — Phase 2 slice

**Bead Mapping:**
- type: `task`
- Priority: `3`
- Estimated Minutes: `14`
- Dependencies: `Task 20`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Create/Modify: `/home/gabriel/repositories/kernl/README.md`
- Create/Modify: `/home/gabriel/repositories/kernl/docs/glossary.md`

**Description / Steps:**
- [ ] **Step 1: Write the README quickstart skeleton**

`README.md`: project one-liner (from STRATEGY), prerequisites (`bd`, `opencode`, Go 1.26), quickstart — `cp orchestrator/kernl.yaml.example kernl.yaml`, `kernl doctor`, `kernl epic list`, `kernl bead run <id>`. Leave a `<!-- GIF: examples/parallel-demo -->` marker for Task 31. A note that integration tests + the packaged example spend real opencode tokens/time.

- [ ] **Step 2: Seed the glossary with Phase 0–2 terms**

`docs/glossary.md`: `bead` (and why not "beat"), `epic`, `take loop`, `dispatch`, `cross-agent review`, `harness/agent`, `knots — dormant`. One paragraph each, anchored to `orchestrator/specs/00-architecture.md` where relevant.

- [ ] **Step 3: Commit**

```bash
git add README.md docs/glossary.md && git commit -m "docs: README quickstart + glossary (Phase 2 slice)"
```

**Acceptance Criteria:**
- [ ] `README.md` is non-empty, has a prerequisites section and a copy-pasteable quickstart using the real subcommand names.
- [ ] `docs/glossary.md` defines `bead`, `epic`, `take loop`, `dispatch`, `cross-agent review`, `knots-dormant`.

---

### Task 22: Passo A — integration test: one real bead, real opencode

**Bead Mapping:**
- type: `story`
- Priority: `3`
- Estimated Minutes: `20`
- Dependencies: `Task 10`, `Task 19`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/integration/passo_a_test.go` (`//go:build integration`)

**Description / Steps:**
- [ ] **Step 1: Write the Passo A integration test**

```go
//go:build integration

package integration

import (
	"context"
	"testing"
)

func TestPassoA_SingleBeadRealOpencode(t *testing.T) {
	h := NewHarness(t) // skips loudly if bd/opencode absent
	defer h.Cleanup()

	a := h.App()
	res, err := a.Driver.RunBead(context.Background(), app.RunBeadInput{
		BeadID:   h.SeedBead(t, "ready_for_implementation"),
		RepoPath: h.RepoPath,
		AgentID:  "opencode",
	})
	if err != nil {
		t.Fatalf("RunBead: %v", err)
	}
	final := h.BeadState(t, res.SessionID)
	if !h.IsAdvanced(final) {
		t.Errorf("bead did not advance past ready_for_implementation: state=%q", final)
	}
}
```

- [ ] **Step 2: Run it (opt-in, spends tokens)**

```bash
cd orchestrator && go test -tags=integration ./internal/integration/ -run TestPassoA -v
```
Expected: PASS — real opencode spawns, the take loop advances the bead. (SKIP if `bd`/`opencode` unavailable.)

- [ ] **Step 3: Confirm the default suite is still clean**

```bash
go test ./... 2>&1 | tail -2
```

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "test(integration): Passo A — one real bead, real opencode, take loop advances"
```

**Acceptance Criteria:**
- [ ] `go test -tags=integration -run TestPassoA` PASSES on a machine with `bd` + `opencode`, or SKIPS loudly without them.
- [ ] The test asserts the bead advanced past `ready_for_implementation` in `bd` after a real opencode run.
- [ ] `go test ./...` (no tag) pass count unchanged.

---

## Phase 3 — Core Feature: parallel epic execution

### Task 23: Parallel epic execution — WorktreeManager + EpicExecutor

**Bead Mapping:**
- type: `epic`
- Priority: `2`
- Estimated Minutes: `0`
- Dependencies: `Task 1`
- Parent: `none`
- Status: `open`

> Epic container for Phase 3. Children: Tasks 24–29. Task 24 (`worktree`) is independent of Phase 2 and can run in parallel with it; Tasks 25–29 depend on both the `worktree` package and the Phase 2 `App`/`Driver`.

---

### Task 24: WorktreeManager — the only git-aware package

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `34`
- Dependencies: `Task 7`
- Parent: `Task 23`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/worktree/worktree.go`
- Test: `orchestrator/internal/worktree/worktree_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing test (fake `git` runner)**

```go
package worktree

import (
	"path/filepath"
	"testing"
)

func TestAddCreatesWorktreeAtEpicBeadPath(t *testing.T) {
	var gotArgs []string
	run := func(dir string, args ...string) (string, error) { gotArgs = args; return "", nil }
	m := New(Deps{Root: "/tmp/kr", Run: run})
	path, err := m.Add("epic-1", "kb-3")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	want := filepath.Join("/tmp/kr", "epic-1", "kb-3")
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if gotArgs[0] != "worktree" || gotArgs[1] != "add" {
		t.Errorf("expected `git worktree add`, got %v", gotArgs)
	}
	// branch name kernl/<bead-id>
	if !contains(gotArgs, "kernl/kb-3") {
		t.Errorf("expected branch kernl/kb-3 in %v", gotArgs)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/worktree/ -v
```
Expected: FAIL — package undefined.

- [ ] **Step 3: Implement WorktreeManager**

`worktree.go`. `Deps{Root string, Run func(dir string, args ...string) (string, error)}` — the real `Run` shells `git`; tests inject a fake. `Manager` with:
- `Add(epicID, beadID) (path string, err error)` — `git worktree add <root>/<epic>/<bead> -b kernl/<bead> <baseRef>`; if the path already exists, fail loud (`KERNL DISPATCH FAILURE: worktree path <p> already exists — a previous run left it. Fix: kernl worktree clean (post-MVP) or rm -rf manually. Next: re-run kernl epic run <epic>`).
- `Remove(epicID, beadID) error`, `Prune() error` — `git worktree prune`.
- `Path(epicID, beadID) string` — pure path join.
All git knowledge is confined here; `epic`/`app` call this, never `git` directly.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/worktree/ -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(worktree): WorktreeManager — per-bead git worktree add/remove/prune"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/worktree/` exits 0.
- [ ] `Add` produces path `<root>/<epic-id>/<bead-id>` and branch `kernl/<bead-id>`.
- [ ] `Add` on an existing path fails loud with `KERNL DISPATCH FAILURE` + an actionable fix + next command.
- [ ] `internal/worktree` is the only new package importing/spawning `git`.

---

### Task 25: Epic DAG — ready-set + cycle detection

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `30`
- Dependencies: `Task 7`
- Parent: `Task 23`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/epic/dag.go`
- Test: `orchestrator/internal/epic/dag_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing tests**

```go
package epic

import "testing"

func TestReadySetReturnsBeadsWithSatisfiedDeps(t *testing.T) {
	d, err := NewDAG([]Node{
		{ID: "a"},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"a"}},
		{ID: "d", DependsOn: []string{"b", "c"}},
	})
	if err != nil {
		t.Fatalf("NewDAG: %v", err)
	}
	ready := d.ReadySet(map[string]bool{}) // nothing done yet
	if !sameSet(ready, []string{"a"}) {
		t.Errorf("ready = %v, want [a]", ready)
	}
	ready = d.ReadySet(map[string]bool{"a": true})
	if !sameSet(ready, []string{"b", "c"}) {
		t.Errorf("ready after a = %v, want [b c]", ready)
	}
}

func TestNewDAGRejectsCycle(t *testing.T) {
	_, err := NewDAG([]Node{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	})
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/epic/ -run 'TestReadySet|TestNewDAGRejectsCycle' -v
```
Expected: FAIL — undefined.

- [ ] **Step 3: Implement the DAG**

`dag.go`. `Node{ID string, DependsOn []string}`. `NewDAG([]Node) (*DAG, error)` — builds adjacency, runs Kahn's algorithm; if a topological order can't consume all nodes, return a cycle error naming the involved bead ids (`KERNL DISPATCH FAILURE: dependency cycle in epic — beads [a b]. Fix: correct the dependency graph in the plan and re-convert`). `ReadySet(done map[string]bool) []string` — nodes whose every `DependsOn` is in `done` and which aren't done themselves. Pure, no I/O.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/epic/ -run 'TestReadySet|TestNewDAGRejectsCycle' -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(epic): DAG — ready-set computation + cycle detection"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/epic/` exits 0 for the DAG tests.
- [ ] `ReadySet` returns exactly the beads with all dependencies satisfied and not yet done.
- [ ] `NewDAG` rejects a cyclic graph with an error naming the cycle's bead ids.

---

### Task 26: Epic bead-source — read `.beads/` + translation layer

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `26`
- Dependencies: `Task 25`, `Task 16`
- Parent: `Task 23`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/epic/beadsource.go`
- Test: `orchestrator/internal/epic/beadsource_test.go`
- Test: `orchestrator/internal/epic/testdata/epic-diamond.jsonl`

**Description / Steps:**
- [ ] **Step 1: Write the failing test**

```go
func TestLoadEpicBuildsDAGFromBackend(t *testing.T) {
	be := &fakeBackend{beads: []backend.Bead{
		{ID: "e", Type: "epic"},
		{ID: "c1", ParentID: "e"},
		{ID: "c2", ParentID: "e", Dependencies: []backend.BeadDependency{{SourceID: "c1", TargetID: "c2"}}},
	}}
	ep, err := LoadEpic(be, "e", "/repo")
	if err != nil {
		t.Fatalf("LoadEpic: %v", err)
	}
	if len(ep.Children) != 2 {
		t.Errorf("children = %d, want 2", len(ep.Children))
	}
	ready := ep.DAG.ReadySet(map[string]bool{})
	if !sameSet(ready, []string{"c1"}) {
		t.Errorf("ready = %v, want [c1]", ready)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/epic/ -run TestLoadEpic -v
```
Expected: FAIL — undefined.

- [ ] **Step 3: Implement the bead-source + translation**

`beadsource.go`. `LoadEpic(be backend.BackendPort, epicID, repoPath) (*Epic, error)`:
- `be.Get(epicID, repoPath)` — must be `type: epic`, else fail loud.
- `be.List(&backend.BeadListFilters{Parent: epicID}, repoPath)` for children.
- Map each child's `Dependencies` (`BeadDependency.SourceID/TargetID`) into `epic.Node{ID, DependsOn}` — the **translation layer**: if `vc-convert-plan-to-beads` emits a shape the `bd` adapter doesn't round-trip cleanly, normalize it here (and only here). Fail loud on a parse mismatch (`KERNL DISPATCH FAILURE: epic <id> child <c> has a dependency shape the bd adapter did not expect — <detail>`).
- Build the `*DAG`; a cycle error from `NewDAG` propagates.
`Epic{ID, Children []backend.Bead, DAG *DAG}`.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/epic/ -run TestLoadEpic -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(epic): bead-source — load epic from .beads/ + dependency translation"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/epic/` exits 0 for bead-source tests.
- [ ] `LoadEpic` returns the parent's children and a `*DAG` built from their dependencies.
- [ ] A non-epic id, or a dependency shape mismatch, fails loud with `KERNL DISPATCH FAILURE` + detail.

---

### Task 27: Epic events — types + parallelism counters

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `16`
- Dependencies: `Task 25`
- Parent: `Task 23`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/epic/events.go`
- Test: `orchestrator/internal/epic/events_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing test**

```go
func TestParallelismTrackerRecordsPeakAndRealized(t *testing.T) {
	pt := NewParallelismTracker(3) // graph allowed up to 3 concurrent
	pt.Started("a"); pt.Started("b")
	pt.Finished("a")
	pt.Started("c")
	m := pt.Metric()
	if m.Peak != 2 {
		t.Errorf("Peak = %d, want 2", m.Peak)
	}
	if m.GraphMax != 3 {
		t.Errorf("GraphMax = %d, want 3", m.GraphMax)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/epic/ -run TestParallelismTracker -v
```
Expected: FAIL — undefined.

- [ ] **Step 3: Implement events + tracker**

`events.go`. `EpicEvent{Type EpicEventType, EpicID, BeadID, SessionID, Detail string, Time int64}`; `EpicEventType` consts: `BeadStateChanged`, `SessionStarted`, `SessionError`, `WaveAdvanced`. `ParallelismTracker` — `Started/Finished` (mutex-guarded), `Metric() ParallelismMetric{Peak, GraphMax, Realized float64}` where `Realized = Peak/GraphMax`. This is the STRATEGY's "paralelismo realizado" metric, the one metric the MVP instruments.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/epic/ -run TestParallelismTracker -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(epic): epic event types + parallelism tracker (realized parallelism metric)"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/epic/` exits 0 for events tests.
- [ ] `EpicEventType` covers `BeadStateChanged`, `SessionStarted`, `SessionError`, `WaveAdvanced`.
- [ ] `ParallelismTracker.Metric()` reports `Peak`, `GraphMax`, and `Realized` and is concurrency-safe.

---

### Task 28a: EpicExecutor core — wave dispatch loop + ready-set integration

**Bead Mapping:**
- type: `task`
- Priority: `3`
- Estimated Minutes: `30`
- Dependencies: `Task 24`, `Task 26`, `Task 27`
- Parent: `Task 23`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/epic/executor.go` (wave dispatch loop only)
- Test: `orchestrator/internal/epic/executor_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing concurrency test**

```go
func TestExecutorRunsIndependentChildrenConcurrently(t *testing.T) {
	ep := diamondEpic(t) // a -> {b,c} -> d
	var mu sync.Mutex
	var concurrent, peak int
	runBead := func(ctx context.Context, in RunInput) (RunResult, error) {
		mu.Lock(); concurrent++; if concurrent > peak { peak = concurrent }; mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock(); concurrent--; mu.Unlock()
		return RunResult{FinalState: "done", Success: true}, nil
	}
	ex := NewExecutor(ExecutorDeps{Epic: ep, RunBead: runBead, Worktree: fakeWT(), MaxConcurrent: 5})
	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if peak < 2 {
		t.Errorf("b and c should run concurrently, peak = %d", peak)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/epic/ -run TestExecutorRunsIndependent -v
```
Expected: FAIL — `NewExecutor` undefined.

- [ ] **Step 3: Implement the core executor**

`executor.go`. `ExecutorDeps{Epic *Epic, RunBead func(ctx, RunInput) (RunResult, error), Worktree worktreeAdder, MaxConcurrent int}`. `Run(ctx)`:
1. Loop: compute `dag.ReadySet(done)`; if empty and not all done → `EpicCompleted`.
2. For each ready bead, `Worktree.Add(epicID, beadID)`, launch a goroutine calling `RunBead`; collect results in a channel.
3. On child result: `Success` → mark `done`; `!Success` → return error.
4. All done → `EpicCompleted`.
`DoneSet() map[string]bool`, `State() EpicState`. `Emit` is a nil no-op in this task (the hook is wired in Task 28b). `MaxConcurrent` is accepted but unconstrained — the semaphore is added next.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/epic/ -run TestExecutorRunsIndependent -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(epic): EpicExecutor core — wave dispatch + ready-set loop"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/epic/` exits 0 including the concurrency test.
- [ ] Independent children run concurrently; all children complete in dependency order.
- [ ] `executor.go` < 500 lines.

---

### Task 28b: EpicExecutor — semaphore, fail-fast, Emit hook + parallelism tracking

**Bead Mapping:**
- type: `task`
- Priority: `3`
- Estimated Minutes: `22`
- Dependencies: `Task 28a`
- Parent: `Task 23`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/epic/wave.go` (semaphore + fail-fast logic)
- Test: `orchestrator/internal/epic/executor_test.go` (append tests)

**Description / Steps:**
- [ ] **Step 1: Write the failing fail-fast + semaphore tests**

```go
func TestExecutorFailFastOnTerminalChildFailure(t *testing.T) {
	ep := diamondEpic(t)
	runBead := func(ctx context.Context, in RunInput) (RunResult, error) {
		if in.BeadID == "b" {
			return RunResult{FinalState: "blocked", Success: false}, nil
		}
		return RunResult{FinalState: "done", Success: true}, nil
	}
	ex := NewExecutor(ExecutorDeps{Epic: ep, RunBead: runBead, Worktree: fakeWT(), MaxConcurrent: 5})
	err := ex.Run(context.Background())
	if err == nil {
		t.Fatal("expected fail-fast error when child b fails terminally")
	}
	if ex.State() != EpicBlocked {
		t.Errorf("epic state = %v, want blocked", ex.State())
	}
	if ex.Dispatched("d") {
		t.Error("d must not be dispatched after b failed")
	}
}

func TestExecutorSemaphoreCapsConcurrency(t *testing.T) {
	ep := wideEpic(t, 10) // 10 independent children
	var mu sync.Mutex
	var concurrent, peak int
	runBead := func(ctx context.Context, in RunInput) (RunResult, error) {
		mu.Lock(); concurrent++; if concurrent > peak { peak = concurrent }; mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock(); concurrent--; mu.Unlock()
		return RunResult{FinalState: "done", Success: true}, nil
	}
	ex := NewExecutor(ExecutorDeps{Epic: ep, RunBead: runBead, Worktree: fakeWT(), MaxConcurrent: 3})
	ex.Run(context.Background())
	if peak > 3 {
		t.Errorf("peak %d exceeded MaxConcurrent 3", peak)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/epic/ -run 'TestExecutorFailFast|TestExecutorSemaphore' -v
```
Expected: FAIL — `EpicBlocked` undefined or semaphore not enforced.

- [ ] **Step 3: Implement semaphore, fail-fast, and Emit**

`wave.go` (separate file if `executor.go` > 500 lines). Additions:
- `chan struct{}` semaphore of size `MaxConcurrent`: acquire before launching a goroutine, release on result receipt.
- `fail-fast`: on child `!Success`, set `failFast=true`, stop dispatching new waves, wait for in-flight siblings (collecting their results without dispatching new ones), set `EpicBlocked`, return error.
- `Emit func(EpicEvent)` hook: emit `SessionStarted`, `WaveAdvanced`, `BeadStateChanged`, `SessionError` at the correct lifecycle points (fed by the epic-SSE hub in Task 31).
- `Dispatched(id) bool` tracker.
- `Parallelism() ParallelismMetric` — delegates to the tracker from Task 27.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/epic/ -run 'TestExecutorFailFast|TestExecutorSemaphore' -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(epic): semaphore + fail-fast + Emit hook + parallelism tracking"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/epic/` exits 0 including all executor tests.
- [ ] Concurrency never exceeds `MaxConcurrent`.
- [ ] A terminal child failure stops new-wave dispatch, sets epic state `blocked`, and dependent beads are never dispatched.
- [ ] `Emit` is wired and callable; `Dispatched(id)` and `Parallelism()` are exposed.

---

### Task 29: `kernl epic run <id>` — wire executor to CLI + embedded server

**Bead Mapping:**
- type: `story`
- Priority: `3`
- Estimated Minutes: `34`
- Dependencies: `Task 28b`, `Task 20`
- Parent: `Task 23`
- Status: `open`

**Files:**
- Modify: `orchestrator/cmd/kernl/epic.go` (add `epic run`)
- Test: `orchestrator/cmd/kernl/epic_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing test**

```go
func TestEpicRunWiresExecutorAndServesGUI(t *testing.T) {
	fakeApp := testAppWithDiamondEpic(t)
	var guiURLPrinted bool
	err := runEpicWithApp(fakeApp, []string{"e"}, func(line string) {
		if strings.Contains(line, "GUI ") && strings.Contains(line, "http://") {
			guiURLPrinted = true
		}
	})
	if err != nil {
		t.Fatalf("epic run: %v", err)
	}
	if !guiURLPrinted {
		t.Error("epic run must print the embedded GUI URL on startup")
	}
}

func TestEpicRunBlockedPrintsNextStep(t *testing.T) {
	fakeApp := testAppWithFailingChild(t)
	var out strings.Builder
	runEpicWithApp(fakeApp, []string{"e"}, func(l string) { out.WriteString(l + "\n") })
	s := out.String()
	if !strings.Contains(s, "blocked") || !strings.Contains(s, "kernl epic run e") {
		t.Errorf("blocked output must name the failed bead and the re-run command: %q", s)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./cmd/kernl/ -run TestEpicRun -v
```
Expected: FAIL — `epic run` not implemented.

- [ ] **Step 3: Implement `epic run`**

`runEpic(args)`: preflight first (F1). Build the `App`; `epic.LoadEpic(...)`; start the embedded HTTP server (`api.NewRouter(a)` + `web/`) in a goroutine and **print `GUI em http://localhost:PORT`** on startup (Confusão #1 — one binary, one process). `epic.NewExecutor(...)` with `RunBead` bound to `app.Driver.RunBead` and `Worktree` bound to the `WorktreeManager`; `Emit` pushes `EpicEvent`s to the epic-SSE hub (Task 30). Stream plain-text progress. On `EpicBlocked`: print which bead failed, why, and `corrija e rode kernl epic run <id> de novo para retomar` (F5). On completion: print the realized-parallelism metric.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./cmd/kernl/ -run TestEpicRun -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(cli): kernl epic run <id> — executor + embedded GUI server + blocked next-step"
```

**Acceptance Criteria:**
- [ ] `go test ./cmd/kernl/` exits 0 including `epic run`.
- [ ] `kernl epic run <id>` runs preflight, starts the executor, serves the embedded GUI, and prints its URL.
- [ ] On `blocked`, the CLI output names the failed bead and the exact re-run command.

---

## Phase 4 — Observability (epic-SSE + GUI)

### Task 30: Observability — epic-SSE endpoint + minimal GUI

**Bead Mapping:**
- type: `epic`
- Priority: `3`
- Estimated Minutes: `0`
- Dependencies: `Task 23`
- Parent: `none`
- Status: `open`

> Epic container for Phase 4. Children: Tasks 31–32. Touches `internal/api` + `web/` — runs in parallel with Phase 5 (different packages); coordinate the one shared file (`internal/epic/executor.go` `Emit` hook) per the eng review conflict flag.

---

### Task 31: Epic-SSE endpoint — `GET /api/epics/{id}/events`

**Bead Mapping:**
- type: `task`
- Priority: `3`
- Estimated Minutes: `30`
- Dependencies: `Task 28b`, `Task 17`
- Parent: `Task 30`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/api/epics.go`
- Test: `orchestrator/internal/api/epics_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing test**

```go
func TestEpicEventsHandlerStreamsExecutorEvents(t *testing.T) {
	hub := NewEpicEventHub()
	a := &app.App{ /* ... */ EpicEvents: hub}
	r := NewRouter(a)
	req := httptest.NewRequest("GET", "/api/epics/e/events", nil)
	w := httptest.NewRecorder()
	go func() {
		hub.Publish(epic.EpicEvent{Type: epic.SessionStarted, EpicID: "e", BeadID: "c1"})
		hub.Close("e")
	}()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "session-started") || !strings.Contains(w.Body.String(), "c1") {
		t.Errorf("SSE body missing event: %q", w.Body.String())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/api/ -run TestEpicEventsHandler -v
```
Expected: FAIL — undefined.

- [ ] **Step 3: Implement the epic-SSE hub + handler**

`epics.go`. `EpicEventHub` — per-epic buffered fan-out, reusing the buffering/fan-out **pattern** of `session.SessionConnectionManager` (don't fork the type — mirror the shape: `Publish`, `Subscribe`/`unsub`, bounded buffer, `Close`). `epicEventsHandler` sets SSE headers, replays the buffer, streams live `EpicEvent`s as `data: {...}\n\n` until client disconnect or epic close. `EpicExecutor.Emit` (Task 28) is bound to `hub.Publish`. Register `GET /api/epics/{id}/events` in `routes.go`.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/api/ -run TestEpicEventsHandler -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(api): epic-SSE endpoint /api/epics/{id}/events"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/api/` exits 0 including the epic-SSE test.
- [ ] `GET /api/epics/{id}/events` streams `bead-state-changed` / `session-started` / `session-error` / `wave-advanced` events as SSE.
- [ ] The hub buffers (bounded) and fans out to multiple subscribers.

---

### Task 32: Minimal monitoring GUI (`web/`)

**Bead Mapping:**
- type: `story`
- Priority: `3`
- Estimated Minutes: `36`
- Dependencies: `Task 31`
- Parent: `Task 30`
- Status: `open`

**Files:**
- Create: `orchestrator/web/index.html`
- Create: `orchestrator/web/app.js`
- Create: `orchestrator/web/style.css`
- Test: `orchestrator/internal/api/web_test.go` (static-serving smoke)

**Description / Steps:**
- [ ] **Step 1: Write the failing static-serving test**

```go
func TestServerServesWebIndex(t *testing.T) {
	a := testApp(t)
	r := NewRouter(a)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Kernl") {
		t.Errorf("expected web/index.html at /, got %d %q", w.Code, w.Body.String()[:80])
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/api/ -run TestServerServesWebIndex -v
```
Expected: FAIL — `/` not routed / `web/` missing.

- [ ] **Step 3: Serve `web/` and build the GUI**

Add a static file route in `routes.go` (`http.FileServer` over `web/`, or `embed.FS`). `index.html` + `app.js`: open `EventSource('/api/epics/<id>/events')`, render a live bead list (id, state — color by state), an active-sessions panel (who is doing what), and a red flag on `session-error`. Pure HTML/JS/CSS, no framework, no relation to the original Vue frontend. If SSE fails to connect, `app.js` falls back to polling `/api/beads` every 2s (risk fallback from the spec). Per-session drill-down (the existing `ServeSSE`) — a link per session, minimal priority.

- [ ] **Step 4: Run to verify it passes + manual check**

```bash
go test ./internal/api/ -run TestServerServesWebIndex -v
# manual: kernl epic run <id>, open the printed URL, watch beads change state
```
Expected: test PASS; GUI renders live state during an epic run.

- [ ] **Step 5: Update README + glossary (Phase 4 slice)**

README: a "Monitoring" section (the GUI URL is printed by `epic run`). Glossary: `epic-SSE`, `wave`.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat(web): minimal monitoring GUI consuming epic-SSE + README/glossary slice"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/api/` exits 0 including the static-serving test.
- [ ] The GUI shows a live bead list with state, active sessions, and a visual error flag, driven by epic-SSE.
- [ ] On SSE connection failure the GUI falls back to REST polling.
- [ ] `README.md` documents the monitoring URL; `docs/glossary.md` defines `epic-SSE` and `wave`.

---

## Phase 5 — Durable run-state & resume

### Task 33: Durable run-state & resume

**Bead Mapping:**
- type: `epic`
- Priority: `3`
- Estimated Minutes: `0`
- Dependencies: `Task 23`
- Parent: `none`
- Status: `open`

> Epic container for Phase 5. Children: Tasks 34–36. Runs in parallel with Phase 4 (different packages). Resume design depends on the Task 9 spike verdict.

---

### Task 34: SQLite run-state store

**Bead Mapping:**
- type: `task`
- Priority: `3`
- Estimated Minutes: `34`
- Dependencies: `Task 28b`
- Parent: `Task 33`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/runstate/store.go`
- Test: `orchestrator/internal/runstate/store_test.go`
- Modify: `orchestrator/go.mod` (`modernc.org/sqlite`)

**Description / Steps:**
- [ ] **Step 1: Add the cgo-free SQLite driver**

```bash
cd orchestrator && go get modernc.org/sqlite
```

- [ ] **Step 2: Write the failing test (`:memory:`)**

```go
package runstate

import "testing"

func TestStoreRoundTripsWorktreeAndAgentRecords(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil { t.Fatalf("Open: %v", err) }
	defer s.Close()

	if err := s.SetWorktree("e", "c1", "/tmp/wt/e/c1"); err != nil {
		t.Fatalf("SetWorktree: %v", err)
	}
	wt, ok := s.Worktree("e", "c1")
	if !ok || wt != "/tmp/wt/e/c1" {
		t.Errorf("Worktree = %q,%v", wt, ok)
	}
	s.RecordAgent("c1", "implementing", AgentRecord{AgentID: "opencode", SessionID: "term-1", Status: "running"})
	rec, ok := s.AgentRecord("c1", "implementing")
	if !ok || rec.SessionID != "term-1" {
		t.Errorf("AgentRecord = %+v,%v", rec, ok)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

```bash
go test ./internal/runstate/ -v
```
Expected: FAIL — package undefined.

- [ ] **Step 4: Implement the store**

`store.go`. `Open(path) (*Store, error)` — opens SQLite in **WAL mode** (`PRAGMA journal_mode=WAL` — neutralizes the shared-JSON-rewrite hazard from `GO_PORT.md §2`), creates two tables: `worktrees(epic_id, bead_id, path, PRIMARY KEY(epic_id,bead_id))` and `agent_records(bead_id, state, agent_id, session_id, status, updated_at, PRIMARY KEY(bead_id,state))`. `SetWorktree/Worktree`, `RecordAgent/AgentRecord`. Fail loud on write errors (`KERNL DISPATCH FAILURE` — and note `bd` remains the source of truth for *bead state*; this store only holds run-state). Keep it `< 500` lines.

- [ ] **Step 5: Run to verify it passes**

```bash
go test ./internal/runstate/ -v
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat(runstate): SQLite WAL run-state store (worktree map + agent records)"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/runstate/` exits 0 (tests use `:memory:`).
- [ ] The store persists `bead→worktree` (1:1) and per-`(bead,state)` agent records, opened in WAL mode.
- [ ] Write failures fail loud; the code comments note `bd` is the source of truth for bead state.

---

### Task 35: Resume logic — detect interruption, resume or re-dispatch

**Bead Mapping:**
- type: `task`
- Priority: `4`
- Estimated Minutes: `40`
- Dependencies: `Task 34`, `Task 9`
- Parent: `Task 33`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/epic/resume.go`
- Test: `orchestrator/internal/epic/resume_test.go`

**Description / Steps:**
- [ ] **Step 1: Write the failing tests**

```go
func TestPlanResumeSkipsDoneResumesInterruptedRedispatchesGap(t *testing.T) {
	be := &fakeBackend{state: map[string]string{
		"c1": "done",                     // already terminal -> skip
		"c2": "implementing",             // active in bd -> resume the interrupted session
		"c3": "ready_for_implementation", // advanced but no agent recorded -> fresh dispatch
	}}
	rs := memStore(t)
	rs.RecordAgent("c2", "implementing", runstate.AgentRecord{AgentID: "opencode", SessionID: "term-9", Status: "running"})

	plan := PlanResume(be, rs, diamondEpic(t), "/repo")

	if plan.Action("c1") != ResumeSkip {
		t.Errorf("c1 should be skipped")
	}
	if plan.Action("c2") != ResumeSession || plan.SessionID("c2") != "term-9" {
		t.Errorf("c2 should resume session term-9")
	}
	if plan.Action("c3") != ResumeFreshDispatch {
		t.Errorf("c3 should be a fresh dispatch")
	}
}

func TestResumeMissingWorktreeFailsLoud(t *testing.T) {
	// bd says c2 active, but the worktree path no longer exists on disk
	plan := PlanResume(beActive("c2"), storeWithMissingWorktree(t, "c2"), diamondEpic(t), "/repo")
	if plan.Action("c2") != ResumeError || plan.Detail("c2") == "" {
		t.Errorf("missing worktree must surface as ResumeError with a detail")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd orchestrator && go test ./internal/epic/ -run TestPlanResume -v && go test ./internal/epic/ -run TestResumeMissing -v
```
Expected: FAIL — undefined.

- [ ] **Step 3: Implement resume planning**

`resume.go`. `PlanResume(be, rs, ep, repoPath) ResumePlan`. Per child:
- bead state terminal in `bd` → `ResumeSkip`.
- bead state active in `bd` AND an agent record exists → `ResumeSession` (carry the `SessionID` for `opencode run -s` per the Task 9 spike verdict + a `NudgeResumedAfterInterruption` nudge).
- bead advanced but no agent record (crash in the gap) → `ResumeFreshDispatch`, **preserving cross-agent exclusion** by reading the persisted agent records (this is the cross-agent-review-through-crash hole the eng review Issue 3 closes).
- worktree path recorded but missing on disk → `ResumeError` with a detail (`KERNL DISPATCH FAILURE: worktree for bead <c> missing — <path>. Fix: re-create or rm the bead's branch; Next: kernl epic run <epic>`).
`EpicExecutor.Run` consults the `ResumePlan` before its first wave. The "blocked" exit path = re-running `kernl epic run` is the same mechanism (eng review outside-voice #12).

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/epic/ -run 'TestPlanResume|TestResumeMissing' -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(epic): resume planning — skip done, resume interrupted, re-dispatch gap, fail loud on missing worktree"
```

**Acceptance Criteria:**
- [ ] `go test ./internal/epic/` exits 0 including resume tests.
- [ ] Done beads are skipped; interrupted sessions resume with their `SessionID`; gap-crashes re-dispatch fresh while honoring cross-agent exclusion from persisted records.
- [ ] A missing worktree surfaces as `ResumeError` with an actionable detail.

---

### Task 36: Integration — Passo B + resume + cross-agent-after-crash

**Bead Mapping:**
- type: `story`
- Priority: `4`
- Estimated Minutes: `40`
- Dependencies: `Task 29`, `Task 35`, `Task 10`
- Parent: `Task 33`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/integration/passo_b_test.go` (`//go:build integration`)
- Create: `orchestrator/internal/integration/resume_test.go` (`//go:build integration`)
- Create: `orchestrator/testdata/beads-epic-diamond/.beads/issues.jsonl`

**Description / Steps:**
- [ ] **Step 1: Write the Passo B integration test**

```go
//go:build integration

func TestPassoB_EpicParallelExecution(t *testing.T) {
	h := NewHarness(t)
	defer h.Cleanup()
	epicID := h.SeedEpic(t, "beads-epic-diamond") // parent + >=2 children w/ deps

	ex := h.RunEpic(t, epicID) // wires real driver + worktree + opencode
	if ex.State() != epic.EpicCompleted {
		t.Fatalf("epic state = %v, want completed", ex.State())
	}
	if ex.Parallelism().Peak < 2 {
		t.Errorf("independent children must have run in parallel, peak = %d", ex.Parallelism().Peak)
	}
	for _, id := range h.ChildIDs(epicID) {
		if !h.IsTerminal(h.BeadState(t, id)) {
			t.Errorf("child %s not terminal", id)
		}
	}
}
```

- [ ] **Step 2: Write the resume + cross-agent integration test**

`resume_test.go` — start the diamond epic, kill the process mid-execution, re-run `kernl epic run`: assert done beads are skipped, the interrupted bead resumes, the epic completes; and a crash between `implementation` and `implementation_review` re-dispatches review excluding the persisted implementer.

- [ ] **Step 3: Run them (opt-in, spends tokens)**

```bash
cd orchestrator && go test -tags=integration ./internal/integration/ -run 'TestPassoB|TestResume' -v
```
Expected: PASS on a machine with `bd` + `opencode` + `git`; SKIP loudly otherwise.

- [ ] **Step 4: Confirm default suite unaffected**

```bash
go test ./... 2>&1 | tail -2
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "test(integration): Passo B parallel epic + resume + cross-agent-after-crash"
```

**Acceptance Criteria:**
- [ ] `go test -tags=integration -run 'TestPassoB|TestResume'` PASSES with the tools present, SKIPS loudly without them.
- [ ] Passo B asserts: epic completes, `Parallelism().Peak >= 2`, all children terminal.
- [ ] The resume test asserts: done beads skipped, interrupted bead resumed, epic completes, cross-agent exclusion honored after a gap crash.
- [ ] `go test ./...` (no tag) pass count unchanged.

---

## Phase 6 — Magical moment & docs finalization

### Task 37: Packaged example, GIF, and docs finalization

**Bead Mapping:**
- type: `epic`
- Priority: `4`
- Estimated Minutes: `0`
- Dependencies: `Task 30`, `Task 33`
- Parent: `none`
- Status: `open`

> Epic container for Phase 6. Children: Tasks 38–39.

---

### Task 38: `examples/parallel-demo/` — packaged epic

**Bead Mapping:**
- type: `task`
- Priority: `4`
- Estimated Minutes: `18`
- Dependencies: `Task 29`
- Parent: `Task 37`
- Status: `open`

**Files:**
- Create: `/home/gabriel/repositories/kernl/examples/parallel-demo/.beads/issues.jsonl`
- Create: `/home/gabriel/repositories/kernl/examples/parallel-demo/README.md`

**Description / Steps:**
- [ ] **Step 1: Build the packaged epic**

`examples/parallel-demo/.beads/issues.jsonl` — a parent `epic` bead + 2–3 child `task` beads with dependencies such that ≥2 run in parallel (e.g. `a → {b, c}`). The example's README states it spends real `opencode` tokens/time and the command: `kernl epic run examples/parallel-demo`.

- [ ] **Step 2: Verify it runs end-to-end**

```bash
cd /home/gabriel/repositories/kernl
go run ./orchestrator/cmd/kernl epic run examples/parallel-demo
```
Expected: epic completes, GUI URL printed, ≥2 beads run concurrently.

- [ ] **Step 3: Commit**

```bash
git add examples/ && git commit -m "feat(examples): parallel-demo — packaged epic for the magical moment"
```

**Acceptance Criteria:**
- [ ] `kernl epic run examples/parallel-demo` runs the packaged epic to completion with observable parallelism.
- [ ] `examples/parallel-demo/README.md` names the command and the token/time cost.

---

### Task 39: GIF + README/glossary finalization

**Bead Mapping:**
- type: `task`
- Priority: `4`
- Estimated Minutes: `20`
- Dependencies: `Task 38`, `Task 32`
- Parent: `Task 37`
- Status: `open`

**Files:**
- Create: `/home/gabriel/repositories/kernl/docs/assets/parallel-demo.gif`
- Modify: `/home/gabriel/repositories/kernl/README.md`
- Modify: `/home/gabriel/repositories/kernl/docs/glossary.md`

**Description / Steps:**
- [ ] **Step 1: Record the GIF/asciinema**

Record `kernl epic run examples/parallel-demo` with the GUI showing beads changing state in parallel. Save to `docs/assets/parallel-demo.gif`.

- [ ] **Step 2: Finalize the README**

Replace the `<!-- GIF -->` marker with the recorded GIF at the top. Ensure the quickstart, prerequisites, monitoring section, and `examples/parallel-demo` command are all coherent end-to-end. Add the open-source framing line from the STRATEGY ("projetado em blocos").

- [ ] **Step 3: Finalize the glossary**

Sweep `docs/glossary.md` for any term introduced across Phases 0–5 that is still undefined (`run-state`, `worktree`, `realized parallelism`, `fail-fast`).

- [ ] **Step 4: Commit**

```bash
git add docs/assets README.md docs/glossary.md && git commit -m "docs: parallel-demo GIF + README/glossary finalization"
```

**Acceptance Criteria:**
- [ ] `README.md` has the GIF at the top and a coherent end-to-end quickstart.
- [ ] `docs/glossary.md` has no term referenced in the README/plan left undefined.

---

## Execution Order & Parallelization

From the eng review's worktree parallelization strategy:

```
Phase 0 (Tasks 2-7)  ── sequential gate, everything depends on the rename
        │
        ├─► Phase 1 (Tasks 9-10)  ── de-risking, can start immediately after Phase 0
        │
        ├─► Lane B: Phase 2 (Tasks 12-22)  ── app/api/cmd — independent of git/worktree
        │
        └─► Lane C: Task 24 (worktree)     ── independent of Phase 2
                  │
        (Lane B + Lane C) ─► Phase 3 rest (Tasks 25-29)
                  │
                  ├─► Phase 4 (Tasks 31-32)  ─┐ parallel — different packages
                  └─► Phase 5 (Tasks 34-36)  ─┘ (coordinate the shared epic/executor.go Emit hook)
                                  │
                                  └─► Phase 6 (Tasks 38-39)
```

**Conflict flag (from eng review):** Tasks 28a, 28b, 31, and 35 all touch `orchestrator/internal/epic/`. Phase 4 and Phase 5 in parallel risk a merge conflict in that package — Task 28b adds the `Emit` binding (consumed by Task 31), Task 35 adds `resume.go`; keep them in separate files (`executor.go`, `wave.go`, `resume.go`) and the conflict surface is minimal, but sequence 31→35 if it bites.

---

## Notes

**Yegge Loop deltas tracked here.**

- _Iteration 1 (draft):_ Initial plan written from STRATEGY + eng review + test plan + DevEx review. 39 tasks across 7 epics (Phases 0–6). Open parameter flagged: `<user>` in the module path. One finding beyond the eng review's framing surfaced and folded into Task 15: no agent-process spawner exists in `foolery-go` (`exec.Command` only in `bdcli.go`/`knots.go`), so the SessionDriver is fully greenfield, not thin wiring.
- _Iteration 2 (self-critique):_ **3 structural Bead Mapping fixes applied.** Task 20 (`kernl epic list`) was missing `Dependencies` and `Parent` — added `Dependencies: Task 19`, `Parent: Task 11` (P3, 18 min). Task 21 (README Phase 2) was missing `Dependencies` and `Parent` — added `Dependencies: Task 20`, `Parent: Task 11` (P3, 14 min). No contradictions against STRATEGY found. Edge-case gaps: two minor (process death between turn_ended/take_iteration_close, zero-bead epic) — both low-impact, not blocking. All 39 tasks now have complete Bead Mapping blocks; dependency graph remains acyclic; priorities consistent with dependency order.

---

## Self-Review

**1. Spec coverage.** STRATEGY tracks → Motor de execução paralela (Tasks 24–29, 34–36), Observabilidade (Tasks 31–32), Loop de gates (plan approval is upstream; the in-MVP gate is the `blocked` state — Tasks 28–29, 35). Eng review Issues 1–10 → Issue 1/2 (one binary, package invariant): all packages under `orchestrator/internal`, only `worktree`/`epic` git-aware; Issue 3 (resume, SQLite): Tasks 34–35; Issue 4 (epic-SSE): Task 31; Issue 5 (worktree location): Task 24; Issue 6 (rename gate): Tasks 3–7; Issue 7 (fail-fast): Task 28; Issue 8 (single nudge builder): Task 15; Issue 9 (integration suite codified): Tasks 10, 22, 36; Issue 10 (concurrency semaphore): Task 28. DevEx F1–F6 → F1 doctor (Task 14, 18), F2 rich yaml + actionable error (Task 13), F3 README cross-cutting (Tasks 21, 32, 39), F4 `epic list` (Task 20), F5 blocked next-step (Task 29), F6 plain-text output (Tasks 18–19, 29); beat→bead (Task 4); LICENSE (Task 6); glossary (Tasks 21, 32, 39); embedded GUI URL (Task 29). Test plan critical paths → Passo A (Task 22), Passo B (Task 36), resume + cross-agent-after-crash (Task 36), GUI (Task 32). **Gap check:** the eng review's "Fase 1 Passo A" is realized in Phase 2 (Task 22) by dependency necessity — noted explicitly in Task 8. No spec requirement left without a task.

**2. Placeholder scan.** One intentional, flagged parameter: `<user>` in the module path (called out in the header and Task 3, to be resolved at Yegge iteration 1). No "TBD" / "add error handling" / "similar to Task N" — every code step carries real Go or real commands.

**3. Type consistency.** Verified against the codebase: `backend.Bead` / `backend.BeadDependency` / `backend.BeadListFilters` (post-Task-4 rename), `backend.BackendPort`, `session.SessionConnectionManager` / `ServeSSE` / `NewSessionRuntimeWithCapabilities`, `terminal.NewTerminalManager` / `WithMaxSessions` / `HandleTakeLoopTurnEnded` / `HandleTakeIterationClose`, `adapter.ResolveDialect`, `config.Load` / `config.Config`. New types are introduced once and referenced consistently: `app.App`, `app.SessionDriver` / `RunBead` / `RunBeadInput` / `RunBeadResult`, `app.BuildNudgePrompt` / `NudgeCause`, `epic.DAG` / `Node` / `ReadySet`, `epic.Epic` / `LoadEpic`, `epic.EpicExecutor` / `NewExecutor` / `EpicEvent`, `epic.ParallelismTracker`, `epic.PlanResume` / `ResumePlan`, `worktree.Manager` / `Add`, `runstate.Store` / `Open`, `preflight.Run` / `Report`.

**4. Bead structural validity.** Every `### Task` has a complete `**Bead Mapping:**` block. 7 epics (Tasks 1, 8, 11, 23, 30, 33, 37), the rest leaf tasks/stories/chores/spike. All `Parent` references point to epics. All `Dependencies` resolve to existing task headings. Dependency graph is acyclic (phases form a DAG; cross-phase deps only ever point backward in number). Priorities agree with dependencies: epics 0→1→1→2→3→3→4; within each epic children carry priority ≥ their epic and ≥ their dependencies' priority (e.g. Task 22 P3 depends on Task 19 P2; Tasks 28a/28b P3 depend on P2 tasks; Task 35 P4 depends on Task 34 P3). Acceptance criteria are command- or test-verifiable throughout. *Fixed in Iteration 2:* Tasks 20 and 21 were missing `Dependencies` and `Parent`; corrections applied, structural validity confirmed.
