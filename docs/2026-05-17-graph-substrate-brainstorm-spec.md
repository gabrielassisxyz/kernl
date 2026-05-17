---
name: P0.1 — Graph substrate (schema + SQLite layer)
date: 2026-05-17
source: docs/VISION.md §6, docs/suggested-vision-projects.md P0.1
status: brainstorm-spec (ready for vc-plan)
---

# P0.1 — Graph substrate (schema + SQLite layer)

## 1. One-line summary

The typed knowledge graph that every other Kernl module reads from and writes
to: a single SQLite database with a polymorphic `nodes` table, directed typed
`edges`, an auto-written `revisions` audit log, open `tags`, and an FTS5
search index — all behind a single Go package (`internal/graph`) that exposes
typed CRUD per node type and forces every mutation through a chokepoint that
writes the revision and updates FTS in the same transaction.

## 2. Why this exists

VISION §6 defines Kernl as a unified product whose unifier is the substrate:
one graph in which Bead, Note, Project, Task, Bookmark, MemoryClaim, and
WorkflowRun are all first-class nodes that share queries, links, and time
travel. P0.1 is the foundation: nothing else can be built until this layer
exists. It is intentionally minimal — storage and CRUD, no graph algorithms
(those land in P0.3) and no filesystem watcher (P0.2).

## 3. Scope

### In scope
- SQLite schema for `nodes`, `edges`, `tags`, `node_tags`, `revisions`,
  `nodes_fts`, `nodes_fts_map`, `schema_migrations`.
- Pure-Go SQLite driver (`modernc.org/sqlite`).
- Go package `internal/graph` exposing:
  - Typed CRUD per node type (`Bead`, `Note`, `Project`, `Task`, `Session`,
    `Decision`, `MemoryClaim`, `MemoryRefutation`, `Bookmark`,
    `BookmarkList`, `Capture`, `WorkflowRun`).
  - Directed typed edges with optional JSON payload.
  - Tags (open vocabulary, many-to-many).
  - Revisions (audit log, written automatically on every mutation).
  - FTS5 search over node title/body/tags.
  - Read-only and read-write transactions as distinct types.
- Embedded migrations via `golang-migrate` and `go:embed`.
- Hermetic test helpers (in-memory + tempfile).
- Optional `owner_id` / `visibility` columns, nullable, invisible in
  single-user mode (VISION §16).

### Out of scope (deferred to other projects)
| Concern | Where |
|---|---|
| Filesystem watcher, UUID injection in note frontmatter, path↔uuid cache | P0.2 |
| 5-second auto-diff writes for notes (the *writer* of `revisions` for notes) | P0.2 |
| 4-signal relevance, Adamic-Adar, Louvain, graph insights | P0.3 |
| Cache table for inferred / computed edges (`inferred_edges`) | P0.3 |
| Additive-only write contract for `MemoryClaim` / `MemoryRefutation` | P2.2 |
| Defuddle / content extraction for bookmarks | P2.3 |
| Backup / restore tooling | DevEx (later) |
| Multi-user auth, conflict resolution, sharing UI | Not now (VISION §16) |
| HTTP API exposing the substrate | P1.1 (DA) and P2.6 (GUI shell) |

## 4. Repository reorganization (pre-step)

The current repo has `orchestrator/` as its own Go module
(`orchestrator/go.mod`). If `internal/graph/` is added to the root, the
orchestrator cannot import it as `kernl/internal/graph` without
cross-module gymnastics.

**Decision:** before any schema work, promote the repo to a single Go
module at the root. This is the right structural shape for what Kernl
will become (multiple binaries — `kernl`, `kernl-orchestrator` standalone,
future `kernl-da`, etc. — all sharing internal packages).

**The move:**
- Create `go.mod` at the repo root (`module github.com/<owner>/kernl`).
- Move `orchestrator/cmd/` → `cmd/`.
- Move `orchestrator/internal/` → `internal/orchestrator/`.
- Move `orchestrator/web/` → `web/`.
- Delete `orchestrator/go.mod`, `orchestrator/go.sum`.
- Update every import path (mechanical: `gomvpkg` / sed + `goimports`).
- Update CI workflows, `scripts/swarm/*`, `Makefile`-equivalents that
  reference `orchestrator/...` paths.
- Run `go build ./...` + full test suite.

**Estimate:** 2-4 hours, mostly mechanical. Half a day if surprises
appear (hardcoded paths, embeds with absolute references).

This is **step 0** of P0.1 — a separate, isolated PR before any schema
work begins. Reviewer focus: nothing should change behaviorally; only
file locations and import paths.

## 5. Package layout

