# P0.1 — Graph Substrate Implementation Plan

> **Bead Target:** This plan maps deterministically to beads. Every task includes a `**Bead Mapping:**` block. The converter will project tasks 1:1 with zero creative interpretation.

**Goal:** Build the typed knowledge graph substrate (SQLite + Go package `internal/graph`) that every other Kernl module reads from and writes to, with audit-by-construction (revisions written in the same tx as every mutation), FTS5 search, and 12 closed node types.

**Architecture:** Single SQLite database accessed via a dual-handle pool (read pool default size, write pool `SetMaxOpenConns(1)`). Mutations flow through a chokepoint in `internal/graph/nodes/` that writes the node, reconciles tags, writes a revision, and updates FTS5 in the same transaction. Each of the 12 node types has its own typed CRUD file backed by a `NodeSpec` interface. `ReadTx`/`WriteTx` are distinct interface types with unexported methods so no outside package can fabricate one and bypass the chokepoint.

**Tech Stack:** Go 1.22+ (stdlib `testing` + `pgregory.net/rapid` for property tests), `modernc.org/sqlite` (pure-Go driver, already in `go.sum`), `github.com/google/uuid` v1.6+ (UUIDv7, already indirect — promoted to direct), hand-rolled migrator (~85 LOC, `golang-migrate`-compatible tracking table for trivial future revert), `go:embed` for SQL migrations.

**Source documents:**
- Spec: `docs/2026-05-17-graph-substrate-brainstorm-spec.md`
- Eng review (decisions baked in): `docs/reviews/vc-plan-eng-review-2026-05-17.md`
- Test plan (gaps #1–#13): `docs/reviews/vc-plan-eng-review-test-plan-2026-05-17.md`
- Vision anchors: `docs/VISION.md` §6, §7.2, §14, §15, §16

---

## Decisions resolved in this plan (post-review)

Items that were open after the review and are now fixed below:

| Question | Decision |
|---|---|
| Migration ordering | One file: `0001_init.up.sql` + `0001_init.down.sql` covers all 7 tables + 1 virtual table. Future schema changes become `0002_*` migrations. |
| Per-type file template | One Go file per type under `internal/graph/nodes/`. Defines a struct embedding `Meta`, the 5 `NodeSpec` methods, 5 CRUD functions, and a typed `Filter` struct. Attrs fields and FTS `Body` mapping are tabulated in Lane A intro. |
| `testutil.NewTestGraph(t)` parallelism | Default uses `t.TempDir()` (tempfile + WAL) — hermetic per test, supports `t.Parallel()`, supports dual-handle. Companion `NewInMemoryTestGraph(t)` uses `file:test_<sanitize(t.Name())>?mode=memory&cache=shared` for tests that don't reopen and want maximum speed. |
| Sentinel errors API | Package-level `var Err* = errors.New("graph: <short>")` in `internal/graph/errors.go`. Callers wrap with `fmt.Errorf("graph: <op>: %w", err)`. Tests use `errors.Is(err, graph.ErrNotFound)`. No constructor functions. |

## Constraints inherited from the eng review (do NOT re-litigate)

- Single root Go module **before** any schema work (Task 1 only)
- Per-type CRUD: 12 explicit files; `NodeSpec` interface; `Meta` embedded
- Migrator hand-rolled, ~85 LOC, but tracking-table schema **identical** to golang-migrate (`version BIGINT PK, dirty BOOLEAN`) and file naming `NNNN_name.up.sql` / `NNNN_name.down.sql`
- FTS5: `fts_rowid INTEGER UNIQUE` column directly on `nodes`. **No** auxiliary `nodes_fts_map` table.
- Revisions: `diff TEXT` column populated at write-time **only** for types implementing `DiffableNode` (Note only in MVP)
- Edges: **no** `edge_events` audit log
- Concurrency: dual `*sql.DB` (read pool default, write pool `SetMaxOpenConns(1)`). **No** Go mutex.
- **Schema fix (mandatory):** `revisions.node_id` has **no** `ON DELETE CASCADE` — revisions outlive their node so audit is preserved
- Error format: `fmt.Errorf("graph: <op>: %w", err)` + named sentinels (`ErrNotFound`, `ErrFTSQuerySyntax`, `ErrSchemaLocked`, `ErrAuthorRequired`)
- Expression indexes on `attrs.*`: **defer**. Concrete trigger to revisit: ≥5 callers of the same `json_extract` filter OR a query >50ms profiled at ~10k nodes of that type.
- Property tests: `pgregory.net/rapid` (new dep, small, has shrinking)
- Tx interfaces (`ReadTx`/`WriteTx`) have an **unexported method** (`isReadTx()` / `isWriteTx()`) so no outside package can forge a Tx and bypass the chokepoint

## Parallelization lanes

```
sequential:        Task 1 (reorg) → Epic 2 (sqlite+migrator) → Epic 3 (chokepoint+types)
parallel-after-3:  Epic 4 Lane A (12 type files)   ┬─ merge
                   Epic 5 Lane B (edges/tags/fts/revisions) ─┘
final:             Task 6 Lane C (property tests, integration sweep)
```

Frozen interfaces before launching Lane A+B: `NodeSpec`, `DiffableNode`, `Meta`, `Author`, `ReadTx`, `WriteTx`, the chokepoint signatures, the sentinel errors. Any change to these post-merge will conflict both lanes.

---

# TASKS

---

## Task 1: Repo reorganization — promote to single root Go module

Mechanical move of the orchestrator module to the repo root so `internal/graph/` is importable by every binary as `github.com/gabrielassisxyz/kernl/internal/graph`. Zero behavioral change.

**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 180
- Dependencies: `none`
- Parent: `none`
- Status: `open`

**Files:**
- Create: `go.mod` (root, copied from `orchestrator/go.mod`, module path unchanged: `github.com/gabrielassisxyz/kernl`)
- Create: `go.sum` (root, copied from `orchestrator/go.sum`)
- Move: `orchestrator/cmd/*` → `cmd/*`
- Move: `orchestrator/internal/*` → `internal/orchestrator/*`
- Move: `orchestrator/web/*` → `web/*` (if present)
- Delete: `orchestrator/go.mod`, `orchestrator/go.sum`
- Modify: every `.go` file with `import "github.com/gabrielassisxyz/kernl/orchestrator/internal/..."` → `import "github.com/gabrielassisxyz/kernl/internal/orchestrator/..."` (mechanical sed)
- Modify: `.github/workflows/*.yml` — replace `cd orchestrator && go test` with `go test ./...`
- Modify: `scripts/swarm/*` — replace paths referencing `orchestrator/...` with new paths
- Modify: `AGENTS.md`, `CLAUDE.md`, `README.md` — paths only

**Description / Steps:**

- [ ] **Step 1: Inventory current module references**

```bash
rg -l 'kernl/orchestrator/(internal|cmd|web)' --type go
rg -l 'cd orchestrator' .github/ scripts/
rg -l 'orchestrator/' AGENTS.md CLAUDE.md README.md docs/
```

Capture the full list — this is the rewrite set.

- [ ] **Step 2: Create root go.mod and go.sum**

```bash
cp orchestrator/go.mod go.mod
cp orchestrator/go.sum go.sum
```

`go.mod` module line stays `module github.com/gabrielassisxyz/kernl` (no change).

- [ ] **Step 3: Move directories**

```bash
git mv orchestrator/cmd cmd
git mv orchestrator/internal internal/orchestrator
[ -d orchestrator/web ] && git mv orchestrator/web web
git rm orchestrator/go.mod orchestrator/go.sum
rmdir orchestrator 2>/dev/null || true
```

- [ ] **Step 4: Rewrite imports**

```bash
rg -l 'kernl/orchestrator/(internal|cmd|web)' --type go | \
  xargs sed -i 's#github.com/gabrielassisxyz/kernl/orchestrator/internal#github.com/gabrielassisxyz/kernl/internal/orchestrator#g; s#github.com/gabrielassisxyz/kernl/orchestrator/cmd#github.com/gabrielassisxyz/kernl/cmd#g; s#github.com/gabrielassisxyz/kernl/orchestrator/web#github.com/gabrielassisxyz/kernl/web#g'
goimports -w .
```

- [ ] **Step 5: Update CI and scripts**

Rewrite every `cd orchestrator && <cmd>` to `<cmd>` (the new root is the module root). Update `scripts/swarm/*` paths likewise.

- [ ] **Step 6: Build + test**

```bash
go build ./...
go test ./...
```

Expected: identical pass/fail to pre-reorg baseline. No behavioral diff.

- [ ] **Step 7: Commit as a single isolated PR**

```bash
git add -A
git commit -m "refactor: promote orchestrator to root go module"
```

**Acceptance Criteria:**
- [ ] `go build ./...` succeeds from repo root
- [ ] `go test ./...` produces identical results to pre-reorg baseline (no new failures, no skipped tests)
- [ ] `find . -name go.mod` returns exactly one path: `./go.mod`
- [ ] CI workflow runs green on the reorg PR with no `cd orchestrator` strings remaining
- [ ] `rg 'kernl/orchestrator/(internal|cmd|web)' --type go` returns zero hits

---

# EPIC 2: SQLite substrate + migration runner

The plumbing layer: opening the DB with correct pragmas, dual-handle pool, hand-rolled migrator, embedded SQL migrations, and the initial schema. Everything above this epic assumes `Open(cfg) → *Graph` works and migrations have applied.

**Bead Mapping:**
- type: `epic`
- Priority: `0`
- Estimated Minutes: 0
- Dependencies: `Task 1`
- Parent: `none`
- Status: `open`

**Files:** (declared by children)

**Description / Steps:** (children below)

**Acceptance Criteria:**
- [ ] `internal/graph.Open(cfg)` returns a `*Graph` with WAL on, foreign keys on, all migrations applied
- [ ] Migration up → down → up round-trip CI test passes
- [ ] `schema_migrations` table matches golang-migrate's schema verbatim (so future revert is trivial)

---

## Task 2.1: SQLite open + pragmas + dual-handle pool

`internal/graph/internal/sqlite/sqlite.go` opens the DB with correct pragmas for both pools. The read pool uses default sizing; the write pool is `SetMaxOpenConns(1)` so the database/sql layer serializes writes — no application mutex.

**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 90
- Dependencies: `Task 1`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/internal/sqlite/sqlite.go`
- Create: `internal/graph/internal/sqlite/sqlite_test.go`

**Description / Steps:**

- [ ] **Step 1: Define the package surface**

```go
package sqlite

type Config struct {
    Path     string // empty → in-memory
    InMemory bool   // when true, journal_mode=MEMORY instead of WAL
}

type Pool struct {
    Read  *sql.DB
    Write *sql.DB
}

func Open(ctx context.Context, cfg Config) (*Pool, error) { /* ... */ }
func (p *Pool) Close() error { /* close write then read */ }
```

- [ ] **Step 2: Write failing test for pragmas**

```go
func TestOpenAppliesPragmas(t *testing.T) {
    p, err := sqlite.Open(context.Background(), sqlite.Config{Path: t.TempDir() + "/x.db"})
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { p.Close() })

    var mode string
    if err := p.Read.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil { t.Fatal(err) }
    if mode != "wal" { t.Fatalf("journal_mode = %q, want wal", mode) }

    var fk int
    if err := p.Read.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil { t.Fatal(err) }
    if fk != 1 { t.Fatalf("foreign_keys = %d, want 1", fk) }
}
```

- [ ] **Step 3: Implement Open**

DSN format for tempfile: `file:<path>?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=temp_store(MEMORY)`.

For in-memory: `file:<name>?mode=memory&cache=shared&_pragma=journal_mode(MEMORY)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=temp_store(MEMORY)`.

**Multi-statement Exec:** `modernc.org/sqlite` accepts multiple SQL statements separated by `;` in a single `ExecContext` call natively. No DSN flag needed, no application-side splitting. The migrator (Task 2.2) relies on this directly — see F4 in the Notes section.

Read pool: `sql.Open("sqlite", dsn)`; default MaxOpenConns.
Write pool: same DSN; `db.SetMaxOpenConns(1)`.

- [ ] **Step 4: Write failing test for write serialization**

```go
func TestWritePoolSerializesWrites(t *testing.T) {
    p, _ := sqlite.Open(context.Background(), sqlite.Config{Path: t.TempDir() + "/x.db"})
    t.Cleanup(func() { p.Close() })

    if _, err := p.Write.Exec("CREATE TABLE k(v INTEGER)"); err != nil { t.Fatal(err) }

    var wg sync.WaitGroup
    var failed int32
    for i := 0; i < 50; i++ {
        wg.Add(1)
        go func(v int) {
            defer wg.Done()
            if _, err := p.Write.Exec("INSERT INTO k VALUES (?)", v); err != nil {
                atomic.AddInt32(&failed, 1)
            }
        }(i)
    }
    wg.Wait()
    if failed != 0 { t.Fatalf("%d writes failed with SQLITE_BUSY", failed) }

    var count int
    p.Read.QueryRow("SELECT count(*) FROM k").Scan(&count)
    if count != 50 { t.Fatalf("count=%d, want 50", count) }
}
```

> Build note for CI: `modernc.org/sqlite` is pure-Go but requires `CGO_ENABLED=0` to avoid accidental CGO linkage. `go env CGO_ENABLED` must return `0` before builds.

- [ ] **Step 5: Run tests, verify they pass**

```bash
go test ./internal/graph/internal/sqlite/... -race
```

- [ ] **Step 6: Commit**

```bash
git add internal/graph/internal/sqlite/
git commit -m "feat(graph): sqlite open with pragmas and dual-handle pool"
```

**Acceptance Criteria:**
- [ ] `TestOpenAppliesPragmas` passes (WAL on, foreign_keys on)
- [ ] `TestWritePoolSerializesWrites` passes under `-race` with 0 failures across 50 concurrent writes
- [ ] In-memory mode test (`Config{InMemory: true, Path: "file:test?mode=memory&cache=shared"}`) returns a usable `*Pool`
- [ ] `Close()` releases both handles (no leaked goroutines under `goleak` — optional verification)

---

## Task 2.2: Hand-rolled migration runner (golang-migrate compatible)

~85 LOC migrator that scans an `embed.FS` for `NNNN_name.up.sql` files, applies any whose version is not in `schema_migrations`. Tracking table schema is **identical** to golang-migrate v4 so future revert is a one-hour swap. Supports `Up(ctx)` (apply all) and `Down(ctx, targetVersion)` (run `.down.sql` files in reverse until target reached) — `Down` is used only by the CI round-trip test, never at runtime.

**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 120
- Dependencies: `Task 2.1`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/internal/migrate/migrate.go`
- Create: `internal/graph/internal/migrate/migrate_test.go`

