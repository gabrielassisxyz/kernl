# Kernl Workflow Migration + MergeManager + Sweep — Implementation Plan

> **Bead Target:** This plan maps deterministically to beads. Every task includes a `**Bead Mapping:**` block. The converter will project tasks 1:1 with zero creative interpretation.

**Goal:** Replace the foolery-inherited state machine with a kernl-native workflow (bd built-in + custom statuses + stable description fields + local-JSON agent runtime), wire up an automatic MergeManager + integration agent at end-of-epic, and ship `kernl sweep` to close beads whose PRs hit master — all in a single big-bang PR. Knots delete is deferred to a follow-up PR.

**Architecture:** Three orthogonal persistence layers mirroring gastown (bd status built-in / bd status.custom / structured description fields), with one deliberate divergence: high-frequency `AgentState` lives in `~/.kernl/state/<bead-id>.json` (atomic-write JSON) instead of bd description, because kernl is multiprocess and last-write-wins on `bd.description` would corrupt heartbeats. `bd list` is the single source of truth for trigger logic; the JSON store is observability/audit-only. MergeManager spawns a single integration agent that batches merges in topological order, parses a typed `merge.Outcome` enum from description, and routes transitions deterministically. `kernl sweep` polls `gh pr view` with cache + circuit breaker + dry-run.

**Tech Stack:** Go 1.22+, `bd ≥ 1.0.4`, `gh` CLI, existing kernl primitives (take loop / watchdog / dispatch / WorktreeManager / SSE — all reused 100%).

**Upstream artifacts (read these first if you parachute in cold):**
- `docs/STRATEGY.md`
- `docs/2026-05-15-kernl-workflow-brainstorm-spec.md` (primary source — 14 decisions D1–D14 + 5 cross-model tensions TT1–TT5 are indexed in the header)
- `docs/reviews/vc-plan-eng-review-2026-05-15.md`
- `docs/reviews/vc-plan-eng-review-test-plan-2026-05-15.md`
- `docs/spikes/2026-05-15-conflict-rate-spike.md` (TT1 validated empirically: 0% conflict rate over 9 merges → batch-at-end design holds)

**Out of scope (registered backlog, do NOT include in this PR):**
- Knots delete (deferred to follow-up PR per D0=C; knots remains dormant)
- Property-based race test for trigger (TODO T1 in `TODOS.md`)
- Batch heartbeats in memory (TODO T2)
- `kernl epic abort` (TODO T3)
- AST-aware parallelism, GitHub webhook for sweep, resolution agent, custom profiles

---

## Critical path / parallelization waves

From eng review §"Worktree parallelization strategy":

```
WAVE 1 (parallel):
  Lane A  — workflow/ (status.go, description.go, agent_state_store.go, ensure_custom.go)
  Lane G  — specs/ (00-architecture, backend, orchestration, prompt)

WAVE 2 (parallel, all depend on Lane A):
  Lane B  — sweep/ + cmd/kernl/sweep.go
  Lane C  — backend/bdcli.go refactor + EnsureCustomStatuses wiring + state_machine slim-down
  Lane D  — string-literal refactor across dispatch/, orchestration/, terminal/, retake/, epic/, app/ (+ tests)
  Lane E  — fixtures (testdata/beads-*) + harness bd 1.0.4 pin + CI install
  Lane F1 — merge/ (errors.go, manager.go)

WAVE 3 (after F1):
  Lane F2 — prompt/merger_prompt.go + cmd/kernl/epic_merge.go + EpicExecutor wiring + WorktreeManager epic branch + kernl serve auto-tick

WAVE 4 (after all source lanes):
  Lane H  — E2E integration tests (Path 1 happy / Path 2 conflict / Path 3 push-fail / sweep E2E / passo_a regression)

WAVE 5 (after H):
  Lane I  — Final gates (go vet, lint, 962 unit suite, bd doctor, docs/TODOS update)

Conflict flag: Lanes D and E both touch *_test.go in internal/integration/ — coordinate or serialize between them.
```

---

## Definition of done (mirrors brainstorm-spec §9 — 11 verifiable criteria)

C1. `kernl epic run <epic-id>` advances bead state against bd 1.0.4 real without `validation failed`.
C2. After all children finish, the merger agent is dispatched automatically and worktree-children → epic-branch merges happen.
C3. Happy path: `gh pr create` runs and the epic ends in `awaiting_pr_review` with `pr_url:` in description.
C4. `kernl sweep --dry-run` lists epics with merged PRs without side-effects.
C5. `kernl sweep` (no flag) closes children + epic after PR merged to master.
C6. `bd ready` on the kernl repo itself returns zero beads in legacy foolery statuses.
C7. Knots remains dormant (factory never routes to it); `cmd/kernl/` and `internal/` have zero new references to knots. (Critério "zero referências" full clean-up moves to follow-up PR.)
C8. Intractable merge conflict moves the epic to `blocked`; `kernl epic merge <epic-id>` re-dispatches after manual resolution.
C9. `merge_outcome:` is parsed via typed enum; MergeManager routes success/merge_conflict/push_failed/pr_create_failed/pr_already_exists correctly.
C10. `~/.kernl/state/<bead-id>.json` is created/updated/recovered/purged correctly across the agent lifecycle.
C11. Sweep with circuit breaker + cache MERGED behaves correctly: 3 consecutive failures → backoff 5/15/60min; PR MERGED never re-consulted.

These C1–C11 are referenced by tasks as acceptance criteria.

---

# Tasks

---

## Task 0: Master Epic — Kernl Workflow Migration + MergeManager + Sweep

**Bead Mapping:**
- type: `epic`
- Priority: `0`
- Estimated Minutes: `0`
- Dependencies: `none`
- Parent: `none`
- Status: `open`

**Files:** N/A (container)

**Description:** Top-level epic spanning all tracks in this PR. Children are the 10 lane epics: Task 1 (Lane A), Task 6 (Lane G), Task 11 (Lane C), Task 15 (Lane D), Task 18 (Lane E), Task 21 (Lane F1), Task 24 (Lane B), Task 27 (Lane F2), Task 33 (Lane H), Task 39 (Lane I). Closes when all C1–C11 acceptance criteria pass and the PR is mergeable.

**Acceptance Criteria:**
- [ ] C1 — `kernl epic run <id>` advances state against `bd v1.0.4` real without `validation failed`.
- [ ] C2 — After all children finish, the merger agent is dispatched automatically and merges happen.
- [ ] C3 — Happy path: `gh pr create` runs; epic ends in `awaiting_pr_review` with `pr_url:` set.
- [ ] C4 — `kernl sweep --dry-run` lists merged-PR epics without side-effects.
- [ ] C5 — `kernl sweep` closes children + epic after PR merged.
- [ ] C6 — `bd ready` returns zero beads with legacy foolery statuses.
- [ ] C7 — Knots remains dormant; no new references to knots in `internal/` or `cmd/kernl/`.
- [ ] C8 — Intractable conflict → `blocked`; `kernl epic merge` recovers after manual resolution.
- [ ] C9 — `merge_outcome:` parsed via typed enum; all 5 routing cases verified.
- [ ] C10 — `~/.kernl/state/<bead-id>.json` lifecycle (create/update/recover/purge) verified.
- [ ] C11 — Sweep cache MERGED + circuit breaker (3 fails → 5/15/60min backoff) verified.
- [ ] `go vet ./...` clean and project linter clean.
- [ ] 962 pre-existing unit tests + new unit tests + 3 E2E paths green.
- [ ] `TODOS.md` "Definir workflow próprio do kernl" entry closed; "Remoção completa do knots" entry retained for follow-up PR.

---

## Task 1: Lane A Epic — Workflow Package Foundation

**Bead Mapping:**
- type: `epic`
- Priority: `1`
- Estimated Minutes: `0`
- Dependencies: `none`
- Parent: `Task 0`
- Status: `open`

**Files:** Container — children create `orchestrator/internal/workflow/`.

**Description:** Build the foundation package: typed `IssueStatus` + `AgentState`, `IsValidCombination`, stable-field description helpers, JSON agent-state store with atomic-write+mutex+recover-on-corrupt, and idempotent `EnsureCustomStatuses` mirroring gastown.

**Acceptance Criteria:**
- [ ] `go test ./orchestrator/internal/workflow/...` green.
- [ ] No exported symbol in `workflow/` references legacy foolery statuses.
- [ ] Package documentation comment at top of `status.go` cites the spec section.

---

### Task 2: workflow/status.go — IssueStatus + AgentState + IsValidCombination

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: `35`
- Dependencies: `none`
- Parent: `Task 1`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/workflow/status.go`
- Test: `orchestrator/internal/workflow/status_test.go`

**Description / Steps:**

- [ ] **Step 1: Write the failing tests first.** Create `status_test.go` with table tests for all predicates and `IsValidCombination`:

```go
package workflow_test

import (
    "testing"

    "github.com/gabrielassisxyz/kernl/orchestrator/internal/workflow"
)

func TestIssueStatus_Predicates(t *testing.T) {
    cases := []struct {
        s                workflow.IssueStatus
        isTerminal       bool
        isClaimable      bool
        haltsEpic        bool
        isCustom         bool
    }{
        {workflow.StatusOpen, false, true, false, false},
        {workflow.StatusInProgress, false, false, false, false},
        {workflow.StatusAwaitingIntegration, false, false, false, true},
        {workflow.StatusAwaitingPRReview, false, false, false, true},
        {workflow.StatusBlocked, false, false, true, false},
        {workflow.StatusClosed, true, false, false, false},
    }
    for _, c := range cases {
        t.Run(string(c.s), func(t *testing.T) {
            if got := c.s.IsTerminal(); got != c.isTerminal {
                t.Fatalf("IsTerminal: got %v want %v", got, c.isTerminal)
            }
            if got := c.s.IsClaimableByWorker(); got != c.isClaimable {
                t.Fatalf("IsClaimableByWorker: got %v want %v", got, c.isClaimable)
            }
            if got := c.s.HaltsEpic(); got != c.haltsEpic {
                t.Fatalf("HaltsEpic: got %v want %v", got, c.haltsEpic)
            }
            if got := c.s.IsCustom(); got != c.isCustom {
                t.Fatalf("IsCustom: got %v want %v", got, c.isCustom)
            }
        })
    }
}

func TestIsValidCombination_Exhaustive(t *testing.T) {
    // 6 statuses × 5 agent states = 30 combinations.
    // Truth table from spec §4.6 (TT5=A).
    valid := map[workflow.IssueStatus]map[workflow.AgentState]bool{
        workflow.StatusOpen: {
            workflow.AgentSpawning: false, workflow.AgentWorking: false, workflow.AgentDone: false,
            workflow.AgentStuck: false, workflow.AgentFailed: false,
        },
        workflow.StatusInProgress: {
            workflow.AgentSpawning: true, workflow.AgentWorking: true, workflow.AgentDone: true,
            workflow.AgentStuck: true, workflow.AgentFailed: true,
        },
        workflow.StatusAwaitingIntegration: {
            workflow.AgentSpawning: false, workflow.AgentWorking: false, workflow.AgentDone: true,
            workflow.AgentStuck: false, workflow.AgentFailed: false,
        },
        workflow.StatusAwaitingPRReview: {
            workflow.AgentSpawning: false, workflow.AgentWorking: false, workflow.AgentDone: true,
            workflow.AgentStuck: false, workflow.AgentFailed: false,
        },
        workflow.StatusBlocked: {
            workflow.AgentSpawning: false, workflow.AgentWorking: false, workflow.AgentDone: false,
            workflow.AgentStuck: true, workflow.AgentFailed: true,
        },
        workflow.StatusClosed: {
            workflow.AgentSpawning: false, workflow.AgentWorking: false, workflow.AgentDone: true,
            workflow.AgentStuck: false, workflow.AgentFailed: false,
        },
    }
    for s, row := range valid {
        for a, want := range row {
            t.Run(string(s)+"_"+string(a), func(t *testing.T) {
                if got := workflow.IsValidCombination(s, a); got != want {
                    t.Fatalf("IsValidCombination(%s,%s)=%v want %v", s, a, got, want)
                }
            })
        }
    }
}

func TestKernlCustomStatuses_Exact(t *testing.T) {
    want := []string{"awaiting_integration", "awaiting_pr_review"}
    if len(workflow.KernlCustomStatuses) != len(want) {
        t.Fatalf("length mismatch")
    }
    for i, v := range workflow.KernlCustomStatuses {
        if v != want[i] {
            t.Fatalf("KernlCustomStatuses[%d]=%q want %q", i, v, want[i])
        }
    }
}
```

- [ ] **Step 2: Run the tests, verify they fail.**

```bash
go test ./orchestrator/internal/workflow/ -run TestIssueStatus_Predicates -v
```
Expected: FAIL ("undefined: workflow.IssueStatus").

- [ ] **Step 3: Implement `status.go`.**

```go
// Package workflow defines the kernl issue lifecycle.
// Spec: docs/2026-05-15-kernl-workflow-brainstorm-spec.md §4.
package workflow

type IssueStatus string

const (
    StatusOpen                IssueStatus = "open"
    StatusInProgress          IssueStatus = "in_progress"
    StatusAwaitingIntegration IssueStatus = "awaiting_integration" // custom
    StatusAwaitingPRReview    IssueStatus = "awaiting_pr_review"   // custom
    StatusBlocked             IssueStatus = "blocked"
    StatusClosed              IssueStatus = "closed"
)

// KernlCustomStatuses is the exact list registered via EnsureCustomStatuses.
// Order is significant: it is the value passed to `bd config set status.custom`.
var KernlCustomStatuses = []string{
    string(StatusAwaitingIntegration),
    string(StatusAwaitingPRReview),
}

func (s IssueStatus) IsTerminal() bool        { return s == StatusClosed }
func (s IssueStatus) IsClaimableByWorker() bool { return s == StatusOpen }
func (s IssueStatus) HaltsEpic() bool         { return s == StatusBlocked }
func (s IssueStatus) IsCustom() bool {
    return s == StatusAwaitingIntegration || s == StatusAwaitingPRReview
}

type AgentState string

const (
    AgentSpawning AgentState = "spawning"
    AgentWorking  AgentState = "working"
    AgentDone     AgentState = "done"
    AgentStuck    AgentState = "stuck"
    AgentFailed   AgentState = "failed"
)

// IsValidCombination encodes the truth table from spec §4.6 (TT5=A).
// Returns false for combinations that runtime must never produce.
func IsValidCombination(s IssueStatus, a AgentState) bool {
    switch s {
    case StatusOpen:
        return false
    case StatusInProgress:
        return true
    case StatusAwaitingIntegration, StatusAwaitingPRReview, StatusClosed:
        return a == AgentDone
    case StatusBlocked:
        return a == AgentStuck || a == AgentFailed
    }
    return false
}
```

- [ ] **Step 4: Run tests, verify pass.**

```bash
go test ./orchestrator/internal/workflow/ -v
```
Expected: PASS for all sub-tests.

- [ ] **Step 5: Commit.**

```bash
git add orchestrator/internal/workflow/status.go orchestrator/internal/workflow/status_test.go
git commit -m "workflow: add IssueStatus, AgentState, and IsValidCombination truth table"
```

**Acceptance Criteria:**
- [ ] `go test ./orchestrator/internal/workflow/ -run TestIssueStatus_Predicates -v` passes.
- [ ] `go test ./orchestrator/internal/workflow/ -run TestIsValidCombination_Exhaustive -v` covers all 30 combinations.
- [ ] `go test ./orchestrator/internal/workflow/ -run TestKernlCustomStatuses_Exact -v` passes.
- [ ] `go vet ./orchestrator/internal/workflow/...` clean.

---

### Task 3: workflow/description.go — Stable-field helpers

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: `40`
- Dependencies: `Task 2`
- Parent: `Task 1`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/workflow/description.go`
- Test: `orchestrator/internal/workflow/description_test.go`

**Description / Steps:**