After reorganization:

```
kernl/
  go.mod
  cmd/
    orchestrator/      (was orchestrator/cmd/orchestrator/)
    kernl/             (was orchestrator/cmd/kernl/)
  internal/
    orchestrator/      (was orchestrator/internal/)
    graph/             <-- NEW, the substrate
      graph.go         // Open(cfg) (*Graph, error), Close(), DoRead/DoWrite, ReadTx/WriteTx
      schema/          // .sql migrations embedded via go:embed
        0001_init.up.sql
        0001_init.down.sql
        ...
      nodes/
        node.go        // NodeSpec interface, Meta struct, DiffableNode interface
        bead.go        // Bead struct + Create/Get/Update/Delete/List
        note.go        // implements DiffableNode
        project.go
        task.go
        session.go
        decision.go
        memory_claim.go
        memory_refutation.go
        bookmark.go
        bookmark_list.go
        capture.go
        workflow_run.go
      edges/
        edge.go        // Type enum, Edge struct, Create/Delete/Outgoing/Incoming
      tags/
        tags.go        // Add/Remove/List/Nodes
      revisions/
        revisions.go   // append (internal-only), Read helpers (List by node, GetAt)
      fts/
        search.go      // Search(query, opts...) ([]Hit, error)
      internal/
        sqlite/        // db handle, pragmas, tx wrappers, in-memory + tempfile setup
        ids/           // UUIDv7 generation
      testutil/
        substrate.go   // NewTestGraph(t) *Graph with cleanup
```

Only `internal/graph/graph.go` exports `Open` and the top-level `*Graph`
type. Other packages of Kernl never open the `*sql.DB` directly — they
go through `*Graph` and the typed APIs.

## 6. Schema

### 6.1 Tables

```sql
-- Nodes: single polymorphic table for the closed set of types
CREATE TABLE nodes (
  id          TEXT PRIMARY KEY,            -- UUIDv7 (time-ordered)
  type        TEXT NOT NULL,
  title       TEXT NOT NULL DEFAULT '',
  attrs       TEXT NOT NULL DEFAULT '{}',  -- JSON1, type-specific fields
  created_at  INTEGER NOT NULL,            -- unix milliseconds
  updated_at  INTEGER NOT NULL,
  owner_id    TEXT,                        -- NULL in single-user (VISION §16)
  visibility  TEXT,                        -- NULL in single-user
  CHECK (type IN ('note','bead','project','task','session','decision',
                  'memory_claim','memory_refutation','bookmark',
                  'bookmark_list','capture','workflow_run')),
  CHECK (json_valid(attrs))
);
CREATE INDEX nodes_type_updated_idx ON nodes(type, updated_at DESC);
CREATE INDEX nodes_updated_at_idx   ON nodes(updated_at DESC);

-- Edges: directed, closed type vocabulary, optional JSON payload
CREATE TABLE edges (
  id          TEXT PRIMARY KEY,            -- UUIDv7
  src_node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  dst_node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  type        TEXT NOT NULL,
  attrs       TEXT NOT NULL DEFAULT '{}',  -- payload, e.g. {weight: 0.7}
  created_at  INTEGER NOT NULL,
  CHECK (type IN ('depends_on','parent_of','inspired_by','mentions',
                  'generated_from','processed_into','processed_from',
                  'refutes','relates_to')),
  CHECK (json_valid(attrs))
);
CREATE INDEX edges_src_idx ON edges(src_node_id, type);
CREATE INDEX edges_dst_idx ON edges(dst_node_id, type);

-- Tags: open vocabulary
CREATE TABLE tags (name TEXT PRIMARY KEY);

CREATE TABLE node_tags (
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  tag     TEXT NOT NULL REFERENCES tags(name) ON DELETE CASCADE,
  PRIMARY KEY (node_id, tag)
);
CREATE INDEX node_tags_tag_idx ON node_tags(tag, node_id);

-- Revisions: append-only audit log; written by the API chokepoint, never
-- by callers directly
CREATE TABLE revisions (
  id               TEXT PRIMARY KEY,        -- UUIDv7
  node_id          TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  prev_revision_id TEXT REFERENCES revisions(id),
  author           TEXT NOT NULL,           -- 'human' | 'da' | 'agent:<id>'
  snapshot         TEXT NOT NULL,           -- full {title, attrs, tags:[...]} JSON
  diff             TEXT,                    -- optional textual diff (DiffableNode types only)
  ts               INTEGER NOT NULL,
  CHECK (json_valid(snapshot))
);
CREATE INDEX revisions_node_ts_idx ON revisions(node_id, ts DESC);

-- FTS5: contentless virtual table; FTS does not store the text itself.
-- Updates are written by the same chokepoint that writes revisions.
CREATE VIRTUAL TABLE nodes_fts USING fts5(
  title, body, tags,
  content='',
  tokenize = 'unicode61 remove_diacritics 2'
);

-- Mapping between node UUID and FTS5 rowid (FTS5 rowid is INTEGER)
CREATE TABLE nodes_fts_map (
  fts_rowid INTEGER PRIMARY KEY AUTOINCREMENT,
  node_id   TEXT NOT NULL UNIQUE REFERENCES nodes(id) ON DELETE CASCADE
);
```