**Description / Steps:**

- [ ] **Step 1: Define the package surface**

```go
package migrate

import (
    "context"
    "database/sql"
    "embed"
)

type Migration struct {
    Version int64  // parsed from filename prefix NNNN_
    Name    string
    Up      string
    Down    string
}

type Runner struct {
    fs       embed.FS
    rootPath string // e.g. "schema"
}

func New(fs embed.FS, rootPath string) *Runner { /* ... */ }

func (r *Runner) Up(ctx context.Context, db *sql.DB) error
func (r *Runner) Down(ctx context.Context, db *sql.DB, target int64) error
func (r *Runner) Current(ctx context.Context, db *sql.DB) (version int64, dirty bool, err error)
```

- [ ] **Step 2: Define tracking table schema (identical to golang-migrate v4)**

The runner creates this on first use:

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
  version BIGINT NOT NULL PRIMARY KEY,
  dirty   BOOLEAN NOT NULL DEFAULT FALSE
);
```

- [ ] **Step 3: Write failing test — initial Up applies one migration**

Use a temp `embed.FS`-like by embedding test fixtures in `migrate/testdata/schema/`:
- `0001_init.up.sql`: `CREATE TABLE k(v INTEGER);`
- `0001_init.down.sql`: `DROP TABLE k;`

```go
//go:embed testdata/schema/*.sql
var testFS embed.FS

func TestUpAppliesPendingMigrations(t *testing.T) {
    p, _ := sqlite.Open(...)
    r := migrate.New(testFS, "testdata/schema")
    if err := r.Up(ctx, p.Write); err != nil { t.Fatal(err) }

    v, dirty, _ := r.Current(ctx, p.Write)
    if v != 1 || dirty { t.Fatalf("v=%d dirty=%v, want 1 false", v, dirty) }

    var n int
    p.Read.QueryRow("SELECT count(*) FROM k").Scan(&n)
    if n != 0 { t.Fatalf("table k missing or non-empty") }
}
```

- [ ] **Step 4: Implement Up**

Algorithm:
1. Ensure `schema_migrations` exists.
2. Load all `NNNN_*.up.sql` from `fs`, parse versions, sort ascending.
3. SELECT MAX(version), dirty FROM schema_migrations.
4. If `dirty=true` → return `graph.ErrSchemaLocked` (declared in Task 3.2).
5. For each migration with `version > current`:
   - BEGIN tx
   - INSERT INTO schema_migrations(version, dirty) VALUES (?, true) ON CONFLICT DO UPDATE SET dirty=true
   - `tx.ExecContext(ctx, content)` — `modernc.org/sqlite` accepts the full multi-statement SQL string in one call (no application-side `;` splitter). If a future migration breaks parsing, the driver returns a parse error and the tx rolls back; `dirty=true` stays sticky.
   - UPDATE schema_migrations SET dirty=false WHERE version=?
   - COMMIT
6. Errors leave `dirty=true` for diagnosis.

- [ ] **Step 5: Write failing test — Down rolls back**

```go
func TestDownRollsBack(t *testing.T) {
    p, _ := sqlite.Open(...)
    r := migrate.New(testFS, "testdata/schema")
    _ = r.Up(ctx, p.Write)
    if err := r.Down(ctx, p.Write, 0); err != nil { t.Fatal(err) }

    v, _, _ := r.Current(ctx, p.Write)
    if v != 0 { t.Fatalf("v=%d, want 0", v) }

    var n int
    err := p.Read.QueryRow("SELECT count(*) FROM k").Scan(&n)
    if err == nil { t.Fatal("table k should not exist") }
}
```

- [ ] **Step 6: Implement Down + up→down→up round-trip test**

```go
func TestUpDownUpRoundTrip(t *testing.T) {
    p, _ := sqlite.Open(...)
    r := migrate.New(testFS, "testdata/schema")
    if err := r.Up(ctx, p.Write); err != nil { t.Fatal(err) }
    if err := r.Down(ctx, p.Write, 0); err != nil { t.Fatal(err) }
    if err := r.Up(ctx, p.Write); err != nil { t.Fatal(err) }
    v, _, _ := r.Current(ctx, p.Write)
    if v != 1 { t.Fatal("round-trip lost version") }
}
```

- [ ] **Step 7: Write failing test — dirty flag is sticky after a broken migration**

Create a second test fixture set in `migrate/testdata/schema_broken/`:
- `0001_init.up.sql`: `CREATE TABEL bad(v INTEGER);`  (intentional `TABEL` typo)
- `0001_init.down.sql`: `DROP TABLE IF EXISTS bad;`

```go
//go:embed testdata/schema_broken/*.sql
var brokenFS embed.FS

func TestDirtyStickyAfterFailedMigration(t *testing.T) {
    p, _ := sqlite.Open(...)
    r := migrate.New(brokenFS, "testdata/schema_broken")

    err := r.Up(ctx, p.Write)
    if err == nil { t.Fatal("expected migration to fail") }

    _, dirty, _ := r.Current(ctx, p.Write)
    if !dirty { t.Fatal("dirty flag not set after failed migration") }

    // Second Up must refuse — dirty is sticky across calls
    err2 := r.Up(ctx, p.Write)
    if !errors.Is(err2, migrate.ErrDirty) {
        t.Fatalf("second Up err=%v, want ErrDirty", err2)
    }
}
```

- [ ] **Step 8: Run, verify, commit**

```bash
go test ./internal/graph/internal/migrate/... -race
git add internal/graph/internal/migrate/
git commit -m "feat(graph): hand-rolled migrator with golang-migrate-compatible tracking"
```

**Acceptance Criteria:**
- [ ] `TestUpAppliesPendingMigrations` passes
- [ ] `TestDownRollsBack` passes
- [ ] `TestUpDownUpRoundTrip` passes
- [ ] `TestDirtyStickyAfterFailedMigration` passes (dirty=true after parse error; subsequent Up returns `ErrDirty`)
- [ ] `schema_migrations` table schema matches golang-migrate v4 verbatim (verifiable by `PRAGMA table_info(schema_migrations)`)
- [ ] Total runner LOC ≤ 120 (target ~85, generous ceiling)

---

## Task 2.3: Initial schema migration `0001_init.up.sql` + `.down.sql`

The complete substrate schema in one atomic migration. Includes the schema-correction fix: `revisions.node_id` has **no** `ON DELETE CASCADE`. Includes `fts_rowid INTEGER UNIQUE` directly on `nodes` (no map table).

**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 60
- Dependencies: `Task 2.2`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/schema/0001_init.up.sql`
- Create: `internal/graph/schema/0001_init.down.sql`
- Create: `internal/graph/schema/schema.go` (declares `//go:embed *.sql` and exports `FS embed.FS`)

**Description / Steps:**

- [ ] **Step 1: Write `0001_init.up.sql`**

```sql
-- Nodes: polymorphic table for the closed set of types
CREATE TABLE IF NOT EXISTS nodes (
  id          TEXT PRIMARY KEY,
  type        TEXT NOT NULL,
  title       TEXT NOT NULL DEFAULT '',
  attrs       TEXT NOT NULL DEFAULT '{}',
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL,
  owner_id    TEXT,
  visibility  TEXT,
  fts_rowid   INTEGER UNIQUE,
  CHECK (type IN ('note','bead','project','task','session','decision',
                  'memory_claim','memory_refutation','bookmark',
                  'bookmark_list','capture','workflow_run')),
  CHECK (json_valid(attrs))
) STRICT;
CREATE INDEX nodes_type_updated_idx ON nodes(type, updated_at DESC);
CREATE INDEX nodes_updated_at_idx   ON nodes(updated_at DESC);

-- Edges: directed, closed type vocabulary, optional JSON payload.
-- owner_id/visibility mirror the nodes table for VISION §16 future multi-user
-- posture; both are NULL in single-user mode and invisible in the UI.
CREATE TABLE edges (
  id          TEXT PRIMARY KEY,
  src_node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  dst_node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  type        TEXT NOT NULL,
  attrs       TEXT NOT NULL DEFAULT '{}',
  created_at  INTEGER NOT NULL,
  owner_id    TEXT,
  visibility  TEXT,
  CHECK (type IN ('depends_on','parent_of','inspired_by','mentions',
                  'generated_from','processed_into','processed_from',
                  'refutes','relates_to')),
  CHECK (json_valid(attrs))
) STRICT;
CREATE INDEX edges_src_idx ON edges(src_node_id, type);
CREATE INDEX edges_dst_idx ON edges(dst_node_id, type);

-- Tags: open vocabulary
CREATE TABLE tags (name TEXT PRIMARY KEY) STRICT;

CREATE TABLE node_tags (
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  tag     TEXT NOT NULL REFERENCES tags(name) ON DELETE CASCADE,
  PRIMARY KEY (node_id, tag)
) STRICT;
CREATE INDEX node_tags_tag_idx ON node_tags(tag, node_id);

-- Revisions: append-only audit log; survives node deletion (no cascade).
-- prev_revision_id forms a per-node linked list; on node delete the chain
-- still walks (the rows persist) but the FK to nodes is intentionally absent.
CREATE TABLE revisions (
  id               TEXT PRIMARY KEY,
  node_id          TEXT NOT NULL,
  prev_revision_id TEXT REFERENCES revisions(id),
  author           TEXT NOT NULL,
  snapshot         TEXT NOT NULL,
  diff             TEXT,
  ts               INTEGER NOT NULL,
  CHECK (json_valid(snapshot))
) STRICT;
CREATE INDEX revisions_node_ts_idx ON revisions(node_id, ts DESC);

-- FTS5: contentless virtual table; rowid linked to nodes.fts_rowid
CREATE VIRTUAL TABLE nodes_fts USING fts5(
  title, body, tags,
  content='',
  tokenize = 'unicode61 remove_diacritics 2'
);
```

- [ ] **Step 2: Write `0001_init.down.sql`**

```sql
DROP TABLE IF EXISTS nodes_fts;
DROP TABLE IF EXISTS revisions;
DROP TABLE IF EXISTS node_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS edges;
DROP TABLE IF EXISTS nodes;
```

(`schema_migrations` is owned by the runner, not this migration.)

- [ ] **Step 3: Embed the FS**

```go
package schema

import "embed"

//go:embed *.sql
var FS embed.FS

const Root = "." // pass to migrate.New(FS, Root)
```

- [ ] **Step 4: Smoke test — apply against a real DB**

```go
func TestInitialSchemaApplies(t *testing.T) {
    p, _ := sqlite.Open(ctx, sqlite.Config{Path: t.TempDir() + "/x.db"})
    r := migrate.New(schema.FS, schema.Root)
    if err := r.Up(ctx, p.Write); err != nil { t.Fatal(err) }

    for _, tbl := range []string{"nodes","edges","tags","node_tags","revisions","nodes_fts"} {
        var n int
        if err := p.Read.QueryRow("SELECT count(*) FROM " + tbl).Scan(&n); err != nil {
            t.Errorf("table %s missing: %v", tbl, err)
        }
    }
}
```