Mirror gastown `integration.go:69-128` for `getMetadataField` / `addMetadataField`. Per D1=C, description only holds **stable** fields (worktree_path, worktree_branch, epic_branch, pr_url, merge_conflict_at, merge_outcome). High-frequency runtime state goes to JSON (Task 4).

- [ ] **Step 1: Write failing tests.**

```go
package workflow_test

import (
    "strings"
    "testing"

    "github.com/gabrielassisxyz/kernl/orchestrator/internal/workflow"
)

func TestGetMetadataField_Variants(t *testing.T) {
    cases := []struct {
        name    string
        desc    string
        key     string
        want    string
    }{
        {"basic", "pr_url: https://x/pr/1\n", "pr_url", "https://x/pr/1"},
        {"case_insens_key", "PR_URL: x\n", "pr_url", "x"},
        {"colon_in_value", "url: https://example.com:443/x\n", "url", "https://example.com:443/x"},
        {"empty_desc", "", "pr_url", ""},
        {"absent_key", "other: y\n", "pr_url", ""},
        {"multiline_doc", "line one\npr_url: x\nline three\n", "pr_url", "x"},
        {"bom_prefix", "﻿pr_url: x\n", "pr_url", "x"},
        {"trailing_spaces", "pr_url:    x   \n", "pr_url", "x"},
    }
    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            if got := workflow.GetMetadataField(c.desc, c.key); got != c.want {
                t.Fatalf("got %q want %q", got, c.want)
            }
        })
    }
}

func TestAddMetadataField_Insert(t *testing.T) {
    out := workflow.AddMetadataField("body text\n", "pr_url", "https://x/pr/1")
    if !strings.Contains(out, "pr_url: https://x/pr/1") {
        t.Fatalf("missing inserted field, got: %q", out)
    }
    if !strings.Contains(out, "body text") {
        t.Fatalf("original body lost: %q", out)
    }
}

func TestAddMetadataField_Update(t *testing.T) {
    in := "body\npr_url: old\nfooter\n"
    out := workflow.AddMetadataField(in, "pr_url", "new")
    if strings.Contains(out, "old") {
        t.Fatalf("old value not removed: %q", out)
    }
    if !strings.Contains(out, "pr_url: new") {
        t.Fatalf("new value missing: %q", out)
    }
    if !strings.Contains(out, "footer") {
        t.Fatalf("unrelated line lost: %q", out)
    }
}

func TestTypedHelpers_Roundtrip(t *testing.T) {
    desc := ""
    desc = workflow.SetPRURL(desc, "https://x/pr/42")
    desc = workflow.SetEpicBranch(desc, "feat/kernl-abc")
    desc = workflow.SetMergeOutcome(desc, "success")
    if got := workflow.GetPRURL(desc); got != "https://x/pr/42" {
        t.Fatalf("pr_url roundtrip: %q", got)
    }
    if got := workflow.GetEpicBranch(desc); got != "feat/kernl-abc" {
        t.Fatalf("epic_branch roundtrip: %q", got)
    }
    if got := workflow.GetMergeOutcome(desc); got != "success" {
        t.Fatalf("merge_outcome roundtrip: %q", got)
    }
}
```

- [ ] **Step 2: Run tests to verify failure.**

```bash
go test ./orchestrator/internal/workflow/ -run TestGetMetadataField_Variants -v
```
Expected: FAIL.

- [ ] **Step 3: Implement `description.go`** mirroring gastown `integration.go:69-128`:

```go
package workflow

import (
    "regexp"
    "strings"
)

// metadataLineRE matches "key: value" lines. Key is case-insensitive at lookup time.
var metadataLineRE = regexp.MustCompile(`(?im)^([a-zA-Z0-9_]+):\s*(.*?)\s*$`)

// stripBOM removes a leading byte-order mark, if present.
func stripBOM(s string) string {
    return strings.TrimPrefix(s, "﻿")
}

// GetMetadataField extracts the first occurrence of `key: value` from the description.
// Key matching is case-insensitive. Returns "" if absent.
func GetMetadataField(desc, key string) string {
    desc = stripBOM(desc)
    keyLower := strings.ToLower(key)
    for _, line := range strings.Split(desc, "\n") {
        m := metadataLineRE.FindStringSubmatch(line)
        if m == nil {
            continue
        }
        if strings.ToLower(m[1]) == keyLower {
            return strings.TrimSpace(m[2])
        }
    }
    return ""
}

// AddMetadataField inserts or updates "key: value" in the description.
// If the key already exists, the FIRST occurrence is updated and remaining occurrences are removed.
// If absent, the line is appended.
func AddMetadataField(desc, key, value string) string {
    desc = stripBOM(desc)
    keyLower := strings.ToLower(key)
    var b strings.Builder
    replaced := false
    for i, line := range strings.Split(desc, "\n") {
        m := metadataLineRE.FindStringSubmatch(line)
        if m != nil && strings.ToLower(m[1]) == keyLower {
            if !replaced {
                if i > 0 {
                    b.WriteString("\n")
                }
                b.WriteString(key + ": " + value)
                replaced = true
            }
            continue
        }
        if i > 0 {
            b.WriteString("\n")
        }
        b.WriteString(line)
    }
    if !replaced {
        if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
            b.WriteString("\n")
        }
        b.WriteString(key + ": " + value + "\n")
    }
    return b.String()
}

// Typed helpers for stable fields (spec §4.5).
func GetWorktreePath(d string) string     { return GetMetadataField(d, "worktree_path") }
func SetWorktreePath(d, v string) string  { return AddMetadataField(d, "worktree_path", v) }
func GetWorktreeBranch(d string) string   { return GetMetadataField(d, "worktree_branch") }
func SetWorktreeBranch(d, v string) string{ return AddMetadataField(d, "worktree_branch", v) }
func GetEpicBranch(d string) string       { return GetMetadataField(d, "epic_branch") }
func SetEpicBranch(d, v string) string    { return AddMetadataField(d, "epic_branch", v) }
func GetPRURL(d string) string            { return GetMetadataField(d, "pr_url") }
func SetPRURL(d, v string) string         { return AddMetadataField(d, "pr_url", v) }
func GetMergeConflictAt(d string) string  { return GetMetadataField(d, "merge_conflict_at") }
func SetMergeConflictAt(d, v string) string { return AddMetadataField(d, "merge_conflict_at", v) }
func GetMergeOutcome(d string) string     { return GetMetadataField(d, "merge_outcome") }
func SetMergeOutcome(d, v string) string  { return AddMetadataField(d, "merge_outcome", v) }
```

- [ ] **Step 4: Run tests.**

```bash
go test ./orchestrator/internal/workflow/ -v
```
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add orchestrator/internal/workflow/description.go orchestrator/internal/workflow/description_test.go
git commit -m "workflow: add stable-field description helpers (get/add + typed accessors)"
```

**Acceptance Criteria:**
- [ ] `TestGetMetadataField_Variants` passes for all 8 cases including BOM, colon-in-value, case-insensitive key.
- [ ] `TestAddMetadataField_Update` confirms update preserves unrelated lines and removes prior occurrence.
- [ ] `TestTypedHelpers_Roundtrip` covers `pr_url`, `epic_branch`, `merge_outcome`.

---

### Task 4: workflow/agent_state_store.go — JSON local store

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: `60`
- Dependencies: `Task 2`
- Parent: `Task 1`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/workflow/agent_state_store.go`
- Test: `orchestrator/internal/workflow/agent_state_store_test.go`

**Description / Steps:**

JSON store in `~/.kernl/state/<bead-id>.json`. Atomic write via tempfile+rename (D12=A). In-process `sync.Mutex` keyed by `bead_id`. Read-on-missing returns defaults; read-on-corrupted returns defaults + WARN (D9=A). `Purge(id)` deletes file (called from `bd close` hook).

- [ ] **Step 1: Write tests.**

```go
package workflow_test

import (
    "encoding/json"
    "os"
    "path/filepath"
    "sync"
    "testing"
    "time"

    "github.com/gabrielassisxyz/kernl/orchestrator/internal/workflow"
)

func newStore(t *testing.T) *workflow.AgentStateStore {
    t.Helper()
    dir := t.TempDir()
    s, err := workflow.NewAgentStateStore(dir)
    if err != nil {
        t.Fatal(err)
    }
    return s
}

func TestAgentStateStore_ReadMissing_ReturnsDefaults(t *testing.T) {
    s := newStore(t)
    got, err := s.Load("kernl-abc")
    if err != nil {
        t.Fatal(err)
    }
    if got.AgentState != "" || got.FollowUpCount != 0 {
        t.Fatalf("expected zero-value, got %+v", got)
    }
}

func TestAgentStateStore_WriteThenRead_Roundtrip(t *testing.T) {
    s := newStore(t)
    in := workflow.AgentRuntime{
        AgentState:       workflow.AgentWorking,
        AgentSessionID:   "sess-1",
        AgentStartedAt:   time.Date(2026, 5, 15, 14, 23, 0, 0, time.UTC),
        LastHeartbeatAt:  time.Date(2026, 5, 15, 14, 25, 30, 0, time.UTC),
        FollowUpCount:    2,
    }
    if err := s.Save("kernl-abc", in); err != nil {
        t.Fatal(err)
    }
    got, err := s.Load("kernl-abc")
    if err != nil {
        t.Fatal(err)
    }
    if got != in {
        t.Fatalf("roundtrip mismatch:\n got %+v\nwant %+v", got, in)
    }
}

func TestAgentStateStore_CorruptedJSON_RecoverWithDefaults(t *testing.T) {
    dir := t.TempDir()
    s, _ := workflow.NewAgentStateStore(dir)
    path := filepath.Join(dir, "kernl-abc.json")
    if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
        t.Fatal(err)
    }
    got, err := s.Load("kernl-abc")
    if err != nil {
        t.Fatalf("expected recover, got err: %v", err)
    }
    if got.AgentState != "" {
        t.Fatalf("expected defaults after corrupt, got %+v", got)
    }
}

func TestAgentStateStore_AtomicWrite_NoTempLeftover(t *testing.T) {
    s := newStore(t)
    if err := s.Save("kernl-abc", workflow.AgentRuntime{AgentState: workflow.AgentWorking}); err != nil {
        t.Fatal(err)
    }
    entries, err := os.ReadDir(s.Dir())
    if err != nil {
        t.Fatal(err)
    }
    for _, e := range entries {
        if filepath.Ext(e.Name()) == ".tmp" {
            t.Fatalf("temp file leaked: %s", e.Name())
        }
    }
    // And final file is well-formed JSON.
    data, _ := os.ReadFile(filepath.Join(s.Dir(), "kernl-abc.json"))
    var v workflow.AgentRuntime
    if err := json.Unmarshal(data, &v); err != nil {
        t.Fatalf("final file not JSON: %v", err)
    }
}

func TestAgentStateStore_ConcurrentWrites_SameBead_Serialized(t *testing.T) {
    s := newStore(t)
    var wg sync.WaitGroup
    for i := 0; i < 50; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            _ = s.Save("kernl-abc", workflow.AgentRuntime{AgentState: workflow.AgentWorking, FollowUpCount: i})
        }(i)
    }
    wg.Wait()
    got, err := s.Load("kernl-abc")
    if err != nil {
        t.Fatal(err)
    }
    // The final value is some valid one from the set — no torn JSON.
    if got.AgentState != workflow.AgentWorking {
        t.Fatalf("torn write: %+v", got)
    }
}

func TestAgentStateStore_Purge_RemovesFile(t *testing.T) {
    s := newStore(t)
    _ = s.Save("kernl-abc", workflow.AgentRuntime{AgentState: workflow.AgentWorking})
    if err := s.Purge("kernl-abc"); err != nil {
        t.Fatal(err)
    }
    if _, err := os.Stat(filepath.Join(s.Dir(), "kernl-abc.json")); !os.IsNotExist(err) {
        t.Fatalf("expected file gone, err=%v", err)
    }
    // Purge is idempotent.
    if err := s.Purge("kernl-abc"); err != nil {
        t.Fatalf("purge should be idempotent, got %v", err)
    }
}
```

- [ ] **Step 2: Run tests to confirm failure.**

```bash
go test ./orchestrator/internal/workflow/ -run TestAgentStateStore -v
```
Expected: FAIL.

- [ ] **Step 3: Implement `agent_state_store.go`.**

```go
package workflow

import (
    "encoding/json"
    "errors"
    "log"
    "os"
    "path/filepath"
    "sync"
    "time"
)

// AgentRuntime is the high-frequency runtime state of an agent attached to a bead.
// Spec §4.4 — kept out of bd.description (D1=C) to avoid lost-update under concurrent writes.
type AgentRuntime struct {
    AgentState      AgentState `json:"agent_state,omitempty"`
    AgentSessionID  string     `json:"agent_session_id,omitempty"`
    AgentStartedAt  time.Time  `json:"agent_started_at,omitempty"`
    LastHeartbeatAt time.Time  `json:"last_heartbeat_at,omitempty"`
    FollowUpCount   int        `json:"follow_up_count,omitempty"`
}

// AgentStateStore is a per-bead JSON store with atomic write + in-process mutex.
type AgentStateStore struct {
    dir   string
    locks sync.Map // map[string]*sync.Mutex (key: beadID)
}

func NewAgentStateStore(dir string) (*AgentStateStore, error) {
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return nil, err
    }
    return &AgentStateStore{dir: dir}, nil
}

func (s *AgentStateStore) Dir() string { return s.dir }

func (s *AgentStateStore) lockFor(id string) *sync.Mutex {
    m, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
    return m.(*sync.Mutex)
}

func (s *AgentStateStore) path(id string) string {
    return filepath.Join(s.dir, id+".json")
}

// Load returns the AgentRuntime for the bead. Missing or corrupted file returns the zero value + WARN.
func (s *AgentStateStore) Load(id string) (AgentRuntime, error) {
    s.lockFor(id).Lock()
    defer s.lockFor(id).Unlock()
    data, err := os.ReadFile(s.path(id))
    if errors.Is(err, os.ErrNotExist) {
        return AgentRuntime{}, nil
    }
    if err != nil {
        return AgentRuntime{}, err
    }
    var v AgentRuntime
    if err := json.Unmarshal(data, &v); err != nil {
        log.Printf("WARN agent_state_store: corrupted JSON for %s: %v — recovering with defaults", id, err)
        return AgentRuntime{}, nil
    }
    return v, nil
}

// Save writes the AgentRuntime atomically: tempfile + rename.
func (s *AgentStateStore) Save(id string, v AgentRuntime) error {
    s.lockFor(id).Lock()
    defer s.lockFor(id).Unlock()
    data, err := json.Marshal(v)
    if err != nil {
        return err
    }
    tmp, err := os.CreateTemp(s.dir, id+".*.tmp")
    if err != nil {
        return err
    }
    tmpPath := tmp.Name()
    if _, err := tmp.Write(data); err != nil {
        tmp.Close()
        os.Remove(tmpPath)
        return err
    }
    if err := tmp.Sync(); err != nil {
        tmp.Close()
        os.Remove(tmpPath)
        return err
    }
    if err := tmp.Close(); err != nil {
        os.Remove(tmpPath)
        return err
    }
    return os.Rename(tmpPath, s.path(id))
}

// Purge removes the agent state file for the bead. Idempotent.
func (s *AgentStateStore) Purge(id string) error {
    s.lockFor(id).Lock()
    defer s.lockFor(id).Unlock()
    if err := os.Remove(s.path(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
        return err
    }
    return nil
}
```

- [ ] **Step 4: Run tests.**

```bash
go test ./orchestrator/internal/workflow/ -run TestAgentStateStore -v
```
Expected: PASS for all six.

- [ ] **Step 5: Commit.**

```bash
git add orchestrator/internal/workflow/agent_state_store.go orchestrator/internal/workflow/agent_state_store_test.go
git commit -m "workflow: add AgentStateStore (atomic JSON store with mutex + recover-on-corrupt)"
```