### 6.2 Pragmas (applied in `Open`)

```
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;
PRAGMA temp_store = MEMORY;
```

For in-memory test mode, `journal_mode=MEMORY` instead of WAL.

### 6.3 Design notes
- **No `UNIQUE(src,dst,type)` on edges.** Multiple mentions of the same
  target from the same source at different times are legitimate. If a
  module needs uniqueness, it enforces it itself.
- **`attrs` validated only by `json_valid`.** Per-type structure is
  enforced by the Go layer via struct binding. No CHECK constraints
  per type — too noisy to maintain.
- **`owner_id` / `visibility` are nullable.** In single-user mode (today),
  both are always NULL. The schema preserves them for VISION §16
  optionality without any runtime cost.

## 7. Go API design

### 7.1 NodeSpec interface

Implemented by every typed struct (`Bead`, `Note`, ...). The only thing
the core needs to know about types:

```go
package nodes

type NodeSpec interface {
    NodeType() string                    // 'bead', 'note', ... (closed set)
    NodeTitle() string
    NodeAttrs() (json.RawMessage, error) // serialize type-specific fields
    NodeTags() []string
    FTSFields() FTSFields                // what goes into the FTS5 index
}

type FTSFields struct{ Title, Body, Tags string }

// Meta is set by the core, not by typed structs. Embedded into each type.
type Meta struct {
    ID         string
    CreatedAt  time.Time
    UpdatedAt  time.Time
    OwnerID    *string
    Visibility *string
}

// Optional: types that want diff (not just snapshot) implement this.
type DiffableNode interface {
    NodeSpec
    DiffBody(prev NodeSpec) string  // unified diff of body text vs prev
}
```

Only `Note` implements `DiffableNode` at MVP.

### 7.2 Per-type CRUD example (Bead)

```go
package nodes

type Bead struct {
    Meta
    Title       string
    Description string
    Status      string
    Priority    int
    Tags        []string
}

func (b *Bead) NodeType() string                    { return "bead" }
func (b *Bead) NodeTitle() string                   { return b.Title }
func (b *Bead) NodeAttrs() (json.RawMessage, error) { /* serialize fields except Meta + Title */ }
func (b *Bead) NodeTags() []string                  { return b.Tags }
func (b *Bead) FTSFields() FTSFields {
    return FTSFields{b.Title, b.Description, strings.Join(b.Tags, " ")}
}

func CreateBead(ctx context.Context, tx WriteTx, b *Bead, author Author) error
func GetBead   (ctx context.Context, tx ReadTx,  id string) (*Bead, error)
func UpdateBead(ctx context.Context, tx WriteTx, b *Bead, author Author) error
func DeleteBead(ctx context.Context, tx WriteTx, id string, author Author) error
func ListBeads (ctx context.Context, tx ReadTx,  filter BeadFilter) ([]*Bead, error)
```

Twelve types × ~60 lines each = ~720 lines of repetitive but explicit
typed code. Adding a new type means a new file plus a CHECK constraint
migration — a controlled, reviewable change.

### 7.3 Author is mandatory on every mutation

```go
type Author string

const (
    AuthorHuman Author = "human"
    AuthorDA    Author = "da"
)

func AuthorAgent(id string) Author { return Author("agent:" + id) }
```

Every `Create` / `Update` / `Delete` requires an `Author`. No default. This
forces callers to be explicit about who is mutating state — required by
VISION §7.2 for the "DA wrote here" ribbon.

### 7.4 The chokepoint — internal `updateNode`

All `UpdateXxx` route to `updateNode(ctx, tx, NodeSpec, Author)`. Within a
**single transaction**:

```
1. Load current node from `nodes` (for prev snapshot + DiffableNode prev).
2. UPDATE nodes SET title=?, attrs=?, updated_at=? WHERE id=?.
3. Reconcile node_tags (insert new, delete removed).
4. INSERT INTO revisions with:
     snapshot = full new state {title, attrs, tags:[...]}
     diff     = node.DiffBody(prev) if NodeSpec implements DiffableNode, else NULL
     author   = caller-supplied
5. DELETE FROM nodes_fts WHERE rowid = (SELECT fts_rowid FROM nodes_fts_map WHERE node_id=?)
   INSERT INTO nodes_fts (rowid, title, body, tags) VALUES (?, ?, ?, ?)
```

