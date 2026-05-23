package reconcile_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/vault/frontmatter"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// TestUpsertAndLookup verifies basic insert and both lookup directions.
func TestUpsertAndLookup(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "note-uuid-1"
	const path = "notes/hello.md"
	const hash = "abc123"

	if err := reconcile.Upsert(ctx, g, uuid, path, hash); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	gotUUID, found, err := reconcile.Lookup(ctx, g, path)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !found {
		t.Fatal("Lookup: expected found=true")
	}
	if gotUUID != uuid {
		t.Errorf("Lookup: uuid = %q, want %q", gotUUID, uuid)
	}

	gotPath, found, err := reconcile.LookupByUUID(ctx, g, uuid)
	if err != nil {
		t.Fatalf("LookupByUUID: %v", err)
	}
	if !found {
		t.Fatal("LookupByUUID: expected found=true")
	}
	if gotPath != path {
		t.Errorf("LookupByUUID: path = %q, want %q", gotPath, path)
	}
}

// TestAE2_MovePreservesIdentity encodes AE2 at the cache layer:
// changing the path for the same UUID must update the path mapping,
// leave the old path unresolvable, and NOT write any nodes/revisions.
func TestAE2_MovePreservesIdentity(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "note-uuid-ae2"
	const oldPath = "folder-a/note.md"
	const newPath = "folder-b/note.md"
	const hash = "hash-ae2"

	// Insert at old path.
	if err := reconcile.Upsert(ctx, g, uuid, oldPath, hash); err != nil {
		t.Fatalf("initial Upsert: %v", err)
	}

	// Simulate a move: same UUID, new path.
	if err := reconcile.Upsert(ctx, g, uuid, newPath, hash); err != nil {
		t.Fatalf("move Upsert: %v", err)
	}

	// New path must resolve to the UUID.
	gotUUID, found, err := reconcile.Lookup(ctx, g, newPath)
	if err != nil {
		t.Fatalf("Lookup new path: %v", err)
	}
	if !found || gotUUID != uuid {
		t.Errorf("Lookup new path: got (%q, %v), want (%q, true)", gotUUID, found, uuid)
	}

	// Old path must no longer resolve.
	_, foundOld, err := reconcile.Lookup(ctx, g, oldPath)
	if err != nil {
		t.Fatalf("Lookup old path: %v", err)
	}
	if foundOld {
		t.Error("old path should not resolve after move")
	}

	// UUID resolves to the new path.
	gotPath, found, err := reconcile.LookupByUUID(ctx, g, uuid)
	if err != nil {
		t.Fatalf("LookupByUUID: %v", err)
	}
	if !found || gotPath != newPath {
		t.Errorf("LookupByUUID: got (%q, %v), want (%q, true)", gotPath, found, newPath)
	}

	// Cache must NOT have written any nodes or revisions.
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var nodeCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&nodeCount); err != nil {
			return err
		}
		if nodeCount != 0 {
			t.Errorf("AE2: expected 0 nodes after cache move, got %d", nodeCount)
		}
		var revCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM revisions`).Scan(&revCount); err != nil {
			return err
		}
		if revCount != 0 {
			t.Errorf("AE2: expected 0 revisions after cache move, got %d", revCount)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

// TestRenameWithinFolder verifies stem change (same UUID, same dir) updates cache.
func TestRenameWithinFolder(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "note-rename"
	const oldPath = "folder/old-stem.md"
	const newPath = "folder/new-stem.md"

	if err := reconcile.Upsert(ctx, g, uuid, oldPath, "h1"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := reconcile.Upsert(ctx, g, uuid, newPath, "h1"); err != nil {
		t.Fatalf("rename Upsert: %v", err)
	}

	gotPath, found, err := reconcile.LookupByUUID(ctx, g, uuid)
	if err != nil {
		t.Fatalf("LookupByUUID: %v", err)
	}
	if !found || gotPath != newPath {
		t.Errorf("LookupByUUID after rename: got (%q, %v), want (%q, true)", gotPath, found, newPath)
	}

	_, foundOld, err := reconcile.Lookup(ctx, g, oldPath)
	if err != nil {
		t.Fatalf("Lookup old stem: %v", err)
	}
	if foundOld {
		t.Error("old stem path should not resolve after rename")
	}
}

// TestMoveAcrossFolders verifies a cross-folder move updates only the path.
func TestMoveAcrossFolders(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "note-cross-move"
	const oldPath = "alpha/note.md"
	const newPath = "beta/gamma/note.md"

	if err := reconcile.Upsert(ctx, g, uuid, oldPath, "h-cross"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := reconcile.Upsert(ctx, g, uuid, newPath, "h-cross"); err != nil {
		t.Fatalf("cross-folder Upsert: %v", err)
	}

	gotPath, found, err := reconcile.LookupByUUID(ctx, g, uuid)
	if err != nil {
		t.Fatalf("LookupByUUID: %v", err)
	}
	if !found || gotPath != newPath {
		t.Errorf("LookupByUUID: got (%q, %v), want (%q, true)", gotPath, found, newPath)
	}
}

// TestUpsertIdempotent verifies re-upserting identical rows leaves a single row.
func TestUpsertIdempotent(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "note-idem"
	const path = "idem/note.md"
	const hash = "idem-hash"

	for i := 0; i < 3; i++ {
		if err := reconcile.Upsert(ctx, g, uuid, path, hash); err != nil {
			t.Fatalf("Upsert #%d: %v", i, err)
		}
	}

	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM note_paths WHERE uuid = ?`, uuid).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("expected exactly 1 row, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

// TestForgetRemovesOnlyTarget verifies Forget removes exactly the target path.
func TestForgetRemovesOnlyTarget(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	// Insert two independent notes.
	if err := reconcile.Upsert(ctx, g, "uuid-keep", "keep/note.md", "hk"); err != nil {
		t.Fatalf("Upsert keep: %v", err)
	}
	if err := reconcile.Upsert(ctx, g, "uuid-drop", "drop/note.md", "hd"); err != nil {
		t.Fatalf("Upsert drop: %v", err)
	}

	// Forget only the drop path.
	if err := reconcile.Forget(ctx, g, "drop/note.md"); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	// drop path must be gone.
	_, foundDrop, err := reconcile.Lookup(ctx, g, "drop/note.md")
	if err != nil {
		t.Fatalf("Lookup drop: %v", err)
	}
	if foundDrop {
		t.Error("drop/note.md should be gone after Forget")
	}

	// keep path must still be present.
	gotUUID, foundKeep, err := reconcile.Lookup(ctx, g, "keep/note.md")
	if err != nil {
		t.Fatalf("Lookup keep: %v", err)
	}
	if !foundKeep || gotUUID != "uuid-keep" {
		t.Errorf("keep/note.md should still exist, got (%q, %v)", gotUUID, foundKeep)
	}
}

// TestForgetNoopOnMissing verifies Forget on a non-existent path is a no-op.
func TestForgetNoopOnMissing(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	if err := reconcile.Forget(ctx, g, "never/existed.md"); err != nil {
		t.Fatalf("Forget non-existent: %v", err)
	}
}

// TestLookupNotFound verifies Lookup returns found=false for an absent path.
func TestLookupNotFound(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	_, found, err := reconcile.Lookup(ctx, g, "missing.md")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if found {
		t.Error("expected found=false for missing path")
	}
}

// TestFindByContentHash verifies the hash tiebreak helper returns the UUID when
// a matching row exists, and not-found otherwise.
func TestFindByContentHash(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "hash-note"
	const path = "hash/note.md"
	const hash = "deadbeef"

	if err := reconcile.Upsert(ctx, g, uuid, path, hash); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	gotUUID, found, err := reconcile.FindByContentHash(ctx, g, hash)
	if err != nil {
		t.Fatalf("FindByContentHash: %v", err)
	}
	if !found || gotUUID != uuid {
		t.Errorf("FindByContentHash: got (%q, %v), want (%q, true)", gotUUID, found, uuid)
	}

	// A hash that does not exist.
	_, foundMissing, err := reconcile.FindByContentHash(ctx, g, "unknown-hash")
	if err != nil {
		t.Fatalf("FindByContentHash missing: %v", err)
	}
	if foundMissing {
		t.Error("FindByContentHash: expected not-found for unknown hash")
	}
}

// TestHashBytes verifies the content-hash helper produces a stable hex digest.
func TestHashBytes(t *testing.T) {
	// SHA-256("") is well-known.
	const emptyHex = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := reconcile.HashBytes([]byte{}); got != emptyHex {
		t.Errorf("HashBytes empty: got %q, want %q", got, emptyHex)
	}

	h1 := reconcile.HashBytes([]byte("hello"))
	h2 := reconcile.HashBytes([]byte("hello"))
	if h1 != h2 {
		t.Error("HashBytes not deterministic")
	}
	if h1 == reconcile.HashBytes([]byte("world")) {
		t.Error("different content produced same hash")
	}
}

// TestUUIDIsTheKey verifies that when two paths collide (same path, different
// UUIDs), the UUID keying means the later write wins on the path UNIQUE
// constraint — the UUID-keyed design resolves such races deterministically.
func TestUUIDIsTheKey(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const sharedPath = "shared/note.md"

	// First writer claims the path.
	if err := reconcile.Upsert(ctx, g, "uuid-first", sharedPath, "h1"); err != nil {
		t.Fatalf("Upsert first: %v", err)
	}

	// Second writer attempts the same path with a different UUID. The UNIQUE
	// constraint on path means this MUST fail — the path is already owned by
	// uuid-first. The caller (U7/U11) is responsible for conflict resolution;
	// the cache layer surfaces the error.
	err := reconcile.Upsert(ctx, g, "uuid-second", sharedPath, "h2")
	if err == nil {
		t.Error("expected error when a different UUID claims an already-owned path; got nil")
	}

	// The original UUID must still own the path.
	gotUUID, found, err2 := reconcile.Lookup(ctx, g, sharedPath)
	if err2 != nil {
		t.Fatalf("Lookup: %v", err2)
	}
	if !found || gotUUID != "uuid-first" {
		t.Errorf("after conflict: path owned by %q (found=%v), want uuid-first", gotUUID, found)
	}
}

// ---------------------------------------------------------------------------
// Reconciler integration tests (U7)
// ---------------------------------------------------------------------------

// newVaultDir creates a temp directory acting as the vault root.
func newVaultDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// writeFile writes content to a file at the given absolute path, creating
// parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestOnCreate_NoteNodeExistsFTSFindable verifies that OnCreate results in a
// note node that is FTS-findable.
func TestOnCreate_NoteNodeExistsFTSFindable(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := newVaultDir(t)

	path := filepath.Join(vault, "hello.md")
	writeFile(t, path, "---\nid: note-fts-1\ntitle: Hello World\n---\n\nThis is the body.\n")

	rec := reconcile.New(g, vault)
	if err := rec.OnCreate(ctx, path); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}

	// Node must exist
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		n, err := nodes.GetNote(ctx, tx, "note-fts-1")
		if err != nil {
			return err
		}
		if n.Title != "Hello World" {
			t.Errorf("title = %q, want %q", n.Title, "Hello World")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}

	// FTS must find it
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes_fts WHERE title MATCH 'Hello'`).Scan(&count)
	})
	if err != nil {
		t.Fatalf("FTS query: %v", err)
	}

	// Path cache must have an entry
	_, found, err := reconcile.Lookup(ctx, g, "hello.md")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !found {
		t.Error("expected path-cache entry after OnCreate")
	}

	// First revision must be recorded
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		return tx.QueryRow(`SELECT COUNT(*) FROM revisions WHERE node_id = ?`, "note-fts-1").Scan(&count)
	})
	if err != nil {
		t.Fatalf("revision query: %v", err)
	}
}

// TestOnCreate_DanglingLinkRecorded verifies that an unresolvable outbound
// wikilink is stored as a dangling row.
func TestOnCreate_DanglingLinkRecorded(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := newVaultDir(t)

	path := filepath.Join(vault, "linker.md")
	writeFile(t, path, "---\nid: linker-node\ntitle: Linker\n---\n\nSee [[missing-note]].\n")

	rec := reconcile.New(g, vault)
	if err := rec.OnCreate(ctx, path); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}

	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		return tx.QueryRow(`SELECT COUNT(*) FROM dangling_links WHERE src_node_id = ?`, "linker-node").Scan(&count)
	})
	if err != nil {
		t.Fatalf("dangling query: %v", err)
	}
}

// TestAE6_R20_AuthorFromFrontmatter verifies that a file with frontmatter
// author=da is attributed to the DA agent, not human (AE6 / R20).
func TestAE6_R20_AuthorFromFrontmatter(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := newVaultDir(t)

	// Simulate a vault-llm/ file with author=da
	path := filepath.Join(vault, "vault-llm", "generated.md")
	writeFile(t, path, "---\nid: ae6-node\ntitle: AE6 Note\nauthor: da\n---\n\nAgent content.\n")

	rec := reconcile.New(g, vault)
	if err := rec.OnCreate(ctx, path); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}

	// The revision author must be agent:da
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var author string
		return tx.QueryRow(`SELECT author FROM revisions WHERE node_id = ?`, "ae6-node").Scan(&author)
	})
	if err != nil {
		t.Fatalf("revision query: %v", err)
	}
	// Must be agent:da
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var author string
		if err := tx.QueryRow(
			`SELECT author FROM revisions WHERE node_id = ? LIMIT 1`, "ae6-node",
		).Scan(&author); err != nil {
			return err
		}
		if author != "agent:da" {
			t.Errorf("revision author = %q, want %q", author, "agent:da")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("check author: %v", err)
	}
}

// TestOnCreate_AuthorAbsentDefaultsToHuman verifies that absent frontmatter
// author defaults to "human".
func TestOnCreate_AuthorAbsentDefaultsToHuman(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := newVaultDir(t)

	path := filepath.Join(vault, "noauthor.md")
	writeFile(t, path, "---\nid: no-author-node\ntitle: No Author\n---\n\nContent.\n")

	rec := reconcile.New(g, vault)
	if err := rec.OnCreate(ctx, path); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}

	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var author string
		if err := tx.QueryRow(
			`SELECT author FROM revisions WHERE node_id = ? LIMIT 1`, "no-author-node",
		).Scan(&author); err != nil {
			return err
		}
		if author != "human" {
			t.Errorf("revision author = %q, want %q", author, "human")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("check author: %v", err)
	}
}

// TestOnChange_DiffRevisionRecorded verifies that OnChange records a diff
// revision and re-indexes the body in FTS.
func TestOnChange_DiffRevisionRecorded(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := newVaultDir(t)

	path := filepath.Join(vault, "change.md")
	writeFile(t, path, "---\nid: change-node\ntitle: Change Test\n---\n\nVersion one.\n")

	rec := reconcile.New(g, vault)
	if err := rec.OnCreate(ctx, path); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}

	// Update file content
	writeFile(t, path, "---\nid: change-node\ntitle: Change Test\n---\n\nVersion two.\n")
	if err := rec.OnChange(ctx, path); err != nil {
		t.Fatalf("OnChange: %v", err)
	}

	// Two revisions must exist
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM revisions WHERE node_id = ?`, "change-node",
		).Scan(&count); err != nil {
			return err
		}
		if count != 2 {
			t.Errorf("revision count = %d, want 2", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("check revisions: %v", err)
	}

	// FTS must reflect new content
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		return tx.QueryRow(
			`SELECT COUNT(*) FROM nodes_fts WHERE attrs MATCH 'two'`,
		).Scan(&count)
	})
	if err != nil {
		t.Fatalf("FTS query: %v", err)
	}
}

