# Engineering Review — P0.1 Graph Substrate

**Reviewer:** vc-plan-eng-review (autonomous, decisions taken by reviewer per user instruction)
**Date:** 2026-05-17
**Source:** `docs/2026-05-17-graph-substrate-brainstorm-spec.md`
**Mode:** non-interactive — decisions baked into the review; user to challenge in follow-up if needed

---

## Step 0 — Scope challenge

### What already exists

| Sub-problem | Existing code | Reuse? |
|---|---|---|
| SQLite open + WAL pragma | `orchestrator/internal/runstate/store.go` (~80 LOC) | **No, do not reuse** — runstate's error format (`KERNL DISPATCH FAILURE: ... — cause: ... — Fix: ...`) is over-engineered theater. Replicating it propagates a wart. The graph package should establish a sane convention; runstate can converge later. |
| UUID generation | `github.com/google/uuid v1.6.0` (transitively present) | **Reuse** — `uuid.NewV7()` is in v1.6+. No new dep needed. |
| Hermetic SQLite test pattern | `runstate/store_test.go` (54 LOC) | **Pattern-reuse** — same shape (`t.TempDir`, `t.Cleanup`), not the code. |
| Migrations | None. runstate uses `CREATE TABLE IF NOT EXISTS` inline. | **N/A** — graph substrate needs real migrations. |

### Complexity check
File count: ~25 files (12 type files + edges + tags + revisions + fts + sqlite + ids + testutil + 3 schema migrations + graph.go). **Triggers the 8-file smell.** Most of the bulk is the 12 per-type CRUD files (~720 LOC of near-duplication). Decision 2 below collapses this materially.

### Layer annotation
- `modernc.org/sqlite` — **[Layer 1]** tried and true, already in `go.sum`.
- `google/uuid` UUIDv7 — **[Layer 1]** standard.
- `golang-migrate/migrate/v4` — **[Layer 2]** popular but a heavy transitive-dep tree for what we need. See Decision 3.
- Polymorphic `nodes` table with closed type enum + JSON `attrs` — **[Layer 3 / EUREKA]** correct call. The "one table per type" alternative would multiply schema and lose cross-type FTS/queries.
- Per-mutation chokepoint writes revision + FTS in the same tx — **[Layer 3]** correct. Structural impossibility-of-bypass is the right shape for an audit substrate.

### TODOS.md cross-reference
`TODOS.md` is 14.9 KB of orchestrator-domain items; nothing in it blocks P0.1 or asks for graph substrate work. Reverse: P0.1 will *enable* several deferred orchestrator items (e.g., richer bead↔note linking) but those stay deferred per the spec.

### Completeness check
The spec proposes the *complete* version (12 typed CRUD, full audit log, FTS, property tests). No shortcut to flag. Good.

### Distribution check
Not introducing a new artifact — this is an internal Go package. CI must add: `go build ./...`, `go test ./internal/graph/...`, migration up/down/up round-trip in CI. No release pipeline change.

### Scope verdict
**Proceed as scoped, with the changes from Decisions 3, 4, 8 below + the critical schema fix in §3.** Net result: file count is roughly as spec (~25 files — per-type pattern preserved per VISION §14 DevEx); dependency count drops by ~30 (no golang-migrate, but designed for trivial future revert); schema loses one table (`nodes_fts_map`) and replaces the cascade on `revisions.node_id` to preserve audit data.

**Initial overrides reverted after VISION re-reading:**
- Per-type CRUD (12 files): kept as spec — contributor approachability per VISION §14.
- Diff storage on revisions: kept as spec — fidelity per VISION §7.2 (revisions show what user saw at the time).

---

## 1. Architecture Review

### Issue 1: Repo reorganization — single root module vs status quo
**Spec:** Promote to single root module (`module github.com/<owner>/kernl`), move `orchestrator/cmd → cmd/`, `orchestrator/internal → internal/orchestrator/`.

**Decision: ADOPT (Option A) — single root module before any schema work.**

| Option | Pros | Cons |
|---|---|---|
| **A. Single root module (spec)** | Clean `kernl/internal/graph` imports from any binary. Right shape for future `kernl`, `kernl-orchestrator`, `kernl-da` binaries. | Large mechanical PR. Risk of breaking CI/`scripts/swarm`, embeds, hardcoded paths. ~2–4h effort. |
| B. Keep multi-module, add separate `kernl-graph` module | Smaller blast radius. | `replace` directives or published versioning forever. Awkward for an internal substrate that every binary needs. |
| C. Put `internal/graph` inside `orchestrator/` | Zero restructure. | Encodes that orchestrator owns the substrate. Wrong direction — substrate must outlive any one binary. |