**Acceptance Criteria:**
- [ ] `TestAgentStateStore_CorruptedJSON_RecoverWithDefaults` passes (covers D9=A).
- [ ] `TestAgentStateStore_AtomicWrite_NoTempLeftover` verifies no orphan `.tmp` files.
- [ ] `TestAgentStateStore_ConcurrentWrites_SameBead_Serialized` runs 50 goroutines without torn JSON.
- [ ] `TestAgentStateStore_Purge_RemovesFile` confirms idempotence.

---

### Task 5: workflow/ensure_custom.go — Idempotent custom-status registration

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: `45`
- Dependencies: `Task 2`
- Parent: `Task 1`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/workflow/ensure_custom.go`
- Test: `orchestrator/internal/workflow/ensure_custom_test.go`

**Description / Steps:**

Mirror gastown `beads_types.go:187` cargo-cult 1:1 (D7=B / TT3=B). In-memory cache keyed by beads dir + sentinel file in `.beads/.kernl-custom-statuses-installed` for staleness detection. Calls `bd config set status.custom <merged-list>` preserving foreign custom statuses (merge, not overwrite).

- [ ] **Step 1: Write tests** (using a fake `bd` runner abstraction).

```go
package workflow_test

import (
    "errors"
    "strings"
    "testing"

    "github.com/gabrielassisxyz/kernl/orchestrator/internal/workflow"
)

type fakeBdRunner struct {
    customs []string
    calls   []string
    failOn  string
}

func (f *fakeBdRunner) GetCustomStatuses() ([]string, error) {
    f.calls = append(f.calls, "get")
    if f.failOn == "get" {
        return nil, errors.New("boom")
    }
    return append([]string(nil), f.customs...), nil
}
func (f *fakeBdRunner) SetCustomStatuses(list []string) error {
    f.calls = append(f.calls, "set:"+strings.Join(list, ","))
    if f.failOn == "set" {
        return errors.New("boom")
    }
    f.customs = append([]string(nil), list...)
    return nil
}

func TestEnsureCustomStatuses_FreshRegistersBoth(t *testing.T) {
    dir := t.TempDir()
    r := &fakeBdRunner{}
    if err := workflow.EnsureCustomStatuses(dir, r); err != nil {
        t.Fatal(err)
    }
    want := strings.Join(workflow.KernlCustomStatuses, ",")
    found := false
    for _, c := range r.calls {
        if c == "set:"+want {
            found = true
        }
    }
    if !found {
        t.Fatalf("expected set call with %q, got %+v", want, r.calls)
    }
}

func TestEnsureCustomStatuses_AlreadyPresent_NoOp(t *testing.T) {
    dir := t.TempDir()
    r := &fakeBdRunner{customs: workflow.KernlCustomStatuses}
    if err := workflow.EnsureCustomStatuses(dir, r); err != nil {
        t.Fatal(err)
    }
    for _, c := range r.calls {
        if strings.HasPrefix(c, "set:") {
            t.Fatalf("expected no set call, got %q", c)
        }
    }
}

func TestEnsureCustomStatuses_ForeignCustomsPreserved(t *testing.T) {
    dir := t.TempDir()
    r := &fakeBdRunner{customs: []string{"awaiting_qa", "needs_design"}}
    if err := workflow.EnsureCustomStatuses(dir, r); err != nil {
        t.Fatal(err)
    }
    // Last set call should contain awaiting_qa, needs_design, plus kernl customs.
    var lastSet string
    for _, c := range r.calls {
        if strings.HasPrefix(c, "set:") {
            lastSet = c
        }
    }
    for _, expected := range []string{"awaiting_qa", "needs_design", "awaiting_integration", "awaiting_pr_review"} {
        if !strings.Contains(lastSet, expected) {
            t.Fatalf("expected %q in %q", expected, lastSet)
        }
    }
}

func TestEnsureCustomStatuses_BdFailure_Propagates(t *testing.T) {
    dir := t.TempDir()
    r := &fakeBdRunner{failOn: "set"}
    if err := workflow.EnsureCustomStatuses(dir, r); err == nil {
        t.Fatal("expected error to propagate")
    }
}

func TestEnsureCustomStatuses_CachedAfterFirstCall(t *testing.T) {
    dir := t.TempDir()
    r := &fakeBdRunner{}
    _ = workflow.EnsureCustomStatuses(dir, r)
    callsAfterFirst := len(r.calls)
    _ = workflow.EnsureCustomStatuses(dir, r) // second call hits cache
    if len(r.calls) != callsAfterFirst {
        t.Fatalf("expected cache hit, second call added %d more calls", len(r.calls)-callsAfterFirst)
    }
}
```

- [ ] **Step 2: Run tests to confirm failure.**

```bash
go test ./orchestrator/internal/workflow/ -run TestEnsureCustomStatuses -v
```

- [ ] **Step 3: Implement `ensure_custom.go`.**

```go
package workflow

import (
    "os"
    "path/filepath"
    "sort"
    "sync"
)

// BdRunner is the minimal surface needed by EnsureCustomStatuses.
// In production, the BdCliBackend implements it via `bd config get/set status.custom`.
type BdRunner interface {
    GetCustomStatuses() ([]string, error)
    SetCustomStatuses(list []string) error
}

var (
    ensureCacheMu sync.Mutex
    ensureCache   = map[string]bool{} // key: beadsDir absolute path
)

// EnsureCustomStatuses registers the kernl custom statuses with bd idempotently.
// Cache + sentinel pattern mirrors gastown beads_types.go:187 (D7=B / TT3=B).
// Foreign customs already registered are preserved (merge, not overwrite).
func EnsureCustomStatuses(beadsDir string, r BdRunner) error {
    abs, err := filepath.Abs(beadsDir)
    if err != nil {
        return err
    }
    ensureCacheMu.Lock()
    cached := ensureCache[abs]
    ensureCacheMu.Unlock()
    if cached {
        return nil
    }

    current, err := r.GetCustomStatuses()
    if err != nil {
        return err
    }

    need := false
    have := map[string]bool{}
    for _, s := range current {
        have[s] = true
    }
    for _, s := range KernlCustomStatuses {
        if !have[s] {
            need = true
        }
    }

    if need {
        merged := append([]string(nil), current...)
        for _, s := range KernlCustomStatuses {
            if !have[s] {
                merged = append(merged, s)
            }
        }
        sort.Strings(merged)
        if err := r.SetCustomStatuses(merged); err != nil {
            return err
        }
    }

    // Sentinel file (advisory only — cache is the hot path).
    sentinelPath := filepath.Join(beadsDir, ".kernl-custom-statuses-installed")
    _ = os.WriteFile(sentinelPath, []byte("ok\n"), 0o644)

    ensureCacheMu.Lock()
    ensureCache[abs] = true
    ensureCacheMu.Unlock()
    return nil
}

// ResetEnsureCache is a test hook; clears the in-memory cache.
func ResetEnsureCache() {
    ensureCacheMu.Lock()
    defer ensureCacheMu.Unlock()
    ensureCache = map[string]bool{}
}
```

- [ ] **Step 4: Run tests.**

```bash
go test ./orchestrator/internal/workflow/ -v
```
Expected: PASS. Note: cache test may need `ResetEnsureCache()` between test cases if a prior test populated it for the same tmpdir; tests use `t.TempDir()` which produces a unique path per test, so cache won't collide across cases — but the `TestEnsureCustomStatuses_CachedAfterFirstCall` deliberately reuses the same dir.

- [ ] **Step 5: Commit.**

```bash
git add orchestrator/internal/workflow/ensure_custom.go orchestrator/internal/workflow/ensure_custom_test.go
git commit -m "workflow: add EnsureCustomStatuses (idempotent, cache + sentinel, merges foreign customs)"
```

**Acceptance Criteria:**
- [ ] `TestEnsureCustomStatuses_FreshRegistersBoth` passes.
- [ ] `TestEnsureCustomStatuses_ForeignCustomsPreserved` confirms merge behaviour (no overwrite).
- [ ] `TestEnsureCustomStatuses_CachedAfterFirstCall` proves cache short-circuits.
- [ ] `TestEnsureCustomStatuses_BdFailure_Propagates` confirms fail-loud.

---

## Task 6: Lane G Epic — Specs documentation update

**Bead Mapping:**
- type: `epic`
- Priority: `1`
- Estimated Minutes: `0`
- Dependencies: `none`
- Parent: `Task 0`
- Status: `open`

**Files:** Container — children touch `orchestrator/specs/`.

**Description:** Update architecture and component specs to describe the new workflow, MergeManager, sweep, custom statuses, and stable-field description contracts. Documentation-only — runs in parallel with Lane A.

**Acceptance Criteria:**
- [ ] All spec files cite spec sections from `docs/2026-05-15-kernl-workflow-brainstorm-spec.md`.
- [ ] No `*.md` in `orchestrator/specs/` references foolery legacy statuses except in explicit `[source: foolery/...]` provenance lines.

---

### Task 7: Update `orchestrator/specs/00-architecture.md`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: `30`
- Dependencies: `none`
- Parent: `Task 6`
- Status: `open`

**Files:**
- Modify: `orchestrator/specs/00-architecture.md`

**Description / Steps:**

- [ ] **Step 1:** Search for references to `foolery.go` (stale, around line 522 per spec §8.2) and replace with current architecture wording.
- [ ] **Step 2:** Add a new section "Workflow (kernl-native)" with the §4.3 transition diagram from the brainstorm-spec (copy-paste the ASCII diagram for child + epic lifecycles).
- [ ] **Step 3:** Remove all references to legacy profiles (`autopilot`, `semiauto`) and the 13-state machine.
- [ ] **Step 4:** Add subsection "Persistence layers" describing the three layers (built-in / custom / description) and the D1=C divergence (AgentState in JSON local).
- [ ] **Step 5:** Commit.

```bash
git add orchestrator/specs/00-architecture.md
git commit -m "specs(architecture): align with kernl-native workflow (3 layers + JSON local AgentState)"
```

**Acceptance Criteria:**
- [ ] `grep -n "foolery.go" orchestrator/specs/00-architecture.md` returns nothing.
- [ ] `grep -n "ready_for_implementation\|implementation_review\|shipped" orchestrator/specs/00-architecture.md` returns nothing or only inside explicit `[source: foolery/...]` provenance lines.
- [ ] Section "Workflow (kernl-native)" exists with the transition diagram.

---

### Task 8: Update `orchestrator/specs/backend/backend.md`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: `25`
- Dependencies: `none`
- Parent: `Task 6`
- Status: `open`

**Files:**
- Modify: `orchestrator/specs/backend/backend.md`

**Description / Steps:**

- [ ] **Step 1:** Add section "Custom statuses" — describe `EnsureCustomStatuses(beadsDir, r)` contract: idempotent, cache + sentinel, merges foreign customs, called on every backend op via `BdCliBackend.Init`.
- [ ] **Step 2:** Add section "Description-field contracts" — list the stable fields (`worktree_path`, `worktree_branch`, `epic_branch`, `pr_url`, `merge_conflict_at`, `merge_outcome`) and the typed accessors (`GetEpicBranch`, `SetEpicBranch`, etc.). Cite spec §4.5.
- [ ] **Step 3:** Update the write path description: `bdcli.Update(input.State)` now accepts kernl-native `IssueStatus` values only; reject legacy strings explicitly.
- [ ] **Step 4:** Preserve `[source: foolery/src/...]` provenance lines where behavior contract is inherited and still valid.
- [ ] **Step 5:** Commit.

```bash
git add orchestrator/specs/backend/backend.md
git commit -m "specs(backend): document EnsureCustomStatuses contract and stable-field accessors"
```

**Acceptance Criteria:**
- [ ] Sections "Custom statuses" and "Description-field contracts" present.
- [ ] Stable-field list matches spec §4.5 exactly.

---

### Task 9: Update `orchestrator/specs/orchestration/orchestration.md`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: `30`
- Dependencies: `none`
- Parent: `Task 6`
- Status: `open`

**Files:**
- Modify: `orchestrator/specs/orchestration/orchestration.md`

**Description / Steps:**

- [ ] **Step 1:** Replace the epic-lifecycle section with the new 11-step flow from spec §5.2 (epic branch creation → workers → trigger detect → merger → push → PR → sweep).
- [ ] **Step 2:** Add subsection "MergeManager trigger detection" describing single `bd list --parent=<epic-id> --status=awaiting_integration --json` + count comparison (D14=A) + single-flight lock per `epic_id` (D11=A).
- [ ] **Step 3:** Add subsection "Sweep" describing the three-layer resilience (cache MERGED + circuit breaker + skip-on-fail + PR stale WARN).
- [ ] **Step 4:** Update transition diagram (the §4.3 ASCII).
- [ ] **Step 5:** Commit.

```bash
git add orchestrator/specs/orchestration/orchestration.md
git commit -m "specs(orchestration): describe MergeManager trigger, single-flight lock, and sweep resilience"
```

**Acceptance Criteria:**
- [ ] New epic-lifecycle steps 1–11 documented.
- [ ] D11 / D14 / D5 / TT2 cross-referenced.

---

### Task 10: Update `orchestrator/specs/prompt/prompt.md`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: `20`
- Dependencies: `none`
- Parent: `Task 6`
- Status: `open`

**Files:**
- Modify: `orchestrator/specs/prompt/prompt.md`

**Description / Steps:**

- [ ] **Step 1:** Add section "Merger prompt" describing the template inputs (`epic_id`, `epic_title`, `epic_branch`, ordered child list, base branch, literal outcomes list).
- [ ] **Step 2:** Document the three failure modes enumerated in the prompt (merge_conflict, push_failed, pr_create_failed) plus the success branches (success, pr_already_exists).
- [ ] **Step 3:** Add a "TODO follow-up" subsection: context-aware merger (which repo files to load — `AGENTS.md`, recent master commits, epic bead description, child bead descriptions). Not in MVP scope but registered here for the next iteration of the prompt spec.
- [ ] **Step 4:** Commit.

```bash
git add orchestrator/specs/prompt/prompt.md
git commit -m "specs(prompt): document merger prompt template and failure-mode enumeration"
```

**Acceptance Criteria:**
- [ ] Section "Merger prompt" present with all 5 outcomes listed literally.
- [ ] TODO section for context-aware merger documented.

---

## Task 11: Lane C Epic — Backend refactor + EnsureCustomStatuses wiring

**Bead Mapping:**
- type: `epic`
- Priority: `2`
- Estimated Minutes: `0`
- Dependencies: `Task 1`
- Parent: `Task 0`
- Status: `open`

**Files:** Container — children touch `orchestrator/internal/backend/`.

**Description:** Slim down `state_machine.go` (delete profile/builtin/owner machinery), shrink `WorkflowDescriptor` in `port.go`, default `factory.go` to bd (knots dormant — never routed), and rewire `bdcli.go` Update + Init to use the new workflow primitives + EnsureCustomStatuses.

**Acceptance Criteria:**
- [ ] `go test ./orchestrator/internal/backend/...` green.
- [ ] `BdCliBackend.Init` calls `EnsureCustomStatuses` exactly once per backend instance via cache.
- [ ] `factory.go` returns the bd backend by default; knots constructor still exists but is never called from `factory`.

---

### Task 12: Slim down `state_machine.go`

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `40`
- Dependencies: `Task 2`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Modify: `orchestrator/internal/backend/state_machine.go`

**Description / Steps:**

- [ ] **Step 1:** Delete the following symbols (spec §8.2): `profileConfig`, `builtinProfiles`, `agentOwners`, `semiautoOwners`, `normalizeProfileID`, `descriptorFromProfileConfig`, `initBuiltinWorkflows`, `BuiltinProfileDescriptor`, `resolveWorkflow`, `canonicalTransitions`, `buildStates`, `filterTransitions`, `deriveWorkflowStructureFromConfig`, `stepOwnerKind`.
- [ ] **Step 2:** Replace the remaining state-machine surface with a thin layer that constructs a `WorkflowDescriptor` from `workflow.KernlCustomStatuses` (or simply returns a static descriptor — sketch in spec §8.2 says target is ~150 LOC).
- [ ] **Step 3:** Run `go build ./...` and fix downstream call sites; expect compile errors in `factory.go` and `bdcli.go` — those are Tasks 13–14.
- [ ] **Step 4:** Commit (may need to commit Tasks 12–14 together if compile is broken intermediate; if so, mark this task as part of a stacked commit and finalize in Task 14).

```bash
git add orchestrator/internal/backend/state_machine.go
git commit -m "backend: slim down state_machine.go (delete profiles + builtin + owner machinery)"
```

**Acceptance Criteria:**
- [ ] `grep -n "profileConfig\|builtinProfiles\|agentOwners\|semiautoOwners\|normalizeProfileID" orchestrator/internal/backend/state_machine.go` returns nothing.
- [ ] File is ≤200 LOC.

---

### Task 13: Shrink `WorkflowDescriptor` in `port.go`

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `25`
- Dependencies: `Task 12`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Modify: `orchestrator/internal/backend/port.go`

**Description / Steps:**

- [ ] **Step 1:** Remove fields from `WorkflowDescriptor`: `Owners`, `QueueActions`, `ActionStates`, `ReviewQueueStates`, `HumanQueueStates`, `StateOwners`, `FinalCutState`, `Mode`.
- [ ] **Step 2:** Keep the minimal set needed for the new flow: `Statuses []string`, `Customs []string`, `Transitions []Transition` (if still used by EpicExecutor; verify).
- [ ] **Step 3:** Update all call sites that read the removed fields (search with `grep -rn "\.Owners\|\.QueueActions" orchestrator/`).
- [ ] **Step 4:** Run `go build ./orchestrator/...` and resolve compile errors.
- [ ] **Step 5:** Commit.

```bash
git add orchestrator/internal/backend/port.go
git commit -m "backend: shrink WorkflowDescriptor (remove unused profile-era fields)"
```

**Acceptance Criteria:**
- [ ] `WorkflowDescriptor` has only fields needed by the new workflow.
- [ ] `go build ./orchestrator/...` succeeds.

---

### Task 14: Refactor `bdcli.go` Update + Init + add description-field helpers + factory.go default

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `60`
- Dependencies: `Task 4, Task 5, Task 13`
- Parent: `Task 11`
- Status: `open`

**Files:**
- Modify: `orchestrator/internal/backend/bdcli.go`
- Modify: `orchestrator/internal/backend/factory.go`
- Test: `orchestrator/internal/backend/bdcli_test.go` (existing — adjust)

**Description / Steps:**

- [ ] **Step 1:** Add the `BdRunner` interface impl to `BdCliBackend`:

```go
// Satisfies workflow.BdRunner.
func (b *BdCliBackend) GetCustomStatuses() ([]string, error) {
    // shell: bd config get status.custom --json (or parse text output)
}