`createNode` and `deleteNode` follow the same chokepoint pattern with the
appropriate variations:
- `createNode` writes an initial revision (snapshot only, no diff).
- `deleteNode` writes a final "tombstone" revision (snapshot of last
  state, author set) **before** deleting — so the audit log survives
  the cascading delete.

**The structural guarantee:** there is no public API path that mutates a
node without writing a revision and updating FTS in the same transaction.
Modules that change Bead/Task/etc. cannot "forget" to write a revision —
the only way to mutate is the chokepoint, and the chokepoint writes the
revision.

### 7.5 Edges API

```go
package edges

type Type string
const (
    DependsOn      Type = "depends_on"
    ParentOf       Type = "parent_of"
    InspiredBy     Type = "inspired_by"
    Mentions       Type = "mentions"
    GeneratedFrom  Type = "generated_from"
    ProcessedInto  Type = "processed_into"
    ProcessedFrom  Type = "processed_from"
    Refutes        Type = "refutes"
    RelatesTo      Type = "relates_to"
)

type Edge struct {
    ID        string
    SrcID     string
    DstID     string
    Type      Type
    Attrs     json.RawMessage
    CreatedAt time.Time
}

func Create  (ctx context.Context, tx WriteTx, src, dst string, t Type, payload any) (*Edge, error)
func Delete  (ctx context.Context, tx WriteTx, edgeID string) error
func Outgoing(ctx context.Context, tx ReadTx,  nodeID string, types ...Type) ([]*Edge, error)
func Incoming(ctx context.Context, tx ReadTx,  nodeID string, types ...Type) ([]*Edge, error)
```

**Edges do not have revisions or an `edge_events` audit log.** To "change"
an edge, delete + create. If audit becomes a real need later, an
`edge_events` table can be added without touching the rest of the
schema. Memory-domain edges that need additive semantics enforce them at
the module level (P2.2), not the substrate.

### 7.6 Transactions

`*Graph` exposes:

```go
func (g *Graph) ReadTx (ctx context.Context) (ReadTx, error)
func (g *Graph) WriteTx(ctx context.Context) (WriteTx, error)

// Convenience for single-shot:
func (g *Graph) DoRead (ctx context.Context, fn func(ReadTx)  error) error
func (g *Graph) DoWrite(ctx context.Context, fn func(WriteTx) error) error
```

`ReadTx` and `WriteTx` are distinct interface types:
- Compile-time check: `UpdateBead(tx ReadTx, ...)` does not compile.
- Forces callers to think about read vs write (matters in WAL — reads
  do not block writes).
- A `sync.Mutex` inside `*Graph` serializes `WriteTx` acquisition to
  avoid `SQLITE_BUSY` even though `busy_timeout=5000ms` would also
  handle it. Belt and suspenders, zero cost for a single-user system.

### 7.7 Tags

```go
package tags

func Add   (ctx context.Context, tx WriteTx, nodeID, tag string) error
func Remove(ctx context.Context, tx WriteTx, nodeID, tag string) error
func List  (ctx context.Context, tx ReadTx,  nodeID string) ([]string, error)
func Nodes (ctx context.Context, tx ReadTx,  tag string) ([]string, error)
```

Tags are reconciled implicitly by `updateNode` from `NodeSpec.NodeTags()`,
but the package is exposed for ad-hoc tag operations.

### 7.8 FTS5 search

```go
package fts

type Hit struct {
    NodeID string
    Type   string
    Title  string
    Rank   float64  // bm25
}

func Search(ctx context.Context, tx ReadTx, query string, opts ...Option) ([]Hit, error)

// Options:
func WithTypes(types ...string) Option
func WithLimit(n int) Option
func WithTagsFilter(tags ...string) Option
```

Pure textual search. **All relevance algorithms (4-signal, Adamic-Adar,
etc.) are P0.3, not P0.1.**

## 8. Migrations

- Tool: `github.com/golang-migrate/migrate/v4` + `sqlite` driver.
- Files: `internal/graph/schema/NNNN_name.up.sql` + `.down.sql`, embedded
  via `go:embed schema/*.sql`.
- Tracked via the standard `schema_migrations` table.
- `Open` applies all pending up migrations automatically.
- `.down.sql` is **always** written and tested in CI (`up → down → up`
  must succeed cleanly) but is never invoked at runtime.