**Why A:** the project will have multiple binaries sharing one substrate. The cost of restructuring grows ~10× per binary added. Do it once, now, while there's still only one Go module.

**[Layer 1]** standard Go monorepo layout.

---

### Issue 2: Per-type CRUD duplication — 12 files × ~60 LOC
**Spec:** Each node type gets its own file with hand-written `CreateXxx / GetXxx / UpdateXxx / DeleteXxx / ListXxx`.

**Decision: AGREE WITH SPEC (Option A) — keep 12 per-type files.**

| Option | Pros | Cons |
|---|---|---|
| **A. Spec — 12 hand-written files (~720 LOC)** | Most explicit at call site. Contributor opens `internal/graph/nodes/` and sees one file per type — the full ontology at a glance. Natural home for future per-type business logic (e.g., URL normalization in `bookmark.go`). Adding a new type = "copy `bead.go`, rename, modify" — 5-minute task, zero learning curve. | DRY violation. 12 places where the chokepoint hookup could be skipped (mitigated by making `Tx` interfaces unimplementable outside the package — see Issue 6 below). Adding a 13th type touches ~60 LOC. |
| B. Generic Create/Get/Update/Delete + per-type List+Filter | DRY. Single chokepoint enforced by the type system. ~250 LOC total. | Requires understanding `NodeSpec[T]` interface and registration before contributing. Less debugger-friendly (jumps land in generic code, not type-specific). |
| C. Code generation from a `types.yaml` | Single source of truth. | New build step, generated files harder to navigate. Premature for 12 types. |

**Why A (reverted from initial override):** VISION §14 makes DevEx a non-negotiable pillar — that applies to contributors, not just users. The type set is closed and Kernl-defined (VISION §6.1) — growth is bounded (~12 today, ~15–18 lifetime). 720 LOC of explicit-but-clear code is preferable to 250 LOC requiring upfront comprehension of a generic abstraction. The chokepoint guarantee can be enforced structurally without generics: make the `Tx` interfaces have an unexported method so no outside package can fabricate a fake `Tx` to bypass the chokepoint.