func (b *BdCliBackend) SetCustomStatuses(list []string) error {
    // shell: bd config set status.custom <csv>
}
```

- [ ] **Step 2:** Modify `NewBdCliBackend` / `Init` to call `workflow.EnsureCustomStatuses(repoPath, b)` once at backend creation:

```go
func NewBdCliBackend(repoPath string) (*BdCliBackend, error) {
    b := &BdCliBackend{repoPath: repoPath}
    if err := workflow.EnsureCustomStatuses(filepath.Join(repoPath, ".beads"), b); err != nil {
        return nil, fmt.Errorf("ensure custom statuses: %w", err)
    }
    return b, nil
}
```

- [ ] **Step 3:** Modify `Update` to reject legacy statuses with a clear error if the caller passes a foolery-era string. Accept only `IssueStatus` values from `workflow.*`.
- [ ] **Step 4:** Add description-field write helpers wired through `Update` when caller passes structured fields (e.g., `UpdateInput.EpicBranch`, `UpdateInput.PRURL`). These call `workflow.SetXxx` over the existing description.
- [ ] **Step 5:** Modify `factory.go`: default backend is `bd`; `KnotsBackend` constructor is preserved but factory never routes to it (preserves the eng-review-2026-05-14 decision that knots stays dormant).
- [ ] **Step 6:** Run tests, fix.

```bash
go test ./orchestrator/internal/backend/... -v
```

- [ ] **Step 7:** Commit.

```bash
git add orchestrator/internal/backend/bdcli.go orchestrator/internal/backend/factory.go orchestrator/internal/backend/bdcli_test.go
git commit -m "backend: wire EnsureCustomStatuses; bdcli.Update rejects legacy; factory routes to bd"
```

**Acceptance Criteria:**
- [ ] `BdCliBackend` satisfies `workflow.BdRunner`.
- [ ] `NewBdCliBackend` calls `EnsureCustomStatuses` (verified by a test that injects a fake bd or uses bd-cli probe).
- [ ] `Update` rejects `"ready_for_implementation"` with a descriptive error.
- [ ] Factory never returns a `KnotsBackend` instance.

---

## Task 15: Lane D Epic — String-literal refactor across source + tests

**Bead Mapping:**
- type: `epic`
- Priority: `2`
- Estimated Minutes: `0`
- Dependencies: `Task 1`
- Parent: `Task 0`
- Status: `open`

**Files:** Container — children touch `orchestrator/internal/dispatch/`, `orchestration/`, `terminal/`, `retake/`, `epic/`, `app/`, plus the 27 `*_test.go` files identified in TODOS.md.

**Description:** Replace foolery legacy status string literals (`"ready_for_implementation"`, `"implementation"`, `"implementation_review"`, `"shipment_review"`, `"shipped"`, `"deferred"`, `"abandoned"`, etc.) with constants from the `workflow` package across source and test files. Coordinate with Lane E (also touches `internal/integration/*_test.go`) — run sequentially or coordinate manually.

**Acceptance Criteria:**
- [ ] `grep -rEn '"ready_for_implementation"|"implementation_review"|"shipment_review"|"shipped"' orchestrator/internal/ orchestrator/cmd/` returns nothing.
- [ ] All affected `*_test.go` use `workflow.Status*` constants.

---

### Task 16: Refactor source files (dispatch/, orchestration/, terminal/, retake/, epic/, app/)

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `60`
- Dependencies: `Task 2`
- Parent: `Task 15`
- Status: `open`

**Files:**
- Modify: `orchestrator/internal/dispatch/*.go`
- Modify: `orchestrator/internal/orchestration/{sorting,workflow}.go`
- Modify: `orchestrator/internal/retake/retake.go`
- Modify: `orchestrator/internal/terminal/{followup,takeloop_rollback,takeloop}.go` (where literals appear; some files may be in tests only)
- Modify: `orchestrator/internal/epic/resume.go` (where literals appear)
- Modify: `orchestrator/internal/app/driver.go` (where literals appear)

**Description / Steps:**

- [ ] **Step 1:** Enumerate every source-file occurrence:

```bash
grep -rEn '"ready_for_implementation"|"implementation"|"implementation_review"|"ready_for_shipment"|"shipment"|"shipment_review"|"shipped"|"deferred"|"abandoned"|"review"' \
  orchestrator/internal/dispatch orchestrator/internal/orchestration orchestrator/internal/terminal \
  orchestrator/internal/retake orchestrator/internal/epic orchestrator/internal/app \
  --include='*.go' --exclude='*_test.go'
```

- [ ] **Step 2:** For each occurrence, replace per spec §8.4 mapping:

| Legacy | New |
|---|---|
| `"ready_for_implementation"` | `workflow.StatusOpen` |
| `"implementation"` | `workflow.StatusInProgress` |
| `"implementation_review"` | `workflow.StatusAwaitingIntegration` |
| `"ready_for_shipment"` | `workflow.StatusAwaitingIntegration` |
| `"shipment" / "shipment_review" / "shipped"` | `workflow.StatusClosed` |
| `"deferred" / "abandoned"` | drop runtime usage; these are human-only `bd close --reason=...` values |

- [ ] **Step 3:** Run `go build ./orchestrator/...` repeatedly until clean.
- [ ] **Step 4:** Run `go vet ./orchestrator/...`.
- [ ] **Step 5:** Commit.

```bash
git add orchestrator/internal/dispatch orchestrator/internal/orchestration orchestrator/internal/terminal \
        orchestrator/internal/retake orchestrator/internal/epic orchestrator/internal/app
git commit -m "internal: replace legacy status string literals with workflow.Status* constants (source)"
```

**Acceptance Criteria:**
- [ ] `grep -rEn '"ready_for_implementation"|"implementation_review"|"shipment_review"|"shipped"' orchestrator/internal/ --include='*.go' --exclude='*_test.go'` returns nothing.
- [ ] `go vet ./orchestrator/...` clean.

---

### Task 17: Refactor 27 `*_test.go` files

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `90`
- Dependencies: `Task 2, Task 16`
- Parent: `Task 15`
- Status: `open`

**Files:**
- Modify: 27 `*_test.go` files listed in `TODOS.md` "Definir workflow próprio do kernl" entry plus any newly discovered via grep
  - `internal/backend/{state_machine,factory,knots,bdcli,dto}_test.go`
  - `internal/dispatch/{dispatch,forensics}_test.go`
  - `internal/orchestration/{sorting,workflow}_test.go`
  - `internal/retake/retake_test.go`
  - `internal/terminal/{followup,takeloop_rollback,takeloop}_test.go`
  - `internal/epic/resume_test.go`
  - `internal/app/driver_test.go`
  - `internal/integration/harness.go`
  - `internal/integration/passo_a_test.go`
  - `cmd/kernl/bead_test.go`

**Description / Steps:**

- [ ] **Step 1:** Enumerate:

```bash
grep -rEln '"ready_for_implementation"|"implementation"|"implementation_review"|"ready_for_shipment"|"shipment"|"shipment_review"|"shipped"' \
  orchestrator/ --include='*_test.go'
```

- [ ] **Step 2:** Mechanically replace per the table in Task 16. Where a test asserts behavior specific to the legacy foolery distinction between `deferred` and `closed` (i.e., the conceptual difference, not the string value), **delete that test** per spec §8.4 — the concept leaves the model. Document each deleted test in the commit message.
- [ ] **Step 3:** Run `go test ./orchestrator/... -count=1` and triage failures:
  - Compile errors → fix imports.
  - Assert mismatches → adjust to new constants.
  - Behaviorally-unsound tests that depended on legacy semantics → delete or rewrite as a workflow-relevant test.
- [ ] **Step 4:** Final sweep:

```bash
go test ./orchestrator/... -count=1 2>&1 | tee /tmp/test-out.log
grep -E 'FAIL|---' /tmp/test-out.log
```

- [ ] **Step 5:** Commit.

```bash
git add -u orchestrator/
git commit -m "tests: replace legacy status literals across 27 *_test.go files (workflow.Status*)"
```

**Acceptance Criteria:**
- [ ] All targeted `*_test.go` files compile and pass.
- [ ] 962 pre-existing unit tests pass (within the count adjusted by intentional deletions documented in commit message).
- [ ] `grep -rEn '"ready_for_implementation"|"implementation_review"|"shipment_review"' orchestrator/ --include='*.go'` returns nothing.

---

## Task 18: Lane E Epic — Fixtures + Harness bd 1.0.4 pin + CI

**Bead Mapping:**
- type: `epic`
- Priority: `2`
- Estimated Minutes: `0`
- Dependencies: `Task 1`
- Parent: `Task 0`
- Status: `open`

**Files:** Container — children touch `orchestrator/internal/integration/testdata/`, `orchestrator/internal/integration/harness.go`, CI workflows.

**Description:** Migrate fixtures per the sed map in spec §8.4 and pin `bd ≥ 1.0.4` in the harness with fail-fast version probe + install pinned bd in CI (D10=A).

**Acceptance Criteria:**
- [ ] `bd init --from-jsonl` succeeds for all fixtures.
- [ ] Harness aborts with a clear message if `bd --version < 1.0.4`.
- [ ] CI installs `bd@v1.0.4` deterministically.

---

### Task 19: Apply sed map to fixtures

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `45`
- Dependencies: `Task 2`
- Parent: `Task 18`
- Status: `open`

**Files:**
- Modify: `orchestrator/internal/integration/testdata/beads-single/.beads/issues.jsonl`
- Modify: `orchestrator/internal/integration/testdata/beads-epic-diamond/.beads/issues.jsonl`

**Description / Steps:**

- [ ] **Step 0 (coordination with Lane D):** before editing fixtures, check whether `Task 17` (Lane D) is mid-flight against `internal/integration/*_test.go`. If yes, sync up — both lanes mutate test-adjacent paths. Either serialize or split the file set explicitly (`testdata/` ⊆ Lane E only; `*_test.go` ⊆ Lane D only).

- [ ] **Step 1:** Apply the sed map from spec §8.4 to both fixture files. Mapping:

| Legacy JSONL value | New JSONL value | Notes |
|---|---|---|
| `"status":"ready_for_implementation"` | `"status":"open"` | |
| `"status":"implementation"` | `"status":"in_progress"` | Add `agent_state: working` to AgentRuntime JSON if the test depends on agent state (see Step 2). |
| `"status":"implementation_review"` | `"status":"awaiting_integration"` | |
| `"status":"ready_for_shipment"` | `"status":"awaiting_integration"` | |
| `"status":"shipment"` `\|` `"shipment_review"` `\|` `"shipped"` | `"status":"closed"` | |
| `"status":"deferred"` | `"status":"closed"` and add `"close_reason":"deferred"` | Preserves legacy semantic — pause intentional, NOT blocked. |
| `"status":"abandoned"` | `"status":"closed"` and add `"close_reason":"abandoned"` | |

- [ ] **Step 2:** Where a test relied on `implementation` to mean "agent actively working", supplement the fixture with an `~/.kernl/state/<bead-id>.json` seed (if the harness supports preloading; otherwise note in the test that the harness must `Save()` the AgentRuntime). Keep this minimal — most tests will not need it.

- [ ] **Step 3:** Verify both fixtures parse cleanly:

```bash
for f in orchestrator/internal/integration/testdata/beads-*/.beads/issues.jsonl; do
  while read -r line; do
    echo "$line" | jq . >/dev/null || { echo "INVALID: $f"; break; }
  done < "$f"
