# P0.1 — Graph Substrate: Atomic Task Breakdown

Derived from `docs/plans/2026-05-17-graph-substrate-plan.md`. Each atomic task is one action (2–5 minutes). Epics are containers (Estimated Minutes = 0). Children reference their parent epic and declare their own dependencies (not inherited from parent).

---

### Task 1: Repo reorganization (epic container)


```json
{
  "key": "epic-1",
  "type": "epic",
  "priority": 0,
  "status": "open",
  "estimated_minutes": 0
}
```
_Container for the mechanical reorg of `orchestrator/*` to repo root. Children (1.1–1.6) execute in heading order. Created during iter-3 of the plan review to satisfy vc-convert's parent-epic requirement._

**Bead Mapping:**
- type: `epic`
- Priority: `0`
- Estimated Minutes: 0
- Dependencies: `none`
- Parent: `none`
- Status: `open`

---

### Task 1.1: Inventory existing module references for reorg


```json
{
  "key": "task-1-1",
  "type": "task",
  "priority": 0,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-1"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 15
- Dependencies: `none`
- Parent: `none`
- Status: `open`

**Files:**
- Modify: `.github/workflows/*.yml`, `scripts/swarm/*`, `AGENTS.md`, `CLAUDE.md`, `README.md`, `docs/*`

**Action:** Run the grep commands from the parent plan's Step 1, capture the full list of files referencing `orchestrator/` paths.

**Command:**
```bash
rg -l 'kernl/orchestrator/(internal|cmd|web)' --type go > /tmp/orchestrator-refs.txt
rg -l 'cd orchestrator' .github/ scripts/ >> /tmp/orchestrator-refs.txt
cat /tmp/orchestrator-refs.txt | sort -u | wc -l
```

**Expected output:** File count (for Step 1.2 verification). All files must be listed.

**Acceptance Criteria:**
- [ ] All files with `orchestrator/` references listed in `/tmp/orchestrator-refs.txt`
- [ ] Zero `orchestrator/` references missed

---

### Task 1.2: Create root go.mod and go.sum


```json
{
  "key": "task-1-2",
  "type": "task",
  "priority": 0,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-1"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 1.1`
- Parent: `none`
- Status: `open`

**Files:**
- Create: `go.mod` (repo root)
- Create: `go.sum` (repo root)
- Delete: `orchestrator/go.mod`
- Delete: `orchestrator/go.sum`

**Action:** Copy `orchestrator/go.mod` → root `go.mod`; copy `go.sum` → root. Verify `module github.com/gabrielassisxyz/kernl` (unchanged). Delete orchestrator module files.

**Command:**
```bash
cp orchestrator/go.mod go.mod
cp orchestrator/go.sum go.sum
git rm orchestrator/go.mod orchestrator/go.sum
git commit -m "refactor: create root module"
```

**Acceptance Criteria:**
- [ ] `cat go.mod | head -1` shows `module github.com/gabrielassisxyz/kernl`
- [ ] `find . -name go.mod` returns exactly `./go.mod`

---

### Task 1.3a: Move directories with `git mv`


```json
{
  "key": "task-1-3a",
  "type": "task",
  "priority": 0,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-1"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 1.2`
- Parent: `none`
- Status: `open`

**Files:**
- Move: `orchestrator/cmd/` → `cmd/`
- Move: `orchestrator/internal/` → `internal/orchestrator/`
- Move: `orchestrator/web/` → `web/` (if present)

**Command:**
```bash
git mv orchestrator/cmd cmd
git mv orchestrator/internal internal/orchestrator
[ -d orchestrator/web ] && git mv orchestrator/web web
git commit -m "refactor: relocate orchestrator subdirs to root (paths only)"
```

**Acceptance Criteria:**
- [ ] `ls orchestrator/` shows only `go.mod`, `go.sum` (or empty if already deleted)
- [ ] `ls cmd/ internal/orchestrator/` returns the moved contents
- [ ] Commit lands cleanly (intentionally breaks build until 1.3b)

---

### Task 1.3b: Rewrite Go import paths via sed


```json
{
  "key": "task-1-3b",
  "type": "task",
  "priority": 0,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-1"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 15
- Dependencies: `Task 1.3a`
- Parent: `none`
- Status: `open`

**Files:**
- Modify: all `.go` files with orchestrator imports (per `rg` output)

**Command:**
```bash
rg -l 'kernl/orchestrator/(internal|cmd|web)' --type go | \
  xargs sed -i 's#github.com/gabrielassisxyz/kernl/orchestrator/internal#github.com/gabrielassisxyz/kernl/internal/orchestrator#g; s#github.com/gabrielassisxyz/kernl/orchestrator/cmd#github.com/gabrielassisxyz/kernl/cmd#g; s#github.com/gabrielassisxyz/kernl/orchestrator/web#github.com/gabrielassisxyz/kernl/web#g'
git commit -am "refactor: rewrite import paths to new root module"
```

**Acceptance Criteria:**
- [ ] `rg 'kernl/orchestrator/(internal|cmd|web)' --type go` returns zero hits

---

### Task 1.3c: `goimports`/`gofmt` cleanup + sanity audit


```json
{
  "key": "task-1-3c",
  "type": "task",
  "priority": 0,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-1"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 1.3b`
- Parent: `none`
- Status: `open`

**Files:**
- Modify: any `.go` file requiring import reordering after Task 1.3b

**Command:**
```bash
go fmt ./...
goimports -w .  # if installed; otherwise skip
git status -s
git diff --stat
git commit -am "refactor: gofmt/goimports cleanup after import rewrite"  # only if dirty
```

**Acceptance Criteria:**
- [ ] `go vet ./...` runs without "undefined" or "could not import" errors
- [ ] `git status -s` shows zero untracked files in `orchestrator/` (only `go.mod`/`go.sum` if not yet removed by Task 1.2)

---

### Task 1.4: Update CI and script paths


```json
{
  "key": "task-1-4",
  "type": "task",
  "priority": 0,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-1"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 15
- Dependencies: `Task 1.3c`
- Parent: `none`
- Status: `open`

**Files:**
- Modify: `.github/workflows/*.yml`
- Modify: `scripts/swarm/*`
- Modify: `AGENTS.md`, `CLAUDE.md`, `README.md`

**Action:** Replace `cd orchestrator && go test` with `go test ./...`. Replace `orchestrator/` paths in scripts and docs.

**Command:**
```bash
rg -l 'cd orchestrator' .github/ scripts/ | xargs sed -i 's/cd orchestrator && //g'
```

**Acceptance Criteria:**
- [ ] `rg 'cd orchestrator' .github/ scripts/` returns zero hits
- [ ] CI workflow YAML syntax valid (`cat` + visual scan)

---

### Task 1.5: Build + verify identical pass/fail


```json
{
  "key": "task-1-5",
  "type": "task",
  "priority": 0,
  "status": "open",
  "estimated_minutes": 45,
  "parent": "epic-1"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 45
- Dependencies: `Task 1.4`
- Parent: `none`
- Status: `open`

**Files:** N/A (verification only)

**Action:** Build from root, run full test suite, compare to baseline.

**Command:**
```bash
go build ./...
go test ./...
```

**Acceptance Criteria:**
- [ ] `go build ./...` succeeds from repo root
- [ ] Test pass/fail counts match pre-reorg baseline (if any, all must pass)
- [ ] Zero compilation errors

---

### Task 1.6: Commit reorg PR


```json
{
  "key": "task-1-6",
  "type": "task",
  "priority": 0,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-1"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 1.5`
- Parent: `none`
- Status: `open`

**Files:** (already staged)

**Action:** Single commit with all reorg changes.

**Command:**
```bash
git add -A
git commit -m "refactor: promote orchestrator to root go module"
```

**Acceptance Criteria:**
- [ ] Commit hash created; log shows one clean refactor commit
- [ ] `git status` clean (no uncommitted changes)

---

### Task 2: SQLite substrate + migration runner


```json
{
  "key": "epic-2",
  "type": "epic",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 0,
  "dependencies": [
    "epic-1"
  ]
}
```
**Bead Mapping:**
- type: `epic`
- Priority: `0`
- Estimated Minutes: 0
- Dependencies: `Task 1.6`
- Parent: `none`
- Status: `open`

---

### Task 2.1.1: Write failing test `TestOpenAppliesPragmas`


```json
{
  "key": "task-2-1-1",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 1.6`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/internal/sqlite/sqlite_test.go` (initial version)

**Action:** Write the test asserting WAL, foreign_keys, and in-memory mode.

**Expected output:** FAIL — package doesn't exist yet.

---

### Task 2.1.2a: Define `Config` + `Pool` types + tempfile DSN


```json
{
  "key": "task-2-1-2a",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 15
- Dependencies: `Task 2.1.1`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/internal/sqlite/sqlite.go`

**Action:** Define `Config{Path string, InMemory bool}` and `Pool{Read, Write *sql.DB}`. Implement `Open` for the **tempfile branch only**: build DSN with `journal_mode=WAL, synchronous=NORMAL, foreign_keys=1, busy_timeout=5000, temp_store=MEMORY`. Open read pool with default `MaxOpenConns`; open write pool with `SetMaxOpenConns(1)`.

**Acceptance Criteria:**
- [ ] tempfile DSN applies WAL + fk=1 + busy_timeout=5000 + temp_store=MEMORY
- [ ] Read pool default conns, Write pool `MaxOpenConns(1)`

---

### Task 2.1.2b: `Open` — in-memory DSN branch


```json
{
  "key": "task-2-1-2b",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 2.1.2a`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Modify: `internal/graph/internal/sqlite/sqlite.go`

**Action:** Handle the `InMemory=true` branch: DSN `file:<name>?mode=memory&cache=shared&_pragma=journal_mode(MEMORY)&...`. Both pool handles must reuse the SAME DSN string so `cache=shared` actually shares the in-memory DB across read+write pools.

**Acceptance Criteria:**
- [ ] in-memory mode applies MEMORY journal + fk=1 + busy_timeout=5000
- [ ] Both read and write pool see the same in-memory DB (write visible from read pool in same `Open` call)

---

### Task 2.1.3: Implement `sqlite/sqlite.go` Close + Pool.Close


```json
{
  "key": "task-2-1-3",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 2.1.2b`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Modify: `internal/graph/internal/sqlite/sqlite.go`

**Action:** Close write pool, then read pool. Return first error encountered.

---

### Task 2.1.4: Write failing test `TestWritePoolSerializesWrites`


```json
{
  "key": "task-2-1-4",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 2.1.3`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Modify: `internal/graph/internal/sqlite/sqlite_test.go`

**Action:** Write the 50-goroutine concurrent write test. Expected: some failures if pool allows >1 conn.

---

### Task 2.1.5: Run sqlite tests with `-race`


```json
{
  "key": "task-2-1-5",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 20,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 20
- Dependencies: `Task 2.1.4`
- Parent: `Epic 2`
- Status: `open`

**Files:** N/A

**Command:**
```bash
go test ./internal/graph/internal/sqlite/... -race
```

**Acceptance Criteria:**
- [ ] All tests pass with zero data races
- [ ] `TestWritePoolSerializesWrites` shows 0 SQLITE_BUSY errors

---

### Task 2.1.6: Commit sqlite package


```json
{
  "key": "task-2-1-6",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 2.1.5`
- Parent: `Epic 2`
- Status: `open`

**Files:** (staged)

**Command:**
```bash
git add internal/graph/internal/sqlite/
git commit -m "feat(graph): sqlite open with pragmas and dual-handle pool"
```

---

### Task 2.2.1: Write `TestUpAppliesPendingMigrations` (failing)


```json
{
  "key": "task-2-2-1",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 2.1.6`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/internal/migrate/migrate_test.go`
- Create: `internal/graph/internal/migrate/testdata/schema/0001_init.up.sql`
- Create: `internal/graph/internal/migrate/testdata/schema/0001_init.down.sql`

**Action:** Write failing test + test fixtures. Expected: FAIL (package doesn't exist).

---

### Task 2.2.2: Define `Runner`, `New`, schema_migrations table creation


```json
{
  "key": "task-2-2-2",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 15
- Dependencies: `Task 2.2.1`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/internal/migrate/migrate.go`

**Action:** Implement struct, constructor, ensure schema_migrations exists with exact golang-migrate-compatible schema.

---

### Task 2.2.3: Implement `Runner.Current` and migration file loading


```json
{
  "key": "task-2-2-3",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 15
- Dependencies: `Task 2.2.2`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Modify: `internal/graph/internal/migrate/migrate.go`

**Action:** Parse NNNN_*.up.sql filenames, sort ascending, read via embed.FS.

---

### Task 2.2.4a: `Runner.Up` — ensure tracking table + load migrations + dirty check


```json
{
  "key": "task-2-2-4a",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 2.2.3`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Modify: `internal/graph/internal/migrate/migrate.go`

**Action:** First half of `Up`: `CREATE TABLE IF NOT EXISTS schema_migrations` (golang-migrate-compatible schema), load all `NNNN_*.up.sql` from embed.FS, parse versions, sort ascending. `SELECT MAX(version), dirty FROM schema_migrations` — if `dirty=true` return `migrate.ErrDirty`.

---

### Task 2.2.4b: `Runner.Up` — apply loop (BEGIN/dirty/Exec/clean/COMMIT)


```json
{
  "key": "task-2-2-4b",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 2.2.4a`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Modify: `internal/graph/internal/migrate/migrate.go`

**Action:** Second half of `Up`: for each migration with `version > current`, in a single tx: `INSERT INTO schema_migrations(version, dirty) VALUES (?, true) ON CONFLICT DO UPDATE SET dirty=true`, then `tx.ExecContext(ctx, migrationContent)` (multi-statement native — parent plan F4), then `UPDATE schema_migrations SET dirty=false WHERE version=?`, then `COMMIT`. On any error, tx rolls back and `dirty=true` stays sticky.

---

### Task 2.2.5: Write `TestDownRollsBack` and `TestUpDownUpRoundTrip`


```json
{
  "key": "task-2-2-5",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 2.2.4b`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Modify: `internal/graph/internal/migrate/migrate_test.go`

**Action:** Add Down tests. Run all tests. Expected: PASS.

---

### Task 2.2.6: Implement `Runner.Down`


```json
{
  "key": "task-2-2-6",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 15
- Dependencies: `Task 2.2.5`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Modify: `internal/graph/internal/migrate/migrate.go`

**Action:** Apply .down.sql migrations in reverse order until target reached.

---

### Task 2.2.7: Run migrate tests with `-race`; verify LOC ceiling


```json
{
  "key": "task-2-2-7",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 2.2.6`
- Parent: `Epic 2`
- Status: `open`

**Command:**
```bash
wc -l internal/graph/internal/migrate/migrate.go
go test ./internal/graph/internal/migrate/... -race
```

**Acceptance Criteria:**
- [ ] All tests pass; LOC ≤ 120
- [ ] dirty-migration test passes (simulated syntax error → dirty=true sticky)

---

### Task 2.2.7a: Write `TestDirtyStickyAfterFailedMigration`


```json
{
  "key": "task-2-2-7a",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 15
- Dependencies: `Task 2.2.7`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Modify: `internal/graph/internal/migrate/migrate_test.go`
- Create: `internal/graph/internal/migrate/testdata/schema_broken/0001_init.up.sql` (contains intentional `CREATE TABEL` typo)
- Create: `internal/graph/internal/migrate/testdata/schema_broken/0001_init.down.sql`

**Action:** Add a fixture set with a syntax error in the `.up.sql`. Test runs `Up`, asserts failure, asserts `dirty=true`, then asserts a second `Up` returns `errors.Is(err, migrate.ErrDirty)` without re-attempting the migration. Backs the parent plan's §2.2 Step 7 + acceptance criterion.

**Acceptance Criteria:**
- [ ] Test passes — first Up fails, dirty=true, second Up returns `ErrDirty`

---

### Task 2.2.8: Commit migrate package


```json
{
  "key": "task-2-2-8",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 2.2.7a`
- Parent: `Epic 2`
- Status: `open`

**Command:**
```bash
git add internal/graph/internal/migrate/
git commit -m "feat(graph): hand-rolled migrator with golang-migrate-compatible tracking + dirty-sticky"
```

---

### Task 2.3.1: Write `0001_init.up.sql` (all STRICT tables)


```json
{
  "key": "task-2-3-1",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 20,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 20
- Dependencies: `Task 2.2.8`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/schema/0001_init.up.sql`

**Action:** Write the full schema per parent plan Spec §6, STRICT tables, `fts_rowid` on nodes, no cascade on revisions.node_id, `edges.owner_id`/`edges.visibility` nullable (VISION §16 parity).

**Acceptance Criteria:**
- [ ] All tables use `STRICT`; foreign keys correct; FTS virtual table present
- [ ] `revisions.node_id` has **no** ON DELETE CASCADE
- [ ] `nodes.fts_rowid INTEGER UNIQUE` present
- [ ] `edges.owner_id TEXT` and `edges.visibility TEXT` columns present (nullable)

---

### Task 2.3.2: Write `0001_init.down.sql`


```json
{
  "key": "task-2-3-2",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 2.3.1`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/schema/0001_init.down.sql`

**Action:** DROP all tables in reverse of creation order. Do NOT drop schema_migrations.

---

### Task 2.3.3: Create `schema/schema.go` with embed.FS


```json
{
  "key": "task-2-3-3",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 2.3.2`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/schema/schema.go`

---

### Task 2.3.4: Write `TestInitialSchemaApplies` smoke test


```json
{
  "key": "task-2-3-4",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 2.3.3`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/schema/schema_test.go`

**Action:** Apply real schema via `Open`, assert all 6 tables queryable.

**Expected:** May FAIL if TableInfo query syntax is wrong.

---

### Task 2.3.5: Write `TestInitialSchemaRoundTrip`


```json
{
  "key": "task-2-3-5",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 2.3.4`
- Parent: `Epic 2`
- Status: `open`

**Action:** Up → Down → Up on actual schema. Assert version=1.

---

### Task 2.3.6: Verify STRICT and revisions FK via PRAGMA


```json
{
  "key": "task-2-3-6",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 2.3.5`
- Parent: `Epic 2`
- Status: `open`

**Command-style test:**
```go
func testSchemaCorrections(t *testing.T, db *sql.DB) {
    // nodes has STRICT
    // CHECK via PRAGMA table_info(nodes) - not from STRICT keyword info directly
    // revisions.node_id has no ON DELETE CASCADE via PRAGMA foreign_key_list(revisions)
}
```

**Acceptance Criteria:**
- [ ] `PRAGMA foreign_key_list(revisions)` shows no CASCADE action on the node_id FK
- [ ] `PRAGMA table_info(edges)` shows `owner_id` and `visibility` columns, both nullable

---

### Task 2.3.6a: Write `TestAttrsRejectInvalidJSON`


```json
{
  "key": "task-2-3-6a",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 2.3.6`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Modify: `internal/graph/schema/schema_test.go`

**Action:** Insert a malformed JSON value into `nodes.attrs` via raw SQL; assert the `CHECK(json_valid(attrs))` constraint rejects it. Backs parent plan §2.3 Step 5a.

**Acceptance Criteria:**
- [ ] Insert returns a constraint error (non-nil); no row inserted

---

### Task 2.3.7: Commit schema package


```json
{
  "key": "task-2-3-7",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 2.3.6a`
- Parent: `Epic 2`
- Status: `open`

**Command:**
```bash
git add internal/graph/schema/
git commit -m "feat(graph): initial schema migration (nodes/edges/tags/revisions/fts)"
```

---

### Task 2.4.1: Define `Config`, `Graph`, `Open`, `Close`


```json
{
  "key": "task-2-4-1",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 2.3.7`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/graph.go` (skeleton)

**Action:** Write struct and constructor calling `sqlite.Open` + `migrate.New` + `r.Up`.

---

### Task 2.4.2: Define unforgeable `ReadTx`/`WriteTx` with explicit delegation


```json
{
  "key": "task-2-4-2",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 2.4.1`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Modify: `internal/graph/graph.go`

**Action:** Add interface definitions, readTx/writeTx wrapper structs. Ensure `Commit`/`Rollback` NOT exposed.

---

### Task 2.4.3: Implement `DoRead`/`DoWrite` helpers


```json
{
  "key": "task-2-4-3",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 2.4.2`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Modify: `internal/graph/graph.go`

**Action:** Begin ro/ro-rw tx from respective pool, call fn, rollback on error, commit on success.

---

### Task 2.4.4: Write `TestOpenAndClose` smoke


```json
{
  "key": "task-2-4-4",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 2.4.3`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Create: `internal/graph/graph_test.go`

---

### Task 2.4.5: Write `TestDoReadDoWrite`


```json
{
  "key": "task-2-4-5",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 2.4.4`
- Parent: `Epic 2`
- Status: `open`

**Files:**
- Modify: `internal/graph/graph_test.go`

---

### Task 2.4.6: Write `TestOpenIdempotence`


```json
{
  "key": "task-2-4-6",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 2.4.5`
- Parent: `Epic 2`
- Status: `open`

**Action:** Open twice on same tempfile → second call must be no-op.

---

### Task 2.4.7: Commit graph top-level


```json
{
  "key": "task-2-4-7",
  "type": "task",
  "priority": 1,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-2"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 2.4.6`
- Parent: `Epic 2`
- Status: `open`

**Command:**
```bash
git add internal/graph/graph.go internal/graph/graph_test.go
git commit -m "feat(graph): Graph type with Open/Close/DoRead/DoWrite + unforgeable Tx"
```

---

### Task 3: Chokepoint + types + errors + testutil


```json
{
  "key": "epic-3",
  "type": "epic",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 0,
  "dependencies": [
    "epic-2"
  ]
}
```
**Bead Mapping:**
- type: `epic`
- Priority: `0`
- Estimated Minutes: 0
- Dependencies: `Epic 2`
- Parent: `none`
- Status: `open`

---

### Task 3.1.1: Write `errors.go` with 4 sentinels


```json
{
  "key": "task-3-1-1",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 2.4.7`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/errors.go`

**Action:** Define `ErrNotFound`, `ErrFTSQuerySyntax`, `ErrSchemaLocked`, `ErrAuthorRequired` as `var ... errors.New()`.

---

### Task 3.1.2: Wire `migrate.ErrDirty` → `graph.ErrSchemaLocked` translation


```json
{
  "key": "task-3-1-2",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.1.1`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/graph.go` (in Open)
- Modify: `internal/graph/internal/migrate/migrate.go` (define ErrDirty in public API)

**Action:** Add `migrate.ErrDirty`; translate to `graph.ErrSchemaLocked` in Open. No circular import—migrate stays clean of graph.

---

### Task 3.1.3: Write `TestSentinelsAreDistinct`


```json
{
  "key": "task-3-1-3",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.1.2`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/errors_test.go`

---

### Task 3.1.4: Write `TestSchemaLockedSurfacesFromOpen`


```json
{
  "key": "task-3-1-4",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 3.1.3`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/errors_test.go`

**Action:** Use in-memory test graph; manually flip dirty via raw SQL; assert `errors.Is(err, ErrSchemaLocked)`.

---

### Task 3.1.5: Commit errors


```json
{
  "key": "task-3-1-5",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.1.4`
- Parent: `Epic 3`
- Status: `open`

---

### Task 3.2.1: Write `node.go` with `NodeSpec`, `Meta`, `FTSFields`, `DiffableNode`, `Author`


```json
{
  "key": "task-3-2-1",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 15
- Dependencies: `Task 3.1.5`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/node.go`

---

### Task 3.2.2: Write node type tests


```json
{
  "key": "task-3-2-2",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 3.2.1`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/node_test.go`

**Action:** Test `Meta.NodeID()`, `Author.Valid()`, `AuthorAgent("x")` formatting.

---

### Task 3.2.3: Verify `Meta` satisfies embedded `NodeID()` contract


```json
{
  "key": "task-3-2-3",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.2.2`
- Parent: `Epic 3`
- Status: `open`

**Action:** Inspection compile check: `var _ nodes.NodeSpec = (*nodes.Bead)(nil)` won't work yet (Bead doesn't exist). Instead verify struct embedding via `go vet` on a dummy type.

---

### Task 3.2.4: Commit node types


```json
{
  "key": "task-3-2-4",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.2.3`
- Parent: `Epic 3`
- Status: `open`

---

### Task 3.3.1: Implement `internal/graph/internal/ids/ids.go`


```json
{
  "key": "task-3-3-1",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.2.4`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/internal/ids/ids.go`

---

### Task 3.3.2: Write `TestUUIDv7Monotonic`


```json
{
  "key": "task-3-3-2",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.3.1`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/internal/ids/ids_test.go`

---

### Task 3.3.3: Write `TestUUIDv7SortableByCreationTime`


```json
{
  "key": "task-3-3-3",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 3.3.2`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/internal/ids/ids_test.go`

---

### Task 3.3.4: Promote uuid dep, commit


```json
{
  "key": "task-3-3-4",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.3.3`
- Parent: `Epic 3`
- Status: `open`

**Command:**
```bash
go mod tidy
git add internal/graph/internal/ids/ go.mod go.sum
git commit -m "feat(graph): UUIDv7 generator + monotone & sortable tests"
```

---

### Task 3.4.1: Write ASCII flow comment at top of `chokepoint.go`


```json
{
  "key": "task-3-4-1",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.3.4`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/chokepoint.go` (header comment only)

**Action:** Copy the ASCII mutation flow diagram into the file header.

---

### Task 3.4.2: Define `createNode`/`updateNode`/`deleteNode` signatures


```json
{
  "key": "task-3-4-2",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.1`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint.go`

**Action:** Add the three function signatures below the ASCII comment.

---

### Task 3.4.3: Write `TestCreateNodeWritesOneRevision` (failing)


```json
{
  "key": "task-3-4-3",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.2`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint_test.go`

**Action:** Write test with `fakeSpec` implementer. Expected: FAIL (createNode not implemented).

---

### Task 3.4.4a: `createNode` — skeleton + Author validation


```json
{
  "key": "task-3-4-4a",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.3`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint.go`

**Action:** Function shell. Validate `author.Valid()` → return `graph.ErrAuthorRequired`. Assign `spec.Meta.ID = ids.New()` if empty. Set `CreatedAt = UpdatedAt = time.Now().UTC()`.

---

### Task 3.4.4b: `createNode` — INSERT into `nodes` table


```json
{
  "key": "task-3-4-4b",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.4a`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint.go`

**Action:** Call `spec.NodeAttrs()`, then `tx.ExecContext("INSERT INTO nodes(id,type,title,attrs,created_at,updated_at) VALUES (?,?,?,?,?,?)", ...)`. `fts_rowid` stays NULL at this stage.

---

### Task 3.4.4c: `createNode` — INSERT into `nodes_fts` + link `fts_rowid`


```json
{
  "key": "task-3-4-4c",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.4b`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint.go`

**Action:** `spec.FTSFields()` → `result, err := tx.ExecContext("INSERT INTO nodes_fts(title,body,tags) VALUES (?,?,?)", ...)`. Read `result.LastInsertId()` → run `UPDATE nodes SET fts_rowid = ? WHERE id = ?`. Two SQL operations linked — the most error-prone part of the chokepoint.

---

### Task 3.4.4d: `createNode` — reconcile tags + INSERT first revision


```json
{
  "key": "task-3-4-4d",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.4c`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint.go`

**Action:** For each tag in `spec.NodeTags()`: `INSERT OR IGNORE INTO tags(name)`, `INSERT INTO node_tags(node_id, tag)`. Then build full snapshot JSON `{title, attrs, tags}` and `INSERT INTO revisions(id, node_id, prev_revision_id=NULL, author, snapshot, diff=NULL, ts) VALUES (...)`.

---

### Task 3.4.5: Run `TestCreateNodeWritesOneRevision`


```json
{
  "key": "task-3-4-5",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.4d`
- Parent: `Epic 3`
- Status: `open`

**Command:**
```bash
go test ./internal/graph/nodes/... -run TestCreateNodeWritesOneRevision -v
```

**Expected:** PASS.

---

### Task 3.4.6: Write `TestUpdateNodeReplacesFTSContent` (failing)


```json
{
  "key": "task-3-4-6",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.5`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint_test.go`

**Action:** Write the FTS gap test. Expected: FAIL (updateNode not implemented).

---

### Task 3.4.7a: `updateNode` — SELECT prev (snapshot + tags + fts_rowid) + `ErrNotFound`


```json
{
  "key": "task-3-4-7a",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.6`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint.go`

**Action:** Validate `author.Valid()` first. Then `SELECT title, attrs, fts_rowid FROM nodes WHERE id=?` — `sql.ErrNoRows` → wrap as `graph.ErrNotFound`. Then `SELECT tag FROM node_tags WHERE node_id=?` to get prev tag set. Also load prev `NodeSpec` shape if needed for DiffableNode (struct-instantiate from prev row).

---

### Task 3.4.7b: `updateNode` — UPDATE nodes (title/attrs/updated_at)


```json
{
  "key": "task-3-4-7b",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.7a`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint.go`

**Action:** `spec.NodeAttrs()` → `tx.ExecContext("UPDATE nodes SET title=?, attrs=?, updated_at=? WHERE id=?", ...)`. `updated_at = time.Now().UTC()`.

---

### Task 3.4.7c: `updateNode` — reconcile tags (compute add/remove diff)


```json
{
  "key": "task-3-4-7c",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.7b`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint.go`

**Action:** Diff prev tag set vs `spec.NodeTags()`. For each added tag: `INSERT OR IGNORE INTO tags` + `INSERT INTO node_tags`. For each removed tag: `DELETE FROM node_tags WHERE node_id=? AND tag=?`. Leave `tags` rows alone (other nodes may reference them).

---

### Task 3.4.7d: `updateNode` — INSERT revision with `prev_revision_id` + DiffableNode branch


```json
{
  "key": "task-3-4-7d",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.7c`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint.go`

**Action:** `SELECT id FROM revisions WHERE node_id=? ORDER BY ts DESC, id DESC LIMIT 1` → `prev_revision_id`. Build full snapshot JSON `{title, attrs, tags}` of NEW state. If `spec` implements `DiffableNode`, call `spec.DiffBody(prevSpec)` → `diff` column; else NULL. `INSERT INTO revisions(...)`.

---

### Task 3.4.7e: `updateNode` — DELETE+INSERT FTS reusing fts_rowid


```json
{
  "key": "task-3-4-7e",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.7d`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint.go`

**Action:** `DELETE FROM nodes_fts WHERE rowid = ?` (prev fts_rowid). Then `INSERT INTO nodes_fts(rowid, title, body, tags) VALUES (?, ?, ?, ?)` reusing the same rowid so `nodes.fts_rowid` stays valid (no UPDATE needed).

---

### Task 3.4.8: Run `TestUpdateNodeReplacesFTSContent`


```json
{
  "key": "task-3-4-8",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.7e`
- Parent: `Epic 3`
- Status: `open`

**Expected:** PASS (no stale FTS entries).

---

### Task 3.4.9: Write `TestDeleteNodePreservesRevisionHistory` (CRITICAL)


```json
{
  "key": "task-3-4-9",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 15
- Dependencies: `Task 3.4.8`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint_test.go`

---

### Task 3.4.10a: `deleteNode` — SELECT current state + `ErrNotFound`


```json
{
  "key": "task-3-4-10a",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.9`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint.go`

**Action:** Validate `author.Valid()`. `SELECT type, title, attrs FROM nodes WHERE id=?` — `sql.ErrNoRows` → `graph.ErrNotFound`. Also `SELECT tag FROM node_tags WHERE node_id=?` to capture tags for the snapshot.

---

### Task 3.4.10b: `deleteNode` — INSERT tombstone revision (BEFORE delete)


```json
{
  "key": "task-3-4-10b",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.10a`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint.go`

**Action:** Build snapshot JSON `{title, attrs, tags}` of LAST state. Resolve `prev_revision_id` via `SELECT id FROM revisions WHERE node_id=? ORDER BY ts DESC, id DESC LIMIT 1`. `INSERT INTO revisions(id, node_id, prev_revision_id, author, snapshot, diff=NULL, ts) VALUES (...)`. **This must happen BEFORE the DELETE** — the test fixture in 3.4.9 asserts the tombstone survives.

---

### Task 3.4.10c: `deleteNode` — DELETE FROM nodes (cascade fires)


```json
{
  "key": "task-3-4-10c",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.10b`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint.go`

**Action:** `tx.ExecContext("DELETE FROM nodes WHERE id=?", id)`. FK cascade removes `edges` (src/dst), `node_tags`, and the `nodes_fts` row via `nodes_fts_map` indirection (or directly via `nodes.fts_rowid`). `revisions` rows are NOT cascaded (per schema fix Decision 6 in eng review) — the tombstone + history survive.

---

### Task 3.4.11: Run `TestDeleteNodePreservesRevisionHistory`


```json
{
  "key": "task-3-4-11",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.10c`
- Parent: `Epic 3`
- Status: `open`

**Expected:** PASS (3 revisions post-delete; chain unbroken).

---

### Task 3.4.11a: Write `TestDeleteTombstonePreservesAuthor`


```json
{
  "key": "task-3-4-11a",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.11`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/chokepoint_test.go`

**Action:** After `CreateNode(author=Human)` + `DeleteNode(id, author=AuthorAgent("kimi"))`, query the most recent revision row (`ORDER BY ts DESC LIMIT 1`) and assert `author == "agent:kimi"`. Backs parent plan §3.4 Step 8a — proves the tombstone records the actual deleter, not an empty/default author.

**Acceptance Criteria:**
- [ ] Test passes with tombstone author exactly matching the Delete caller's `Author` value

---

### Task 3.4.12: Write `TestEmptyAuthorRejected`


```json
{
  "key": "task-3-4-12",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.11a`
- Parent: `Epic 3`
- Status: `open`

---

### Task 3.4.13: Write `TestEmptyTitleAllowed` (boundary)


```json
{
  "key": "task-3-4-13",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.12`
- Parent: `Epic 3`
- Status: `open`

---

### Task 3.4.14: Write `TestDiffableNodeStoresDiff`


```json
{
  "key": "task-3-4-14",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.13`
- Parent: `Epic 3`
- Status: `open`

**Action:** Register a test-only DiffableNode implementer, verify diff column is non-NULL on update, NULL on create.

---

### Task 3.4.15: Run full chokepoint test suite with `-race`


```json
{
  "key": "task-3-4-15",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 15
- Dependencies: `Task 3.4.14`
- Parent: `Epic 3`
- Status: `open`

**Command:**
```bash
go test ./internal/graph/nodes/... -race
```

---

### Task 3.4.16: Commit chokepoint


```json
{
  "key": "task-3-4-16",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.4.15`
- Parent: `Epic 3`
- Status: `open`

**Command:**
```bash
git add internal/graph/nodes/chokepoint.go internal/graph/nodes/chokepoint_test.go
git commit -m "feat(graph/nodes): chokepoint createNode/updateNode/deleteNode with audit-by-construction"
```

---

### Task 3.5.1: Implement `testutil.NewTestGraph(t)` (tempfile)


```json
{
  "key": "task-3-5-1",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.16`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/testutil/substrate.go`

---

### Task 3.5.2: Implement `testutil.NewInMemoryTestGraph(t)`


```json
{
  "key": "task-3-5-2",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 10
- Dependencies: `Task 3.5.1`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/testutil/substrate.go`

---

### Task 3.5.3: Write `TestIsolationAcrossParallelTests` (cross-check, not just self-check)


```json
{
  "key": "task-3-5-3",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 15
- Dependencies: `Task 3.5.2`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Create: `internal/graph/testutil/substrate_test.go`

**Action:** Table-driven `t.Parallel()` siblings each insert a distinct sentinel tag, then assert (a) their own tag is present (count=1), AND (b) NO sibling's tag is visible (count=0). Backs parent plan §3.5 Step 3 — concretized isolation contract.

**Acceptance Criteria:**
- [ ] Every parallel sibling sees only its own sentinel tag, no leaks

---

### Task 3.5.3a: Write `TestCloseWithInFlightTxsDoesNotPanic`


```json
{
  "key": "task-3-5-3a",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 15
- Dependencies: `Task 3.5.3`
- Parent: `Epic 3`
- Status: `open`

**Files:**
- Modify: `internal/graph/testutil/substrate_test.go`

**Action:** Spawn 4 goroutines doing best-effort `DoRead` in a loop. After 20ms, main calls `g.Close()` and asserts no error. Close `stop` channel, wait for goroutines. Test passes if no panic/deadlock/race report under `-race`. Backs parent plan §3.5 Step 3a — Dim 4 missing test for Close-while-active concurrency safety.

**Acceptance Criteria:**
- [ ] Test passes under `go test -race`; no panic, no deadlock, no data race reported

---

### Task 3.5.4: Commit testutil


```json
{
  "key": "task-3-5-4",
  "type": "task",
  "priority": 2,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-3"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `0`
- Estimated Minutes: 5
- Dependencies: `Task 3.5.3a`
- Parent: `Epic 3`
- Status: `open`

---

### Task 4: Lane A — Per-type CRUD (12 node types)


```json
{
  "key": "epic-4",
  "type": "epic",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 0,
  "dependencies": [
    "epic-3"
  ]
}
```
**Bead Mapping:**
- type: `epic`
- Priority: `1`
- Estimated Minutes: 0
- Dependencies: `Epic 3`
- Parent: `none`
- Status: `open`

## Lane A expansion — 12 types × 8 atomic tasks each

Each of the 12 type implementations is a self-contained 8-step bead chain (10 min code + 5 tests × 10 min + 5 min commit). Note (4.2) has 2 extra atoms (`4.2.3b` DiffBody impl, `4.2.7b` TestNoteUpdateStoresDiff). All Lane A chains depend only on Epic 3 completion (`Task 3.4.16`, `Task 3.5.4`) and are mutually independent — the swarm can run all 12 in parallel.

> **Anti-conflict note:** every chain creates its own `internal/graph/nodes/{type}.go` and `{type}_test.go` files. Per-type code never modifies a sibling type's file. The shared `chokepoint.go` is frozen by end of Epic 3 — no Lane A task touches it.

---

### Task 4.1.1: Bead — define struct + NodeSpec methods


```json
{
  "key": "task-4-1-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.16`, `Task 3.5.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/bead.go`

**Action:** Define `Bead{Meta; Title string; Description string; Status string; Priority int; AssigneeID *string; Tags []string}`. Implement `NodeType()="bead"`, `NodeTitle()`, `NodeAttrs()` (marshal attrs subset), `NodeTags()`, `FTSFields{Title, Body: Description, Tags: strings.Join(Tags," ")}`.

---

### Task 4.1.2: Bead — CRUD + filter delegating to chokepoint


```json
{
  "key": "task-4-1-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.1.1`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bead.go`

**Action:** `BeadFilter{Status string, AssigneeID string, MinPriority int, Limit int, UpdatedSince *time.Time, Tags []string}`. Implement `CreateBead`/`UpdateBead`/`DeleteBead` delegating to `CreateNode`/`UpdateNode`/`DeleteNode`; `GetBead(id)` SELECT WHERE type='bead'; `ListBeads(f)` with `json_extract(attrs,'$.status')` filter.

---

### Task 4.1.3: Bead — roundtrip test


```json
{
  "key": "task-4-1-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.1.2`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/bead_test.go`

**Action:** `TestBeadRoundtrip`: `CreateBead` → `GetBead(id)` returns struct with identical Meta + Title + all attrs + Tags.

---

### Task 4.1.4: Bead — Update produces exactly 1 revision


```json
{
  "key": "task-4-1-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.1.3`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bead_test.go`

**Action:** `TestBeadUpdateProducesOneRevision`: Create + UpdateBead with `AuthorAgent("foo")`; assert `count(revisions WHERE node_id=?)` == 2 and second row's author == "agent:foo".

---

### Task 4.1.5: Bead — Delete preserves revisions


```json
{
  "key": "task-4-1-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.1.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bead_test.go`

**Action:** `TestBeadDeletePreservesRevisions`: Create + Update + DeleteBead; assert 3 revision rows; walk `prev_revision_id` from latest reaches the create row.

---

### Task 4.1.6: Bead — FTS roundtrip test


```json
{
  "key": "task-4-1-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.1.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bead_test.go`

**Action:** `TestBeadFTSRoundtrip`: CreateBead with `Description: "unique-token-xyz"`; `fts.Search(tx, "unique-token-xyz")` returns the bead.

---

### Task 4.1.7: Bead — List filter test


```json
{
  "key": "task-4-1-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.1.6`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bead_test.go`

**Action:** `TestBeadListFilter`: create 3 beads with Status `open`/`closed`/`open`; `ListBeads(BeadFilter{Status: "open", Limit: 10})` returns exactly the 2 open beads.

---

### Task 4.1.8: Bead — commit


```json
{
  "key": "task-4-1-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 4.1.7`
- Parent: `Epic 4`
- Status: `open`

**Command:**
```bash
git add internal/graph/nodes/bead.go internal/graph/nodes/bead_test.go
git commit -m "feat(graph/nodes): Bead CRUD + filter"
```

---

### Task 4.2.1: Note — define struct + NodeSpec methods (implements DiffableNode)


```json
{
  "key": "task-4-2-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.16`, `Task 3.5.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/note.go`

**Action:** Define `Note{Meta; Title string; Body string; Path string; Frontmatter json.RawMessage; Tags []string}`. Implement `NodeType()="note"`, `NodeAttrs()`, `NodeTags()`, `FTSFields{Title, Body: Body, Tags: ...}`. Body is substrate-cached per VISION §6.2 — P0.2 owns FS↔SQLite sync.

---

### Task 4.2.2: Note — CRUD + filter delegating to chokepoint


```json
{
  "key": "task-4-2-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.2.1`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/note.go`

**Action:** `NoteFilter{PathPrefix string, Limit int, UpdatedSince *time.Time, Tags []string}`. CRUD delegates to chokepoint. `ListNotes` filters by `json_extract(attrs,'$.path') LIKE ?`.

---

### Task 4.2.3: Note — roundtrip test


```json
{
  "key": "task-4-2-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.2.2`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/note_test.go`

**Action:** `TestNoteRoundtrip`: Create → Get identical (Meta + Title + Body + Path + Frontmatter + Tags).

---

### Task 4.2.3b: Note — implement inline `DiffBody` LCS


```json
{
  "key": "task-4-2-3b",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 15
- Dependencies: `Task 4.2.3`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/note.go`

**Action:** Implement `func (n *Note) DiffBody(prev NodeSpec) string` — ~30 LOC inline line-by-line diff. Split bodies by `\n`, compute LCS, emit `+`/`-` prefixed lines. No new dependency. Scaling note (Myers upgrade) lives in parent plan §4.2 — do NOT optimize now.

---

### Task 4.2.4: Note — Update produces exactly 1 revision


```json
{
  "key": "task-4-2-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.2.3b`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/note_test.go`

**Action:** Same shape as `TestBeadUpdateProducesOneRevision` adapted to Note.

---

### Task 4.2.5: Note — Delete preserves revisions


```json
{
  "key": "task-4-2-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.2.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/note_test.go`

---

### Task 4.2.6: Note — FTS roundtrip test


```json
{
  "key": "task-4-2-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.2.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/note_test.go`

**Action:** CreateNote with body containing unique token; `fts.Search` returns it.

---

### Task 4.2.7: Note — List filter test


```json
{
  "key": "task-4-2-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.2.6`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/note_test.go`

**Action:** Create 3 notes with paths `/a/x.md`, `/a/y.md`, `/b/z.md`; `ListNotes(NoteFilter{PathPrefix: "/a/"})` returns 2.

---

### Task 4.2.7b: Note — `TestNoteUpdateStoresDiff`


```json
{
  "key": "task-4-2-7b",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.2.7`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/note_test.go`

**Action:** Create + UpdateNote with different body; the new revision row's `diff` column is non-NULL and contains both old and new text. First revision (post-Create) has `diff IS NULL`.

---

### Task 4.2.8: Note — commit


```json
{
  "key": "task-4-2-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 4.2.7b`
- Parent: `Epic 4`
- Status: `open`

**Command:**
```bash
git add internal/graph/nodes/note.go internal/graph/nodes/note_test.go
git commit -m "feat(graph/nodes): Note CRUD + DiffableNode + filter"
```

---

### Task 4.3.1: Project — define struct + NodeSpec methods


```json
{
  "key": "task-4-3-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.16`, `Task 3.5.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/project.go`

**Action:** `Project{Meta; Title; Description string; Status string; StartedAt *time.Time; ClosedAt *time.Time; Tags []string}`. NodeSpec methods; FTS body = Description.

---

### Task 4.3.2: Project — CRUD + filter


```json
{
  "key": "task-4-3-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.3.1`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/project.go`

**Action:** `ProjectFilter{Status string, ActiveOnly bool, ...common}`. `ActiveOnly=true` → `WHERE json_extract(attrs,'$.closed_at') IS NULL`.

---

### Task 4.3.3: Project — roundtrip test


```json
{
  "key": "task-4-3-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.3.2`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/project_test.go`

---

### Task 4.3.4: Project — Update produces 1 revision


```json
{
  "key": "task-4-3-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.3.3`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/project_test.go`

---

### Task 4.3.5: Project — Delete preserves revisions


```json
{
  "key": "task-4-3-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.3.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/project_test.go`

---

### Task 4.3.6: Project — FTS roundtrip


```json
{
  "key": "task-4-3-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.3.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/project_test.go`

---

### Task 4.3.7: Project — List filter test


```json
{
  "key": "task-4-3-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.3.6`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/project_test.go`

**Action:** Create 3 projects (1 with `ClosedAt` set, 2 active); `ListProjects(ProjectFilter{ActiveOnly: true})` returns 2.

---

### Task 4.3.8: Project — commit


```json
{
  "key": "task-4-3-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 4.3.7`
- Parent: `Epic 4`
- Status: `open`

**Command:**
```bash
git add internal/graph/nodes/project.go internal/graph/nodes/project_test.go
git commit -m "feat(graph/nodes): Project CRUD + filter"
```

---

### Task 4.4.1: Task — define struct + NodeSpec methods


```json
{
  "key": "task-4-4-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.16`, `Task 3.5.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/task.go`

**Action:** `Task{Meta; Title; Description string; Status string; Priority int; ProjectID *string; DueAt *time.Time; Tags []string}`. FTS body = Description.

---

### Task 4.4.2: Task — CRUD + filter


```json
{
  "key": "task-4-4-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.4.1`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/task.go`

**Action:** `TaskFilter{Status string, ProjectID string, DueBefore *time.Time, ...common}`.

---

### Task 4.4.3: Task — roundtrip test


```json
{
  "key": "task-4-4-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.4.2`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/task_test.go`

---

### Task 4.4.4: Task — Update produces 1 revision


```json
{
  "key": "task-4-4-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.4.3`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/task_test.go`

---

### Task 4.4.5: Task — Delete preserves revisions


```json
{
  "key": "task-4-4-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.4.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/task_test.go`

---

### Task 4.4.6: Task — FTS roundtrip


```json
{
  "key": "task-4-4-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.4.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/task_test.go`

---

### Task 4.4.7: Task — List filter test


```json
{
  "key": "task-4-4-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.4.6`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/task_test.go`

**Action:** Filter by Status="todo" returns subset; filter by ProjectID returns subset.

---

### Task 4.4.8: Task — commit


```json
{
  "key": "task-4-4-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 4.4.7`
- Parent: `Epic 4`
- Status: `open`

**Command:**
```bash
git add internal/graph/nodes/task.go internal/graph/nodes/task_test.go
git commit -m "feat(graph/nodes): Task CRUD + filter"
```

---

### Task 4.5.1: Session — define struct + NodeSpec methods


```json
{
  "key": "task-4-5-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.16`, `Task 3.5.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/session.go`

**Action:** `Session{Meta; Title; StartedAt time.Time; EndedAt *time.Time; AgentID string; Summary string; Tags []string}`. FTS body = Summary.

---

### Task 4.5.2: Session — CRUD + filter


```json
{
  "key": "task-4-5-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.5.1`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/session.go`

**Action:** `SessionFilter{AgentID string, Active bool, ...common}`. `Active=true` → `WHERE json_extract(attrs,'$.ended_at') IS NULL`.

---

### Task 4.5.3: Session — roundtrip test


```json
{
  "key": "task-4-5-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.5.2`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/session_test.go`

---

### Task 4.5.4: Session — Update produces 1 revision


```json
{
  "key": "task-4-5-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.5.3`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/session_test.go`

---

### Task 4.5.5: Session — Delete preserves revisions


```json
{
  "key": "task-4-5-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.5.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/session_test.go`

---

### Task 4.5.6: Session — FTS roundtrip


```json
{
  "key": "task-4-5-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.5.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/session_test.go`

---

### Task 4.5.7: Session — List filter test


```json
{
  "key": "task-4-5-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.5.6`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/session_test.go`

---

### Task 4.5.8: Session — commit


```json
{
  "key": "task-4-5-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 4.5.7`
- Parent: `Epic 4`
- Status: `open`

**Command:**
```bash
git add internal/graph/nodes/session.go internal/graph/nodes/session_test.go
git commit -m "feat(graph/nodes): Session CRUD + filter"
```

---

### Task 4.6.1: Decision — define struct + NodeSpec methods


```json
{
  "key": "task-4-6-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.16`, `Task 3.5.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/decision.go`

**Action:** `Decision{Meta; Title; Body string; Context string; Outcome string; DecidedAt time.Time; Tags []string}`. FTS body = `Body + " " + Context + " " + Outcome`.

---

### Task 4.6.2: Decision — CRUD + filter


```json
{
  "key": "task-4-6-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.6.1`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/decision.go`

**Action:** `DecisionFilter{Since *time.Time, ...common}`.

---

### Task 4.6.3: Decision — roundtrip test


```json
{
  "key": "task-4-6-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.6.2`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/decision_test.go`

---

### Task 4.6.4: Decision — Update produces 1 revision


```json
{
  "key": "task-4-6-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.6.3`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/decision_test.go`

---

### Task 4.6.5: Decision — Delete preserves revisions


```json
{
  "key": "task-4-6-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.6.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/decision_test.go`

---

### Task 4.6.6: Decision — FTS roundtrip (multi-field)


```json
{
  "key": "task-4-6-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.6.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/decision_test.go`

**Action:** Create decision with unique tokens in EACH of Body / Context / Outcome; assert `fts.Search` finds it by any of the three.

---

### Task 4.6.7: Decision — List filter test


```json
{
  "key": "task-4-6-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.6.6`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/decision_test.go`

---

### Task 4.6.8: Decision — commit


```json
{
  "key": "task-4-6-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 4.6.7`
- Parent: `Epic 4`
- Status: `open`

**Command:**
```bash
git add internal/graph/nodes/decision.go internal/graph/nodes/decision_test.go
git commit -m "feat(graph/nodes): Decision CRUD + filter"
```

---

### Task 4.7.1: MemoryClaim — define struct + NodeSpec methods


```json
{
  "key": "task-4-7-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.16`, `Task 3.5.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/memory_claim.go`

**Action:** `MemoryClaim{Meta; Title; Statement string; Confidence float64; Subject string; Source string; Tags []string}`. FTS body = Statement. Note: additive write contract is P2.2's concern, not here.

---

### Task 4.7.2: MemoryClaim — CRUD + filter


```json
{
  "key": "task-4-7-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.7.1`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/memory_claim.go`

**Action:** `MemoryClaimFilter{Subject string, MinConfidence float64, ...common}`.

---

### Task 4.7.3: MemoryClaim — roundtrip test


```json
{
  "key": "task-4-7-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.7.2`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/memory_claim_test.go`

---

### Task 4.7.4: MemoryClaim — Update produces 1 revision


```json
{
  "key": "task-4-7-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.7.3`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/memory_claim_test.go`

---

### Task 4.7.5: MemoryClaim — Delete preserves revisions


```json
{
  "key": "task-4-7-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.7.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/memory_claim_test.go`

---

### Task 4.7.6: MemoryClaim — FTS roundtrip


```json
{
  "key": "task-4-7-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.7.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/memory_claim_test.go`

---

### Task 4.7.7: MemoryClaim — List filter test


```json
{
  "key": "task-4-7-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.7.6`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/memory_claim_test.go`

---

### Task 4.7.8: MemoryClaim — commit


```json
{
  "key": "task-4-7-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 4.7.7`
- Parent: `Epic 4`
- Status: `open`

**Command:**
```bash
git add internal/graph/nodes/memory_claim.go internal/graph/nodes/memory_claim_test.go
git commit -m "feat(graph/nodes): MemoryClaim CRUD + filter"
```

---

### Task 4.8.1: MemoryRefutation — define struct + NodeSpec methods


```json
{
  "key": "task-4-8-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.16`, `Task 3.5.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/memory_refutation.go`

**Action:** `MemoryRefutation{Meta; Title; ClaimID string; Reason string; Confidence float64; Tags []string}`. FTS body = Reason.

---

### Task 4.8.2: MemoryRefutation — CRUD + filter


```json
{
  "key": "task-4-8-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.8.1`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/memory_refutation.go`

**Action:** `MemoryRefutationFilter{ClaimID string, ...common}`.

---

### Task 4.8.3: MemoryRefutation — roundtrip test


```json
{
  "key": "task-4-8-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.8.2`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/memory_refutation_test.go`

---

### Task 4.8.4: MemoryRefutation — Update produces 1 revision


```json
{
  "key": "task-4-8-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.8.3`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/memory_refutation_test.go`

---

### Task 4.8.5: MemoryRefutation — Delete preserves revisions


```json
{
  "key": "task-4-8-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.8.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/memory_refutation_test.go`

---

### Task 4.8.6: MemoryRefutation — FTS roundtrip


```json
{
  "key": "task-4-8-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.8.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/memory_refutation_test.go`

---

### Task 4.8.7: MemoryRefutation — List filter test


```json
{
  "key": "task-4-8-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.8.6`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/memory_refutation_test.go`

---

### Task 4.8.8: MemoryRefutation — commit


```json
{
  "key": "task-4-8-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 4.8.7`
- Parent: `Epic 4`
- Status: `open`

**Command:**
```bash
git add internal/graph/nodes/memory_refutation.go internal/graph/nodes/memory_refutation_test.go
git commit -m "feat(graph/nodes): MemoryRefutation CRUD + filter"
```

---

### Task 4.9.1: Bookmark — define struct + NodeSpec methods


```json
{
  "key": "task-4-9-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.16`, `Task 3.5.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/bookmark.go`

**Action:** `Bookmark{Meta; Title; URL string; Description string; ArchivedAt *time.Time; Excerpt string; Tags []string}`. FTS body = `Description + " " + Excerpt`.

---

### Task 4.9.2: Bookmark — CRUD + filter


```json
{
  "key": "task-4-9-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.9.1`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bookmark.go`

**Action:** `BookmarkFilter{IncludeArchived bool, ...common}`. Default `IncludeArchived=false` → `WHERE json_extract(attrs,'$.archived_at') IS NULL`.

---

### Task 4.9.3: Bookmark — roundtrip test


```json
{
  "key": "task-4-9-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.9.2`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/bookmark_test.go`

---

### Task 4.9.4: Bookmark — Update produces 1 revision


```json
{
  "key": "task-4-9-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.9.3`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bookmark_test.go`

---

### Task 4.9.5: Bookmark — Delete preserves revisions


```json
{
  "key": "task-4-9-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.9.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bookmark_test.go`

---

### Task 4.9.6: Bookmark — FTS roundtrip


```json
{
  "key": "task-4-9-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.9.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bookmark_test.go`

---

### Task 4.9.7: Bookmark — List filter test (archived excluded by default)


```json
{
  "key": "task-4-9-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.9.6`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bookmark_test.go`

**Action:** Create 2 active + 1 archived; default filter returns 2; `IncludeArchived=true` returns 3.

---

### Task 4.9.8: Bookmark — commit


```json
{
  "key": "task-4-9-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 4.9.7`
- Parent: `Epic 4`
- Status: `open`

**Command:**
```bash
git add internal/graph/nodes/bookmark.go internal/graph/nodes/bookmark_test.go
git commit -m "feat(graph/nodes): Bookmark CRUD + filter"
```

---

### Task 4.10.1: BookmarkList — define struct + NodeSpec methods


```json
{
  "key": "task-4-10-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.16`, `Task 3.5.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/bookmark_list.go`

**Action:** `BookmarkList{Meta; Title; Description string; Tags []string}`. FTS body = Description.

---

### Task 4.10.2: BookmarkList — CRUD + filter


```json
{
  "key": "task-4-10-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.10.1`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bookmark_list.go`

**Action:** `BookmarkListFilter{...common only}`.

---

### Task 4.10.3: BookmarkList — roundtrip test


```json
{
  "key": "task-4-10-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.10.2`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/bookmark_list_test.go`

---

### Task 4.10.4: BookmarkList — Update produces 1 revision


```json
{
  "key": "task-4-10-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.10.3`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bookmark_list_test.go`

---

### Task 4.10.5: BookmarkList — Delete preserves revisions


```json
{
  "key": "task-4-10-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.10.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bookmark_list_test.go`

---

### Task 4.10.6: BookmarkList — FTS roundtrip


```json
{
  "key": "task-4-10-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.10.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bookmark_list_test.go`

---

### Task 4.10.7: BookmarkList — List filter test


```json
{
  "key": "task-4-10-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.10.6`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/bookmark_list_test.go`

---

### Task 4.10.8: BookmarkList — commit


```json
{
  "key": "task-4-10-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 4.10.7`
- Parent: `Epic 4`
- Status: `open`

**Command:**
```bash
git add internal/graph/nodes/bookmark_list.go internal/graph/nodes/bookmark_list_test.go
git commit -m "feat(graph/nodes): BookmarkList CRUD + filter"
```

---

### Task 4.11.1: Capture — define struct + NodeSpec methods


```json
{
  "key": "task-4-11-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.16`, `Task 3.5.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/capture.go`

**Action:** `Capture{Meta; Title; Body string; CapturedFrom string; Tags []string}`. FTS body = Body.

---

### Task 4.11.2: Capture — CRUD + filter


```json
{
  "key": "task-4-11-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.11.1`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/capture.go`

**Action:** `CaptureFilter{CapturedFromPrefix string, ...common}`.

---

### Task 4.11.3: Capture — roundtrip test


```json
{
  "key": "task-4-11-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.11.2`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/capture_test.go`

---

### Task 4.11.4: Capture — Update produces 1 revision


```json
{
  "key": "task-4-11-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.11.3`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/capture_test.go`

---

### Task 4.11.5: Capture — Delete preserves revisions


```json
{
  "key": "task-4-11-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.11.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/capture_test.go`

---

### Task 4.11.6: Capture — FTS roundtrip


```json
{
  "key": "task-4-11-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.11.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/capture_test.go`

---

### Task 4.11.7: Capture — List filter test


```json
{
  "key": "task-4-11-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.11.6`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/capture_test.go`

---

### Task 4.11.8: Capture — commit


```json
{
  "key": "task-4-11-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 4.11.7`
- Parent: `Epic 4`
- Status: `open`

**Command:**
```bash
git add internal/graph/nodes/capture.go internal/graph/nodes/capture_test.go
git commit -m "feat(graph/nodes): Capture CRUD + filter"
```

---

### Task 4.12.1: WorkflowRun — define struct + NodeSpec methods


```json
{
  "key": "task-4-12-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 3.4.16`, `Task 3.5.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/workflow_run.go`

**Action:** `WorkflowRun{Meta; Title; WorkflowName string; Status string; StartedAt time.Time; EndedAt *time.Time; RunData json.RawMessage; Tags []string}`. FTS body = `WorkflowName + " " + Status`.

---

### Task 4.12.2: WorkflowRun — CRUD + filter


```json
{
  "key": "task-4-12-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.12.1`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/workflow_run.go`

**Action:** `WorkflowRunFilter{WorkflowName string, Status string, ...common}`.

---

### Task 4.12.3: WorkflowRun — roundtrip test


```json
{
  "key": "task-4-12-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.12.2`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Create: `internal/graph/nodes/workflow_run_test.go`

---

### Task 4.12.4: WorkflowRun — Update produces 1 revision


```json
{
  "key": "task-4-12-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.12.3`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/workflow_run_test.go`

---

### Task 4.12.5: WorkflowRun — Delete preserves revisions


```json
{
  "key": "task-4-12-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.12.4`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/workflow_run_test.go`

---

### Task 4.12.6: WorkflowRun — FTS roundtrip


```json
{
  "key": "task-4-12-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.12.5`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/workflow_run_test.go`

---

### Task 4.12.7: WorkflowRun — List filter test


```json
{
  "key": "task-4-12-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 4.12.6`
- Parent: `Epic 4`
- Status: `open`

**Files:**
- Modify: `internal/graph/nodes/workflow_run_test.go`

---

### Task 4.12.8: WorkflowRun — commit


```json
{
  "key": "task-4-12-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-4"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 4.12.7`
- Parent: `Epic 4`
- Status: `open`

**Command:**
```bash
git add internal/graph/nodes/workflow_run.go internal/graph/nodes/workflow_run_test.go
git commit -m "feat(graph/nodes): WorkflowRun CRUD + filter"
```

---

### Task 5: Lane B — Edges, Tags, FTS Search, Revisions read


```json
{
  "key": "epic-5",
  "type": "epic",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 0,
  "dependencies": [
    "epic-3"
  ]
}
```
**Bead Mapping:**
- type: `epic`
- Priority: `1`
- Estimated Minutes: 0
- Dependencies: `Epic 3`
- Parent: `none`
- Status: `open`

---

### Task 5.1.1: Define `Edge.Type` enum and `Edge` struct


```json
{
  "key": "task-5-1-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Epic 3`, `Task 4.1`
- Parent: `Epic 5`
- Status: `open`

> Dep on Task 4.1 (Bead): downstream edge tests (5.1.5–5.1.10) construct src/dst via `nodes.CreateBead` rather than maintaining a duplicate `fakeSpec`.

**Files:**
- Create: `internal/graph/edges/edge.go` (types)

---

### Task 5.1.2: Implement `Create`


```json
{
  "key": "task-5-1-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 5.1.1`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.1.3: Implement `Delete`


```json
{
  "key": "task-5-1-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.1.2`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.1.4a: Implement `Outgoing`


```json
{
  "key": "task-5-1-4a",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 8,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 8
- Dependencies: `Task 5.1.3`
- Parent: `Epic 5`
- Status: `open`

**Files:**
- Modify: `internal/graph/edges/edge.go`

**Action:** `SELECT id, src_node_id, dst_node_id, type, attrs, created_at FROM edges WHERE src_node_id = ? AND (?1 OR type IN (...))`. Variadic `types ...Type` builds the IN clause; empty types = no filter.

---

### Task 5.1.4b: Implement `Incoming` (mirror of Outgoing)


```json
{
  "key": "task-5-1-4b",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 7,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 7
- Dependencies: `Task 5.1.4a`
- Parent: `Epic 5`
- Status: `open`

**Files:**
- Modify: `internal/graph/edges/edge.go`

**Action:** Same SQL shape as `Outgoing` but `WHERE dst_node_id = ?`. Reuse the row-scan helper from 5.1.4a to avoid duplication.

---

### Task 5.1.5: Write `TestCreateAndOutgoing`


```json
{
  "key": "task-5-1-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 5.1.4b`
- Parent: `Epic 5`
- Status: `open`

**Files:**
- Create: `internal/graph/edges/edge_test.go`

---

### Task 5.1.6: Write `TestDeleteSourceCascadesEdge`


```json
{
  "key": "task-5-1-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.1.5`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.1.7: Write `TestDeleteDestCascadesEdge`


```json
{
  "key": "task-5-1-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.1.6`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.1.8: Write `TestSelfEdgeAllowed`


```json
{
  "key": "task-5-1-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.1.7`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.1.9: Write `TestEdgeToNonexistentDestFails`


```json
{
  "key": "task-5-1-9",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.1.8`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.1.10: Write `TestOutgoingFiltersByType`


```json
{
  "key": "task-5-1-10",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.1.9`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.1.11: Commit edges


```json
{
  "key": "task-5-1-11",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.1.10`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.2.1: Implement `tags.Add`


```json
{
  "key": "task-5-2-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Epic 3`
- Parent: `Epic 5`
- Status: `open`

**Files:**
- Create: `internal/graph/tags/tags.go`

---

### Task 5.2.2: Implement `tags.Remove`/`List`/`Nodes`


```json
{
  "key": "task-5-2-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 5.2.1`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.2.3: Write `TestAddAndList`


```json
{
  "key": "task-5-2-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.2.2`
- Parent: `Epic 5`
- Status: `open`

**Files:**
- Create: `internal/graph/tags/tags_test.go`

---

### Task 5.2.4: Write `TestNodesByTag`


```json
{
  "key": "task-5-2-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.2.3`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.2.5: Write `TestRemove`


```json
{
  "key": "task-5-2-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.2.4`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.2.6: Write `TestAddEmptyTagRejected`


```json
{
  "key": "task-5-2-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.2.5`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.2.7: Write `TestCascadeOnNodeDelete`


```json
{
  "key": "task-5-2-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.2.6`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.2.8: Commit tags


```json
{
  "key": "task-5-2-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.2.7`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.3.1: Define `Hit`, `Option`, options funcs


```json
{
  "key": "task-5-3-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Epic 3`, `Task 4.1`, `Task 4.2`
- Parent: `Epic 5`
- Status: `open`

> Dep on Task 4.1 (Bead) and Task 4.2 (Note): FTS tests seed the index via real `nodes.CreateNote` / `nodes.CreateBead` fixtures.

**Files:**
- Create: `internal/graph/fts/search.go` (types)

---

### Task 5.3.2: Implement core `Search` SQL query


```json
{
  "key": "task-5-3-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 15
- Dependencies: `Task 5.3.1`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.3.3: Implement FTS error wrapping with substring check


```json
{
  "key": "task-5-3-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.3.2`
- Parent: `Epic 5`
- Status: `open`

**Implementation note:** String-match on `"fts5: syntax error"` from modernc.org/sqlite. Document exact substring.

---

### Task 5.3.4: Write `TestSearchReturnsHit`


```json
{
  "key": "task-5-3-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.3.3`
- Parent: `Epic 5`
- Status: `open`

**Files:**
- Create: `internal/graph/fts/search_test.go`

---

### Task 5.3.5: Write `TestFTSPortugueseDiacritics`


```json
{
  "key": "task-5-3-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.3.4`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.3.5a: Write `TestFTSMixedEnglishPortuguese`


```json
{
  "key": "task-5-3-5a",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.3.5`
- Parent: `Epic 5`
- Status: `open`

**Files:**
- Modify: `internal/graph/fts/search_test.go`

**Action:** Single note with a body mixing English and Portuguese (e.g., `"the cat slept on the sofá while the cachorro barked"`). Iterate `[]string{"cat","slept","sofa","cachorro"}` and assert each term yields exactly one hit. Backs test-plan §FTS5 "Mixed-language body (EN+PT) → both languages searchable" and parent plan §5.3 Step 6.

**Acceptance Criteria:**
- [ ] All four mixed-language terms return the seeded note

---

### Task 5.3.6: Write `TestFTSSpecialCharsInBody`


```json
{
  "key": "task-5-3-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.3.5a`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.3.7: Write `TestFTSEmptyQuery` (decided: empty → `ErrFTSQuerySyntax`)


```json
{
  "key": "task-5-3-7",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.3.6`
- Parent: `Epic 5`
- Status: `open`

**Action:** Per plan iteration-2 decision C5: empty string is not a valid FTS5 query. `Search(tx, "")` must return `errors.Is(err, graph.ErrFTSQuerySyntax)` without touching the DB.

**Acceptance Criteria:**
- [ ] Empty query returns wrapped `ErrFTSQuerySyntax`

---

### Task 5.3.8: Write `TestFTSMaxLengthQuery`


```json
{
  "key": "task-5-3-8",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.3.7`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.3.9: Write `TestSearchWithTypes`


```json
{
  "key": "task-5-3-9",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.3.8`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.3.10: Write `TestSearchWithTagsFilter`


```json
{
  "key": "task-5-3-10",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.3.9`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.3.11: Write `TestSearchMalformedQueryReturnsWrappedError`


```json
{
  "key": "task-5-3-11",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.3.10`
- Parent: `Epic 5`
- Status: `open`

**Action:** Use fixture `"AND"` (bare operator) — guaranteed parse error across modernc.org/sqlite versions. Do NOT use `"unbalanced` (newer FTS5 tokenizers accept it as a partial token rather than failing). Backs parent plan §5.3 Step 9.

**Acceptance Criteria:**
- [ ] `errors.Is(err, graph.ErrFTSQuerySyntax)` returns true for query `"AND"`

---

### Task 5.3.12: Commit FTS


```json
{
  "key": "task-5-3-12",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.3.11`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.4.1: Define `Revision` struct and `List`/`GetAt`


```json
{
  "key": "task-5-4-1",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Epic 3`
- Parent: `Epic 5`
- Status: `open`

**Files:**
- Create: `internal/graph/revisions/revisions.go`

**Query order:** `ORDER BY ts DESC, id DESC` (tiebreaker).

---

### Task 5.4.2: Write `TestListReturnsAllRevisions`


```json
{
  "key": "task-5-4-2",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.4.1`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.4.3: Write `TestGetAtReturnsHistoricalState`


```json
{
  "key": "task-5-4-3",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.4.2`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.4.4: Write `TestPrevRevisionChainUnbroken`


```json
{
  "key": "task-5-4-4",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 5.4.3`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.4.5: Write `TestListOrderDeterministicUnderCollision`


```json
{
  "key": "task-5-4-5",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.4.4`
- Parent: `Epic 5`
- Status: `open`

**Action:** Two rapid updates → List order matches creation order (ts collision, id tiebreaks).

---

### Task 5.4.6: Commit revisions


```json
{
  "key": "task-5-4-6",
  "type": "task",
  "priority": 3,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.4.5`
- Parent: `Epic 5`
- Status: `open`

---

### Task 5.5: Integration test suite covering spec §11 success criteria


```json
{
  "key": "epic-5-5",
  "type": "epic",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 0,
  "dependencies": [
    "epic-4",
    "epic-5"
  ]
}
```
**Bead Mapping:**
- type: `epic`
- Priority: `1`
- Estimated Minutes: 0
- Dependencies: `Epic 4`, `Epic 5`
- Parent: `none`
- Status: `open`

> One file (`internal/graph/integration_test.go`) with 8 named tests, each mapping 1:1 to a numbered item in `docs/2026-05-17-graph-substrate-brainstorm-spec.md` §11. The naming convention `TestSpec11_NN_*` makes the spec↔test link mechanically auditable via `grep`. Backs parent plan §5.5.

---

### Task 5.5.1: Write `TestSpec11_01_CreateGetBeadRoundtrip`


```json
{
  "key": "task-5-5-1",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-5-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Epic 4`, `Epic 5`
- Parent: `Task 5.5`
- Status: `open`

**Files:**
- Create: `internal/graph/integration_test.go`

**Action:** Open → `CreateBead` → `GetBead` returns identical content (covers spec §11 #1).

---

### Task 5.5.2: Write `TestSpec11_02_UpdateBeadProducesOneRevision`


```json
{
  "key": "task-5-5-2",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-5-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 5.5.1`
- Parent: `Task 5.5`
- Status: `open`

**Action:** `UpdateBead` writes exactly one new revision row with correct author and full snapshot (covers spec §11 #2).

---

### Task 5.5.3: Write `TestSpec11_03_NoteBodyFTSReturnsHit`


```json
{
  "key": "task-5-5-3",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-5-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 5.5.2`
- Parent: `Task 5.5`
- Status: `open`

**Action:** `CreateNote(body)` → `fts.Search(word)` returns it (covers spec §11 #3).

---

### Task 5.5.4: Write `TestSpec11_04_NoteBodyFTSReplacedOnUpdate`


```json
{
  "key": "task-5-5-4",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-5-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 5.5.3`
- Parent: `Task 5.5`
- Status: `open`

**Action:** `CreateNote` + `UpdateNote(newBody)` → old word returns no hits; new word returns the note (covers spec §11 #4).

---

### Task 5.5.5: Write `TestSpec11_05_EdgeCreateAndCascade`


```json
{
  "key": "task-5-5-5",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-5-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 5.5.4`
- Parent: `Task 5.5`
- Status: `open`

**Action:** `CreateEdge(A→B, depends_on)` → `Outgoing(A)` returns edge → `DeleteNode(A)` → edge removed; revisions of A preserved (covers spec §11 #5).

---

### Task 5.5.6: Write `TestSpec11_06_TagAddListNodesRemove`


```json
{
  "key": "task-5-5-6",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-5-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 5.5.5`
- Parent: `Task 5.5`
- Status: `open`

**Action:** `AddTag(node, "y")` → `tags.Nodes("y")` returns node → `RemoveTag` empties (covers spec §11 #6).

---

### Task 5.5.7: Write `TestSpec11_07_PersistenceAcrossReopen`


```json
{
  "key": "task-5-5-7",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-5-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 10
- Dependencies: `Task 5.5.6`
- Parent: `Task 5.5`
- Status: `open`

**Action:** Open tempfile → CRUD → Close → re-Open → data persists; FTS still queryable (covers spec §11 #7).

---

### Task 5.5.8: Write `TestSpec11_08_PropertyTestPasses` (thin wrapper)


```json
{
  "key": "task-5-5-8",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.5.7`
- Parent: `Task 5.5`
- Status: `open`

**Action:** Thin wrapper that calls the rapid property test from Task 6 — proves §11 #8 is covered by the integration suite (test will be implemented after Task 6 lands; defer running until then).

---

### Task 5.5.9: Commit integration suite


```json
{
  "key": "task-5-5-9",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-5-5"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `1`
- Estimated Minutes: 5
- Dependencies: `Task 5.5.8`
- Parent: `Task 5.5`
- Status: `open`

**Command:**
```bash
git add internal/graph/integration_test.go
git commit -m "test(graph): integration_test.go maps 1:1 to spec §11 success criteria"
```

**Acceptance Criteria:**
- [ ] `grep -c TestSpec11_ internal/graph/integration_test.go` returns 8
- [ ] All 8 tests pass under `go test ./internal/graph/...`

---

### Task 6: Property tests + integration sweep


```json
{
  "key": "epic-6",
  "type": "epic",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 0,
  "dependencies": [
    "epic-4",
    "epic-5"
  ]
}
```
**Bead Mapping:**
- type: `epic`
- Priority: `2`
- Estimated Minutes: 0
- Dependencies: `Epic 4`, `Epic 5`, `Task 5.5`
- Parent: `none`
- Status: `open`

---

### Task 6.1: Add `pgregory.net/rapid` dependency


```json
{
  "key": "task-6-1",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-6"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: 5
- Dependencies: `Epic 4`, `Epic 5`
- Parent: `Task 6`
- Status: `open`

**Files:**
- Modify: `go.mod`, `go.sum`

**Command:**
```bash
go get pgregory.net/rapid@latest
go mod tidy
```

---

### Task 6.2a: Define property test model struct + test scaffold (in-memory)


```json
{
  "key": "task-6-2a",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 10,
  "parent": "epic-6"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: 10
- Dependencies: `Task 6.1`
- Parent: `Task 6`
- Status: `open`

**Files:**
- Create: `internal/graph/property_test.go`

**Action:** Define `model{g *graph.Graph, nodes map[string]NodeSpec, edges map[string]edges.Edge, revCount map[string]int}`. Scaffold `TestSubstrateProperties(t)` with `rapid.Check` calling `g := testutil.NewInMemoryTestGraph(t)` (per spec §10 #3 / parent plan iter-2 decision C1). Loop placeholder calls `m.apply(rt, op)` and `m.checkInvariants(rt)` (impls TBD in 6.2b/6.3).

**Acceptance Criteria:**
- [ ] `TestSubstrateProperties` exists, compiles, uses `NewInMemoryTestGraph`
- [ ] Skeleton runs with empty `apply` and `checkInvariants` (no-ops)

---

### Task 6.2b: Implement `apply(rt, op)` dispatcher with 6 op handlers


```json
{
  "key": "task-6-2b",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 20,
  "parent": "epic-6"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: 20
- Dependencies: `Task 6.2a`
- Parent: `Task 6`
- Status: `open`

**Files:**
- Modify: `internal/graph/property_test.go`

**Action:** Implement the 6 op handlers — `create`, `update`, `delete`, `add_tag`, `add_edge`, `delete_edge` — each draws op-specific rapid inputs, runs the graph mutation via `g.DoWrite`, and updates the in-memory mirror (`nodes`, `edges`, `revCount`). Skip op gracefully if precondition fails (e.g., `update` on a node that doesn't exist in the mirror).

**Acceptance Criteria:**
- [ ] All 6 ops dispatch correctly under rapid; no panics across 100 shrunk cases

---

### Task 6.3: Implement property test `checkInvariants`


```json
{
  "key": "task-6-3",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 20,
  "parent": "epic-6"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: 20
- Dependencies: `Task 6.2b`
- Parent: `Task 6`
- Status: `open`

**Action:**
1. `sum(revCount) == count(revisions)`
2. FTS reflects current state for all live nodes
3. Deleted nodes: revisions persist, no edges/tags/fts rows
4. UUIDv7 monotone ordering
5. `prev_revision_id` chain unbroken for all nodes

---

### Task 6.4: Write `TestConcurrentWritesSerialize`


```json
{
  "key": "task-6-4",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-6"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: 15
- Dependencies: `Task 6.3`
- Parent: `Task 6`
- Status: `open`

**Files:**
- Create: `internal/graph/concurrency_test.go`

**Action:** 100 goroutines call `DoWrite` simultaneously. Assert 0 SQLITE_BUSY.

---

### Task 6.5: Write `TestReaderSeesPreDeleteSnapshot` (with `readStarted` barrier)


```json
{
  "key": "task-6-5",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-6"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: 15
- Dependencies: `Task 6.4`
- Parent: `Task 6`
- Status: `open`

**Action:** Hold ReadTx open, delete node concurrently, release read → verify reader still sees node.
**Critical race fix (parent plan §6 Step 5):** the test MUST use a `readStarted := make(chan struct{})` channel that the goroutine closes immediately AFTER `BeginTx`. Main waits on `<-readStarted` before issuing the delete. Without this barrier the test flakes — main can win the race and delete before the read tx is open, in which case the reader correctly sees the post-delete state and the test fails for the wrong reason.
**Goroutine safety:** main goroutine collects result from a buffered channel; no `t.Fatal` in spawned goroutine.

**Acceptance Criteria:**
- [ ] Test uses `readStarted` barrier; deterministic under `go test -race -count=10`

---

### Task 6.6: Write `TestBulkInsertLatency`


```json
{
  "key": "task-6-6",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 15,
  "parent": "epic-6"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: 15
- Dependencies: `Task 6.5`
- Parent: `Task 6`
- Status: `open`

**Action:** 1000 sequential node inserts. Measure aggregate time. Assert sub-ms per insert and sub-ms per query. (Profile-driven: if >50ms for the batch, flag for investigation.)

---

### Task 6.7: Run full `go test ./internal/graph/... -race -count=10 -timeout=180s`


```json
{
  "key": "task-6-7",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 30,
  "parent": "epic-6"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: 30
- Dependencies: `Task 6.6`
- Parent: `Task 6`
- Status: `open`

**Command:**
```bash
CGO_ENABLED=0 go test ./internal/graph/... -race -count=10 -timeout=180s
```

**Acceptance Criteria:**
- [ ] All tests pass
- [ ] Zero data races
- [ ] Property tests shrink to minimal reproducer on failure

---

### Task 6.8: Commit property tests + integration sweep


```json
{
  "key": "task-6-8",
  "type": "task",
  "priority": 4,
  "status": "open",
  "estimated_minutes": 5,
  "parent": "epic-6"
}
```
**Bead Mapping:**
- type: `task`
- Priority: `2`
- Estimated Minutes: 5
- Dependencies: `Task 6.7`
- Parent: `Task 6`
- Status: `open`

**Command:**
```bash
git add internal/graph/property_test.go internal/graph/concurrency_test.go go.mod go.sum
git commit -m "test(graph): property tests (rapid) + concurrency gaps #11/#12 + bulk latency"
```

---

# Summary

| Track | Tasks | Est. Total Minutes |
|---|---|---|
| Task 1 (reorg) | 1.1, 1.2, 1.3a–1.3c, 1.4–1.6 | 150 |
| Epic 2 (sqlite+migrator+schema+graph) | 2.1.1, 2.1.2a–b, 2.1.3–2.4.7 (+ 2.2.4a–b, 2.2.7a, 2.3.6a) | 365 |
| Epic 3 (chokepoint+types+errors+testutil) | 3.1.1–3.5.4 (+ 3.4.4a–d, 3.4.7a–e, 3.4.10a–c, 3.4.11a, 3.5.3a) | 380 |
| Epic 4 — Lane A (12 types, expanded) | 4.1.1–4.12.8 + 4.2.3b + 4.2.7b (98 atoms total) | ~750 |
| Epic 5 — Lane B (edges/tags/fts/revisions) | 5.1.1–5.4.6 (+ 5.1.4a–b, 5.3.5a) | 245 |
| Task 5.5 (integration §11) | 5.5.1–5.5.9 | 90 |
| Task 6 (property+concurrency+bulk) | 6.1, 6.2a–b, 6.3–6.8 | 140 |
| **Total** | **~165 atomic tasks** | **~2120 min ≈ 35 engineer-hours** |

**Contiguous block of pure development** (after Task 1.6 and Epic 3 merge, with Lanes A+B parallel): ~20 engineer-hours.

**Granularity:** every atom is now ≤ 15 min (most are 5–10). No single task carries >15 min of irreversible work; if an agent wedges on any one, the lost slot is bounded. Lane A's 12 chains are mutually independent — full parallelism after Epic 3 merges.

**CI Note:** P0.1 does not ship a `.github/workflows/graph-tests.yml`. Per iteration 2 of the plan review (see `docs/plans/2026-05-17-graph-substrate-plan.md` Notes section, decision C3), CI integration is deferred. Local `go test ./internal/graph/... -race` is the "done" gate.