- [ ] **Step 5: Up→down→up round-trip on the real schema**

```go
func TestInitialSchemaRoundTrip(t *testing.T) {
    p, _ := sqlite.Open(...)
    r := migrate.New(schema.FS, schema.Root)
    if err := r.Up(ctx, p.Write); err != nil { t.Fatal(err) }
    if err := r.Down(ctx, p.Write, 0); err != nil { t.Fatal(err) }
    if err := r.Up(ctx, p.Write); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 6: Commit**

```bash
git add internal/graph/schema/
git commit -m "feat(graph): initial schema migration (nodes/edges/tags/revisions/fts)"
```

- [ ] **Step 5a: Add `attrs` JSON validation test**

```go
func TestAttrsRejectInvalidJSON(t *testing.T) {
    p, _ := sqlite.Open(ctx, sqlite.Config{Path: t.TempDir() + "/x.db"})
    r := migrate.New(schema.FS, schema.Root)
    _ = r.Up(ctx, p.Write)

    _, err := p.Write.ExecContext(ctx,
        `INSERT INTO nodes(id,type,attrs,created_at,updated_at) VALUES('x','bead','{not json',0,0)`)
    if err == nil { t.Fatal("CHECK(json_valid(attrs)) did not reject malformed JSON") }
}
```

**Acceptance Criteria:**
- [ ] `TestInitialSchemaApplies` passes; all 6 tables queryable
- [ ] `TestInitialSchemaRoundTrip` passes
- [ ] `TestAttrsRejectInvalidJSON` passes (proves CHECK constraint is active)
- [ ] `revisions.node_id` has **no** ON DELETE CASCADE (verify by `PRAGMA foreign_key_list(revisions)`)
- [ ] `nodes.fts_rowid` is `INTEGER UNIQUE` (verify by `PRAGMA table_info(nodes)` + `PRAGMA index_list(nodes)`)
- [ ] `edges.owner_id` and `edges.visibility` columns present and nullable (verify by `PRAGMA table_info(edges)`)
- [ ] FTS5 virtual table queryable: `SELECT count(*) FROM nodes_fts` returns 0

---

## Task 2.4: `*Graph` top-level type + `Open`/`Close`/`DoRead`/`DoWrite`

Wires the pool, the migrator, and the schema into a single `*Graph` type. This is the public entry point the rest of the codebase will use.

**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 75
- Dependencies: `Task 2.3`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/graph.go`
- Create: `internal/graph/graph_test.go`

**Description / Steps:**

- [ ] **Step 1: Define `Config` and `*Graph`**

```go
package graph

type Config struct {
    Path     string // empty → in-memory
    InMemory bool
}

type Graph struct {
    pool *sqlite.Pool
}

func Open(ctx context.Context, cfg Config) (*Graph, error) {
    p, err := sqlite.Open(ctx, sqlite.Config{Path: cfg.Path, InMemory: cfg.InMemory})
    if err != nil { return nil, fmt.Errorf("graph: open: %w", err) }
    r := migrate.New(schema.FS, schema.Root)
    if err := r.Up(ctx, p.Write); err != nil {
        p.Close()
        return nil, fmt.Errorf("graph: migrate: %w", err)
    }
    return &Graph{pool: p}, nil
}

func (g *Graph) Close() error { return g.pool.Close() }
```

- [ ] **Step 2: Define `ReadTx` / `WriteTx` interfaces (unforgeable, no Commit/Rollback leak)**

```go
type ReadTx interface {
    QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
    isReadTx()
}

type WriteTx interface {
    ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
    QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
    isWriteTx()
}

// NOTE: do NOT embed *sql.Tx (it would leak Commit/Rollback to callers
// and break the DoRead/DoWrite lifecycle contract). Delegate explicitly.

type readTx struct{ tx *sql.Tx }
func (r readTx) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) { return r.tx.QueryContext(ctx, q, args...) }
func (r readTx) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row        { return r.tx.QueryRowContext(ctx, q, args...) }
func (readTx) isReadTx() {}

type writeTx struct{ tx *sql.Tx }
func (w writeTx) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) { return w.tx.ExecContext(ctx, q, args...) }
func (w writeTx) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) { return w.tx.QueryContext(ctx, q, args...) }
func (w writeTx) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row         { return w.tx.QueryRowContext(ctx, q, args...) }
func (writeTx) isWriteTx() {}
```

The unexported methods (`isReadTx`/`isWriteTx`) prevent any package outside `internal/graph` from implementing these interfaces. Explicit delegation (instead of embedding `*sql.Tx`) prevents `Commit`/`Rollback` from leaking to callers — the lifecycle is owned by `DoRead`/`DoWrite`.

- [ ] **Step 3: `DoRead` / `DoWrite` helpers**

```go
func (g *Graph) DoRead(ctx context.Context, fn func(ReadTx) error) error {
    tx, err := g.pool.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
    if err != nil { return fmt.Errorf("graph: begin read: %w", err) }
    if err := fn(readTx{tx}); err != nil { tx.Rollback(); return err }
    return tx.Commit()
}

func (g *Graph) DoWrite(ctx context.Context, fn func(WriteTx) error) error {
    tx, err := g.pool.Write.BeginTx(ctx, nil)
    if err != nil { return fmt.Errorf("graph: begin write: %w", err) }
    if err := fn(writeTx{tx}); err != nil { tx.Rollback(); return err }
    return tx.Commit()
}
```

- [ ] **Step 4: Write smoke tests**

```go
func TestOpenAndClose(t *testing.T) {
    g, err := graph.Open(ctx, graph.Config{Path: t.TempDir() + "/x.db"})
    if err != nil { t.Fatal(err) }
    if err := g.Close(); err != nil { t.Fatal(err) }
}

func TestDoReadDoWrite(t *testing.T) {
    g, _ := graph.Open(ctx, graph.Config{Path: t.TempDir() + "/x.db"})
    t.Cleanup(func() { g.Close() })

    err := g.DoWrite(ctx, func(tx graph.WriteTx) error {
        _, err := tx.ExecContext(ctx, "INSERT INTO tags(name) VALUES (?)", "smoke")
        return err
    })
    if err != nil { t.Fatal(err) }

    err = g.DoRead(ctx, func(tx graph.ReadTx) error {
        var n int
        return tx.QueryRowContext(ctx, "SELECT count(*) FROM tags").Scan(&n)
    })
    if err != nil { t.Fatal(err) }
}
```

- [ ] **Step 5: Compile-fail testdata — Tx forgery is impossible**

Create `internal/graph/testdata/forge_tx.go.broken`:

```go
package forgetest
import "github.com/gabrielassisxyz/kernl/internal/graph"
type fake struct{}
func (fake) ExecContext(...) (...) { ... }
// ... no isWriteTx() method possible — unexported
var _ graph.WriteTx = fake{}  // MUST NOT COMPILE
```