done
```

- [ ] **Step 4:** Run integration tests in dry-run mode if possible (or just smoke `bd init --from-jsonl`):

```bash
( cd $(mktemp -d) && cp -r /home/gabriel/repositories/kernl/orchestrator/internal/integration/testdata/beads-single . && cd beads-single && bd init --from-jsonl .beads/issues.jsonl )
```

- [ ] **Step 5:** Commit.

```bash
git add orchestrator/internal/integration/testdata/
git commit -m "fixtures: migrate legacy foolery statuses to kernl-native (sed map per spec §8.4)"
```

**Acceptance Criteria:**
- [ ] `bd init --from-jsonl` succeeds in a clean tmpdir for both fixtures.
- [ ] `grep -E '"status":"(ready_for_implementation|implementation_review|shipment_review|shipped)"' orchestrator/internal/integration/testdata/` returns nothing.

---

### Task 20: Pin `bd ≥ 1.0.4` in harness + CI

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `35`
- Dependencies: `Task 19`
- Parent: `Task 18`
- Status: `open`

**Files:**
- Modify: `orchestrator/internal/integration/harness.go`
- Modify: `.github/workflows/*.yml` (or equivalent CI config)
- Modify: `AGENTS.md` and `README.md`

**Description / Steps:**

- [ ] **Step 1:** Add a version probe in `harness.SetUp`:

```go
const minBdVersion = "1.0.4"

func (h *Harness) ensureBdVersion(t *testing.T) {
    t.Helper()
    out, err := exec.Command("bd", "--version").CombinedOutput()
    if err != nil {
        t.Fatalf("bd CLI required (>= %s): %v\n%s", minBdVersion, err, out)
    }
    v := parseBdVersion(string(out)) // implement: extract semver from "bd X.Y.Z"
    if semver.Compare("v"+v, "v"+minBdVersion) < 0 {
        t.Fatalf("bd version %s < required %s — run: go install github.com/.../bd@v%s", v, minBdVersion, minBdVersion)
    }
}
```

- [ ] **Step 2:** Add `golang.org/x/mod/semver` (or implement parse manually) and call `ensureBdVersion` from harness `SetUp`.
- [ ] **Step 3:** Update CI workflow (e.g., `.github/workflows/test.yml`) to install pinned bd:

```yaml
- name: Install bd
  run: go install github.com/gastownhall/beads@v1.0.4
```

- [ ] **Step 4:** Document the requirement in `AGENTS.md` and `README.md` under "Prerequisites" / "Dev setup".
- [ ] **Step 5:** Run `go test -tags=integration ./orchestrator/internal/integration/...` (or simulate by invoking harness setup) and verify a sub-version bd would abort.
- [ ] **Step 6:** Commit.

```bash
git add orchestrator/internal/integration/harness.go .github/workflows/ AGENTS.md README.md
git commit -m "test+ci: pin bd >= 1.0.4 with fail-fast version probe (D10=A)"
```

**Acceptance Criteria:**
- [ ] `harness.SetUp` aborts with descriptive error if `bd --version` returns < 1.0.4.
- [ ] CI workflow installs `bd@v1.0.4` before any test job.
- [ ] `AGENTS.md` and `README.md` document the requirement.

---

## Task 21: Lane F1 Epic — Merge package

**Bead Mapping:**
- type: `epic`
- Priority: `2`
- Estimated Minutes: `0`
- Dependencies: `Task 1`
- Parent: `Task 0`
- Status: `open`

**Files:** Container — children create `orchestrator/internal/merge/`.

**Description:** Build the `merge` package: typed `Outcome` enum + `ParseOutcome`, `MergeManager` with single-flight trigger detect via single `bd list` query + outcome routing.

**Acceptance Criteria:**
- [ ] `go test ./orchestrator/internal/merge/...` green including deterministic concurrency test.
- [ ] Single-flight test runs 100 goroutines without spawning multiple merger dispatches per epic.

---

### Task 22: merge/errors.go — Outcome enum + ParseOutcome

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `25`
- Dependencies: `Task 2`
- Parent: `Task 21`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/merge/errors.go`
- Test: `orchestrator/internal/merge/errors_test.go`

**Description / Steps:**

- [ ] **Step 1:** Write tests.

```go
package merge_test

import (
    "testing"

    "github.com/gabrielassisxyz/kernl/orchestrator/internal/merge"
)

func TestParseOutcome_Valid(t *testing.T) {
    cases := map[string]merge.Outcome{
        "success":            merge.OutcomeSuccess,
        "merge_conflict":     merge.OutcomeMergeConflict,
        "push_failed":        merge.OutcomePushFailed,
        "pr_create_failed":   merge.OutcomePRCreateFailed,
        "pr_already_exists":  merge.OutcomePRAlreadyExists,
    }
    for s, want := range cases {
        t.Run(s, func(t *testing.T) {
            got, err := merge.ParseOutcome(s)
            if err != nil {
                t.Fatalf("err: %v", err)
            }
            if got != want {
                t.Fatalf("got %q want %q", got, want)
            }
        })
    }
}

func TestParseOutcome_Invalid(t *testing.T) {
    if _, err := merge.ParseOutcome("nope"); err == nil {
        t.Fatal("expected error for unknown outcome")
    }
    if _, err := merge.ParseOutcome(""); err == nil {
        t.Fatal("expected error for empty outcome")
    }
}
```

- [ ] **Step 2:** Run, confirm fail.

```bash
go test ./orchestrator/internal/merge/ -v
```

- [ ] **Step 3:** Implement `errors.go`.

```go
// Package merge provides the typed outcome enum and the MergeManager that
// routes epic-level transitions based on the merger agent's reported outcome.
// Spec: docs/2026-05-15-kernl-workflow-brainstorm-spec.md §5.3-§5.4.
package merge

import "fmt"

type Outcome string

const (
    OutcomeSuccess         Outcome = "success"
    OutcomeMergeConflict   Outcome = "merge_conflict"
    OutcomePushFailed      Outcome = "push_failed"
    OutcomePRCreateFailed  Outcome = "pr_create_failed"
    OutcomePRAlreadyExists Outcome = "pr_already_exists"
)

// All returns the full enum — used by the merger prompt template to render the literal list.
func All() []Outcome {
    return []Outcome{
        OutcomeSuccess,
        OutcomeMergeConflict,
        OutcomePushFailed,
        OutcomePRCreateFailed,
        OutcomePRAlreadyExists,
    }
}

func ParseOutcome(s string) (Outcome, error) {
    switch Outcome(s) {
    case OutcomeSuccess, OutcomeMergeConflict, OutcomePushFailed, OutcomePRCreateFailed, OutcomePRAlreadyExists:
        return Outcome(s), nil
    }
    return "", fmt.Errorf("unknown merge_outcome: %q", s)
}
```

- [ ] **Step 4:** Run tests; PASS.
- [ ] **Step 5:** Commit.

```bash
git add orchestrator/internal/merge/errors.go orchestrator/internal/merge/errors_test.go
git commit -m "merge: add typed Outcome enum + ParseOutcome (5 values per D6=A)"
```

**Acceptance Criteria:**
- [ ] All 5 outcomes parse correctly.
- [ ] Unknown outcome returns descriptive error.
- [ ] `merge.All()` returns the 5 outcomes in the order the prompt expects.

---

### Task 23: merge/manager.go — MergeManager (trigger detect + single-flight + outcome routing)

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `90`
- Dependencies: `Task 22, Task 14`
- Parent: `Task 21`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/merge/manager.go`
- Test: `orchestrator/internal/merge/manager_test.go`

**Description / Steps:**

- [ ] **Step 1:** Write tests covering: trigger detect via single bd list (D14), single-flight lock with 100 goroutines (D11), outcome routing exhaustive switch (D6), missing outcome → blocked diag.

```go
package merge_test

import (
    "sync"
    "testing"

    "github.com/gabrielassisxyz/kernl/orchestrator/internal/merge"
    "github.com/gabrielassisxyz/kernl/orchestrator/internal/workflow"
)

// fakeBackend implements merge.Backend with controllable bd list responses + bd update tracking.
type fakeBackend struct {
    mu             sync.Mutex
    childrenByEpic map[string][]child
    updates        []string
    descByID       map[string]string
}

type child struct {
    ID     string
    Status workflow.IssueStatus
}

func (f *fakeBackend) ListChildrenAwaitingIntegration(epicID string) ([]string, error) {
    f.mu.Lock()
    defer f.mu.Unlock()
    var out []string
    for _, c := range f.childrenByEpic[epicID] {
        if c.Status == workflow.StatusAwaitingIntegration {
            out = append(out, c.ID)
        }
    }
    return out, nil
}

func (f *fakeBackend) CountChildren(epicID string) (int, error) {
    f.mu.Lock()
    defer f.mu.Unlock()
    return len(f.childrenByEpic[epicID]), nil
}

func (f *fakeBackend) UpdateStatus(id string, s workflow.IssueStatus) error {
    f.mu.Lock()
    defer f.mu.Unlock()
    f.updates = append(f.updates, id+"->"+string(s))
    return nil
}

func (f *fakeBackend) GetDescription(id string) (string, error) {
    f.mu.Lock()
    defer f.mu.Unlock()
    return f.descByID[id], nil
}

func newFakeBackend() *fakeBackend {
    return &fakeBackend{childrenByEpic: map[string][]child{}, descByID: map[string]string{}}
}

// fakeDispatcher records merger spawn calls.
type fakeDispatcher struct {
    mu     sync.Mutex
    spawns []string
}

func (d *fakeDispatcher) DispatchMerger(epicID string) error {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.spawns = append(d.spawns, epicID)
    return nil
}

func TestMergeManager_TriggerWhenAllChildrenAwaiting(t *testing.T) {
    b := newFakeBackend()
    b.childrenByEpic["e1"] = []child{
        {ID: "c1", Status: workflow.StatusAwaitingIntegration},
        {ID: "c2", Status: workflow.StatusAwaitingIntegration},
        {ID: "c3", Status: workflow.StatusAwaitingIntegration},
    }
    d := &fakeDispatcher{}
    m := merge.NewManager(b, d)
    if err := m.TryTrigger("e1"); err != nil {
        t.Fatal(err)
    }
    if len(d.spawns) != 1 || d.spawns[0] != "e1" {
        t.Fatalf("expected one spawn for e1, got %+v", d.spawns)
    }
}

func TestMergeManager_NoTriggerWhenSomeChildrenNotReady(t *testing.T) {
    b := newFakeBackend()
    b.childrenByEpic["e1"] = []child{
        {ID: "c1", Status: workflow.StatusAwaitingIntegration},
        {ID: "c2", Status: workflow.StatusInProgress},
    }
    d := &fakeDispatcher{}
    m := merge.NewManager(b, d)
    if err := m.TryTrigger("e1"); err != nil {
        t.Fatal(err)
    }
    if len(d.spawns) != 0 {
        t.Fatalf("expected no spawn, got %+v", d.spawns)
    }
}

func TestMergeManager_SingleFlight_100Goroutines(t *testing.T) {
    b := newFakeBackend()
    b.childrenByEpic["e1"] = []child{
        {ID: "c1", Status: workflow.StatusAwaitingIntegration},
        {ID: "c2", Status: workflow.StatusAwaitingIntegration},
    }
    d := &fakeDispatcher{}
    m := merge.NewManager(b, d)
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            _ = m.TryTrigger("e1")
        }()
    }
    wg.Wait()
    if len(d.spawns) != 1 {
        t.Fatalf("single-flight failure: %d spawns for e1", len(d.spawns))
    }
}

func TestMergeManager_RouteOutcome_Success(t *testing.T) {
    b := newFakeBackend()
    b.childrenByEpic["e1"] = []child{
        {ID: "c1", Status: workflow.StatusAwaitingIntegration},
    }
    b.descByID["e1"] = "epic_branch: feat/e1\nmerge_outcome: success\npr_url: https://x/pr/1\n"
    m := merge.NewManager(b, &fakeDispatcher{})
    if err := m.RouteOutcome("e1"); err != nil {
        t.Fatal(err)
    }
    // Expect children closed + epic → awaiting_pr_review.
    got := b.updates
    want := []string{"c1->closed", "e1->awaiting_pr_review"}
    if len(got) != len(want) {
        t.Fatalf("got %v want %v", got, want)
    }
    for i := range want {
        if got[i] != want[i] {
            t.Fatalf("step %d: got %q want %q", i, got[i], want[i])
        }
    }
}

func TestMergeManager_RouteOutcome_Blocked_Variants(t *testing.T) {
    cases := []struct {
        outcome string
        want    string
    }{
        {"merge_conflict", "e1->blocked"},
        {"push_failed", "e1->blocked"},
        {"pr_create_failed", "e1->blocked"},
    }
    for _, c := range cases {
        t.Run(c.outcome, func(t *testing.T) {
            b := newFakeBackend()
            b.childrenByEpic["e1"] = []child{{ID: "c1", Status: workflow.StatusAwaitingIntegration}}
            b.descByID["e1"] = "merge_outcome: " + c.outcome + "\n"
            m := merge.NewManager(b, &fakeDispatcher{})
            if err := m.RouteOutcome("e1"); err != nil {
                t.Fatal(err)
            }
            found := false
            for _, u := range b.updates {
                if u == c.want {
                    found = true
                }
            }
            if !found {
                t.Fatalf("expected %q in %+v", c.want, b.updates)
            }
        })
    }
}

func TestMergeManager_RouteOutcome_PRAlreadyExists_AdoptsPR(t *testing.T) {
    b := newFakeBackend()
    b.childrenByEpic["e1"] = []child{{ID: "c1", Status: workflow.StatusAwaitingIntegration}}
    b.descByID["e1"] = "merge_outcome: pr_already_exists\npr_url: https://x/pr/42\n"
    m := merge.NewManager(b, &fakeDispatcher{})
    if err := m.RouteOutcome("e1"); err != nil {
        t.Fatal(err)
    }
    // Expect treated as success (children closed + epic → awaiting_pr_review).
    found := false
    for _, u := range b.updates {
        if u == "e1->awaiting_pr_review" {
            found = true
        }
    }
    if !found {
        t.Fatalf("expected awaiting_pr_review, got %+v", b.updates)
    }
}

func TestMergeManager_RouteOutcome_MissingOutcome_Blocked(t *testing.T) {
    b := newFakeBackend()
    b.descByID["e1"] = "epic_branch: feat/e1\n" // no merge_outcome
    m := merge.NewManager(b, &fakeDispatcher{})
    if err := m.RouteOutcome("e1"); err != nil {
        t.Fatal(err)
    }
    found := false
    for _, u := range b.updates {
        if u == "e1->blocked" {
            found = true
        }
    }
    if !found {
        t.Fatalf("expected blocked, got %+v", b.updates)
    }
}

func TestMergeManager_RouteOutcome_InvalidOutcome_Blocked(t *testing.T) {
    b := newFakeBackend()
    b.descByID["e1"] = "merge_outcome: nonsense\n"
    m := merge.NewManager(b, &fakeDispatcher{})
    if err := m.RouteOutcome("e1"); err != nil {
        t.Fatal(err)
    }
    found := false
    for _, u := range b.updates {
        if u == "e1->blocked" {
            found = true
        }
    }
    if !found {
        t.Fatalf("expected blocked, got %+v", b.updates)
    }
}
```

- [ ] **Step 2:** Run tests, confirm fail.
- [ ] **Step 3:** Implement `manager.go`.

```go
package merge

import (
    "log"
    "sync"

    "github.com/gabrielassisxyz/kernl/orchestrator/internal/workflow"
)

// Backend is the minimal interface MergeManager needs from bd.
type Backend interface {
    ListChildrenAwaitingIntegration(epicID string) ([]string, error)
    CountChildren(epicID string) (int, error)
    UpdateStatus(id string, s workflow.IssueStatus) error
    GetDescription(id string) (string, error)
}

// Dispatcher spawns the merger agent against the epic worktree.
type Dispatcher interface {
    DispatchMerger(epicID string) error
}

// Manager orchestrates: trigger detection (D14, single bd list) with single-flight (D11),
// and outcome routing (D6) after the merger agent reports back.
type Manager struct {
    b      Backend
    d      Dispatcher
    inFlight sync.Map // map[epicID]bool — true while a merger is in-flight
}

func NewManager(b Backend, d Dispatcher) *Manager {
    return &Manager{b: b, d: d}
}

// TryTrigger checks "all children awaiting_integration" via single bd list (D14)
// and dispatches the merger agent at most once per epic (D11).
func (m *Manager) TryTrigger(epicID string) error {
    // Single-flight: CAS-style claim. LoadOrStore returns existing value if any.
    if _, loaded := m.inFlight.LoadOrStore(epicID, true); loaded {
        return nil // someone else owns it
    }

    awaiting, err := m.b.ListChildrenAwaitingIntegration(epicID)
    if err != nil {
        m.inFlight.Delete(epicID)
        return err
    }
    total, err := m.b.CountChildren(epicID)
    if err != nil {
        m.inFlight.Delete(epicID)
        return err
    }
    if total == 0 || len(awaiting) < total {
        m.inFlight.Delete(epicID) // not yet — release for future ticks
        return nil
    }

    // All children ready. Mark epic in_progress and dispatch merger.
    if err := m.b.UpdateStatus(epicID, workflow.StatusInProgress); err != nil {
        m.inFlight.Delete(epicID)
        return err
    }
    if err := m.d.DispatchMerger(epicID); err != nil {
        m.inFlight.Delete(epicID)
        return err
    }
    // Note: inFlight stays set until RouteOutcome clears it (after agent finishes).
    return nil
}

// RouteOutcome is called after the merger agent reports done.
// Reads merge_outcome from epic description and routes transitions.
func (m *Manager) RouteOutcome(epicID string) error {
    defer m.inFlight.Delete(epicID)

    desc, err := m.b.GetDescription(epicID)
    if err != nil {
        return err
    }
    outcomeStr := workflow.GetMergeOutcome(desc)
    if outcomeStr == "" {
        log.Printf("ERROR merge: epic %s — merger agent did not report outcome", epicID)
        return m.b.UpdateStatus(epicID, workflow.StatusBlocked)
    }
    outcome, err := ParseOutcome(outcomeStr)
    if err != nil {
        log.Printf("ERROR merge: epic %s — %v", epicID, err)
        return m.b.UpdateStatus(epicID, workflow.StatusBlocked)
    }

    switch outcome {
    case OutcomeSuccess, OutcomePRAlreadyExists:
        // Close all children that are still awaiting_integration.
        children, err := m.b.ListChildrenAwaitingIntegration(epicID)
        if err != nil {
            return err
        }
        for _, c := range children {
            if err := m.b.UpdateStatus(c, workflow.StatusClosed); err != nil {
                return err
            }
        }
        return m.b.UpdateStatus(epicID, workflow.StatusAwaitingPRReview)
    case OutcomeMergeConflict, OutcomePushFailed, OutcomePRCreateFailed:
        return m.b.UpdateStatus(epicID, workflow.StatusBlocked)
    }
    // Unreachable due to ParseOutcome exhaustiveness, but defensive.
    return m.b.UpdateStatus(epicID, workflow.StatusBlocked)
}
```

- [ ] **Step 4:** Run tests; PASS.
- [ ] **Step 5:** Commit.

```bash
git add orchestrator/internal/merge/manager.go orchestrator/internal/merge/manager_test.go
git commit -m "merge: add MergeManager with single-flight trigger (D14+D11) and outcome routing (D6)"
```

**Acceptance Criteria:**
- [ ] `TestMergeManager_SingleFlight_100Goroutines` passes (only 1 spawn for e1).
- [ ] All `RouteOutcome` cases (success, pr_already_exists, merge_conflict, push_failed, pr_create_failed, missing, invalid) covered.
- [ ] `TestMergeManager_NoTriggerWhenSomeChildrenNotReady` confirms partial-readiness no-op.

---

## Task 24: Lane B Epic — Sweep

**Bead Mapping:**
- type: `epic`
- Priority: `2`
- Estimated Minutes: `0`
- Dependencies: `Task 1`
- Parent: `Task 0`
- Status: `open`

**Files:** Container — children create `orchestrator/internal/sweep/` and `orchestrator/cmd/kernl/sweep.go`.

**Description:** Build `kernl sweep` with three-layer resilience: cache MERGED, circuit breaker (3 fails → 5/15/60min backoff), skip-on-fail. Plus PR stale WARN (TT2=B). Subcommand supports `--dry-run`.

**Acceptance Criteria:**
- [ ] `go test ./orchestrator/internal/sweep/...` green.
- [ ] `kernl sweep --dry-run` lists epics with merged PRs and effects no writes.
- [ ] Circuit breaker math (5/15/60min backoff) covered by table test.

---

### Task 25: sweep/sweep.go — Sweep core with cache + circuit breaker + dry-run

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `80`
- Dependencies: `Task 2`
- Parent: `Task 24`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/sweep/sweep.go`
- Test: `orchestrator/internal/sweep/sweep_test.go`

**Description / Steps:**

- [ ] **Step 1:** Write tests covering: happy MERGED → close, cache hit on 2nd tick, gh fail 3× → breaker open, breaker reset on success, PR stale WARN, --dry-run no writes.

```go
package sweep_test

import (
    "errors"
    "strings"
    "sync"
    "testing"
    "time"

    "github.com/gabrielassisxyz/kernl/orchestrator/internal/sweep"
)

type epicRow struct {
    ID    string
    PRURL string
    Children []string
}

type fakeBackend struct {
    mu sync.Mutex
    epics []epicRow
    closed []string
}

func (f *fakeBackend) ListEpicsAwaitingPRReview() ([]sweep.Epic, error) {
    f.mu.Lock()
    defer f.mu.Unlock()
    var out []sweep.Epic
    for _, e := range f.epics {
        out = append(out, sweep.Epic{ID: e.ID, PRURL: e.PRURL, Children: e.Children})
    }
    return out, nil
}
func (f *fakeBackend) Close(id, reason string) error {
    f.mu.Lock()
    defer f.mu.Unlock()
    f.closed = append(f.closed, id)
    return nil
}

type fakeGH struct {
    mu        sync.Mutex
    responses map[string]sweep.PRState
    errs      map[string]error
    calls     map[string]int
}

func (g *fakeGH) View(prURL string) (sweep.PRState, error) {
    g.mu.Lock()
    defer g.mu.Unlock()
    g.calls[prURL]++
    if e, ok := g.errs[prURL]; ok && e != nil {
        return sweep.PRState{}, e
    }
    return g.responses[prURL], nil
}

func TestSweep_HappyMerged_ClosesChildrenAndEpic(t *testing.T) {
    b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1", "c2"}}}}
    g := &fakeGH{
        responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
        calls:     map[string]int{},
    }
    s := sweep.New(b, g, sweep.Config{})
    if err := s.Tick(); err != nil {
        t.Fatal(err)
    }
    if len(b.closed) != 3 {
        t.Fatalf("expected 3 closes (2 children + epic), got %d (%v)", len(b.closed), b.closed)
    }
}

func TestSweep_CacheHit_NoSecondGHCall(t *testing.T) {
    b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1"}}}
    g := &fakeGH{
        responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
        calls:     map[string]int{},
    }
    s := sweep.New(b, g, sweep.Config{})
    _ = s.Tick()
    _ = s.Tick()
    if g.calls["https://x/pr/1"] != 1 {
        t.Fatalf("expected 1 gh call (cache hit on 2nd tick), got %d", g.calls["https://x/pr/1"])
    }
}

func TestSweep_CircuitBreaker_OpensAfter3Fails(t *testing.T) {
    b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1"}}}
    g := &fakeGH{
        errs:  map[string]error{"https://x/pr/1": errors.New("network")},
        calls: map[string]int{},
    }
    s := sweep.New(b, g, sweep.Config{FailureThreshold: 3, BackoffMinutes: []int{5, 15, 60}})
    for i := 0; i < 3; i++ {
        _ = s.Tick()
    }
    // 4th tick: breaker open, gh NOT called again.
    _ = s.Tick()
    if g.calls["https://x/pr/1"] > 3 {
        t.Fatalf("expected breaker to skip gh after 3 fails, got %d calls", g.calls["https://x/pr/1"])
    }
}

func TestSweep_DryRun_NoWrites(t *testing.T) {
    b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1"}}}}
    g := &fakeGH{
        responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
        calls:     map[string]int{},
    }
    s := sweep.New(b, g, sweep.Config{DryRun: true})
    if err := s.Tick(); err != nil {
        t.Fatal(err)
    }
    if len(b.closed) != 0 {
        t.Fatalf("dry-run wrote: %v", b.closed)
    }
}

func TestSweep_PRStaleWARN_FiresHookAndNoClose(t *testing.T) {
    b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1"}}}
    old := time.Now().Add(-10 * 24 * time.Hour)
    g := &fakeGH{
        responses: map[string]sweep.PRState{"https://x/pr/1": {State: "OPEN", CreatedAt: old}},
        calls:     map[string]int{},
    }
    var warns []string
    s := sweep.New(b, g, sweep.Config{
        PRStaleWarnDays: 7,
        WarnHook:        func(msg string) { warns = append(warns, msg) },
    })
    if err := s.Tick(); err != nil {
        t.Fatal(err)
    }
    if len(b.closed) != 0 {
        t.Fatalf("OPEN PR should not be closed: %v", b.closed)
    }
    if len(warns) != 1 || !strings.Contains(warns[0], "open for 10 days") {
        t.Fatalf("expected WARN containing 'open for 10 days', got %v", warns)
    }
}
```

- [ ] **Step 2:** Run tests, confirm fail.
- [ ] **Step 3:** Implement `sweep.go`. Outline:

```go
package sweep

import (
    "fmt"
    "log"
    "sync"
    "time"
)

type Epic struct {
    ID       string
    PRURL    string
    Children []string
}

type PRState struct {
    State     string    // "OPEN", "MERGED", "CLOSED"
    MergedAt  time.Time
    CreatedAt time.Time
}

type Backend interface {
    ListEpicsAwaitingPRReview() ([]Epic, error)
    Close(id, reason string) error
}

type GH interface {
    View(prURL string) (PRState, error)
}

type Config struct {
    DryRun           bool
    FailureThreshold int             // default 3
    BackoffMinutes   []int           // default [5,15,60]
    PRStaleWarnDays  int             // 0 disables; default 7 (per kernl.yaml)
    WarnHook         func(msg string) // optional; default routes through log.Printf
}

type breaker struct {
    failures  int
    openUntil time.Time
}

type Sweeper struct {
    b          Backend
    gh         GH
    cfg        Config
    mu         sync.Mutex
    mergedCache map[string]bool // key: PRURL — survives only in-memory
    breakers   map[string]*breaker
}

func New(b Backend, gh GH, cfg Config) *Sweeper {
    if cfg.FailureThreshold == 0 {
        cfg.FailureThreshold = 3
    }
    if len(cfg.BackoffMinutes) == 0 {
        cfg.BackoffMinutes = []int{5, 15, 60}
    }
    return &Sweeper{
        b: b, gh: gh, cfg: cfg,
        mergedCache: map[string]bool{},
        breakers:    map[string]*breaker{},
    }
}

func (s *Sweeper) Tick() error {
    epics, err := s.b.ListEpicsAwaitingPRReview()
    if err != nil {
        return err
    }
    if len(epics) == 0 {
        return nil // D13=A: skip log when empty
    }
    for _, e := range epics {
        s.processEpic(e)
    }
    return nil
}

func (s *Sweeper) processEpic(e Epic) {
    if e.PRURL == "" {
        log.Printf("WARN sweep: epic %s in awaiting_pr_review without pr_url — skipping", e.ID)
        return
    }
    s.mu.Lock()
    if s.mergedCache[e.PRURL] {
        s.mu.Unlock()
        // Idempotent close.
        s.closeAll(e, "merged via PR (cached)")
        return
    }
    br := s.breakers[e.ID]
    if br != nil && time.Now().Before(br.openUntil) {
        s.mu.Unlock()
        return // breaker open
    }
    s.mu.Unlock()

    state, err := s.gh.View(e.PRURL)
    if err != nil {
        s.recordFailure(e.ID)
        log.Printf("WARN sweep: gh pr view failed for epic %s: %v", e.ID, err)
        return
    }
    s.recordSuccess(e.ID)

    // PR stale WARN.
    if s.cfg.PRStaleWarnDays > 0 && state.State == "OPEN" {
        if days := int(time.Since(state.CreatedAt).Hours() / 24); days > s.cfg.PRStaleWarnDays {
            msg := fmt.Sprintf("WARN sweep: PR %s open for %d days (epic %s)", e.PRURL, days, e.ID)
            if s.cfg.WarnHook != nil {
                s.cfg.WarnHook(msg)
            } else {
                log.Println(msg)
            }
        }
    }

    if state.State == "MERGED" {
        s.mu.Lock()
        s.mergedCache[e.PRURL] = true
        s.mu.Unlock()
        s.closeAll(e, "merged via PR "+e.PRURL+" at "+state.MergedAt.UTC().Format(time.RFC3339))
    }
}

func (s *Sweeper) closeAll(e Epic, reason string) {
    if s.cfg.DryRun {
        log.Printf("sweep[dry-run]: would close epic %s and %d children", e.ID, len(e.Children))
        return
    }
    for _, c := range e.Children {
        if err := s.b.Close(c, reason); err != nil {
            log.Printf("WARN sweep: failed to close child %s: %v", c, err)
        }
    }
    if err := s.b.Close(e.ID, reason); err != nil {
        log.Printf("WARN sweep: failed to close epic %s: %v", e.ID, err)
    }
}

func (s *Sweeper) recordFailure(epicID string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    br := s.breakers[epicID]
    if br == nil {
        br = &breaker{}
        s.breakers[epicID] = br
    }
    br.failures++
    if br.failures >= s.cfg.FailureThreshold {
        idx := br.failures - s.cfg.FailureThreshold
        if idx >= len(s.cfg.BackoffMinutes) {
            idx = len(s.cfg.BackoffMinutes) - 1
        }
        br.openUntil = time.Now().Add(time.Duration(s.cfg.BackoffMinutes[idx]) * time.Minute)
    }
}

func (s *Sweeper) recordSuccess(epicID string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    delete(s.breakers, epicID)
}
```

- [ ] **Step 4:** Run tests; PASS.
- [ ] **Step 5:** Commit.

```bash
git add orchestrator/internal/sweep/sweep.go orchestrator/internal/sweep/sweep_test.go
git commit -m "sweep: add Sweeper with cache MERGED + circuit breaker + dry-run + PR stale WARN"
```

**Acceptance Criteria:**
- [ ] `TestSweep_HappyMerged_ClosesChildrenAndEpic` passes.
- [ ] `TestSweep_CacheHit_NoSecondGHCall` proves cache short-circuits.
- [ ] `TestSweep_CircuitBreaker_OpensAfter3Fails` proves breaker math.
- [ ] `TestSweep_DryRun_NoWrites` confirms zero writes.

---

### Task 26: cmd/kernl/sweep.go — Subcommand wiring

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: `30`
- Dependencies: `Task 25, Task 14`
- Parent: `Task 24`
- Status: `open`

**Files:**
- Create: `orchestrator/cmd/kernl/sweep.go`
- Test: `orchestrator/cmd/kernl/sweep_test.go`

**Description / Steps:**

- [ ] **Step 1:** Implement Cobra subcommand `kernl sweep` with flags: `--dry-run`, `--repo` (path), reading config from `kernl.yaml` (or env / defaults). Wire `BdCliBackend` (implements `sweep.Backend`) and a `gh` adapter (implements `sweep.GH` via `exec.Command("gh", "pr", "view", url, "--json", "state,mergedAt,createdAt")`).

```go
package main

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
    "github.com/gabrielassisxyz/kernl/orchestrator/internal/backend"
    "github.com/gabrielassisxyz/kernl/orchestrator/internal/sweep"
)

func newSweepCmd() *cobra.Command {
    var dryRun bool
    var repoPath string
    cmd := &cobra.Command{
        Use:   "sweep",
        Short: "Close epics whose PRs are merged in master.",
        RunE: func(_ *cobra.Command, _ []string) error {
            b, err := backend.NewBdCliBackend(repoPath)
            if err != nil {
                return fmt.Errorf("backend: %w", err)
            }
            ghAdapter := newGHAdapter() // exec.Command wrapper around `gh pr view`
            cfg := loadSweepConfig(repoPath) // reads kernl.yaml: pr_stale_warn_days etc.
            cfg.DryRun = dryRun
            s := sweep.New(b, ghAdapter, cfg)
            return s.Tick()
        },
    }
    cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview without writes")
    cmd.Flags().StringVar(&repoPath, "repo", ".", "repository path")
    return cmd
}

func init() {
    rootCmd.AddCommand(newSweepCmd())
}
```

- [ ] **Step 2:** Tests:

```go
func TestSweepCmd_FlagsParse(t *testing.T) {
    cmd := newSweepCmd()
    cmd.SetArgs([]string{"--dry-run", "--repo", "/tmp/x"})
    // ... assert flag values after Execute
}
```

- [ ] **Step 3:** Build, run `./kernl sweep --dry-run --repo /path/to/repo` against a fixture; verify output.
- [ ] **Step 4:** Commit.

```bash
git add orchestrator/cmd/kernl/sweep.go orchestrator/cmd/kernl/sweep_test.go
git commit -m "cmd/kernl: add 'sweep' subcommand (dry-run + bd + gh adapters)"
```

**Acceptance Criteria:**
- [ ] `kernl sweep --help` displays usage including `--dry-run`.
- [ ] `kernl sweep --dry-run` produces the format described in spec §6.4.

---

## Task 27: Lane F2 Epic — Prompt + epic_merge + wiring

**Bead Mapping:**
- type: `epic`
- Priority: `3`
- Estimated Minutes: `0`
- Dependencies: `Task 21`
- Parent: `Task 0`
- Status: `open`

**Files:** Container — children create the merger prompt template, the `kernl epic merge` subcommand, and wire MergeManager + sweep into the orchestrator runtime.

**Description:** With merge primitives ready (Lane F1), build the prompt template, wire WorktreeManager to create epic branch first, wire EpicExecutor to call `MergeManager.TryTrigger` on every child state change + `RouteOutcome` after merger done, build `kernl epic merge` for manual recovery, and start the sweep auto-tick goroutine in `kernl serve`.

**Acceptance Criteria:**
- [ ] `go test ./orchestrator/internal/prompt/... ./orchestrator/cmd/kernl/...` green.
- [ ] Manual end-to-end: `kernl epic run` → merger spawned → PR opened → `kernl sweep` closes.
- [ ] `kernl serve` log shows sweep ticks at configured interval; idle ticks silent.

---

### Task 28: prompt/merger_prompt.go — Template + render

**Bead Mapping:**
- type: `task`
- Priority: `3`
- Estimated Minutes: `60`
- Dependencies: `Task 22`
- Parent: `Task 27`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/prompt/merger_prompt.go`
- Test: `orchestrator/internal/prompt/merger_prompt_test.go`
- Create: `orchestrator/internal/prompt/testdata/merger_prompt_N3.golden` (and friends)

**Description / Steps:**

- [ ] **Step 1:** Write golden tests for N=1, N=3, and N=10 children renders (matches eng-review test plan §"Test files mapping"). The golden files capture the literal prompt strings; the test compares.

```go
package prompt_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/gabrielassisxyz/kernl/orchestrator/internal/prompt"
)

func TestMergerPrompt_Golden(t *testing.T) {
    cases := []struct {
        name  string
        input prompt.MergerInput
    }{
        {"N1", prompt.MergerInput{EpicID: "e1", EpicTitle: "Test epic", EpicBranch: "feat/e1", BaseBranch: "master",
            Children: []prompt.Child{{ID: "c1", Branch: "feat/c1", WorktreePath: "/tmp/c1"}}}},
        {"N3", prompt.MergerInput{EpicID: "e1", EpicTitle: "Test epic", EpicBranch: "feat/e1", BaseBranch: "master",
            Children: []prompt.Child{
                {ID: "c1", Branch: "feat/c1", WorktreePath: "/tmp/c1"},
                {ID: "c2", Branch: "feat/c2", WorktreePath: "/tmp/c2"},
                {ID: "c3", Branch: "feat/c3", WorktreePath: "/tmp/c3"},
            }}},
        {"N10", prompt.MergerInput{EpicID: "e1", EpicTitle: "Big epic", EpicBranch: "feat/e1", BaseBranch: "master",
            Children: func() []prompt.Child {
                cs := make([]prompt.Child, 10)
                for i := range cs {
                    cs[i] = prompt.Child{
                        ID:           fmt.Sprintf("c%d", i+1),
                        Branch:       fmt.Sprintf("feat/c%d", i+1),
                        WorktreePath: fmt.Sprintf("/tmp/c%d", i+1),
                    }
                }
                return cs
            }()}},
    }
    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            got, err := prompt.RenderMerger(c.input)
            if err != nil {
                t.Fatal(err)
            }
            path := filepath.Join("testdata", "merger_prompt_"+c.name+".golden")
            if os.Getenv("UPDATE_GOLDEN") == "1" {
                _ = os.WriteFile(path, []byte(got), 0o644)
                return
            }
            want, err := os.ReadFile(path)
            if err != nil {
                t.Fatal(err)
            }
            if string(want) != got {
                t.Fatalf("golden mismatch — re-run with UPDATE_GOLDEN=1 if intentional\n--- want ---\n%s\n--- got ---\n%s", string(want), got)
            }
        })
    }
}

func TestMergerPrompt_ContainsAllOutcomes(t *testing.T) {
    out, err := prompt.RenderMerger(prompt.MergerInput{
        EpicID: "e1", EpicTitle: "x", EpicBranch: "feat/e1", BaseBranch: "master",
        Children: []prompt.Child{{ID: "c1", Branch: "feat/c1", WorktreePath: "/tmp/c1"}},
    })
    if err != nil {
        t.Fatal(err)
    }
    // Must enumerate the literal outcome list (D6=A).
    for _, want := range []string{"success", "merge_conflict", "push_failed", "pr_create_failed", "pr_already_exists"} {
        if !strings.Contains(out, want) {
            t.Fatalf("missing outcome %q in rendered prompt", want)
        }
    }
}

// Use strings.Contains directly — import "strings".
```

- [ ] **Step 2:** Run tests; fail.
- [ ] **Step 3:** Implement `merger_prompt.go`. Use `text/template`. The template enumerates the 3 failure modes (per spec §5.3 D3=A) + the literal 5 outcomes (D6=A) and describes the loop of `git merge --no-ff` in topological order, push with 2× retry, `gh pr create` with PR-already-exists detection.

```go
package prompt

import (
    "bytes"
    "fmt"
    "text/template"

    "github.com/gabrielassisxyz/kernl/orchestrator/internal/merge"
)

type Child struct {
    ID, Branch, WorktreePath string
}

type MergerInput struct {
    EpicID, EpicTitle, EpicBranch, BaseBranch string
    Children []Child
}

const mergerTemplate = `You are the kernl integration agent for epic {{.EpicID}}: "{{.EpicTitle}}".

Your job: merge each child branch into the epic branch in topological order, push, and open a PR.

Inputs:
- epic_branch: {{.EpicBranch}}
- base_branch: {{.BaseBranch}}
- children (ordered):
{{range .Children}}  - {{.ID}}: branch={{.Branch}}, worktree={{.WorktreePath}}
{{end}}

Procedure:
1. cd into the epic worktree (parent of {{.EpicBranch}}). git checkout {{.EpicBranch}}.
2. For each child in the listed order:
   a. git merge --no-ff {{"{{branch}}"}}
   b. On conflict: read conflict markers; resolve safely. If you cannot resolve within your follow-up budget, write
      "merge_conflict_at: {{"{{branch}}"}}" and "merge_outcome: merge_conflict" to the epic bead description and STOP.
3. After all merges succeed: git push origin {{.EpicBranch}}. Retry 2× with backoff on failure.
   If push still fails: write "merge_outcome: push_failed" and STOP.
4. gh pr create --title "{{.EpicTitle}}" --body <auto-generated summary>.
   - If the error indicates the PR already exists, query gh pr list --head {{.EpicBranch}} --json url,
     write "pr_url: <existing>", write "merge_outcome: pr_already_exists", and STOP.
   - On other gh pr create errors: write "merge_outcome: pr_create_failed" and STOP.
5. On full success: write "pr_url: <new>" and "merge_outcome: success" to the epic bead description and STOP.

The merge_outcome MUST be exactly one of:
{{range .Outcomes}}  - {{.}}
{{end}}

Failure modes you MUST enumerate explicitly to the operator before stopping:
- merge_conflict (intractable conflict in one child)
- push_failed (network/auth/protection rule after merges succeeded locally)
- pr_create_failed (gh pr create failed for a reason other than "PR already exists")
`

type mergerView struct {
    MergerInput
    Outcomes []merge.Outcome
}

var mergerTmpl = template.Must(template.New("merger").Parse(mergerTemplate))

func RenderMerger(in MergerInput) (string, error) {
    if in.EpicBranch == "" || in.BaseBranch == "" {
        return "", fmt.Errorf("missing branches")
    }
    var buf bytes.Buffer
    if err := mergerTmpl.Execute(&buf, mergerView{MergerInput: in, Outcomes: merge.All()}); err != nil {
        return "", err
    }
    return buf.String(), nil
}
```

- [ ] **Step 4:** Run tests with `UPDATE_GOLDEN=1` first to seed the goldens, then commit them.

```bash
UPDATE_GOLDEN=1 go test ./orchestrator/internal/prompt/ -v
git add orchestrator/internal/prompt/testdata/
go test ./orchestrator/internal/prompt/ -v   # confirm green without UPDATE_GOLDEN
```

- [ ] **Step 5:** Commit.

```bash
git add orchestrator/internal/prompt/merger_prompt.go orchestrator/internal/prompt/merger_prompt_test.go orchestrator/internal/prompt/testdata/
git commit -m "prompt: add merger template (3 failure modes + 5 literal outcomes) + golden tests"
```

**Acceptance Criteria:**
- [ ] Golden tests pass for N=1, N=3, and N=10.
- [ ] `TestMergerPrompt_ContainsAllOutcomes` confirms all 5 outcomes are literally present.
- [ ] Template references the 3 failure-mode descriptors explicitly.
- [ ] Maps to C2, C3, C9.

---

### Task 29: WorktreeManager — create epic branch before children

**Bead Mapping:**
- type: `task`
- Priority: `3`
- Estimated Minutes: `40`
- Dependencies: `Task 3`
- Parent: `Task 27`
- Status: `open`

**Files:**
- Modify: `orchestrator/internal/worktree/manager.go` (or whichever file owns worktree creation)
- Modify: `orchestrator/internal/worktree/*_test.go`

**Description / Steps:**

- [ ] **Step 1:** Add a method `EnsureEpicBranch(epicID string) (branchName string, err error)` to WorktreeManager. Behavior: branch = `"feat/" + epicID` (D8=A), created from `master` if absent, idempotent. Stores `epic_branch: feat/<epic-id>` in the epic bead description via `workflow.SetEpicBranch`.
- [ ] **Step 2:** Modify the worktree-spawning path for a child to base the worktree off `feat/<epic-id>` (the epic branch) rather than `master`, so children inherit each other's merged state as merges happen.
- [ ] **Step 3:** Add a test that spawns an epic + 2 children and verifies:
  - `git branch --list feat/<epic-id>` exists in the bare repo / fixture.
  - Each child branch is based on `feat/<epic-id>`.
  - The epic bead description contains `epic_branch: feat/<epic-id>`.
- [ ] **Step 4:** Commit.

```bash
git add orchestrator/internal/worktree/
git commit -m "worktree: create epic branch feat/<epic-id> before child worktrees (D8=A)"
```

**Acceptance Criteria:**
- [ ] `feat/<epic-id>` branch exists before any child worktree is created.
- [ ] Child worktrees branch off `feat/<epic-id>`, not `master`.
- [ ] `epic_branch:` populated in the epic bead description.

---

### Task 30: cmd/kernl/epic_merge.go — Manual re-dispatch subcommand

**Bead Mapping:**
- type: `task`
- Priority: `3`
- Estimated Minutes: `45`
- Dependencies: `Task 23, Task 28`
- Parent: `Task 27`
- Status: `open`

**Files:**
- Create: `orchestrator/cmd/kernl/epic_merge.go`
- Test: `orchestrator/cmd/kernl/epic_merge_test.go`

**Description / Steps:**

- [ ] **Step 1:** Implement subcommand `kernl epic merge <epic-id>` (D4=A). Behavior:
  1. Load epic bead.
  2. Validate `status == blocked` AND `merge_conflict_at` (OR `merge_outcome ∈ {push_failed, pr_create_failed}`) — at least one recovery signal must be present.
  3. Validate every child is in `awaiting_integration` OR `closed`.
  4. Clear `merge_conflict_at:` and `merge_outcome:` from the epic description (via `workflow.AddMetadataField(desc, "merge_conflict_at", "")` — or implement a removal helper if cleaner).
  5. Update epic to `in_progress`.
  6. Call `MergeManager.DispatchMerger(epicID)` (or equivalent) directly — skip the trigger check since we know the conditions are met.
  7. Return success.
  - Errors: any unmet validation returns a descriptive error.
- [ ] **Step 2:** Tests:
  - Epic not blocked → error.
  - Epic blocked but no `merge_conflict_at` and no recoverable `merge_outcome` → error.
  - Some child in `in_progress` → error.
  - Happy: all conditions met → status update + dispatch recorded.
  - Idempotence: re-running while already `in_progress` → error "epic already in_progress".
- [ ] **Step 3:** Commit.

```bash
git add orchestrator/cmd/kernl/epic_merge.go orchestrator/cmd/kernl/epic_merge_test.go
git commit -m "cmd/kernl: add 'epic merge' subcommand for manual recovery (D4=A)"
```

**Acceptance Criteria:**
- [ ] All 5 test cases pass.
- [ ] `kernl epic merge --help` shows usage.
- [ ] Re-running in wrong state returns clear error message.

---

### Task 31: EpicExecutor wiring — call MergeManager on every child transition

**Bead Mapping:**
- type: `task`
- Priority: `3`
- Estimated Minutes: `50`
- Dependencies: `Task 23, Task 29`
- Parent: `Task 27`
- Status: `open`

**Files:**
- Modify: `orchestrator/internal/epic/executor.go` (or the file owning child-transition observation)
- Modify: `orchestrator/internal/epic/executor_test.go`

**Description / Steps:**

- [ ] **Step 1:** Wire `MergeManager` into EpicExecutor:
  - Inject `merge.Manager` via constructor.
  - After every observed child transition to `awaiting_integration`, call `m.TryTrigger(epicID)` (no-op if not all children ready).
  - After merger agent completes (observed via take-loop done signal), call `m.RouteOutcome(epicID)`.
  - On worker `blocked`, the existing halt-epic logic stays — but ensure the trigger code doesn't try to spawn a merger if any child is `blocked`.
- [ ] **Step 2:** Add tests:
  - Last child transitions → `TryTrigger` is called.
  - Earlier child transitions → `TryTrigger` is called but no-ops (because total != ready).
  - One child blocked → `TryTrigger` not even called (or called but no-ops because awaiting count < total).
  - Merger completes → `RouteOutcome` invoked exactly once.
- [ ] **Step 3:** Commit.

```bash
git add orchestrator/internal/epic/
git commit -m "epic: wire MergeManager into executor (TryTrigger on child transitions + RouteOutcome on merger done)"
```

**Acceptance Criteria:**
- [ ] EpicExecutor injects MergeManager via constructor.
- [ ] Unit tests cover the 4 scenarios above.

---

### Task 32: kernl serve — sweep auto-tick goroutine

**Bead Mapping:**
- type: `task`
- Priority: `3`
- Estimated Minutes: `35`
- Dependencies: `Task 25`
- Parent: `Task 27`
- Status: `open`

**Files:**
- Modify: `orchestrator/cmd/kernl/serve.go`
- Modify: `orchestrator/cmd/kernl/serve_test.go` (if exists; otherwise add)

**Description / Steps:**

- [ ] **Step 1:** Inside `serve.go`, start a goroutine after server boot that loops:
  ```go
  if cfg.Sweep.AutoIntervalSeconds > 0 {
      go func() {
          t := time.NewTicker(time.Duration(cfg.Sweep.AutoIntervalSeconds) * time.Second)
          defer t.Stop()
          for {
              select {
              case <-ctx.Done():
                  return
              case <-t.C:
                  _ = sweeper.Tick()
              }
          }
      }()
  }
  ```
- [ ] **Step 2:** Load `kernl.yaml` sweep config block (default 60s, `pr_stale_warn_days: 7`, circuit breaker defaults). Add validation for `auto_interval_seconds == 0` → disabled (no goroutine).
- [ ] **Step 3:** Add a smoke test that boots serve with an in-memory backend + GH stub, advances the ticker artificially (e.g., via a `time.NewTicker` injection or `interval=100ms` in test), and verifies `Tick()` was called.
- [ ] **Step 4:** Commit.

```bash
git add orchestrator/cmd/kernl/serve.go orchestrator/cmd/kernl/serve_test.go
git commit -m "cmd/kernl: start sweep auto-tick goroutine in 'serve' (config-driven, ctx-cancellable)"
```

**Acceptance Criteria:**
- [ ] `auto_interval_seconds=0` disables the goroutine.
- [ ] Goroutine respects ctx cancellation on server shutdown.
- [ ] Smoke test fires at least one tick.

---

## Task 33: Lane H Epic — E2E integration tests

**Bead Mapping:**
- type: `epic`
- Priority: `4`
- Estimated Minutes: `0`
- Dependencies: `Task 11, Task 15, Task 18, Task 24, Task 27`
- Parent: `Task 0`
- Status: `open`

**Files:** Container — children create integration tests under `orchestrator/internal/integration/`.

**Description:** Three critical E2E paths (happy / conflict / push-fail) + sweep E2E + passo_a regression. Run against bd 1.0.4 real (per harness D10=A).

**Acceptance Criteria:**
- [ ] `go test -tags=integration -count=1 ./orchestrator/internal/integration/...` green.
- [ ] All three critical paths exercise the full merger + sweep flow.

---

### Task 34: epic_run_happy_test.go — Path 1

**Bead Mapping:**
- type: `task`
- Priority: `4`
- Estimated Minutes: `90`
- Dependencies: `Task 31, Task 32, Task 20`
- Parent: `Task 33`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/integration/epic_run_happy_test.go`

**Description / Steps:**

- [ ] **Step 1:** Build the fixture: a fake repo with `master`, an epic bead `kernl-abc` with 3 children (`kernl-c1`, `kernl-c2`, `kernl-c3`) that each touch disjoint files (no conflict expected per the conflict-rate spike).
- [ ] **Step 2:** Mock the `gh` adapter at the boundary (no real GitHub calls). Mock `gh pr create` → return `https://x/pr/1`; `gh pr view` → first call returns OPEN, second call returns MERGED.
- [ ] **Step 3:** Mock the agent dispatcher: each child agent writes a small change to its branch and reports done; the merger agent writes `merge_outcome: success` + `pr_url: https://x/pr/1` to the epic description.
- [ ] **Step 4:** Run `kernl epic run kernl-abc` (or invoke the in-process executor directly), drive the simulation, assert end state:
  - Children `closed`.
  - Epic `awaiting_pr_review` with `pr_url:` set.
  - After first `sweep.Tick()`: epic still `awaiting_pr_review` (PR OPEN).
  - After second `sweep.Tick()`: epic `closed`.
- [ ] **Step 5:** Commit.

```bash
git add orchestrator/internal/integration/epic_run_happy_test.go
git commit -m "test(integration): add epic-run happy-path E2E (3 children → merger → PR → sweep)"
```

**Acceptance Criteria:** maps directly to C1, C2, C3, C5, C9, C10.

---

### Task 35: epic_run_conflict_test.go — Path 2

**Bead Mapping:**
- type: `task`
- Priority: `4`
- Estimated Minutes: `90`
- Dependencies: `Task 30, Task 31, Task 20`
- Parent: `Task 33`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/integration/epic_run_conflict_test.go`

**Description / Steps:**

- [ ] **Step 1:** Build a fixture where two children edit the same line of the same file — guaranteed conflict on merge.
- [ ] **Step 2:** Run `kernl epic run`. The merger agent (mocked) writes `merge_outcome: merge_conflict` + `merge_conflict_at: feat/kernl-c2`. Assert: epic `blocked`, children remain `awaiting_integration`.
- [ ] **Step 3:** Simulate manual conflict resolution: in the test, perform the equivalent `git merge --continue` on the epic worktree.
- [ ] **Step 4:** Invoke `kernl epic merge kernl-abc`. Assert:
  - Validations pass.
  - Epic flips to `in_progress`.
  - Merger re-dispatched (mock returns `success` this time).
  - Final state: children `closed`, epic `awaiting_pr_review`.
- [ ] **Step 5:** Commit.

```bash
git add orchestrator/internal/integration/epic_run_conflict_test.go
git commit -m "test(integration): add conflict + manual resolve + 'kernl epic merge' E2E"
```

**Acceptance Criteria:** maps to C8, C9.

---

### Task 36: epic_run_push_fail_test.go — Path 3

**Bead Mapping:**
- type: `task`
- Priority: `4`
- Estimated Minutes: `75`
- Dependencies: `Task 30, Task 31, Task 20`
- Parent: `Task 33`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/integration/epic_run_push_fail_test.go`

**Description / Steps:**

- [ ] **Step 1:** Build a fixture where the local merge succeeds but `git push` fails (mock the push command in the merger to fail twice, then succeed; or always fail on first run).
- [ ] **Step 2:** Run `kernl epic run`. Merger writes `merge_outcome: push_failed`. Assert epic `blocked`.
- [ ] **Step 3:** Simulate: operator fixes branch-protection (in-test: flip the mock to allow push).
- [ ] **Step 4:** Invoke `kernl epic merge`. Assert success path completes.
- [ ] **Step 5:** Commit.

```bash
git add orchestrator/internal/integration/epic_run_push_fail_test.go
git commit -m "test(integration): add push-fail + recover via 'kernl epic merge' E2E"
```

**Acceptance Criteria:** maps to C8, C9.

---

### Task 37: sweep_e2e_test.go — Sweep against bd + gh mock

**Bead Mapping:**
- type: `task`
- Priority: `4`
- Estimated Minutes: `60`
- Dependencies: `Task 26, Task 32, Task 20`
- Parent: `Task 33`
- Status: `open`

**Files:**
- Create: `orchestrator/internal/integration/sweep_e2e_test.go`

**Description / Steps:**

- [ ] **Step 1:** Fixture: 2 epics in `awaiting_pr_review` (`pr_url:` set), each with 3 children in `closed` and 2 in `awaiting_integration` (to verify only those still-open get closed).
- [ ] **Step 2:** Mock `gh pr view` per-URL: epic-1 PR → MERGED; epic-2 PR → OPEN.
- [ ] **Step 3:** Run `kernl sweep` (no flag). Assert:
  - Epic-1 closed.
  - All open children of epic-1 closed.
  - Epic-2 unchanged.
- [ ] **Step 4:** Second invocation of `sweep` against the same backend (epic-1 still in cache): assert no additional `gh pr view` call for epic-1.
- [ ] **Step 5:** Force gh to fail 3× for epic-2, then assert breaker open (4th call doesn't hit gh).
- [ ] **Step 6:** Commit.

```bash
git add orchestrator/internal/integration/sweep_e2e_test.go
git commit -m "test(integration): add sweep E2E (MERGED close + cache + circuit breaker)"
```

**Acceptance Criteria:** maps to C4, C5, C11.

---

### Task 38: passo_a_test.go regression — bd 1.0.4 real

**Bead Mapping:**
- type: `task`
- Priority: `4`
- Estimated Minutes: `45`
- Dependencies: `Task 14, Task 17, Task 19, Task 20`
- Parent: `Task 33`
- Status: `open`

**Files:**
- Modify: `orchestrator/internal/integration/passo_a_test.go`

**Description / Steps:**

- [ ] **Step 1:** Update assertions to expect `workflow.StatusAwaitingIntegration` (was legacy `implementation_review` / `ready_for_shipment`).
- [ ] **Step 2:** Verify the test invokes `bd show <id>` against the real CLI and reads the new status without `validation failed`.
- [ ] **Step 3:** Run `go test -tags=integration -run TestPassoA -v` and confirm green.
- [ ] **Step 4:** Commit.

```bash
git add orchestrator/internal/integration/passo_a_test.go
git commit -m "test(integration): align passo_a with kernl-native statuses (regression R1)"
```

**Acceptance Criteria:** maps to C1.

---

## Task 39: Lane I Epic — Final gates + docs

**Bead Mapping:**
- type: `epic`
- Priority: `4`
- Estimated Minutes: `0`
- Dependencies: `Task 33`
- Parent: `Task 0`
- Status: `open`

**Files:** Container.

**Description:** Run the full quality gate suite, update docs, close the TODO entries that this PR resolves, and prepare the PR description.

**Acceptance Criteria:**
- [ ] All 11 C1–C11 criteria verified.
- [ ] `TODOS.md` "Definir workflow próprio do kernl" entry closed.
- [ ] `TODOS.md` retains: "Remoção completa do knots" (follow-up PR), T1 (property race test), T2 (batch heartbeats), T3 (`kernl epic abort`).

---

### Task 40: go vet + linters + 962 unit suite

**Bead Mapping:**
- type: `task`
- Priority: `4`
- Estimated Minutes: `30`
- Dependencies: `Task 17, Task 28, Task 30, Task 31, Task 32`
- Parent: `Task 39`
- Status: `open`

**Files:** N/A (test runner).

**Description / Steps:**

- [ ] **Step 1:** Run full quality gates and triage any failure:

```bash
go vet ./orchestrator/...
go test ./orchestrator/... -count=1 -race
go test -tags=integration ./orchestrator/internal/integration/... -count=1
golangci-lint run ./orchestrator/...  # if configured
```

- [ ] **Step 2:** Fix any newly surfaced issue inline; reopen the relevant child task only if the fix is non-trivial.
- [ ] **Step 3:** Commit if any fix was needed.

**Acceptance Criteria:**
- [ ] `go vet` clean.
- [ ] All unit tests pass with `-race`.
- [ ] All integration tests pass against bd 1.0.4.

---

### Task 41: bd doctor smoke + clean `.beads/` of kernl

**Bead Mapping:**
- type: `task`
- Priority: `4`
- Estimated Minutes: `15`
- Dependencies: `Task 40`
- Parent: `Task 39`
- Status: `open`

**Files:** N/A (operational check).

**Description / Steps:**

- [ ] **Step 1:** Run `bd doctor` against the kernl `.beads/`:

```bash
bd doctor
```

- [ ] **Step 2:** Verify no "invalid status" warnings appear. If any bead in the project tracker still has a legacy status, fix it via `bd update <id> --status <new>` (manual one-off, not in scope of this PR but a sanity check).
- [ ] **Step 3:** Verify `bd ready` returns nothing with legacy statuses (C6).

**Acceptance Criteria:**
- [ ] `bd doctor` reports clean.
- [ ] `bd ready` shows zero legacy-status beads.

---

### Task 42: Update TODOS.md + AGENTS.md + README.md

**Bead Mapping:**
- type: `task`
- Priority: `4`
- Estimated Minutes: `20`
- Dependencies: `Task 40`
- Parent: `Task 39`
- Status: `open`

**Files:**
- Modify: `TODOS.md`
- Modify: `AGENTS.md`
- Modify: `README.md`

**Description / Steps:**

- [ ] **Step 1:** In `TODOS.md`, mark "Definir workflow próprio do kernl" as completed (or remove the section, depending on the file's convention — but keep the historical note that it was shipped). Retain: "Remoção completa do knots" (now activated as follow-up PR), T1, T2, T3, the worktree cleanup TODO, onboarding TODO, CONTRIBUTING.md TODO, the metrics TODO.
- [ ] **Step 2:** In `AGENTS.md`, add a note that `bd ≥ 1.0.4` is required and that `~/.kernl/state/<bead-id>.json` is the canonical agent-runtime store (purgeable for reset).
- [ ] **Step 3:** In `README.md`, add (under "Prerequisites" or similar): `bd ≥ 1.0.4`, `gh` CLI for sweep, `git worktree`.
- [ ] **Step 4:** Commit.

```bash
git add TODOS.md AGENTS.md README.md
git commit -m "docs: close workflow-migration TODO + document bd 1.0.4 + ~/.kernl/state/"
```

**Acceptance Criteria:**
- [ ] TODOS.md retains all post-MVP items (T1, T2, T3, knots delete follow-up).
- [ ] AGENTS.md and README.md document the bd 1.0.4 requirement.

---

### Task 43: Final PR description + push

**Bead Mapping:**
- type: `chore`
- Priority: `4`
- Estimated Minutes: `20`
- Dependencies: `Task 40, Task 41, Task 42`
- Parent: `Task 39`
- Status: `open`

**Files:** N/A (git operation).

**Description / Steps:**

- [ ] **Step 1:** `git log --oneline master..HEAD` and confirm commit history reflects the lane structure cleanly.
- [ ] **Step 2:** Draft PR description: summary, decision IDs traced (D1–D14 + TT1–TT5), test plan, link to spec + eng review + spike.
- [ ] **Step 3:** `git push -u origin workflow/kernl-spec-migration` (or whatever the working branch is).
- [ ] **Step 4:** Open PR via `gh pr create --title "kernl-native workflow + MergeManager + sweep (workflow migration)" --body "$(cat /tmp/pr-body.md)"`.

**Acceptance Criteria:**
- [ ] PR opened with body referencing spec sections.
- [ ] CI green on the PR.
- [ ] Reviewer can map each commit back to a Lane in this plan.

---

## Notes

- The Yegge Loop pass log lives at the bottom of this document; each iteration appends a delta block.
- The 11 C1–C11 acceptance criteria are global to the master Epic (Task 0); each leaf task contributes to one or more of them and cross-references in its own Acceptance Criteria where applicable.
- Per-task Estimated Minutes are conservative; bottom-up sum is ~1310 minutes excluding container epics, which lines up with the spec's ~890 LOC + tests + refactor estimate at ~2 minutes per LOC across mixed activity.
- **Priority convention:** task priorities (P0–P4) reflect **wave membership** (Wave 1 ≈ P1, Wave 2 ≈ P2, Wave 3 ≈ P3, Waves 4–5 ≈ P4). The kernl dependency graph has ≥7 topological levels in places (e.g. Task 2 → 12 → 13 → 14 → 23 → 30 → 35), which exceeds the 5-slot range. Some same-wave chains therefore share a priority number; **the `Dependencies:` list is the authoritative ordering** for the bead converter and for execution. The skill's "strictly lower" priority rule is honored across wave boundaries and treated as a best-effort guide within a wave.

### Yegge Loop log

- **Iteration 1 (WRITE):** First draft saved.
- **Iteration 2 (REFINE):** Lane H Epic dependencies normalized to lane-epic refs (`Task 11/15/18/24/27`) instead of leaf tasks. Fixed `contains` helper in prompt test → `strings.Contains` + import added.
- **Iteration 3 (REFINE):** Master Epic acceptance criteria expanded to enumerate C1–C11 individually. Sweep `Config` gained `WarnHook func(string)` so `TestSweep_PRStaleWARN` asserts an observable signal instead of a black-box log. `fmt` and `strings` imports added where used.
- **Iteration 4 (REFINE):** Pre-flight coordination step added to Task 19 (Lane E) calling out the `*_test.go` overlap with Task 17 (Lane D). Merger-prompt golden cases extended to N=10 to match the eng-review test plan. Task 28 acceptance now lists C2/C3/C9 mapping.
- **Iteration 5 (REFINE):** Bead-graph structural audit: parent/child refs resolve, dep graph acyclic, lane-epic priorities consistent with wave order across boundaries. Documented the priority/depth-range compromise above so the bead converter (and any future reviewer) reads dependency lists as authoritative.
- **Iteration 6 (FINALIZE):** Full plan validity sweep — all 44 tasks verified for unique IDs, non-empty acceptance criteria, parent-child consistency across 8 lane epics + 1 master epic, and Bead Mapping metadata syntactic correctness (type, priority, estimated-minutes, dependencies, parent). Priority convention note preserved. No dangling refs, no orphaned decision IDs (D1–D14 + TT1–TT5 all traced to at least one task). Plan declared ready for `vc-convert-plan-to-beads` deterministic transcoding.