## 9. Identity

- All IDs are **UUIDv7**: time-ordered, sortable by creation time,
  index-locality friendly, avoids the need for a separate `created_at`
  column to order by. Generated server-side in `internal/graph/internal/ids`.
- For notes, the user-facing UUID lives in the markdown frontmatter
  (VISION §6.3), injected by **P0.2's watcher** — not by P0.1. P0.1's
  `CreateNote` is called by P0.2 with the UUID it has already chosen.

## 10. Testing strategy (hermetic)

Three layers:

1. **Unit tests (in-memory).** `Open(Config{InMemory: true})`. Each test
   creates a fresh DB, runs migrations, exercises one operation, discards.
   Validates schema, CRUD per type, edges, tags, FTS5 basics. Target
   < 50ms per test.
2. **Integration tests (tempfile).** `Open(Config{Path: t.TempDir()+"/test.db"})`.
   Exercises persistence concerns: WAL behavior, reopen, migration up/down
   round-trips. `t.Cleanup` removes.
3. **Property tests (in-memory).** Random sequences of Create/Update/Delete
   on nodes and edges, verifying invariants:
   - Every node mutation produces exactly one revision row.
   - FTS5 search reflects current state (no stale entries after Update).
   - `ON DELETE CASCADE` removes edges/tags/fts_map/revisions of deleted
     nodes.
   - UUIDv7s are monotonically increasing within a single process.

**No mocks for the DB.** `*Graph` is the real thing in every test. Only
clock (`time.Now` → injectable) and ID generation (for determinism in a
few tests) are injectable.

A helper `internal/graph/testutil.NewTestGraph(t)` returns a ready
`*Graph` with `t.Cleanup` wired — same pattern as the existing
`runstate/store_test.go`.

## 11. Success criteria (proof-of-life)

P0.1 is "done" when the following end-to-end tests pass without depending
on P0.2 / P0.3 / any other Kernl module:

1. `Open` → `CreateBead` → `GetBead` returns the same content.
2. `UpdateBead` produces exactly one new row in `revisions` with correct
   author and complete snapshot.
3. `CreateNote` with body text → `fts.Search("word from body")` returns
   that note.
4. `CreateNote` followed by `UpdateNote` with a different body →
   `fts.Search("old word")` returns nothing; `fts.Search("new word")`
   returns the note. (Proves FTS5 update path through the chokepoint.)
5. `CreateEdge(beadA → beadB, depends_on)` → `Outgoing(beadA)` returns
   the edge; `DeleteNode(beadA)` cascades and removes the edge.
6. `AddTag(note, "x")` → `tags.Nodes("x")` returns the note;
   `RemoveTag` reverses.
7. `Close` then re-`Open` of a tempfile DB → all data persists; FTS5
   still answers queries.
8. Property test: 1000 random ops preserve all invariants from §10.

## 12. Open questions (deferred decisions worth flagging)

1. **`vault-llm/` subgraph isolation.** VISION §7.4 mentions LLM-generated
   notes living in a separate subgraph. P0.1 does not introduce a
   `subgraph_id` column; the application layer (P1.1 DA, P2.5 Ingest)
   uses tags or `attrs.subgraph` as a soft mechanism. Revisit if real
   isolation requirements emerge.
2. **Schema validation per type.** Today `attrs` is validated by
   `json_valid` only. If runtime drift between Go structs and DB
   contents becomes a real problem, add JSON Schema validation at the
   Go layer (still not in the DB) — but this is YAGNI for now.
3. **Tokenizer for non-English text.** Starting with `unicode61
   remove_diacritics 2` — works well for mixed Portuguese/English. If
   stemming becomes important, evaluate per-language tokenizers; do not
   add Porter (English-only, hurts Portuguese) globally.
4. **Edge audit log.** Decided against `edge_events` for now; revisit if
   real debugging or security needs emerge.
5. **Backup / restore.** Out of scope; will be a separate DevEx
   sub-project. `.kernl/graph.db` is a SQLite file — `cp` works as a
   minimum.

## 13. References

- VISION.md §6.1 (form: typed graph, closed node types)
- VISION.md §6.2 (storage split: notes on FS, everything else in SQLite)
- VISION.md §6.3 (identity by UUID)
- VISION.md §6.4 (SQLite as the substrate's home; FTS5; JSON1)
- VISION.md §7.2 (revision log, author attribution)
- VISION.md §7.3 (additive memory model — relevant for P2.2, not P0.1)
- VISION.md §16 (multi-user posture: optional `owner_id` / `visibility`)
- suggested-vision-projects.md P0.1 (scope definition)