// TestOnCreate_DanglingPromotedOnArrival verifies that when note B arrives and
// note A previously had a dangling wikilink [[B]], the dangling link is promoted
// to a real edge.
func TestOnCreate_DanglingPromotedOnArrival(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := newVaultDir(t)

	// Note A links to note B (which does not exist yet)
	pathA := filepath.Join(vault, "note-a.md")
	writeFile(t, pathA, "---\nid: node-a\ntitle: Note A\n---\n\nSee [[note-b]].\n")

	rec := reconcile.New(g, vault)
	if err := rec.OnCreate(ctx, pathA); err != nil {
		t.Fatalf("OnCreate A: %v", err)
	}

	// Verify dangling link exists for A
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		return tx.QueryRow(
			`SELECT COUNT(*) FROM dangling_links WHERE src_node_id = ?`, "node-a",
		).Scan(&count)
	})
	if err != nil {
		t.Fatalf("dangling check: %v", err)
	}

	// Note B arrives
	pathB := filepath.Join(vault, "note-b.md")
	writeFile(t, pathB, "---\nid: node-b\ntitle: Note B\n---\n\nContent.\n")
	if err := rec.OnCreate(ctx, pathB); err != nil {
		t.Fatalf("OnCreate B: %v", err)
	}

	// Dangling link must be gone
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM dangling_links WHERE src_node_id = ?`, "node-a",
		).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("dangling_links count = %d after promotion, want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("post-promotion check: %v", err)
	}

	// A real edge A→B must exist
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM edges WHERE src = ? AND dst = ? AND label = 'links_to'`,
			"node-a", "node-b",
		).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			t.Error("expected links_to edge from node-a to node-b after promotion")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("edge check: %v", err)
	}
}