**Mitigation for the original DRY concern:** add a compile-fail testdata file (TODO #2) that proves no outside package can construct a `Tx`. This closes the "12 places to forget the chokepoint" risk without sacrificing the per-type file structure.

---

### Issue 3: Migrations runner — golang-migrate vs hand-rolled
**Spec:** `github.com/golang-migrate/migrate/v4` + sqlite driver + `go:embed schema/*.sql`.

**Decision: ADOPT Option C — hand-roll the migrator (~85 LOC), designed for trivial future revert to golang-migrate.**

| Option | Pros | Cons |
|---|---|---|
| A. golang-migrate v4 (spec) | Battle-tested. Mature lock handling for concurrent migrators. | ~30 transitive deps for one-process single-user SQLite. Solves a more general problem than we have. |
| B. pressly/goose | Simpler API than migrate. | Still a dep tree. |
| **C. Hand-rolled (~85 LOC), golang-migrate-compatible** | Zero new deps. Read embedded `.sql` in order, apply if not in `schema_migrations`. Easy to read. | Have to write up/down/up CI test ourselves (spec requires anyway). |

**Why C:** [Layer 3] migrate/goose solve "the DB might be Postgres, MySQL, Cassandra; users might want CLI-driven migrations; teams want roll-forward and roll-back operationally across distributed processes." Kernl is single-user single-process SQLite with `Open` applying migrations automatically. The standard solution is overkill; ~30 transitive deps for a new package is meaningful in an open-source project where dep audits influence trust.

**Reversibility contract (mandatory in implementation):** the hand-rolled migrator MUST use the same tracking-table schema and file-naming convention as golang-migrate v4, so reverting later is ~1 hour of work (swap ~85 LOC for ~10 LOC; golang-migrate adopts the existing tracking table without data migration).

```sql
-- Use golang-migrate's exact schema, NOT a custom one:
CREATE TABLE schema_migrations (
  version BIGINT NOT NULL PRIMARY KEY,
  dirty   BOOLEAN NOT NULL DEFAULT FALSE
);
```

File naming: `NNNN_name.up.sql` / `NNNN_name.down.sql` (spec already uses this — keep it).

**Future revert cost (if you change your mind):** trivial. `go get github.com/golang-migrate/migrate/v4` → delete the ~85 LOC → replace with `migrate.New(...).Up()`. golang-migrate reads our tracking table and continues from where we left off. Zero migration of existing data.

---

### Issue 4: FTS5 indirection — contentless table + `nodes_fts_map` mapping table
**Spec:** `nodes_fts` is contentless; a separate `nodes_fts_map(fts_rowid INTEGER PK AUTOINCREMENT, node_id TEXT UNIQUE)` maps UUID → FTS5 rowid. Every search joins.

**Decision: ADOPT Option C — drop the map table; store `fts_rowid INTEGER` directly on `nodes`.**

| Option | Pros | Cons |
|---|---|---|
| A. Spec — separate map table | Clean separation. | Extra table, extra index, an extra join on every search hit, two writes on every Create. |
| B. FTS5 external-content (`content='nodes'`) | No duplication of text. | Requires body to be a real column on `nodes`. Body lives in `attrs` JSON for notes — JSON1 → FTS5 external-content is awkward and brittle. |
| **C. `fts_rowid INTEGER UNIQUE` column on `nodes`** | Same write count as A, one less table, one less join, one less index. Trivial to drop later if needs change. | Couples `nodes` schema to FTS5 rowid concept (acceptable — FTS5 is the chosen search engine, not a swappable layer). |

**Why C:** the map table is solving a problem (FTS5 wants INTEGER rowids; node IDs are TEXT UUIDs) that's cheaper to solve with a column. Search-path becomes `SELECT id, type, title FROM nodes WHERE fts_rowid IN (SELECT rowid FROM nodes_fts WHERE nodes_fts MATCH ?)` — one join, no detour through a third table.

---

### Issue 5: Edge audit log
**Spec:** No `edge_events`. To change an edge, delete + create.

**Decision: AGREE WITH SPEC.** YAGNI. Edge mutations are rare; if audit becomes a real need it's an additive table later. Confirmed.

---

### Issue 6: Failure scenario for the chokepoint
For each chokepoint-driven mutation, one realistic production failure:

| Codepath | Failure | Handled? |
|---|---|---|
| `updateNode` mid-tx: disk full | tx aborts, mutation rolls back cleanly | ✅ SQLite atomicity |
| `createNode` with body containing FTS5 special chars (`"`, `*`, `^`) | Insert succeeds; search later with naive query may fail. | ⚠️ Need test — see Test §4 |
| `deleteNode` while another tx reads outgoing edges | Reader sees pre-delete snapshot (WAL); cascading delete on commit. | ✅ WAL |
| Two writers race for the same UpdateBead | Per-connection serialization via SetMaxOpenConns(1) on write pool (Decision 9) | ✅ |
| Process killed during `Open` after some migrations applied | Next `Open` resumes from `schema_migrations`. | ✅ |
| Process killed mid-tx in `updateNode` | WAL replay; either full tx or nothing. revisions stays consistent with nodes. | ✅ |

Critical: the spec's claim "no public API path mutates a node without writing a revision" must be enforced by package boundaries. Make `Tx` interfaces unimplementable outside `internal/graph` (private method), so a downstream package can't fabricate a fake Tx and bypass the chokepoint.

---

### Issue 7: Diff storage on revisions
**Spec:** Diff column populated only when type implements `DiffableNode`; otherwise NULL.

**Decision: AGREE WITH SPEC (Option A) — store diff at write time for diffable types.**

| Option | Pros | Cons |
|---|---|---|
| **A. Spec — store diff at write time for diffable types** | Cristallizes the diff as the user saw it at the time. Survives future changes to the diff algorithm (unified → syntax-aware) without revisions retroactively showing different deltas. Read-side instant for "DA wrote here" ribbon (VISION §7.2). | Small write-time cost; small storage cost. Both negligible for a write rate of ~1 revision per 5s per note (P0.2). |
| B. Snapshot only; derive diff on-demand | One less column. | If the diff algorithm changes later, old revisions show diffs the user never saw. Loses fidelity for a substrate where audit trail is sacred (VISION §7.2, §7.3). |

**Why A (reverted from initial override):** the optimization that motivated B (cheaper writes, less storage) doesn't matter at the write rate VISION specifies (5s auto-save on notes). The fidelity argument is real: time-travel through revisions should show what the user saw, not what today's diff algorithm produces. Storing the diff is a tiny cost that buys permanence.

---

### Issue 8: WriteTx serialization
**Spec:** `sync.Mutex` inside `*Graph` to serialize `WriteTx` acquisition; "belt and suspenders" over `busy_timeout=5000ms`.

**Decision: ADOPT Option C — dual-handle pattern (read pool, write pool sized to 1). No application mutex.**

| Option | Pros | Cons |
|---|---|---|
| A. Spec — Go mutex + busy_timeout | Works. | Mutex is in application space; the SQL driver pool is the right serialization layer. Doubles the locking surface. |
| B. busy_timeout only | Simple. | `SQLITE_BUSY` retries are observable as latency spikes; cleaner to never let it happen. |
| **C. Two `*sql.DB` handles: read pool (default size), write pool with `SetMaxOpenConns(1)`** | Pool naturally serializes writes. Reads scale freely. Standard modernc.org/sqlite pattern. No app-level locks. | Two handles instead of one. ReadTx uses read pool, WriteTx uses write pool — type-distinct interfaces still hold. |

**Why C:** the database/sql connection pool is the canonical serialization point. Mutexes layered on top of it are a code smell. Bonus: runstate can later adopt the same pattern (it currently uses a single handle).

---

### Architecture summary
8 issues raised. **5 adopted with changes** (single root module, hand-rolled migrator, `fts_rowid` column, dual-handle pool, schema correction for revisions cascade). **3 agreed with spec** (per-type CRUD, edge audit log, diff at write time). The schema is materially correct after the cascade fix; the changes reduce dependencies and one schema table without reducing capability.

---

## 2. Code Quality Review

### Issue 9: Error formatting convention
**Observed:** `runstate/store.go` uses `fmt.Errorf("KERNL DISPATCH FAILURE: open sqlite: %w — cause: %v — Fix: verify path is writable", err, err)`. Verbose, theatrical, duplicates `err` twice (the `%v` after `%w` is redundant). The spec doesn't specify a convention; the risk is the new package inherits this by osmosis.

**Decision: ADOPT Option B — idiomatic Go errors `graph: <op>: %w` + sentinel errors.**

| Option | Pros | Cons |
|---|---|---|
| A. Match runstate's "KERNL DISPATCH FAILURE: ... — Fix: ..." | Consistent with one prior package. | Format is hostile to `errors.Is/As`. Fix-hints belong in `kernl doctor`, not in every error string. The `%v` after `%w` is a bug pattern. |
| **B. `fmt.Errorf("graph: <op>: %w", err)` with sentinel errors for known failure classes** | Standard Go. Composes with `errors.Is`. Quiet. CLI/UI layers compose fix-hints by inspecting wrapped sentinels. | Diverges from runstate. |

**Why B:** `errors.Is/As` composability is a real capability, not a style preference — modules that catch specific failure classes (e.g., "FTS query syntax error" → show user-friendly message, vs. "DB locked" → retry) need typed errors. Fix-hints belong in `kernl doctor` (VISION §14) where they can use the user's actual state, not in static error strings. Convergence path: runstate aligns to this convention as part of TODO #3 in a separate cleanup PR.

**Sentinel errors to define in P0.1:**
- `graph.ErrNotFound` — Get/Update/Delete of nonexistent node
- `graph.ErrFTSQuerySyntax` — wraps FTS5 parse errors (UI shows clear message)
- `graph.ErrSchemaLocked` — concurrent migration attempt
- `graph.ErrAuthorRequired` — empty Author passed to chokepoint

---

### Issue 10: `Author` validation
**Spec:** `type Author string` + `AuthorAgent(id string) Author { return Author("agent:" + id) }`. Permits `Author("anything")`.

**Decision: AGREE WITH SPEC (Option A).** Single-user system; Author is presentation metadata for the "DA wrote here" ribbon. Sealed-enum + ID-format validation buys little, and invalid data manifests as literal display, not security failure. No change.

---

### Issue 11: Stale-diagram maintenance
The spec includes one ASCII diagram (the package layout in §5). Inside the implementation, the chokepoint flow in §7.4 deserves an inline ASCII comment in `internal/graph/graph.go` (or wherever `updateNode` lives), and **must be updated when the chokepoint logic changes.** Flagged for inclusion in the plan as a maintenance requirement.

---

### Code quality summary
2 issues raised, 1 noted. The package layout is clean. Main risk is replicating runstate's error format — addressed by Decision 9.

---

## 3. Test Review

### Test Framework Detection
- **Runtime:** Go (`go.mod` present in `orchestrator/`; will move to root post-reorg).
- **Framework:** stdlib `testing` + `t.TempDir` / `t.Cleanup`. Already in use in `runstate/store_test.go`.
- **Property tests:** Recommend adopting `pgregory.net/rapid` (shrinking; better than `testing/quick`). New dep, small, no transitive baggage.

### Coverage diagram

```
CODE PATHS                                                  USER FLOWS (test scenarios)
[+] internal/graph/graph.go                                 [+] Bead lifecycle
  ├── Open(cfg)                                               ├── [★★★ PLAN] Create → Get → Update → Delete
  │   ├── [★★★ PLAN] WAL + foreign_keys pragmas applied      └── [★★  PLAN] List with filter (type, status)
  │   ├── [★★★ PLAN] Migrations apply idempotently           [+] Note + FTS
  │   ├── [★★  PLAN] InMemory mode (journal_mode=MEMORY)       ├── [★★★ PLAN] Create with body → Search finds it
  │   └── [GAP]      Corrupt schema_migrations row → error    ├── [★★★ PLAN] Update body → old word gone, new found
  ├── ReadTx / WriteTx                                         ├── [GAP]      FTS5 special chars in query (", *, ^) [→E2E]
  │   ├── [★★★ PLAN] Type-distinct interfaces compile-checked ├── [GAP]      Unicode + diacritics (Portuguese)
  │   └── [GAP]      Write pool serializes concurrent writes  └── [GAP]      Empty query / single char / max length
  └── DoRead / DoWrite                                       [+] Edges
      └── [★★  PLAN] Single-shot helpers                       ├── [★★★ PLAN] Create → Outgoing → Delete cascade
                                                                ├── [GAP]      Self-edge (src == dst) — allowed?
[+] internal/graph/nodes/                                       └── [GAP]      Edge between deleted node → FK error
  ├── chokepoint updateNode                                  [+] Revisions
  │   ├── [★★★ PLAN] Writes revision in same tx               ├── [★★★ PLAN] Exactly one row per mutation
  │   ├── [★★★ PLAN] Reconciles node_tags (add/remove)        ├── [★★  PLAN] Delete writes tombstone before cascade
  │   ├── [★★  PLAN] Updates FTS in same tx                   ├── [GAP]      prev_revision_id chain unbroken
  │   ├── [GAP]      Author=empty string rejected             └── [GAP]      Tombstone author preserved post-delete
  │   └── [GAP]      Bypass attempt (fake Tx) — compile-fail [+] Persistence
  └── per-type filters                                         ├── [★★★ PLAN] Close → re-Open → data persists
                                                               └── [★★  PLAN] Migration up → down → up round-trip [CI]
[+] internal/graph/fts/
  └── Search                                                 [+] Concurrency
      ├── [★★  PLAN] Returns hits ordered by bm25              ├── [GAP]      Two concurrent WriteTx serialize cleanly
      ├── [GAP]      WithTypes filter                           └── [GAP]      Reader sees pre-delete snapshot under WAL
      └── [GAP]      WithTagsFilter
                                                              [+] Identity
[+] internal/graph/edges/                                      ├── [★★  PLAN] UUIDv7 monotonic within a process
  ├── Create / Delete                                          └── [GAP]      UUIDv7 sortable by created_at
  └── Outgoing / Incoming                                                     (proves index locality claim)
      └── [★★★ PLAN] Filter by edge type

[+] Property tests (rapid)
  ├── [★★★ PLAN] 1000 random ops preserve all invariants
  │   ├── exactly one revision per mutation
  │   ├── FTS reflects current state (no stale)
  │   ├── ON DELETE CASCADE removes edges/tags/fts/revisions
  │   └── UUIDv7 monotone

COVERAGE: 17/30 paths planned (57%)
GAPS: 13 — none are regressions (this is greenfield), all are completeness.
```

Legend: ★★★ behavior + edge + error  |  ★★ happy path  |  ★ smoke  |  [→E2E] integration test  |  PLAN = listed in spec §10/§11  |  GAP = missing from spec

### Required additions to the plan (gaps → tests)

1. **FTS5 special-character safety:** insert a note with body containing `"`, `*`, `^`, `~`, `:`. Search with each character escaped. Asserts no panic, expected hits. Required.
2. **Unicode + diacritics (Portuguese):** "ação", "coração" → search "acao" returns both with `remove_diacritics 2`. Required — the spec explicitly chose this tokenizer for Portuguese.
3. **Empty / single-char / max-length FTS queries:** documented behavior (likely: empty → error; single char → may return nothing; max length → still works). Required.
4. **`WithTypes` and `WithTagsFilter` option tests:** one per option. Required.
5. **Self-edge policy:** decide and test. Recommendation: allow at the schema level (no CHECK), document that callers should validate if undesired. Test: `Create(src=X, dst=X, type=relates_to)` succeeds.
6. **Edge against missing node:** `Create(src=X, dst=NONEXISTENT)` must fail with FK error (proves `foreign_keys=ON` is actually on).
7. **`prev_revision_id` chain unbroken:** after N updates, walking back from latest revision via `prev_revision_id` visits all N revisions in order. Required for time-travel.
8. **Tombstone post-delete:** delete a node; query `revisions` directly (revisions cascade with the node — so the tombstone test must read the revision *before* the deletion commits, OR keep revisions outside the cascade. **Schema question raised below.**)
9. **Author=empty string rejected at the chokepoint** (not the DB — empty `Author` is a programmer error).
10. **Compile-fail test:** a `testdata/` file that, if `go vet`'d, would attempt to construct a `Tx` outside the package. CI runs `go vet ./...` and asserts the build error. Optional but cheap.
11. **Concurrent WriteTx serialization:** two goroutines call `DoWrite` simultaneously; both succeed without `SQLITE_BUSY`; mutation ordering is consistent.
12. **Reader sees pre-delete snapshot under WAL:** start a read tx, in another goroutine delete a node, read tx still sees the node. Confirms WAL isolation.
13. **UUIDv7 sortable by created_at:** generate 100 IDs across a 100ms wall-clock spread; sorted-by-ID equals sorted-by-CreatedAt.

### Schema correction surfaced by Issue 8
The spec has `revisions.node_id REFERENCES nodes(id) ON DELETE CASCADE`. Combined with "delete writes a tombstone revision *before* deleting" — the tombstone is written, then the node is deleted, then the cascade wipes the tombstone (and all prior revisions). **The audit log does not survive the delete as the spec claims.**

**Decision: change schema** so revisions do NOT cascade-delete with the node. Either:
- (a) drop `ON DELETE CASCADE` on `revisions.node_id` (revisions become orphaned but preserved); or
- (b) add a `deleted_at INTEGER` column to `nodes` for tombstone-as-soft-delete on the node itself, no cascade.

**Picked: (a).** Soft delete on the master table (b) contaminates every query with `WHERE deleted_at IS NULL` forever — exactly the trap Decision 11 of the original spec correctly avoids. Orphaned revisions are fine because the snapshot contains the node's data; the FK constraint can be dropped on `revisions.node_id` (or replaced with `ON DELETE SET NULL` if you want the link to remain when the node still exists). Add an explicit test: "Delete node → revisions table still has all prior rows + tombstone."

This is **critical** — left unfixed, the substrate silently loses audit data on every delete.

### Test plan artifact
Written separately to `docs/reviews/vc-plan-eng-review-test-plan-2026-05-17.md`.

---

## 4. Performance Review

### Issue 12: FTS5 join cost (resolved by Issue 4)
Spec's join through `nodes_fts_map` adds a lookup per search hit. Resolved by Issue 4 — `fts_rowid` lives on `nodes`.

### Issue 13: `List<Type>(filter)` over `nodes` table
The composite index `nodes_type_updated_idx ON nodes(type, updated_at DESC)` is correct for "latest N of type X". Filter on `attrs.status` requires JSON1 extraction (`json_extract(attrs, '$.status')`) — **not indexed**. For tables with many beads, a `bead.status = 'open'` filter scans all beads of type=bead.

**Decision: defer expression indexes to a profile-driven addition.** For v1, accept the scan.

**Concrete trigger criteria for adding an expression index later (not "someday"):**
- A specific `json_extract(attrs, '$.X')` filter appears ≥5 times in the codebase (signal of hot query), OR
- Profiling shows a query >50ms in realistic volume (~10k nodes of the relevant type).

When a criterion fires, the index is a one-line migration:
```sql
CREATE INDEX bead_status_idx ON nodes(json_extract(attrs, '$.status')) WHERE type='bead';
```

Likely candidates (documented in TODOS for visibility, not implemented now): `bead.status`, `task.priority`, `bookmark.archived_at`.

**Why defer:** for ~10k nodes on local SQLite, full-type scans complete sub-millisecond. Adding indexes preemptively costs write-time work and storage for queries that may never become hot. Documenting concrete triggers (not vague "if it becomes a problem") makes the deferral honest — the team knows exactly when to revisit.

### Issue 14: Revision history reads
`revisions_node_ts_idx ON revisions(node_id, ts DESC)` — correct for "history of node X". No issue.

### Issue 15: Chokepoint read-before-write cost
`updateNode` does `SELECT current` before `UPDATE`. One extra round-trip per write. For SQLite on local disk this is sub-millisecond. Acceptable. Could be optimized later via `UPDATE ... RETURNING` if hot, but that needs SQLite 3.35+ (modernc.org/sqlite is fine here). **Defer.**

### Performance summary
3 issues evaluated, 1 resolved by architectural change, 2 deferred with documented escape hatches. No P0 perf concerns.

---

## NOT in scope (consolidated)

Lifted from spec §3 + reviewer additions:

| Item | Rationale |
|---|---|
| Filesystem watcher, UUID injection, path↔uuid cache | P0.2 |
| 5-second auto-diff writes for notes | P0.2 (writer of revisions for notes) |
| 4-signal relevance, Adamic-Adar, Louvain | P0.3 |
| `inferred_edges` cache table | P0.3 |
| Additive-only write contract for MemoryClaim | P2.2 (module-enforced, not substrate) |
| Defuddle / bookmark extraction | P2.3 |
| Backup / restore tooling | DevEx later (file copy works for now) |
| Multi-user auth | Not now (§16) |
| HTTP API exposing the substrate | P1.1 (DA), P2.6 (GUI) |
| Expression indexes on `attrs.*` paths | Profile-driven, not v1 |
| `edge_events` audit log | Add additively if needed |
| Cross-package compile-fail test for Tx forgery | Optional, can ship later |

---

## What already exists

| Sub-problem | Existing | Plan reuses? |
|---|---|---|
| Pure-Go SQLite driver | `modernc.org/sqlite` in `orchestrator/go.sum` | Yes |
| UUID v6 lib supporting v7 | `github.com/google/uuid v1.6.0` (indirect → make direct) | Yes |
| Hermetic test pattern with `t.TempDir`/`t.Cleanup` | `runstate/store_test.go` | Pattern, not code |
| WAL + sqlite open boilerplate | `runstate/store.go` Open() | Rewrite cleanly — do not inherit the verbose error format |

---

## TODOS.md updates

Three items proposed (decisions taken by reviewer; flag in report for user review):

1. **Expression indexes on common `attrs.*` filters** — *Why:* future scale (bead.status, task.priority filters do table scans today). *Add as TODO* with profile-driven trigger.
2. **Cross-package compile-fail test for `Tx` forgery** — *Why:* hardens the chokepoint invariant. *Add as TODO* — not blocking v1 since the unexported method already prevents implementation.
3. **Converge runstate error format with the new `graph: op: err` convention** — *Why:* consistency across the project; current runstate format is hostile to `errors.Is/As`. *Add as TODO* — separate cleanup PR.

---

## Failure modes (for each new codepath)

| Codepath | Realistic failure | Test? | Handler? | User sees? |
|---|---|---|---|---|
| `Open` with locked WAL file | `SQLITE_BUSY` on init | NO (add) | Yes (busy_timeout) | Slow open; eventually OK |
| `updateNode` mid-tx process kill | Partial state | Implicit (WAL replay) | Yes (atomic tx) | Nothing — recovers on next Open |
| `Search` with malformed FTS5 query | Parse error from FTS5 | **NO** (add — gap #1) | NO (caller sees raw error) | **Cryptic error** — critical UX gap; wrap in `graph: search: query syntax: %w` |
| `CreateEdge` with non-existent dst_node_id | FK violation | **NO** (add — gap #6) | Yes (FK on) | Error returned |
| `DeleteNode` losing revision history | **Schema bug** — see Test §schema correction | YES (mandatory regression-style test) | After fix: cascade dropped | After fix: history preserved |
| Concurrent WriteTx | `SQLITE_BUSY` | **NO** (add — gap #11) | After Decision 8: pool serializes | Nothing — invisible |

**Critical gap (silent + no test + no handler):** the **revisions cascade-delete bug** above. The substrate would silently lose audit data the first time a node is deleted. Must be fixed in P0.1, not deferred.

**Secondary gap:** malformed FTS5 query → cryptic error. Cheap to fix (wrap + document).

---

## Worktree parallelization strategy

P0.1 has internal ordering that constrains parallelism:

| Step | Modules touched | Depends on |
|---|---|---|
| 0. Repo reorganization | entire repo | — |
| 1. SQLite open + pragmas + migrations runner | `internal/graph/internal/sqlite`, `internal/graph/schema` | 0 |
| 2. NodeSpec interface + chokepoint + generic CRUD | `internal/graph`, `internal/graph/nodes` | 1 |
| 3. Per-type NodeSpec impls (12 types) + filters | `internal/graph/nodes/*.go` | 2 |
| 4. Edges API | `internal/graph/edges` | 1 (schema), 2 (Tx types) |
| 5. Tags API | `internal/graph/tags` | 1, 2 |
| 6. FTS Search API | `internal/graph/fts` | 1, 2 |
| 7. Revisions read API | `internal/graph/revisions` | 1, 2 |
| 8. Property tests | `internal/graph/testutil`, all packages | 2–7 |

**Parallel lanes after step 2 lands:**
- Lane A: step 3 (12 type files — internally parallelizable across the 12, but one PR)
- Lane B: steps 4 + 5 + 6 + 7 — independent packages, each can be a separate PR

**Execution order:** 0 → 1 → 2 (sequential, foundational). Then launch A and B in parallel worktrees. Merge both. Then 8.

**Conflict flag:** Lane A and Lane B both register their types/edges/etc with the generic chokepoint — if the chokepoint surface needs to change after step 2 lands, both lanes are affected. Mitigation: nail the `Tx` and `NodeSpec` interfaces in step 2 and freeze them before launching A+B.

---

## Outside voice

Skipped (running autonomously per user instruction). Recommend running once before implementation begins.

---

## Completion summary

- Step 0: Scope Challenge — **scope reduced** (Decisions 3, 4 + schema fix: ~30 deps removed, 1 schema table removed, audit-loss bug fixed). Per-type CRUD and diff-storage kept as spec after VISION re-reading.
- Architecture Review: **8 issues** — 5 changes adopted, 3 agreed-with-spec
- Code Quality Review: **2 issues** — 1 change adopted (error format), 1 agreed-with-spec
- Test Review: diagram produced, **13 gaps** identified + 1 critical schema bug (revisions cascade) to be fixed in P0.1
- Performance Review: **3 issues** evaluated, 1 resolved architecturally (`fts_rowid` column), 2 deferred with concrete trigger criteria
- NOT in scope: written
- What already exists: written
- TODOS.md updates: **3 items** proposed (expression indexes with concrete triggers, Tx-forgery compile-fail test, runstate error-format convergence)
- Failure modes: **1 critical gap** (revisions cascade — fix mandated in plan), 1 secondary (FTS error wrapping — covered by `ErrFTSQuerySyntax` sentinel)
- Outside voice: **skipped** — recommend running before implementation
- Parallelization: **2 parallel lanes** after step 2; sequential 0→1→2 and final 8
- Final decisions on flags: error format = idiomatic `graph: op: %w` + sentinels; expression indexes = defer with concrete triggers (≥5 callers OR >50ms profiled).

## Decision provenance (overrides revisited)

| # | Topic | Initial position | Final position | Why changed |
|---|---|---|---|---|
| 1 | Repo reorg (single root module) | Adopt spec | Adopt spec | — |
| 2 | Per-type CRUD | Override → generics | **Agree with spec** | VISION §14 (contributor DevEx) — explicit per-type files lower contribution friction |
| 3 | Migrator | Override → hand-rolled | **Override** + design for trivial revert (golang-migrate-compatible tracking table) | Single-process SQLite doesn't need migrate/goose's distributed-locking complexity; ~30 deps cost > ~85 LOC cost |
| 4 | `fts_rowid` column vs map table | Override → column | **Override** | One less JOIN per search; schema-purity argument is real but can be addressed by `DROP COLUMN` later |
| 5 | Edge audit log | Agree with spec (defer) | Agree with spec | — |
| 6 | Schema fix: revisions cascade | Critical override | **Override (mandatory)** | Spec has a logical bug — tombstone written then cascade-wiped in same tx |
| 7 | Diff storage | Override → on-demand | **Agree with spec** | VISION §7.2 — revisions should show what the user saw at the time; algorithm changes shouldn't retroactively alter history |
| 8 | WriteTx serialization | Override → dual pool | **Override** | Pool is the canonical serialization layer for SQLite + database/sql |
| 9 | Error format | Override → idiomatic | **Override (final)** | `errors.Is/As` composability is a capability, not preference; fix-hints belong in `kernl doctor` |
| 13 | Expression indexes | Defer | **Defer with concrete triggers** | ≥5 callers OR >50ms profiled — honest deferral, not vague "someday" |
