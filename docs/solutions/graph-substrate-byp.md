---
id: kernl-byp
title: Chokepoint + types + errors + testutil
epic_type: implementation
status: shipped
merged_at: 2026-05-19
pr: https://github.com/gabrielassisxyz/kernl/pull/9
---

# Compound: kernl-byp

## Why this epic existed

The graph substrate PR (#8) gave us empty tables. Before any per-type CRUD could be written (Epic 4), we needed a single chokepoint for node mutations and the supporting infrastructure (types, IDs, errors, test substrate). Without the chokepoint, 12 different node-type packages would each duplicate the same INSERT/UPDATE/DELETE logic.

## What the swarm got right

- **Chokepoint architecture** — One file (`chokepoint.go`) handles create/update/delete for all node types. Per-type CRUD (Lane A) will call into it rather than duplicating SQL.
- **Revision-first design** — Every mutation writes a revision row. Delete stores a tombstone revision, preserving audit history. The `ON DELETE SET NULL` FK correction was discovered during implementation and landed in the same PR.
- **UUIDv7 generator** — Monotonic, sortable, time-embedded IDs. No snowflake-style external dependency.
- **Test substrate** — `NewInMemoryTestGraph` isolates every test in its own shared-cache in-memory SQLite instance. `Close-while-active` test caught a real race that required `atomic.Bool` in `Graph.closed`.
- **Error sentinels** — Four typed errors (`ErrNotFound`, `ErrFTSQuerySyntax`, `ErrSchemaLocked`, `ErrAuthorRequired`) give the API surface predictable failure modes.

## What the human corrected at PR review

- **Schema fix: `revisions.node_id` FK** — The original schema defined `node_id REFERENCES nodes(id)` which prevented node deletion (revision rows blocked DELETE). Changed to `ON DELETE SET NULL` so tombstone revisions survive while allowing cascade delete on the node itself. The existing schema test was updated to assert this behavior rather than the opposite.
- **Race on `Graph.closed`** — `TestCloseWithInFlightTxsDoesNotPanic` (run with `-race`) flagged a data race between goroutines calling `DoRead` and `Close`. Fixed by replacing the plain `bool` with `atomic.Bool`.

## Design decisions worth carrying forward

1. **External-content FTS5** — `nodes_fts` is a virtual table backed by `nodes`. The chokepoint explicitly DELETE+INSERTs the FTS row on update, keeping the index in sync. This is more reliable than relying on `content=` triggers, which SQLite FTS5 does not provide automatically.
2. **Tags as a separate table with `node_tags` join** — Normalized tag storage avoids duplication. `upsertTag` inserts with `INSERT OR IGNORE INTO tags`; reconcile logic computes add/remove diffs.
3. **DiffableNode interface deferred** — `DiffableNode` exists in the type system but `updateNode` currently stores a full snapshot JSON in the `diff` column. This is correct for now; per-type diff computation can be wired in when Lane A needs it.

## Follow-up work

- Connect the chokepoint to higher-level per-type CRUD (Epic 4: Lane A).
- Build read-side queries: FTS search, edge traversal, tag listing.
- Wire `DiffableNode.DiffBody` into `updateNode` when the first concrete type implements it.

## Files

| Path | Lines | Notes |
|------|-------|-------|
| `internal/graph/nodes/chokepoint.go` | 432 | Single mutation chokepoint |
| `internal/graph/nodes/node.go` | 46 | Type contracts (NodeSpec, Meta, FTSFields, Author) |
| `internal/graph/errors.go` | 10 | Sentinel errors |
| `internal/graph/internal/ids/ids.go` | 8 | UUIDv7 wrapper |
| `internal/graph/testutil/substrate.go` | 36 | Test graph constructors |
| `internal/graph/graph.go` | +5/-5 | atomic.Bool + ErrSchemaLocked translation |
| `internal/graph/schema/0001_init.up.sql` | 2 bytes | FK change: ON DELETE SET NULL |
| `internal/graph/schema/schema_test.go` | +13/-4 | Assert tombstone survival |