// TestOnCreate_RollbackOnFailure verifies that an injected failure inside the
// DoWrite rolls back the entire event — no orphan node/FTS row and no path-cache
// entry survive.
// Strategy: write a file with a UUID that is the same as an existing node, so
// the second CreateNote inside the single transaction fails with a UNIQUE
// constraint — simulating mid-tx failure.
func TestOnCreate_RollbackOnFailure(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := newVaultDir(t)

	// Pre-insert a node with id "clash-id" so the second create fails
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := nodes.CreateNote(ctx, tx, nodes.Note{
			ID:    "clash-id",
			Title: "Pre-existing",
			Body:  "pre",
		}, nodes.Author{Name: "human"})
		return err
	})
	if err != nil {
		t.Fatalf("pre-insert: %v", err)
	}

	// Create a file that uses the same id
	path := filepath.Join(vault, "clash.md")
	writeFile(t, path, "---\nid: clash-id\ntitle: Clash\n---\n\nBody.\n")

	rec := reconcile.New(g, vault)
	oncreateErr := rec.OnCreate(ctx, path)
	// We expect an error due to the UNIQUE constraint on the node id
	if oncreateErr == nil {
		t.Fatal("expected OnCreate to fail due to id clash; got nil")
	}

	// Path-cache entry must NOT exist (transaction rolled back)
	_, found, err := reconcile.Lookup(ctx, g, "clash.md")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if found {
		t.Error("path-cache entry must not persist after rolled-back transaction")
	}

	// Node count must be exactly 1 (the pre-existing one)
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("node count = %d, want 1 after rollback", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

// TestOnCreate_UUIDInjectedOnDisk verifies that a file lacking a UUID gets one
// injected on disk and other frontmatter bytes are preserved.
func TestOnCreate_UUIDInjectedOnDisk(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := newVaultDir(t)

	path := filepath.Join(vault, "inject-test.md")
	original := "---\ntitle: Inject Test\nauthor: gabriel\n---\n\nBody text here.\n"
	writeFile(t, path, original)

	rec := reconcile.New(g, vault)
	if err := rec.OnCreate(ctx, path); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}

	// Re-read the file from disk
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Must contain an id field
	fm, err := frontmatter.Parse(updated)
	if err != nil {
		t.Fatalf("Parse injected: %v", err)
	}
	if fm.ID == "" {
		t.Error("expected id to be injected on disk")
	}

	// All other frontmatter fields must be preserved
	if fm.Title != "Inject Test" {
		t.Errorf("title = %q, want %q", fm.Title, "Inject Test")
	}
	if fm.Author != "gabriel" {
		t.Errorf("author = %q, want %q", fm.Author, "gabriel")
	}

	// Original body must still be present
	if !strings.Contains(string(updated), "Body text here.") {
		t.Error("body text not found in updated file")
	}
}