`make verify-forge-fails` (or a `go test` helper) attempts to build this file and asserts the build fails with `cannot use fake{} as graph.WriteTx`. **Optional in v1 (per review TODO #2)** — flag as a follow-up if it complicates the task.

- [ ] **Step 6: Commit**

```bash
git add internal/graph/graph.go internal/graph/graph_test.go
git commit -m "feat(graph): Graph type with Open/Close/DoRead/DoWrite + unforgeable Tx"
```

**Acceptance Criteria:**
- [ ] `TestOpenAndClose` passes
- [ ] `TestDoReadDoWrite` passes
- [ ] `ReadTx` and `WriteTx` have unexported methods (`isReadTx`/`isWriteTx`) verifiable by inspection
- [ ] **Open idempotency:** sequential `Open → Close → Open` on the same tempfile path returns success twice, the second call applies zero pending migrations (verifiable: `r.Current()` returns the same version before and after the second `Up`; no new rows inserted into `schema_migrations`), and no `SQLITE_BUSY` error surfaces from the second Open
- [ ] Errors are wrapped with `graph: <op>: %w` (no `KERNL DISPATCH FAILURE` strings)

---

# EPIC 3: Chokepoint + types + errors + testutil

The semantic core. `NodeSpec` defines what a node looks like; the chokepoint (`createNode`/`updateNode`/`deleteNode`) is the **only** code path that mutates a node. It writes the node, reconciles tags, writes a revision, and updates FTS in the same tx. Per-type CRUD (Epic 4) delegates to this. After this epic lands, Lane A and Lane B can run in parallel.

**Bead Mapping:**
- type: `epic`
- Priority: `0`
- Estimated Minutes: 0
- Dependencies: `Epic 2`
- Parent: `none`
- Status: `open`

**Acceptance Criteria:**
- [ ] `NodeSpec`, `Meta`, `DiffableNode`, `Author` defined and **frozen** — zero changes to these types or the chokepoint signatures after this epic merges
- [ ] Sentinel errors defined (`ErrNotFound`, `ErrFTSQuerySyntax`, `ErrSchemaLocked`, `ErrAuthorRequired`)
- [ ] Chokepoint enforces: every mutation writes exactly one revision in the same tx; empty Author rejected with `ErrAuthorRequired`
- [ ] `testutil.NewTestGraph(t)` works for parallel tests; `NewInMemoryTestGraph(t)` works for speed-sensitive tests

---

## Task 3.1: Sentinel errors package

Define all package-level error sentinels in one file. Done early so chokepoint, migrator, and FTS can reference them.

**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 30
- Dependencies: `Task 2.4`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/errors.go`
- Create: `internal/graph/errors_test.go`

**Description / Steps:**

- [ ] **Step 1: Define the sentinels**

```go
package graph

import "errors"

var (
    ErrNotFound        = errors.New("graph: not found")
    ErrFTSQuerySyntax  = errors.New("graph: fts query syntax")
    ErrSchemaLocked    = errors.New("graph: schema migration dirty")
    ErrAuthorRequired  = errors.New("graph: author required")
)
```

- [ ] **Step 2: Migrator references `ErrSchemaLocked`**

Update `internal/graph/internal/migrate/migrate.go` (from Task 2.2) to return `graph.ErrSchemaLocked` when it finds `dirty=true`. (Migrate imports a small `internal/graph/errsentinel` sub-package to avoid the migrate→graph→migrate cycle — OR the migrator returns a private `errDirty` and the `*Graph.Open` translates it. **Pick the second**, simpler.)

```go
// in migrate
var ErrDirty = errors.New("migration: dirty")

// in graph.Open
if err := r.Up(ctx, p.Write); err != nil {
    if errors.Is(err, migrate.ErrDirty) {
        return nil, fmt.Errorf("graph: open: %w", ErrSchemaLocked)
    }
    return nil, fmt.Errorf("graph: migrate: %w", err)
}
```

- [ ] **Step 3: Write tests**

```go
func TestSentinelsAreDistinct(t *testing.T) {
    if errors.Is(graph.ErrNotFound, graph.ErrFTSQuerySyntax) {
        t.Fatal("sentinels collide")
    }
}

// NOTE: testing against in-memory variant to avoid WAL lock contention
// during rapid Open/Close from the same process
func TestSchemaLockedSurfacesFromOpen(t *testing.T) {
    g := testutil.NewInMemoryTestGraph(t)
    // ... trigger dirty via direct SQL
}
```

- [ ] **Step 4: Commit**

```bash
git add internal/graph/errors.go internal/graph/errors_test.go internal/graph/internal/migrate/
git commit -m "feat(graph): sentinel errors + ErrSchemaLocked surfaced from migrator"
```

**Acceptance Criteria:**
- [ ] All four sentinels exported and distinct
- [ ] `errors.Is(err, graph.ErrSchemaLocked)` returns true when Open encounters a dirty migration
- [ ] Sentinel messages all start with `graph: `

---

## Task 3.2: `NodeSpec`, `Meta`, `DiffableNode`, `Author`

The shared types every node implementation uses. Lives in `internal/graph/nodes/node.go`. Frozen surface before Lane A starts.

**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 60
- Dependencies: `Task 3.1`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/node.go`
- Create: `internal/graph/nodes/node_test.go`

**Description / Steps:**

- [ ] **Step 1: Define the interface set**

```go
package nodes

import (
    "encoding/json"
    "time"
)

type NodeSpec interface {
    NodeType() string                    // closed set: 'bead','note','project',...
    NodeID() string                      // from embedded Meta
    NodeTitle() string
    NodeAttrs() (json.RawMessage, error) // type-specific fields, excluding Meta+Title
    NodeTags() []string
    FTSFields() FTSFields
}

type FTSFields struct {
    Title string
    Body  string
    Tags  string
}

type Meta struct {
    ID         string
    CreatedAt  time.Time
    UpdatedAt  time.Time
    OwnerID    *string
    Visibility *string
}

func (m Meta) NodeID() string { return m.ID }

type DiffableNode interface {
    NodeSpec
    DiffBody(prev NodeSpec) string
}

type Author string

const (
    AuthorHuman Author = "human"
    AuthorDA    Author = "da"
)

func AuthorAgent(id string) Author { return Author("agent:" + id) }

func (a Author) Valid() bool { return string(a) != "" }
```

- [ ] **Step 2: Write tests**

```go
func TestMetaImplementsNodeID(t *testing.T) {
    var m Meta = Meta{ID: "x"}
    if m.NodeID() != "x" { t.Fail() }
}

func TestAuthorValid(t *testing.T) {
    if (Author("")).Valid() { t.Fatal("empty Author must be invalid") }
    if !AuthorHuman.Valid() { t.Fatal("human must be valid") }
    if !AuthorAgent("kimi").Valid() { t.Fatal("agent must be valid") }
    if string(AuthorAgent("kimi")) != "agent:kimi" { t.Fatal("prefix wrong") }
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/graph/nodes/node.go internal/graph/nodes/node_test.go
git commit -m "feat(graph/nodes): NodeSpec, Meta, DiffableNode, Author"
```

**Acceptance Criteria:**
- [ ] `Meta` embedding satisfies `NodeID()` for any wrapping struct
- [ ] `Author("")` returns `false` from `Valid()`
- [ ] `AuthorAgent("x")` produces literal `"agent:x"`

---

## Task 3.3: UUIDv7 generator

Wraps `google/uuid` v1.6+. Lives in `internal/graph/internal/ids/`. Exposed via `ids.New()` returning a string.

**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 30
- Dependencies: `Task 1`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/internal/ids/ids.go`
- Create: `internal/graph/internal/ids/ids_test.go`
- Modify: `go.mod` — promote `github.com/google/uuid` from indirect to direct

**Description / Steps:**

- [ ] **Step 1: Implement**

```go
package ids

import "github.com/google/uuid"

func New() string {
    id, err := uuid.NewV7()
    if err != nil { panic("uuid.NewV7 failed: " + err.Error()) }
    return id.String()
}
```

- [ ] **Step 2: Monotonicity test (gap #13 from test plan)**

```go
func TestUUIDv7MonotonicWithinProcess(t *testing.T) {
    const n = 10_000
    prev := ids.New()
    for i := 0; i < n; i++ {
        cur := ids.New()
        if cur <= prev {
            t.Fatalf("non-monotone at i=%d: prev=%s cur=%s", i, prev, cur)
        }
        prev = cur
    }
}
```

- [ ] **Step 3: Sortable-by-creation-time test**

```go
func TestUUIDv7SortableByCreationTime(t *testing.T) {
    type stamped struct{ id string; t time.Time }
    var rows []stamped
    deadline := time.Now().Add(100 * time.Millisecond)
    for time.Now().Before(deadline) {
        rows = append(rows, stamped{ids.New(), time.Now()})
    }
    sortedByID := make([]stamped, len(rows))
    copy(sortedByID, rows)
    sort.Slice(sortedByID, func(i,j int) bool { return sortedByID[i].id < sortedByID[j].id })

    for i := range rows {
        if rows[i].id != sortedByID[i].id {
            t.Fatalf("UUIDv7 sort order != creation order at i=%d", i)
        }
    }
}
```

- [ ] **Step 4: Commit**

```bash
go mod tidy
git add internal/graph/internal/ids/ go.mod go.sum
git commit -m "feat(graph): UUIDv7 generator + monotone & sortable tests"
```

**Acceptance Criteria:**
- [ ] `TestUUIDv7MonotonicWithinProcess` passes for n=10_000
- [ ] `TestUUIDv7SortableByCreationTime` passes
- [ ] `go.mod` lists `github.com/google/uuid` as a direct dep

---

## Task 3.4: Chokepoint — `createNode`/`updateNode`/`deleteNode`

The single mutation path. All per-type `CreateXxx`/`UpdateXxx`/`DeleteXxx` funcs (Lane A) delegate to these. They live in `internal/graph/nodes/chokepoint.go`. Each runs a sequence inside the caller's `WriteTx`: load prev (if Update/Delete), upsert node, reconcile tags, write revision (with diff if `DiffableNode`), update FTS5 (delete then insert by `fts_rowid`).

**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 240
- Dependencies: `Task 3.2`, `Task 3.3`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/chokepoint.go`
- Create: `internal/graph/nodes/chokepoint_test.go`

**Description / Steps:**

- [ ] **Step 1: Define chokepoint signatures (exported within `nodes` package; called by per-type files)**

```go
package nodes

// CreateNode inserts a new node, reconciles tags, writes initial revision,
// and inserts into FTS5. Author must be non-empty.
func CreateNode(ctx context.Context, tx graph.WriteTx, spec NodeSpec, author Author) error

// UpdateNode loads prev, applies new state, reconciles tags, writes revision
// (with diff if spec is DiffableNode), updates FTS5.
func UpdateNode(ctx context.Context, tx graph.WriteTx, spec NodeSpec, author Author) error

// DeleteNode writes a final tombstone revision (snapshot of last state) BEFORE
// deleting the node. Cascade removes edges/tags/fts mapping. Revisions persist
// (no FK cascade).
func DeleteNode(ctx context.Context, tx graph.WriteTx, id string, author Author) error
```

- [ ] **Step 2: ASCII flow diagram as a top-of-file comment (per Issue 11 of the review)**

```go
// Mutation flow — single transaction, no public bypass:
//
//   createNode(spec):
//     1. validate Author non-empty                     → ErrAuthorRequired
//     2. assign Meta.ID = ids.New() if empty
//     3. INSERT nodes (with fts_rowid = NULL)
//     4. INSERT INTO nodes_fts (title,body,tags) → rowid R
//     5. UPDATE nodes SET fts_rowid = R WHERE id = ?
//     6. reconcile tags (INSERT new, INSERT INTO node_tags; skip empty strings)
//     7. INSERT revisions (snapshot, diff=NULL, author)
//
//   updateNode(spec):
//     1. validate Author non-empty                     → ErrAuthorRequired
//     2. SELECT prev row + prev tags + prev fts_rowid  → ErrNotFound if absent
//     3. UPDATE nodes SET title=?, attrs=?, updated_at=?
//     4. reconcile tags (compute diff: add/remove)
//     5. INSERT revisions (snapshot, diff=DiffBody(prev) if DiffableNode, author,
//        prev_revision_id = (SELECT id FROM revisions WHERE node_id=? ORDER BY ts DESC LIMIT 1))
//     6. DELETE FROM nodes_fts WHERE rowid = prev.fts_rowid
//        INSERT INTO nodes_fts (rowid,title,body,tags) — reuse same fts_rowid
//
//   deleteNode(id):
//     1. validate Author non-empty                     → ErrAuthorRequired
//     2. SELECT current spec                            → ErrNotFound if absent
//     3. INSERT revisions (snapshot=tombstone, author, ts)   — BEFORE delete
//     4. DELETE FROM nodes WHERE id = ?                       — cascades edges/tags/fts
//     5. revisions rows persist (no FK cascade — intentional)
//
// REVIEWER NOTE: when changing this flow, update both the comment AND the
// chokepoint_test.go invariants.
```

- [ ] **Step 3: Write failing test — Create writes exactly one revision**

```go
func TestCreateNodeWritesOneRevision(t *testing.T) {
    g := testutil.NewTestGraph(t)
    spec := &fakeSpec{id: "n1", typ: "bead", title: "x", tags: []string{"a"}}

    err := g.DoWrite(ctx, func(tx graph.WriteTx) error {
        return nodes.CreateNode(ctx, tx, spec, nodes.AuthorHuman)
    })
    if err != nil { t.Fatal(err) }

    var n int
    g.DoRead(ctx, func(tx graph.ReadTx) error {
        return tx.QueryRowContext(ctx, "SELECT count(*) FROM revisions WHERE node_id=?", "n1").Scan(&n)
    })
    if n != 1 { t.Fatalf("revisions count = %d, want 1", n) }
}
```

(`fakeSpec` is a test-only NodeSpec implementer in `chokepoint_test.go`.)

- [ ] **Step 4: Implement createNode**

Full SQL implementation following the flow diagram. ~80 LOC.

- [ ] **Step 5: Write failing test — Update writes one revision; FTS reflects current state (gap #5/§4 of test plan)**

```go
func TestUpdateNodeReplacesFTSContent(t *testing.T) {
    g := testutil.NewTestGraph(t)
    spec := &fakeSpec{id: "n1", typ: "note", title: "t", body: "hello world"}

    g.DoWrite(ctx, func(tx graph.WriteTx) error { return nodes.CreateNode(ctx, tx, spec, nodes.AuthorHuman) })

    // Update body
    spec.body = "goodbye moon"
    g.DoWrite(ctx, func(tx graph.WriteTx) error { return nodes.UpdateNode(ctx, tx, spec, nodes.AuthorHuman) })

    // Stale word "hello" must not match; new word "goodbye" must match
    g.DoRead(ctx, func(tx graph.ReadTx) error {
        var n int
        tx.QueryRowContext(ctx, "SELECT count(*) FROM nodes_fts WHERE nodes_fts MATCH ?", "hello").Scan(&n)
        if n != 0 { t.Errorf("stale FTS entry for 'hello'") }
        tx.QueryRowContext(ctx, "SELECT count(*) FROM nodes_fts WHERE nodes_fts MATCH ?", "goodbye").Scan(&n)
        if n != 1 { t.Errorf("new FTS entry missing for 'goodbye'") }
        return nil
    })
}
```

- [ ] **Step 6: Implement updateNode**

Including diff for DiffableNode types. ~100 LOC.

- [ ] **Step 7: Write failing test — Delete writes tombstone and preserves history (CRITICAL — schema-correction regression test)**

```go
func TestDeleteNodePreservesRevisionHistory(t *testing.T) {
    g := testutil.NewTestGraph(t)
    spec := &fakeSpec{id: "n1", typ: "bead", title: "v1"}

    g.DoWrite(ctx, func(tx graph.WriteTx) error { return nodes.CreateNode(ctx, tx, spec, nodes.AuthorHuman) })
    spec.title = "v2"
    g.DoWrite(ctx, func(tx graph.WriteTx) error { return nodes.UpdateNode(ctx, tx, spec, nodes.AuthorHuman) })
    g.DoWrite(ctx, func(tx graph.WriteTx) error { return nodes.DeleteNode(ctx, tx, "n1", nodes.AuthorHuman) })

    var revs int
    g.DoRead(ctx, func(tx graph.ReadTx) error {
        return tx.QueryRowContext(ctx, "SELECT count(*) FROM revisions WHERE node_id=?", "n1").Scan(&revs)
    })
    if revs != 3 { t.Fatalf("revisions after delete = %d, want 3 (create+update+tombstone)", revs) }

    // Walk back from latest via prev_revision_id — chain unbroken
    var current string
    g.DoRead(ctx, func(tx graph.ReadTx) error {
        return tx.QueryRowContext(ctx, "SELECT id FROM revisions WHERE node_id=? ORDER BY ts DESC LIMIT 1", "n1").Scan(&current)
    })
    walked := 1
    for {
        var prev sql.NullString
        g.DoRead(ctx, func(tx graph.ReadTx) error {
            return tx.QueryRowContext(ctx, "SELECT prev_revision_id FROM revisions WHERE id=?", current).Scan(&prev)
        })
        if !prev.Valid { break }
        current = prev.String
        walked++
    }
    if walked != 3 { t.Fatalf("prev_revision_id chain walked %d, want 3", walked) }
}
```

- [ ] **Step 8: Implement deleteNode**

- [ ] **Step 9: Write failing test — empty Author rejected (gap #9)**

```go
func TestEmptyAuthorRejected(t *testing.T) {
    g := testutil.NewTestGraph(t)
    spec := &fakeSpec{id: "n1", typ: "bead", title: "x"}

    err := g.DoWrite(ctx, func(tx graph.WriteTx) error {
        return nodes.CreateNode(ctx, tx, spec, nodes.Author(""))
    })
    if !errors.Is(err, graph.ErrAuthorRequired) {
        t.Fatalf("err=%v, want ErrAuthorRequired", err)
    }
}
```

- [ ] **Step 8a: Write test — tombstone author preserved (gap)**

The §3.4 chain proves revision *count*; this asserts the tombstone's `author` column matches what was passed to `DeleteNode`. Without this, the chokepoint could silently write an empty/wrong author on the final revision and the audit lies.

```go
func TestDeleteTombstonePreservesAuthor(t *testing.T) {
    g := testutil.NewTestGraph(t)
    spec := &fakeSpec{id: "n1", typ: "bead", title: "x"}
    g.DoWrite(ctx, func(tx graph.WriteTx) error { return nodes.CreateNode(ctx, tx, spec, nodes.AuthorHuman) })
    g.DoWrite(ctx, func(tx graph.WriteTx) error { return nodes.DeleteNode(ctx, tx, "n1", nodes.AuthorAgent("kimi")) })

    var gotAuthor string
    g.DoRead(ctx, func(tx graph.ReadTx) error {
        return tx.QueryRowContext(ctx,
            "SELECT author FROM revisions WHERE node_id=? ORDER BY ts DESC LIMIT 1", "n1").Scan(&gotAuthor)
    })
    if gotAuthor != "agent:kimi" {
        t.Fatalf("tombstone author=%q, want agent:kimi", gotAuthor)
    }
}
```

- [ ] **Step 9a: Write test — empty title allowed (spec §10 boundary input, gap)**

```go
func TestEmptyTitleAllowed(t *testing.T) {
    g := testutil.NewTestGraph(t)
    spec := &fakeSpec{id: "n1", typ: "bead", title: ""}

    err := g.DoWrite(ctx, func(tx graph.WriteTx) error {
        return nodes.CreateNode(ctx, tx, spec, nodes.AuthorHuman)
    })
    if err != nil { t.Fatalf("empty title must be allowed: %v", err) }

    var gotTitle string
    g.DoRead(ctx, func(tx graph.ReadTx) error {
        return tx.QueryRowContext(ctx, "SELECT title FROM nodes WHERE id=?", "n1").Scan(&gotTitle)
    })
    if gotTitle != "" { t.Fatalf("title=%q, want empty", gotTitle) }
}
```

- [ ] **Step 10: Run full chokepoint test suite, verify all pass**

```bash
go test ./internal/graph/nodes/... -race -run Chokepoint
```

- [ ] **Step 11: Commit**

```bash
git add internal/graph/nodes/chokepoint.go internal/graph/nodes/chokepoint_test.go
git commit -m "feat(graph/nodes): chokepoint createNode/updateNode/deleteNode with audit-by-construction"
```

**Acceptance Criteria:**
- [ ] `TestCreateNodeWritesOneRevision` passes
- [ ] `TestUpdateNodeReplacesFTSContent` passes (no stale FTS entries)
- [ ] `TestDeleteNodePreservesRevisionHistory` passes (3 revisions including tombstone; chain unbroken)
- [ ] `TestDeleteTombstonePreservesAuthor` passes (tombstone row's `author` matches Delete caller)
- [ ] `TestEmptyAuthorRejected` passes with `errors.Is(err, ErrAuthorRequired)`
- [ ] `TestEmptyTitleAllowed` passes
- [ ] All Create/Update/Delete invariants run inside a single `WriteTx` (verifiable: a panic mid-fn rolls back everything)
- [ ] Diff column is populated for DiffableNode, NULL otherwise (separate test with two specs)

---

## Task 3.5: `testutil.NewTestGraph(t)` + `NewInMemoryTestGraph(t)`

Hermetic test helpers. Tempfile variant is the default (parallel-safe via `t.TempDir()`). In-memory variant exists for speed-sensitive single-handle tests; uses unique DSN per test based on sanitized `t.Name()` + `cache=shared` so both pool handles see the same memory DB.

**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 45
- Dependencies: `Task 2.4`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/testutil/substrate.go`
- Create: `internal/graph/testutil/substrate_test.go`

**Description / Steps:**

- [ ] **Step 1: Implement tempfile helper (default)**

```go
package testutil

import (
    "context"
    "path/filepath"
    "testing"
    "github.com/gabrielassisxyz/kernl/internal/graph"
)

func NewTestGraph(t *testing.T) *graph.Graph {
    t.Helper()
    path := filepath.Join(t.TempDir(), "test.db")
    g, err := graph.Open(context.Background(), graph.Config{Path: path})
    if err != nil { t.Fatalf("NewTestGraph: %v", err) }
    t.Cleanup(func() { _ = g.Close() })
    return g
}
```

- [ ] **Step 2: Implement in-memory helper**

```go
var memSeq atomic.Int64

func NewInMemoryTestGraph(t *testing.T) *graph.Graph {
    t.Helper()
    // Unique DSN per call (avoids cross-test contamination even when cache=shared)
    name := sanitize(t.Name()) + "_" + strconv.FormatInt(memSeq.Add(1), 10)
    dsn := "file:" + name + "?mode=memory&cache=shared"
    g, err := graph.Open(context.Background(), graph.Config{Path: dsn, InMemory: true})
    if err != nil { t.Fatalf("NewInMemoryTestGraph: %v", err) }
    t.Cleanup(func() { _ = g.Close() })
    return g
}

func sanitize(s string) string { /* replace '/' and other unsafe chars */ }
```

- [ ] **Step 3: Write tests verifying parallel-safe isolation (cross-check, not just self-check)**

```go
// Run as a table-driven test with multiple parallel siblings; each writes a
// distinct sentinel tag and asserts NO sibling's tag is visible in its DB.
func TestIsolationAcrossParallelTests(t *testing.T) {
    cases := []string{"alpha", "bravo", "charlie", "delta"}
    for _, name := range cases {
        name := name
        t.Run(name, func(t *testing.T) {
            t.Parallel()
            g := testutil.NewTestGraph(t)
            g.DoWrite(ctx, func(tx graph.WriteTx) error {
                _, err := tx.ExecContext(ctx, "INSERT INTO tags(name) VALUES (?)", name)
                return err
            })

            // Self-check: my own tag is present, count=1
            var mine int
            g.DoRead(ctx, func(tx graph.ReadTx) error {
                return tx.QueryRowContext(ctx, "SELECT count(*) FROM tags WHERE name=?", name).Scan(&mine)
            })
            if mine != 1 { t.Fatalf("self-tag missing: count=%d", mine) }

            // Cross-check: NO sibling's tag is visible
            for _, other := range cases {
                if other == name { continue }
                var n int
                g.DoRead(ctx, func(tx graph.ReadTx) error {
                    return tx.QueryRowContext(ctx, "SELECT count(*) FROM tags WHERE name=?", other).Scan(&n)
                })
                if n != 0 { t.Fatalf("isolation leak: saw sibling tag %q (count=%d)", other, n) }
            }
        })
    }
}
```

Run with `go test -count=10 -parallel=8` to stress.

- [ ] **Step 3a: Close-while-active-goroutines race test**

```go
// Catches deadlocks / double-close panics when Close() races with in-flight
// read/write transactions. Run under -race to surface data races.
func TestCloseWithInFlightTxsDoesNotPanic(t *testing.T) {
    g := testutil.NewTestGraph(t)

    var wg sync.WaitGroup
    stop := make(chan struct{})
    for i := 0; i < 4; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for {
                select {
                case <-stop: return
                default:
                    // Best-effort reads; ignore errors after Close()
                    _ = g.DoRead(ctx, func(tx graph.ReadTx) error {
                        var n int
                        return tx.QueryRowContext(ctx, "SELECT count(*) FROM nodes").Scan(&n)
                    })
                }
            }
        }()
    }

    time.Sleep(20 * time.Millisecond) // let goroutines start
    if err := g.Close(); err != nil { t.Errorf("Close returned err: %v", err) }
    close(stop)
    wg.Wait()
    // If we reach here without panic/deadlock, the test passes.
}
```

- [ ] **Step 4: Commit**

```bash
git add internal/graph/testutil/
git commit -m "feat(graph/testutil): NewTestGraph (tempfile) + NewInMemoryTestGraph"
```

**Acceptance Criteria:**
- [ ] `NewTestGraph(t)` returns a `*Graph` with all migrations applied, cleanup wired via `t.Cleanup`
- [ ] `TestIsolationAcrossParallelTests` passes — every sibling sees only its own tag, never a peer's
- [ ] `TestCloseWithInFlightTxsDoesNotPanic` passes under `-race` (no panic, no deadlock, no race report)
- [ ] `NewInMemoryTestGraph(t)` × 10 parallel invocations isolated by unique DSN

---

# EPIC 4: Lane A — Per-type CRUD (12 node types)

Twelve files implementing the same template against `NodeSpec` + chokepoint. All children of this epic can be implemented in parallel after Epic 3 lands. Each task delivers one type file + its tests.

**Bead Mapping:**
- type: `epic`
- Priority: `1`
- Estimated Minutes: 0
- Dependencies: `Epic 3`
- Parent: `none`
- Status: `open`

## File template (apply to every Lane A child)

```go
package nodes

import (
    "context"
    "encoding/json"
    "strings"
    "time"
    "github.com/gabrielassisxyz/kernl/internal/graph"
)

type <Type> struct {
    Meta
    Title string
    // ... type-specific fields (see attrs table below)
    Tags  []string
}

func (n *<Type>) NodeType() string  { return "<lower_type>" }
func (n *<Type>) NodeTitle() string { return n.Title }
func (n *<Type>) NodeAttrs() (json.RawMessage, error) {
    return json.Marshal(struct{ /* fields excluding Meta+Title */ }{ /* ... */ })
}
func (n *<Type>) NodeTags() []string { return n.Tags }
func (n *<Type>) FTSFields() FTSFields {
    return FTSFields{
        Title: n.Title,
        Body:  /* see FTS body mapping */,
        Tags:  strings.Join(n.Tags, " "),
    }
}

type <Type>Filter struct { /* see filter fields per type */ }

func Create<Type>(ctx context.Context, tx graph.WriteTx, n *<Type>, author Author) error {
    return CreateNode(ctx, tx, n, author)
}
func Get<Type>(ctx context.Context, tx graph.ReadTx, id string) (*<Type>, error) { /* load + unmarshal */ }
func Update<Type>(ctx context.Context, tx graph.WriteTx, n *<Type>, author Author) error {
    return UpdateNode(ctx, tx, n, author)
}
func Delete<Type>(ctx context.Context, tx graph.WriteTx, id string, author Author) error {
    return DeleteNode(ctx, tx, id, author)
}
func List<Type>s(ctx context.Context, tx graph.ReadTx, f <Type>Filter) ([]*<Type>, error) { /* type-scoped query */ }
```

## Per-type attrs + FTS body mapping

| Type | attrs fields | FTS `Body` source |
|---|---|---|
| **Bead** | `Description string`, `Status string`, `Priority int`, `AssigneeID *string` | `Description` |
| **Note** | `Body string`, `Path string`, `Frontmatter json.RawMessage` | `Body` *(implements `DiffableNode`)* |
| **Project** | `Description string`, `Status string`, `StartedAt *time.Time`, `ClosedAt *time.Time` | `Description` |
| **Task** | `Description string`, `Status string`, `Priority int`, `ProjectID *string`, `DueAt *time.Time` | `Description` |
| **Session** | `StartedAt time.Time`, `EndedAt *time.Time`, `AgentID string`, `Summary string` | `Summary` |
| **Decision** | `Body string`, `Context string`, `Outcome string`, `DecidedAt time.Time` | `Body + " " + Context + " " + Outcome` |
| **MemoryClaim** | `Statement string`, `Confidence float64`, `Subject string`, `Source string` | `Statement` |
| **MemoryRefutation** | `ClaimID string`, `Reason string`, `Confidence float64` | `Reason` |
| **Bookmark** | `URL string`, `Description string`, `ArchivedAt *time.Time`, `Excerpt string` | `Description + " " + Excerpt` |
| **BookmarkList** | `Description string` | `Description` |
| **Capture** | `Body string`, `CapturedFrom string` | `Body` |
| **WorkflowRun** | `WorkflowName string`, `Status string`, `StartedAt time.Time`, `EndedAt *time.Time`, `RunData json.RawMessage` | `WorkflowName + " " + Status` |

> **Note on `Note.Body`:** in P0.1 the `Body` field is *substrate cache*, populated by callers. The filesystem is the source of truth for user notes per VISION §6.2; the watcher in P0.2 owns the FS↔SQLite synchronization. P0.1's job is just to store/index whatever `Body` text is passed in.

## Per-type Filter (List funcs)

Every `List<Type>s` takes a `<Type>Filter` with:
- Common: `Limit int` (0 → 100 default), `UpdatedSince *time.Time`, `Tags []string` (AND-match)
- Type-specific subset (e.g., `BeadFilter` adds `Status string`, `AssigneeID string`; `BookmarkFilter` adds `IncludeArchived bool`)

Type filters that hit `json_extract(attrs, '$.X')` accept the scan cost per Decision 13 of the review. Expression indexes deferred.

## Per-type tests (apply to every Lane A child)

Each type file ships with a `*_test.go` containing:
1. **Roundtrip:** `Create<Type>` → `Get<Type>` returns identical content (incl. all attrs).
2. **Update produces revision:** `Update<Type>` writes one revision row with correct author.
3. **Delete preserves revisions:** post-delete, revisions table has create+update+tombstone (regression for schema fix).
4. **FTS round-trip:** body text indexed → `Search` returns the node by a word from `Body`.
5. **Filter:** `List<Type>s(filter)` returns only matching rows.

---

## Tasks 4.1 – 4.12 (one per type, generated from template above)

Each Lane A task is structurally identical: implement the template, fill in the per-type attrs, write the 5 tests, commit. Estimated 60 minutes each.

---

### Task 4.1: `Bead`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 60
- Dependencies: `Task 3.4`, `Task 3.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/bead.go`
- Create: `internal/graph/nodes/bead_test.go`

**Description / Steps:**
- [ ] Apply the file template with `<Type>=Bead`, attrs from table, FTS body = `Description`.
- [ ] `BeadFilter` adds `Status string`, `AssigneeID string`, `MinPriority int`.
- [ ] Write the 5 standard per-type tests against `testutil.NewTestGraph(t)`.
- [ ] Run `go test ./internal/graph/nodes/ -race -run Bead` and verify all pass.
- [ ] Commit: `feat(graph/nodes): Bead CRUD + filter`.

**Acceptance Criteria:**
- [ ] Roundtrip test passes (Create → Get → identical)
- [ ] Update revision test passes (exactly 1 new revision row, correct author)
- [ ] Delete-preserves-revisions test passes (3 rows post-delete)
- [ ] FTS round-trip test passes
- [ ] `ListBeads(BeadFilter{Status: "open", Limit: 10})` returns only matching beads

---

### Task 4.2: `Note` (implements `DiffableNode`)

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 90
- Dependencies: `Task 3.4`, `Task 3.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/note.go`
- Create: `internal/graph/nodes/note_test.go`

**Description / Steps:**
- [ ] Apply template with `<Type>=Note`, attrs `Body string, Path string, Frontmatter json.RawMessage`.
- [ ] Implement `DiffBody(prev NodeSpec) string` using an inline line-by-line diff (~30 LOC, no new dependency): split old/new body by `\n`, find longest common subsequence of lines, emit `+`/`-` prefix output. Sufficient for MVP audit fidelity.
  - **Scale note (do NOT implement now):** the LCS implementation is O(N×M) in time and memory over the line count. Acceptable at MVP write rates (5s autosave, typical note <500 lines). If notes routinely exceed ~5k lines and write latency becomes user-visible, the upgrade path is **Myers diff (O(ND))** — same input/output contract, swap the LCS body for Myers. Document this as a follow-up bead with concrete trigger criteria (write p95 > 50ms on a 10k-line note) when filing the post-P0.1 TODOs.
- [ ] Standard 5 tests + 1 extra: **`TestNoteUpdateStoresDiff`** — after Update, the new revision row has non-NULL `diff` column containing both old and new body text.
- [ ] Commit: `feat(graph/nodes): Note CRUD + DiffableNode impl`.

**Acceptance Criteria:**
- [ ] All 5 standard tests pass
- [ ] `TestNoteUpdateStoresDiff` passes; diff column contains a textual diff
- [ ] First revision after Create has `diff=NULL` (no prev to diff against)

---

### Task 4.3: `Project`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 60
- Dependencies: `Task 3.4`, `Task 3.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/project.go`
- Create: `internal/graph/nodes/project_test.go`

**Description / Steps:** Apply template with attrs from table. `ProjectFilter` adds `Status string`, `ActiveOnly bool` (translates to `ClosedAt IS NULL`). Standard 5 tests. Commit.

**Acceptance Criteria:** All 5 standard tests pass; `ListProjects` with `ActiveOnly=true` excludes closed projects.

---

### Task 4.4: `Task`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 60
- Dependencies: `Task 3.4`, `Task 3.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/task.go`
- Create: `internal/graph/nodes/task_test.go`

**Description / Steps:** Template + attrs. `TaskFilter` adds `Status string`, `ProjectID string`, `DueBefore *time.Time`. Standard 5 tests. Commit.

**Acceptance Criteria:** All 5 standard tests pass.

---

### Task 4.5: `Session`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 60
- Dependencies: `Task 3.4`, `Task 3.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/session.go`
- Create: `internal/graph/nodes/session_test.go`

**Description / Steps:** Template + attrs. `SessionFilter` adds `AgentID string`, `Active bool` (translates to `EndedAt IS NULL`). Standard 5 tests. Commit.

**Acceptance Criteria:** All 5 standard tests pass.

---

### Task 4.6: `Decision`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 60
- Dependencies: `Task 3.4`, `Task 3.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/decision.go`
- Create: `internal/graph/nodes/decision_test.go`

**Description / Steps:** Template + attrs (`Body, Context, Outcome, DecidedAt`). FTS body concatenates the three text fields. `DecisionFilter` adds `Since *time.Time`. Standard 5 tests. Commit.

**Acceptance Criteria:** All 5 standard tests pass; FTS finds Decisions by words in any of Body/Context/Outcome.

---

### Task 4.7: `MemoryClaim`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 60
- Dependencies: `Task 3.4`, `Task 3.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/memory_claim.go`
- Create: `internal/graph/nodes/memory_claim_test.go`

**Description / Steps:** Template + attrs (`Statement, Confidence, Subject, Source`). `MemoryClaimFilter` adds `Subject string`, `MinConfidence float64`. Note: additive write contract is **P2.2's** concern, not this task — P0.1 just stores the rows. Standard 5 tests. Commit.

**Acceptance Criteria:** All 5 standard tests pass.

---

### Task 4.8: `MemoryRefutation`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 60
- Dependencies: `Task 3.4`, `Task 3.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/memory_refutation.go`
- Create: `internal/graph/nodes/memory_refutation_test.go`

**Description / Steps:** Template + attrs (`ClaimID, Reason, Confidence`). `MemoryRefutationFilter` adds `ClaimID string`. Standard 5 tests. Commit.

**Acceptance Criteria:** All 5 standard tests pass.

---

### Task 4.9: `Bookmark`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 60
- Dependencies: `Task 3.4`, `Task 3.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/bookmark.go`
- Create: `internal/graph/nodes/bookmark_test.go`

**Description / Steps:** Template + attrs (`URL, Description, ArchivedAt, Excerpt`). `BookmarkFilter` adds `IncludeArchived bool` (default false → `ArchivedAt IS NULL`). Standard 5 tests. Commit.

**Acceptance Criteria:** All 5 standard tests pass; `ListBookmarks(BookmarkFilter{})` excludes archived bookmarks by default.

---

### Task 4.10: `BookmarkList`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 45
- Dependencies: `Task 3.4`, `Task 3.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/bookmark_list.go`
- Create: `internal/graph/nodes/bookmark_list_test.go`

**Description / Steps:** Template + attrs (`Description string`). `BookmarkListFilter` has only the common fields. Standard 5 tests. Commit.

**Acceptance Criteria:** All 5 standard tests pass.

---

### Task 4.11: `Capture`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 45
- Dependencies: `Task 3.4`, `Task 3.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/capture.go`
- Create: `internal/graph/nodes/capture_test.go`

**Description / Steps:** Template + attrs (`Body, CapturedFrom`). `CaptureFilter` adds `CapturedFromPrefix string`. Standard 5 tests. Commit.

**Acceptance Criteria:** All 5 standard tests pass.

---

### Task 4.12: `WorkflowRun`

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 60
- Dependencies: `Task 3.4`, `Task 3.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/workflow_run.go`
- Create: `internal/graph/nodes/workflow_run_test.go`

**Description / Steps:** Template + attrs (`WorkflowName, Status, StartedAt, EndedAt, RunData`). `WorkflowRunFilter` adds `WorkflowName string`, `Status string`. Standard 5 tests. Commit.

**Acceptance Criteria:** All 5 standard tests pass.

---

# EPIC 5: Lane B — Edges, Tags, FTS Search, Revisions read

Four sibling packages providing the non-node substrate surfaces. All four are independent and can be implemented in parallel after Epic 3 lands.

**Bead Mapping:**
- type: `epic`
- Priority: `1`
- Estimated Minutes: 0
- Dependencies: `Epic 3`
- Parent: `none`
- Status: `open`

---

## Task 5.1: Edges package

`internal/graph/edges/edge.go`. Closed `Type` enum, `Create/Delete/Outgoing/Incoming` functions. No revisions, no audit log. Cascade-deletes via FK when source or dest node is deleted.

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 90
- Dependencies: `Epic 3`, `Task 4.1`
- Parent: `Epic 5`
- Status: `open`

> Dep on `Task 4.1` (Bead): the edge tests use `nodes.CreateBead` to create the src/dst nodes. Alternative would be a `fakeSpec` helper in the edges package, but reusing Bead keeps the test fixture realistic and avoids duplicating a NodeSpec implementer.

**Files:**
- Create: `internal/graph/edges/edge.go`
- Create: `internal/graph/edges/edge_test.go`

**Description / Steps:**

- [ ] **Step 1: Define `Type` enum and `Edge` struct**

```go
package edges

type Type string

const (
    DependsOn     Type = "depends_on"
    ParentOf      Type = "parent_of"
    InspiredBy    Type = "inspired_by"
    Mentions      Type = "mentions"
    GeneratedFrom Type = "generated_from"
    ProcessedInto Type = "processed_into"
    ProcessedFrom Type = "processed_from"
    Refutes       Type = "refutes"
    RelatesTo     Type = "relates_to"
)

type Edge struct {
    ID        string
    SrcID     string
    DstID     string
    Type      Type
    Attrs     json.RawMessage
    CreatedAt time.Time
}
```

- [ ] **Step 2: CRUD funcs**

```go
func Create(ctx context.Context, tx graph.WriteTx, src, dst string, t Type, payload any) (*Edge, error)
func Delete(ctx context.Context, tx graph.WriteTx, edgeID string) error
func Outgoing(ctx context.Context, tx graph.ReadTx, nodeID string, types ...Type) ([]*Edge, error)
func Incoming(ctx context.Context, tx graph.ReadTx, nodeID string, types ...Type) ([]*Edge, error)
```

- [ ] **Step 3: Standard tests**

```go
func TestCreateAndOutgoing(t *testing.T) { /* create A,B,edge → Outgoing(A) returns edge */ }
func TestDeleteSourceCascadesEdge(t *testing.T) { /* delete A → edge gone */ }
func TestDeleteDestCascadesEdge(t *testing.T) { /* delete B → edge gone */ }
```

- [ ] **Step 4: Gap tests (#5, #6 from test plan)**

```go
func TestSelfEdgeAllowed(t *testing.T) {
    // Schema allows it; document policy: caller validates if undesired
    g := testutil.NewTestGraph(t)
    nodes.CreateBead(...) // bead X
    _, err := g.DoWriteEdge(..., X, X, edges.RelatesTo)
    if err != nil { t.Fatal("self-edge should be allowed") }
}

func TestEdgeToNonexistentDestFails(t *testing.T) {
    // Proves foreign_keys=ON
    g := testutil.NewTestGraph(t)
    nodes.CreateBead(...) // bead X
    _, err := edges.Create(ctx, tx, "X", "NONEXISTENT", edges.Mentions, nil)
    if err == nil { t.Fatal("expected FK violation") }
}
```

- [ ] **Step 5: Filter by type test**

```go
func TestOutgoingFiltersByType(t *testing.T) {
    // Create A, B, C; edges A→B (depends_on), A→C (mentions)
    // Outgoing(A, DependsOn) returns only A→B
}
```

- [ ] **Step 6: Commit**

```bash
git add internal/graph/edges/
git commit -m "feat(graph/edges): typed directed edges with cascade"
```

**Acceptance Criteria:**
- [ ] All standard tests pass
- [ ] `TestSelfEdgeAllowed` passes
- [ ] `TestEdgeToNonexistentDestFails` returns FK violation
- [ ] `TestOutgoingFiltersByType` filters correctly
- [ ] No edge audit log table created (confirms eng review decision)

---

## Task 5.2: Tags package

`internal/graph/tags/tags.go`. Open-vocabulary `Add/Remove/List/Nodes`. Note that `updateNode` in the chokepoint already reconciles tags from `NodeSpec.NodeTags()`; this package is for ad-hoc tag operations outside a node update.

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 60
- Dependencies: `Epic 3`
- Parent: `Epic 5`
- Status: `open`

**Files:**
- Create: `internal/graph/tags/tags.go`
- Create: `internal/graph/tags/tags_test.go`

**Description / Steps:**

- [ ] **Step 1: Package surface**

```go
package tags

func Add(ctx context.Context, tx graph.WriteTx, nodeID, tag string) error
func Remove(ctx context.Context, tx graph.WriteTx, nodeID, tag string) error
func List(ctx context.Context, tx graph.ReadTx, nodeID string) ([]string, error)
func Nodes(ctx context.Context, tx graph.ReadTx, tag string) ([]string, error)
```

`Add` upserts into `tags` then inserts into `node_tags`. Empty tag → reject with `fmt.Errorf("graph: tags add: empty tag")`.

- [ ] **Step 2: Tests**

```go
func TestAddAndList(t *testing.T) { /* Add → List returns it */ }
func TestNodesByTag(t *testing.T) { /* Add tag to two nodes → Nodes returns both */ }
func TestRemove(t *testing.T) { /* Add then Remove → List empty */ }
func TestAddEmptyTagRejected(t *testing.T) { /* err non-nil */ }
func TestCascadeOnNodeDelete(t *testing.T) {
    // Add tag, delete node → node_tags row gone (cascade)
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/graph/tags/
git commit -m "feat(graph/tags): open-vocabulary tags with cascade"
```

**Acceptance Criteria:**
- [ ] All 5 tests pass
- [ ] Empty tag rejected at API
- [ ] Node delete cascades to `node_tags`

---

## Task 5.3: FTS Search package

`internal/graph/fts/search.go`. Pure textual search with `WithTypes`, `WithLimit`, `WithTagsFilter` options. Wraps FTS5 syntax errors in `ErrFTSQuerySyntax`. Joins on `nodes.fts_rowid` (no map table).

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 120
- Dependencies: `Epic 3`, `Task 4.1`, `Task 4.2`
- Parent: `Epic 5`
- Status: `open`

> Dep on `Task 4.1` (Bead) and `Task 4.2` (Note): tests below seed the FTS index via `nodes.CreateNote` / `nodes.CreateBead`. Same rationale as Task 5.1 — realistic fixtures over duplicated `fakeSpec`.

**Files:**
- Create: `internal/graph/fts/search.go`
- Create: `internal/graph/fts/search_test.go`

**Description / Steps:**

- [ ] **Step 1: Package surface**

```go
package fts

type Hit struct {
    NodeID string
    Type   string
    Title  string
    Rank   float64 // bm25
}

type Option func(*opts)
type opts struct{ types []string; limit int; tags []string }

func WithTypes(types ...string) Option       { return func(o *opts){ o.types = types } }
func WithLimit(n int) Option                  { return func(o *opts){ o.limit = n } }
func WithTagsFilter(tags ...string) Option   { return func(o *opts){ o.tags = tags } }

func Search(ctx context.Context, tx graph.ReadTx, query string, options ...Option) ([]Hit, error)
```

- [ ] **Step 2: Implement core search SQL**

```sql
SELECT n.id, n.type, n.title, bm25(nodes_fts) AS rank
FROM nodes_fts
JOIN nodes n ON n.fts_rowid = nodes_fts.rowid
WHERE nodes_fts MATCH ?
  AND (?1 OR n.type IN (...))             -- types filter
  AND (?2 OR EXISTS (SELECT 1 FROM node_tags nt
                     WHERE nt.node_id=n.id AND nt.tag IN (...) ))   -- tags filter
ORDER BY rank
LIMIT ?
```

- [ ] **Step 3: FTS5 error wrapping**

```go
rows, err := tx.QueryContext(ctx, query, ...)
if err != nil {
    // Modernc.org/sqlite error string for FTS5 parse failure:
    //   "fts5: syntax error near ..."
    // Document and regression-test this exact substring. If the driver changes
    // format, the regression test below will fail loudly and we will update.
    if err.Error() != "" && strings.Contains(err.Error(), "fts5: syntax error") {
        return nil, fmt.Errorf("graph: search: %w: %v", graph.ErrFTSQuerySyntax, err)
    }
    return nil, fmt.Errorf("graph: search: %w", err)
}
```

- [ ] **Step 4: Standard test — basic search returns hit**

```go
func TestSearchReturnsHit(t *testing.T) {
    g := testutil.NewTestGraph(t)
    nodes.CreateNote(..., body="the quick brown fox")
    hits, _ := fts.Search(ctx, tx, "fox")
    if len(hits) != 1 { t.Fatal(...) }
}
```

- [ ] **Step 5: Gap #1 — FTS5 special chars in body**

```go
func TestFTSSpecialCharsInBody(t *testing.T) {
    g := testutil.NewTestGraph(t)
    for _, body := range []string{`hello "world"`, `star*here`, `caret^test`, `tilde~zone`, `colon:tag`} {
        nodes.CreateNote(..., body=body)
    }
    // Search with FTS5-escaped versions returns expected hits without panic
    hits, err := fts.Search(ctx, tx, `"hello"`)
    // ...
}
```

- [ ] **Step 6: Gap #2 — Portuguese diacritics + mixed EN/PT body**

```go
func TestFTSPortugueseDiacritics(t *testing.T) {
    g := testutil.NewTestGraph(t)
    nodes.CreateNote(..., body="ação coração")
    hits, _ := fts.Search(ctx, tx, "acao")
    if len(hits) != 1 { t.Fatalf("remove_diacritics=2 not working") }
}

// Test plan §FTS5 — "Mixed-language body (EN + PT) → both languages searchable".
// Single note, body has English and Portuguese; each language's terms must hit.
func TestFTSMixedEnglishPortuguese(t *testing.T) {
    g := testutil.NewTestGraph(t)
    nodes.CreateNote(..., body="the cat slept on the sofá while the cachorro barked")

    for _, term := range []string{"cat", "slept", "sofa", "cachorro"} {
        hits, err := fts.Search(ctx, tx, term)
        if err != nil { t.Fatalf("term %q: %v", term, err) }
        if len(hits) != 1 { t.Fatalf("term %q: hits=%d, want 1", term, len(hits)) }
    }
}
```

- [ ] **Step 7: Gap #3 — Empty / single-char / max-length queries**

**Empty query decision:** an empty string is not a valid FTS5 query. `Search` returns `ErrFTSQuerySyntax` (wrapped) without ever touching the DB. Test asserts that contract.

```go
func TestFTSEmptyQuery(t *testing.T) {
    g := testutil.NewTestGraph(t)
    _, err := fts.Search(ctx, tx, "")
    if !errors.Is(err, graph.ErrFTSQuerySyntax) {
        t.Fatalf("empty query err=%v, want ErrFTSQuerySyntax", err)
    }
}
func TestFTSMaxLengthQuery(t *testing.T) {
    g := testutil.NewTestGraph(t)
    q := strings.Repeat("a", 10_000)
    hits, err := fts.Search(ctx, tx, q)
    // Pass condition: no panic. Either zero hits or wrapped ErrFTSQuerySyntax.
    // The driver must not return a raw fts5 internal error.
    if err != nil && !errors.Is(err, graph.ErrFTSQuerySyntax) {
        t.Fatalf("max-length query returned unwrapped err: %v", err)
    }
    _ = hits
}
```

- [ ] **Step 8: Gap #4 — WithTypes and WithTagsFilter**

```go
func TestSearchWithTypes(t *testing.T) {
    // Create note with body "alpha" and bead with body "alpha"
    hits, _ := fts.Search(ctx, tx, "alpha", fts.WithTypes("note"))
    if len(hits) != 1 || hits[0].Type != "note" { t.Fail() }
}
func TestSearchWithTagsFilter(t *testing.T) {
    // Create two notes with body "alpha"; tag only one with "x"
    hits, _ := fts.Search(ctx, tx, "alpha", fts.WithTagsFilter("x"))
    if len(hits) != 1 { t.Fail() }
}
```

- [ ] **Step 9: Malformed FTS5 query**

```go
func TestSearchMalformedQueryReturnsWrappedError(t *testing.T) {
    // FTS5 reliably rejects a bare operator with no operands. "AND" alone
    // produces "fts5: syntax error" across all modernc.org/sqlite versions.
    // Avoid `"unbalanced` — newer FTS5 tokenizers tolerate an unmatched
    // opening quote as a partial token rather than a parse error.
    _, err := fts.Search(ctx, tx, "AND")
    if !errors.Is(err, graph.ErrFTSQuerySyntax) {
        t.Fatalf("err=%v, want ErrFTSQuerySyntax", err)
    }
}
```

- [ ] **Step 10: Commit**

```bash
git add internal/graph/fts/
git commit -m "feat(graph/fts): bm25 search with type/tag filters + error wrapping"
```

**Acceptance Criteria:**
- [ ] All listed tests pass
- [ ] `errors.Is(err, ErrFTSQuerySyntax)` works for malformed queries
- [ ] No raw FTS5 internals leaked in error messages

---

## Task 5.4: Revisions read API

`internal/graph/revisions/revisions.go`. Read-only surface — writes happen exclusively through the chokepoint. Exposes `List(nodeID)` and `GetAt(nodeID, ts)` for time-travel reads.

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 60
- Dependencies: `Epic 3`
- Parent: `Epic 5`
- Status: `open`

**Files:**
- Create: `internal/graph/revisions/revisions.go`
- Create: `internal/graph/revisions/revisions_test.go`

**Description / Steps:**

- [ ] **Step 1: Package surface**

```go
package revisions

type Revision struct {
    ID             string
    NodeID         string
    PrevRevisionID *string
    Author         string
    Snapshot       json.RawMessage
    Diff           *string  // nil unless DiffableNode
    Ts             time.Time
}

func List(ctx context.Context, tx graph.ReadTx, nodeID string) ([]*Revision, error)
func GetAt(ctx context.Context, tx graph.ReadTx, nodeID string, at time.Time) (*Revision, error)
```

`List` returns revisions ordered by `ts DESC, id DESC` (newest first; `id` as tiebreaker for wall-clock collision).
`GetAt` returns the revision active at `at` (latest revision with `ts <= at`, `id` tiebreaker).

- [ ] **Step 2: Tests**

```go
func TestListReturnsAllRevisions(t *testing.T) {
    // Create + 2 updates → List returns 3 in ts, id order
}

func TestGetAtReturnsHistoricalState(t *testing.T) {
    // Create at t1, Update at t2, Update at t3
    // GetAt(t2 + 1ms) returns revision 2's snapshot
    // GetAt(t1) returns revision 1's snapshot
}

func TestPrevRevisionChainUnbroken(t *testing.T) {  // gap #7 from test plan
    // After N updates, walking from latest via prev_revision_id visits all N
}

func TestListOrderDeterministicUnderCollision(t *testing.T) {
    // Two rapid updates issued in sequence → List order matches creation order
    // (ts may collide but id/UUIDv7 tiebreaks reliably)
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/graph/revisions/
git commit -m "feat(graph/revisions): read-only revision history + GetAt time-travel"
```

**Acceptance Criteria:**
- [ ] All 3 tests pass
- [ ] `TestPrevRevisionChainUnbroken` confirms gap #7
- [ ] No `Create`/`Update`/`Delete` exported — write surface is exclusively the chokepoint

---

## Task 5.5: Integration test suite covering spec §11 success criteria

The brainstorm spec §11 defines 8 end-to-end success criteria for "P0.1 is done". The plan covers them implicitly through per-package tests in Lanes A and B, but no single file ties them to the spec line items. This task creates `internal/graph/integration_test.go` with **one named test per §11 criterion** so the spec mapping is auditable and the "done" definition is mechanically verifiable.

**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 90
- Dependencies: `Epic 4`, `Epic 5`
- Parent: `none`
- Status: `open`

**Files:**
- Create: `internal/graph/integration_test.go`

**Description / Steps:**

- [ ] **Step 1: One test per spec §11 criterion**

```go
package graph_test

// Each test maps 1:1 to a numbered item in
// docs/2026-05-17-graph-substrate-brainstorm-spec.md §11. Keep the names
// stable so the spec ↔ test mapping survives refactors.

func TestSpec11_01_CreateGetBeadRoundtrip(t *testing.T)         { /* Open → CreateBead → GetBead identical */ }
func TestSpec11_02_UpdateBeadProducesOneRevision(t *testing.T)  { /* UpdateBead → exactly 1 new revision with correct author + full snapshot */ }
func TestSpec11_03_NoteBodyFTSReturnsHit(t *testing.T)          { /* CreateNote(body) → Search(word) returns it */ }
func TestSpec11_04_NoteBodyFTSReplacedOnUpdate(t *testing.T)    { /* Create + Update → old word gone, new word found */ }
func TestSpec11_05_EdgeCreateAndCascade(t *testing.T)           { /* CreateEdge(A→B, depends_on) → Outgoing(A) → DeleteNode(A) → edge gone, revisions of A preserved */ }
func TestSpec11_06_TagAddListNodesRemove(t *testing.T)          { /* AddTag → tags.Nodes returns it → RemoveTag reverses */ }
func TestSpec11_07_PersistenceAcrossReopen(t *testing.T)        { /* Close + reopen tempfile → all data persists, FTS still answers */ }
func TestSpec11_08_PropertyTestPasses(t *testing.T)             { /* Thin wrapper that invokes the rapid property test — proves §11 #8 covered */ }
```

- [ ] **Step 2: Commit**

```bash
git add internal/graph/integration_test.go
git commit -m "test(graph): integration_test.go covers all 8 spec §11 success criteria"
```

**Acceptance Criteria:**
- [ ] One named test per §11 criterion exists in `internal/graph/integration_test.go`
- [ ] Each test name embeds the spec section reference (`TestSpec11_NN_*`) so a `grep -c TestSpec11_` returns 8
- [ ] All 8 tests pass under `go test ./internal/graph/...`
- [ ] When a §11 criterion changes in a future spec revision, the corresponding test name surfaces in `git grep` immediately

---

# TASK 6: Lane C — Property tests + integration sweep

Closes test plan gaps #11, #12, #13 (concurrency, WAL isolation, UUIDv7 sortable). Uses `pgregory.net/rapid` for the random-op property test that exercises all chokepoint invariants over 1000 ops.

**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: 270
- Dependencies: `Epic 4`, `Epic 5`, `Task 5.5`
- Parent: `none`
- Status: `open`

**Files:**
- Create: `internal/graph/property_test.go`
- Create: `internal/graph/concurrency_test.go`
- Create: `internal/graph/bench_test.go`
- Modify: `go.mod` — add `pgregory.net/rapid`

**Description / Steps:**

- [ ] **Step 1: Add `pgregory.net/rapid` dep**

```bash
go get pgregory.net/rapid@latest
go mod tidy
```

- [ ] **Step 2: Define the property test state machine**

```go
package graph_test

import "pgregory.net/rapid"

type model struct {
    g        *graph.Graph
    nodes    map[string]nodes.NodeSpec  // mirror of live nodes
    edges    map[string]edges.Edge      // mirror of live edges
    revCount map[string]int             // expected revision count per node
}

// Spec §10 #3: property tests run on in-memory storage for speed.
// rapid runs ~100 shrunk cases × 1000 ops each; tempfile+WAL would
// dominate wall-clock under -count=10 (Step 6). NewInMemoryTestGraph
// uses a unique DSN per call (cache=shared) so both pool handles share
// one in-memory DB with full hermetic isolation per t.Run.
func TestSubstrateProperties(t *testing.T) {
    rapid.Check(t, func(rt *rapid.T) {
        m := &model{g: testutil.NewInMemoryTestGraph(t), nodes: map[string]nodes.NodeSpec{}, ...}
        for i := 0; i < 1000; i++ {
            op := rapid.SampledFrom([]string{"create","update","delete","add_tag","add_edge","delete_edge"}).Draw(rt, "op")
            m.apply(rt, op)
            m.checkInvariants(rt)
        }
    })
}
```

- [ ] **Step 3: Implement `checkInvariants`**

Invariants enforced after every op:
1. **Exactly one revision per mutation:** `sum(revCount) == count(revisions)`.
2. **FTS reflects current state:** for every live node, `Search(uniqueTokenInBody)` returns exactly that node.
3. **Cascade preserves revisions:** for every deleted node, revisions count ≥ pre-delete count (tombstone added, no rows lost).
4. **Cascade removes edges/tags/fts:** for every deleted node, no edges/tags/fts rows remain referencing it.
5. **UUIDv7 monotonic:** newly created IDs sort lexicographically after all prior IDs.

- [ ] **Step 4: Gap #11 — concurrent WriteTx**

```go
func TestConcurrentWritesSerialize(t *testing.T) {
    g := testutil.NewTestGraph(t)
    const N = 100
    var wg sync.WaitGroup
    var busyCount int32
    for i := 0; i < N; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            err := g.DoWrite(ctx, func(tx graph.WriteTx) error {
                return nodes.CreateBead(ctx, tx, &nodes.Bead{Meta: nodes.Meta{ID: fmt.Sprintf("b%d", i)}, Title: "x"}, nodes.AuthorHuman)
            })
            if err != nil && strings.Contains(err.Error(), "SQLITE_BUSY") {
                atomic.AddInt32(&busyCount, 1)
            }
        }(i)
    }
    wg.Wait()
    if busyCount != 0 { t.Fatalf("%d writes saw SQLITE_BUSY (write pool should serialize)", busyCount) }
    // Verify all N beads exist
}
```

- [ ] **Step 5: Gap #12 — reader sees pre-delete under WAL**

```go
func TestReaderSeesPreDeleteSnapshot(t *testing.T) {
    g := testutil.NewTestGraph(t)
    // Create bead X
    g.DoWrite(ctx, func(tx graph.WriteTx) error { return nodes.CreateBead(ctx, tx, &nodes.Bead{Meta: nodes.Meta{ID:"X"}, Title:"x"}, nodes.AuthorHuman) })

    // BARRIER: ensure the reader's tx is open BEFORE we delete. Without this
    // signal, main can win the race and delete X before BeginTx — the reader
    // would then see post-delete state and the test would flake.
    readStarted := make(chan struct{})
    readDone    := make(chan struct{})
    readResult  := make(chan error, 1)
    go func() {
        readResult <- g.DoRead(ctx, func(tx graph.ReadTx) error {
            close(readStarted) // tx now open; snapshot acquired under WAL
            <-readDone         // hold the snapshot until main releases
            b, err := nodes.GetBead(ctx, tx, "X")
            if err != nil { return fmt.Errorf("reader error: %w", err) }
            if b == nil   { return fmt.Errorf("reader lost view of X under WAL") }
            return nil
        })
    }()

    <-readStarted // guaranteed: snapshot captured before delete
    g.DoWrite(ctx, func(tx graph.WriteTx) error { return nodes.DeleteBead(ctx, tx, "X", nodes.AuthorHuman) })

    // Release reader, collect result in main goroutine (no t.Fatal in goroutine)
    close(readDone)
    if err := <-readResult; err != nil {
        t.Fatal(err)
    }
}
```

- [ ] **Step 5a: Bulk-insert latency benchmark (test plan §Boundary inputs)**

```go
// 1000 sequential bead inserts under the write pool. The test asserts both
// throughput (aggregate time bound) and per-query latency afterwards. The
// numbers are profile-driven thresholds, not hard contracts — if a future
// change blows past 50ms aggregate, that is a signal to investigate, not
// necessarily a regression to revert. Adjust thresholds with profiling data.
func TestBulkInsertLatency(t *testing.T) {
    g := testutil.NewTestGraph(t)
    const N = 1000

    start := time.Now()
    for i := 0; i < N; i++ {
        i := i
        if err := g.DoWrite(ctx, func(tx graph.WriteTx) error {
            return nodes.CreateBead(ctx, tx, &nodes.Bead{
                Meta:  nodes.Meta{ID: fmt.Sprintf("b%04d", i)},
                Title: fmt.Sprintf("bead %d", i),
            }, nodes.AuthorHuman)
        }); err != nil { t.Fatalf("insert %d: %v", i, err) }
    }
    insertElapsed := time.Since(start)

    var count int
    queryStart := time.Now()
    g.DoRead(ctx, func(tx graph.ReadTx) error {
        return tx.QueryRowContext(ctx, "SELECT count(*) FROM nodes WHERE type='bead'").Scan(&count)
    })
    queryElapsed := time.Since(queryStart)

    if count != N { t.Fatalf("count=%d, want %d", count, N) }
    t.Logf("bulk insert: %d in %v (%.2f µs/op); count query: %v",
        N, insertElapsed, float64(insertElapsed.Microseconds())/float64(N), queryElapsed)

    // Soft bound — flag for investigation, don't fail unless wildly off
    if insertElapsed > 5*time.Second {
        t.Errorf("bulk insert took %v for %d ops — investigate", insertElapsed, N)
    }
    if queryElapsed > 50*time.Millisecond {
        t.Errorf("count query took %v over %d rows — investigate", queryElapsed, N)
    }
}
```

- [ ] **Step 6: Run with race + parallel**

```bash
go test ./internal/graph/... -race -count=10 -timeout=180s
```

- [ ] **Step 7: Commit**

```bash
git add internal/graph/property_test.go internal/graph/concurrency_test.go go.mod go.sum
git commit -m "test(graph): property tests (rapid) + concurrency gaps #11/#12"
```

**Acceptance Criteria:**
- [ ] `TestSubstrateProperties` passes for 1000 ops × default rapid runs (~100 shrunk cases), running on `NewInMemoryTestGraph` per spec §10 #3
- [ ] All 5 invariants enforced after each op (failures must shrink to minimal reproducer)
- [ ] `TestConcurrentWritesSerialize` passes with 0 `SQLITE_BUSY` errors across 100 concurrent writes
- [ ] `TestReaderSeesPreDeleteSnapshot` passes deterministically (the `readStarted` barrier eliminates the start-order race; no flakes under `-count=10`)
- [ ] `TestBulkInsertLatency` passes: 1000 inserts complete under 5s, count query under 50ms (soft bounds, fail loudly if breached)
- [ ] Full `go test ./internal/graph/... -race -count=10` is green

---

## Final acceptance — P0.1 is "done" when:

- [ ] All tasks above closed
- [ ] `go test ./internal/graph/... -race` green (single run)
- [ ] `go test ./internal/graph/... -race -count=10 -timeout=180s` green (stress run)
- [ ] All 8 success criteria from spec §11 demonstrably pass via `internal/graph/integration_test.go` (one named test per criterion; `grep -c TestSpec11_` returns 8)
- [ ] All 13 test plan gaps have corresponding tests in the repo
- [ ] No `KERNL DISPATCH FAILURE` strings in `internal/graph/...` (idiomatic error format only)
- [ ] `revisions.node_id` has no `ON DELETE CASCADE` (verifiable via `PRAGMA foreign_key_list(revisions)`)
- [ ] `nodes.fts_rowid INTEGER UNIQUE` column present (no `nodes_fts_map` table)
- [ ] `edges.owner_id` and `edges.visibility` columns present (nullable; VISION §16 multi-user posture)
- [ ] Three follow-up TODOs filed as beads (per review): expression indexes with trigger criteria, Tx-forgery compile-fail test, runstate error-format convergence
- [ ] One follow-up TODO filed: Myers diff upgrade for `Note.DiffBody` (trigger: write p95 > 50ms on 10k-line notes)

> **Note on CI:** P0.1 does not ship a `.github/workflows/graph-tests.yml`. CI integration for the substrate is deferred — local `go test ./internal/graph/... -race` is the "done" gate during this project's lifetime. Adding the workflow file is a downstream chore once the substrate stabilizes.

---

## Notes (refinement deltas — track Yegge Loop iterations here)

_Iteration 1 (2026-05-17): initial draft based on spec + eng review + test plan. Open questions from prompt resolved as documented in "Decisions resolved in this plan" section. Awaiting user review for Yegge Loop iterations 2+._

_Iteration 2 (2026-05-17): user-driven review against spec, eng review, test plan, and VISION §6/§7.2/§14/§15/§16. Applied changes:_
- _C1: property tests now run on `NewInMemoryTestGraph` per spec §10 #3 (was tempfile)._
- _C2: `edges` table gains nullable `owner_id` / `visibility` columns for VISION §16 parity with `nodes`._
- _C3: CI workflow removed from scope. Plan and tasks file align — no `.github/workflows/graph-tests.yml` ships in P0.1._
- _C4: `TestBulkInsertLatency` added to Task 6 (1000 sequential inserts, soft latency bounds)._
- _C5: `TestFTSEmptyQuery` decided — empty query returns wrapped `ErrFTSQuerySyntax`._
- _F1: `TestReaderSeesPreDeleteSnapshot` gains `readStarted` barrier to eliminate the test's own start-order race._
- _F2: Note.DiffBody LCS scaling documented as a future Myers-upgrade follow-up bead (trigger criteria attached); not implemented now._
- _F3: malformed FTS5 fixture changed from `"unbalanced` to `"AND"` (bare operator) for reliable parse error._
- _F4: multi-statement SQL fallback removed; rely on `modernc.org/sqlite`'s native multi-statement Exec._
- _F5: new `Task 5.5` creates `internal/graph/integration_test.go` with one named test per spec §11 criterion (8 tests, auditable mapping)._
- _Dim 4: added missing tests — tombstone author (`TestDeleteTombstonePreservesAuthor`), dirty sticky (`TestDirtyStickyAfterFailedMigration`), `attrs` invalid JSON (`TestAttrsRejectInvalidJSON`), mixed EN/PT FTS (`TestFTSMixedEnglishPortuguese`), Close-with-in-flight-tx race (`TestCloseWithInFlightTxsDoesNotPanic`)._
- _Dim 5: removed duplicate Lane A attrs table (lines 1442–1451 of iter 1)._
- _Dim 3: `Task 3.1` now declares `Dependencies: Task 2.4` explicitly; Lane B tasks (`5.1`, `5.3`) declare deps on `Task 4.1` (and 4.2 for FTS) since their tests use real `nodes.CreateBead`/`CreateNote` fixtures._
- _Dim 2: Open idempotency criterion concretized (sequential Open → Close → Open, second Up applies zero migrations, no SQLITE_BUSY); testutil isolation upgraded to cross-check (every parallel sibling asserts no peer's tag is visible)._
- _Minor: Go 1.22+ declared in Tech Stack header; Lane A intro adds note that `Note.Body` is substrate cache (P0.2 owns FS↔SQLite sync per VISION §6.2)._
